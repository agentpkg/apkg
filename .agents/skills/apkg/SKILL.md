---
name: manage-agent-packages
description: Agent Package Manager (MCP servers, agent skills, etc.). Installs, configures, and manages packages for coding agents.
---

## Agent Package Manager

You manage packages (MCP server, agent skills, subagents, etc.) for various coding agents.

Your task is to use the apkg CLI to manage these packages as asked (adding a new MCP server, removing a MCP server, etc.)

### apkg CLI

#### Install skills

```bash
apkg install skill owner/repo/path@ref # installs from github
apkg install skill ./some/local/path
apkg install skill -g owner/repo/path@ref
```

#### Install MCP servers

```bash
apkg install mcp my-server -t stdio --package npm:@modelcontextprotocol/server-filesystem
apkg install mcp my-server -t stdio --package uv:some-pip-mcp-server==v0.1.0
apkg install mcp my-server -t stdio --package go:github.com/some-repo@latest
apkg install mcp my-server -t stdio --command /usr/local/bin/my-server --args flag1,flag2
apkg install mcp my-server -t http --url https://example.com/mcp
apkg install mcp my-server -t stdio --image my-image:latest
apkg install mcp kubernetes -t http --image quay.io/containers/kubernetes_mcp_server:v0.0.57 --volume ~/.kube/config:/kubeconfig:ro --env KUBECONFIG=/kubeconfig --transport http --newtork kind # requires the kubeconfig from kind export kubeconfig --internal
```

#### Install all

```bash
apkg install # install all the configured packages from apkg.toml
```

#### Remove skill

```bash
apkg remove skill name
```

#### Remove MCP

```bash
apkg remove mcp name
```

#### Remove all

```bash
apkg remove
```

### managing containers

apkg can manage the lifecycle of containers when they are run with http transport.
To do this, you must run in a background process (that lives for the lifetime the mcp server is needed, at least):
```bash
apkg serve
```

