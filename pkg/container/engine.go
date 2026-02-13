package container

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Engine represents a detected container runtime (docker or podman).
type Engine struct {
	Path string // absolute path to the binary
	Name string // "docker" or "podman"
}

// DetectEngine finds a container engine by first checking the
// APKG_CONTAINER_ENGINE env var, then searching PATH for docker and podman.
func DetectEngine() (*Engine, error) {
	if override := os.Getenv("APKG_CONTAINER_ENGINE"); override != "" {
		path, err := exec.LookPath(override)
		if err != nil {
			return nil, fmt.Errorf("APKG_CONTAINER_ENGINE=%q not found in PATH: %w", override, err)
		}
		name := override
		// Normalise to just the binary name if a full path was given.
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}
		return &Engine{Path: path, Name: name}, nil
	}

	for _, candidate := range []string{"docker", "podman"} {
		path, err := exec.LookPath(candidate)
		if err == nil {
			return &Engine{Path: path, Name: candidate}, nil
		}
	}

	return nil, fmt.Errorf("no container engine found: install docker or podman, or set APKG_CONTAINER_ENGINE")
}

// Pull pulls an image if it isn't already present locally.
func (e *Engine) Pull(ctx context.Context, image string) error {
	cmd := exec.CommandContext(ctx, e.Path, "image", "inspect", image)
	if err := cmd.Run(); err == nil {
		return nil // image already present
	}

	cmd = exec.CommandContext(ctx, e.Path, "pull", image)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pulling image %q: %w", image, err)
	}
	return nil
}

// RunOpts holds optional parameters for running a container.
type RunOpts struct {
	Env     map[string]string // environment variables passed via -e
	Args    []string          // arguments appended after the image (entrypoint args)
	Volumes []string          // bind mounts passed via -v (host:container[:ro])
	Network string            // container network (--network); "host" skips port mapping
}

// Run starts a detached container with the given name, mapping hostPort to
// containerPort, and returns the container ID.
func (e *Engine) Run(ctx context.Context, name, image string, hostPort, containerPort int, opts *RunOpts) (string, error) {
	args := []string{
		"run", "-d",
		"--name", name,
	}

	isHostNetwork := opts != nil && opts.Network == "host"

	if opts != nil && opts.Network != "" {
		args = append(args, "--network", opts.Network)
	}

	// Port mapping is not supported with --network=host; the container
	// shares the host network stack so its ports are already reachable.
	if !isHostNetwork {
		args = append(args, "-p", fmt.Sprintf("%d:%d", hostPort, containerPort))
	}

	if opts != nil {
		for k, v := range opts.Env {
			args = append(args, "-e", k+"="+v)
		}
		for _, vol := range opts.Volumes {
			args = append(args, "-v", expandVolumeTilde(vol))
		}
	}

	args = append(args, image)

	if opts != nil && len(opts.Args) > 0 {
		args = append(args, opts.Args...)
	}

	cmd := exec.CommandContext(ctx, e.Path, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("starting container %q: %w", name, execError(err))
	}
	return strings.TrimSpace(string(out)), nil
}

// Stop stops and removes a container by name. It is not an error if the
// container does not exist.
func (e *Engine) Stop(ctx context.Context, name string) error {
	// Stop, then remove. Ignore errors from stop (container may not be running).
	stop := exec.CommandContext(ctx, e.Path, "stop", name)
	_ = stop.Run()

	rm := exec.CommandContext(ctx, e.Path, "rm", "-f", name)
	if err := rm.Run(); err != nil {
		return fmt.Errorf("removing container %q: %w", name, execError(err))
	}
	return nil
}

// ImageDigest returns the image ID (sha256 digest) for a locally available image.
func (e *Engine) ImageDigest(ctx context.Context, image string) (string, error) {
	cmd := exec.CommandContext(ctx, e.Path, "image", "inspect", "--format", "{{.Id}}", image)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("inspecting image %q: %w", image, execError(err))
	}
	digest := strings.TrimSpace(string(out))
	// Normalize: strip "sha256:" prefix if present so callers get a bare hex string.
	digest = strings.TrimPrefix(digest, "sha256:")
	return digest, nil
}

// IsRunning checks whether a container with the given name is currently running.
func (e *Engine) IsRunning(ctx context.Context, name string) (bool, error) {
	cmd := exec.CommandContext(ctx, e.Path,
		"container", "inspect", "-f", "{{.State.Running}}", name)
	out, err := cmd.Output()
	if err != nil {
		return false, nil // container doesn't exist
	}
	return strings.TrimSpace(string(out)) == "true", nil
}

// expandVolumeTilde expands a leading "~/" in the host-path portion of a
// volume mount string (host:container[:opts]).  exec.Command bypasses the
// shell, so tilde expansion doesn't happen automatically.
func expandVolumeTilde(vol string) string {
	if !strings.HasPrefix(vol, "~/") {
		return vol
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return vol // can't expand; return as-is
	}
	// Replace only the leading "~" (the host path part before the first colon
	// after the prefix).
	return home + vol[1:]
}

// execError extracts stderr from an *exec.ExitError when available.
func execError(err error) error {
	if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(ee.Stderr)))
	}
	return err
}
