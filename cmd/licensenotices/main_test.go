package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFixture creates synthetic license inputs without relying on cached modules.
func writeFixture(t *testing.T, root, name, text string) {
	t.Helper()
	filename := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestDecodeDependencies(t *testing.T) {
	items, err := decodeDependencies(strings.NewReader(`{"ImportPath":"fmt","Standard":true} {"ImportPath":"example.com/lib","Module":{"Path":"example.com/lib","Version":"v1.0.0"}}`))
	if err != nil || len(items) != 2 || items[1].Module.Version != "v1.0.0" {
		t.Fatalf("items=%v error=%v", items, err)
	}
	if _, err := decodeDependencies(strings.NewReader(`{"ImportPath":`)); err == nil {
		t.Fatal("expected invalid JSON to fail")
	}
}

func TestCollectNoticesAndVersionedSources(t *testing.T) {
	root := t.TempDir()
	goroot, moduleRoot := filepath.Join(root, "go"), filepath.Join(root, "module")
	writeFixture(t, goroot, "LICENSE", "Go redistribution license")
	writeFixture(t, goroot, "src/vendor/example.com/lib/LICENSE", "Vendored license")
	writeFixture(t, goroot, "src/vendor/example.com/lib/lib.go", "package lib\n// Copyright fixture. Permission to redistribute.\nfunc Value() {}\n")
	writeFixture(t, goroot, "src/vendor/example.com/lib/copy.s", "// Copyright Lucent Technologies and Vita Nuova.\n// Permission is hereby granted.\n// Retain this entire notice.\n#include \"textflag.h\"\nTEXT copy(SB), $0\n")
	writeFixture(t, moduleRoot, "LICENSE.md", "Module license")
	writeFixture(t, moduleRoot, "NOTICE", "Module attribution")
	writeFixture(t, moduleRoot, "PATENTS", "Patent grant")
	writeFixture(t, moduleRoot, "lib.go", "// Copyright fixture author.\npackage lib\n")
	items := []dependency{
		{ImportPath: "example.com/Lib", Dir: moduleRoot, Module: &module{Path: "example.com/Lib", Version: "v1.2.3", Dir: moduleRoot}, GoFiles: []string{"lib.go"}},
		{ImportPath: "vendor/example.com/lib", Dir: filepath.Join(goroot, "src/vendor/example.com/lib"), Standard: true, GoFiles: []string{"lib.go"}, SFiles: []string{"copy.s"}},
	}
	notices, err := collect(items, goroot, "go1.27.1")
	if err != nil {
		t.Fatal(err)
	}
	data, err := render(notices)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Module license", "Module attribution", "Patent grant", "Vendored license", "Permission to redistribute", "Lucent Technologies and Vita Nuova", "Retain this entire notice", "https://proxy.golang.org/example.com/!lib/@v/v1.2.3.zip", "https://go.dev/dl/go1.27.1.src.tar.gz"} {
		if !strings.Contains(string(data), expected) {
			t.Errorf("missing %q", expected)
		}
	}
	if strings.Contains(string(data), root) {
		t.Fatal("notice bundle leaks local paths")
	}
	items[0], items[1] = items[1], items[0]
	notices, err = collect(append(items, items...), goroot, "go1.27.1")
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := render(notices)
	if err != nil || string(data) != string(repeated) {
		t.Fatalf("reordered/duplicate targets changed output: %v", err)
	}
}

func TestCollectRejectsMissingLicensesAndReplacements(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "LICENSE", "Go license")
	for _, selected := range []*module{nil, {Path: "example.com/lib"}, {Path: "example.com/lib", Version: "v1.0.0", Dir: t.TempDir()}, {Path: "example.com/lib", Version: "v1.0.0", Dir: root, Replace: &module{Dir: root}}} {
		if _, err := collect([]dependency{{ImportPath: "example.com/lib", Module: selected}}, root, "go1.27.1"); err == nil {
			t.Errorf("expected invalid dependency to fail: %+v", selected)
		}
	}
}

func TestGoLicenseInDistributionParent(t *testing.T) {
	root := t.TempDir()
	goroot := filepath.Join(root, "libexec")
	writeFixture(t, root, "LICENSE", "Go distribution license")
	notices, err := collect(nil, goroot, "go1.27.1")
	if err != nil || notices["Go go1.27.1"].Texts["LICENSE"] != "Go distribution license" {
		t.Fatalf("parent license was not retained: %v", err)
	}
	if _, err := collect(nil, filepath.Join(t.TempDir(), "missing"), "go1.27.1"); err == nil {
		t.Fatal("expected absent Go license to fail")
	}
}

func TestIncompleteApacheLicenseFails(t *testing.T) {
	notices := map[string]*notice{"example": {Source: "https://example.com/source", Texts: map[string]string{"LICENSE": "Apache License reference only"}}}
	if _, err := render(notices); err == nil {
		t.Fatal("expected incomplete Apache text to fail")
	}
}
