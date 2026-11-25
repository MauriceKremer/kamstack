# kamstack

kamstack installs, controls and updates a matched pair of [KAM](../kam) and
[KAMmon](../kammon) on a Raspberry Pi. It is the only place where the two are
described as one pair.

The three binaries:

| Binary | What it is |
|---|---|
| `kam` | The MeshCore node: radio, database, rooms, Starlark automations, loopback REST API. |
| `kammon` | The web monitor that reads KAM's API. |
| `kamstack` | Setup, service control, `doctor` and the checked updater. It never supervises a process; systemd does. |

## Install

You need Raspberry Pi OS (or another systemd Linux) and a MeshCore-compatible
radio plugged in over USB. Run as root:

```sh
curl -fsSL https://github.com/MauriceKremer/kamstack/releases/latest/download/install.sh | sudo sh
```

The script picks the right architecture, verifies the SHA-256 of the download,
installs `kamstack` in `/opt/kamstack/bin`, runs `kamstack setup`, then installs
the pinned `kam` and `kammon` from the release manifest. It asks for the node
name and prints the two generated KAMmon passwords **once** — write them down.

With the radio plugged in, finish the first run:

```sh
sudo kamstack usb       # pick the radio; writes its by-id path into kam.yml
sudo kamstack doctor    # one readable report: versions, paths, units, HTTP, disk, journal
```

Open `http://<pi-address>:8080` and sign in as `admin` or `viewer`.

### Install by hand

```sh
# download kamstack_linux_<arch> from a release, or build it:
go build -o kamstack .

sudo install -d -m 0755 /opt/kamstack/bin
sudo install -m 0755 kamstack /opt/kamstack/bin/kamstack
sudo kamstack setup      # writes configs and units, asks for the node name
sudo kamstack update     # installs the pinned kam and kammon
```

## Everyday use

```sh
sudo kamstack status            # units, KAM API, KAMmon, modem link
sudo kamstack start|stop|restart
sudo kamstack logs -f           # both units through journalctl
sudo kamstack usb               # re-pick the radio after replugging it
sudo kamstack doctor            # the bug-report artifact
```

## Update and roll back

```sh
sudo kamstack update --check    # installed -> available; exit 0 current, 2 outdated
sudo kamstack update            # verified, backed up, swapped, health-checked
```

An update stops both units, copies `kam.db`, its WAL files and `rooms/` into
`/var/lib/kamstack/backups/`, keeps the newest three, swaps the binaries (the old
ones stay as `kam.prev` and `kammon.prev`), starts the pair and checks
`/v1/status` and KAMmon's `/` for 10 seconds. If that check fails, it puts the
`.prev` binaries back and starts the old version again.

To roll back by hand after a successful update:

```sh
sudo systemctl stop kamstack-kam kamstack-kammon
sudo mv /opt/kamstack/bin/kam.prev /opt/kamstack/bin/kam
sudo mv /opt/kamstack/bin/kammon.prev /opt/kamstack/bin/kammon
sudo systemctl start kamstack-kam kamstack-kammon
```

## Where things live

| Path | What |
|---|---|
| `/opt/kamstack/bin` | `kam`, `kammon`, `kamstack` and the `.prev` copies |
| `/etc/kamstack/kam.yml` | KAM's config (node name, radio port, channels, rooms) |
| `/etc/kamstack/kammon.yml` | KAMmon's config (passwords, users, listen address) |
| `/var/lib/kamstack/kam.db` | KAM's SQLite database (WAL) |
| `/var/lib/kamstack/rooms/` | Room identity keys — back these up |
| `/var/lib/kamstack/backups/` | Update backups |
| `systemd` units | `kamstack-kam.service`, `kamstack-kammon.service` |

Do not delete `kam.db` to fix a problem. `kamstack doctor` names a database from
an older version (`no such column` in the journal) instead of guessing: restore a
backup or start from an empty data directory deliberately.

### Nodes installed before web-user editing

Older `kamstack` units ran KAMmon under `ProtectSystem=strict` without a
`ReadWritePaths`, so the service could read but not rewrite `kammon.yml`. Adding
or editing a web user then failed. `kamstack doctor` flags this, and new installs
ship the fixed unit. To fix an existing node:

```sh
sudo mkdir -p /etc/systemd/system/kamstack-kammon.service.d
printf [Service]
ReadWritePaths=/etc/kamstack
 | sudo tee /etc/systemd/system/kamstack-kammon.service.d/50-readwrite.conf
sudo systemctl daemon-reload && sudo systemctl restart kamstack-kammon
```

## Security and TLS

A fresh install serves KAMmon on `:8080` over the LAN with the generated
passwords. That is the intended home setup: KAM's own API stays on loopback
(`127.0.0.1:8766`), and KAMmon has no transport restrictions of its own.

If you expose KAMmon beyond your LAN, put TLS in front: a reverse proxy with
HTTPS, or KAMmon's own `tls_cert`/`tls_key` settings. Secrets are plain text in
the YAML files by design; `kamstack` generates them once and never stores a
second copy.

## Releasing (maintainers)

There are two independent deploys.

### Manuals and other docs

`kam` and `kammon` both have GitHub Pages enabled on `main` → `/docs`. Merge a
docs or manual change to `main` and Pages rebuilds; no tag and no Pi update is
involved. Release archives contain only the binary, the default config,
`LICENSE` and `README.md`, so manuals never ship inside a binary release.

### A binary release

On a checkout with `kam/`, `kammon/` and `kamstack/` as siblings, run:

```sh
./scripts/release.sh          # asks for the version
./scripts/release.sh 0.8.0    # or pass it
```

It checks that each repo is clean on `main`, tags `kam` and `kammon`, waits for
their releases, updates the pins in `stack.yml`, and then tags `kamstack`. The
steps below are what the script performs by hand.

A release is three tags and one update on each Pi. Order matters: both component
releases must exist before `kamstack` is tagged, because the manifest step
verifies them and fails loudly otherwise. Use one version number for the pair.

1. **KAM** — merge to `main`, let CI pass, then tag and push:

   ```sh
   git tag -a v0.8.0 -m "kamstack v0.8.0"
   git push origin v0.8.0
   ```

   The workflow verifies the build, cross-compiles arm64/armv7/amd64, and
   publishes `kam_<version>_linux_<arch>.tar.gz` plus `SHA256SUMS`.

2. **KAMmon** — the same tag number, the same steps. Its release notes also
   name the KAM API contract commit (`docs/kam-api/KAM_COMMIT`).

3. **kamstack** — update `stack.yml` to pin the new tags and refresh
   `contract_commit` from `kammon/docs/kam-api/KAM_COMMIT`, merge that change to
   `main`, then tag the same number:

   ```sh
   git tag -a v0.8.0 -m "kamstack v0.8.0"
   git push origin v0.8.0
   ```

   The workflow verifies both component releases and their checksums, then
   publishes the `kamstack` archives, the stable `kamstack_linux_<arch>` assets
   and `install.sh` used by the bootstrap script, `SHA256SUMS`, and
   `manifest.json`.

4. **Deploy** — on each Pi:

   ```sh
   sudo kamstack update --check   # exit 0 current, 2 outdated
   sudo kamstack update           # backup, stop, swap, start, health check, rollback
   ```

   A fresh machine uses the install script from the new release instead.

Docs-only changes do not need tags or a Pi update.

## Development

```sh
go build ./...
go test -race ./...
go vet ./... && gofmt -l .
```

The distribution plan and requirement ids (`DST-n`) live in
[docs/PLAN.md](docs/PLAN.md); decisions are in [docs/DECISIONS.md](docs/DECISIONS.md).
