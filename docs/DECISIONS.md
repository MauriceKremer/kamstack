# Decisions

Every decision has an id, the outcome, and why. Record new ones here before or
alongside the change, not after; a rejected option that someone is likely to
propose again is worth a line too.

| Id | Decision | Why |
|---|---|---|
| D1 | `kamstack` is the only place that knows KAM and KAMmon as a pair. KAM's code and docs never mention KAMmon. | The pair is a distribution concern. Keeping it in one binary keeps both projects independent and honest. |
| D2 | One version number for the pair (`v0.7.0`); each component is tagged with the same number in its own repo. | A matched pair is the only supported combination, so one number says what works together. |
| D3 | systemd owns the services. `kamstack` writes unit files and calls `systemctl`; it never supervises a process, tails logs for storage, or rotates them (`journalctl` does that). | One supervisor, no half-reimplemented process management, logs where the platform already puts them. |
| D4 | Secrets are plain text in the YAML configs, generated at setup and printed once. No secrets store, no environment plumbing. | Matches KAM and KAMmon's own stance; a hobbyist node does not need a vault. |
| D5 | Release integrity is SHA-256 checksums only. No signing. | Corruption and a truncated download are the realistic failures, not a targeted attacker rewriting a hobby project's binaries. |
| D6 | No auto-update timer. `kamstack update` runs only when a person runs it. | Rollback has to prove itself on real hardware first (plan Phase 6). |
| D7 | Backups copy the database and `rooms/` while the services are stopped; there is no SQLite dependency and no `sqlite3` CLI on the Pi. | Copying a stopped WAL database is the documented safe method and adds no dependency. |
| D8 | A fresh install serves KAMmon on `:8080` over the LAN with generated passwords. Anyone exposing it beyond the LAN puts TLS in front (a reverse proxy or KAMmon's own `tls_cert`/`tls_key`). | Easy setup first; TLS is the operator's call when they cross the LAN boundary. |
| D9 | The module lives in the `kamstack/` subdirectory of the working tree (nested as `kamstack/kamstack` next to `kam/` and `kammon/`). | Keeps the three related modules together in one working copy; the nesting is accepted. |
