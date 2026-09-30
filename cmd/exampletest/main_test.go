package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ledermayer/iaclens/internal/analyze"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

func TestVerifyContracts(t *testing.T) {
	makeCase := func() (Report, Expectation) {
		var r Report
		r.Iaclens.Analysis = analyze.Result{CodebaseKind: "module", ComponentCounts: map[string]int{"module": 1}, ExampleCount: 1, Calls: []analyze.Call{{Path: ".", Stage: "checks"}}, Units: []analyze.Unit{{Path: "examples/default", Role: "example", Owner: ".", Classification: analyze.Classification{Kind: "deployment", Method: "signal"}, Checks: []analyze.CheckResult{{ID: "deliberate", Status: "fail"}}}}}
		e := Expectation{Kind: "module", Components: map[string]int{"module": 1}, Examples: 1, MinimumJevCalls: 1, Units: map[string]UnitExpectation{"examples/default": {Kind: "deployment", Method: "signal", Role: "example", Owner: "."}}, Checks: map[string]map[string]string{"examples/default": {"deliberate": "fail"}}}
		return r, e
	}
	t.Run("expected policy failure is successful evaluation", func(t *testing.T) {
		r, e := makeCase()
		if err := verify(r, e); err != nil {
			t.Fatal(err)
		}
	})
	cases := map[string]func(*Report){
		"missing live call":        func(r *Report) { r.Iaclens.Analysis.Calls = nil },
		"wrong ownership":          func(r *Report) { r.Iaclens.Analysis.Units[0].Owner = "other" },
		"unexpected policy status": func(r *Report) { r.Iaclens.Analysis.Units[0].Checks[0].Status = "pass" },
		"missing unit":             func(r *Report) { r.Iaclens.Analysis.Units = nil },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			r, e := makeCase()
			change(&r)
			if verify(r, e) == nil {
				t.Fatal("accepted incorrect result")
			}
		})
	}
}

// reportSchema compiles the published contract locally, without fetching schemas.
func reportSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	data, err := os.ReadFile("../../schemas/report-v2.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	const schemaURL = "https://iaclens.invalid/report-v2.schema.json"
	if err := compiler.AddResource(schemaURL, document); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(schemaURL)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

// validateReport checks raw documents; the runner's Report type is intentionally partial.
func validateReport(t *testing.T, schema *jsonschema.Schema, data []byte) map[string]any {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatal(err)
	}
	return document
}

func TestSavedReportsMatchSchema(t *testing.T) {
	schema := reportSchema(t)
	for _, name := range names {
		for _, mode := range []string{"offline", "jev"} {
			t.Run(name+"/"+mode, func(t *testing.T) {
				data, err := os.ReadFile(filepath.Join("../../examples", name, "results", mode, "report.json"))
				if err != nil {
					t.Fatal(err)
				}
				validateReport(t, schema, data)
			})
		}
	}
}

func TestReportSchemaRejectsInvalidDocuments(t *testing.T) {
	schema := reportSchema(t)
	data, err := os.ReadFile("../../examples/custom-policy/results/offline/report.json")
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(map[string]any){
		"schema version":     func(report map[string]any) { report["schema_version"] = 3 },
		"digest":             func(report map[string]any) { report["source_digest"] = "not-a-sha256" },
		"missing repository": func(report map[string]any) { delete(report, "repository") },
		"invalid check status": func(report map[string]any) {
			analysis := report["analysis"].(map[string]any)
			unit := analysis["units"].([]any)[0].(map[string]any)
			unit["checks"].([]any)[0].(map[string]any)["status"] = "unknown"
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			document := validateReport(t, schema, data)
			mutate(document["iaclens"].(map[string]any))
			if err := schema.Validate(document); err == nil {
				t.Fatal("invalid report was accepted")
			}
		})
	}
	document := validateReport(t, schema, data)
	document["future_metadata"] = "compatible extension"
	if err := schema.Validate(document); err != nil {
		t.Fatalf("additive fields must remain compatible: %v", err)
	}
}

func TestFreshCLIReportContract(t *testing.T) {
	schema := reportSchema(t)
	root := t.TempDir()
	fixture := filepath.Join(root, "source")
	if err := copySource("../../examples/reusable-module/source", fixture); err != nil {
		t.Fatal(err)
	}
	if data, err := exec.Command("git", "init", "-q", fixture).CombinedOutput(); err != nil {
		t.Fatalf("initialize fixture: %v: %s", err, data)
	}
	binary := filepath.Join(root, "iaclens.exe")
	build := exec.Command("go", "build", "-o", binary, "../iaclens")
	if data, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v: %s", err, data)
	}
	config, err := filepath.Abs("../../examples/reusable-module/rules.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"offline", "prepare"} {
		for _, format := range []string{"json", "yaml"} {
			t.Run(mode+"/"+format, func(t *testing.T) {
				out := filepath.Join(root, mode+"."+format)
				requests := filepath.Join(root, mode+"-requests.json")
				const prefix = "Operator notes, not YAML\n"
				if format == "yaml" {
					if err := os.WriteFile(out, []byte(prefix), 0600); err != nil {
						t.Fatal(err)
					}
				}
				command := exec.Command(binary, "--source", fixture, "--config", config, "--mode", mode, "--format", format, "--out", out, "--request-out", requests)
				stdout, err := command.Output()
				if err != nil {
					t.Fatal(err)
				}
				if json.Valid(stdout) || !strings.Contains(string(stdout), "Scanned") {
					t.Fatal("stdout must remain a human summary, not the JSON report")
				}
				data, err := os.ReadFile(out)
				if err != nil {
					t.Fatal(err)
				}
				if format == "yaml" {
					if !strings.HasPrefix(string(data), prefix) {
						t.Fatal("operator notes were not preserved")
					}
					_, payload, found := strings.Cut(string(data), "# BEGIN IACLENS\n")
					if !found {
						t.Fatal("missing start marker")
					}
					payload, _, found = strings.Cut(payload, "# END IACLENS")
					if !found {
						t.Fatal("missing end marker")
					}
					var document map[string]any
					if err := yaml.Unmarshal([]byte(payload), &document); err != nil {
						t.Fatal(err)
					}
					data, err = json.Marshal(document)
					if err != nil {
						t.Fatal(err)
					}
				}
				report := validateReport(t, schema, data)["iaclens"].(map[string]any)
				analysis := report["analysis"].(map[string]any)
				if _, found := analysis["jev_calls"]; found {
					t.Fatal("offline/prepare must not call the model")
				}
				inventory := map[string]bool{}
				for _, unit := range report["units"].([]any) {
					inventory[unit.(map[string]any)["path"].(string)] = true
				}
				for _, unit := range analysis["units"].([]any) {
					if !inventory[unit.(map[string]any)["path"].(string)] {
						t.Fatal("analysis path does not match inventory")
					}
				}
				if mode == "prepare" {
					data, err := os.ReadFile(requests)
					if err != nil || !json.Valid(data) || !bytes.Contains(data, []byte(`"files"`)) {
						t.Fatalf("prepare must write source-bearing request data: %v", err)
					}
				}
			})
		}
	}
}
