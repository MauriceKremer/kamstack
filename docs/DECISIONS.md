# Decisions

Every decision has an id, the outcome, and why. Record new ones here before or
alongside the change, not after; a rejected option that someone is likely to
propose again is worth a line too.

| Id | Decision | Why |
|---|---|---|
| D1 | `kamstack` is the only place that knows KAM and KAMmon as a pair. KAM's code and docs never mention KAMmon. The three binaries are `kam`, `kammon` and `kamstack`. | The pair is a distribution concern. Keeping it in one binary keeps both projects independent and honest. |
| D2 | One version number for the pair (`v0.7.0`); each component is tagged with the same number in its own repo. | A matched pair is the only supported combination, so one number says what works together. |
| D3 | systemd owns the services. `kamstack` writes unit files and calls `systemctl`; it never supervises a process, tails logs for storage, or rotates them (`journalctl` does that). | One supervisor, no half-reimplemented process management, logs where the platform already puts them. |
| D4 | Secrets are plain text in the YAML configs, generated at setup and printed once. No secrets store, no environment plumbing. | Matches KAM and KAMmon's own stance; a hobbyist node does not need a vault. |
| D5 | Release integrity is SHA-256 checksums only. No signing. | Corruption and a truncated download are the realistic failures, not a targeted attacker rewriting a hobby project's binaries. |
| D6 | No auto-update timer. `kamstack update` runs only when a person runs it. | Rollback has to prove itself on real hardware first (plan Phase 6). |
| D7 | Backups copy the database and `rooms/` while the services are stopped; there is no SQLite dependency and no `sqlite3` CLI on the Pi. | Copying a stopped WAL database is the documented safe method and adds no dependency. |
| D8 | A fresh install serves KAMmon on `:8080` over the LAN with generated passwords. Anyone exposing it beyond the LAN puts TLS in front (a reverse proxy or KAMmon's own `tls_cert`/`tls_key`). | Easy setup first; TLS is the operator's call when they cross the LAN boundary. |
| D9 | The module lives in the `kamstack/` subdirectory of the working tree (nested as `kamstack/kamstack` next to `kam/` and `kammon/`). | Keeps the three related modules together in one working copy; the nesting is accepted. |
| D10 | `manifest.json` names both components with their version and a per-target `url`/`sha256`; target keys are `linux/arm64`, `linux/armv7`, `linux/amd64`. | The updater needs one document that says what to download and how to verify it, and the keys match the archive names the component workflows publish. |
| D11 | TLS posture: a fresh install serves KAMmon on `:8080` over the LAN with generated passwords; KAM's API stays on loopback. Exposing KAMmon beyond the LAN needs TLS in front — a reverse proxy or KAMmon's `tls_cert`/`tls_key`. `kamstack` manages no certificates, redirects or HSTS. | Matches KAMmon D31 and keeps setup to one command; transport security is the operator's decision at the LAN boundary, where they can choose the right service. |
| D12 | Releases publish stable `kamstack_linux_<arch>` aliases and `install.sh` beside the versioned archives and `SHA256SUMS`. | `releases/latest/download/install.sh` must bootstrap without first discovering the release version, and the alias still has a checksum to verify. |
