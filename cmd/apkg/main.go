package main

import (
	"github.com/agentpkg/agentpkg/pkg/cmd"
	_ "github.com/agentpkg/agentpkg/pkg/projector/claudecode"
	_ "github.com/agentpkg/agentpkg/pkg/projector/cursor"
	_ "github.com/agentpkg/agentpkg/pkg/projector/gemini"
)

func main() {
	cmd.Execute()
}
