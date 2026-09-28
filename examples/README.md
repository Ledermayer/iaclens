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
    offline/      # generated and ignored
    jev/          # generated and ignored
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

Each PR runs six offline jobs followed by six live jobs (at most two simultaneous
live jobs). CI does not provision infrastructure or fetch provider release feeds.
Live validation requires actual Jev calls and expected semantic check statuses;
a skipped or offline result cannot pass the live gate. Expected failures such as
the custom-policy check are valid test outcomes, while unexpected failures or
uncertainty fail the suite and retain reports for inspection.

PR jobs upload results as 14-day artifacts and append job summaries. Artifact
names identify example, mode and attempt. After downloading one, place its files
under that example's results/<mode>/ directory for the same local layout. The
workflow does not commit reports, update expected.json, or post bot PR comments.

For a fork/Dependabot contribution, a maintainer must first review/promote it onto
a same-repository branch for a secret-bearing live run. See CONTRIBUTING.md.

The API client retries an explicit HTTP 503 at most twice (three total attempts),
with one- and two-second delays. It does not retry model answers, expectation
mismatches, authentication failures, or ambiguous transport errors. Reported Jev
calls count completed evaluations; temporary failed HTTP attempts are not separate
model decisions. An exhausted API failure blocks the PR gate and retains run
metadata; consult the job log when no complete report could be written.
