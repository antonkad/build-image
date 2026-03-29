// Multi-language build pipeline for kad.dev
//
// Builds container images from Git repositories. Each language ecosystem
// (JS, Go, Python, Rust) lives in its own file and registers framework
// configs via init(). Publish() routes to the right builder based on
// the framework's Builder field.

package main

import (
	"context"
	"dagger/build/internal/dagger"
	"fmt"
	"strings"

	telemetry "github.com/dagger/otel-go"
	"go.opentelemetry.io/otel/attribute"
)

type Build struct{}

// FrameworkConfig defines how to build and run a framework.
// Each language file registers its frameworks into the global map via init().
type FrameworkConfig struct {
	// Builder strategy: "static-nginx", "node-server", "go-binary", "python-server", "rust-binary"
	Builder string

	// Build phase
	BaseImage       string // build stage image (e.g. "node:23-slim", "golang:1.24-alpine")
	RuntimeImage    string // runtime stage image for multi-stage builds (e.g. "gcr.io/distroless/static")
	BuildOutputPath string // where build output lands (e.g. "dist", ".next", "build")

	// Runtime phase
	StartCmd    []string // entrypoint command (e.g. ["pnpm", "run", "start"])
	DefaultPort int      // default exposed port
}

// frameworks is the unified config map. Each language file adds entries via init().
var frameworks = map[string]FrameworkConfig{}

func (m *Build) Test() *dagger.Container {
	return dag.Container().
		From("node:23-slim").
		WithExec([]string{"apt", "update"}).
		WithExec([]string{"apt", "install", "wget", "-y"}).
		WithExec([]string{"bash", "-c", "npm install eslint --save-dev  | while IFS= read -r line; do wget --quiet --post-data=\"{'log': '$line'}\" http://host.docker.internal:4000 -O -; done"})
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
	framework string,
	// +optional
	packageManager string,
	// +optional
	ExposedPort *int,
	// Image name for the registry (e.g. project ID)
	imageName string,
	// Commit hash used as the image tag
	commitHash string,
	// Registry URL (e.g. 192.168.1.150:30082)
	registryUrl string,
	// Registry username
	registryUser string,
	// Registry password
	registryPassword *dagger.Secret,
) (_ string, rerr error) {
	var container *dagger.Container
	var err error

	if framework == "dockerfile" {
		container, err = m.BuildDocker(ctx, jobAttempt, repository, ref, path, job)
	} else {
		cfg, ok := frameworks[framework]
		if !ok {
			return "", fmt.Errorf("unsupported framework: %s", framework)
		}
		switch cfg.Builder {
		case "static-nginx":
			container, err = m.BuildStaticNginx(ctx, jobAttempt, repository, ref, path, job, framework, packageManager, ExposedPort)
		case "node-server":
			container, err = m.BuildNodeServer(ctx, jobAttempt, repository, ref, path, job, framework, packageManager, ExposedPort)
		case "go-binary":
			container, err = m.BuildGoBinary(ctx, jobAttempt, repository, ref, path, job, framework, ExposedPort)
		default:
			return "", fmt.Errorf("unsupported builder %q for framework %q", cfg.Builder, framework)
		}
	}

	if err != nil {
		return "", fmt.Errorf("error building %s: %w", framework, err)
	}

	// "publish" span only covers the container publish to registry
	ctx, span := Tracer().Start(ctx, "publish")
	span.SetAttributes(attribute.String("kad.jobAttempt", jobAttempt))
	defer telemetry.End(span, func() error { return rerr })

	// Use short commit hash as tag for unique, addressable images
	imageTag := commitHash
	if len(imageTag) > 7 {
		imageTag = imageTag[:7]
	}
	imageRef := fmt.Sprintf("%s/%s:%s", registryUrl, imageName, imageTag)
	addr, err := container.
		WithRegistryAuth(registryUrl, registryUser, registryPassword).
		Publish(ctx, imageRef)
	if err != nil {
		return "", err
	}
	return addr, err
}

func (m *Build) BuildDocker(
	ctx context.Context,
	jobAttempt string,
	repository string,
	ref string,
	path string,
	job string,
) (*dagger.Container, error) {
	if ref == "" {
		ref = "HEAD"
	}

	source, err := createDirectory(ctx, repository, &ref, &path, jobAttempt, job)
	if err != nil {
		return nil, fmt.Errorf("Error creating directory: %v", err)
	}

	build, err := source.
		DockerBuild(dagger.DirectoryDockerBuildOpts{
			Dockerfile: "Dockerfile",
		}).Sync(ctx)

	return build, err
}

func createDirectory(ctx context.Context, repository string, ref *string, path *string, executionID string, job string) (_ *dagger.Directory, rerr error) {
	ctx, span := Tracer().Start(ctx, "git")
	span.SetAttributes(attribute.String("kad.jobAttempt", executionID))
	defer telemetry.End(span, func() error { return rerr })

	var gitRepo *dagger.Directory
	var err error
	if ref != nil && *ref != "" && !strings.EqualFold(*ref, "HEAD") {
		gitRepo, err = dag.Git(repository).Branch(*ref).Tree().Sync(ctx)
	} else {
		gitRepo, err = dag.Git(repository).Head().Tree().Sync(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("git clone failed: %w", err)
	}

	if path != nil && *path != "" {
		return gitRepo.Directory(*path), nil
	}

	return gitRepo.Directory("."), nil
}
