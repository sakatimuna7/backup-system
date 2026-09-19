# backup-system

Small, auditable VPS backup agent. It keeps the user interface simple and delegates encrypted snapshots, deduplication, retention, verification, and restore to [restic](https://restic.net/).

## Requirements

- Linux
- Go 1.22+ to build
- `restic` in `PATH`
- A restic repository and password file

## Build

```sh
go build -o backup-system ./
```

## Configure

```sh
sudo install -d -m 700 /etc/backup-system
sudo cp config.example.yml /etc/backup-system/config.yml
sudo chmod 600 /etc/backup-system/config.yml
sudo install -m 600 /dev/null /etc/backup-system/password
# Put the repository password in /etc/backup-system/password.
```

Edit only `config.yml` for paths, excludes, repository, and retention. The password stays outside the YAML file so it cannot be committed accidentally.

## Commands

```sh
backup-system init
backup-system backup
backup-system snapshots
backup-system verify
backup-system retention --dry-run
backup-system retention --prune
backup-system restore latest /tmp/server-restore
```

Use another config with `-config /path/to/config.yml`. `backup` and `retention --prune` take an exclusive lock. Retention is dry-run by default unless `--prune` is explicit or `retention.prune: true` is configured for a normal `backup` run.

## MVP boundary

The agent intentionally does not implement its own storage format, encryption, deduplication, database dumping, scheduler, or notification system. Use pre-backup hooks or systemd around it for those concerns.

## Recovery

A full recovery checklist is available in [`RECOVERY.md`](RECOVERY.md). Always restore to a temporary target first, verify content, then selectively copy to live paths.

Quick recovery flow:

```sh
backup-system snapshots
backup-system restore latest /tmp/server-restore
```

The repository password is required; losing it makes the encrypted repository unusable.

## CI

This project includes a GitHub Actions workflow at `.github/workflows/ci.yml` that runs:

- `gofmt` check
- build
- unit tests
- end-to-end sandbox backup/restore test with real `restic`

