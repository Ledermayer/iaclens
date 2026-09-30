package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/semver"
	"gopkg.in/yaml.v3"
)

// fileChange records both reviewed revisions; contents are parsed, never executed.
type fileChange struct {
	Name, Status  string
	Before, After []byte
}

// versionKind accepts only forward stable versions and determines the actual bump.
func versionKind(before, after string) (string, error) {
	before = "v" + strings.TrimPrefix(before, "v")
	after = "v" + strings.TrimPrefix(after, "v")
	if !semver.IsValid(before) || !semver.IsValid(after) || semver.Prerelease(before) != "" || semver.Prerelease(after) != "" || semver.Compare(before, after) >= 0 {
		return "", fmt.Errorf("update is not a forward stable version")
	}
	if semver.Major(before) != semver.Major(after) {
		return majorUpdate, nil
	}
	if semver.MajorMinor(before) != semver.MajorMinor(after) {
		return minorUpdate, nil
	}
	return patchUpdate, nil
}

// workflowNode rejects multi-document YAML and aliases before structural comparison.
func workflowNode(data []byte) (*yaml.Node, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("workflow must contain exactly one YAML document")
	}
	return &document, nil
}

// actionPin permits only an existing GitHub-owned action's full SHA to change.
func actionPin(before, after string, updates map[string]update) (string, error) {
	oldName, oldSHA, oldOK := strings.Cut(before, "@")
	newName, newSHA, newOK := strings.Cut(after, "@")
	item, known := updates[oldName]
	if !oldOK || !newOK || oldName != newName || !strings.HasPrefix(oldName, "actions/") || !known || item.Version == "" {
		return "", fmt.Errorf("unknown action or non-pin workflow change")
	}
	for _, sha := range []string{oldSHA, newSHA} {
		if decoded, err := hex.DecodeString(sha); err != nil || len(decoded) != 20 {
			return "", fmt.Errorf("action references must be full commit SHAs")
		}
	}
	version := "v" + strings.TrimPrefix(item.Version, "v")
	if !semver.IsValid(version) || semver.Prerelease(version) != "" {
		return "", fmt.Errorf("action release is not a stable version")
	}
	return oldName, nil
}

// compareWorkflow ignores presentation comments but rejects changes to execution policy.
func compareWorkflow(before, after *yaml.Node, key string, updates map[string]update, changed map[string]bool) error {
	if before.Kind == yaml.AliasNode || after.Kind == yaml.AliasNode || before.Kind != after.Kind || before.Tag != after.Tag || len(before.Content) != len(after.Content) {
		return fmt.Errorf("workflow structure changed")
	}
	if before.Value != after.Value {
		if key != "uses" || before.Kind != yaml.ScalarNode {
			return fmt.Errorf("workflow content other than action pins changed")
		}
		name, err := actionPin(before.Value, after.Value, updates)
		if err != nil {
			return err
		}
		changed[name] = true
	}
	for index := range before.Content {
		childKey := ""
		if before.Kind == yaml.MappingNode && index%2 == 1 {
			childKey = before.Content[index-1].Value
		}
		if err := compareWorkflow(before.Content[index], after.Content[index], childKey, updates, changed); err != nil {
			return err
		}
	}
	return nil
}

// moduleChanges requires the same dependencies and directives, changing versions only.
func moduleChanges(change fileChange, updates map[string]update, changed map[string]bool) (map[string]bool, error) {
	before, err := modfile.Parse("go.mod", change.Before, nil)
	if err != nil {
		return nil, err
	}
	after, err := modfile.Parse("go.mod", change.After, nil)
	if err != nil {
		return nil, err
	}
	if len(before.Require) != len(after.Require) {
		return nil, fmt.Errorf("Go dependencies were added or removed")
	}
	versions := map[string]string{}
	for _, requirement := range before.Require {
		versions[requirement.Mod.Path] = requirement.Mod.Version
	}
	allowedSums := map[string]bool{}
	for _, requirement := range after.Require {
		name, version := requirement.Mod.Path, requirement.Mod.Version
		previous, exists := versions[name]
		if !exists {
			return nil, fmt.Errorf("unknown Go dependency")
		}
		if previous == version {
			continue
		}
		item, known := updates[name]
		kind, err := versionKind(previous, version)
		if !known || err != nil || kind != item.Kind || strings.TrimPrefix(item.Version, "v") != strings.TrimPrefix(version, "v") {
			return nil, fmt.Errorf("Go dependency change does not match signed metadata")
		}
		changed[name] = true
		allowedSums[name+" "+previous] = true
		allowedSums[name+" "+version] = true
		if err := after.AddRequire(name, previous); err != nil {
			return nil, err
		}
	}
	original, err := before.Format()
	if err != nil {
		return nil, err
	}
	normalized, err := after.Format()
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(original, normalized) {
		return nil, fmt.Errorf("Go directives or non-version content changed")
	}
	return allowedSums, nil
}

// checkSums restricts checksum additions/removals to the versions actually updated.
func checkSums(change fileChange, allowed map[string]bool) error {
	counts := map[string]int{}
	for _, line := range strings.Split(strings.TrimSpace(string(change.Before)), "\n") {
		counts[line]++
	}
	for _, line := range strings.Split(strings.TrimSpace(string(change.After)), "\n") {
		counts[line]--
	}
	for line, difference := range counts {
		if difference == 0 || line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 || !allowed[fields[0]+" "+strings.TrimSuffix(fields[1], "/go.mod")] {
			return fmt.Errorf("unrelated Go checksum change")
		}
	}
	return nil
}

// dependencyChanges validates every group member and returns whether approval is needed.
func dependencyChanges(files []fileChange, items []update) (bool, error) {
	updates, changed := map[string]update{}, map[string]bool{}
	major, actions := false, false
	for _, item := range items {
		updates[item.Name] = item
		major = major || item.Kind == majorUpdate
		actions = actions || strings.HasPrefix(item.Name, "actions/")
	}
	allowedSums := map[string]bool{}
	for _, file := range files {
		if file.Status != "modified" {
			return false, fmt.Errorf("added, deleted or renamed files require manual review")
		}
		if actions {
			if !strings.HasPrefix(file.Name, ".github/workflows/") || !strings.HasSuffix(file.Name, ".yml") {
				return false, fmt.Errorf("unexpected file in Actions update")
			}
			before, err := workflowNode(file.Before)
			if err != nil {
				return false, err
			}
			after, err := workflowNode(file.After)
			if err != nil {
				return false, err
			}
			if err := compareWorkflow(before, after, "", updates, changed); err != nil {
				return false, err
			}
		} else if file.Name == "go.mod" {
			var err error
			allowedSums, err = moduleChanges(file, updates, changed)
			if err != nil {
				return false, err
			}
		} else if file.Name != "go.sum" {
			return false, fmt.Errorf("unexpected file in Go update")
		}
	}
	for _, file := range files {
		if file.Name == "go.sum" {
			if err := checkSums(file, allowedSums); err != nil {
				return false, err
			}
		}
	}
	if len(changed) == 0 || len(changed) != len(updates) {
		return false, fmt.Errorf("changed dependencies do not match all metadata entries")
	}
	return major, nil
}
