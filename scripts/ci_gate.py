#!/usr/bin/env python3
"""Fail closed on missing, failed or unexpectedly skipped required jobs."""
import os


def validate(mode, plan, tests, package, examples):
    if plan != 'success':
        raise ValueError('Change selection did not succeed')
    if mode == 'full':
        valid = tests == package == examples == 'success'
    elif mode == 'examples':
        valid = tests == package == 'skipped' and examples == 'success'
    elif mode in {'results', 'docs'}:
        valid = tests == package == examples == 'skipped'
    else:
        valid = False
    if not valid:
        raise ValueError(f'Required validation incomplete: {mode}, test={tests}, package={package}, examples={examples}')


if __name__ == '__main__':
    validate(*(os.environ[key] for key in ['MODE', 'PLAN', 'TEST', 'PACKAGE', 'EXAMPLES']))
    with open(os.environ['GITHUB_STEP_SUMMARY'], 'a') as f:
        f.write(f"Validated route: **{os.environ['MODE']}**. Live assessments are advisory.\n")
