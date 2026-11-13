// Copyright 2026 Dolthub, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package targets

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Container describes one target's container lifecycle.
//
// Fixed container names are intentional: after a run kept alive for
// investigation, a human connects with `mongosh <HostURI>`, so names and
// ports have to be predictable.
//
// Use 127.0.0.1 rather than "localhost" in HostURI: where Go's resolver hands
// back the IPv6 loopback first, clients can see `dial tcp [::1]:27017:
// connect: connection refused` although the container listens, because
// mongod binds IPv4 only inside the container and docker's IPv6 host-port
// proxy is best-effort.
type Container struct {
	Name    string   // fixed container name, e.g. "mongodb-bench"
	Image   string   // tag to run, e.g. "mongo:8.0" or "dumbodb-bench:local"
	HostURI string   // URI the host (and mongosh) uses to reach it
	RunArgs []string // everything between `docker run -d ...` and the image name
}

// EnsureImage pulls image if it is missing. It deliberately reuses a cached
// image, so for a mutable tag (e.g. :latest) run `docker pull` first to
// refresh it.
func EnsureImage(ctx context.Context, image string) error {
	if imagePresent(ctx, image) {
		return nil
	}
	fmt.Fprintf(os.Stderr, "==> pulling %s\n", image)
	return runDocker(ctx, nil, "pull", image)
}

// BuildDumboDBImage builds image from the dumbodb source tree at srcDir with
// benchmarks/Dockerfile.dumbodb. It always rebuilds: Docker's layer cache
// makes an unchanged rebuild cheap, and a stale binary is the worse bug.
func BuildDumboDBImage(ctx context.Context, image, srcDir string) error {
	dockerfile, err := locateDumboDBDockerfile()
	if err != nil {
		return err
	}
	if _, err := os.Stat(srcDir); err != nil {
		return fmt.Errorf("product repo path %q: %w", srcDir, err)
	}
	fmt.Fprintf(os.Stderr, "==> building %s from %s (using %s)\n", image, srcDir, dockerfile)
	return runDocker(ctx, nil, "build", "-f", dockerfile, "-t", image, srcDir)
}

// locateDumboDBDockerfile accepts being run from the parity repo root (the
// usual `go run ./benchmarks/cmd/compare`) or from below it.
func locateDumboDBDockerfile() (string, error) {
	candidates := []string{
		"benchmarks/Dockerfile.dumbodb",
		"Dockerfile.dumbodb",
		"../Dockerfile.dumbodb",
		"../../Dockerfile.dumbodb",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("could not find benchmarks/Dockerfile.dumbodb (cwd=%s)", cwd())
}

// Start starts c if it isn't already running. A stopped container of the same
// name is removed first; its exit state may reflect a prior crash.
func (c Container) Start(ctx context.Context) error {
	state := c.State(ctx)
	switch state {
	case "running":
		fmt.Fprintf(os.Stderr, "==> reusing running container %s\n", c.Name)
		return nil
	case "exited", "created", "dead":
		fmt.Fprintf(os.Stderr, "==> removing stale container %s (state=%s)\n", c.Name, state)
		_ = runDocker(ctx, nil, "rm", "-f", "-v", c.Name)
	case "":
	default:
		fmt.Fprintf(os.Stderr, "==> container %s in unexpected state %q - recreating\n", c.Name, state)
		_ = runDocker(ctx, nil, "rm", "-f", "-v", c.Name)
	}
	args := []string{"run", "-d", "--name", c.Name}
	args = append(args, c.RunArgs...)
	args = append(args, c.Image)
	fmt.Fprintf(os.Stderr, "==> starting %s (%s)\n", c.Name, c.Image)
	return runDocker(ctx, nil, args...)
}

// Stop stops and removes c; a missing container is fine.
func (c Container) Stop(ctx context.Context) {
	if c.State(ctx) == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "==> stopping %s\n", c.Name)
	if err := runDocker(ctx, io.Discard, "stop", c.Name); err != nil {
		fmt.Fprintf(os.Stderr, "    (stop failed: %v)\n", err)
	}
	if err := runDocker(ctx, io.Discard, "rm", "-v", c.Name); err != nil {
		fmt.Fprintf(os.Stderr, "    (rm failed: %v)\n", err)
	}
}

// Fresh replaces c with a new container, so its server starts with empty data.
func (c Container) Fresh(ctx context.Context, healthTimeout time.Duration) error {
	c.Stop(ctx)
	if err := c.Start(ctx); err != nil {
		return err
	}
	return c.WaitHealthy(ctx, healthTimeout)
}

// WaitHealthy polls Ping until it succeeds or timeout passes, then dumps the
// container's state and log tail.
func (c Container) WaitHealthy(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		attemptCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := ping(attemptCtx, c.HostURI)
		cancel()
		if err == nil {
			fmt.Fprintf(os.Stderr, "==> %s healthy (%s)\n", c.Name, c.HostURI)
			return nil
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	fmt.Fprintf(os.Stderr, "\n==> %s failed health check - dumping diagnostics:\n", c.Name)
	fmt.Fprintf(os.Stderr, "    state: %s\n", c.State(ctx))
	_ = runDocker(ctx, os.Stderr, "logs", "--tail", "30", c.Name)
	return fmt.Errorf("%s never became healthy at %s: %w", c.Name, c.HostURI, lastErr)
}

// CheckServing fails with a triage hint when c is not running or its server
// does not answer, which is easier to act on than a connection refused from
// deep inside a run.
func (c Container) CheckServing(ctx context.Context, label string) error {
	if state := c.State(ctx); state != "running" {
		if state == "" {
			state = "missing"
		}
		return fmt.Errorf("%s container %q is %q, not running - check `docker logs %s` for why it exited",
			label, c.Name, state, c.Name)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := ping(pingCtx, c.HostURI); err != nil {
		return fmt.Errorf("%s container %q is running but not reachable at %s: %w - check `docker logs %s`",
			label, c.Name, c.HostURI, err, c.Name)
	}
	return nil
}

// State returns the docker state ("running", "exited", "created", ...), or
// "" when there is no such container.
func (c Container) State(ctx context.Context) string {
	out := &strings.Builder{}
	cmd := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Status}}", c.Name)
	cmd.Stdout = out
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(out.String())
}

func ping(ctx context.Context, uri string) error {
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return err
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	return client.Ping(ctx, nil)
}

func imagePresent(ctx context.Context, image string) bool {
	cmd := exec.CommandContext(ctx, "docker", "image", "inspect", image)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}

// runDocker streams output to stderr when out is nil, so long builds and
// pulls show progress.
func runDocker(ctx context.Context, out io.Writer, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	if out == nil {
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stdout = out
		cmd.Stderr = out
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

func cwd() string {
	d, err := os.Getwd()
	if err != nil {
		return "?"
	}
	return d
}
