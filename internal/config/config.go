// Package config loads declarative Terraform-only rulesets. It never runs scripts.
package config

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/Ledermayer/iaclens/internal/jev"
	"github.com/Ledermayer/iaclens/internal/scan"
	"github.com/Ledermayer/iaclens/rules"
	"gopkg.in/yaml.v3"
)

type Signal struct {
	ID    string `yaml:"id"`
	Kind  string `yaml:"kind"`
	Block string `yaml:"block"`
}
type Classification struct {
	Overrides      map[string]string `yaml:"overrides"`
	Signals        []Signal          `yaml:"signals"`
	MinConfidence  float64           `yaml:"min_confidence"`
	MinProbability float64           `yaml:"min_probability"`
	Question       jev.Question      `yaml:"question"`
}
type Check struct {
	ID             string       `yaml:"id"`
	Engine         string       `yaml:"engine"`
	Enabled        *bool        `yaml:"enabled"`
	AppliesTo      []string     `yaml:"applies_to"`
	Severity       string       `yaml:"severity"`
	Block          string       `yaml:"block"`
	Attribute      string       `yaml:"attribute"`
	Assert         string       `yaml:"assert"`
	Message        string       `yaml:"message"`
	MinConfidence  float64      `yaml:"min_confidence"`
	MinProbability float64      `yaml:"min_probability"`
	Question       jev.Question `yaml:"question"`
}

func (c Check) Active() bool { return c.Enabled == nil || *c.Enabled }

type Structure struct {
	ExampleDirs   []string          `yaml:"example_dirs"`
	ExampleOwners map[string]string `yaml:"example_owners"`
}
type Config struct {
	Version        int            `yaml:"version"`
	Name           string         `yaml:"name"`
	Structure      Structure      `yaml:"structure"`
	Parse          scan.Options   `yaml:"parse"`
	Classification Classification `yaml:"classification"`
	Checks         []Check        `yaml:"checks"`
	Digest         string         `yaml:"-"`
}

func Load(filename string) (Config, error) {
	data := rules.Default
	if filename != "" {
		var err error
		data, err = os.ReadFile(filename)
		if err != nil {
			return Config{}, err
		}
	}
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return c, fmt.Errorf("ruleset: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return c, fmt.Errorf("ruleset must contain one YAML document")
	}
	c.Digest = fmt.Sprintf("%x", sha256.Sum256(data))
	return c, c.Validate()
}
func kind(s string) bool           { return s == "module" || s == "deployment" || s == "unknown" }
func thresholds(c, p float64) bool { return c >= 0 && c <= 1 && p >= 0 && p <= 1 }
func question(q jev.Question, keys ...string) error {
	if q.Type != "choice" || strings.TrimSpace(q.Instructions) == "" {
		return fmt.Errorf("question requires type choice and instructions")
	}
	if len(q.Criteria) != len(keys) {
		return fmt.Errorf("criteria must contain exactly %v", keys)
	}
	for _, k := range keys {
		if strings.TrimSpace(q.Criteria[k]) == "" {
			return fmt.Errorf("missing criteria description for %s", k)
		}
	}
	return nil
}
func (c Config) Validate() error {
	if c.Version != 1 || c.Name == "" {
		return fmt.Errorf("ruleset requires version: 1 and name")
	}
	for _, dir := range c.Structure.ExampleDirs {
		if dir == "" || strings.ContainsAny(dir, "/\\") {
			return fmt.Errorf("structure.example_dirs requires directory basenames")
		}
	}
	for child, parent := range c.Structure.ExampleOwners {
		if child == parent || !cleanPath(child) || !cleanPath(parent) {
			return fmt.Errorf("invalid example ownership %q -> %q", child, parent)
		}
	}
	if err := c.Parse.Validate(); err != nil {
		return err
	}
	for p, k := range c.Classification.Overrides {
		if !kind(k) || path.Clean(p) != p || path.IsAbs(p) || p == ".." || strings.HasPrefix(p, "../") || strings.Contains(p, "\\") {
			return fmt.Errorf("invalid classification override %q: %q", p, k)
		}
	}
	if !thresholds(c.Classification.MinConfidence, c.Classification.MinProbability) {
		return fmt.Errorf("invalid classification thresholds")
	}
	if err := question(c.Classification.Question, "module", "deployment", "unknown"); err != nil {
		return err
	}
	ids := map[string]bool{}
	validateBlock := func(b string) error {
		if !scan.KnownBlock(b) {
			return fmt.Errorf("unsupported block selector %q", b)
		}
		root := strings.Split(b, ".")[0]
		for _, v := range c.Parse.Blocks {
			if root == v {
				return nil
			}
		}
		return fmt.Errorf("selector %s requires collected block %s", b, root)
	}
	for _, s := range c.Classification.Signals {
		if s.ID == "" || ids[s.ID] || (s.Kind != "module" && s.Kind != "deployment") {
			return fmt.Errorf("invalid/duplicate signal %q", s.ID)
		}
		ids[s.ID] = true
		if err := validateBlock(s.Block); err != nil {
			return err
		}
	}
	ids = map[string]bool{}
	for _, r := range c.Checks {
		if r.ID == "" || ids[r.ID] {
			return fmt.Errorf("empty/duplicate check ID %q", r.ID)
		}
		ids[r.ID] = true
		if len(r.AppliesTo) == 0 {
			return fmt.Errorf("check %s requires applies_to", r.ID)
		}
		for _, k := range r.AppliesTo {
			if !kind(k) {
				return fmt.Errorf("invalid kind %q", k)
			}
		}
		if r.Severity != "info" && r.Severity != "warning" && r.Severity != "error" {
			return fmt.Errorf("invalid severity for %s", r.ID)
		}
		switch r.Engine {
		case "go":
			if err := validateBlock(r.Block); err != nil {
				return err
			}
			if r.Assert != "exists" && r.Assert != "absent" && r.Assert != "all_have_attribute" && r.Assert != "all_nonempty" {
				return fmt.Errorf("unsupported assertion %q", r.Assert)
			}
			if strings.HasPrefix(r.Assert, "all_") && r.Attribute == "" {
				return fmt.Errorf("%s requires attribute", r.ID)
			}
			if r.Question.Type != "" || r.Question.Instructions != "" || len(r.Question.Criteria) > 0 || r.MinConfidence != 0 || r.MinProbability != 0 {
				return fmt.Errorf("Go check %s contains Jev settings", r.ID)
			}
		case "jev":
			if r.Block != "" || r.Attribute != "" || r.Assert != "" {
				return fmt.Errorf("Jev check %s contains Go predicates", r.ID)
			}
			if !thresholds(r.MinConfidence, r.MinProbability) {
				return fmt.Errorf("invalid thresholds for %s", r.ID)
			}
			if err := question(r.Question, "pass", "fail", "unknown"); err != nil {
				return fmt.Errorf("%s: %w", r.ID, err)
			}
		default:
			return fmt.Errorf("unsupported engine %q", r.Engine)
		}
	}
	return nil
}

func cleanPath(p string) bool {
	return p != "" && path.Clean(p) == p && !path.IsAbs(p) && p != ".." && !strings.HasPrefix(p, "../") && !strings.Contains(p, "\\")
}
