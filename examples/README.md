# Repository archetype examples

Each example describes a separate synthetic Terraform Git repository. All fixture
inputs and their independent full rulesets are tracked here; none are customer code.
Do not run terraform apply: cloud credentials/provisioning are not part of testing.

| Example | Independent components | Supporting code / policy |
|---|---|---|
| reusable-module | One inferred reusable module | A deployment example owned by the module |
| single-deployment | One deployment with an explicit backend | Live environment-purpose assessment |
| module-library | Two inferred reusable modules | A shared examples tree explicitly owned by storage |
| multi-deployment | Dev and prod entry points | Independent deployment checks |
| mixed-repository | Reusable module and deployment | Module example excluded from component counts |
| custom-policy | Explicit reusable module | Deliberate company-nullability failure |

```text
examples/<archetype>/
  source/         # contents of the hypothetical repository
  rules.yaml      # independent collection, ownership and check configuration
  expected.json   # reviewed structural/status assertions, offline and live
  results/
    offline/      # generated and committed on the PR branch
    jev/          # generated and committed on the PR branch
      report.json
      report.yaml
      run.json
      summary.md
```

From the iaclens root:

```sh
go build -o work/iaclens ./cmd/iaclens
go run ./cmd/exampletest --example reusable-module --mode offline --bin work/iaclens
# Export LLM_GATEWAY_API_KEY to run the real Jev calls:
go run ./cmd/exampletest --example reusable-module --mode jev --bin work/iaclens
```

The runner copies source/ into a temporary directory, initializes a separate Git
repository there, and invokes the real CLI with the example's rules.yaml. This is
necessary because pointing the CLI directly at examples/foo/source would otherwise
analyse the enclosing iaclens repository. No nested .git directories are committed.

The CLI is invoked once per mode. JSON is the canonical run report; the runner
serializes the same data as YAML, without issuing another model call. Only the
machine-specific root path is normalized to source for portability. Source and
ruleset hashes, actual model answers, probabilities, token usage, tested revision,
workflow SHA/run/attempt, elapsed time, and assertion outcome remain available.

Each PR exposes one Examples gate check. It builds once, validates all six
examples offline, then runs advisory live assessments sequentially. Offline
failures block merging; live failures, timeouts, unavailable credentials, and
skipped fork/Dependabot live runs do not. Every live invocation has a three-minute
limit. CI does not provision infrastructure.

All reports are written under each example's results/<mode>/ directory. One
14-day artifact contains the complete directory structure, linked from the workflow
summary. Extract it into examples/ to review all JSON/YAML reports locally.
Expected policy failures (such as custom-policy) remain valid test outcomes.

The API client retries an explicit HTTP 503 at most twice (three total attempts),
with one- and two-second delays. It does not retry model answers, expectation
mismatches, authentication failures, or ambiguous transport errors. Reported Jev
calls count completed evaluations; temporary failed HTTP attempts are not separate
model decisions. An exhausted API failure is advisory and retains run
metadata; consult the job log when no complete report could be written.

For same-repository PRs, Actions commits the generated reports directly to the
feature branch after the run, including failure metadata. Open examples/<name>/results/
in the PR Files changed tab or branch browser. A separate publishing job has write
permissions and never executes PR code. It refuses to push if the branch advanced.
The bot dispatches CI and offline verification on the generated commit; this second
run does not call Jev or publish again. Fork/Dependabot runs retain artifacts only.
