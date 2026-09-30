#!/usr/bin/env python3
"""Attach a completed lightweight verification to GitHub's PR merge SHA."""
import json
import os
import subprocess


def target_matches(pr, head, base, parents):
    return (pr['head']['sha'] == head and pr['base']['sha'] == base
            and len(parents) == 2 and set(parents) == {head, base})


def api(path, payload=None):
    command = ['gh', 'api', path]
    if payload is not None:
        command += ['--method', 'POST', '--input', '-']
    return json.loads(subprocess.check_output(command, input=json.dumps(payload) if payload is not None else None, text=True))


def main():
    repo, head, base = (os.environ[k] for k in ['GITHUB_REPOSITORY','VALIDATED_HEAD','VALIDATED_BASE'])
    # A pull_request run validates the pre-results head. Attesting that older
    # SHA to the current merge would hide the generated commit from the gate.
    if os.environ.get('GITHUB_EVENT_NAME') == 'pull_request':
        print('Pull-request validation does not attest the synthetic merge commit.')
        return
    prs = api(f'repos/{repo}/pulls?state=open&per_page=100')
    pr = next((p for p in prs if p['head']['sha'] == head and p['head']['repo']['full_name'] == repo), None)
    if pr is None:
        print('No current PR at this head; no merge check published.')
        return
    pr = api(f"repos/{repo}/pulls/{pr['number']}")
    merge = pr['merge_commit_sha']
    if not merge:
        raise ValueError('GitHub has not prepared a merge commit')
    commit = api(f'repos/{repo}/git/commits/{merge}')
    if not target_matches(pr, head, base, [p['sha'] for p in commit['parents']]):
        raise ValueError('PR head/base changed; refusing to attest a different merge')
    url = f"https://github.com/{repo}/actions/runs/{os.environ['GITHUB_RUN_ID']}"
    api(f'repos/{repo}/check-runs', {
        'name':'Validation gate', 'head_sha':merge, 'status':'completed', 'conclusion':'success',
        'details_url':url,
        'output':{'title':'Generated-results verification passed',
                  'summary':f'Validated allowed output paths and successful direct-parent evidence for head `{head}` against unchanged base `{base}`. No source changes require rebuilding. [Verification run]({url}).'}
    })
    print(f'Published verified result on PR merge commit {merge}')


if __name__ == '__main__': main()
