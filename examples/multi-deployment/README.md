# multi-deployment

Synthetic Terraform repository fixture for iaclens. Do not apply it to a cloud account.

- `source/`: repository contents, staged into a temporary Git worktree by the runner.
- `rules.yaml`: complete, independent ruleset for this archetype.
- `expected.json`: reviewed structural/check expectations for offline and live runs.
- `results/`: generated local reports, ignored by Git; CI uploads the same layout as artifacts.

Run from the iaclens checkout:

```sh
go build -o work/iaclens ./cmd/iaclens
go run ./cmd/exampletest --example multi-deployment --mode offline --bin work/iaclens
# With LLM_GATEWAY_API_KEY exported:
go run ./cmd/exampletest --example multi-deployment --mode jev --bin work/iaclens
```
