apkg is a CLI for managing skills/mcp servers/subagents/etc. for coding agents. Think of it as a npm for coding agents.

## Writing CLI Commands

CLI commands are added in pkg/cmd. These should only be handling I/O, command parsing, display (e.g. with bubble tea, huh, etc.), and user input. All the logic should be in other packages.

## Writing tests

All tests in this repo follow a `map[string]struct` table test format
