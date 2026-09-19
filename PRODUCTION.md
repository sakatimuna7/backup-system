# Production Readiness Checklist

This document confirms that `backup-system` v0.5.0 is production-ready for VPS backups with optional recovery planning.

## Scope

- **Backups**: Automated restic-based snapshots to local, SFTP, S3, or Google Drive
- **Verification**: Integrity checks and dry-run restore testing
- **Scheduling**: Systemd timers with jitter and persistent missed-run recovery
- **Recovery Planning**: Read-only manifests and staged restore validation (MVP)

## What's Ready

### Core Features
- ✓ Multi-repository backup (local, SFTP, S3-compatible, Google Drive)
- ✓ Retention policy (daily, weekly, monthly with optional prune)
- ✓ Verification after backup
- ✓ Snapshot listing and manual restore
- ✓ Systemd timer integration
- ✓ Configuration validation

### Configuration & Secrets
- ✓ YAML config with absolute paths and named repositories
- ✓ Password file (mode 600) kept separate
- ✓ rclone config support (mode 600) for OAuth backends
- ✓ Secrets never logged or committed

### Recovery (Milestone 1-3)
- ✓ `recovery plan` — read-only manifest preview
- ✓ `recovery check` — validate staging and repository
- ✓ `recovery run` — restore to staging (non-destructive)

### Testing
- ✓ Unit tests (config parsing, flag handling, recovery schema)
- ✓ Integration tests (real restic sandbox E2E)
- ✓ Systemd unit generation and verification
- ✓ CI/CD workflow (gofmt, build, test, E2E, release)

## What's Not Included (Future Milestones)

- Database dump/restore orchestration (milestone 4)
- Package installation or user creation (milestone 5)
- Application build, start, and healthcheck (milestone 6)

Deploy only the staged recovery (milestones 1-3) to production. Post-recovery application setup remains manual or scripted separately.

## Deployment Steps

1. **Install binary**
   ```sh
   curl -LO https://github.com/sakatimuna7/backup-system/releases/download/v0.5.0/backup-system-linux-amd64
   curl -LO https://github.com/sakatimuna7/backup-system/releases/download/v0.5.0/SHA256SUMS
   sha256sum --check SHA256SUMS
   sudo install -m 755 backup-system-linux-amd64 /usr/local/bin/backup-system
   ```

2. **Initialize**
   ```sh
   sudo backup-system install
   sudo nano /etc/backup-system/config.yml
   sudo nano /etc/backup-system/password
   sudo backup-system config-check
   sudo backup-system init
   ```

3. **Test backup**
   ```sh
   sudo backup-system backup
   sudo backup-system snapshots
   sudo backup-system verify
   ```

4. **Enable scheduler (optional)**
   ```sh
   sudo backup-system schedule render
   sudo backup-system schedule install
   sudo systemctl status backup-system.timer
   ```

5. **Test recovery (optional)**
   ```sh
   sudo backup-system recovery plan
   sudo backup-system recovery check
   sudo backup-system recovery run
   ls /recovery/staging/  # inspect restored files
   ```

## Support & Limits

- **Host**: Linux only (tested on Ubuntu 24.04)
- **Restic**: 0.16.0 or later required
- **Go**: 1.22+ for building
- **Backup size**: Limited by disk and bandwidth; typical VPS: 5-50 GB
- **Retention**: Prune happens at backup time; no background cleanup jobs
- **Restore**: Always to a temporary directory first; never overwrite live paths directly

## Known Limitations

- Recovery run restores to staging only; post-restore setup (database, packages, services) is manual
- No built-in notification system; monitor systemd timer output or use syslog
- No dashboard or web UI; CLI-only
- Multi-destination backups run serially (by design for small VPS)

## Support

- Bug reports: https://github.com/sakatimuna7/backup-system/issues
- Security issues: Do not open public issues; contact the maintainer
- Feature requests: Check existing issues or open a new discussion
