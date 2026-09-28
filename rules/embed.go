// Package rules carries the repo-local default ruleset into released binaries.
package rules

import _ "embed"

// Default is the same YAML users can copy and customize with --config.
//
//go:embed default.yaml
var Default []byte
