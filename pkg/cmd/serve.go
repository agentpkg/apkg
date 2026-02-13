package cmd

import (
	"github.com/agentpkg/agentpkg/pkg/container"
	"github.com/agentpkg/agentpkg/pkg/serve"
	"github.com/agentpkg/agentpkg/pkg/store"
	"github.com/spf13/cobra"
)

func newServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the MCP container proxy",
		Long: `Starts an HTTP proxy that manages containerized MCP servers.

The proxy discovers installed container images by scanning ~/.apkg/oci/
and lazily starts them on first request. Containers are stopped after an
idle timeout and restarted automatically on the next request.

Agent configurations point at this proxy using the X-MCP-Server and
X-MCP-Server-Digest headers for routing.`,
		RunE: runServe,
	}

	cmd.Flags().Int("port", serve.DefaultPort, "Port to listen on")

	return cmd
}

func runServe(cmd *cobra.Command, args []string) error {
	port, err := cmd.Flags().GetInt("port")
	if err != nil {
		return err
	}

	engine, err := container.DetectEngine()
	if err != nil {
		return err
	}

	st, err := store.Default()
	if err != nil {
		return err
	}

	srv, err := serve.NewServerFromStore(st, port, engine)
	if err != nil {
		return err
	}

	return srv.ListenAndServe(cmd.Context())
}
