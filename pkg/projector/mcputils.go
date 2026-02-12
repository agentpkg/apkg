package projector

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentpkg/agentpkg/pkg/mcp"
)

const (
	configFilePerms = 0o644
)

func ReadJsonConfig(path string) (map[string]any, error) {
	config := make(map[string]any)

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return config, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read %q: %w", path, err)
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse %q as json: %w", path, err)
	}

	return config, nil
}

func WriteJsonConfig(path string, config map[string]any) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create directory for %q: %w", path, err)
	}

	if err := os.WriteFile(path, data, configFilePerms); err != nil {
		return fmt.Errorf("failed to write %q: %w", path, err)
	}

	return nil
}

func BuildMCPServerJsonConfig(server mcp.MCPServer) map[string]any {
	config := make(map[string]any)

	if server.Transport() == "stdio" {
		config["command"] = server.Command()
		if args := server.Args(); len(args) > 0 {
			config["args"] = args
		}
		if env := server.Env(); len(env) > 0 {
			config["env"] = env
		}
	} else {
		config["type"] = server.Transport()
		config["url"] = server.URL()
		if headers := server.Headers(); len(headers) > 0 {
			config["headers"] = headers
		}
	}

	return config
}

func GetOrCreateMap(parent map[string]any, key string) map[string]any {
	if v, ok := parent[key]; ok {
		if m, ok := v.(map[string]any); ok {
			return m
		}
	}

	m := make(map[string]any)
	parent[key] = m
	
	return m
}
