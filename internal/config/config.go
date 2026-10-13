package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nicolegros/skl/internal/lock"
	"gopkg.in/yaml.v3"
)

// Config is skl.yaml: where skills are linked and which skills are wanted.
type Config struct {
	Directories []string          // ~-expanded
	Skills      map[string]Source // installed name -> source
	path        string
	text        string // file contents as last read or written, the base Save edits
}

// Dir returns the directory where skl.yaml and the lock file live.
func Dir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "skl")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "skl")
}

// LockPath returns the path of the lock file.
func LockPath() string {
	return filepath.Join(Dir(), "skl.lock")
}

// DataDir returns the directory holding the skill store and backups.
func DataDir() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "skl")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "skl")
}

// StoreDir returns the directory holding the single copy of each installed skill.
func StoreDir() string {
	return filepath.Join(DataDir(), "skills")
}

// BackupsDir returns the directory holding backups made before overwriting.
func BackupsDir() string {
	return filepath.Join(DataDir(), "backups")
}

const defaultDirectories = `# Directories where skills are installed
directories:
  - ~/.kiro/skills       # Kiro
  - ~/.copilot/skills    # Copilot
  - ~/.claude/skills     # Claude
  - ~/.agents/skills     # Codex
  - ~/.pi/agent/skills   # Pi
`

// Load reads skl.yaml, creating it from config.yaml and the lock, or from defaults, if missing.
// The whole file is validated; directories are returned ~-expanded.
func Load() (*Config, error) {
	path := filepath.Join(Dir(), "skl.yaml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return migrate(path)
	}
	if err != nil {
		return nil, err
	}
	return parse(path, string(data))
}

// migrate writes skl.yaml from config.yaml (or defaults) plus one skills entry per lock entry, then deletes config.yaml.
func migrate(path string) (*Config, error) {
	legacy := filepath.Join(filepath.Dir(path), "config.yaml")
	text := defaultDirectories
	data, err := os.ReadFile(legacy)
	switch {
	case err == nil:
		text = string(data)
	case !os.IsNotExist(err):
		return nil, err
	}
	// Seed even without config.yaml: an empty skills map would prune everything already installed.
	lf, err := lock.Load(LockPath())
	if err != nil {
		return nil, err
	}
	skills := map[string]Source{}
	for _, s := range lf.Skills {
		skills[s.InstalledName()] = Source{Repo: s.Repo, Path: s.Path, Ref: s.Ref}
	}

	c, err := parse(legacy, text)
	if err != nil {
		return nil, err
	}
	c.path, c.Skills = path, skills
	if err := c.Save(); err != nil {
		return nil, err
	}
	if err := os.Remove(legacy); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return c, nil
}

func parse(path, text string) (*Config, error) {
	c := &Config{Skills: map[string]Source{}, path: path, text: text}
	root, err := topMapping(text)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for i := 0; root != nil && i+1 < len(root.Content); i += 2 {
		k, v := root.Content[i], root.Content[i+1]
		switch k.Value {
		case "directories":
			if err := v.Decode(&c.Directories); err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
		case "skills":
			if err := c.parseSkills(v); err != nil {
				return nil, err
			}
		default:
			return nil, c.lineErr(k, "unknown key %q (expected directories or skills)", k.Value)
		}
	}

	store := StoreDir()
	for i, d := range c.Directories {
		expanded := filepath.Clean(expandPath(d))
		if rel, err := filepath.Rel(store, expanded); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("directory %q is inside the skill store %q", d, store)
		}
		c.Directories[i] = expanded
	}
	return c, nil
}

func (c *Config) parseSkills(v *yaml.Node) error {
	if v.Kind == yaml.ScalarNode && v.Tag == "!!null" {
		return nil
	}
	if v.Kind != yaml.MappingNode {
		return c.lineErr(v, "skills must be a map of name: owner/repo[/path][@ref]")
	}
	if v.Style&yaml.FlowStyle != 0 && lastLine(v) != v.Line {
		return c.lineErr(v, "write skills one per line, or as {} on a single line")
	}
	for i := 0; i+1 < len(v.Content); i += 2 {
		k, val := v.Content[i], v.Content[i+1]
		name := k.Value
		switch {
		case k.Kind != yaml.ScalarNode || name == "":
			return c.lineErr(k, "empty skill name")
		case name == "." || name == ".." || strings.ContainsAny(name, `/\`):
			return c.lineErr(k, "invalid skill name %q", name)
		}
		if _, dup := c.Skills[name]; dup {
			return c.lineErr(k, "skill %q is listed twice", name)
		}
		if val.Kind != yaml.ScalarNode || val.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
			return c.lineErr(val, "%s: source must be a single-line string", name)
		}
		src, err := ParseSource(val.Value)
		if err != nil {
			return c.lineErr(val, "%s: %v", name, err)
		}
		c.Skills[name] = src
	}
	return nil
}

func (c *Config) lineErr(n *yaml.Node, format string, a ...any) error {
	return fmt.Errorf("%s:%d: %s", c.path, n.Line, fmt.Sprintf(format, a...))
}

// Save writes Skills back to skl.yaml. Everything outside the skills entries is kept byte for byte.
func (c *Config) Save() error {
	text, err := render(c.text, c.Skills)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, []byte(text), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, c.path); err != nil {
		return err
	}
	c.text = text
	return nil
}

// render edits text line by line using node positions; re-encoding the tree would realign comments and drop blank lines.
func render(text string, skills map[string]Source) (string, error) {
	root, err := topMapping(text)
	if err != nil {
		return "", err
	}
	var lines []string
	if text != "" {
		lines = strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	}

	var key, val *yaml.Node
	for i := 0; root != nil && i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "skills" {
			key, val = root.Content[i], root.Content[i+1]
		}
	}
	switch {
	case key == nil:
		lines = append(lines, block(skills)...)
	case val.Kind == yaml.MappingNode && val.Style&yaml.FlowStyle == 0 && len(skills) > 0:
		lines = editEntries(lines, val, skills)
	default:
		lines = slices.Replace(lines, key.Line-1, lastLine(val), block(skills)...)
	}
	return strings.Join(lines, "\n") + "\n", nil
}

// block renders a whole skills section, sorted by name.
func block(skills map[string]Source) []string {
	if len(skills) == 0 {
		return []string{"skills: {}"}
	}
	out := []string{"skills:"}
	for _, name := range sortedNames(skills) {
		out = append(out, "  "+entry(name, skills[name]))
	}
	return out
}

// editEntries deletes, replaces, and inserts entries of a block mapping without touching other lines.
func editEntries(lines []string, m *yaml.Node, skills map[string]Source) []string {
	indent := strings.Repeat(" ", m.Content[0].Column-1)
	drop := map[int]bool{}
	replace := map[int]string{}
	inserts := map[int][]string{}
	type kept struct {
		name  string
		start int
	}
	var keep []kept
	listed := map[string]bool{}
	end := 0
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i], m.Content[i+1]
		listed[k.Value] = true
		start := headStart(lines, k)
		end = v.Line
		want, ok := skills[k.Value]
		switch {
		case !ok:
			for l := start; l <= v.Line; l++ {
				drop[l] = true
			}
		case want.String() != v.Value:
			replace[v.Line] = replaceValue(lines[v.Line-1], v.Column-1, scalar(want.String()))
		}
		if ok {
			keep = append(keep, kept{k.Value, start})
		}
	}

	for _, name := range sortedNames(skills) {
		if listed[name] {
			continue
		}
		at := end + 1
		for _, k := range keep {
			if k.name > name {
				at = k.start
				break
			}
		}
		inserts[at] = append(inserts[at], indent+entry(name, skills[name]))
	}

	var out []string
	for i, l := range lines {
		out = append(out, inserts[i+1]...)
		if drop[i+1] {
			continue
		}
		if r, ok := replace[i+1]; ok {
			l = r
		}
		out = append(out, l)
	}
	return append(out, inserts[len(lines)+1]...)
}

// headStart returns the first line of the comment block directly above k, or k's own line.
func headStart(lines []string, k *yaml.Node) int {
	if k.HeadComment == "" {
		return k.Line
	}
	comment := strings.Split(k.HeadComment, "\n")
	start := k.Line - len(comment)
	if start < 1 {
		return k.Line
	}
	for i, c := range comment {
		if strings.TrimSpace(lines[start-1+i]) != strings.TrimSpace(c) {
			return k.Line
		}
	}
	return start
}

// replaceValue swaps the scalar token starting at rune column col, keeping any trailing comment.
func replaceValue(line string, col int, value string) string {
	r := []rune(line)
	end := len(r)
	switch r[col] {
	case '"':
		for i := col + 1; i < len(r); i++ {
			if r[i] == '\\' {
				i++
			} else if r[i] == '"' {
				end = i + 1
				break
			}
		}
	case '\'':
		for i := col + 1; i < len(r); i++ {
			if r[i] == '\'' {
				if i+1 < len(r) && r[i+1] == '\'' {
					i++
					continue
				}
				end = i + 1
				break
			}
		}
	default:
		for i := col + 1; i < len(r); i++ {
			if r[i] == '#' && r[i-1] == ' ' {
				end = i
				break
			}
		}
		for end > col && r[end-1] == ' ' {
			end--
		}
	}
	return string(r[:col]) + value + string(r[end:])
}

func entry(name string, src Source) string {
	return scalar(name) + ": " + scalar(src.String())
}

// scalar renders s as YAML, quoting only when a plain scalar would read back differently.
func scalar(s string) string {
	out, _ := yaml.Marshal(s)
	return strings.TrimSuffix(string(out), "\n")
}

func sortedNames(skills map[string]Source) []string {
	names := make([]string, 0, len(skills))
	for name := range skills {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// topMapping parses text and returns its top-level mapping, nil for an empty document.
func topMapping(text string) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("line %d: expected directories and skills keys", root.Line)
	}
	return root, nil
}

func lastLine(n *yaml.Node) int {
	last := n.Line
	for _, c := range n.Content {
		last = max(last, lastLine(c))
	}
	return last
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
