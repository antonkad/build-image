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

package main

import (
	"context"
	"dagger/build/internal/dagger"
	"fmt"
	"math"
	"math/rand/v2"
)

type Build struct{}

func (m *Build) BuildEnv(source *dagger.Directory) *dagger.Container {
	// create a Dagger cache volume for dependencies
	//nodeCache := dag.CacheVolume("node")

	return dag.Container().
		// start from a base Node.js container
		From("node:23-slim").
		// add the source code at /src
		WithDirectory("/src", source).
		WithExec([]string{"npm", "install", "-g", "pnpm"}).
		// change the working directory to /src
		WithWorkdir("/src").
		// run npm install to install dependencies
		WithExec([]string{"pnpm", "install"})

}

func (m *Build) BuildNginx(ctx context.Context, source *dagger.Directory) (*dagger.Container, error) {
	build, err := m.BuildEnv(source).
		WithExec([]string{"pnpm", "run", "build"}).
		Directory("./dist").
		Sync(ctx)

	if err != nil {
		// unexpected error, could be network failure.
		return nil, fmt.Errorf("run Build: %w", err)
	}
	return dag.Container().From("nginx:1.25-alpine").
		WithDirectory("/usr/share/nginx/html", build).
		WithExposedPort(80), nil
}

func (m *Build) Build(ctx context.Context, source *dagger.Directory) (*dagger.Container, error) {
	build, err := m.BuildEnv(source).
		WithExec([]string{"pnpm", "run", "build"}).
		Sync(ctx)

	if err != nil {
		// unexpected error, could be network failure.
		return nil, fmt.Errorf("run Build: %w", err)
	}
	return build, nil
}

func (m *Build) StartNext(ctx context.Context, source *dagger.Directory) (string, error) {
	buildContainer, err := m.Build(ctx, source)
	if err != nil {
		return "", fmt.Errorf("error in build: %w", err)
	}

	start, err := buildContainer.
		WithEntrypoint([]string{"pnpm", "run", "start"}).
		WithExposedPort(3000).
		Sync(ctx)

	if err != nil {
		// unexpected error, could be network failure.
		return "", fmt.Errorf("run Build: %w", err)
	}
	exitCode, err := start.ExitCode(ctx)
	if err != nil {
		// exit code not found
		return "", fmt.Errorf("get exit code: %w", err)
	}
	fmt.Printf(string(exitCode))

	addr, err := start.Publish(ctx, fmt.Sprintf("ttl.sh/love-letter-%.0f", math.Floor(rand.Float64()*10000000)))
	if err != nil {
		return "", err
	}

	return addr, nil
}
