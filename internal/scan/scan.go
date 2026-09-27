// Package scan extracts source facts without evaluating Terraform or loading providers.
package scan

import (
	"fmt"
	"os"
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
}}

func Read(root string) (Inventory, error) {
	inv := Inventory{Files: map[string]string{}}
	groups := map[string][]Symbol{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && (strings.HasPrefix(d.Name(), ".") || d.Name() == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if !strings.HasSuffix(path, ".tf") && !strings.HasSuffix(path, ".tf.json") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		inv.Files[rel] = string(src)
		parser := hclparse.NewParser()
		var f *hcl.File
		var diags hcl.Diagnostics
		if strings.HasSuffix(path, ".json") {
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
		dir := filepath.ToSlash(filepath.Dir(rel))
		if _, ok := groups[dir]; !ok {
			groups[dir] = []Symbol{}
		}
		for _, b := range content.Blocks {
			s := Symbol{Kind: b.Type, File: rel, Line: b.DefRange.Start.Line, Attributes: map[string]string{}}
			if len(b.Labels) == 1 {
				s.Name = b.Labels[0]
			}
			if len(b.Labels) == 2 {
				s.Type = b.Labels[0]
				s.Name = b.Labels[1]
			}
			// Attribute expressions stay verbatim: unknown values must never be guessed.
			attrs, _ := b.Body.JustAttributes()
			for name, a := range attrs {
				r := a.Expr.Range()
				s.Attributes[name] = string(r.SliceBytes(src))
			}
			if b.Type == "terraform" {
				c, _, ds := b.Body.PartialContent(&hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{{Type: "required_providers"}}})
				if ds.HasErrors() {
					return fmt.Errorf("%s", ds.Error())
				}
				for _, p := range c.Blocks {
					as, ds := p.Body.JustAttributes()
					if ds.HasErrors() {
						return fmt.Errorf("%s", ds.Error())
					}
					for n, a := range as {
						s.Attributes["required_providers."+n] = string(a.Expr.Range().SliceBytes(src))
					}
				}
			}
			groups[dir] = append(groups[dir], s)
		}
		return nil
	})
	if err != nil {
		return inv, err
	}
	if len(inv.Files) == 0 {
		return inv, fmt.Errorf("no Terraform files in %s", root)
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
