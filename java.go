package main

import (
	"context"
	"dagger/build/internal/dagger"
	"fmt"

	telemetry "github.com/dagger/otel-go"
	"go.opentelemetry.io/otel/attribute"
)

func init() {
	frameworks["spring-boot"] = FrameworkConfig{
		Builder:         "java-maven",
		BaseImage:       "maven:3.9-eclipse-temurin-21",
		RuntimeImage:    "eclipse-temurin:21-jre-alpine",
		BuildOutputPath: "target",
		StartCmd:        []string{"java", "-jar", "/app/app.jar"},
		DefaultPort:     8080,
	}
}

// BuildJavaMaven builds a Spring Boot (Maven) app using a multi-stage build.
// Build stage: maven:3.9-eclipse-temurin-21 — runs mvn package to produce a fat JAR.
// Runtime stage: eclipse-temurin:21-jre-alpine — copies the JAR into a minimal JRE image.
func (m *Build) BuildJavaMaven(
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
	// Override the default install command (e.g. "mvn dependency:go-offline")
	dependenciesCmd string,
	// +optional
	// Override the default build command (e.g. "mvn package -Pprod -DskipTests")
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

	// Dependencies step — download Maven dependencies into local cache
	depCmd := "mvn dependency:go-offline -B"
	if dependenciesCmd != "" {
		depCmd = dependenciesCmd
	}

	ctx, depSpan := Tracer().Start(ctx, "dependencies")
	depSpan.SetAttributes(attribute.String("kad.jobAttempt", jobAttempt))

	builder := dag.Container().
		From(cfg.BaseImage).
		WithDirectory("/src", source).
		WithWorkdir("/src").
		WithMountedCache("/root/.m2", dag.CacheVolume("maven-m2")).
		WithExec([]string{"/bin/sh", "-c", fmt.Sprintf(
			`%s 2>&1 | while IFS= read -r line; do echo '{"jobAttempt":"%s","job":"%s","step":"dependencies","message":"'"$line"'"}'; done`,
			depCmd, jobAttempt, job,
		)})

	builder, err = builder.Sync(ctx)
	depSpan.End()
	if err != nil {
		return nil, fmt.Errorf("dependencies failed: %w", err)
	}

	// Build step — package into a fat JAR, skip tests
	bCmd := "mvn package -B -DskipTests"
	if buildCmd != "" {
		bCmd = buildCmd
	}

	ctx, buildSpan := Tracer().Start(ctx, "build")
	buildSpan.SetAttributes(attribute.String("kad.jobAttempt", jobAttempt))
	defer telemetry.End(buildSpan, func() error { return rerr })

	// Build and copy the fat JAR to a known path /app.jar for easy extraction
	builder, err = builder.
		WithExec([]string{"/bin/sh", "-c", fmt.Sprintf(
			`%s 2>&1 | while IFS= read -r line; do echo '{"jobAttempt":"%s","job":"%s","step":"build","message":"'"$line"'"}'; done`,
			bCmd, jobAttempt, job,
		)}).
		// Copy the fat JAR (excludes *.jar.original created by Spring Boot repackager) to a fixed path
		WithExec([]string{"/bin/sh", "-c",
			`cp $(ls /src/target/*.jar | grep -v '.jar.original' | head -n1) /app.jar`,
		}).
		Sync(ctx)
	if err != nil {
		return nil, fmt.Errorf("maven build failed: %w", err)
	}

	// Runtime stage: copy JAR into minimal JRE image
	runtime, err := dag.Container().
		From(cfg.RuntimeImage).
		WithFile("/app/app.jar", builder.File("/app.jar")).
		WithEntrypoint(cfg.StartCmd).
		WithExposedPort(*exposedPort).
		Sync(ctx)
	if err != nil {
		return nil, fmt.Errorf("runtime container failed: %w", err)
	}

	return runtime, nil
}
