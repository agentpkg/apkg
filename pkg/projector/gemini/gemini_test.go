package gemini

import (
	"testing"
)

func TestSupportsSkills(t *testing.T) {
	g := &geminiProjector{}
	if !g.SupportsSkills() {
		t.Error("SupportsSkills() = false, want true")
	}
}
