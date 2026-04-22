package main

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

// container describes one benchmark target's container lifecycle.
//
// Fixed container/image names are an intentional design choice: the
// investigation flow documented on pa-lxa has a human running
// `mongosh mongodb://localhost:27018` after the bench run, so the names and
// ports have to be predictable.
type container struct {
	name    string   // fixed container name, e.g. "mongodb-bench"
	image   string   // tag to run, e.g. "mongo:8.0" or "dumbodb-bench:local"
	hostURI string   // URI the host (and mongosh) uses to reach it
	runArgs []string // everything between `docker run -d ...` and the image name
}

var (
	mongoContainer = container{
		name:    "mongodb-bench",
		image:   "mongo:8.0",
		hostURI: "mongodb://localhost:27017",
		runArgs: []string{"-p", "27017:27017"},
	}
	dumboContainer = container{
		name:    "dumbodb-bench",
		image:   "dumbodb-bench:local",
		hostURI: "mongodb://localhost:27018",
		runArgs: []string{"-p", "27018:27018"},
	}
)

// ensureMongoImage pulls mongo:8.0 if it's missing. Not fatal if pull fails —
// the subsequent `docker run` will surface the real error.
func ensureMongoImage(ctx context.Context) error {
	if imagePresent(ctx, mongoContainer.image) {
		return nil
	}
	fmt.Fprintf(os.Stderr, "==> pulling %s\n", mongoContainer.image)
	return runDocker(ctx, nil, "pull", mongoContainer.image)
}

// buildDumboImage builds dumbodb-bench:local from the product repo. Always
// rebuilds — Docker's layer cache makes repeat builds cheap when the source
// hasn't changed, and stale binaries are the worst possible benchmark bug.
func buildDumboImage(ctx context.Context, productRepoPath string) error {
	dockerfile, err := locateDockerfile()
	if err != nil {
		return err
	}
	if _, err := os.Stat(productRepoPath); err != nil {
		return fmt.Errorf("product repo path %q: %w", productRepoPath, err)
	}
	fmt.Fprintf(os.Stderr, "==> building %s from %s (using %s)\n", dumboContainer.image, productRepoPath, dockerfile)
	return runDocker(ctx, nil,
		"build",
		"-f", dockerfile,
		"-t", dumboContainer.image,
		productRepoPath,
	)
}

// locateDockerfile finds benchmarks/Dockerfile.dumbodb relative to the
// current working directory. The compare runner is normally invoked from the
// parity repo root (`go run ./benchmarks/cmd/compare`) so that's the common
// case; we also accept being run from inside benchmarks/.
func locateDockerfile() (string, error) {
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

// startContainer starts c if it isn't already running. If a container with
// the same name exists but is stopped, it is removed first (its exit state
// might reflect a prior crash we don't want to inherit).
func startContainer(ctx context.Context, c container) error {
	state := containerState(ctx, c.name)
	switch state {
	case "running":
		fmt.Fprintf(os.Stderr, "==> reusing running container %s\n", c.name)
		return nil
	case "exited", "created", "dead":
		fmt.Fprintf(os.Stderr, "==> removing stale container %s (state=%s)\n", c.name, state)
		_ = runDocker(ctx, nil, "rm", "-f", c.name)
	case "":
		// no such container, fall through
	default:
		fmt.Fprintf(os.Stderr, "==> container %s in unexpected state %q — recreating\n", c.name, state)
		_ = runDocker(ctx, nil, "rm", "-f", c.name)
	}
	args := []string{"run", "-d", "--name", c.name}
	args = append(args, c.runArgs...)
	args = append(args, c.image)
	fmt.Fprintf(os.Stderr, "==> starting %s (%s)\n", c.name, c.image)
	return runDocker(ctx, nil, args...)
}

// stopContainer stops and removes c. Missing containers are fine — this is
// the teardown path and we want it to be idempotent.
func stopContainer(ctx context.Context, c container) {
	if containerState(ctx, c.name) == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "==> stopping %s\n", c.name)
	if err := runDocker(ctx, io.Discard, "stop", c.name); err != nil {
		fmt.Fprintf(os.Stderr, "    (stop failed: %v)\n", err)
	}
	if err := runDocker(ctx, io.Discard, "rm", c.name); err != nil {
		fmt.Fprintf(os.Stderr, "    (rm failed: %v)\n", err)
	}
}

// waitHealthy polls Ping against the container's host URI until it succeeds
// or times out. Timeout is long enough for cold-start DumboDB (dolt engine
// init) but short enough to catch genuinely broken containers.
func waitHealthy(ctx context.Context, c container, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		// Short per-attempt timeout so we cycle quickly early on.
		attemptCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		client, err := mongo.Connect(attemptCtx, options.Client().ApplyURI(c.hostURI))
		if err == nil {
			err = client.Ping(attemptCtx, nil)
			_ = client.Disconnect(context.Background())
		}
		cancel()
		if err == nil {
			fmt.Fprintf(os.Stderr, "==> %s healthy (%s)\n", c.name, c.hostURI)
			return nil
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("%s never became healthy at %s: %w", c.name, c.hostURI, lastErr)
}

// containerState returns the docker state string for name, or "" if no such
// container exists. States we care about: "running", "exited", "created".
func containerState(ctx context.Context, name string) string {
	out := &strings.Builder{}
	cmd := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Status}}", name)
	cmd.Stdout = out
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(out.String())
}

func imagePresent(ctx context.Context, image string) bool {
	cmd := exec.CommandContext(ctx, "docker", "image", "inspect", image)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}

// runDocker shells out to docker. If out is nil, output is streamed to
// stderr so the user sees progress (especially for long builds and pulls).
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
