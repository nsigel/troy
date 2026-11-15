package sandbox

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strings"
	"sync"

	"go.yaml.in/yaml/v3"
)

// Skill is an immutable bundled skill. Version covers instructions and supporting files.
type Skill struct {
	Name         string   `json:"name" yaml:"name"`
	Description  string   `json:"description" yaml:"description"`
	Location     string   `json:"location" yaml:"-"`
	Directory    string   `json:"directory" yaml:"-"`
	Version      string   `json:"version" yaml:"-"`
	Instructions string   `json:"instructions" yaml:"-"`
	Resources    []string `json:"resources" yaml:"-"`
}

var Skills = sync.OnceValues(func() ([]Skill, error) { return discoverSkills(Recipes) })
var skillName = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func discoverSkills(files fs.FS) ([]Skill, error) {
	paths, err := fs.Glob(files, "recipes/*/SKILL.md")
	if err != nil {
		return nil, err
	}
	var skills []Skill
	seen := map[string]bool{}
	for _, filename := range paths {
		b, err := fs.ReadFile(files, filename)
		if err != nil {
			return nil, err
		}
		text := strings.ReplaceAll(string(b), "\r\n", "\n")
		if !strings.HasPrefix(text, "---\n") {
			return nil, fmt.Errorf("%s: missing YAML frontmatter", filename)
		}
		header, body, ok := strings.Cut(text[4:], "\n---\n")
		if !ok {
			return nil, fmt.Errorf("%s: unterminated frontmatter", filename)
		}
		var s Skill
		if err := yaml.Unmarshal([]byte(header), &s); err != nil {
			return nil, fmt.Errorf("%s: %w", filename, err)
		}
		dir := path.Dir(filename)
		if !skillName.MatchString(s.Name) || len(s.Name) > 64 || s.Name != path.Base(dir) || seen[s.Name] || strings.TrimSpace(s.Description) == "" || strings.TrimSpace(body) == "" {
			return nil, fmt.Errorf("%s: invalid/duplicate name, missing description or empty instructions", filename)
		}
		seen[s.Name] = true
		s.Directory = "/usr/local/lib/troy/skills/" + s.Name
		s.Location = s.Directory + "/SKILL.md"
		s.Instructions = strings.TrimSpace(body)
		hash := sha256.New()
		err = fs.WalkDir(files, dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			data, err := fs.ReadFile(files, p)
			if err != nil {
				return err
			}
			fmt.Fprintf(hash, "%s\x00%d\x00", p, len(data))
			hash.Write(data)
			if p != filename {
				s.Resources = append(s.Resources, strings.TrimPrefix(p, dir+"/"))
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		s.Version = fmt.Sprintf("%x", hash.Sum(nil))
		// Tool output must contain the complete instructions. Larger context belongs in references.
		encoded, _ := json.Marshal(s)
		if len(encoded) > 14000 {
			return nil, fmt.Errorf("%s: skill instructions/resource list too large; split into references", filename)
		}
		skills = append(skills, s)
	}
	return skills, nil
}
