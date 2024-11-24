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
	"bytes"
	"context"
	"dagger/build/internal/dagger"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Build struct{}
type LogRecord struct {
	ID        string    `json:"id,omitempty"`
	Command   string    `json:"command,omitempty"`
	Stdout    string    `json:"stdout,omitempty"`
	Stderr    string    `json:"stderr,omitempty"`
	ExitCode  int       `json:"exitCode,omitempty"`
	Commit    string    `json:"commit,omitempty"`
	ProjectID string    `json:"projectID,omitempty"`
	Timestamp time.Time `json:"timestamp,omitempty"`
}

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

func (m *Build) Build(
	ctx context.Context,
	repository string,
	// +optional
	ref string,
	// +optional
	path string,
) (*dagger.Container, error) {
	version := "HEAD"
	if ref != "" {
		version = ref
	}
	source, err := createDirectory(ctx, repository, &ref, &path)
	if err != nil {
		return nil, fmt.Errorf("Error creating directory: %v", err)
	}

	// Execute the build process.
	command := []string{"pnpm", "run", "build"}
	build, err := m.BuildEnv(source).
		WithExec(command).
		Sync(ctx)

	var e *dagger.ExecError
	if errors.As(err, &e) {
		record := LogRecord{
			Command:   strings.Join(command, " "),
			Stdout:    e.Stdout,
			Stderr:    e.Stderr,
			ExitCode:  e.ExitCode,
			Commit:    version,
			ProjectID: "mwe47v732g3llus",
			Timestamp: time.Now(),
		}

		// Push logs to the API
		if err := createLogRecord(ctx, record); err != nil {
			return nil, fmt.Errorf("failed to create log record: %w", err)
		}

		return nil, err
	} else if err != nil {
		return nil, err
	}

	// Collect build output.
	stdout, stderr, exitCode, logErr := fetchBuildLogs(ctx, build)
	if logErr != nil {
		return nil, logErr
	}

	// Log record structure
	record := LogRecord{
		Command:   strings.Join(command, " "),
		Stdout:    stdout,
		Stderr:    stderr,
		ExitCode:  exitCode,
		Commit:    version,
		ProjectID: "mwe47v732g3llus",
	}

	// Push logs to the API
	if err := createLogRecord(ctx, record); err != nil {
		return nil, fmt.Errorf("failed to create log record: %w", err)
	}

	return build, err
}

func createDirectory(ctx context.Context, repository string, branch *string, path *string) (*dagger.Directory, error) {
	// Create the Git repository reference

	var gitRepo *dagger.Directory
	// Check if branch is provided and non-empty
	if branch != nil && *branch != "" {
		gitRepo = dag.Git(repository).Branch(*branch).Tree()
	} else {
		gitRepo = dag.Git(repository).Head().Tree()
	}
	// If a directory is specified, narrow down to that directory
	if path != nil && *path != "" {
		return gitRepo.Directory(*path), nil
	}

	// Return the root of the repository if no directory is specified
	return gitRepo.Directory("."), nil
}

// Helper to fetch build logs.
func fetchBuildLogs(ctx context.Context, build *dagger.Container) (string, string, int, error) {
	stdout, err := build.Stdout(ctx)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to fetch stdout: %w", err)
	}
	stderr, err := build.Stderr(ctx)
	if err != nil {
		return stdout, "", 0, fmt.Errorf("failed to fetch stderr: %w", err)
	}
	exitCode, err := build.ExitCode(ctx)
	if err != nil {
		return stdout, stderr, 0, fmt.Errorf("failed to fetch exit code: %w", err)
	}
	return stdout, stderr, exitCode, nil
}

func createLogRecord(ctx context.Context, record LogRecord) error {
	url := "http://host.docker.internal:8090"

	client := &http.Client{}
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal record: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url+"/api/collections/logs/records", bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create POST request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	//req.Header.Set("Authorization", "TOKEN "+token)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("POST request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected response: %s", string(body))
	}

	return nil
}

/*
	addr, err := start.Publish(ctx, fmt.Sprintf("ttl.sh/love-letter-%.0f", math.Floor(rand.Float64()*10000000)))
	if err != nil {
		return  err
	}
*/

/*
func (m *Build) BuildNginx(ctx context.Context, source *dagger.Directory) (*dagger.Container, error) {
	build, err := m.BuildEnv(source).
		WithExec([]string{"pnpm", "run", "wawa"}).
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


func (m *Build) BuildNext(ctx context.Context, source *dagger.Directory) (*dagger.Container, error) {
	buildContainer, err := m.Build(ctx, source)
	if err != nil {
		return nil, fmt.Errorf("error in build: %w", err)
	}

	container, err := buildContainer.
		WithEntrypoint([]string{"pnpm", "run", "start"}).
		WithExposedPort(3000).
		Sync(ctx)

	if err != nil {
		// unexpected error, could be network failure.
		return nil, fmt.Errorf("run Build: %w", err)
	}

	return container, nil
}*/
