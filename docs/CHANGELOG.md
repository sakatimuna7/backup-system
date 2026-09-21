# Changelog

All notable changes to this project are documented here.

## [0.12.0] - 2026-09-21

### Added
- **Pre/post hooks** — `backup.hooks.before`, `backup.hooks.after`, `backup.hooks.on_error` (bash commands run around backup)
- **Status file** — `status.file` writes JSON after every backup (`last_run`, `success`, `repos[]`, `duration`) — compatible with Uptime Kuma / healthcheck.io
- **Safety checks** — `safety.min_free_disk_mb` and `safety.min_free_memory_mb` abort backup before it starts if resources are low
- **Stale lock auto-unlock** — `safety.lock_timeout_min` auto-runs `restic unlock` on repos with locks older than N minutes

## [0.11.0] - 2026-09-20

### Added
- `backup-system update` — self-update command, downloads latest release from GitHub
- `backup-system update --check` — check for updates without downloading
- SHA256 verified before replacing binary (atomic rename)
- Detects correct arch (amd64/arm64) automatically
- 3 new tests: `TestSelfUpdateCheck`, `TestParseBackupOutput`, `TestFormatBackupNotif`

## [0.10.0] - 2026-09-20

### Added
- Detailed per-repo Telegram notifications after every backup
- Shows: snapshot ID, files new/changed, size added, stored size
- First backup detected (`no parent snapshot`) → labeled "full (first backup)"
- Summary line: `All N repo(s) OK ✓` or `N OK, M failed`
- Hostname included in notification header

## [0.9.0] - 2026-09-20

### Added
- Native Telegram notification support — no more n8n webhook dependency
- `notify.telegram` config section: `token_file`, `token`, `chat_id`, `thread_id`
- Sends `✅ Backup berhasil` / `❌ Backup gagal` after every `backup` command
- GDrive (rclone) as second repository — offsite backup alongside local
- **`env_file` support** — centralize all secrets in one mode-600 file
- `${VAR}` substitution in `config.yml` via `os.ExpandEnv` (stdlib, zero deps)
- `loadEnvFile` rejects files with permissions wider than 0600 (security gate)
- `.env` and `*.env` added to `.gitignore`

### Bug Fix
- `token_file` parser now correctly skips comment lines (`#`) in `.env` files

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
