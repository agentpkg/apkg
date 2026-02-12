package skill

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"sigs.k8s.io/yaml"
)

const (
	TypeSkill      = "skill"
	skillsFileName = "SKILL.md"
)

var (
	yamlFrontMatterDelim = []byte{'-', '-', '-'}
	validSkillNameRegex  = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$`)
)

type Skill interface {
	// Name returns the name of the package
	Name() string
	// Type returns the type of the package (e.g. "skill", or "mcp" in the future)
	Type() string
	// Dir returns where the package contents lives on disk
	Dir() string
	// Validate makes sure package contents are okay
	Validate() error
}

func Load(dir string) (Skill, error) {
	f, err := os.Open(filepath.Join(dir, skillsFileName))
	if err != nil {
		return nil, fmt.Errorf("failed to open %s file in %q", skillsFileName, dir)
	}

	reader := bufio.NewReader(f)
	inFrontMatter := false
	yamlBuffer := bytes.Buffer{}

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("error reading SKILLS.md frontmatter: %w", err)
		}

		if bytes.HasPrefix(line, yamlFrontMatterDelim) {
			if inFrontMatter {
				break
			}

			inFrontMatter = true
			continue
		}

		_, err = yamlBuffer.Write(line)
		if err != nil {
			return nil, fmt.Errorf("error constructing yaml frontmatter buffer while parsing SKILLS.md: %w", err)
		}
	}

	if yamlBuffer.Len() == 0 {
		return nil, fmt.Errorf("%s in %q is missing YAML front matter ('---' delimiters)", skillsFileName, dir)
	}

	s := &skill{dir: dir}
	err = yaml.Unmarshal(yamlBuffer.Bytes(), s)
	return s, err
}

type skill struct {
	SkillName     string            `json:"name"`
	Description   string            `json:"description"`
	License       string            `json:"license,omitempty"`
	Compatability string            `json:"compatability,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	AllowedTools  string            `json:"allowed-tools,omitempty"` // space delimited string
	dir           string
}

func (s *skill) Name() string {
	return s.SkillName
}

func (s *skill) Type() string {
	return TypeSkill
}

func (s *skill) Dir() string {
	return s.dir
}

func (s *skill) Validate() error {
	var err error
	if !validSkillNameRegex.Match([]byte(s.SkillName)) {
		err = errors.Join(err, fmt.Errorf("skill name must be max 64 characters with only lowercase letters, numbers, and hyphens. must not start or end with a hyphen"))
	}

	if len(s.Description) > 1024 {
		err = errors.Join(err, fmt.Errorf("skill description must be max 1024 characters"))
	}
	if len(s.Description) == 0 {
		err = errors.Join(err, fmt.Errorf("skill description must be provided"))
	}

	if len(s.Compatability) > 500 {
		err = errors.Join(err, fmt.Errorf("compatability must be max 500 characters"))
	}

	return err
}
