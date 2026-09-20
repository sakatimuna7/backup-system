# Recovery Guide

This guide covers restoring a server from a `backup-system` snapshot.

## Prerequisites

1. A working restic repository (local, SFTP, or S3-compatible)
2. The repository password (stored in `/etc/backup-system/password` or equivalent)
3. `restic` installed on the recovery machine
4. `backup-system` binary (or raw `restic` commands — see [Manual Recovery](#manual-recovery-without-backup-system))

## Recovery Checklist

Follow this checklist in order. Do NOT skip the sandbox restore.

### 1. Install restic and backup-system

```sh
sudo apt install restic          # or download from https://restic.net
sudo install -m 755 backup-system /usr/local/bin/backup-system
```

### 2. Place the password file

```sh
sudo install -d -m 700 /etc/backup-system
sudo install -m 600 /path/to/password /etc/backup-system/password
```

If the password is lost, the repository is unrecoverable. There is no recovery for a lost password.

### 3. Write a recovery config

```sh
sudo install -m 600 /dev/null /etc/backup-system/config.yml
sudo nano /etc/backup-system/config.yml
```

Minimal config for recovery (only repository + password needed):

```yaml
version: 3

repositories:
  - name: local
    url: "local:/var/backups/restic"
    password_file: "/etc/backup-system/password"
    required: true

backup:
  paths: ["/etc/nginx"]   # not critical during restore

retention:
  daily: 0
  weekly: 0
  monthly: 0
  prune: false

verify:
  after_backup: false
```

### 4. List available snapshots

```sh
sudo backup-system -config /etc/backup-system/config.yml snapshots
```

Note the snapshot ID or use `latest`.

### 5. Restore to a sandbox directory (NEVER restore directly to `/`)

```sh
sudo backup-system -config /etc/backup-system/config.yml \
  restore latest /tmp/server-restore
```

### 6. Inspect the restored data

```sh
find /tmp/server-restore -maxdepth 3 -type f | head -50
ls -la /tmp/server-restore/etc/nginx
cat /tmp/server-restore/etc/hostname
```

Verify that critical files exist and are readable.

### 7. Selectively restore to the live server

Only after sandbox verification, copy specific files or directories:

```sh
# Example: restore nginx config
sudo cp -a /tmp/server-restore/etc/nginx /etc/nginx
sudo systemctl reload nginx

# Example: restore app files
sudo rsync -aHAX /tmp/server-restore/home/deploy/apps/ /home/deploy/apps/
```

For a full server recovery (e.g., after total disk failure), restore into a
new empty directory on the fresh install:

```sh
sudo mkdir -p /mnt/recovery
# The target must be empty. After installing backup-system + restic + password:
sudo backup-system -config /etc/backup-system/config.yml \
  restore latest /mnt/recovery

sudo rsync -aHAX --info=progress2 /mnt/recovery/ /
```

### 8. Restore database dumps (if applicable)

If database dumps were included in the backup paths:

```sh
# PostgreSQL
sudo -u postgres psql -f /tmp/server-restore/var/backups/postgresql.sql postgres

# Redis (if RDB was backed up)
sudo cp /tmp/server-restore/var/lib/redis/dump.rdb /var/lib/redis/dump.rdb
sudo chown redis:redis /var/lib/redis/dump.rdb
sudo systemctl restart redis
```

### 9. Verify services

```sh
sudo systemctl status nginx
sudo systemctl status postgresql
sudo systemctl status docker
```

### 10. Clean up

```sh
rm -rf /tmp/server-restore
rm -rf /mnt/recovery
```

## Manual Recovery (without backup-system)

If `backup-system` is unavailable, use `restic` directly:

```sh
export RESTIC_REPOSITORY="sftp:backup@backup.example.com:/srv/restic/vps-01"
export RESTIC_PASSWORD_FILE=/etc/backup-system/password

restic snapshots
restic restore latest --target /tmp/server-restore
restic restore <snapshot-id> --target /tmp/server-restore
restic restore latest --target /tmp/server-restore --include /etc/nginx
```

## Common Issues

| Problem | Solution |
|---|---|
| `repository not found` | Check `repository.url` in config. Run `restic init` if repository is new. |
| `wrong password` | Verify password file content. No trailing newline issues. |
| `lock on repository` | Another process may be running. Use `restic unlock` cautiously. |
| Restore is slow | Use `--include` to restore only needed paths. |
| Snapshot is empty | Check `backup.paths` in config. Verify paths existed at backup time. |
| File permissions wrong | Use `restic restore --target ...` which preserves metadata. Do not use `cp` without `-a`. |

## Testing Recovery

A backup that has never been restored is untrusted. Test recovery periodically:

```sh
# Monthly recovery drill
sudo backup-system -config /etc/backup-system/config.yml \
  restore latest /tmp/recovery-drill

# Verify a critical file
test -f /tmp/recovery-drill/etc/hostname && echo "OK" || echo "FAIL"

rm -rf /tmp/recovery-drill
```
