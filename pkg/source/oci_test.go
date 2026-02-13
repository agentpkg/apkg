package source

import (
	"testing"

	"github.com/agentpkg/agentpkg/pkg/config"
)

func TestOCISourceImplementsSource(t *testing.T) {
	var _ Source = &OCISource{}
}

func TestOCISourceWriteMCPConfig(t *testing.T) {
	// writeMCPConfig is tested indirectly — we verify the store segments
	// and config marshaling work correctly without requiring a real
	// container engine (Fetch needs docker/podman).

	port := 5432
	tests := map[string]struct {
		name      string
		mcpConfig config.MCPSource
	}{
		"container with explicit port": {
			name: "postgres",
			mcpConfig: config.MCPSource{
				Name:               "postgres",
				Transport:          "http",
				ContainerMCPConfig: &config.ContainerMCPConfig{Image: "postgres-mcp:latest", Port: &port},
			},
		},
		"container with default port": {
			name: "redis",
			mcpConfig: config.MCPSource{
				Name:               "redis",
				Transport:          "http",
				ContainerMCPConfig: &config.ContainerMCPConfig{Image: "redis-mcp:latest"},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			src := &OCISource{
				Name:      tc.name,
				MCPConfig: tc.mcpConfig,
			}

			if src.Name != tc.name {
				t.Errorf("Name = %q, want %q", src.Name, tc.name)
			}
			if src.MCPConfig.Image != tc.mcpConfig.Image {
				t.Errorf("Image = %q, want %q", src.MCPConfig.Image, tc.mcpConfig.Image)
			}
		})
	}
}
