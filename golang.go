package main

import (
	"context"
	"dagger/build/internal/dagger"
	"fmt"

	telemetry "github.com/dagger/otel-go"
	"go.opentelemetry.io/otel/attribute"
)

func init() {
	frameworks["go"] = FrameworkConfig{
		Builder:      "go-binary",
		BaseImage:    "golang:1.24-alpine",
		RuntimeImage: "gcr.io/distroless/static",
		StartCmd:     []string{"/app"},
		DefaultPort:  8080,
	}
}

// BuildGoBinary builds a Go app into a static binary and packages it in a minimal distroless image.
// Multi-stage: golang:1.24-alpine (build) → gcr.io/distroless/static (runtime).
// Covers all Go frameworks (Gin, Fiber, Echo, net/http).
func (m *Build) BuildGoBinary(
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
	// Override the default install command (e.g. "go mod tidy && go mod download")
	dependenciesCmd string,
	// +optional
	// Override the default build command (e.g. "go build -o /app ./cmd/server")
	buildCmd string,
	// +optional
	exposedPort *int,
) (_ *dagger.Container, rerr error) {
	cfg := frameworks[framework]

	if exposedPort == nil {
		exposedPort = new(int)
		*exposedPort = cfg.DefaultPort
	}
	if ref == "" {
		ref = "HEAD"
	}

	source, err := createDirectory(ctx, repository, &ref, &path, jobAttempt, job)
	if err != nil {
		return nil, fmt.Errorf("error creating directory: %v", err)
	}

	// Dependencies step
	depCmd := "go mod download -x"
	if dependenciesCmd != "" {
		depCmd = dependenciesCmd
	}

	ctx, depSpan := Tracer().Start(ctx, "dependencies")
	depSpan.SetAttributes(attribute.String("kad.jobAttempt", jobAttempt))

	builder := dag.Container().
		From(cfg.BaseImage).
		WithDirectory("/src", source).
		WithWorkdir("/src").
		WithMountedCache("/go/pkg/mod", dag.CacheVolume("go-mod")).
		WithMountedCache("/root/.cache/go-build", dag.CacheVolume("go-build")).
		WithExec([]string{"/bin/sh", "-c", fmt.Sprintf(
			`%s 2>&1 | while IFS= read -r line; do echo '{"jobAttempt":"%s","job":"%s","step":"dependencies","message":"'"$line"'"}'; done`,
			depCmd, jobAttempt, job,
		)})

	builder, err = builder.Sync(ctx)
	depSpan.End()
	if err != nil {
		return nil, fmt.Errorf("dependencies failed: %w", err)
	}

	// Build step
	bCmd := "go build -v -o /out/app ."
	if buildCmd != "" {
		bCmd = buildCmd
	}

	ctx, buildSpan := Tracer().Start(ctx, "build")
	buildSpan.SetAttributes(attribute.String("kad.jobAttempt", jobAttempt))
	defer telemetry.End(buildSpan, func() error { return rerr })

	builder, err = builder.
		WithEnvVariable("CGO_ENABLED", "0").
		WithExec([]string{"/bin/sh", "-c", fmt.Sprintf(
			`set -o pipefail; %s 2>&1 | while IFS= read -r line; do echo '{"jobAttempt":"%s","job":"%s","step":"build","message":"'"$line"'"}'; done`,
			bCmd, jobAttempt, job,
		)}).
		Sync(ctx)
	if err != nil {
		return nil, fmt.Errorf("go build failed: %w", err)
	}

	// Runtime stage: copy binary to distroless
	runtime, err := dag.Container().
		From(cfg.RuntimeImage).
		WithFile("/app", builder.File("/out/app")).
		WithEntrypoint(cfg.StartCmd).
		WithExposedPort(*exposedPort).
		Sync(ctx)
	if err != nil {
		return nil, fmt.Errorf("runtime container failed: %w", err)
	}

	return runtime, nil
}
