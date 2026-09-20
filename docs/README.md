# backup-system Documentation

Complete documentation for backup-system v0.9.0 — a small, auditable VPS backup agent powered by [restic](https://restic.net/).

## Quick Links

- **[Getting Started](GETTING-STARTED.md)** — Install, configure, first backup
- **[Commands Reference](COMMANDS.md)** — All CLI commands with examples
- **[Recovery Guide](RECOVERY.md)** — Step-by-step disaster recovery
- **[Production Deployment](PRODUCTION.md)** — Deployment checklist & limits
- **[Changelog](CHANGELOG.md)** — Version history v0.1.0 → v0.9.0

## References

- **[Config Schema](references/config-schema.md)** — YAML configuration reference
- **[Troubleshooting](references/troubleshooting.md)** — Common issues & solutions

## Features

✅ Multi-repository backups (local, SFTP, S3, Google Drive)  
✅ Encrypted snapshots with restic  
✅ Telegram notifications (native, no n8n needed)  
✅ Database restore (PostgreSQL, MySQL)  
✅ Package installation (apt-get)  
✅ Runtime installer (nvm, bun, npm-global, shell)  
✅ User creation with groups  
✅ Systemd timer scheduling  
✅ Recovery staging (non-destructive)  
✅ Retention policies (daily/weekly/monthly)  

## Quick Start

```sh
# Install
sudo install -m 755 backup-system /usr/local/bin/backup-system
sudo backup-system install

# Configure
sudo nano /etc/backup-system/config.yml
sudo nano /etc/backup-system/password

# Validate & backup
backup-system config-check
backup-system init
backup-system backup
```

## Command Summary

```sh
backup-system backup           # Create snapshot
backup-system snapshots        # List backups
backup-system verify           # Check integrity
backup-system restore          # Restore snapshot
backup-system recovery plan    # Preview recovery
backup-system recovery check   # Validate recovery
backup-system schedule install # Enable timer
```

## Support

- **Issues**: https://github.com/sakatimuna7/backup-system/issues
- **Security**: Contact maintainer privately
- **Docs**: https://github.com/sakatimuna7/backup-system/tree/main/docs
