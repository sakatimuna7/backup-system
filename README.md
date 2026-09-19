# backup-system

Small, auditable VPS backup agent. It keeps the user interface simple and delegates encrypted snapshots, deduplication, retention, verification, and restore to [restic](https://restic.net/).

## Requirements

- Linux
- Go 1.22+ to build
- `restic` in `PATH`
- A restic repository and password file

## Install from a release

Download the binary for Linux and verify its checksum:

```sh
curl -LO https://github.com/sakatimuna7/backup-system/releases/download/v0.5.0/backup-system-linux-amd64
curl -LO https://github.com/sakatimuna7/backup-system/releases/download/v0.5.0/SHA256SUMS
sha256sum --check SHA256SUMS
sudo install -m 755 backup-system-linux-amd64 /usr/local/bin/backup-system
```

Use `backup-system-linux-arm64` on ARM64 hosts.

## Build

```sh
go build -o backup-system ./
```

## Configure

Quick start for first-time setup:

```sh
sudo install -m 755 backup-system /usr/local/bin/backup-system
sudo backup-system install
sudo nano /etc/backup-system/config.yml
sudo nano /etc/backup-system/password
```

`install` creates:

- `/etc/backup-system/config.yml` (template)
- `/etc/backup-system/password` (empty, mode `600`)

Manual setup (equivalent):

```sh
sudo install -d -m 700 /etc/backup-system
sudo cp config.example.yml /etc/backup-system/config.yml
sudo chmod 600 /etc/backup-system/config.yml
sudo install -m 600 /dev/null /etc/backup-system/password
# Put the repository password in /etc/backup-system/password.
```

Edit only `config.yml` for paths, excludes, repository, and retention. The password stays outside the YAML file so it cannot be committed accidentally.

## Help and commands

```sh
backup-system --help
backup-system backup --help
```

Mistyped commands return a close suggestion, for example `bakcup` suggests `backup`.

```sh
backup-system install
backup-system config-check
backup-system repositories
backup-system init
backup-system backup
backup-system backup --repository local
backup-system snapshots
backup-system verify --repository remote
backup-system restore --repository remote latest /tmp/server-restore
backup-system snapshots
backup-system verify
backup-system version
backup-system retention --dry-run
backup-system retention --prune
backup-system restore latest /tmp/server-restore
backup-system schedule render
backup-system schedule install
backup-system schedule status
backup-system recovery plan
backup-system recovery check
backup-system recovery run
```

For Google Drive or another rclone backend, configure OAuth with `rclone config`, copy its config to `/etc/backup-system/rclone.conf` with mode `600`, then set `url: "rclone:gdrive:backup-system/vps-01"` and `rclone_config` in the repository entry. The wrapper passes `RCLONE_CONFIG` only to that repository operation; OAuth tokens never belong in `config.yml`.

Use another config with `-config /path/to/config.yml`. Run `config-check` before `init` or `backup` to validate the YAML, password file, and restic installation. `backup` and `retention --prune` take an exclusive lock. Retention is dry-run by default unless `--prune` is explicit or `retention.prune: true` is configured for a normal `backup` run.

Common errors are actionable: a missing config points to `backup-system install`, a missing restic binary suggests `sudo apt install restic`, and restic failures include the exit code while preserving restic's detailed output.

## systemd (optional)

Verify the timer before installing it:

```bash
make verify-systemd
```

The timer runs daily at 02:00, survives missed runs with `Persistent=true`, and adds up to 15 minutes of jitter.

## Development

```bash
make test
make coverage
make verify-systemd
```

The CI workflow also runs race-enabled tests, statement coverage, the real restic sandbox E2E, and systemd timer verification.

Templates are provided in [`contrib/`](contrib):

- `contrib/backup-system.service`
- `contrib/backup-system.timer`

Install:

```sh
sudo cp contrib/backup-system.service /etc/systemd/system/
sudo cp contrib/backup-system.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now backup-system.timer
```

## MVP boundary

The agent intentionally does not implement its own storage format, encryption, deduplication, database dumping, scheduler, or notification system. Use pre-backup hooks or systemd around it for those concerns.

## Recovery

A full recovery checklist is available in [`RECOVERY.md`](RECOVERY.md). Always restore to a temporary target first, verify content, then selectively copy to live paths.

**Recovery workflow** (optional manifest for orchestrated recovery):

```sh
# 1. Preview the recovery plan (read-only, no changes)
backup-system recovery plan

# 2. Validate dependencies before staging restore
backup-system recovery check

# 3. Restore to staging directory (non-destructive)
backup-system recovery run
```

Quick manual recovery flow:

```sh
backup-system snapshots
backup-system restore latest /tmp/server-restore
```

The recovery manifest in `config.yml` under `recovery:` is optional and used only by these commands. `recovery plan` shows what would be restored; `recovery check` validates the staging path and repository; `recovery run` restores the snapshot to the staging directory without modifying the live system.

The repository password is required; losing it makes the encrypted repository unusable.

## CI

This project includes a GitHub Actions workflow at `.github/workflows/ci.yml` that runs:

- `gofmt` check
- build
- unit tests
- end-to-end sandbox backup/restore test with real `restic`

