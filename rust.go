package main

import (
	"context"
	"dagger/build/internal/dagger"
	"fmt"

	telemetry "github.com/dagger/otel-go"
	"go.opentelemetry.io/otel/attribute"
)

func init() {
	frameworks["rust"] = FrameworkConfig{
		Builder:      "rust-binary",
		BaseImage:    "rust:1.83-slim",
		RuntimeImage: "gcr.io/distroless/cc",
		StartCmd:     []string{"/app"},
		DefaultPort:  3000,
	}
}

// BuildRustBinary builds a Rust app into a release binary and packages it in a minimal distroless image.
// Multi-stage: rust:1.77-slim (build) → gcr.io/distroless/cc (runtime).
// Covers all Rust web frameworks (Axum, Actix, Rocket, Warp).
// The binary name is extracted from Cargo.toml automatically.
func (m *Build) BuildRustBinary(
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
	// Override the default install command (not typically used for Rust)
	dependenciesCmd string,
	// +optional
	// Override the default build command (e.g. "cargo build --release --bin myapp")
	buildCmd string,
	// +optional
	// Override runtime version (unused for Rust currently)
	runtimeVersion string,
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

	// Dependencies step — fetch crates into the cargo registry cache
	depCmd := "cargo fetch"
	if dependenciesCmd != "" {
		depCmd = dependenciesCmd
	}

	ctx, depSpan := Tracer().Start(ctx, "dependencies")
	depSpan.SetAttributes(attribute.String("kad.jobAttempt", jobAttempt))

	builder := dag.Container().
		From(cfg.BaseImage).
		WithDirectory("/src", source).
		WithWorkdir("/src").
		WithMountedCache("/usr/local/cargo/registry", dag.CacheVolume("cargo-registry")).
		WithMountedCache("/usr/local/cargo/git", dag.CacheVolume("cargo-git")).
		WithMountedCache("/src/target", dag.CacheVolume("cargo-target")).
		WithExec([]string{"/bin/sh", "-c", fmt.Sprintf(
			`%s 2>&1 | while IFS= read -r line; do echo '{"jobAttempt":"%s","job":"%s","step":"dependencies","message":"'"$line"'"}'; done`,
			depCmd, jobAttempt, job,
		)})

	builder, err = builder.Sync(ctx)
	depSpan.End()
	if err != nil {
		return nil, fmt.Errorf("dependencies failed: %w", err)
	}

	// Build step — compile release binary
	bCmd := "cargo build --release"
	if buildCmd != "" {
		bCmd = buildCmd
	}

	ctx, buildSpan := Tracer().Start(ctx, "build")
	buildSpan.SetAttributes(attribute.String("kad.jobAttempt", jobAttempt))
	defer telemetry.End(buildSpan, func() error { return rerr })

	// After building, extract the binary name from Cargo.toml and copy it to /app
	buildAndCopy := fmt.Sprintf(
		`set -e; %s 2>&1 | while IFS= read -r line; do echo '{"jobAttempt":"%s","job":"%s","step":"build","message":"'"$line"'"}'; done; `+
			`BIN=$(grep -m1 'name\s*=' Cargo.toml | sed 's/.*=\s*"\(.*\)"/\1/' | tr -d '[:space:]'); `+
			`cp target/release/"$BIN" /app`,
		bCmd, jobAttempt, job,
	)

	builder, err = builder.
		WithExec([]string{"/bin/sh", "-c", buildAndCopy}).
		Sync(ctx)
	if err != nil {
		return nil, fmt.Errorf("cargo build failed: %w", err)
	}

	// Runtime stage: copy binary to distroless/cc (provides glibc for dynamically linked binaries)
	runtime, err := dag.Container().
		From(cfg.RuntimeImage).
		WithFile("/app", builder.File("/app")).
		WithEntrypoint(cfg.StartCmd).
		WithExposedPort(*exposedPort).
		Sync(ctx)
	if err != nil {
		return nil, fmt.Errorf("runtime container failed: %w", err)
	}

	return runtime, nil
}
