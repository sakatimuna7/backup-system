# Configuration Schema Reference

Complete YAML schema for `/etc/backup-system/config.yml`.

## Root

```yaml
version: 3                 # Required: config version (currently 3)
repositories: [...]       # List of backup destinations
backup: {...}            # What to backup
retention: {...}         # Snapshot retention policy
verify: {...}            # Post-backup verification
schedule: {...}          # Systemd timer (optional)
recovery: {...}          # Recovery manifest (optional)
```

## Repositories

```yaml
repositories:
  - name: local                          # Required: unique identifier
    url: "local:/var/backups/restic"    # Required: restic URL
    password_file: "/etc/backup-system/password"  # Required
    rclone_config: "/etc/backup-system/rclone.conf"  # Optional (rclone only)
    required: true                       # Optional: fail if this repo fails
```

### Supported URL Schemes

- `local:/path/to/repo` — Local directory
- `sftp:user@host:/path` — SFTP
- `s3:s3.amazonaws.com/bucket/path` — S3-compatible
- `rclone:remote:path` — rclone backend

## Backup

```yaml
backup:
  paths:           # Required: list of absolute paths to backup
    - "/etc"
    - "/home"
    - "/var/www"
  exclude:         # Optional: paths to skip
    - "/proc"
    - "/sys"
    - "/tmp"
    - "/var/cache"
```

Paths must be absolute. Exclusions are patterns matched by restic.

## Retention

```yaml
retention:
  daily: 7         # Keep 7 daily snapshots (or 0 to disable)
  weekly: 4        # Keep 4 weekly snapshots
  monthly: 3       # Keep 3 monthly snapshots
  prune: false     # Optional: prune during backup (dry-run by default)
```

Run `backup-system retention --prune` to delete old snapshots.

## Verify

```yaml
verify:
  after_backup: true   # Optional: check repository after backup
```

## Schedule (Systemd Timer)

```yaml
schedule:
  enabled: true                # Optional: enable systemd timer
  on_calendar: "*-*-* 02:00:00"  # Systemd calendar (daily at 02:00)
  persistent: true             # Retry missed runs
  randomized_delay: "15m"      # Jitter to avoid thundering herd
```

Calendar format examples:
- `*-*-* 02:00:00` — Daily at 02:00
- `Mon *-*-* 03:00:00` — Mondays at 03:00
- `*-01-01 00:00:00` — January 1st at midnight

## Recovery (Optional)

Recovery manifest for orchestrated recovery. `recovery plan` is read-only.

### Packages

```yaml
recovery:
  packages:
    apt:              # Debian/Ubuntu packages
      - nginx
      - postgresql
      - redis-server
```

Installed during `recovery run` (apt-get install).

### Users

```yaml
recovery:
  users:
    - name: deploy              # Required: username
      shell: "/bin/bash"        # Required: login shell
      home: "/home/deploy"      # Required: absolute home path
      create_home: true         # Optional: create home directory
      groups: [sudo, docker]    # Optional: supplementary groups
```

Users are created during `recovery run` (useradd + usermod).

### Directories

```yaml
recovery:
  directories:
    - path: "/var/www"          # Required: absolute path
      owner: "www-data"         # Optional: username
      group: "www-data"         # Optional: group name
      mode: "0755"              # Optional: permissions
```

## Databases

```yaml
recovery:
  databases:
    - name: production          # Required: identifier
      engine: postgresql        # Required: "postgresql" or "mysql"
      database: myapp           # Required: database name
      owner: postgres           # Required: db user
      dump_path: "/var/backups/postgresql/myapp.sql"  # Required: absolute path
      restore: true             # Optional: restore during recovery run
```

Dumps must be included in `backup.paths` to be recovered.

## Downloads

```yaml
recovery:
  downloads:
    - name: custom-cli          # Required: identifier
      url: "https://example.com/binary"  # Required: HTTPS URL
      sha256: "0000...0000"      # Required: 64-char SHA-256
      install_to: "/usr/local/bin/custom-cli"  # Required: absolute path
      mode: "0755"              # Required: file permissions
```

## Restore

```yaml
recovery:
  restore:
    snapshot: latest            # Optional: snapshot ID or "latest"
    staging: "/recovery/staging"  # Required: absolute staging path
    include:                    # Optional: paths to restore
      - "/etc/nginx"
      - "/home/deploy/apps"
    exclude:                    # Optional: paths to skip
      - "/etc/shadow"
      - "/etc/ssh"
      - "/root/.ssh"
```

## Example: Full Config

```yaml
version: 3

repositories:
  - name: local
    url: "local:/var/backups/restic"
    password_file: "/etc/backup-system/password"
    required: true
  - name: gdrive
    url: "rclone:gdrive:backup/vps-01"
    password_file: "/etc/backup-system/password"
    rclone_config: "/etc/backup-system/rclone.conf"
    required: false

backup:
  paths:
    - "/etc"
    - "/home"
    - "/var/www"
  exclude:
    - "/tmp"
    - "/var/cache"

retention:
  daily: 7
  weekly: 4
  monthly: 3
  prune: false

verify:
  after_backup: true

schedule:
  enabled: true
  on_calendar: "*-*-* 02:00:00"
  persistent: true
  randomized_delay: "15m"

recovery:
  packages:
    apt:
      - nginx
      - postgresql
  users:
    - name: deploy
      shell: "/bin/bash"
      home: "/home/deploy"
      create_home: true
      groups: [sudo]
  restore:
    snapshot: latest
    staging: "/recovery/staging"
    include:
      - "/etc/nginx"
      - "/home/deploy"
    exclude:
      - "/etc/shadow"
```

## Validation

Run `backup-system config-check` to validate your YAML.
