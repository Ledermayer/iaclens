package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// botCommit supplies synthetic API-verified metadata, never real credentials.
func botCommit() commitInfo {
	var commit commitInfo
	commit.SHA = "head"
	commit.Author = user{Login: botLogin, ID: botID, Type: "Bot"}
	commit.Commit.Verification.Verified = true
	commit.Commit.Verification.Reason = "valid"
	commit.Commit.Message = "build(deps): bump example\n\n---\nupdated-dependencies:\n- dependency-name: example.com/lib\n  dependency-version: 1.2.1\n  dependency-type: direct:production\n  update-type: version-update:semver-patch\n...\nSigned-off-by: dependabot[bot]"
	return commit
}

func TestSignedUpdates(t *testing.T) {
	updates, err := signedUpdates(botCommit())
	if err != nil || len(updates) != 1 || updates[0].Kind != patchUpdate {
		t.Fatalf("updates=%v err=%v", updates, err)
	}
	for _, change := range []func(*commitInfo){
		func(commit *commitInfo) { commit.Author.ID++ },
		func(commit *commitInfo) { commit.Author.Type = "User" },
		func(commit *commitInfo) { commit.Commit.Verification.Verified = false },
		func(commit *commitInfo) { commit.Commit.Message = "no signed metadata" },
		func(commit *commitInfo) { commit.Commit.Message += "\n---\nupdated-dependencies: []" },
	} {
		commit := botCommit()
		change(&commit)
		if _, err := signedUpdates(commit); err == nil {
			t.Fatal("untrusted metadata accepted")
		}
	}
}

func TestGroupedMetadataChecksEveryDependency(t *testing.T) {
	commit := botCommit()
	commit.Commit.Message = strings.Replace(commit.Commit.Message, "\n...\n", "\n- dependency-name: example.com/other\n  dependency-version: 2.0.0\n  update-type: version-update:semver-major\n...\n", 1)
	updates, err := signedUpdates(commit)
	if err != nil || len(updates) != 2 || updates[1].Kind != majorUpdate {
		t.Fatalf("group metadata lost a member: %v %v", updates, err)
	}
	commit.Commit.Message = strings.Replace(commit.Commit.Message, majorUpdate, "unrecognized", 1)
	if _, err := signedUpdates(commit); err == nil {
		t.Fatal("unknown member of group was accepted")
	}
}

func TestHeadSpecificApproval(t *testing.T) {
	maintainer := user{Login: "maintainer", Type: "User"}
	writers := map[string]bool{"maintainer": true}
	cases := []struct {
		name     string
		reviews  []review
		approved bool
		blocked  bool
	}{
		{"none", nil, false, false},
		{"current", []review{{ID: 1, User: maintainer, State: "APPROVED", CommitID: "head"}}, true, false},
		{"stale", []review{{ID: 1, User: maintainer, State: "APPROVED", CommitID: "old"}}, false, false},
		{"dismissed", []review{{ID: 1, User: maintainer, State: "APPROVED", CommitID: "head"}, {ID: 2, User: maintainer, State: "DISMISSED", CommitID: "head"}}, false, false},
		{"changes", []review{{ID: 2, User: maintainer, State: "CHANGES_REQUESTED", CommitID: "head"}, {ID: 1, User: maintainer, State: "APPROVED", CommitID: "head"}}, false, true},
		{"comment", []review{{ID: 1, User: maintainer, State: "APPROVED", CommitID: "head"}, {ID: 2, User: maintainer, State: "COMMENTED", CommitID: "head"}}, true, false},
		{"outsider", []review{{ID: 1, User: user{Login: "outsider", Type: "User"}, State: "APPROVED", CommitID: "head"}}, false, false},
		{"bot", []review{{ID: 1, User: user{Login: "maintainer", Type: "Bot"}, State: "APPROVED", CommitID: "head"}}, false, false},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			approved, blocked := approval(item.reviews, "head", writers)
			if approved != item.approved || blocked != item.blocked {
				t.Fatalf("got approved=%v blocked=%v", approved, blocked)
			}
		})
	}
}

func TestDependencyOnlyChanges(t *testing.T) {
	oldSHA, newSHA := strings.Repeat("a", 40), strings.Repeat("b", 40)
	workflow := "name: CI\non: pull_request\njobs:\n  test:\n    steps:\n      - uses: actions/checkout@" + oldSHA + " # v4\n      - run: go test ./...\n"
	good := fileChange{Name: ".github/workflows/ci.yml", Status: "modified", Before: []byte(workflow), After: []byte(strings.ReplaceAll(workflow, oldSHA, newSHA))}
	items := []update{{Name: "actions/checkout", Version: "5.0.0", Kind: majorUpdate}}
	major, err := dependencyChanges([]fileChange{good}, items)
	if err != nil || !major {
		t.Fatalf("valid major action update rejected: %v", err)
	}
	for _, mutate := range []func(*fileChange){
		func(change *fileChange) {
			change.After = []byte(strings.ReplaceAll(string(change.After), "go test ./...", "echo bypass"))
		},
		func(change *fileChange) {
			change.After = []byte(strings.ReplaceAll(string(change.After), "actions/checkout@", "unknown/checkout@"))
		},
		func(change *fileChange) {
			change.After = []byte(strings.ReplaceAll(string(change.After), newSHA, "v5"))
		},
		func(change *fileChange) { change.Name = "README.md" },
		func(change *fileChange) { change.Status = "added" },
		func(change *fileChange) { change.After = append(change.After, []byte("---\nother: document\n")...) },
	} {
		bad := good
		mutate(&bad)
		if _, err := dependencyChanges([]fileChange{bad}, items); err == nil {
			t.Fatal("unexpected workflow change accepted")
		}
	}
}

func TestGoModuleChanges(t *testing.T) {
	before := "module example.com/app\n\ngo 1.27.1\n\nrequire example.com/lib v1.2.0\n"
	good := fileChange{Name: "go.mod", Status: "modified", Before: []byte(before), After: []byte(strings.ReplaceAll(before, "v1.2.0", "v1.2.1"))}
	items := []update{{Name: "example.com/lib", Version: "1.2.1", Kind: patchUpdate}}
	if major, err := dependencyChanges([]fileChange{good}, items); err != nil || major {
		t.Fatalf("valid Go patch rejected: %v", err)
	}
	for _, replacement := range []string{
		strings.ReplaceAll(string(good.After), "go 1.27.1", "go 1.28.0"),
		string(good.After) + "\nreplace example.com/lib => ../local\n",
		strings.ReplaceAll(string(good.After), "v1.2.1", "v1.2.1-rc.1"),
		strings.ReplaceAll(string(good.After), "example.com/lib", "example.com/new"),
	} {
		bad := good
		bad.After = []byte(replacement)
		if _, err := dependencyChanges([]fileChange{bad}, items); err == nil {
			t.Fatal("unexpected Go change accepted")
		}
	}
	sum := fileChange{Name: "go.sum", Status: "modified", Before: []byte("example.com/lib v1.2.0 h1:old\n"), After: []byte("example.com/lib v1.2.1 h1:new\n")}
	if _, err := dependencyChanges([]fileChange{sum, good}, items); err != nil {
		t.Fatal(err)
	}
	sum.After = append(sum.After, []byte("example.com/unknown v1.0.0 h1:unknown\n")...)
	if _, err := dependencyChanges([]fileChange{good, sum}, items); err == nil {
		t.Fatal("unrelated checksum accepted")
	}
}

// fakeAPI records every write while returning API-shaped, synthetic evidence.
type fakeAPI struct {
	responses  map[string]any
	writes     []string
	payloads   []any
	beforeRead func(string)
}

func (client *fakeAPI) request(method, endpoint string, payload, result any) error {
	if method != "GET" {
		client.writes = append(client.writes, method+" "+endpoint)
		client.payloads = append(client.payloads, payload)
	}
	if method == "GET" && client.beforeRead != nil {
		client.beforeRead(endpoint)
	}
	if result == nil {
		return nil
	}
	response, exists := client.responses[endpoint]
	if !exists {
		return fmt.Errorf("unexpected API request: %s", endpoint)
	}
	data, err := json.Marshal(response)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, result)
}

// controllerFixture models a green patch update and current green main.
func controllerFixture() (*fakeAPI, pullRequest) {
	root := "repos/" + repositoryName
	repo := repository{ID: 42, FullName: repositoryName, DefaultBranch: defaultBranch}
	mergeable := true
	pr := pullRequest{Number: 7, User: user{Login: botLogin, ID: botID, Type: "Bot"}, State: "open", Commits: 1, ChangedFiles: 1,
		Head: reference{SHA: "head", Ref: "dependabot/action", Repo: repo}, Base: reference{SHA: "base", Ref: defaultBranch, Repo: repo}, Mergeable: &mergeable, MergeableState: "clean"}
	commit := botCommit()
	commit.Commit.Message = strings.ReplaceAll(commit.Commit.Message, "example.com/lib", "actions/checkout")
	jobs := make([]map[string]string, 0, len(requiredJobs))
	for _, name := range requiredJobs {
		jobs = append(jobs, map[string]string{"name": name, "conclusion": "success"})
	}
	client := &fakeAPI{responses: map[string]any{
		root:                    repo,
		root + "/branches/main": map[string]any{"protected": true, "commit": map[string]string{"sha": "base"}},
		root + "/pulls?state=open&base=main&sort=created&direction=asc&per_page=100&page=1": []pullRequest{pr},
		root + "/pulls/7": pr,
		root + "/pulls/7/commits?per_page=100&page=1": []commitInfo{commit},
		root + "/pulls/7/reviews?per_page=100&page=1": []review{},
		root + "/pulls/7/files?per_page=100&page=1":   []map[string]string{{"filename": ".github/workflows/ci.yml", "status": "modified"}},
		root + "/actions/runs/1/jobs?per_page=100":    map[string]any{"total_count": len(jobs), "jobs": jobs},
		root + "/pulls/7/merge":                       map[string]any{"merged": true, "sha": "merged"},
	}}
	for _, ref := range []reference{pr.Base, pr.Head} {
		client.responses[root+"/actions/workflows/ci.yml/runs?per_page=100&head_sha="+ref.SHA] = map[string]any{"workflow_runs": []runInfo{{ID: 1, Actor: pr.User, HeadSHA: ref.SHA, HeadBranch: ref.Ref, HeadRepository: repo, Path: ".github/workflows/ci.yml", Event: "pull_request", Status: "completed", Conclusion: "success"}}}
		sha := strings.Repeat("a", 40)
		if ref.SHA == "head" {
			sha = strings.Repeat("b", 40)
		}
		data := []byte("jobs:\n  test:\n    steps:\n      - uses: actions/checkout@" + sha + "\n")
		client.responses[root+"/contents/"+url.PathEscape(".github/workflows/ci.yml")+"?ref="+ref.SHA] = map[string]any{"type": "file", "encoding": "base64", "size": len(data), "content": base64.StdEncoding.EncodeToString(data)}
	}
	return client, pr
}

func TestControllerDryRunAndProtectedMerge(t *testing.T) {
	client, _ := controllerFixture()
	if err := reconcile(client, false); err != nil || len(client.writes) != 0 {
		t.Fatalf("dry-run mutated GitHub: %v %v", client.writes, err)
	}
	if err := reconcile(client, true); err != nil {
		t.Fatal(err)
	}
	if len(client.writes) != 2 || !strings.HasSuffix(client.writes[0], "/pulls/7/merge") || !strings.HasSuffix(client.writes[1], "/ci.yml/dispatches") {
		t.Fatalf("unexpected mutation sequence: %v", client.writes)
	}
	merge := client.payloads[0].(map[string]string)
	if merge["sha"] != "head" || merge["merge_method"] != "squash" {
		t.Fatalf("merge is not commit-specific squash: %v", merge)
	}
}

func TestControllerRejectsHeadRace(t *testing.T) {
	client, pr := controllerFixture()
	reads := 0
	client.beforeRead = func(endpoint string) {
		if endpoint == "repos/"+repositoryName+"/pulls/7" {
			reads++
			if reads == 2 {
				pr.Head.SHA = "new-head"
				client.responses[endpoint] = pr
			}
		}
	}
	if err := reconcile(client, true); err == nil || len(client.writes) != 0 {
		t.Fatalf("head race was not rejected: %v %v", client.writes, err)
	}
}

func TestControllerHoldsUnsafeCandidates(t *testing.T) {
	for _, scenario := range []string{"major", "foreign", "failed-ci", "unprotected", "human-push", "base-advanced"} {
		t.Run(scenario, func(t *testing.T) {
			client, pr := controllerFixture()
			root := "repos/" + repositoryName
			switch scenario {
			case "major":
				commits := client.responses[root+"/pulls/7/commits?per_page=100&page=1"].([]commitInfo)
				commits[0].Commit.Message = strings.ReplaceAll(commits[0].Commit.Message, patchUpdate, majorUpdate)
			case "foreign":
				pr.Head.Repo.ID++
				client.responses[root+"/pulls/7"] = pr
			case "failed-ci":
				client.responses[root+"/actions/runs/1/jobs?per_page=100"] = map[string]any{"total_count": 0, "jobs": []any{}}
			case "unprotected":
				client.responses[root+"/branches/main"] = map[string]any{"protected": false, "commit": map[string]string{"sha": "base"}}
			case "human-push":
				response := client.responses[root+"/actions/workflows/ci.yml/runs?per_page=100&head_sha=head"].(map[string]any)
				runs := response["workflow_runs"].([]runInfo)
				runs[0].Actor = user{Login: "human", ID: 1, Type: "User"}
			case "base-advanced":
				pr.Base.SHA = "new-base"
				client.responses[root+"/pulls/7"] = pr
			}
			_ = reconcile(client, true)
			if len(client.writes) != 0 {
				t.Fatalf("unsafe candidate caused writes: %v", client.writes)
			}
		})
	}
}

func TestReadOnlyAPIRejectsWrites(t *testing.T) {
	if err := (githubAPI{readOnly: true}).request("PUT", "unused", nil, nil); err == nil {
		t.Fatal("read-only client accepted a mutation")
	}
}

func TestWorkflowTrustBoundaries(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/dependabot-auto-merge.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			If          string
			Permissions map[string]string
			Steps       []struct {
				Uses, Run string
				With      map[string]any
			}
		}
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	preview := workflow.Jobs["preview"]
	controller := workflow.Jobs["reconcile"]
	if preview.Permissions["contents"] != "read" || preview.Permissions["pull-requests"] != "read" || preview.Permissions["actions"] != "read" {
		t.Fatal("PR preview must remain read-only")
	}
	if !strings.Contains(controller.If, "github.ref == 'refs/heads/main'") || !strings.Contains(controller.If, "github.event_name != 'pull_request'") {
		t.Fatal("write job lacks the default-branch trust guard")
	}
	if controller.Steps[0].With["ref"] != "${{ github.workflow_sha }}" || controller.Steps[0].With["persist-credentials"] != false {
		t.Fatal("write job must check out only trusted workflow code without credentials")
	}
	if strings.Contains(string(data), "pull_request_target") || strings.Contains(string(data), "gh release") || strings.Contains(string(data), "git tag") {
		t.Fatal("dependency automation must not run PR target code or publish releases")
	}
}
