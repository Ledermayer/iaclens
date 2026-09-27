package scan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseBoundariesAndExpressions(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"main.tf":               "# resource \"fake\" \"fake\" {}\nresource \"azurerm_resource_group\" \"this\" {\n name = \"brace } and \\n text\"\n location = var.location\n}\n",
		"examples/demo/main.tf": "module \"demo\" { source = \"../..\" }",
	} {
		path := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(path), 0755)
		os.WriteFile(path, []byte(content), 0600)
	}
	inv, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Modules) != 2 || len(inv.Modules[0].Symbols) != 1 {
		t.Fatalf("%+v", inv)
	}
	s := inv.Modules[0].Symbols[0]
	if s.Type != "azurerm_resource_group" || s.Attributes["location"] != "var.location" {
		t.Fatalf("%+v", s)
	}
}
func TestMalformedFails(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "main.tf"), []byte("resource {"), 0600)
	if _, err := Read(root); err == nil {
		t.Fatal("accepted malformed HCL")
	}
}
func TestPreserveAndReplace(t *testing.T) {
	existing := []byte("enrichment:\n  owner: team\n")
	first, err := Render(existing, map[string]int{"version": 1})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Render(first, map[string]int{"version": 2})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(second), string(existing)) || strings.Count(string(second), "BEGIN IACLENS") != 1 || strings.Contains(string(second), "version: 1") {
		t.Fatal(string(second))
	}
	if _, err := Render([]byte("# BEGIN IACLENS\n"), nil); err == nil {
		t.Fatal("accepted incomplete markers")
	}
}

func TestJSONAndProviderConstraints(t *testing.T) {
	root := t.TempDir()
	content := `{"terraform":{"required_version":">= 1.9","required_providers":{"azurerm":{"source":"hashicorp/azurerm","version":"~> 4.0"}}},"resource":{"azurerm_resource_group":{"this":{"name":"demo","location":"westeurope"}}}}`
	if err := os.WriteFile(filepath.Join(root, "main.tf.json"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	inv, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range inv.Modules[0].Symbols {
		if s.Kind == "terraform" && strings.Contains(s.Attributes["required_providers.azurerm"], "hashicorp/azurerm") {
			found = true
		}
	}
	if !found {
		t.Fatalf("provider constraint missing: %+v", inv)
	}
}
