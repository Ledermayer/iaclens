#!/usr/bin/env python3
"""Run only the routed examples; offline is required and live is advisory."""
import json
import os
from pathlib import Path
import shutil
import subprocess
from ci_plan import NAMES

selected = json.loads(os.environ['SELECTED'])
assert selected and all(name in NAMES for name in selected)
for name in selected:
    shutil.rmtree(Path('examples', name, 'results'), ignore_errors=True)
failed = False
for name in selected:
    result = subprocess.run(['./work/exampletest', '--example', name, '--mode', 'offline', '--bin', 'work/iaclens'])
    failed |= result.returncode != 0
if os.environ.get('LLM_GATEWAY_API_KEY'):
    for name in selected:
        try:
            result = subprocess.run(['./work/exampletest', '--example', name, '--mode', 'jev', '--bin', 'work/iaclens'], timeout=180)
            if result.returncode:
                print(f'::warning::Live assessment failed for {name}; advisory only.', flush=True)
        except subprocess.TimeoutExpired:
            print(f'::warning::Live assessment timed out for {name}; advisory only.', flush=True)
            # No stale success is retained: the runner cleared its files before execution.
            folder = Path('examples',name,'results','jev')
            folder.mkdir(parents=True,exist_ok=True)
            (folder/'run.json').write_text(json.dumps({'example':name,'mode':'jev','status':'failed','error':'Live assessment exceeded 180 seconds'})+'\n')
else:
    with open(os.environ['GITHUB_STEP_SUMMARY'],'a') as f:
        f.write('\nLive assessments skipped: trusted API credential unavailable.\n')
for name in selected:
    shutil.copytree(Path('examples',name,'results'), Path('work/example-results',name,'results'), dirs_exist_ok=True)
raise SystemExit(1 if failed else 0)
