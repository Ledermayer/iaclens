// exampletest stages each fixture as its own Git repository, runs the real CLI,
// verifies independent expectations, and retains reports even on mismatch.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/Ledermayer/iaclens/internal/analyze"
	"gopkg.in/yaml.v3"
)

var names = []string{"reusable-module", "single-deployment", "module-library", "multi-deployment", "mixed-repository", "custom-policy"}

type UnitExpectation struct {
	Kind   string `json:"kind"`
	Method string `json:"method"`
	Role   string `json:"role"`
	Owner  string `json:"owner"`
}
type Expectation struct {
	Kind            string                       `json:"kind"`
	Components      map[string]int               `json:"components"`
	Examples        int                          `json:"examples"`
	Units           map[string]UnitExpectation   `json:"units"`
	Checks          map[string]map[string]string `json:"checks"`
	MinimumJevCalls int                          `json:"minimum_jev_calls"`
}
type Report struct {
	Iaclens struct {
		Analysis analyze.Result `json:"analysis"`
	} `json:"iaclens"`
}

func verify(report Report, want Expectation) error {
	got := report.Iaclens.Analysis
	if got.CodebaseKind != want.Kind || !reflect.DeepEqual(got.ComponentCounts, want.Components) || got.ExampleCount != want.Examples {
		return fmt.Errorf("repository mismatch: kind=%s components=%v examples=%d; want %s %v %d", got.CodebaseKind, got.ComponentCounts, got.ExampleCount, want.Kind, want.Components, want.Examples)
	}
	if len(got.Calls) < want.MinimumJevCalls {
		return fmt.Errorf("expected at least %d actual Jev calls; got %d", want.MinimumJevCalls, len(got.Calls))
	}
	if len(got.Units) != len(want.Units) {
		return fmt.Errorf("unit count mismatch")
	}
	seen := map[string]bool{}
	for _, u := range got.Units {
		expected, ok := want.Units[u.Path]
		if !ok || seen[u.Path] {
			return fmt.Errorf("unexpected/duplicate unit %s", u.Path)
		}
		seen[u.Path] = true
		if u.Classification.Kind != expected.Kind || u.Classification.Method != expected.Method || u.Role != expected.Role || u.Owner != expected.Owner {
			return fmt.Errorf("unit %s mismatch: kind=%s method=%s role=%s owner=%s; want %+v", u.Path, u.Classification.Kind, u.Classification.Method, u.Role, u.Owner, expected)
		}
		checks := map[string]string{}
		for _, c := range u.Checks {
			checks[c.ID] = c.Status
		}
		for id, status := range want.Checks[u.Path] {
			if checks[id] != status {
				return fmt.Errorf("%s check %s = %q; want %q", u.Path, id, checks[id], status)
			}
		}
	}
	for p := range want.Checks {
		if !seen[p] {
			return fmt.Errorf("expected checks for missing unit %s", p)
		}
	}
	return nil
}
func copySource(source, target string) error {
	return filepath.WalkDir(source, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, p)
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink fixtures are not supported: %s", rel)
		}
		out := filepath.Join(target, rel)
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return os.MkdirAll(out, 0755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(out, data, 0600)
	})
}
func run() error {
	name := flag.String("example", "", "Example archetype")
	mode := flag.String("mode", "offline", "offline or jev")
	bin := flag.String("bin", "work/iaclens", "CLI binary path")
	flag.Parse()
	valid := false
	for _, n := range names {
		valid = valid || n == *name
	}
	if !valid {
		return fmt.Errorf("--example must be one of %v", names)
	}
	if *mode != "offline" && *mode != "jev" {
		return fmt.Errorf("invalid mode")
	}
	binary, err := filepath.Abs(*bin)
	if err != nil {
		return err
	}
	root := filepath.Join("examples", *name)
	expectedBytes, err := os.ReadFile(filepath.Join(root, "expected.json"))
	if err != nil {
		return err
	}
	var expected map[string]Expectation
	dec := json.NewDecoder(bytes.NewReader(expectedBytes))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&expected); err != nil {
		return err
	}
	want, ok := expected[*mode]
	if !ok {
		return fmt.Errorf("missing mode expectations")
	}
	results := filepath.Join(root, "results", *mode)
	if err = os.MkdirAll(results, 0755); err != nil {
		return err
	}
	reportPath, err := filepath.Abs(filepath.Join(results, "report.json"))
	if err != nil {
		return err
	}
	// Never let a failed invocation leave an old success report masquerading as fresh.
	for _, f := range []string{"report.json", "report.yaml", "run.json", "summary.md"} {
		if err = os.Remove(filepath.Join(results, f)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	temp, err := os.MkdirTemp("", "iaclens-example-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	fixture := filepath.Join(temp, *name)
	if err = copySource(filepath.Join(root, "source"), fixture); err != nil {
		return err
	}
	if out, err := exec.Command("git", "init", "-q", fixture).CombinedOutput(); err != nil {
		return fmt.Errorf("git init: %w: %s", err, out)
	}
	cfg, err := filepath.Abs(filepath.Join(root, "rules.yaml"))
	if err != nil {
		return err
	}
	command := exec.Command(binary, "--source", fixture, "--config", cfg, "--mode", *mode, "--provider", "llmgateway", "--format", "json", "--out", reportPath)
	started := time.Now().UTC()
	log, runErr := command.CombinedOutput()
	fmt.Print(string(log))
	var report Report
	var raw map[string]any
	if runErr == nil {
		data, err := os.ReadFile(reportPath)
		if err != nil {
			return err
		}
		if err = json.Unmarshal(data, &report); err != nil {
			return err
		}
		if err = json.Unmarshal(data, &raw); err != nil {
			return err
		}
		// Only the machine-specific staging path is normalized for portable artifacts.
		raw["iaclens"].(map[string]any)["repository"].(map[string]any)["root"] = "source"
		data, err = json.MarshalIndent(raw, "", "  ")
		if err != nil {
			return err
		}
		if err = os.WriteFile(reportPath, append(data, '\n'), 0600); err != nil {
			return err
		}
		y, err := yaml.Marshal(raw)
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(results, "report.yaml"), y, 0600); err != nil {
			return err
		}
		runErr = verify(report, want)
	}
	status := "passed"
	failure := ""
	if runErr != nil {
		status = "failed"
		failure = runErr.Error()
	}
	sha, _ := exec.Command("git", "rev-parse", "HEAD").Output()
	meta := map[string]any{"example": *name, "mode": *mode, "status": status, "error": failure, "started_at": started.Format(time.RFC3339), "duration_ms": time.Since(started).Milliseconds(), "source_revision": strings.TrimSpace(string(sha)), "workflow_sha": os.Getenv("GITHUB_SHA"), "run_id": os.Getenv("GITHUB_RUN_ID"), "run_attempt": os.Getenv("GITHUB_RUN_ATTEMPT"), "report_root_normalization": "Temporary fixture worktree mapped to source"}
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(results, "run.json"), append(b, '\n'), 0600); err != nil {
		return err
	}
	summary := fmt.Sprintf("### %s — %s: %s\n\nRepository kind: `%s`. Components: `%v`. Owned examples: %d. Actual Jev calls: %d.\n\nReports: `examples/%s/results/%s/` in the workflow artifact.\n", *name, *mode, status, report.Iaclens.Analysis.CodebaseKind, report.Iaclens.Analysis.ComponentCounts, report.Iaclens.Analysis.ExampleCount, len(report.Iaclens.Analysis.Calls), *name, *mode)
	if raw == nil {
		summary = fmt.Sprintf("### %s — %s: %s\n\nCLI did not produce a report; classification and completed Jev calls are unavailable. See the workflow log for the API error.\n", *name, *mode, status)
	}
	if runErr != nil {
		summary += "\nValidation: " + failure + "\n"
	}
	if err = os.WriteFile(filepath.Join(results, "summary.md"), []byte(summary), 0600); err != nil {
		return err
	}
	if p := os.Getenv("GITHUB_STEP_SUMMARY"); p != "" {
		f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, err = f.WriteString(summary)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	fmt.Print(summary)
	return runErr
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "exampletest:", err)
		os.Exit(1)
	}
}
