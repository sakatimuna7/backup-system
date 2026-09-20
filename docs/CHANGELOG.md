# Changelog

All notable changes to this project are documented here.

## [0.9.0] - 2026-09-20

### Added
- Native Telegram notification support — no more n8n webhook dependency
- `notify.telegram` config section: `token_file`, `token`, `chat_id`, `thread_id`
- Sends `✅ Backup berhasil` / `❌ Backup gagal` after every `backup` command
- GDrive (rclone) as second repository — offsite backup alongside local
- `notify.telegram.token_file` supports `.env` format (`KEY=value`) with comment skipping

## [0.8.1] - 2026-09-20

### Changed
- Config version error messages now include supported range, binary version, and a direct link to the Version Compatibility docs
- `recovery requires config version 3` error now includes the current version and fix instruction

## [0.8.0] - 2026-09-20

### Added
- Runtime installer support in `recovery run`: `nvm`, `bun`, `npm-global`, `shell`
- `nvm` — installs nvm + specific Node.js version, sets default alias
- `npm-global` — installs npm global packages via nvm (e.g. `9router`)
- `bun` — installs bun via official install script
- `shell` — runs arbitrary install command (e.g. `uv tool install headroom-ai`)
- `runtimes` section in `recovery` config block
- Recovery execution order updated: packages → runtimes → users → databases

## [0.7.0] - 2026-09-19

### Added
- Package installation support via apt-get
- User creation with shell, home directory, and group management
- `recovery run` now installs packages and creates users before database restore
- Skip existing users automatically

### Changed
- Recovery workflow order: packages → users → databases
- `installPackages` runs apt-get update before install
- `createUser` checks user existence before creation

## [0.6.0] - 2026-09-19

### Added
- Database restore support for PostgreSQL and MySQL
- `recovery run` now restores database dumps from staging
- Database validation: engine (postgresql/mysql), absolute dump paths, required fields
- Recovery plan shows database count

### Changed
- `RecoveryDatabase.restore` field controls whether database is restored
- Database dumps read from staging directory (e.g., `/recovery/staging/var/backups/postgresql/myapp.sql`)

## [0.5.0] - 2026-09-19

### Added
- `recovery plan` — read-only preview of recovery manifest (packages, users, downloads, staging path)
- `recovery check` — validate staging directory, repository access, and dependencies before restore
- `recovery run` — non-destructive restore to staging directory (minimal MVP)
- Recovery manifest schema in `config.yml` with typed fields: packages, users, directories, downloads, databases, applications, services

### What's Included
- Restic-based encrypted snapshots, deduplication, and retention
- Multi-repository support (local, SFTP, S3-compatible, Google Drive via rclone)
- Systemd timer scheduling with jitter and missed-run recovery
- Configuration validation and actionable error messages
- Dry-run and verification modes

### What's Not Included
- Database dump orchestration (use pre-backup hooks)
- Application build or deployment (milestone 5+)
- Package installation or user creation (milestone 5+)
- Service restart or health checks (milestone 6+)

## [0.4.1] - 2026-09-19

### Fixed
- Schedule subcommands (`render`, `install`, `status`) no longer incorrectly parsed as repository selectors

## [0.4.0] - 2026-09-19

### Added
- Recovery manifest schema (YAML) with optional recovery configuration
- `recovery plan` command — read-only display of recovery manifest

## [0.3.0] - 2026-09-18

### Added
- Multi-repository support with required/optional policy
- rclone backend integration for Google Drive, S3, and other cloud storage

## [0.2.0] - 2026-09-17

### Added
- Systemd timer scheduling: `schedule render`, `schedule install`, `schedule status`

## [0.1.1] - 2026-09-16

### Added
- Help text and command suggestions for typos

## [0.1.0] - 2026-09-15

### Added
- Initial MVP: backup, snapshots, verify, restore
- Single-repository restic wrapper
- Config YAML with backup paths, exclusions, retention policy
- Password file (mode 600) separate from config
