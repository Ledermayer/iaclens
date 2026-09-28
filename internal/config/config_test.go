package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ledermayer/iaclens/rules"
)

func TestStrictConfig(t *testing.T) {
	for _, change := range []struct{ from, to string }{
		{"version: 1", "version: 99"},
		{"min_confidence: 0.8", "min_confidnce: 0.8"},
		{"assert: all_nonempty", "assert: execute_shell"},
		{"engine: go", "engine: both"},
		{"block: variable", "block: pipeline"},
		{"severity: warning", "severity: banana"},
	} {
		t.Run(change.to, func(t *testing.T) {
			f := filepath.Join(t.TempDir(), "rules.yaml")
			os.WriteFile(f, []byte(strings.Replace(string(rules.Default), change.from, change.to, 1)), 0600)
			if _, err := Load(f); err == nil {
				t.Fatal("accepted invalid config")
			}
		})
	}
}
func TestDefaultAndExternalConfigSame(t *testing.T) {
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(t.TempDir(), "rules.yaml")
	os.WriteFile(f, rules.Default, 0600)
	other, err := Load(f)
	if err != nil {
		t.Fatal(err)
	}
	if c.Digest != other.Digest {
		t.Fatal("default and external rules differ")
	}
}
