package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const (
	repositoryName = "Ledermayer/iaclens"
	defaultBranch  = "main"
	pageSize       = 100
	maxPages       = 10
	maxSourceBytes = 1 << 20
	commandTimeout = 45 * time.Second
)

const repositoryAPI = "repos/" + repositoryName

var workflowPath = regexp.MustCompile(`^\.github/workflows/[A-Za-z0-9_.-]+\.yml$`)
var requiredJobs = []string{"Test (ubuntu-latest)", "Test (macos-latest)", "Test (windows-latest)", "Verify release packaging", "Selected examples", "Validation gate"}

// apiClient makes the mutation boundary replaceable in tests.
type apiClient interface {
	request(string, string, any, any) error
}

// githubAPI uses the runner's scoped token without exposing it to a shell or PR code.
type githubAPI struct{ readOnly bool }

func (client githubAPI) request(method, endpoint string, payload, result any) error {
	if client.readOnly && method != "GET" {
		return fmt.Errorf("dry-run cannot mutate GitHub")
	}
	arguments := []string{"api", "--hostname", "github.com", endpoint, "--method", method}
	var input []byte
	if payload != nil {
		var err error
		input, err = json.Marshal(payload)
		if err != nil {
			return err
		}
		arguments = append(arguments, "--input", "-")
	}
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, "gh", arguments...)
	command.Stdin = bytes.NewReader(input)
	output, err := command.Output()
	if err != nil {
		return fmt.Errorf("GitHub %s %s failed: %w", method, endpoint, err)
	}
	if result != nil {
		return json.Unmarshal(output, result)
	}
	return nil
}

// repository and reference retain API identity separately from display names.
type repository struct {
	ID            int64
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
}

type reference struct {
	SHA, Ref string
	Repo     repository
}

// pullRequest includes completeness counts and GitHub's native mergeability decision.
type pullRequest struct {
	Number         int
	User           user
	State          string
	Draft          bool
	Commits        int
	ChangedFiles   int `json:"changed_files"`
	Head, Base     reference
	Mergeable      *bool
	MergeableState string `json:"mergeable_state"`
}

type runInfo struct {
	ID                              int64
	Actor                           user
	HeadSHA                         string     `json:"head_sha"`
	HeadBranch                      string     `json:"head_branch"`
	HeadRepository                  repository `json:"head_repository"`
	Path, Event, Status, Conclusion string
}

// listAll fails closed if the bounded pagination cannot establish a complete list.
func listAll[item any](client apiClient, endpoint string) ([]item, error) {
	var all []item
	separator := "?"
	if strings.Contains(endpoint, "?") {
		separator = "&"
	}
	for page := 1; page <= maxPages; page++ {
		var items []item
		path := fmt.Sprintf("%s%sper_page=%d&page=%d", endpoint, separator, pageSize, page)
		if err := client.request("GET", path, nil, &items); err != nil {
			return nil, err
		}
		all = append(all, items...)
		if len(items) < pageSize {
			return all, nil
		}
	}
	return nil, fmt.Errorf("pagination limit reached; no automatic decision")
}

// source reads a bounded blob at an immutable revision without creating a worktree.
func source(client apiClient, filename, sha string) ([]byte, error) {
	var blob struct {
		Type, Encoding, Content string
		Size                    int
	}
	endpoint := repositoryAPI + "/contents/" + url.PathEscape(filename) + "?ref=" + url.QueryEscape(sha)
	if err := client.request("GET", endpoint, nil, &blob); err != nil {
		return nil, err
	}
	if blob.Type != "file" || blob.Encoding != "base64" || blob.Size > maxSourceBytes {
		return nil, fmt.Errorf("unsupported or oversized dependency file")
	}
	data, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(blob.Content, "\n", ""))
	if err != nil || len(data) != blob.Size {
		return nil, fmt.Errorf("dependency file decoding or size mismatch")
	}
	return data, nil
}

// filesFor verifies complete, modified-only file evidence before reading any content.
func filesFor(client apiClient, pr pullRequest) ([]fileChange, error) {
	entries, err := listAll[struct{ Filename, Status string }](client, fmt.Sprintf("repos/%s/pulls/%d/files", repositoryName, pr.Number))
	if err != nil {
		return nil, err
	}
	if len(entries) != pr.ChangedFiles || len(entries) == 0 {
		return nil, fmt.Errorf("incomplete changed-file list")
	}
	var files []fileChange
	for _, entry := range entries {
		if entry.Status != "modified" || (entry.Filename != "go.mod" && entry.Filename != "go.sum" && !workflowPath.MatchString(entry.Filename)) {
			return nil, fmt.Errorf("unexpected dependency-update file")
		}
		before, err := source(client, entry.Filename, pr.Base.SHA)
		if err != nil {
			return nil, err
		}
		after, err := source(client, entry.Filename, pr.Head.SHA)
		if err != nil {
			return nil, err
		}
		files = append(files, fileChange{Name: entry.Filename, Status: entry.Status, Before: before, After: after})
	}
	return files, nil
}

// reviewsFor resolves present-day write permissions, not author association labels.
func reviewsFor(client apiClient, number int) ([]review, map[string]bool, error) {
	reviews, err := listAll[review](client, fmt.Sprintf("repos/%s/pulls/%d/reviews", repositoryName, number))
	if err != nil {
		return nil, nil, err
	}
	writers := map[string]bool{}
	for _, item := range reviews {
		if item.User.Type != "User" {
			continue
		}
		if _, known := writers[item.User.Login]; known {
			continue
		}
		var permission struct{ Permission string }
		endpoint := repositoryAPI + "/collaborators/" + url.PathEscape(item.User.Login) + "/permission"
		if err := client.request("GET", endpoint, nil, &permission); err != nil {
			return nil, nil, err
		}
		writers[item.User.Login] = permission.Permission == "admin" || permission.Permission == "maintain" || permission.Permission == "write"
	}
	return reviews, writers, nil
}

// latestCI accepts only this repository's CI workflow at the exact requested head.
func latestCI(client apiClient, repo repository, sha, branch string) (*runInfo, error) {
	var result struct {
		Runs []runInfo `json:"workflow_runs"`
	}
	endpoint := repositoryAPI + "/actions/workflows/ci.yml/runs?per_page=100&head_sha=" + sha
	if err := client.request("GET", endpoint, nil, &result); err != nil {
		return nil, err
	}
	for _, run := range result.Runs {
		if run.HeadSHA == sha && run.HeadBranch == branch && run.HeadRepository.ID == repo.ID && run.Path == ".github/workflows/ci.yml" {
			return &run, nil
		}
	}
	return nil, nil
}

// fullCIPassed requires actual successful jobs, not a run containing only skipped work.
func fullCIPassed(client apiClient, run *runInfo) (bool, error) {
	if run == nil || run.Status != "completed" || run.Conclusion != "success" {
		return false, nil
	}
	var result struct {
		Total int `json:"total_count"`
		Jobs  []struct{ Name, Conclusion string }
	}
	if err := client.request("GET", fmt.Sprintf("repos/%s/actions/runs/%d/jobs?per_page=100", repositoryName, run.ID), nil, &result); err != nil {
		return false, err
	}
	if result.Total != len(result.Jobs) {
		return false, fmt.Errorf("incomplete CI job evidence")
	}
	passed := map[string]bool{}
	for _, job := range result.Jobs {
		passed[job.Name] = job.Conclusion == "success"
	}
	for _, name := range requiredJobs {
		if !passed[name] {
			return false, nil
		}
	}
	return true, nil
}

// evaluate joins immutable provenance, strict diff checks and current approval state.
func evaluate(client apiClient, repo repository, pr pullRequest) (bool, error) {
	if pr.User.Login != botLogin || pr.User.ID != botID || pr.User.Type != "Bot" || pr.Head.Repo.ID != repo.ID || pr.Base.Repo.ID != repo.ID || pr.Base.Ref != defaultBranch || pr.State != "open" || pr.Draft || pr.Commits != 1 {
		return false, fmt.Errorf("not a supported single-commit Dependabot PR")
	}
	commits, err := listAll[commitInfo](client, fmt.Sprintf("repos/%s/pulls/%d/commits", repositoryName, pr.Number))
	if err != nil {
		return false, err
	}
	if len(commits) != 1 || commits[0].SHA != pr.Head.SHA {
		return false, fmt.Errorf("commit evidence does not match the current head")
	}
	updates, err := signedUpdates(commits[0])
	if err != nil {
		return false, err
	}
	files, err := filesFor(client, pr)
	if err != nil {
		return false, err
	}
	major, err := dependencyChanges(files, updates)
	if err != nil {
		return false, err
	}
	reviews, writers, err := reviewsFor(client, pr.Number)
	if err != nil {
		return false, err
	}
	approved, blocked := approval(reviews, pr.Head.SHA, writers)
	if blocked || (major && !approved) {
		return false, fmt.Errorf("current-head human approval required or changes requested")
	}
	run, err := latestCI(client, repo, pr.Head.SHA, pr.Head.Ref)
	if err != nil {
		return false, err
	}
	if run == nil || run.Event != "pull_request" || run.Actor.Login != botLogin || run.Actor.ID != botID || run.Actor.Type != "Bot" {
		return false, fmt.Errorf("current-head PR CI must originate from Dependabot")
	}
	return fullCIPassed(client, run)
}

// dispatchMain explicitly covers bot-token merge events without creating a release.
func dispatchMain(client apiClient) error {
	return client.request("POST", repositoryAPI+"/actions/workflows/ci.yml/dispatches", map[string]string{"ref": defaultBranch}, nil)
}

// mergeCurrent rechecks authorization and asks GitHub to enforce native protection.
func mergeCurrent(client apiClient, repo repository, expected pullRequest, validatedMain string) error {
	var branch struct{ Commit struct{ SHA string } }
	if err := client.request("GET", repositoryAPI+"/branches/main", nil, &branch); err != nil {
		return err
	}
	if branch.Commit.SHA != validatedMain || expected.Base.SHA != validatedMain {
		return fmt.Errorf("main advanced after validation; wait for its CI")
	}
	run, err := latestCI(client, repo, validatedMain, defaultBranch)
	if err != nil {
		return err
	}
	passed, err := fullCIPassed(client, run)
	if err != nil {
		return err
	}
	if !passed {
		return fmt.Errorf("current main CI is no longer successful")
	}
	var current pullRequest
	endpoint := fmt.Sprintf("repos/%s/pulls/%d", repositoryName, expected.Number)
	if err := client.request("GET", endpoint, nil, &current); err != nil {
		return err
	}
	if current.Head.SHA != expected.Head.SHA || current.Base.SHA != expected.Base.SHA || current.Mergeable == nil || !*current.Mergeable || current.MergeableState != "clean" {
		return fmt.Errorf("head/base changed or native merge requirements are not satisfied")
	}
	eligible, err := evaluate(client, repo, current)
	if err != nil {
		return err
	}
	if !eligible {
		return fmt.Errorf("CI is no longer successful")
	}
	var result struct {
		Merged bool
		SHA    string
	}
	if err := client.request("PUT", endpoint+"/merge", map[string]string{"sha": current.Head.SHA, "merge_method": "squash"}, &result); err != nil {
		return err
	}
	if !result.Merged {
		return fmt.Errorf("GitHub refused protected merge")
	}
	fmt.Printf("Merged PR #%d at %s; requesting main CI.\n", current.Number, result.SHA)
	return dispatchMain(client)
}

// reconcile processes at most one merge; the schedule recovers missed events/dispatches.
func reconcile(client apiClient, apply bool) error {
	var repo repository
	if err := client.request("GET", repositoryAPI, nil, &repo); err != nil {
		return err
	}
	if repo.FullName != repositoryName || repo.DefaultBranch != defaultBranch {
		return fmt.Errorf("unexpected repository identity or default branch")
	}
	var branch struct {
		Protected bool
		Commit    struct{ SHA string }
	}
	if err := client.request("GET", repositoryAPI+"/branches/main", nil, &branch); err != nil {
		return err
	}
	if !branch.Protected {
		return fmt.Errorf("main must remain protected")
	}
	run, err := latestCI(client, repo, branch.Commit.SHA, defaultBranch)
	if err != nil {
		return err
	}
	passed, err := fullCIPassed(client, run)
	if err != nil {
		return err
	}
	if !passed {
		fmt.Println("Holding merges until current main has successful full CI.")
		if run == nil && apply {
			return dispatchMain(client)
		}
		return nil
	}
	prs, err := listAll[pullRequest](client, repositoryAPI+"/pulls?state=open&base=main&sort=created&direction=asc")
	if err != nil {
		return err
	}
	for _, candidate := range prs {
		if candidate.User.Login != botLogin {
			continue
		}
		var pr pullRequest
		if err := client.request("GET", fmt.Sprintf("repos/%s/pulls/%d", repositoryName, candidate.Number), nil, &pr); err != nil {
			return err
		}
		eligible, err := evaluate(client, repo, pr)
		if err != nil || !eligible {
			fmt.Printf("PR #%d held: %v (eligible=%v)\n", pr.Number, err, eligible)
			continue
		}
		fmt.Printf("PR #%d eligible at %s.\n", pr.Number, pr.Head.SHA)
		if apply {
			return mergeCurrent(client, repo, pr, branch.Commit.SHA)
		}
	}
	return nil
}

func main() {
	apply := flag.Bool("apply", false, "Merge eligible PRs; only allowed in the main-branch Actions controller")
	flag.Parse()
	if *apply && (os.Getenv("GITHUB_ACTIONS") != "true" || os.Getenv("GITHUB_REPOSITORY") != repositoryName || os.Getenv("GITHUB_REF") != "refs/heads/main") {
		fmt.Fprintln(os.Stderr, "apply requires the trusted main-branch workflow")
		os.Exit(1)
	}
	if err := reconcile(githubAPI{readOnly: !*apply}, *apply); err != nil {
		fmt.Fprintln(os.Stderr, "dependabotmerge:", err)
		os.Exit(1)
	}
}
