# backup-system

**Small, auditable VPS backup agent powered by [restic](https://restic.net/).**

[![Version](https://img.shields.io/badge/version-0.11.0-blue.svg)](https://github.com/sakatimuna7/backup-system/releases/tag/v0.11.0)
[![Tests](https://img.shields.io/badge/tests-24%2F24%20passing-success.svg)](https://github.com/sakatimuna7/backup-system)

## Features

✅ Multi-repository backups (local, SFTP, S3, Google Drive)  
✅ Encrypted snapshots with deduplication  
✅ Database restore (PostgreSQL, MySQL)  
✅ Package installation (apt-get)  
✅ User creation with groups  
✅ Systemd timer scheduling  
✅ Recovery staging (non-destructive)  

## Quick Start

```sh
# Install
curl -LO https://github.com/sakatimuna7/backup-system/releases/download/v0.11.0/backup-system-linux-amd64
curl -LO https://github.com/sakatimuna7/backup-system/releases/download/v0.11.0/SHA256SUMS
sha256sum --check SHA256SUMS
sudo install -m 755 backup-system-linux-amd64 /usr/local/bin/backup-system

# Setup
backup-system install
sudo nano /etc/backup-system/config.yml
sudo nano /etc/backup-system/password

# Backup
backup-system config-check
backup-system init
backup-system backup
```

## Documentation

📖 **[Full Documentation →](docs/README.md)**

- [Getting Started](docs/GETTING-STARTED.md) — Install & configure
- [Commands Reference](docs/COMMANDS.md) — All CLI commands
- [Recovery Guide](docs/RECOVERY.md) — Disaster recovery
- [Production Deployment](docs/PRODUCTION.md) — Deployment checklist
- [Configuration Schema](docs/references/config-schema.md) — YAML reference
- [Troubleshooting](docs/references/troubleshooting.md) — Common issues

## Example Commands

```sh
backup-system backup           # Create snapshot
backup-system snapshots        # List backups
backup-system verify           # Check integrity
backup-system restore          # Restore snapshot
backup-system recovery plan    # Preview recovery
backup-system schedule install # Enable timer
```

## Requirements

- Linux (Ubuntu 20.04+, Debian 11+)
- Go 1.22+ (to build)
- `restic` in PATH

## Build from Source

```sh
git clone https://github.com/sakatimuna7/backup-system.git
cd backup-system
go build -o backup-system .
sudo install -m 755 backup-system /usr/local/bin/backup-system
```

## License

MIT License — see [LICENSE](LICENSE)

## Support

- **Issues**: https://github.com/sakatimuna7/backup-system/issues
- **Docs**: [docs/README.md](docs/README.md)
- **Security**: Contact maintainer privately
