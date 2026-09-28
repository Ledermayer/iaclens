#!/usr/bin/env python3
"""Route changes against a proven ancestor, never a commit-message assertion."""
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile

NAMES = ['reusable-module', 'single-deployment', 'module-library', 'multi-deployment', 'mixed-repository', 'custom-policy']
OUTPUTS = {'report.json', 'report.yaml', 'run.json', 'summary.md'}


def generated(path):
    parts = path.split('/')
    return (len(parts) == 5 and parts[0] == 'examples' and parts[1] in NAMES
            and parts[2] == 'results' and parts[3] in {'offline', 'jev'} and parts[4] in OUTPUTS)


def classify(paths, proven_parent=False, parent_examples=()):
    if paths and all(generated(p) for p in paths):
        owners = {p.split('/')[1] for p in paths}
        if not proven_parent or not owners.issubset(set(parent_examples)):
            raise ValueError('Results-only changes require a successfully validated direct parent and matching examples')
        return 'results', []
    selected = set()
    for p in paths:
        if generated(p):
            continue
        # Go and shared build/analysis inputs always take precedence over docs.
        if p.endswith('.go') or p in {'go.mod', 'go.sum'} or p.startswith(('rules/', 'scripts/', '.github/workflows/')):
            return 'full', NAMES.copy()
        if p.endswith('.md') or p.startswith('docs/') or p in {'.github/CODEOWNERS', '.github/dependabot.yml', '.gitignore', '.gitattributes', 'LICENSE'}:
            continue
        parts = p.split('/')
        if len(parts) >= 3 and parts[0] == 'examples' and parts[1] in NAMES:
            if parts[2] == 'source' or p.endswith(('/rules.yaml', '/expected.json')):
                selected.add(parts[1])
                continue
        # Unknown shared inputs are conservative; adding an archetype needs runner changes.
        return 'full', NAMES.copy()
    return ('examples', sorted(selected)) if selected else ('docs', [])


def run(*args):
    return subprocess.check_output(args, text=True).strip()


def api(path):
    return json.loads(run('gh', 'api', path))


def find_baseline(repo, head, base):
    ancestors = run('git', 'rev-list', '--first-parent', '--max-count=50', head + '^').splitlines()
    runs = api(f'repos/{repo}/actions/workflows/ci.yml/runs?per_page=100')['workflow_runs']
    for sha in ancestors:
        for candidate in runs:
            if candidate['head_sha'] != sha:
                continue
            jobs = api(f"repos/{repo}/actions/runs/{candidate['id']}/jobs?per_page=100")['jobs']
            if not any(j['name'] == 'Validation gate' and j['conclusion'] == 'success' for j in jobs):
                continue
            with tempfile.TemporaryDirectory() as temp:
                result = subprocess.run(['gh', 'run', 'download', str(candidate['id']), '--repo', repo,
                                         '--name', 'validation-plan', '--dir', temp], capture_output=True)
                if result.returncode:
                    continue
                evidence = json.loads(Path(temp, 'plan.json').read_text())
            if evidence.get('schema') == 1 and evidence.get('head') == sha and evidence.get('base') == base:
                return sha, evidence
    return None, None


def main():
    event = json.loads(Path(os.environ['GITHUB_EVENT_PATH']).read_text())
    event_name = os.environ['GITHUB_EVENT_NAME']
    repo = os.environ['GITHUB_REPOSITORY']
    head = run('git', 'rev-parse', 'HEAD')
    if event_name == 'pull_request':
        base = event['pull_request']['base']['sha']
    else:
        # Dispatch on a PR branch (including [skip ci] results commits) uses the
        # current PR base. Main pushes/manual runs deliberately get full validation.
        prs = api(f'repos/{repo}/pulls?state=open&per_page=100')
        pr = next((p for p in prs if p['head']['sha'] == head and p['head']['repo']['full_name'] == repo), None)
        base = pr['base']['sha'] if pr else head
    baseline, prior = (None, None) if base == head else find_baseline(repo, head, base)
    comparison = baseline or (run('git', 'merge-base', head, base) if base != head else None)
    paths = run('git', 'diff', '--name-only', '--no-renames', comparison, head).splitlines() if comparison else []
    parent = run('git', 'rev-parse', head + '^')
    mode, examples = classify(paths, baseline == parent, (prior or {}).get('examples', [])) if comparison else ('full', NAMES.copy())
    plan = dict(schema=1, head=head, base=base, baseline=baseline, mode=mode, examples=examples, changed=paths)
    Path('work').mkdir(exist_ok=True)
    Path('work/plan.json').write_text(json.dumps(plan, indent=2) + '\n')
    with open(os.environ['GITHUB_OUTPUT'], 'a') as output:
        output.write(f'mode={mode}\nexamples={json.dumps(examples)}\nhead={head}\n')
    with open(os.environ['GITHUB_STEP_SUMMARY'], 'a') as summary:
        summary.write(f'## Validation route: {mode}\n\nExamples: {", ".join(examples) or "none"}.\n\nCompared with `{comparison or "full checkout"}`.\n')
    print(json.dumps(plan, indent=2))


if __name__ == '__main__':
    main()
