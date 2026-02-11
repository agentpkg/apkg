package projector

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentpkg/agentpkg/pkg/pkg/skill"
)

// SkillProjector projects skills into a given agent directory by creating
// symlinks under <projectDir>/<agentDir>/skills/<skill-name>.
type SkillProjector struct {
	// AgentDir is the agent-specific directory name (e.g. ".claude", ".gemini").
	AgentDir string
}

func (sp *SkillProjector) ProjectSkills(projectDir string, packages []skill.Skill) error {
	skillsDir := filepath.Join(projectDir, sp.AgentDir, "skills")
	err := os.MkdirAll(skillsDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to make %q dir for skills: %w", skillsDir, err)
	}

	var projectErr error
	for _, p := range packages {
		link := filepath.Join(skillsDir, p.Name())
		// if exists & is symlink - overwrite
		// if exists & is not symlink - error (TODO: accept user input to confirm overwrite?)
		exists, isSymlink := checkExistenceAndIsSymlink(link)
		if !exists {
			err := os.Symlink(p.Dir(), link)
			if err != nil {
				projectErr = errors.Join(projectErr, fmt.Errorf("failed to create symlink for skill %q: %w", p.Name(), err))
			}

			continue
		}

		if isSymlink {
			err := overwriteSymlink(p.Dir(), link)
			if err != nil {
				projectErr = errors.Join(projectErr, fmt.Errorf("failed to overwrite symlink for skill %q: %w", p.Name(), err))
			}
		} else {
			projectErr = errors.Join(projectErr, fmt.Errorf("failed to symlink skill %q: file/dir already exists at path", p.Name()))
		}
	}

	return projectErr
}

func overwriteSymlink(newTargetPath, linkPath string) error {
	tmpLinkPath := fmt.Sprintf("%s.tmp", linkPath)

	if err := os.Remove(tmpLinkPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing temporary link dir: %w", err)
	}

	if err := os.Symlink(newTargetPath, tmpLinkPath); err != nil {
		return fmt.Errorf("failed to create temporary symlink: %w", err)
	}

	if err := os.Rename(tmpLinkPath, linkPath); err != nil {
		os.Remove(tmpLinkPath)
		return fmt.Errorf("failed to rename temporary symlink: %w", err)
	}

	return nil
}

func checkExistenceAndIsSymlink(path string) (exists, isSymlink bool) {
	exists, isSymlink = true, false
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			exists = false
		}

		isSymlink = false
		return
	}

	isSymlink = info.Mode()&os.ModeSymlink == os.ModeSymlink
	return
}
