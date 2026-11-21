# Plan — painless setup and updating (KAM + KAMmon on a Raspberry Pi)

Goal: a Pi user gets a running, service-managed KAM + KAMmon from one command, and keeps it
current with one more. Artifacts live on GitHub Releases in the existing repositories; a small
third binary (`kamstack`) owns setup, USB discovery, service control, doctor and update.

Status legend: `[x]` done, `[ ]` open. Requirement ids are local to this plan (`DST-n`) so they
can be cited in commits; they are not new product requirements for KAM or KAMmon.

Principles this plan must not break (see [agent.md](../../agent.md)):

- KAM never mentions KAMmon. The pair only exists in `kamstack`, a separate module and repo.
- KAM and KAMmon stay standalone Go modules with one static binary each, and this plan adds no
  new dependencies to either. (`kam` already pulls sqlite/cron/serial/starlark; `kammon` is stdlib
  plus `yaml.v3`.) `kamstack` is stdlib plus `yaml.v3`, and nothing else.
- systemd runs the services. `kamstack` never supervises a process, collects logs or rotates them.
- Plain-text secrets in YAML, generated at setup and printed once. No secrets store, no env plumbing.
- Honest labels, plain words in docs, beginner-first.
- The smallest thing that works: no signing, no .deb, no SD image, no auto-update timer until asked.

---

## Phase 0 — Prerequisites (both repos)

Nothing can be downloaded until the binaries can say which version they are.

- [x] **DST-1** `kam`: add `internal/version` (or `main.version`, one variable) and a `version`
      subcommand that prints `kam <version> <goos>/<goarch>`; local builds default to `dev`.
      Implemented as `main.version` in `version.go`, so the release workflow's `-X main.version` works.
- [x] **DST-2** `kammon`: same variable, exposed as `-version` (kammon has no subcommands today).
      Implemented as `main.version` in `cmd/kammon/version.go`, same `-X main.version` flag.
- [x] **DST-3** Both repos: `-version`/`version` covered by a test that the value is non-empty,
      so a build without `-ldflags` is still coherent.
- [x] **DST-4** Both repos: confirm `LICENSE` and the default config (`kam.yml`, `kammon.yml`)
      are the files a user receives; no other files are packaged.
      KAMmon had no `LICENSE`; it now matches KAM's MIT license. The archive contents are fixed
      by DST-9/DST-13: binary, default config, `LICENSE`, `README.md` — nothing else.
- [x] **DST-5** Root `agent.md`: add the coming `kamstack` repo (the third binary) and a pointer
      to this plan.
- [x] **DST-6** Agree the stack version scheme: one number for the pair (`v0.7.0`), each component
      tagged with the same number in its own repo. Documented in `docs/DECISIONS.md` (D2).

**Done when:** `./kam version` and `./kammon -version` print a version on a normal local build and
a tagged build prints the tag.

---

## Phase 1 — Release artifacts for `kam`

- [x] **DST-7** `../kam/.github/workflows/release.yml`, triggered on `v*` tags:
      job `verify` (the existing CI steps: gofmt, vet, build, `go test -race`) → job `release`
      (`needs: verify`, `permissions: contents: write`).
      Implemented as `verify` → matrix `build` → `release` (`needs: build`).
- [x] **DST-8** Build matrix: `linux/arm64`, `linux/armv7` (`GOARM=7`, 32-bit Pi OS),
      `linux/amd64` (x86 testers, VPS). `CGO_ENABLED=0` (modernc SQLite is pure Go).
      `-ldflags "-s -w -X main.version=$TAG"`.
- [x] **DST-9** Package each target as `kam_<version>_linux_<arch>.tar.gz` containing `kam`,
      `kam.yml` (the Pi default config — radio port `/dev/ttyUSB0`, changed from the repo's macOS
      `/dev/cu.usbserial-*`), `LICENSE`, `README.md`.
- [x] **DST-10** Publish `SHA256SUMS` next to the archives; release notes via
      `gh release create --generate-notes`.
- [x] **DST-11** Smoke check in the workflow: unpack the `linux/amd64` archive and run
      `./kam version` — proves the artifact, not just the build.
      The tag keeps its `v` (`v0.7.0`); the binary version and archive names drop it (`0.7.0`).

**Done when:** pushing tag `v0.7.0` produces a release with 3 archives, `SHA256SUMS`, and the
amd64 archive runs `kam version` → `0.7.0`. Nothing else in KAM changes.

---

## Phase 2 — Release artifacts for `kammon`

Same shape, deliberately duplicated rather than shared: two repos do not get a reusable-workflow
dependency for ~40 lines of YAML.

- [x] **DST-12** `../kammon/.github/workflows/release.yml` mirroring DST-7..11,
      `kammon_<version>_linux_<arch>.tar.gz` containing `kammon`, `kammon.yml` (the default
      config), `LICENSE`, `README.md`. The amd64 smoke check runs `kammon -version`.
- [x] **DST-13** Release notes state the KAM version KAMmon is built against
      (`../kammon/docs/kam-api/KAM_COMMIT`) — that is the compatibility statement the matched pair relies on.
      Notes are generated with the API, then prefixed with `KAM @<short> (base commit <sha>)`.

**Done when:** tag `v0.7.0` in kammon produces the same artifact set and its notes name the KAM
contract commit.

---

## Phase 3 — `kamstack` skeleton and service layout

New module `kamstack.bytecoder.nl`, repo `kamstack`, one small binary (stdlib plus `yaml.v3`). Not a
daemon. Its own repository, next to `kam` and `kammon` in the working tree.

- [x] **DST-14** Repo scaffold: `go.mod` (Go 1.25), `main.go` with subcommand dispatch,
      `docs/DECISIONS.md`, the same `ci.yml` as the other two repos, `AGENTS.md`. Scaffolded in this
      repository.
- [x] **DST-15** Embed the default `kam.yml` and `kammon.yml` with `go:embed` (the same Pi-targeted
      defaults the tarballs ship — radio port `/dev/ttyUSB0`, not the repos' macOS dev config — so
      setup needs no network).
      Implemented in `defaults/` and read by `config.go`.
- [x] **DST-16** Layout constants and creation: `/opt/kamstack/bin`, `/etc/kamstack`,
      `/var/lib/kamstack`, `/var/lib/kamstack/rooms`; system user `kamstack` (no login shell),
      added to `dialout` when present.
      Implemented in `layout.go`; paths are `app` fields so tests use a temporary tree.
- [x] **DST-17** `kamstack setup`: create layout, write configs with `db_path` pointing into
      `/var/lib/kamstack` (room identities derive from it as `<dbdir>/rooms`; there is no separate
      `rooms` key), ask for the node name (default: hostname), generate the KAMmon `admin`/`viewer`
      passwords and print them once, install units, `systemctl enable --now`. Keep KAM's loopback
      API (`monitor.listen`, default `127.0.0.1:8766`) and KAMmon's `kam_monitor_url` pointing at
      it. Flags for scripted use: `--node-name`, `--port`, `--yes`.
      Also copies `kam`/`kammon` from beside `kamstack` when present; otherwise it enables the units
      and warns instead of starting a service with no binary.
- [x] **DST-18** `kamstack usb`: list `/dev/serial/by-id/*` with the driver behind each
      (`cp210x`, `ch341`, `cdc_acm`), mark candidates, prompt when there is more than one, and
      write the **by-id path** into `kam.yml` (never `ttyUSB0`); offer a `kam` restart.
      Implemented in `usb.go`; the table reads `/sys/class/tty/<tty>/device/driver`.
- [x] **DST-19** `kamstack start|stop|restart|status` — thin `systemctl` wrappers, exit codes passed
      through; `status` prints unit state, API reachability and modem link in one screen.
      Implemented in `services.go`; `main.run` returns a wrapped `*exec.ExitError`'s code.
- [x] **DST-20** `kamstack logs [-f] [-n N]` — `journalctl -u kamstack-kam -u kamstack-kammon`.
      Implemented in `logs.go`.
- [x] **DST-21** `kamstack version` — its own version plus the versions of the installed `kam` and
      `kammon`.
      Implemented in `main.go`; missing binaries are reported as not installed, not as an error.
- [x] **DST-22** `kamstack doctor` — versions, config paths, serial path exists and is readable,
      unit states, `/v1/status` and KAMmon `/` reachability, disk free, last 20 journal lines.
      This is the bug-report artifact.
      Implemented in `doctor.go`; `collectDoctor` gathers and `renderDoctor` formats.
- [x] **DST-23** Tests: unit parsing (`usb` table, config writing, doctor assembly) against fixtures;
      no test needs a Pi or a radio.
      Covered by `config_test.go`, `usb_test.go`, `doctor_test.go`, `setup_test.go`.

Unit files (both installed by `setup`, both `WantedBy=multi-user.target`):

```ini
# kamstack-kam.service
[Service]
User=kamstack
SupplementaryGroups=dialout
ExecStart=/opt/kamstack/bin/kam -config /etc/kamstack/kam.yml
Restart=on-failure
RestartSec=5
StateDirectory=kamstack
ProtectSystem=strict
ReadWritePaths=/etc/kamstack /var/lib/kamstack

# kamstack-kammon.service
[Service]
User=kamstack
ExecStart=/opt/kamstack/bin/kammon -config /etc/kamstack/kammon.yml
Restart=on-failure
RestartSec=5
ProtectSystem=strict
```

**Why `ReadWritePaths=/etc/kamstack`:** KAM rewrites `kam.yml` at runtime (`channel upsert`,
`PUT /v1/modem/radio`). The config directory must be writable by `kamstack:kamstack` (0660).
KAMmon writes nothing, so it runs fully locked down.

**Done when:** on a clean Pi, `./kamstack setup` + `./kamstack usb` leaves both services enabled,
`systemctl status` green, the KAMmon login reachable from the LAN, and `kamstack doctor` printing
a complete, readable report.

---

## Phase 4 — The matched pair and `kamstack update`

The updater is the only place that knows both components together.

- [ ] **DST-24** `stack.yml` in the `kamstack` repo pins the pair: `kam: v0.7.0`,
      `kammon: v0.7.0` (plus `contract_commit` for humans).
- [ ] **DST-25** `kamstack` release workflow (tag in the `kamstack` repo): read `stack.yml`, verify
      each pinned component release and its `SHA256SUMS` exist, and generate `manifest.json`
      (version, per-target URLs and checksums) attached to the `kamstack` release together with
      the `kamstack` binaries. Fail loudly if a pinned tag is missing.
- [ ] **DST-26** `kamstack update --check`: fetch the manifest, compare with installed versions,
      print `installed / available`, exit `0` when current and `2` when outdated (cron-friendly).
- [ ] **DST-27** `kamstack update`: download `kam` and `kammon` for this machine's arch to a temp
      dir, verify sha256, **stop both units**, back up state, swap binaries keeping `.prev`,
      start, health check, roll back on failure.
- [ ] **DST-28** Backup before swap, while the services are stopped: `kam.db`, `kam.db-wal`,
      `kam.db-shm` (those that exist) and `rooms/` into
      `/var/lib/kamstack/backups/<YYYY-MM-DD-HHMM>-<version>.tar.gz`, keeping the newest 3.
      Copying files while stopped is the README's own documented safe method — no SQLite
      dependency, and the Pi has no `sqlite3` CLI.
- [ ] **DST-29** Health check: `GET /v1/status` answers 200 and `GET /` on KAMmon answers (200/303)
      within 10 s of start; otherwise restore `.prev`, start again, report failure and leave the
      backup in place.
- [ ] **DST-30** Never touch `/etc/kamstack/*` or data during update, and never delete `kam.db` —
      a schema change is reported by `doctor` (`no such column` in the journal) as
      "database from an older version; restore the backup or reset the data" rather than guessed at.
- [ ] **DST-31** Tests for the update state machine with a fake `systemctl` and a local file server:
      current, outdated, checksum mismatch, health-check failure with rollback, backup pruning.

**Done when:** on the Pi, `kamstack update --check` correctly reports the installed pair, a real
`kamstack update` from `v0.7.0` to `v0.7.1` completes with services running, and a deliberately
broken binary triggers a rollback with the old version running again.

---

## Phase 5 — Bootstrap and documentation

- [ ] **DST-32** `install.sh` (in the `kamstack` repo, attached to its release):
      `curl -fsSL https://github.com/<org>/kamstack/releases/latest/download/install.sh | sh` —
      detect arch, download `kamstack` and `install.sh`'s checksum, install to
      `/opt/kamstack/bin`, then hand over to `sudo kamstack setup`. The script stays under ~40 lines.
- [ ] **DST-33** `README.md`: install, first run, USB discovery, update, rollback,
      backups, `doctor`, and where the configs live. Plain words, beginner-first.
- [ ] **DST-34** `../kam/README.md` and `../kammon/README.md`: a short "Install on a Raspberry Pi"
      section pointing at `kamstack`; the manual build instructions stay.
- [ ] **DST-35** `docs/DECISIONS.md` records: three binaries and where the pair lives,
      matched-pair versioning, stop-then-copy backups, sha256 only (no signing), no auto-update
      timer, systemd owns supervision, generated plain-text secrets, TLS posture below.
- [ ] **DST-36** TLS posture, written down, no code: a fresh install serves KAMmon on `:8080` over
      the LAN with generated passwords; anyone exposing it beyond the LAN puts TLS in front
      (reverse proxy, or KAMmon's existing `tls_cert`/`tls_key`). Consistent with KAMmon D31.

**Done when:** a Pi that has never seen the project goes from `curl | sh` to a login page in one
session, following only the README.

---

## Phase 6 — Validate on the real Pi

The deployment staged at `raspimaster.local:~/kamstack` becomes the first packaged install.

- [ ] **DST-37** Migrate: keep the existing `kam.db` and `rooms/lobby.key` (back them up first),
      install via `install.sh`, restore the data into `/var/lib/kamstack`, point `kam.yml` at the
      by-id serial path.
- [ ] **DST-38** Reboot test: services come back by themselves, KAM reopens the radio, KAMmon's
      pages fill in again.
- [ ] **DST-39** Unplug/replug the radio; confirm the by-id path still resolves and restarting
      `kam` alone reopens the port without touching `kammon`.
- [ ] **DST-40** Upgrade test on the Pi with a tagged release (DST-31 path, real hardware), then
      a rollback on purpose.
- [ ] **DST-41** Capture `kamstack doctor` output as the reference bug report and check the
      README's steps against a second, fresh machine if one is available.

**Done when:** DST-37..40 pass on the Pi and the doctor output needs no explanation to read.

---

## Deferred on purpose

Not in this plan; revisit when a real need shows up and record it in `docs/DECISIONS.md`.

- Auto-update timer (`kamstack update` on a schedule). Rollback must prove itself first.
- Pre-flashed SD image, `.deb`/apt repo, Windows/macOS packaging.
- Release signing (checksums only: corruption is the real risk, not a targeted attacker).
- Database migrations in KAM — the current policy stays (backup, then reset).
- A first-run wizard in the KAMmon web UI: setup stays a CLI step, and KAMmon must not manage KAM.
- Version display in the KAMmon UI and a `version` field in `GET /v1/status`
  (nice for `doctor`, not needed while `doctor` can run the binaries).
- Anything that phones home, registers a node, or collects metrics.

## Risks

| Risk | Mitigation |
|---|---|
| Schema change plus an updater equals silent data loss for testers | Backup before swap (DST-28), never delete data, `doctor` names the symptom (DST-30) |
| Mismatched pair (new KAMmon, old KAM contract) | One manifest, one version, both swapped in one update (DST-24..27) |
| `ttyUSB0` reordering makes the node deaf | by-id paths only (DST-18) |
| Unauthenticated KAMmon on the LAN | generated passwords, TLS decision recorded (DST-17, DST-36) |
| Two repos drifting apart in packaging | both workflows are copies by design; the manifest fails if a pinned tag is absent (DST-25) |
