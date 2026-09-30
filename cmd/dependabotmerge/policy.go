// dependabotmerge evaluates dependency updates without executing pull request code.
package main

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	botLogin    = "dependabot[bot]"
	botID       = 49699333
	patchUpdate = "version-update:semver-patch"
	minorUpdate = "version-update:semver-minor"
	majorUpdate = "version-update:semver-major"
)

// user is the authenticated GitHub identity, not the commit's free-form author.
type user struct {
	Login string
	Type  string
	ID    int64
}

// commitInfo captures the API-verified signature and the signed commit message.
type commitInfo struct {
	SHA    string
	Author user
	Commit struct {
		Message      string
		Verification struct {
			Verified bool
			Reason   string
		}
	}
}

// update is the dependency record in Dependabot's signed YAML commit trailer.
type update struct {
	Name    string `yaml:"dependency-name"`
	Version string `yaml:"dependency-version"`
	Kind    string `yaml:"update-type"`
}

// review retains the head-specific decision needed for major update approval.
type review struct {
	ID       int64
	User     user
	State    string
	CommitID string `json:"commit_id"`
}

// signedUpdates rejects unverifiable identities, malformed trailers and unknown types.
func signedUpdates(commit commitInfo) ([]update, error) {
	if commit.Author.Login != botLogin || commit.Author.ID != botID || commit.Author.Type != "Bot" ||
		!commit.Commit.Verification.Verified || commit.Commit.Verification.Reason != "valid" {
		return nil, fmt.Errorf("current commit is not verified Dependabot source")
	}
	parts := strings.Split(commit.Commit.Message, "\n---\n")
	if len(parts) != 2 {
		return nil, fmt.Errorf("missing or ambiguous Dependabot metadata")
	}
	trailer, _, found := strings.Cut(parts[1], "\n...\n")
	if !found {
		return nil, fmt.Errorf("unterminated Dependabot metadata")
	}
	var metadata struct {
		Updates []update `yaml:"updated-dependencies"`
	}
	if err := yaml.Unmarshal([]byte(trailer), &metadata); err != nil {
		return nil, fmt.Errorf("invalid Dependabot metadata: %w", err)
	}
	if len(metadata.Updates) == 0 {
		return nil, fmt.Errorf("no dependency updates")
	}
	seen := map[string]bool{}
	for _, item := range metadata.Updates {
		if item.Name == "" || seen[item.Name] || (item.Kind != patchUpdate && item.Kind != minorUpdate && item.Kind != majorUpdate) {
			return nil, fmt.Errorf("unknown or duplicate dependency/update type")
		}
		seen[item.Name] = true
	}
	return metadata.Updates, nil
}

// approval returns the current-head approvers and any effective request for changes.
func approval(reviews []review, head string, writers map[string]bool) (bool, bool) {
	latest := map[string]review{}
	for _, item := range reviews {
		if item.User.Type != "User" || !writers[item.User.Login] || item.State == "COMMENTED" || item.State == "PENDING" {
			continue
		}
		if previous, ok := latest[item.User.Login]; !ok || item.ID > previous.ID {
			latest[item.User.Login] = item
		}
	}
	approved, blocked := false, false
	for _, item := range latest {
		blocked = blocked || item.State == "CHANGES_REQUESTED"
		approved = approved || (item.State == "APPROVED" && item.CommitID == head)
	}
	return approved, blocked
}
