package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRepositoryRootFromNestedDirectory(t *testing.T) {
	root := t.TempDir()
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "init", "-q", real).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, output)
	}
	nested := filepath.Join(real, "modules", "example")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	got, err := repositoryRoot(nested)
	if err != nil {
		t.Fatal(err)
	}
	if got != real {
		t.Fatalf("got %s want %s", got, real)
	}
}
func TestRepositoryRootRejectsNonRepository(t *testing.T) {
	if _, err := repositoryRoot(t.TempDir()); err == nil {
		t.Fatal("expected repository boundary error")
	}
}
