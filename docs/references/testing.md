# Documentation Index

Complete documentation untuk backup-system v0.8.0.

## User Guides

### [README.md](README.md)
- Quick start setup
- Build & install
- Commands overview (15+ commands)
- systemd timer setup
- Recovery workflow
- rclone OAuth integration

### [RECOVERY.md](RECOVERY.md)
- Full recovery checklist (10 steps)
- Sandbox restore validation
- Database recovery (PostgreSQL, Redis)
- Manual recovery without backup-system
- Common issues & troubleshooting
- Monthly recovery drill template

### [PRODUCTION.md](PRODUCTION.md)
- Production readiness checklist
- Deployment steps (5 steps)
- Support & limits
- Known limitations

## Release Notes

### [CHANGELOG.md](CHANGELOG.md)
- v0.8.0 — Runtime installer (nvm, bun, npm-global, shell)
- v0.7.0 — Package install + user creation
- v0.6.0 — Database restore (PostgreSQL, MySQL)
- v0.5.0 — Recovery run (staging)
- v0.4.1 — Recovery check
- v0.4.0 — Recovery manifest + plan
- v0.3.0 — Multi-repository + rclone
- v0.2.0 — Systemd timer
- v0.1.0 — MVP backup/restore

### [WRAPUP-v0.7.0.md](WRAPUP-v0.7.0.md)
- Security audit results
- Data integrity validation
- Test coverage (24/24 pass)
- Known MVP limitations
- Ready-for-deployment checklist

## CLI Help

All commands available via `--help`:

```sh
backup-system --help              # Global help
backup-system <command> --help    # Command-specific help
backup-system recovery --help     # Recovery subcommands
```

## Quick Reference

**Setup:**
```sh
backup-system install
backup-system config-check
backup-system init
```

**Backup & Monitor:**
```sh
backup-system backup
backup-system snapshots
backup-system verify --dry-run
```

**Recovery:**
```sh
backup-system recovery plan        # Preview (read-only)
backup-system recovery check       # Validate
backup-system recovery run         # Restore to staging
```

**Scheduling:**
```sh
backup-system schedule render
backup-system schedule install
backup-system schedule status
```

## Configuration

See `config.example.yml` for full example with:
- Multi-repository setup
- Retention policy
- Systemd scheduling
- Recovery manifest (packages, users, databases)

## Support

- Issues: https://github.com/sakatimuna7/backup-system/issues
- Security: Contact maintainer privately
