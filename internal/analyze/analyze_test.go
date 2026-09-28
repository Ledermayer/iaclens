package analyze

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Ledermayer/iaclens/internal/config"
	"github.com/Ledermayer/iaclens/internal/jev"
	"github.com/Ledermayer/iaclens/internal/scan"
)

func fixture(t *testing.T, files map[string]string) (scan.Inventory, config.Config) {
	t.Helper()
	c, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for p, s := range files {
		p = filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0600); err != nil {
			t.Fatal(err)
		}
	}
	inv, err := scan.Read(root, c.Parse)
	if err != nil {
		t.Fatal(err)
	}
	return inv, c
}
func never(t *testing.T) Evaluate {
	return func(context.Context, jev.Request) (jev.Response, error) {
		t.Fatal("unexpected Jev call")
		return jev.Response{}, nil
	}
}
func TestModuleOwnsDeploymentExample(t *testing.T) {
	inv, c := fixture(t, map[string]string{
		"main.tf": `variable "name" { description = "Name" }`,
		"examples/basic/main.tf": `terraform {
 backend "local" {}
}`,
	})
	c.Classification.Overrides = map[string]string{".": "module"}
	got, err := Run(context.Background(), inv, c, "jev", "test", "auto", never(t))
	if err != nil {
		t.Fatal(err)
	}
	if got.CodebaseKind != "module" || got.ExampleCount != 1 || got.ComponentCounts["deployment"] != 0 {
		t.Fatalf("%+v", got)
	}
	ex := got.Units[1]
	if ex.Role != "example" || ex.Owner != "." || ex.OwnerScope != "component" || ex.Classification.Kind != "deployment" {
		t.Fatalf("%+v", ex)
	}
	for _, r := range got.Units[0].Checks {
		if r.ID == "deployment-core-version" {
			t.Fatal("deployment rule ran on module")
		}
	}
	for _, r := range ex.Checks {
		if r.ID == "module-state-is-caller-owned" {
			t.Fatal("module rule ran on deployment example")
		}
	}
}
func TestMixedRepositoryAndNestedOwnership(t *testing.T) {
	inv, c := fixture(t, map[string]string{
		"modules/a/main.tf": `output "id" { value = "a" }`,
		"modules/a/examples/basic/main.tf": `terraform {
 backend "local" {}
}`,
		"modules/b/main.tf": `output "id" { value = "b" }`,
		"env/dev/main.tf": `terraform {
 backend "local" {}
}`,
		"env/prod/main.tf": `terraform {
 cloud { organization = "demo" }
}`,
	})
	c.Classification.Overrides = map[string]string{"modules/a": "module", "modules/b": "module"}
	got, err := Run(context.Background(), inv, c, "jev", "test", "auto", never(t))
	if err != nil {
		t.Fatal(err)
	}
	if got.CodebaseKind != "mixed" || got.ComponentCounts["module"] != 2 || got.ComponentCounts["deployment"] != 2 || got.ExampleCount != 1 {
		t.Fatalf("%+v", got)
	}
	for _, u := range got.Units {
		if u.Role == "example" && u.Owner != "modules/a" {
			t.Fatal(u)
		}
	}
}
func TestAmbiguousOfflineWithholdsProfile(t *testing.T) {
	inv, c := fixture(t, map[string]string{"main.tf": `resource "terraform_data" "x" { input = "x" }`})
	got, err := Run(context.Background(), inv, c, "offline", "test", "auto", never(t))
	if err != nil {
		t.Fatal(err)
	}
	if got.CodebaseKind != "unknown" {
		t.Fatal(got)
	}
	for _, r := range got.Units[0].Checks {
		if r.ID == "module-state-is-caller-owned" && r.Status != "unchecked" {
			t.Fatal(r)
		}
	}
}
func TestJevClassificationOnceThenOnlySelectedChecks(t *testing.T) {
	inv, c := fixture(t, map[string]string{"main.tf": `variable "name" { type = string }`})
	calls := 0
	eval := func(_ context.Context, req jev.Request) (jev.Response, error) {
		calls++
		if _, ok := req.Questions["classification"]; !ok || len(req.Questions) != 1 {
			t.Fatalf("%+v", req)
		}
		return jev.Response{Model: "test", Answers: map[string]jev.Answer{"classification": {Choice: "module", Confidence: 0.9, Probabilities: map[string]float64{"module": 0.95}}}}, nil
	}
	got, err := Run(context.Background(), inv, c, "jev", "test", "auto", eval)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || got.CodebaseKind != "module" {
		t.Fatalf("calls=%d result=%+v", calls, got)
	}
	found := false
	for _, r := range got.Units[0].Checks {
		if r.ID == "input-descriptions" {
			found = true
			if r.Status != "fail" {
				t.Fatal(r)
			}
		}
	}
	if !found {
		t.Fatal("module rule missing")
	}
	c.Classification.MinConfidence = 0.99
	got, err = Run(context.Background(), inv, c, "jev", "test", "auto", eval)
	if err != nil {
		t.Fatal(err)
	}
	if got.CodebaseKind != "unknown" {
		t.Fatal("accepted uncertain classification")
	}
}
func TestConfigControlsCheckPolicyAndOwnership(t *testing.T) {
	inv, c := fixture(t, map[string]string{"main.tf": `variable "name" { description = "Name" }`, "demo/main.tf": `terraform {
 backend "local" {}
}`})
	c.Structure.ExampleOwners = map[string]string{"demo": "."}
	c.Classification.Overrides = map[string]string{".": "module"}
	c.Checks = []config.Check{{ID: "company-rule", Engine: "go", AppliesTo: []string{"module"}, Severity: "error", Block: "variable", Attribute: "sensitive", Assert: "all_have_attribute", Message: "Company-specific input rule"}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	got, err := Run(context.Background(), inv, c, "offline", "test", "auto", never(t))
	if err != nil {
		t.Fatal(err)
	}
	if got.CodebaseKind != "module" || got.ExampleCount != 1 || len(got.Units[0].Checks) != 1 || got.Units[0].Checks[0].Status != "fail" {
		t.Fatalf("%+v", got)
	}
}
func TestExpressionDescriptionsRemainUnknown(t *testing.T) {
	inv, c := fixture(t, map[string]string{"main.tf": `variable "name" { description = local.description }`})
	got, err := Run(context.Background(), inv, c, "offline", "test", "module", never(t))
	if err != nil {
		t.Fatal(err)
	}
	if got.Units[0].Checks[0].Status != "unchecked" {
		t.Fatal(got)
	}
}
func TestConflictingSignalsAndInvalidOwner(t *testing.T) {
	inv, c := fixture(t, map[string]string{"main.tf": `terraform {
 backend "local" {}
}`})
	c.Classification.Signals = append(c.Classification.Signals, config.Signal{ID: "conflict", Kind: "module", Block: "terraform.backend"})
	got, err := Run(context.Background(), inv, c, "jev", "test", "auto", never(t))
	if err != nil {
		t.Fatal(err)
	}
	if got.Units[0].Classification.Method != "conflict" {
		t.Fatal(got)
	}
	c.Structure.ExampleOwners = map[string]string{"missing": "."}
	if _, err := Run(context.Background(), inv, c, "offline", "test", "auto", nil); err == nil {
		t.Fatal("accepted invalid owner")
	}
}
func TestConfiguredJevCheckOnlyAfterClassification(t *testing.T) {
	inv, c := fixture(t, map[string]string{"main.tf": `output "id" { value = "x" }`})
	enabled := true
	c.Checks[len(c.Checks)-1].Enabled = &enabled
	calls := 0
	eval := func(_ context.Context, req jev.Request) (jev.Response, error) {
		calls++
		key, value := "classification", "module"
		if calls == 2 {
			key, value = "module-purpose", "pass"
		}
		if len(req.Questions) != 1 || req.Questions[key].Type != "choice" {
			t.Fatal(req.Questions)
		}
		return jev.Response{Model: "test", Answers: map[string]jev.Answer{key: {Choice: value, Confidence: 0.95, Probabilities: map[string]float64{value: 0.95}}}}, nil
	}
	got, err := Run(context.Background(), inv, c, "jev", "test", "auto", eval)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || got.Units[0].Checks[len(got.Units[0].Checks)-1].Status != "pass" {
		t.Fatal(got)
	}
}
