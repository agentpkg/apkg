# apkg

apkg is like NPM for coding agents. It allows you to install and manage Skills, MCP Servers (coming soon), and Subagents (coming soon) across
the coding agents of your choice.

While coding agents and applications like Claude Code, Gemini CLI, and Cursor often have similar features such as skills, mcp servers, and subagents,
they all require them to be configured in different ways. This forces repos to decide between:
1. Have config files for many (or all) of these agents, and try to keep them in sync
2. Support only one agent
3. Support no agents

Similarly, for global config of these agents, developers must struggle with the same tradeoffs on their machines.

With `apkg`, the `apkg.toml` file becomes the single source of truth: all skills/mcp servers/etc. are included in the file. From there, `apkg install`
will install everything you need, and configure them for the agent(s) of your choice!

## Getting Started

### Install

**macOS (Homebrew)**

```bash
brew install agentpkg/apkg/apkg
```

**Fedora / RHEL (dnf)**

Install directly from the release URL (replace `<version>` with the desired release tag, e.g. `v0.1.0`):

```bash
# x86_64
sudo dnf install https://github.com/agentpkg/agentpkg/releases/download/<version>/apkg-<version>-1.x86_64.rpm

# aarch64
sudo dnf install https://github.com/agentpkg/agentpkg/releases/download/<version>/apkg-<version>-1.aarch64.rpm
```

**Debian / Ubuntu**

Download the `.deb` package from the [releases page](https://github.com/agentpkg/agentpkg/releases) and install it:

```bash
sudo dpkg -i apkg_<version>_amd64.deb
```

**Manual (Linux / macOS)**

Download the appropriate ZIP archive from the [releases page](https://github.com/agentpkg/agentpkg/releases) and place the `apkg` binary somewhere on your `PATH`.

### Usage

1. Run `apkg init` in your repo, or `apkg install` if there is already a `apkg.toml` file in your repo
2. Add any new skills you want with `apkg install skill owner/repo/path/to/skill@githubref`

