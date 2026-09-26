# Cortisol

- [Go CLI](cli/README.md): wraps Codex and renders externally supplied status values.
- [Dashboard](dashboard/README.md): browser dashboard with a mock snapshot contract.
- [Server](server/): Go HTTP service and job queue.
- [Project decisions and metric definitions](CONTEXT.md).

Run the CLI demo with Go 1.27 or later:

```sh
cd cli
go run ./cmd/cortisol --demo
```

Launch Codex with a live status file supplied by your metric function:

```sh
go run ./cmd/cortisol --metrics status.json -- resume
```
