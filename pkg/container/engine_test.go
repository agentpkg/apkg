package container

import (
	"os"
	"os/exec"
	"testing"
)

func TestDetectEngine(t *testing.T) {
	tests := map[string]struct {
		envVar  string
		wantErr bool
		// wantName is only checked when wantErr is false and envVar is set.
		wantName string
	}{
		"env var set to docker": {
			envVar:   "docker",
			wantName: "docker",
		},
		"env var set to podman": {
			envVar:   "podman",
			wantName: "podman",
		},
		"env var set to nonexistent binary": {
			envVar:  "nonexistent-engine-abc123",
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if tc.envVar != "" && !tc.wantErr {
				if _, err := exec.LookPath(tc.envVar); err != nil {
					t.Skipf("%s not in PATH, skipping", tc.envVar)
				}
			}

			if tc.envVar != "" {
				t.Setenv("APKG_CONTAINER_ENGINE", tc.envVar)
			}

			eng, err := DetectEngine()
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.wantName != "" && eng.Name != tc.wantName {
				t.Errorf("Name = %q, want %q", eng.Name, tc.wantName)
			}
			if eng.Path == "" {
				t.Error("Path is empty")
			}
		})
	}
}

func TestDetectEngineFromPath(t *testing.T) {
	// Unset the env var so detection falls through to PATH lookup.
	t.Setenv("APKG_CONTAINER_ENGINE", "")

	// At least one of docker or podman should be available in most dev/CI envs,
	// but skip rather than fail if neither is present.
	hasDocker := false
	if _, err := exec.LookPath("docker"); err == nil {
		hasDocker = true
	}
	hasPodman := false
	if _, err := exec.LookPath("podman"); err == nil {
		hasPodman = true
	}

	if !hasDocker && !hasPodman {
		t.Skip("neither docker nor podman in PATH")
	}

	eng, err := DetectEngine()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if eng.Name != "docker" && eng.Name != "podman" {
		t.Errorf("Name = %q, want docker or podman", eng.Name)
	}

	// Docker should be preferred over podman when both are present.
	if hasDocker && eng.Name != "docker" {
		t.Errorf("Name = %q, want docker (should prefer docker over podman)", eng.Name)
	}
}

func TestDetectEngineNoEngineAvailable(t *testing.T) {
	// Point PATH to an empty directory so no engine can be found.
	emptyDir := t.TempDir()
	t.Setenv("PATH", emptyDir)
	t.Setenv("APKG_CONTAINER_ENGINE", "")

	// Clear any cached lookups by verifying the tools aren't found.
	if _, err := exec.LookPath("docker"); err == nil {
		t.Skip("docker found despite empty PATH — possibly cached")
	}

	_, err := DetectEngine()
	if err == nil {
		t.Fatal("expected error when no engine is available, got nil")
	}
}

func TestExpandVolumeTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}

	tests := map[string]struct {
		input string
		want  string
	}{
		"tilde with container path": {
			input: "~/.kube/config:/kubeconfig:ro",
			want:  home + "/.kube/config:/kubeconfig:ro",
		},
		"tilde only host path": {
			input: "~/data:/data",
			want:  home + "/data:/data",
		},
		"absolute path unchanged": {
			input: "/home/user/.kube/config:/kubeconfig:ro",
			want:  "/home/user/.kube/config:/kubeconfig:ro",
		},
		"relative path unchanged": {
			input: "./data:/data",
			want:  "./data:/data",
		},
		"bare tilde no slash unchanged": {
			input: "~data:/data",
			want:  "~data:/data",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := expandVolumeTilde(tc.input)
			if got != tc.want {
				t.Errorf("expandVolumeTilde(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestExecError(t *testing.T) {
	tests := map[string]struct {
		err      error
		contains string
	}{
		"exit error with stderr": {
			err:      &exec.ExitError{Stderr: []byte("something went wrong")},
			contains: "something went wrong",
		},
		"plain error": {
			err:      os.ErrNotExist,
			contains: "not exist",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := execError(tc.err)
			if result == nil {
				t.Fatal("expected non-nil error")
			}
		})
	}
}
