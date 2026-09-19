# Troubleshooting

Common issues and solutions.

## Installation & Setup

### "restic is not installed or not in PATH"

**Solution:**
```sh
sudo apt install restic
backup-system config-check
```

Verify restic is installed:
```sh
which restic
restic version
```

### "config not found"

**Solution:**
```sh
backup-system install
sudo nano /etc/backup-system/config.yml
```

Use `-config` to specify custom location:
```sh
backup-system -config /tmp/config.yml config-check
```

## Configuration Errors

### "password file must not be empty"

**Solution:**
```sh
echo "your-password" | sudo tee /etc/backup-system/password
sudo chmod 600 /etc/backup-system/password
```

### "path must be absolute"

**Problem:** Config has relative paths like `backup.paths: ["etc", "home"]`

**Solution:** Use absolute paths:
```yaml
backup:
  paths:
    - "/etc"
    - "/home"
```

### "password file: permission denied"

**Problem:** Password file is world-readable or group-readable

**Solution:**
```sh
sudo chmod 600 /etc/backup-system/password
```

### "rclone_config: no such file"

**Problem:** Specified rclone.conf doesn't exist

**Solution:**
1. Setup rclone: `rclone config`
2. Copy config: `sudo install -m 600 ~/.config/rclone/rclone.conf /etc/backup-system/rclone.conf`
3. Update config.yml to match

## Backup Issues

### "repository already locked"

**Problem:** Another backup is running or previous backup crashed

**Solution:**
```sh
restic -r /var/backups/restic --password-file /etc/backup-system/password unlock
```

Or wait for the lock to timeout (default: several hours).

### Backup is very slow

**Possible causes:**
- First backup (reading all files)
- Large file count
- Slow storage (network, USB)

**Solutions:**
1. Exclude unnecessary paths in `backup.exclude`
2. Run incremental backups (after first backup, only changed files)
3. Increase backup window or run during off-peak

### "no parent snapshot found"

**Normal on first backup.** Restic reads all files. Subsequent backups are incremental (faster).

## Repository Issues

### "repository not found"

**Problem:** Restic can't access the repo URL

**Solution:**
1. Check URL: `backup-system config-check`
2. Verify path exists: `ls -la /var/backups/restic/`
3. For SFTP/S3: verify credentials and connectivity
4. For rclone: `rclone ls remote:path`

### "wrong password"

**Problem:** Password file content doesn't match repository password

**Solutions:**
1. Verify password file has correct password: `sudo cat /etc/backup-system/password`
2. No trailing newline issues: `echo -n "password" | sudo tee /etc/backup-system/password`
3. If password lost, repository is unrecoverable

### Repository corruption

**Problem:** `check` finds errors

**Solution:**
```sh
restic -r /var/backups/restic check --repair
```

Use `--repair` cautiously. Test recovery first.

## Recovery Issues

### "staging: stat /recovery/staging: no such file or directory"

**Problem:** Staging directory doesn't exist

**Solution:**
```sh
sudo mkdir -p /recovery/staging
sudo chmod 755 /recovery/staging
backup-system recovery check
```

### "database restore failed"

**Possible causes:**
- Database server not running
- Wrong database name or user
- Dump file corrupt or missing

**Solutions:**
1. Ensure PostgreSQL/MySQL is running: `sudo systemctl status postgresql`
2. Check dump exists: `ls -la /recovery/staging/var/backups/postgresql/`
3. Validate dump: `file dump.sql`

### Restore creates files with wrong permissions

**Problem:** Restored files aren't readable by their owners

**Solution:** Use `restic` directly with `-a` (all metadata):
```sh
restic -r /var/backups/restic restore --target /tmp/restore latest
```

backup-system preserves metadata by default.

## Scheduling Issues

### Systemd timer not running

**Problem:** Timer scheduled but never executes

**Solutions:**
```sh
# Check timer status
sudo systemctl status backup-system.timer

# View timer next run
sudo systemctl list-timers backup-system.timer

# Check service status
sudo systemctl status backup-system.service

# View logs
sudo journalctl -u backup-system.service -n 50
```

### "invalid calendar format"

**Problem:** `schedule.on_calendar` is malformed

**Solution:**
```sh
# Validate calendar format
systemd-analyze calendar "*-*-* 02:00:00"
```

Valid examples:
- `*-*-* 02:00:00` — Daily at 02:00
- `Mon *-*-* 03:00:00` — Mondays at 03:00

### Missed runs not caught up

**Problem:** `persistent: true` not configured

**Solution:**
```yaml
schedule:
  enabled: true
  on_calendar: "*-*-* 02:00:00"
  persistent: true
```

With `persistent: true`, missed runs are executed on next system boot.

## Performance

### High CPU during backup

**Cause:** restic is compressing data

**Solutions:**
- Exclude large media files (`.iso`, `.tar`, videos)
- Run backups during off-peak hours
- Single-threaded design is intentional (for small VPS)

### High memory usage

**Cause:** Large snapshots or many files

**Solutions:**
- Exclude non-critical paths
- Use smaller backup windows
- Monitor with: `ps aux | grep restic`

## Data Integrity

### How to verify a backup is valid

```sh
# Check repository integrity
backup-system verify

# List snapshots
backup-system snapshots

# Restore to staging and inspect
backup-system restore latest /tmp/verify
find /tmp/verify -type f | head -20
```

### How to test recovery

```sh
# Monthly recovery drill
backup-system recovery plan
mkdir -p /recovery/staging
backup-system recovery check
backup-system recovery run
ls -la /recovery/staging/etc/hostname
```

A backup that has never been restored is untrusted.

## Getting Help

- **Check logs:** `sudo journalctl -u backup-system.service`
- **Enable verbose mode:** Add debug output from restic via environment: `RESTIC_DEBUG=true`
- **Test with E2E script:** `./test-e2e.sh`
- **Report issues:** https://github.com/sakatimuna7/backup-system/issues
