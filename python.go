package main

import (
	"context"
	"dagger/build/internal/dagger"
	"fmt"
	"strings"

	telemetry "github.com/dagger/otel-go"
	"go.opentelemetry.io/otel/attribute"
)

// resolvePythonVersion detects the Python version from project files.
// Priority: explicit override → .python-version → runtime.txt → default from FrameworkConfig.
func resolvePythonVersion(ctx context.Context, source *dagger.Directory, override string, defaultImage string) string {
	if override != "" {
		return fmt.Sprintf("python:%s-slim", override)
	}

	// .python-version (e.g. "3.13.1" or "3.12")
	if contents, err := source.File(".python-version").Contents(ctx); err == nil {
		v := strings.TrimSpace(contents)
		if parts := strings.SplitN(v, ".", 3); len(parts) >= 2 {
			return fmt.Sprintf("python:%s.%s-slim", parts[0], parts[1])
		}
	}

	// runtime.txt (Heroku-style, e.g. "python-3.12.0")
	if contents, err := source.File("runtime.txt").Contents(ctx); err == nil {
		v := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(contents), "python-"))
		if parts := strings.SplitN(v, ".", 3); len(parts) >= 2 {
			return fmt.Sprintf("python:%s.%s-slim", parts[0], parts[1])
		}
	}

	return defaultImage
}

func init() {
	frameworks["fastapi"] = FrameworkConfig{
		Builder:     "python-server",
		BaseImage:   "python:3.12-slim",
		StartCmd:    []string{"uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"},
		DefaultPort: 8000,
	}
	frameworks["flask"] = FrameworkConfig{
		Builder:     "python-server",
		BaseImage:   "python:3.12-slim",
		StartCmd:    []string{"gunicorn", "--bind", "0.0.0.0:5000", "app:app"},
		DefaultPort: 5000,
	}
	frameworks["django"] = FrameworkConfig{
		Builder:     "python-server",
		BaseImage:   "python:3.12-slim",
		StartCmd:    []string{"gunicorn", "--bind", "0.0.0.0:8000", "config.wsgi:application"},
		DefaultPort: 8000,
	}
}

// BuildPythonServer builds a Python app and packages it in a slim runtime image.
// Covers FastAPI (uvicorn), Flask (gunicorn), and Django (gunicorn).
func (m *Build) BuildPythonServer(
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
	// Override the default install command (e.g. "pip install -e .")
	dependenciesCmd string,
	// +optional
	// Override the default build command (not used for most Python apps)
	buildCmd string,
	// +optional
	// Override runtime version (e.g. "3.13" for Python 3.13)
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

	// Resolve Python version
	baseImage := resolvePythonVersion(ctx, source, runtimeVersion, cfg.BaseImage)

	// Dependencies step
	depCmd := "pip install --no-cache-dir -r requirements.txt"
	if dependenciesCmd != "" {
		depCmd = dependenciesCmd
	}

	ctx, depSpan := Tracer().Start(ctx, "dependencies")
	depSpan.SetAttributes(attribute.String("kad.jobAttempt", jobAttempt))

	builder := dag.Container().
		From(baseImage).
		WithDirectory("/app", source).
		WithWorkdir("/app").
		WithMountedCache("/root/.cache/pip", dag.CacheVolume("pip-cache")).
		WithExec([]string{"/bin/sh", "-c", fmt.Sprintf(
			`%s 2>&1 | while IFS= read -r line; do echo '{"jobAttempt":"%s","job":"%s","step":"dependencies","message":"'"$line"'"}'; done`,
			depCmd, jobAttempt, job,
		)})

	builder, err = builder.Sync(ctx)
	depSpan.End()
	if err != nil {
		return nil, fmt.Errorf("dependencies failed: %w", err)
	}

	// Build step (optional — many Python apps skip this, but support buildCmd override)
	ctx, buildSpan := Tracer().Start(ctx, "build")
	buildSpan.SetAttributes(attribute.String("kad.jobAttempt", jobAttempt))
	defer telemetry.End(buildSpan, func() error { return rerr })

	if buildCmd != "" {
		builder, err = builder.
			WithExec([]string{"/bin/sh", "-c", fmt.Sprintf(
				`%s 2>&1 | while IFS= read -r line; do echo '{"jobAttempt":"%s","job":"%s","step":"build","message":"'"$line"'"}'; done`,
				buildCmd, jobAttempt, job,
			)}).
			Sync(ctx)
		if err != nil {
			return nil, fmt.Errorf("build failed: %w", err)
		}
	}

	// Final container with entrypoint
	container, err := builder.
		WithEntrypoint(cfg.StartCmd).
		WithExposedPort(*exposedPort).
		Sync(ctx)
	if err != nil {
		return nil, fmt.Errorf("runtime container failed: %w", err)
	}

	return container, nil
}
