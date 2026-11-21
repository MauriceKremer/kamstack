# AGENTS.md

`kamstack` sets up, controls and updates a KAM + KAMmon node on a Raspberry Pi.
It is a small CLI, not a daemon: it writes unit files, calls `systemctl` and
`journalctl`, discovers the radio, and performs checked updates. It is the only
place where KAM and KAMmon are described as one pair. See [docs/PLAN.md](docs/PLAN.md).

## Philosophy

For enthusiasts, like KAM and KAMmon: pragmatism and easy setup before strict
design and hardening.

- Choose the simplest thing that works. One binary, plain flags, no new dependency beyond `yaml.v3`.
- Write docs for beginners, in plain words. Honest labels: say what a command actually does.
- Keep cheap correctness safeguards: verify checksums, never delete user data, roll back a failed update.

## Rules

1. **The pair exists only here.** KAM's code and docs never mention KAMmon. Everything about running both together lives in this module.
2. **systemd owns the services.** Never supervise a process, collect or rotate logs. `kamstack` starts, stops and reports; `journalctl` keeps the logs.
3. **Never destroy data.** Update never touches `/etc/kamstack/*` or the database contents, and never deletes `kam.db`. Back up while stopped, swap, health-check, roll back on failure.
4. **By-id serial paths only.** Write `/dev/serial/by-id/...` into `kam.yml`, never `/dev/ttyUSB*`; the kernel renumbers those.
5. **Checksums only, no signing.** And no auto-update timer until it is asked for.

## The sibling modules

KAM and KAMmon are independent Go modules checked out beside this one as `../kam`
and `../kammon`. `kamstack` relies on:

| Path | What |
|---|---|
| `../kam` | The node: builds as a static binary; rewrites `kam.yml` at runtime; serves a loopback REST API (`monitor.listen`, default `127.0.0.1:8766`). |
| `../kammon` | The web monitor: reads KAM's API; serves `:8080`; supports `tls_cert`/`tls_key`. |
| `docs/PLAN.md` | The distribution plan; the requirement ids (`DST-n`) come from there. |

## Commands

```sh
go build ./...
go test -race ./...
go vet ./... && gofmt -l .
```

## Layout

| Path | Purpose |
|---|---|
| `main.go` | Entry point and subcommand dispatch |
| `app.go` | Install paths, command runner and HTTP probes (injectable for tests) |
| `config.go` | Embedded Pi defaults and YAML edits (`node_name`, `db_path`, serial port, passwords) |
| `layout.go` | Directory/user layout and binary copying |
| `units.go` | The two systemd unit files |
| `setup.go` | `setup`: layout, configs, units, enable/start |
| `usb.go` | `usb`: by-id discovery and driver table |
| `services.go`, `logs.go` | systemctl/status and journalctl wrappers |
| `doctor.go` | The bug-report report and its assembly |
| `defaults/` | Bundled `kam.yml` and `kammon.yml`, embedded with `go:embed` |
| `docs/DECISIONS.md` | Decision log (id, outcome, why) |
