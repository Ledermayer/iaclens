// Package analyze selects one owner per decision and applies directory-scoped rules.
package analyze

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/Ledermayer/iaclens/internal/config"
	"github.com/Ledermayer/iaclens/internal/jev"
	"github.com/Ledermayer/iaclens/internal/scan"
)

type Evaluate func(context.Context, jev.Request) (jev.Response, error)
type Classification struct {
	Kind   string      `json:"kind" yaml:"kind"`
	Method string      `json:"method" yaml:"method"`
	Reason string      `json:"reason" yaml:"reason"`
	Answer *jev.Answer `json:"answer,omitempty" yaml:"answer,omitempty"`
}
type CheckResult struct {
	ID       string      `json:"id" yaml:"id"`
	Engine   string      `json:"engine" yaml:"engine"`
	Status   string      `json:"status" yaml:"status"`
	Severity string      `json:"severity" yaml:"severity"`
	Message  string      `json:"message" yaml:"message"`
	Evidence []string    `json:"evidence,omitempty" yaml:"evidence,omitempty"`
	Answer   *jev.Answer `json:"answer,omitempty" yaml:"answer,omitempty"`
}
type Unit struct {
	Path           string         `json:"path" yaml:"path"`
	Role           string         `json:"role" yaml:"role"`
	Owner          string         `json:"owner,omitempty" yaml:"owner,omitempty"`
	OwnerScope     string         `json:"owner_scope,omitempty" yaml:"owner_scope,omitempty"`
	Classification Classification `json:"classification" yaml:"classification"`
	Checks         []CheckResult  `json:"checks" yaml:"checks"`
}
type Call struct {
	Path   string       `json:"path" yaml:"path"`
	Stage  string       `json:"stage" yaml:"stage"`
	Result jev.Response `json:"result" yaml:"result"`
}
type Prepared struct {
	Path    string      `json:"path"`
	Stage   string      `json:"stage"`
	Request jev.Request `json:"request"`
}
type Result struct {
	ComponentCounts        map[string]int `json:"component_counts" yaml:"component_counts"`
	ExampleCount           int            `json:"example_count" yaml:"example_count"`
	ClassificationComplete bool           `json:"classification_complete" yaml:"classification_complete"`
	CodebaseKind           string         `json:"codebase_kind" yaml:"codebase_kind"`
	KindScope              string         `json:"kind_scope" yaml:"kind_scope"`
	Units                  []Unit         `json:"units" yaml:"units"`
	Calls                  []Call         `json:"jev_calls,omitempty" yaml:"jev_calls,omitempty"`
	Requests               []Prepared     `json:"-" yaml:"-"`
}

func selectBlocks(m scan.Module, kind string) []scan.Symbol {
	var result []scan.Symbol
	var walk func([]scan.Symbol)
	walk = func(ss []scan.Symbol) {
		for _, s := range ss {
			if s.Kind == kind {
				result = append(result, s)
			}
			walk(s.Children)
		}
	}
	walk(m.Symbols)
	return result
}
func classify(m scan.Module, c config.Classification, override string) Classification {
	if m.Path == "." && override != "auto" {
		return Classification{Kind: override, Method: "override", Reason: "--kind"}
	}
	if k, ok := c.Overrides[m.Path]; ok {
		return Classification{Kind: k, Method: "override", Reason: "ruleset path override"}
	}
	kinds := map[string]bool{}
	var reasons []string
	for _, s := range c.Signals {
		if len(selectBlocks(m, s.Block)) > 0 {
			kinds[s.Kind] = true
			reasons = append(reasons, s.ID)
		}
	}
	if len(kinds) > 1 {
		return Classification{Kind: "unknown", Method: "conflict", Reason: "conflicting configured signals: " + strings.Join(reasons, ", ")}
	}
	for k := range kinds {
		return Classification{Kind: k, Method: "go", Reason: strings.Join(reasons, ", ")}
	}
	return Classification{Kind: "unknown", Method: "pending", Reason: "no conclusive configured signal"}
}
func state(inv scan.Inventory, m scan.Module, unit Unit) any {
	files := map[string]string{}
	for p, src := range inv.Files {
		if path.Dir(p) == m.Path {
			files[p] = src
		}
	}
	return map[string]any{"path": m.Path, "role": unit.Role, "owner": unit.Owner, "files": files}
}
func accepted(a jev.Answer, confidence, probability float64) bool {
	return a.Choice != "unknown" && a.Confidence >= confidence && a.Probabilities[a.Choice] >= probability
}
func hasKind(kinds []string, k string) bool {
	for _, v := range kinds {
		if v == k {
			return true
		}
	}
	return false
}
func deterministic(m scan.Module, c config.Check) CheckResult {
	r := CheckResult{ID: c.ID, Engine: "go", Severity: c.Severity, Message: c.Message, Status: "pass"}
	blocks := selectBlocks(m, c.Block)
	count := 0
	failed := false
	uncertain := false
	for _, s := range blocks {
		r.Evidence = append(r.Evidence, fmt.Sprintf("%s:%d", s.File, s.Line))
		if c.Attribute == "" {
			count++
			continue
		}
		_, present := s.Attributes[c.Attribute]
		if present {
			count++
		}
		if !present {
			failed = true
			continue
		}
		if c.Assert == "all_nonempty" {
			v, known := s.Literals[c.Attribute]
			if !known {
				uncertain = true
			} else if strings.TrimSpace(v) == "" {
				failed = true
			}
		}
	}
	switch c.Assert {
	case "exists":
		failed = count == 0
	case "absent":
		failed = count != 0
	case "all_have_attribute", "all_nonempty":
		if len(blocks) == 0 {
			r.Status = "not_applicable"
			return r
		}
	}
	if failed {
		r.Status = "fail"
	} else if uncertain {
		r.Status = "unchecked"
	}
	return r
}

// Run never sends the same classification to both Go and Jev. Classification
// precedes profile selection; only enabled applicable semantic checks are sent.
func Run(ctx context.Context, inv scan.Inventory, c config.Config, mode, model, override string, eval Evaluate) (Result, error) {
	result := Result{Units: []Unit{}}
	if override != "auto" && override != "module" && override != "deployment" && override != "unknown" {
		return result, fmt.Errorf("invalid --kind %q", override)
	}
	if override != "auto" {
		found := false
		for _, m := range inv.Modules {
			found = found || m.Path == "."
		}
		if !found {
			return result, fmt.Errorf("--kind requires Terraform files at --source; use ruleset path overrides for a container")
		}
	}
	if mode == "jev" && eval == nil {
		return result, fmt.Errorf("live mode requires an evaluator")
	}
	ownership, err := structure(inv, c.Structure)
	if err != nil {
		return result, err
	}
	for _, m := range inv.Modules {
		u := Unit{Path: m.Path, Classification: classify(m, c.Classification, override), Checks: []CheckResult{}}
		u.Role = ownership[m.Path].Role
		u.Owner = ownership[m.Path].Owner
		u.OwnerScope = ownership[m.Path].OwnerScope
		if u.Classification.Method == "pending" {
			req := jev.Build(state(inv, m, u), model, map[string]jev.Question{"classification": c.Classification.Question})
			if mode == "prepare" {
				result.Requests = append(result.Requests, Prepared{Path: m.Path, Stage: "classification", Request: req})
				u.Classification.Method = "prepared"
			}
			if mode == "offline" {
				u.Classification.Method = "unavailable"
				u.Classification.Reason = "Jev not run; set a path override or --kind if known"
			}
			if mode == "jev" {
				resp, err := eval(ctx, req)
				if err != nil {
					return result, fmt.Errorf("classify %s: %w", m.Path, err)
				}
				result.Calls = append(result.Calls, Call{Path: m.Path, Stage: "classification", Result: resp})
				a := resp.Answers["classification"]
				u.Classification = Classification{Kind: "unknown", Method: "jev", Reason: "insufficient evidence or confidence; profile selection withheld", Answer: &a}
				if accepted(a, c.Classification.MinConfidence, c.Classification.MinProbability) {
					u.Classification.Kind = a.Choice
					u.Classification.Reason = "accepted by configured thresholds"
				}
			}
		}
		questions := map[string]jev.Question{}
		indexes := map[string]int{}
		policies := map[string]config.Check{}
		for _, check := range c.Checks {
			if !check.Active() {
				continue
			}
			applicable := hasKind(check.AppliesTo, u.Classification.Kind)
			if !applicable && u.Classification.Kind != "unknown" {
				continue
			}
			if !applicable {
				u.Checks = append(u.Checks, CheckResult{ID: check.ID, Engine: check.Engine, Status: "unchecked", Severity: check.Severity, Message: "code kind unresolved; " + check.Message})
				continue
			}
			if check.Engine == "go" {
				u.Checks = append(u.Checks, deterministic(m, check))
				continue
			}
			r := CheckResult{ID: check.ID, Engine: "jev", Status: "unchecked", Severity: check.Severity, Message: check.Message}
			indexes[check.ID] = len(u.Checks)
			policies[check.ID] = check
			u.Checks = append(u.Checks, r)
			questions[check.ID] = check.Question
		}
		if len(questions) > 0 {
			req := jev.Build(state(inv, m, u), model, questions)
			if mode == "prepare" {
				result.Requests = append(result.Requests, Prepared{Path: m.Path, Stage: "checks", Request: req})
			}
			if mode == "jev" {
				resp, err := eval(ctx, req)
				if err != nil {
					return result, fmt.Errorf("checks %s: %w", m.Path, err)
				}
				result.Calls = append(result.Calls, Call{Path: m.Path, Stage: "checks", Result: resp})
				for id, a := range resp.Answers {
					r := &u.Checks[indexes[id]]
					r.Answer = &a
					p := policies[id]
					if accepted(a, p.MinConfidence, p.MinProbability) {
						r.Status = a.Choice
					}
				}
			}
		}
		result.Units = append(result.Units, u)
	}
	result.KindScope = "repository components (examples excluded)"
	known := map[string]bool{}
	unknown := false
	result.ComponentCounts = map[string]int{"module": 0, "deployment": 0, "unknown": 0}
	for _, u := range result.Units {
		if u.Role == "example" {
			result.ExampleCount++
			continue
		}
		result.ComponentCounts[u.Classification.Kind]++
		if u.Classification.Kind == "unknown" {
			unknown = true
		} else {
			known[u.Classification.Kind] = true
		}
	}
	result.ClassificationComplete = !unknown && len(known) > 0
	result.CodebaseKind = "unknown"
	if len(known) > 1 {
		result.CodebaseKind = "mixed"
	} else if !unknown {
		for k := range known {
			result.CodebaseKind = k
		}
	}

	return result, nil
}

// structure assigns ownership separately from the Terraform code's execution role.
// Examples retain their own checks, but never count as independent components.
func structure(inv scan.Inventory, c config.Structure) (map[string]Unit, error) {
	units := map[string]Unit{}
	for _, m := range inv.Modules {
		units[m.Path] = Unit{Role: "component"}
	}
	for _, m := range inv.Modules {
		parts := strings.Split(m.Path, "/")
		for i, part := range parts {
			match := false
			for _, name := range c.ExampleDirs {
				if part == name {
					match = true
				}
			}
			if !match {
				continue
			}
			parent := strings.Join(parts[:i], "/")
			if parent == "" {
				parent = "."
			}
			owner, scope := ".", "repository"
			for {
				if p, ok := units[parent]; ok && p.Role != "example" {
					owner, scope = parent, "component"
					break
				}
				if parent == "." {
					break
				}
				parent = path.Dir(parent)
			}
			units[m.Path] = Unit{Role: "example", Owner: owner, OwnerScope: scope}
			break
		}
	}
	// Explicit assignments are resolved before validating targets, so map order
	// cannot turn an example into another example's component owner.
	for child, owner := range c.ExampleOwners {
		if _, ok := units[child]; !ok {
			return nil, fmt.Errorf("example path %s has no Terraform files", child)
		}
		units[child] = Unit{Role: "example", Owner: owner, OwnerScope: "component"}
	}
	for child, u := range units {
		if u.Role == "example" && u.OwnerScope == "component" {
			p, ok := units[u.Owner]
			if !ok || p.Role != "component" {
				return nil, fmt.Errorf("example %s owner %s is not a component", child, u.Owner)
			}
		}
	}
	return units, nil
}
