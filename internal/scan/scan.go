// Package scan extracts source facts without evaluating Terraform or loading providers.
package scan

import (
	"fmt"
	"github.com/zclconf/go-cty/cty"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"gopkg.in/yaml.v3"
)

type Symbol struct {
	Kind       string            `yaml:"kind" json:"kind"`
	Type       string            `yaml:"type,omitempty" json:"type,omitempty"`
	Name       string            `yaml:"name,omitempty" json:"name,omitempty"`
	File       string            `yaml:"file" json:"file"`
	Line       int               `yaml:"line" json:"line"`
	Attributes map[string]string `yaml:"attributes,omitempty" json:"attributes,omitempty"`
	Literals   map[string]string `yaml:"literals,omitempty" json:"literals,omitempty"`
	Children   []Symbol          `yaml:"children,omitempty" json:"children,omitempty"`
}
type Module struct {
	Path    string   `yaml:"path" json:"path"`
	Symbols []Symbol `yaml:"symbols" json:"symbols"`
}
type Inventory struct {
	Modules []Module          `yaml:"modules" json:"modules"`
	Files   map[string]string `yaml:"-" json:"files"`
}

var schema = &hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{
	{Type: "resource", LabelNames: []string{"type", "name"}}, {Type: "data", LabelNames: []string{"type", "name"}},
	{Type: "ephemeral", LabelNames: []string{"type", "name"}}, {Type: "action", LabelNames: []string{"type", "name"}},
	{Type: "variable", LabelNames: []string{"name"}}, {Type: "output", LabelNames: []string{"name"}},
	{Type: "module", LabelNames: []string{"name"}}, {Type: "provider", LabelNames: []string{"name"}},
	{Type: "terraform"}, {Type: "locals"}, {Type: "check", LabelNames: []string{"name"}},
	{Type: "import"}, {Type: "moved"}, {Type: "removed"},
}}

// Options define collection policy; HCL grammar remains implemented by the parser.
type Options struct {
	Include      []string `yaml:"include"`
	ExcludeDirs  []string `yaml:"exclude_dirs"`
	ExcludePaths []string `yaml:"exclude_paths"`
	Blocks       []string `yaml:"blocks"`
}

var nested = map[string][]hcl.BlockHeaderSchema{
	"terraform": {{Type: "backend", LabelNames: []string{"type"}}, {Type: "cloud"}, {Type: "required_providers"}},
	"variable":  {{Type: "validation"}},
	"resource":  {{Type: "lifecycle"}},
	"data":      {{Type: "lifecycle"}},
	"output":    {{Type: "precondition"}},
	"check":     {{Type: "assert"}},
}

func KnownBlock(name string) bool {
	for _, s := range schema.Blocks {
		if name == s.Type {
			return true
		}
		for _, n := range nested[s.Type] {
			if name == s.Type+"."+n.Type {
				return true
			}
		}
	}
	return false
}
func (o Options) Validate() error {
	if len(o.Include) == 0 || len(o.Blocks) == 0 {
		return fmt.Errorf("parse.include and parse.blocks cannot be empty")
	}
	for _, pattern := range o.Include {
		if _, err := path.Match(pattern, "main.tf"); err != nil {
			return fmt.Errorf("invalid include glob: %w", err)
		}
	}
	for _, dir := range o.ExcludeDirs {
		if dir == "" || strings.ContainsAny(dir, "/\\") {
			return fmt.Errorf("exclude_dirs must contain directory basenames")
		}
	}
	for _, p := range o.ExcludePaths {
		if path.Clean(p) != p || path.IsAbs(p) || p == "." || p == ".." || strings.HasPrefix(p, "../") || strings.Contains(p, "\\") {
			return fmt.Errorf("invalid exclude path %q", p)
		}
	}
	for _, b := range o.Blocks {
		if strings.Contains(b, ".") || !KnownBlock(b) {
			return fmt.Errorf("unsupported collected block %q", b)
		}
	}
	return nil
}
func symbol(b *hcl.Block, src []byte, rel string, kind string) (Symbol, error) {
	s := Symbol{Kind: kind, File: rel, Line: b.DefRange.Start.Line, Attributes: map[string]string{}, Literals: map[string]string{}}
	if len(b.Labels) == 1 {
		s.Name = b.Labels[0]
	}
	if len(b.Labels) == 2 {
		s.Type = b.Labels[0]
		s.Name = b.Labels[1]
	}
	children, rest, diags := b.Body.PartialContent(&hcl.BodySchema{Blocks: nested[b.Type]})
	if diags.HasErrors() {
		return s, fmt.Errorf("%s", diags.Error())
	}
	// JustAttributes may report other valid nested blocks; extract attributes while
	// retaining their original source in Files. This is not Terraform validation.
	attrs, _ := rest.JustAttributes()
	for name, a := range attrs {
		s.Attributes[name] = string(a.Expr.Range().SliceBytes(src))
		val, ds := a.Expr.Value(nil)
		if !ds.HasErrors() && val.IsKnown() && !val.IsNull() && val.Type() == cty.String {
			s.Literals[name] = val.AsString()
		}
	}
	for _, child := range children.Blocks {
		c, err := symbol(child, src, rel, kind+"."+child.Type)
		if err != nil {
			return s, err
		}
		s.Children = append(s.Children, c)
		if kind == "terraform" && child.Type == "required_providers" {
			for n, v := range c.Attributes {
				s.Attributes["required_providers."+n] = v
			}
		}
	}
	return s, nil
}
func Read(root string, opts Options) (Inventory, error) {
	inv := Inventory{Files: map[string]string{}}
	if err := opts.Validate(); err != nil {
		return inv, err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return inv, err
	}
	if !info.IsDir() {
		return inv, fmt.Errorf("source must be a directory, not a file or symlink")
	}
	groups := map[string][]Symbol{}
	selected := map[string]bool{}
	for _, k := range opts.Blocks {
		selected[k] = true
	}
	err = filepath.WalkDir(root, func(filename string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, filename)
		rel = filepath.ToSlash(rel)
		excluded := false
		for _, p := range opts.ExcludePaths {
			if rel == p || strings.HasPrefix(rel, p+"/") {
				excluded = true
			}
		}
		if d.IsDir() {
			for _, name := range opts.ExcludeDirs {
				if filename != root && d.Name() == name {
					excluded = true
				}
			}
			if excluded {
				return filepath.SkipDir
			}
			return nil
		}
		if excluded || d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		// Terraform code only, even if a broad user glob matches other files.
		if !strings.HasSuffix(filename, ".tf") && !strings.HasSuffix(filename, ".tf.json") {
			return nil
		}
		include := false
		for _, pattern := range opts.Include {
			if match, _ := path.Match(pattern, path.Base(rel)); match {
				include = true
			}
		}
		if !include {
			return nil
		}
		src, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		inv.Files[rel] = string(src)
		parser := hclparse.NewParser()
		var f *hcl.File
		var diags hcl.Diagnostics
		if strings.HasSuffix(filename, ".json") {
			f, diags = parser.ParseJSON(src, rel)
		} else {
			f, diags = parser.ParseHCL(src, rel)
		}
		if diags.HasErrors() {
			return fmt.Errorf("%s", diags.Error())
		}
		content, _, diags := f.Body.PartialContent(schema)
		if diags.HasErrors() {
			return fmt.Errorf("%s", diags.Error())
		}
		dir := path.Dir(rel)
		if _, ok := groups[dir]; !ok {
			groups[dir] = []Symbol{}
		}
		for _, b := range content.Blocks {
			if !selected[b.Type] {
				continue
			}
			s, err := symbol(b, src, rel, b.Type)
			if err != nil {
				return err
			}
			groups[dir] = append(groups[dir], s)
		}
		return nil
	})
	if err != nil {
		return inv, err
	}
	if len(inv.Files) == 0 {
		return inv, fmt.Errorf("no Terraform files in %s after scan filters", root)
	}
	dirs := make([]string, 0, len(groups))
	for dir := range groups {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	for _, dir := range dirs {
		inv.Modules = append(inv.Modules, Module{Path: dir, Symbols: groups[dir]})
	}
	return inv, nil
}

// Render preserves all text outside this tool's owned block.
func Render(existing []byte, document any) ([]byte, error) {
	const begin = "# BEGIN IACLENS"
	const end = "# END IACLENS"
	payload, err := yaml.Marshal(document)
	if err != nil {
		return nil, err
	}
	block := begin + "\n" + string(payload) + end + "\n"
	text := string(existing)
	lines := strings.SplitAfter(text, "\n")
	start, finish, offset := -1, -1, 0
	for _, line := range lines {
		trim := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if trim == begin {
			if start != -1 {
				return nil, fmt.Errorf("duplicate IACLENS block")
			}
			start = offset
		}
		if trim == end {
			if finish != -1 || start < 0 {
				return nil, fmt.Errorf("invalid IACLENS markers")
			}
			finish = offset + len(line)
		}
		offset += len(line)
	}
	if (start < 0) != (finish < 0) {
		return nil, fmt.Errorf("incomplete IACLENS block")
	}
	if start >= 0 {
		return []byte(text[:start] + block + text[finish:]), nil
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return []byte(text + block), nil
}
