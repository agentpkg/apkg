package claudecode

import (
	"testing"
)

func TestSupportsSkills(t *testing.T) {
	c := &claudeCodeProjector{}
	if !c.SupportsSkills() {
		t.Error("SupportsSkills() = false, want true")
	}
}
