package main

import (
	"context"
	"dagger/build/internal/dagger"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	telemetry "github.com/dagger/otel-go"
	"go.opentelemetry.io/otel/attribute"
)

var nodeMajorVersionRe = regexp.MustCompile(`(\d+)`)

func init() {
	// Static (npm build → copy output to nginx)
	frameworks["react"] = FrameworkConfig{
		Builder:         "static-nginx",
		BuildOutputPath: "build",
		DefaultPort:     8080,
	}
	frameworks["vue"] = FrameworkConfig{
		Builder:         "static-nginx",
		BuildOutputPath: "dist",
		DefaultPort:     8080,
	}
	frameworks["svelte"] = FrameworkConfig{
		Builder:         "static-nginx",
		BuildOutputPath: "public",
		DefaultPort:     8080,
	}
	frameworks["angular"] = FrameworkConfig{
		Builder:         "static-nginx",
		BuildOutputPath: "", // resolved from angular.json
		DefaultPort:     8080,
	}

	frameworks["astro"] = FrameworkConfig{
		Builder:         "static-nginx",
		BuildOutputPath: "dist",
		DefaultPort:     8080,
	}
	frameworks["vite"] = FrameworkConfig{
		Builder:         "static-nginx",
		BuildOutputPath: "dist",
		DefaultPort:     8080,
	}
	frameworks["gatsby"] = FrameworkConfig{
		Builder:         "static-nginx",
		BuildOutputPath: "public",
		DefaultPort:     8080,
	}

	// Server (npm build → node runtime)
	frameworks["nextjs"] = FrameworkConfig{
		Builder:         "node-server",
		BuildOutputPath: ".next",
		StartCmd:        []string{"pnpm", "run", "start"},
		DefaultPort:     3000,
	}
	frameworks["nuxt"] = FrameworkConfig{
		Builder:         "node-server",
		BuildOutputPath: ".output",
		StartCmd:        []string{"node", ".output/server/index.mjs"},
		DefaultPort:     3000,
	}
	frameworks["remix"] = FrameworkConfig{
		Builder:         "node-server",
		BuildOutputPath: "build",
		StartCmd:        []string{"pnpm", "run", "start"},
		DefaultPort:     3000,
	}
	frameworks["sveltekit"] = FrameworkConfig{
		Builder:         "node-server",
		BuildOutputPath: "build",
		StartCmd:        []string{"node", "build"},
		DefaultPort:     3000,
	}
}

// resolveNodeVersion determines the Node.js Docker image tag to use.
// Priority: explicit override → .nvmrc → .node-version → package.json engines.node → "22" (LTS).
// Returns a full image reference like "node:22-slim".
func resolveNodeVersion(ctx context.Context, source *dagger.Directory, override string) string {
	const fallback = "node:22-slim"

	// 1. Explicit override wins
	if override != "" {
		return fmt.Sprintf("node:%s-slim", override)
	}

	// 2. Try .nvmrc
	if contents, err := source.File(".nvmrc").Contents(ctx); err == nil {
		if v := extractMajorVersion(strings.TrimSpace(contents)); v != "" {
			return fmt.Sprintf("node:%s-slim", v)
		}
	}

	// 3. Try .node-version
	if contents, err := source.File(".node-version").Contents(ctx); err == nil {
		if v := extractMajorVersion(strings.TrimSpace(contents)); v != "" {
			return fmt.Sprintf("node:%s-slim", v)
		}
	}

	// 4. Try package.json engines.node
	if contents, err := source.File("package.json").Contents(ctx); err == nil {
		var pkg struct {
			Engines struct {
				Node string `json:"node"`
			} `json:"engines"`
		}
		if err := json.Unmarshal([]byte(contents), &pkg); err == nil && pkg.Engines.Node != "" {
			if v := extractMajorVersion(pkg.Engines.Node); v != "" {
				return fmt.Sprintf("node:%s-slim", v)
			}
		}
	}

	// 5. Fallback to LTS
	return fallback
}

// extractMajorVersion pulls the first major version number from a version string.
// Examples: "v22.12.0" → "22", "^20.19 || ^22" → "20", ">=24" → "24", "18.x" → "18".
func extractMajorVersion(s string) string {
	// Strip leading "v" prefix
	s = strings.TrimPrefix(s, "v")
	if m := nodeMajorVersionRe.FindString(s); m != "" {
		return m
	}
	return ""
}

func (m *Build) NpmInstall(ctx context.Context, source *dagger.Directory, jobAttempt string, job string, packageManager string, dependenciesCmd string, nodeImage string) *dagger.Container {
	ctx, span := Tracer().Start(ctx, "dependencies")
	span.SetAttributes(attribute.String("kad.jobAttempt", jobAttempt))
	defer span.End()

	stepName := "dependencies"
	if packageManager == "" {
		packageManager = "pnpm"
	}

	// Use custom install command if provided, otherwise default to "<packageManager> install"
	var commandStr string
	if dependenciesCmd != "" {
		commandStr = dependenciesCmd
	} else {
		commandStr = strings.Join([]string{packageManager, "install"}, " ")
	}

	install, _ := dag.Container().
		From(nodeImage).
		WithExec([]string{"sh", "-c", "apt-get update && apt-get install -y jq && rm -rf /var/lib/apt/lists/*"}).
		WithDirectory("/src", source).
		WithExec([]string{"npm", "install", "-g", "pnpm"}).
		WithWorkdir("/src").
		WithMountedCache("/root/.npm", dag.CacheVolume("node-21")).
		WithEnvVariable("CACHEBUSTER", time.Now().String()).
		WithExec([]string{"/bin/sh", "-c", fmt.Sprintf(
			"%s 2>&1 | while IFS= read -r line; do echo '{\"jobAttempt\":\"%s\",\"job\":\"%s\",\"step\":\"%s\",\"message\":\"'\"$line\"'\"}'; done",
			commandStr, jobAttempt, job, stepName,
		)}).
		Sync(ctx)

	return install
}

func (m *Build) NpmBuild(
	ctx context.Context,
	// +optional
	jobAttempt string,
	repository string,
	// +optional
	ref string,
	// +optional
	path string,
	// +optional
	job string,
	// +optional
	packageManager string,
	// +optional
	dependenciesCmd string,
	// +optional
	buildCmd string,
	// +optional
	// Override Node.js major version (e.g. "22"). Auto-detected if omitted.
	runtimeVersion string,
) (_ *dagger.Container, rerr error) {
	stepName := "build"

	if packageManager == "" {
		packageManager = "pnpm"
	}
	if ref == "" {
		ref = "HEAD"
	}
	source, err := createDirectory(ctx, repository, &ref, &path, jobAttempt, job)
	if err != nil {
		return nil, fmt.Errorf("Error creating directory: %v", err)
	}

	nodeImage := resolveNodeVersion(ctx, source, runtimeVersion)

	installed := m.NpmInstall(ctx, source, jobAttempt, job, packageManager, dependenciesCmd, nodeImage)

	// "build" span only covers the actual build command
	ctx, span := Tracer().Start(ctx, "build")
	span.SetAttributes(attribute.String("kad.jobAttempt", jobAttempt))
	defer telemetry.End(span, func() error { return rerr })

	// Use custom build command if provided, otherwise default to "<packageManager> run build"
	var commandStr string
	if buildCmd != "" {
		commandStr = buildCmd
	} else {
		commandStr = strings.Join([]string{packageManager, "run", "build"}, " ")
	}

	build, err := installed.
		WithExec([]string{"/bin/sh", "-c", fmt.Sprintf(
			"%s 2>&1 | while IFS= read -r line; do echo \"$line\" | jq -c -R '{jobAttempt: \"%s\", job: \"%s\", step: \"%s\", message: .}'; done",
			commandStr, jobAttempt, job, stepName,
		)}).
		Sync(ctx)

	return build, err
}

func nginxConf(port int) string {
	return fmt.Sprintf(`server {
    listen %d;
    server_name _;
    root /usr/share/nginx/html;
    index index.html;
    location / {
        try_files $uri $uri/ /index.html;
    }
}`, port)
}

// BuildStaticNginx builds a JS app and serves the output with nginx.
// Used for: react, vue, svelte, angular, vite, astro (static), gatsby, etc.
func (m *Build) BuildStaticNginx(
	ctx context.Context,
	// +optional
	jobAttempt string,
	repository string,
	// +optional
	ref string,
	// +optional
	path string,
	// +optional
	job string,
	framework string,
	// +optional
	packageManager string,
	// +optional
	dependenciesCmd string,
	// +optional
	buildCmd string,
	// +optional
	// Override Node.js major version (e.g. "22"). Auto-detected if omitted.
	runtimeVersion string,
	// +optional
	exposedPort *int,
) (*dagger.Container, error) {
	cfg := frameworks[framework]
	if exposedPort == nil {
		exposedPort = new(int)
		*exposedPort = cfg.DefaultPort
	}

	build, err := m.NpmBuild(ctx, jobAttempt, repository, ref, path, job, packageManager, dependenciesCmd, buildCmd, runtimeVersion)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	// Angular has a special output path resolution
	if framework == "angular" {
		outputPath, errPath := getAngularOutputPath(ctx, build)
		if errPath != nil {
			return nil, fmt.Errorf("%w", errPath)
		}
		return dag.Container().From("nginx:1.25-alpine").
			WithDirectory("/usr/share/nginx/html", build.Directory(outputPath)).
			WithNewFile("/etc/nginx/conf.d/default.conf", nginxConf(*exposedPort)).
			WithExposedPort(*exposedPort), nil
	}

	outputPath := cfg.BuildOutputPath
	if _, err := build.Directory(outputPath).Entries(ctx); err != nil {
		return nil, fmt.Errorf("expected output directory %q not found for framework %q — check your project's framework setting", outputPath, framework)
	}

	return dag.Container().From("nginx:1.25-alpine").
		WithDirectory("/usr/share/nginx/html", build.Directory(outputPath)).
		WithNewFile("/etc/nginx/conf.d/default.conf", nginxConf(*exposedPort)).
		WithExposedPort(*exposedPort), nil
}

// BuildNodeServer builds a JS app and runs it with node.
// Used for: nextjs, nuxt, remix, sveltekit, astro (SSR), etc.
func (m *Build) BuildNodeServer(
	ctx context.Context,
	// +optional
	jobAttempt string,
	repository string,
	// +optional
	ref string,
	// +optional
	path string,
	// +optional
	job string,
	framework string,
	// +optional
	packageManager string,
	// +optional
	dependenciesCmd string,
	// +optional
	buildCmd string,
	// +optional
	// Override Node.js major version (e.g. "22"). Auto-detected if omitted.
	runtimeVersion string,
	// +optional
	exposedPort *int,
) (*dagger.Container, error) {
	cfg := frameworks[framework]
	if exposedPort == nil {
		exposedPort = new(int)
		*exposedPort = cfg.DefaultPort
	}

	build, err := m.NpmBuild(ctx, jobAttempt, repository, ref, path, job, packageManager, dependenciesCmd, buildCmd, runtimeVersion)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	if cfg.BuildOutputPath != "" {
		if _, err := build.Directory(cfg.BuildOutputPath).Entries(ctx); err != nil {
			return nil, fmt.Errorf("expected %s directory not found — check your project's framework setting (currently %q)", cfg.BuildOutputPath, framework)
		}
	}

	startCmd := cfg.StartCmd
	if len(startCmd) == 0 {
		if packageManager == "" {
			packageManager = "pnpm"
		}
		startCmd = []string{packageManager, "run", "start"}
	}

	container, err := build.
		WithEntrypoint(startCmd).
		WithExposedPort(*exposedPort).
		Sync(ctx)
	if err != nil {
		return nil, fmt.Errorf("run Build: %w", err)
	}

	return container, nil
}

func getAngularOutputPath(ctx context.Context, build *dagger.Container) (string, error) {
	file, err := build.Directory(".").File("/angular.json").Contents(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to read angular.json: %w", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(file), &data); err != nil {
		return "", fmt.Errorf("failed to decode angular.json: %w", err)
	}

	projects := data["projects"].(map[string]interface{})
	for _, project := range projects {
		architect := project.(map[string]interface{})["architect"].(map[string]interface{})
		build := architect["build"].(map[string]interface{})
		options := build["options"].(map[string]interface{})
		return options["outputPath"].(string), nil
	}

	return "", fmt.Errorf("outputPath not found")
}
