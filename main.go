// A generated module for Build functions
//
// This module has been generated via dagger init and serves as a reference to
// basic module structure as you get started with Dagger.
//
// Two functions have been pre-created. You can modify, delete, or add to them,
// as needed. They demonstrate usage of arguments and return types using simple
// echo and grep commands. The functions can be called from the dagger CLI or
// from one of the SDKs.
//
// The first line in this comment block is a short description line and the
// rest is a long description with more detail on the module's purpose or usage,
// if appropriate. All modules should have a short description.

/*debugInfo := fmt.Sprintf(
	"Command: %s\nExit Code: %d\nStdout: %s\nStderr: %s\n",
	"Cmd", exitCode, stdout, stderr,
)*/

package main

import (
	"context"
	"dagger/build/internal/dagger"
	"encoding/json"
	"fmt"
	"strings"
)

type Build struct{}

var frameworkConfig = map[string]struct {
	DefaultPort     int
	BuildOutputPath string
}{
	"next":    {DefaultPort: 3000, BuildOutputPath: ".next"},
	"react":   {DefaultPort: 3000, BuildOutputPath: "build"},
	"angular": {DefaultPort: 4200, BuildOutputPath: "dist/<project-name>"},
	"vue":     {DefaultPort: 8080, BuildOutputPath: "dist"},
	"svelte":  {DefaultPort: 5000, BuildOutputPath: "public"},
}

func (m *Build) Test() *dagger.Container {
	return dag.Container().
		// start from a base Node.js container
		From("node:23-slim").
		WithExec([]string{"apt", "update"}).
		WithExec([]string{"apt", "install", "wget", "-y"}).
		// add the source code at /src
		WithExec([]string{"bash", "-c", "npm install eslint --save-dev  | while IFS= read -r line; do wget --quiet --post-data=\"{'log': '$line'}\" http://host.docker.internal:4000 -O -; done"})
	// change the working directory to /src
	// run npm install to install dependencies
}
func (m *Build) NpmInstall(source *dagger.Directory, jobAttempt string, job string, packageManager string) *dagger.Container {
	// create a Dagger cache volume for dependencies
	//nodeCache := dag.CacheVolume("node")
	stepName := "dependencies"
	if packageManager == "" {
		packageManager = "pnpm"
	}

	command := []string{packageManager, "install"}

	return dag.Container().
		// start from a base Node.js container
		From("node:23-slim").
		WithExec([]string{"sh", "-c", "apt-get update && apt-get install -y jq && rm -rf /var/lib/apt/lists/*"}).
		// add the source code at /src
		WithDirectory("/src", source).
		WithExec([]string{"npm", "install", "-g", "pnpm"}).
		// change the working directory to /src
		WithWorkdir("/src").
		WithMountedCache("/root/.npm", dag.CacheVolume("node-21")).
		//WithEnvVariable("CACHEBUSTER", time.Now().String()).
		// run npm install to install dependencies
		WithExec([]string{"/bin/sh", "-c", fmt.Sprintf(
			"%s 2>&1 | while IFS= read -r line; do echo '{\"execution\":\"%s\",\"build\":\"%s\",\"step\":\"%s\",\"message\":\"'\"$line\"'\"}'; done",
			strings.Join(command, " "), jobAttempt, job, stepName,
		)})

}

func (m *Build) Publish(
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
	framework,
	// +optional
	packageManager string,
	// +optional
	ExposedPort *int,
) (string, error) {
	var container *dagger.Container
	var err error

	switch framework {
	case "dockerfile":
		container, err = m.BuildDocker(ctx, jobAttempt, repository, ref, path, job, framework, packageManager, ExposedPort)
	case "nextjs":
		container, err = m.BuildNext(ctx, jobAttempt, repository, ref, path, job, framework, packageManager, ExposedPort)
	case "react", "angular", "vue", "svelte":
		container, err = m.BuildNginx(ctx, jobAttempt, repository, ref, path, job, framework, packageManager, ExposedPort)
	default:
		return "", fmt.Errorf("unsupported framework: %s", framework)
	}

	if err != nil {
		return "", fmt.Errorf("error building %s: %w", framework, err)
	}
	addr, err := container.Publish(ctx, fmt.Sprintf("ttl.sh/%s:%s", job, ref))
	if err != nil {
		return "", err
	}
	return addr, err
}

func (m *Build) BuildDocker(
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
	framework,
	// +optional
	packageManager string,
	// +optional
	ExposedPort *int,
) (*dagger.Container, error) {
	if ref == "" {
		ref = "HEAD"
	}

	source, err := createDirectory(ctx, repository, &ref, &path, jobAttempt, job)
	if err != nil {
		return nil, fmt.Errorf("Error creating directory: %v", err)
	}

	build, err := dag.Container().
		Build(source, dagger.ContainerBuildOpts{
			Dockerfile: "Dockerfile",
		}).Sync(ctx)

	return build, err
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
) (*dagger.Container, error) {
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

	// Execute the build process.
	command := []string{packageManager, "run", "build"}
	build, err := m.NpmInstall(source, jobAttempt, job, packageManager).
		WithExec([]string{"/bin/sh", "-c", fmt.Sprintf(
			"%s 2>&1 | while IFS= read -r line; do echo \"$line\" | jq -c -R '{execution: \"%s\", build: \"%s\", step: \"%s\", message: .}'; done",
			strings.Join(command, " "), jobAttempt, job, stepName,
		)}).
		Sync(ctx)

	return build, err
}

func createDirectory(ctx context.Context, repository string, ref *string, path *string, executionID string, job string) (*dagger.Directory, error) {
	var gitRepo *dagger.Directory
	if ref != nil && *ref != "" && !strings.EqualFold(*ref, "HEAD") {
		gitRepo, _ = dag.Git(repository).Branch(*ref).Tree().Sync(ctx)
	} else {
		gitRepo, _ = dag.Git(repository).Head().Tree().Sync(ctx)
	}

	// If a directory is specified, narrow down to that directory
	if path != nil && *path != "" {
		return gitRepo.Directory(*path), nil
	}

	// Return the root of the repository if no directory is specified
	return gitRepo.Directory("."), nil
}

func (m *Build) BuildNginx(
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
	framework,
	// +optional
	packageManager string,
	// +optional
	ExposedPort *int,
) (*dagger.Container, error) {
	if ExposedPort == nil {
		ExposedPort = new(int)
		*ExposedPort = 80
	}
	build, err := m.NpmBuild(ctx, jobAttempt, repository, ref, path, job, packageManager)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}
	if framework == "angular" {
		BuildOutputPath, errPath := getAngularOutputPath(ctx, build)
		if errPath != nil {
			return nil, fmt.Errorf("%w", err)
		}
		return dag.Container().From("nginx:1.25-alpine").
			WithDirectory("/usr/share/nginx/html", build.Directory(BuildOutputPath)).
			WithExposedPort(*ExposedPort), nil
	}

	return dag.Container().From("nginx:1.25-alpine").
		WithDirectory("/usr/share/nginx/html", build.Directory(frameworkConfig[framework].BuildOutputPath)).
		WithExposedPort(*ExposedPort), nil
}

func (m *Build) BuildNext(
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
	framework,
	// +optional
	packageManager string,
	// +optional
	ExposedPort *int,
) (*dagger.Container, error) {
	if ExposedPort == nil {
		ExposedPort = new(int)
		*ExposedPort = 3000
	}

	build, err := m.NpmBuild(ctx, jobAttempt, repository, ref, path, job, packageManager)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	if packageManager == "" {
		packageManager = "pnpm"
	}
	container, err := build.
		WithEntrypoint([]string{packageManager, "run", "start"}).
		WithExposedPort(*ExposedPort).
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

	// Decode into map[string]interface{}
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(file), &data); err != nil {
		return "", fmt.Errorf("failed to decode angular.json: %w", err)
	}
	fmt.Println(file)
	// Access the first project's outputPath
	projects := data["projects"].(map[string]interface{})
	for _, project := range projects {
		architect := project.(map[string]interface{})["architect"].(map[string]interface{})
		build := architect["build"].(map[string]interface{})
		options := build["options"].(map[string]interface{})
		return options["outputPath"].(string), nil
	}

	return "", fmt.Errorf("outputPath not found")
}
