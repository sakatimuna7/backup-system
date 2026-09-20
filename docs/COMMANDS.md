# Commands Reference

All backup-system commands with examples.

## Global Flags

```sh
backup-system -config /path/to/config.yml <command>
backup-system <command> --help
```

## Setup Commands

### install
Create default config and password file.

```sh
backup-system install
```

Creates:
- `/etc/backup-system/config.yml` (template)
- `/etc/backup-system/password` (empty, mode 600)

### config-check
Validate configuration, paths, permissions, and restic installation.

```sh
backup-system config-check
```

## Repository Commands

### init
Initialize restic repository.

```sh
backup-system init
backup-system init --repository local
```

### repositories
List configured repositories with status (required/optional).

```sh
backup-system repositories
```

Output:
```
local            required   local:/var/backups/restic
remote           optional   rclone:gdrive:backup-system/vps-01
```

## Backup Commands

### backup
Create a backup snapshot.

```sh
backup-system backup                    # All repositories
backup-system backup --repository local # Specific repository
```

Options:
- `--repository name` — Target specific repo

### snapshots
List available snapshots.

```sh
backup-system snapshots
backup-system snapshots --repository remote
```

Output:
```
ID        Time                 Host             Tags        Paths
--------------------------------------------------------------
abc123de  2026-09-19 23:02:10  VM-9-142-ubuntu              /etc
                                                            /home
```

### verify
Check repository integrity.

```sh
backup-system verify                    # All repositories
backup-system verify --repository local # Specific repo
```

## Restore Commands

### restore
Restore a snapshot to a temporary directory (NEVER to root).

```sh
backup-system restore latest /tmp/restore
backup-system restore abc123de /tmp/restore
backup-system restore latest /tmp/restore --repository local
```

Always verify in the staging directory before copying to live paths.

## Retention Commands

### retention
Preview or prune old snapshots based on retention policy.

```sh
backup-system retention                 # Dry-run (default)
backup-system retention --dry-run       # Explicit dry-run
backup-system retention --prune         # Actually delete
```

Options:
- `--dry-run` — Preview what would be deleted (default)
- `--prune` — Delete old snapshots

## Schedule Commands (systemd timers)

### schedule render
Preview systemd units that would be generated.

```sh
backup-system schedule render
```

### schedule install
Generate and install systemd timer & service units.

```sh
sudo backup-system schedule install
sudo systemctl daemon-reload
sudo systemctl enable --now backup-system.timer
```

### schedule status
Check if systemd timer is active.

```sh
backup-system schedule status
sudo systemctl status backup-system.timer
sudo systemctl status backup-system.service
```

### schedule remove
Uninstall systemd units.

```sh
sudo backup-system schedule remove
sudo systemctl daemon-reload
```

## Recovery Commands

Recovery commands are orchestrated, read-only, or staging-safe. No live changes.

### recovery plan
Display the recovery manifest (read-only, no changes).

```sh
backup-system recovery plan
```

Output:
```
Recovery plan (read-only)
  apt packages: 1
  users: 1
  databases: 1
  downloads: 0
  restore staging: /recovery/staging
  no changes made
```

### recovery check
Validate staging directory, repository access, and manifest dependencies.

```sh
backup-system recovery check
```

### recovery run
Restore snapshot to staging directory (non-destructive).

```sh
backup-system recovery run
```

Restores to staging path configured in `recovery.restore.staging` (default: `/recovery/staging`).

**Execution order:**
1. Install apt packages (`recovery.packages.apt`)
2. Install runtimes (`recovery.runtimes`) — nvm, bun, npm-global, shell
3. Create users (`recovery.users`)
4. Restore databases (`recovery.databases`)
5. Restore files to staging

**Runtime types supported:**

| Type | Example |
|------|---------|
| `nvm` | Install nvm + Node.js `24` |
| `npm-global` | `npm install -g 9router` via nvm |
| `bun` | Install bun via bun.sh |
| `shell` | Any arbitrary install command |

## Help Commands

### --help
Get help for any command.

```sh
backup-system --help               # Global help
backup-system backup --help        # Command-specific help
backup-system recovery --help      # Subcommand help
```

### version
Show installed version.

```sh
backup-system version
```

## Common Workflows

### Initial Setup

```sh
backup-system install
sudo nano /etc/backup-system/config.yml
sudo nano /etc/backup-system/password
backup-system config-check
backup-system init
```

### Daily Backup

```sh
backup-system backup
backup-system snapshots
backup-system verify
```

### Enable Automatic Backups

```sh
backup-system schedule render
sudo backup-system schedule install
```

### Recovery Drill

```sh
backup-system recovery plan
mkdir -p /recovery/staging
backup-system recovery check
backup-system recovery run
ls /recovery/staging/etc
```

### Full Restore

```sh
backup-system snapshots
backup-system restore latest /tmp/server-restore
sudo rsync -aHAX /tmp/server-restore/ /
```

## Typo Suggestions

Mistyped commands return close suggestions:

```sh
backup-system bakcup    # Suggests: "backup"
backup-system snapsoht  # Suggests: "snapshots"
```

## Exit Codes

- `0` — Success
- `1` — Error (check output)
- `2` — Usage error (typo or wrong flags)
