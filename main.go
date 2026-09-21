package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	version       = "0.12.0"
	resticTimeout = 12 * time.Hour
)

var errResticMissing = errors.New("restic is not installed or not in PATH; install it with: sudo apt install restic")

func resticPath() (string, error) {
	path, err := exec.LookPath("restic")
	if err != nil {
		return "", errResticMissing
	}
	return path, nil
}

type Config struct {
	Version      int                `yaml:"version"`
	EnvFile      string             `yaml:"env_file"`
	Repositories []RepositoryConfig `yaml:"repositories,omitempty"`
	Repository   RepositoryConfig   `yaml:"repository,omitempty"` // v1 compatibility
	Backup       BackupConfig       `yaml:"backup"`
	Retention    RetentionConfig    `yaml:"retention"`
	Verify       VerifyConfig       `yaml:"verify"`
	Schedule     ScheduleConfig     `yaml:"schedule"`
	Recovery     RecoveryConfig     `yaml:"recovery"`
	Notify       NotifyConfig       `yaml:"notify"`
	Status       StatusConfig       `yaml:"status"`
	Safety       SafetyConfig       `yaml:"safety"`
}

type StatusConfig struct {
	File string `yaml:"file"` // path to write JSON status after backup
}

type SafetyConfig struct {
	MinFreeDiskMB   int `yaml:"min_free_disk_mb"`   // abort if free disk < threshold
	MinFreeMemoryMB int `yaml:"min_free_memory_mb"` // abort if free memory < threshold
	LockTimeoutMin  int `yaml:"lock_timeout_min"`   // auto-unlock stale locks older than N minutes
}

type NotifyConfig struct {
	Telegram *TelegramNotifyConfig `yaml:"telegram"`
}

type TelegramNotifyConfig struct {
	TokenFile string `yaml:"token_file"`
	Token     string `yaml:"token"`
	ChatID    string `yaml:"chat_id"`
	ThreadID  string `yaml:"thread_id"`
}

type RecoveryConfig struct {
	Packages     RecoveryPackages      `yaml:"packages"`
	Runtimes     []RecoveryRuntime     `yaml:"runtimes"`
	Users        []RecoveryUser        `yaml:"users"`
	Directories  []RecoveryDirectory   `yaml:"directories"`
	Restore      RecoveryRestore       `yaml:"restore"`
	Databases    []RecoveryDatabase    `yaml:"databases"`
	Applications []RecoveryApplication `yaml:"applications"`
	Services     RecoveryServices      `yaml:"services"`
	Downloads    []RecoveryDownload    `yaml:"downloads"`
}
type RecoveryPackages struct {
	Apt []string `yaml:"apt"`
}
type RecoveryRuntime struct {
	Name    string `yaml:"name"`
	Install string `yaml:"install"`
	Version string `yaml:"version"`
}
type RecoveryUser struct {
	Name       string   `yaml:"name"`
	Shell      string   `yaml:"shell"`
	Home       string   `yaml:"home"`
	Groups     []string `yaml:"groups"`
	CreateHome bool     `yaml:"create_home"`
}
type RecoveryDirectory struct {
	Path  string `yaml:"path"`
	Owner string `yaml:"owner"`
	Group string `yaml:"group"`
	Mode  string `yaml:"mode"`
}
type RecoveryRestore struct {
	Snapshot string   `yaml:"snapshot"`
	Staging  string   `yaml:"staging"`
	Include  []string `yaml:"include"`
	Exclude  []string `yaml:"exclude"`
}
type RecoveryDatabase struct {
	Name     string `yaml:"name"`
	Engine   string `yaml:"engine"`
	Database string `yaml:"database"`
	Owner    string `yaml:"owner"`
	DumpPath string `yaml:"dump_path"`
	Restore  bool   `yaml:"restore"`
}
type RecoveryApplication struct {
	Name         string              `yaml:"name"`
	Path         string              `yaml:"path"`
	Owner        string              `yaml:"owner"`
	Type         string              `yaml:"type"`
	Service      string              `yaml:"service"`
	Dependencies []string            `yaml:"dependencies"`
	Build        []string            `yaml:"build"`
	Start        []string            `yaml:"start"`
	Healthcheck  RecoveryHealthcheck `yaml:"healthcheck"`
}
type RecoveryHealthcheck struct {
	URL string `yaml:"url"`
}
type RecoveryServices struct {
	Enable []string `yaml:"enable"`
	Start  []string `yaml:"start"`
}
type RecoveryDownload struct {
	Name      string `yaml:"name"`
	URL       string `yaml:"url"`
	SHA256    string `yaml:"sha256"`
	InstallTo string `yaml:"install_to"`
	Mode      string `yaml:"mode"`
}

type RecoveryPlan struct {
	Configured bool
	Packages   int
	Users      int
	Databases  int
	Downloads  int
}

func recoveryConfigured(r RecoveryConfig) bool {
	return len(r.Packages.Apt) > 0 || len(r.Runtimes) > 0 || len(r.Users) > 0 || len(r.Directories) > 0 ||
		len(r.Restore.Include) > 0 || len(r.Restore.Exclude) > 0 || r.Restore.Staging != "" ||
		len(r.Databases) > 0 || len(r.Applications) > 0 || len(r.Services.Enable) > 0 || len(r.Services.Start) > 0 || len(r.Downloads) > 0
}

func validateRecovery(r RecoveryConfig) error {
	for _, user := range r.Users {
		if user.Name == "" || user.Home == "" || !filepath.IsAbs(user.Home) {
			return errors.New("recovery user name and absolute home are required")
		}
	}
	for _, dir := range r.Directories {
		if dir.Path == "" || !filepath.IsAbs(dir.Path) {
			return errors.New("recovery directory paths must be absolute")
		}
	}
	if r.Restore.Staging != "" && !filepath.IsAbs(r.Restore.Staging) {
		return errors.New("recovery.restore.staging must be absolute")
	}
	for _, path := range append(append([]string{}, r.Restore.Include...), r.Restore.Exclude...) {
		if !filepath.IsAbs(path) {
			return fmt.Errorf("recovery restore path must be absolute: %s", path)
		}
	}
	for _, db := range r.Databases {
		if db.Engine != "postgresql" && db.Engine != "mysql" {
			return fmt.Errorf("recovery database %s engine must be postgresql or mysql", db.Name)
		}
		if db.Database == "" {
			return fmt.Errorf("recovery database %s requires database name", db.Name)
		}
		if db.DumpPath == "" || !filepath.IsAbs(db.DumpPath) {
			return fmt.Errorf("recovery database %s dump_path must be absolute", db.Name)
		}
	}
	for _, download := range r.Downloads {
		u, err := url.Parse(download.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return fmt.Errorf("recovery download %s must use an https URL", download.Name)
		}
		if len(download.SHA256) != 64 {
			return fmt.Errorf("recovery download %s requires a 64-character SHA-256", download.Name)
		}
		if _, err := hex.DecodeString(download.SHA256); err != nil {
			return fmt.Errorf("recovery download %s has an invalid SHA-256", download.Name)
		}
		if download.InstallTo == "" || !filepath.IsAbs(download.InstallTo) {
			return fmt.Errorf("recovery download %s install_to must be absolute", download.Name)
		}
	}
	return nil
}

func recoveryPlan(c Config) RecoveryPlan {
	return RecoveryPlan{
		Configured: recoveryConfigured(c.Recovery),
		Packages:   len(c.Recovery.Packages.Apt), Users: len(c.Recovery.Users), Databases: len(c.Recovery.Databases), Downloads: len(c.Recovery.Downloads),
	}
}

type RepositoryConfig struct {
	Name         string `yaml:"name"`
	URL          string `yaml:"url"`
	PasswordFile string `yaml:"password_file"`
	RcloneConfig string `yaml:"rclone_config"`
	Required     *bool  `yaml:"required"`
}

func boolPtr(v bool) *bool                  { return &v }
func (r RepositoryConfig) isRequired() bool { return r.Required == nil || *r.Required }
func (r RepositoryConfig) displayName() string {
	if r.Name != "" {
		return r.Name
	}
	return "default"
}

type BackupConfig struct {
	Paths   []string    `yaml:"paths"`
	Exclude []string    `yaml:"exclude"`
	Hooks   BackupHooks `yaml:"hooks"`
}

type BackupHooks struct {
	Before  []string `yaml:"before"`
	After   []string `yaml:"after"`
	OnError []string `yaml:"on_error"`
}
type RetentionConfig struct {
	Daily   int  `yaml:"daily"`
	Weekly  int  `yaml:"weekly"`
	Monthly int  `yaml:"monthly"`
	Prune   bool `yaml:"prune"`
}
type VerifyConfig struct {
	AfterBackup bool `yaml:"after_backup"`
}
type ScheduleConfig struct {
	Enabled         bool   `yaml:"enabled"`
	OnCalendar      string `yaml:"on_calendar"`
	Persistent      bool   `yaml:"persistent"`
	RandomizedDelay string `yaml:"randomized_delay"`
}

type app struct {
	cfg  Config
	lock *os.File
}

func loadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, fmt.Errorf("config not found: %s (run `backup-system install` first)", path)
		}
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}

	// Pass 1: decode only env_file field (no expansion yet)
	var envPass struct {
		EnvFile string `yaml:"env_file"`
	}
	_ = yaml.NewDecoder(strings.NewReader(string(data))).Decode(&envPass)
	if envPass.EnvFile != "" {
		if err := loadEnvFile(envPass.EnvFile); err != nil {
			return Config{}, fmt.Errorf("env_file %s: %w", envPass.EnvFile, err)
		}
	}

	// Pass 2: expand ${VAR} then parse full config
	expanded := os.ExpandEnv(string(data))

	var c Config
	decoder := yaml.NewDecoder(strings.NewReader(expanded))
	decoder.KnownFields(true)
	if err := decoder.Decode(&c); err != nil {
		return c, fmt.Errorf("invalid config: %w", err)
	}
	if c.Version < 1 || c.Version > 3 {
		return c, fmt.Errorf(
			"unsupported config version %d\n"+
				"  supported: 1, 2, 3 (current: 3)\n"+
				"  this binary (v%s) requires config version 1–3\n"+
				"  see: https://github.com/sakatimuna7/backup-system/blob/main/docs/references/config-schema.md#version-compatibility",
			c.Version, version)
	}
	if c.Version < 3 && recoveryConfigured(c.Recovery) {
		return c, fmt.Errorf(
			"recovery requires config version 3 (current: %d)\n"+
				"  add 'version: 3' at the top of your config.yml\n"+
				"  see: https://github.com/sakatimuna7/backup-system/blob/main/docs/references/config-schema.md#version-compatibility",
			c.Version)
	}
	if err := validateRecovery(c.Recovery); err != nil {
		return c, err
	}
	if len(c.Repositories) == 0 && c.Repository.URL != "" {
		c.Repositories = []RepositoryConfig{{Name: "default", URL: c.Repository.URL, PasswordFile: c.Repository.PasswordFile}}
	}
	if len(c.Repositories) == 0 {
		return c, errors.New("repositories must not be empty")
	}
	seen := make(map[string]bool, len(c.Repositories))
	for i := range c.Repositories {
		r := &c.Repositories[i]
		if r.Name == "" {
			return c, errors.New("repository name is required")
		}
		if seen[r.Name] {
			return c, fmt.Errorf("duplicate repository name: %s", r.Name)
		}
		seen[r.Name] = true
		if r.URL == "" {
			return c, fmt.Errorf("repository %s: url is required", r.Name)
		}
		if r.PasswordFile == "" {
			r.PasswordFile = "/etc/backup-system/password"
		}
		if strings.HasPrefix(r.URL, "rclone:") {
			parts := strings.SplitN(strings.TrimPrefix(r.URL, "rclone:"), ":", 2)
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				return c, fmt.Errorf("repository %s: rclone URL must be rclone:<remote>:<path>", r.Name)
			}
			if r.RcloneConfig == "" {
				return c, fmt.Errorf("repository %s: rclone_config is required for rclone URLs", r.Name)
			}
		}
	}
	c.Repository = c.Repositories[0] // legacy internal callers use the first repository

	if len(c.Backup.Paths) == 0 {
		return c, errors.New("backup.paths must not be empty")
	}
	for _, p := range append(append([]string{}, c.Backup.Paths...), c.Backup.Exclude...) {
		if !filepath.IsAbs(p) {
			return c, fmt.Errorf("path must be absolute: %s", p)
		}
	}
	if c.Retention.Daily < 0 || c.Retention.Weekly < 0 || c.Retention.Monthly < 0 {
		return c, errors.New("retention values cannot be negative")
	}
	if c.Schedule.Enabled && strings.TrimSpace(c.Schedule.OnCalendar) == "" {
		return c, errors.New("schedule.on_calendar is required when schedule.enabled is true")
	}
	if c.Schedule.Enabled {
		if err := validateCalendar(c.Schedule.OnCalendar); err != nil {
			return c, err
		}
	}
	if c.Schedule.RandomizedDelay != "" {
		if d, err := time.ParseDuration(c.Schedule.RandomizedDelay); err != nil || d < 0 {
			return c, errors.New("schedule.randomized_delay must be a non-negative duration, e.g. 15m")
		}
	}
	for _, r := range c.Repositories {
		lst, err := os.Lstat(r.PasswordFile)
		if err != nil {
			return c, fmt.Errorf("repository %s password file: %w", r.Name, err)
		}
		if lst.Mode()&os.ModeSymlink != 0 {
			return c, fmt.Errorf("repository %s password_file must not be a symlink", r.Name)
		}
		if !lst.Mode().IsRegular() {
			return c, fmt.Errorf("repository %s password_file must be a regular file", r.Name)
		}
		if lst.Size() == 0 {
			return c, fmt.Errorf("repository %s password_file must not be empty", r.Name)
		}
		if lst.Mode().Perm()&0o077 != 0 {
			return c, fmt.Errorf("repository %s password_file must not be group/world-readable", r.Name)
		}
		if strings.HasPrefix(r.URL, "rclone:") {
			if _, err := exec.LookPath("rclone"); err != nil {
				return c, fmt.Errorf("repository %s requires rclone; install it with: sudo apt install rclone", r.Name)
			}
			rc, err := os.Lstat(r.RcloneConfig)
			if err != nil {
				return c, fmt.Errorf("repository %s rclone_config: %w", r.Name, err)
			}
			if rc.Mode()&os.ModeSymlink != 0 || !rc.Mode().IsRegular() {
				return c, fmt.Errorf("repository %s rclone_config must be a regular file", r.Name)
			}
			if rc.Mode().Perm()&0o077 != 0 {
				return c, fmt.Errorf("repository %s rclone_config must not be group/world-readable", r.Name)
			}
		}
	}
	return c, nil
}

func validateRestoreTarget(target string) error {
	if target == "/" {
		return errors.New("restore target / is refused; restore to a sandbox or explicit mount point")
	}
	info, err := os.Stat(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("restore target: %w", err)
	}
	if !info.IsDir() {
		return errors.New("restore target must be a directory")
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return fmt.Errorf("read restore target: %w", err)
	}
	if len(entries) != 0 {
		return errors.New("restore target must be empty; restore to a new sandbox directory")
	}
	return nil
}

func acquireLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, errors.New("another operation is already running")
	}
	return f, nil
}
func cleanResticEnv(env []string) []string {
	clean := make([]string, 0, len(env))
	for _, item := range env {
		key, _, ok := strings.Cut(item, "=")
		if ok && (key == "RESTIC_REPOSITORY" || key == "RESTIC_PASSWORD_FILE" || key == "RCLONE_CONFIG") {
			continue
		}
		clean = append(clean, item)
	}
	return clean
}

func (a *app) runRepo(repo RepositoryConfig, args ...string) error {
	_, _, err := a.runRepoCapture(repo, args...)
	return err
}

func (a *app) runRepoCapture(repo RepositoryConfig, args ...string) (string, string, error) {
	restic, err := resticPath()
	if err != nil {
		return "", "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), resticTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, restic, args...)
	cmd.Env = cleanResticEnv(os.Environ())
	cmd.Env = append(cmd.Env, "RESTIC_REPOSITORY="+repo.URL, "RESTIC_PASSWORD_FILE="+repo.PasswordFile)
	if repo.RcloneConfig != "" {
		cmd.Env = append(cmd.Env, "RCLONE_CONFIG="+repo.RcloneConfig)
	}
	var outBuf, errBuf strings.Builder
	cmd.Stdout = io.MultiWriter(os.Stdout, &outBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &errBuf)
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return outBuf.String(), errBuf.String(), fmt.Errorf("restic timed out after %s; check repository connectivity and retry", resticTimeout)
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return outBuf.String(), errBuf.String(), fmt.Errorf("restic failed (exit %d): check the repository URL, password file, and restic output above", exitErr.ExitCode())
		}
		return outBuf.String(), errBuf.String(), fmt.Errorf("run restic: %w", err)
	}
	return outBuf.String(), errBuf.String(), nil
}
func (a *app) run(args ...string) error { return a.runRepo(a.cfg.Repositories[0], args...) }

// BackupResult holds parsed restic backup output for one repository.
type BackupResult struct {
	Repo           string
	Required       bool
	SnapshotID     string // short ID
	FilesNew       string
	FilesChanged   string
	FilesUnchanged string
	Added          string // e.g. "2.681 MiB"
	Stored         string // e.g. "461.060 KiB stored"
	NoParent       bool   // first backup (no parent snapshot)
	Err            error
}

func parseBackupOutput(out string) (snapshotID, filesNew, filesChanged, filesUnchanged, added, stored string, noParent bool) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "snapshot ") && strings.Contains(line, "saved") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				snapshotID = parts[1]
			}
		}
		if strings.HasPrefix(line, "Files:") {
			// Files:           0 new,     7 changed, 20577 unmodified
			parts := strings.Fields(line)
			if len(parts) >= 7 {
				filesNew = parts[1]
				filesChanged = parts[3]
				filesUnchanged = parts[5]
			}
		}
		if strings.HasPrefix(line, "Added to the repository:") {
			// Added to the repository: 2.681 MiB (461.060 KiB stored)
			rest := strings.TrimPrefix(line, "Added to the repository:")
			rest = strings.TrimSpace(rest)
			if idx := strings.Index(rest, "("); idx >= 0 {
				added = strings.TrimSpace(rest[:idx])
				stored = strings.TrimSuffix(strings.TrimSpace(rest[idx+1:]), ")")
			} else {
				added = rest
			}
		}
		if strings.Contains(line, "no parent snapshot found") {
			noParent = true
		}
	}
	return
}

func formatBackupNotif(results []BackupResult, hostname string) (success bool, text string) {
	success = true
	for _, r := range results {
		if r.Err != nil && r.Required {
			success = false
			break
		}
	}

	icon := "✅"
	if !success {
		icon = "❌"
	}

	var sb strings.Builder
	sb.WriteString(icon + " *Backup " + hostname + "*\n")
	sb.WriteString(strings.Repeat("─", 28) + "\n")

	for _, r := range results {
		label := r.Repo
		if r.Required {
			label += " (required)"
		}
		if r.Err != nil {
			sb.WriteString("❌ `" + label + "`\n")
			sb.WriteString("   Error: " + r.Err.Error() + "\n")
			continue
		}
		sb.WriteString("✅ `" + label + "`\n")
		if r.SnapshotID != "" {
			sb.WriteString("   Snapshot: `" + r.SnapshotID + "`\n")
		}
		if r.NoParent {
			sb.WriteString("   Type: full (first backup)\n")
		} else if r.FilesNew != "" {
			sb.WriteString("   Files: " + r.FilesNew + " new, " + r.FilesChanged + " changed\n")
		}
		if r.Added != "" {
			line := "   Size: " + r.Added
			if r.Stored != "" {
				line += " (" + r.Stored + ")"
			}
			sb.WriteString(line + "\n")
		}
	}

	sb.WriteString(strings.Repeat("─", 28) + "\n")
	ok, fail := 0, 0
	for _, r := range results {
		if r.Err != nil {
			fail++
		} else {
			ok++
		}
	}
	if fail == 0 {
		sb.WriteString("All " + strconv.Itoa(ok) + " repo(s) OK ✓")
	} else {
		sb.WriteString(strconv.Itoa(ok) + " OK, " + strconv.Itoa(fail) + " failed")
	}

	return success, sb.String()
}

func (a *app) backupRepo(repo RepositoryConfig) error {
	args := append([]string{"backup"}, a.cfg.Backup.Paths...)
	for _, p := range a.cfg.Backup.Exclude {
		args = append(args, "--exclude", p)
	}
	if err := a.runRepo(repo, args...); err != nil {
		return err
	}
	if a.cfg.Verify.AfterBackup {
		return a.runRepo(repo, "check")
	}
	return nil
}
func (a *app) retentionRepo(repo RepositoryConfig, prune bool) error {
	args := []string{"forget", "--keep-daily", strconv.Itoa(a.cfg.Retention.Daily), "--keep-weekly", strconv.Itoa(a.cfg.Retention.Weekly), "--keep-monthly", strconv.Itoa(a.cfg.Retention.Monthly), "--dry-run"}
	if prune {
		args[len(args)-1] = "--prune"
	}
	return a.runRepo(repo, args...)
}
func selectRepositories(c Config, name string) ([]RepositoryConfig, error) {
	if name == "" {
		return c.Repositories, nil
	}
	for _, r := range c.Repositories {
		if r.Name == name {
			return []RepositoryConfig{r}, nil
		}
	}
	return nil, fmt.Errorf("unknown repository %q", name)
}
func runRepositories(c Config, name string, fn func(RepositoryConfig) error) error {
	repos, err := selectRepositories(c, name)
	if err != nil {
		return err
	}
	failed, requiredFailed := 0, 0
	for _, repo := range repos {
		if err := fn(repo); err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "repository %s: FAILED: %v\n", repo.displayName(), err)
			if repo.isRequired() {
				requiredFailed++
			}
		} else {
			fmt.Printf("repository %s: OK\n", repo.displayName())
		}
	}
	if failed > 0 && requiredFailed > 0 {
		return fmt.Errorf("%d required repository(s) failed", requiredFailed)
	}
	return nil
}

func repositorySelector(args []string) (string, error) {
	selector := ""
	for i := 0; i < len(args); i++ {
		if args[i] != "--repository" {
			return "", fmt.Errorf("unexpected argument %q; use --repository name", args[i])
		}
		if selector != "" || i+1 >= len(args) || args[i+1] == "" {
			return "", errors.New("--repository requires one name")
		}
		selector = args[i+1]
		i++
	}
	return selector, nil
}

func validRepositoryArgs(args []string) bool {
	_, err := repositorySelector(args)
	return err == nil && (len(args) == 0 || (len(args) == 2 && args[0] == "--repository"))
}

func restoreArgs(args []string) (string, []string, error) {
	selector := ""
	positional := make([]string, 0, 2)
	for i := 0; i < len(args); i++ {
		if args[i] == "--repository" {
			if selector != "" || i+1 >= len(args) || args[i+1] == "" {
				return "", nil, errors.New("--repository requires one name")
			}
			selector = args[i+1]
			i++
			continue
		}
		positional = append(positional, args[i])
	}
	if len(positional) != 2 {
		return "", nil, errors.New("usage: restore [--repository name] <snapshot> <target>")
	}
	return selector, positional, nil
}

func retentionArgs(args []string) (string, bool, error) {
	selector := ""
	prune := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--prune":
			if prune {
				return "", false, errors.New("--prune specified twice")
			}
			prune = true
		case "--dry-run":
			if prune {
				return "", false, errors.New("--dry-run cannot be combined with --prune")
			}
		case "--repository":
			if selector != "" || i+1 >= len(args) || args[i+1] == "" {
				return "", false, errors.New("--repository requires one name")
			}
			selector = args[i+1]
			i++
		default:
			return "", false, fmt.Errorf("unexpected argument %q", args[i])
		}
	}
	return selector, prune, nil
}

var commands = []string{"install", "version", "update", "config-check", "repositories", "init", "backup", "snapshots", "verify", "retention", "restore", "recovery", "schedule"}

const (
	systemdDir      = "/etc/systemd/system"
	serviceUnitName = "backup-system.service"
	timerUnitName   = "backup-system.timer"
)

func systemdArg(value string) string {
	value = strings.ReplaceAll(value, `%`, `%%`)
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

func serviceUnit(configPath, binaryPath string) string {
	return fmt.Sprintf(`[Unit]
Description=backup-system scheduled backup
Wants=network-online.target
After=network-online.target

[Service]
Type=oneshot
ExecStart=%s -config %s backup
Nice=10
IOSchedulingClass=best-effort
IOSchedulingPriority=7
NoNewPrivileges=true
PrivateTmp=true
`, systemdArg(binaryPath), systemdArg(configPath))
}

func validateCalendar(calendar string) error {
	cmd := exec.Command("systemd-analyze", "calendar", calendar)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("invalid schedule.on_calendar %q: %s", calendar, strings.TrimSpace(string(output)))
	}
	return nil
}

func timerUnit(c ScheduleConfig) string {
	persistent := "false"
	if c.Persistent {
		persistent = "true"
	}
	unit := fmt.Sprintf(`[Unit]
Description=Run backup-system scheduled backup

[Timer]
OnCalendar=%s
Persistent=%s
Unit=%s`, c.OnCalendar, persistent, serviceUnitName)
	if c.RandomizedDelay != "" {
		unit += "\nRandomizedDelaySec=" + c.RandomizedDelay
	}
	return unit + "\n\n[Install]\nWantedBy=timers.target\n"
}

func writeUnit(path, content string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".backup-system-unit-*")
	if err != nil {
		return fmt.Errorf("create temporary unit: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return fmt.Errorf("set unit permissions: %w", err)
	}
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary unit: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("install %s: %w", path, err)
	}
	return nil
}

func executablePath() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find backup-system executable: %w", err)
	}
	return filepath.Abs(path)
}

func systemctl(args ...string) error {
	cmd := exec.Command("systemctl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("systemctl %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

func scheduleStatus() error {
	cmd := exec.Command("systemctl", "show", timerUnitName, "--no-pager", "--property=LoadState,ActiveState,UnitFileState,NextElapseUSecRealtime,LastTriggerUSec")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl show %s: %w", timerUnitName, err)
	}
	if strings.Contains(string(output), "LoadState=not-found") {
		fmt.Printf("schedule: not installed\ntimer: %s\n", timerUnitName)
		return nil
	}
	fmt.Printf("schedule: %s\n%s", timerUnitName, output)
	return nil
}

func scheduleInstall(c Config, configPath string) error {
	if !c.Schedule.Enabled {
		return errors.New("schedule is disabled in config.yml")
	}
	if err := validateCalendar(c.Schedule.OnCalendar); err != nil {
		return err
	}
	binaryPath, err := executablePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(systemdDir, 0o755); err != nil {
		return fmt.Errorf("create systemd unit directory: %w", err)
	}
	if err := writeUnit(filepath.Join(systemdDir, serviceUnitName), serviceUnit(configPath, binaryPath)); err != nil {
		return err
	}
	if err := writeUnit(filepath.Join(systemdDir, timerUnitName), timerUnit(c.Schedule)); err != nil {
		return err
	}
	if err := systemctl("daemon-reload"); err != nil {
		return err
	}
	if err := systemctl("enable", "--now", timerUnitName); err != nil {
		return err
	}
	fmt.Printf("schedule: installed\ntimer: %s\ncalendar: %s\n", timerUnitName, c.Schedule.OnCalendar)
	return nil
}

func scheduleRemove() error {
	servicePath := filepath.Join(systemdDir, serviceUnitName)
	timerPath := filepath.Join(systemdDir, timerUnitName)
	serviceExists := !errors.Is(func() error { _, err := os.Stat(servicePath); return err }(), os.ErrNotExist)
	timerExists := !errors.Is(func() error { _, err := os.Stat(timerPath); return err }(), os.ErrNotExist)
	if !serviceExists && !timerExists {
		fmt.Println("schedule: not installed")
		return nil
	}
	if err := systemctl("disable", "--now", timerUnitName); err != nil {
		return err
	}
	for _, name := range []string{serviceUnitName, timerUnitName} {
		if err := os.Remove(filepath.Join(systemdDir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", name, err)
		}
	}
	return systemctl("daemon-reload")
}

func printHelp(command string) {
	if command == "" {
		fmt.Println("backup-system - simple, auditable VPS backups powered by restic")
		fmt.Println()
		fmt.Println("USAGE")
		fmt.Println("  backup-system [-config path] <command> [flags]")
		fmt.Println()
		fmt.Println("AVAILABLE COMMANDS")
		fmt.Println("  install:       Create the config and password file")
		fmt.Println("  version:       Show the installed version")
		fmt.Println("  config-check:  Validate config, secrets, and restic")
		fmt.Println("  repositories:  List configured repositories")
		fmt.Println("  init:          Initialize the restic repository")
		fmt.Println("  backup:        Create a backup snapshot")
		fmt.Println("  snapshots:     List available snapshots")
		fmt.Println("  verify:        Check repository integrity")
		fmt.Println("  retention:     Preview or prune old snapshots")
		fmt.Println("  restore:       Restore a snapshot to an empty directory")
		fmt.Println("  schedule:      Install, inspect, or remove the backup timer")
		fmt.Println()
		fmt.Println("SCHEDULE COMMANDS")
		fmt.Println("  backup-system schedule render")
		fmt.Println("  backup-system schedule install")
		fmt.Println("  backup-system schedule status")
		fmt.Println("  backup-system schedule remove")
		fmt.Println()
		fmt.Println("FLAGS")
		fmt.Println("  -config path   Use a config file other than /etc/backup-system/config.yml")
		fmt.Println("  --help         Show help for a command")
		fmt.Println()
		fmt.Println("EXAMPLES")
		fmt.Println("  backup-system install")
		fmt.Println("  backup-system config-check")
		fmt.Println("  backup-system backup")
		fmt.Println("  backup-system restore latest /tmp/server-restore")
		fmt.Println()
		fmt.Println("Run 'backup-system <command> --help' for more information about a command.")
		return
	}
	fmt.Printf("backup-system %s\n\n", command)
	switch command {
	case "install":
		fmt.Println("Create the default config and password file.")
	case "version":
		fmt.Println("Show the installed backup-system version.")
	case "update":
		fmt.Println("Check for a newer release on GitHub and update the binary in-place.")
		fmt.Println("\nUSAGE\n  backup-system update           # update to latest\n  backup-system update --check   # check only, no download")
		fmt.Println("\nRequires write permission to the installed binary path (run with sudo if needed).")
		fmt.Println("SHA256 is verified before replacing the binary.")
	case "config-check":
		fmt.Println("Validate YAML, paths, password permissions, and restic.")
	case "repositories":
		fmt.Println("List configured repositories and their required/optional status.")
		fmt.Println("\nUSAGE\n  backup-system repositories")
		fmt.Println("\nRepository URLs and paths are shown; credential contents are never displayed.")
	case "init", "backup", "snapshots", "verify":
		fmt.Println("Use --repository name to target one repository, or omit it to process all configured repositories.")
		fmt.Println("\nUSAGE\n  backup-system " + command + " [--repository name]")
	case "retention":
		fmt.Println("Preview retention by default; use --prune to delete old snapshots.")
		fmt.Println("\nUSAGE\n  backup-system retention [--dry-run|--prune] [--repository name]")
	case "restore":
		fmt.Println("Restore a snapshot into a new or empty absolute directory.")
		fmt.Println("\nUSAGE\n  backup-system restore [--repository name] <snapshot|latest> <absolute-target>")
	case "recovery":
		fmt.Println("Show recovery plan or validate recovery prerequisites.")
		fmt.Println("\nUSAGE\n  backup-system recovery <plan|check>")
		fmt.Println("\nSUBCOMMANDS")
		fmt.Println("  plan  Display manifest from config (read-only)")
		fmt.Println("  check Validate staging, repository, and manifest paths")
	case "schedule":
		fmt.Println("Manage the systemd timer that runs backup-system backup.")
		fmt.Println("\nUSAGE\n  backup-system schedule <render|install|status|remove>")
	}
	fmt.Println("\nUse 'backup-system --help' for the full command list.")
}

func levenshtein(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i, ar := range a {
		cur := make([]int, len(b)+1)
		cur[0] = i + 1
		for j, br := range b {
			cost := 0
			if ar != br {
				cost = 1
			}
			cur[j+1] = min(cur[j]+1, prev[j+1]+1, prev[j]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

func suggestion(input string) string {
	best, distance := "", 3
	for _, command := range commands {
		if d := levenshtein(input, command); d < distance {
			best, distance = command, d
		}
	}
	return best
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func recoveryCheck(c Config, stagingPath, repoPath string) []string {
	var errs []string

	// Staging path harus ada dan writable
	if stat, err := os.Stat(stagingPath); err != nil {
		errs = append(errs, fmt.Sprintf("staging: %v", err))
	} else if !stat.IsDir() {
		errs = append(errs, "staging must be a directory")
	}

	// Repository path harus ada
	if stat, err := os.Stat(repoPath); err != nil {
		errs = append(errs, fmt.Sprintf("repository: %v", err))
	} else if !stat.IsDir() {
		errs = append(errs, "repository must be a directory")
	}

	// Download URLs dan checksum sudah divalidasi di loadConfig

	return errs
}

func recoveryRun(c Config, stagingOnly bool) []string {
	var errs []string

	stagingPath := c.Recovery.Restore.Staging
	if stagingPath == "" {
		stagingPath = "/recovery/staging"
	}

	repoPath := strings.TrimPrefix(c.Repository.URL, "local:")
	if !strings.HasPrefix(c.Repository.URL, "local:") {
		errs = append(errs, "recovery run only supports local repositories for now")
		return errs
	}

	// Validate staging dan repo exist
	checkErrs := recoveryCheck(c, stagingPath, repoPath)
	if len(checkErrs) > 0 {
		return checkErrs
	}

	// Install packages
	if len(c.Recovery.Packages.Apt) > 0 {
		if pkgErrs := installPackages(c.Recovery.Packages.Apt); len(pkgErrs) > 0 {
			errs = append(errs, pkgErrs...)
		}
	}

	// Install runtimes (nvm, bun, etc.)
	if len(c.Recovery.Runtimes) > 0 {
		if rtErrs := installRuntimes(c.Recovery.Runtimes); len(rtErrs) > 0 {
			errs = append(errs, rtErrs...)
		}
	}

	// Create users
	if len(c.Recovery.Users) > 0 {
		if userErrs := createUsers(c.Recovery.Users); len(userErrs) > 0 {
			errs = append(errs, userErrs...)
		}
	}

	// Restore databases yang ada di staging
	for _, db := range c.Recovery.Databases {
		if !db.Restore {
			continue
		}
		dumpPath := filepath.Join(stagingPath, strings.TrimPrefix(db.DumpPath, "/"))
		if _, err := os.Stat(dumpPath); err != nil {
			errs = append(errs, fmt.Sprintf("database %s dump not found: %s", db.Name, dumpPath))
			continue
		}

		if err := restoreDatabase(db, dumpPath); err != nil {
			errs = append(errs, fmt.Sprintf("database %s restore failed: %v", db.Name, err))
		}
	}

	return errs
}


// ── Hooks ─────────────────────────────────────────────────────────────────

func runHooks(hooks []string, label string) error {
	for i, cmd := range hooks {
		c := exec.Command("bash", "-c", cmd)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("hook[%s][%d] %q failed: %w", label, i, cmd, err)
		}
	}
	return nil
}

// ── Safety checks ─────────────────────────────────────────────────────────

func checkSafety(s SafetyConfig) error {
	if s.MinFreeDiskMB > 0 {
		var stat syscall.Statfs_t
		if err := syscall.Statfs("/", &stat); err == nil {
			freeMB := int(stat.Bavail * uint64(stat.Bsize) / 1024 / 1024)
			if freeMB < s.MinFreeDiskMB {
				return fmt.Errorf("not enough free disk: %d MB available, need %d MB", freeMB, s.MinFreeDiskMB)
			}
		}
	}
	if s.MinFreeMemoryMB > 0 {
		data, err := os.ReadFile("/proc/meminfo")
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "MemAvailable:") {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						kb, _ := strconv.Atoi(fields[1])
						freeMB := kb / 1024
						if freeMB < s.MinFreeMemoryMB {
							return fmt.Errorf("not enough free memory: %d MB available, need %d MB", freeMB, s.MinFreeMemoryMB)
						}
					}
					break
				}
			}
		}
	}
	return nil
}

func unlockStaleLocks(a *app, repos []RepositoryConfig, timeoutMin int) {
	if timeoutMin <= 0 {
		return
	}
	for _, repo := range repos {
		out, _, _ := a.runRepoCapture(repo, "list", "locks")
		stale := false
		threshold := time.Now().Add(-time.Duration(timeoutMin) * time.Minute)
		for _, line := range strings.Split(out, "\n") {
			// restic lock lines contain timestamp
			if strings.Contains(line, "T") && len(line) > 20 {
				// try parse date portion
				parts := strings.Fields(line)
				for _, p := range parts {
					if t, err := time.Parse(time.RFC3339, p); err == nil {
						if t.Before(threshold) {
							stale = true
						}
					}
				}
			}
		}
		if stale {
			fmt.Printf("unlocking stale lock in %s...\n", repo.displayName())
			_ = a.runRepo(repo, "unlock")
		}
	}
}

// ── Status file ───────────────────────────────────────────────────────────

type backupStatus struct {
	Version    string        `json:"version"`
	Hostname   string        `json:"hostname"`
	LastRun    time.Time     `json:"last_run"`
	Success    bool          `json:"success"`
	Duration   string        `json:"duration"`
	Repos      []repoStatus  `json:"repos"`
}

type repoStatus struct {
	Name       string `json:"name"`
	Success    bool   `json:"success"`
	SnapshotID string `json:"snapshot_id,omitempty"`
	FilesNew   string `json:"files_new,omitempty"`
	Added      string `json:"added,omitempty"`
	Error      string `json:"error,omitempty"`
}

func writeStatusFile(path string, results []BackupResult, success bool, dur time.Duration) {
	if path == "" {
		return
	}
	hostname, _ := os.Hostname()
	status := backupStatus{
		Version:  version,
		Hostname: hostname,
		LastRun:  time.Now().UTC(),
		Success:  success,
		Duration: dur.Round(time.Second).String(),
	}
	for _, r := range results {
		rs := repoStatus{Name: r.Repo, Success: r.Err == nil, SnapshotID: r.SnapshotID, FilesNew: r.FilesNew, Added: r.Added}
		if r.Err != nil {
			rs.Error = r.Err.Error()
		}
		status.Repos = append(status.Repos, rs)
	}
	data, _ := json.MarshalIndent(status, "", "  ")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "warn: status file mkdir: %v\n", err)
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "warn: status file write: %v\n", err)
	}
}
// loadEnvFile reads KEY=value pairs from path into the process environment.
// Requires file mode 0600 or stricter. Skips blank lines and # comments.
// No shell expansion — values are stored literally.
func loadEnvFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot stat: %w", err)
	}
	if info.Mode().Perm()&0o177 != 0 {
		return fmt.Errorf("insecure permissions %04o (must be 0600)", info.Mode().Perm())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for i, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("line %d: invalid format (expected KEY=value)", i+1)
		}
		k = strings.TrimSpace(k)
		if k == "" {
			return fmt.Errorf("line %d: empty key", i+1)
		}
		if err := os.Setenv(k, strings.TrimSpace(v)); err != nil {
			return fmt.Errorf("line %d: setenv %s: %w", i+1, k, err)
		}
	}
	return nil
}

// selfUpdate checks GitHub for a newer release and replaces the running binary.
// checkOnly=true prints status without downloading.
func selfUpdate(checkOnly bool) error {
	const repo = "sakatimuna7/backup-system"
	type ghRelease struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/" + repo + "/releases/latest")
	if err != nil {
		return fmt.Errorf("fetch latest release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return fmt.Errorf("parse release: %w", err)
	}

	latest := strings.TrimPrefix(rel.TagName, "v")
	if latest == version {
		fmt.Printf("backup-system %s is already up to date\n", version)
		return nil
	}
	fmt.Printf("Current: %s → Latest: %s\n", version, latest)
	if checkOnly {
		fmt.Println("Run `backup-system update` to install.")
		return nil
	}

	// Determine asset name for this platform
	arch := runtime.GOARCH // amd64, arm64
	assetName := fmt.Sprintf("backup-system-linux-%s", arch)
	sha256Name := "SHA256SUMS"

	var downloadURL, sha256URL string
	for _, a := range rel.Assets {
		switch a.Name {
		case assetName:
			downloadURL = a.BrowserDownloadURL
		case sha256Name:
			sha256URL = a.BrowserDownloadURL
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("no asset %q in release %s", assetName, rel.TagName)
	}

	// Download binary to temp file
	fmt.Printf("Downloading %s...\n", assetName)
	tmp, err := os.CreateTemp("", "backup-system-update-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	defer os.Remove(tmp.Name())

	resp2, err := client.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("download binary: %w", err)
	}
	defer resp2.Body.Close()

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), resp2.Body); err != nil {
		return fmt.Errorf("write binary: %w", err)
	}
	tmp.Close()
	actualHash := hex.EncodeToString(h.Sum(nil))

	// Verify SHA256 if available
	if sha256URL != "" {
		resp3, err := client.Get(sha256URL)
		if err == nil {
			defer resp3.Body.Close()
			body, _ := io.ReadAll(resp3.Body)
			verified := false
			for _, line := range strings.Split(string(body), "\n") {
				parts := strings.Fields(line)
				if len(parts) == 2 && parts[1] == assetName {
					if parts[0] != actualHash {
						return fmt.Errorf("SHA256 mismatch: expected %s got %s", parts[0], actualHash)
					}
					verified = true
					break
				}
			}
			if verified {
				fmt.Println("SHA256 verified ✓")
			}
		}
	}

	// Atomic replace: chmod → rename over existing binary
	if err := os.Chmod(tmp.Name(), 0755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate self: %w", err)
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return fmt.Errorf("resolve symlink: %w", err)
	}
	if err := os.Rename(tmp.Name(), self); err != nil {
		return fmt.Errorf("replace binary (try sudo): %w", err)
	}
	fmt.Printf("✅ Updated to backup-system %s\n", latest)
	return nil
}

func sendTelegramNotify(cfg *TelegramNotifyConfig, success bool, detail string) error {
	if cfg == nil {
		return nil
	}
	token := cfg.Token
	if token == "" && cfg.TokenFile != "" {
		raw, err := os.ReadFile(cfg.TokenFile)
		if err != nil {
			return fmt.Errorf("notify: cannot read token_file: %w", err)
		}
		// support KEY=value or bare value
		for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "#") {
				continue
			}
			if after, ok := strings.CutPrefix(line, "TELEGRAM_BOT_TOKEN="); ok {
				token = strings.TrimSpace(after)
				break
			}
		}
	}
	if token == "" {
		return errors.New("notify: telegram token is empty")
	}
	icon, title := "✅", "Backup berhasil"
	if !success {
		icon, title = "❌", "Backup gagal"
	}
	text := icon + " " + title
	if detail != "" {
		text += "\n" + detail
	}
	form := url.Values{}
	form.Set("chat_id", cfg.ChatID)
	form.Set("text", text)
	if cfg.ThreadID != "" {
		form.Set("message_thread_id", cfg.ThreadID)
	}
	resp, err := http.PostForm(
		fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token),
		form,
	)
	if err != nil {
		return fmt.Errorf("notify: telegram request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("notify: telegram returned %d", resp.StatusCode)
	}
	return nil
}

func installRuntimes(runtimes []RecoveryRuntime) []string {
	var errs []string
	for _, rt := range runtimes {
		if err := installRuntime(rt); err != nil {
			errs = append(errs, fmt.Sprintf("runtime %s install failed: %v", rt.Name, err))
		}
	}
	return errs
}

func installRuntime(rt RecoveryRuntime) error {
	switch rt.Name {
	case "nvm":
		// Install nvm via official install script
		version := rt.Version
		if version == "" {
			version = "v0.40.1"
		}
		url := fmt.Sprintf("https://raw.githubusercontent.com/nvm-sh/nvm/%s/install.sh", version)
		cmd := exec.Command("bash", "-c", fmt.Sprintf("curl -fsSL %s | bash", url))
		cmd.Env = append(os.Environ(), "HOME=/root")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%w (output: %s)", err, string(out))
		}
		// Install node version if specified in Install field
		if rt.Install != "" {
			cmd = exec.Command("bash", "-c",
				fmt.Sprintf(`. /root/.nvm/nvm.sh && nvm install %s && nvm alias default %s`, rt.Install, rt.Install))
			cmd.Env = append(os.Environ(), "HOME=/root")
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("nvm install %s failed: %w (output: %s)", rt.Install, err, string(out))
			}
		}
	case "bun":
		// Install bun via official install script
		cmd := exec.Command("bash", "-c", "curl -fsSL https://bun.sh/install | bash")
		cmd.Env = append(os.Environ(), "HOME=/root")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%w (output: %s)", err, string(out))
		}
	case "npm-global":
		// Install npm global package, requires nvm already active
		if rt.Install == "" {
			return fmt.Errorf("npm-global requires install field (package name)")
		}
		cmd := exec.Command("bash", "-c",
			fmt.Sprintf(`. /root/.nvm/nvm.sh && npm install -g %s`, rt.Install))
		cmd.Env = append(os.Environ(), "HOME=/root")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%w (output: %s)", err, string(out))
		}
	case "shell":
		// Generic: run arbitrary install command
		if rt.Install == "" {
			return fmt.Errorf("shell runtime requires install field (command to run)")
		}
		cmd := exec.Command("bash", "-c", rt.Install)
		cmd.Env = append(os.Environ(), "HOME=/root")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%w (output: %s)", err, string(out))
		}
	default:
		return fmt.Errorf("unknown runtime: %s (supported: nvm, bun, npm-global, shell)", rt.Name)
	}
	return nil
}

func installPackages(packages []string) []string {
	var errs []string
	if len(packages) == 0 {
		return errs
	}

	// apt-get update
	cmd := exec.Command("apt-get", "update")
	if out, err := cmd.CombinedOutput(); err != nil {
		errs = append(errs, fmt.Sprintf("apt-get update failed: %v (output: %s)", err, string(out)))
		return errs
	}

	// apt-get install
	args := append([]string{"install", "-y"}, packages...)
	cmd = exec.Command("apt-get", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		errs = append(errs, fmt.Sprintf("apt-get install failed: %v (output: %s)", err, string(out)))
	}

	return errs
}

func createUsers(users []RecoveryUser) []string {
	var errs []string
	for _, user := range users {
		if err := createUser(user); err != nil {
			errs = append(errs, fmt.Sprintf("user %s creation failed: %v", user.Name, err))
		}
	}
	return errs
}

func createUser(user RecoveryUser) error {
	// Check if user exists
	_, err := exec.Command("id", user.Name).CombinedOutput()
	if err == nil {
		return nil // user already exists
	}

	// Build useradd args
	args := []string{"-s", user.Shell, "-d", user.Home}
	if user.CreateHome {
		args = append(args, "-m")
	}
	args = append(args, user.Name)

	cmd := exec.Command("useradd", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("useradd failed: %w (output: %s)", err, string(out))
	}

	// Add to groups
	for _, group := range user.Groups {
		cmd := exec.Command("usermod", "-aG", group, user.Name)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("usermod failed for group %s: %w (output: %s)", group, err, string(out))
		}
	}

	return nil
}

func restoreDatabase(db RecoveryDatabase, dumpPath string) error {
	switch db.Engine {
	case "postgresql":
		return restorePostgreSQL(db, dumpPath)
	case "mysql":
		return restoreMySQL(db, dumpPath)
	default:
		return fmt.Errorf("unsupported engine: %s", db.Engine)
	}
}

func restorePostgreSQL(db RecoveryDatabase, dumpPath string) error {
	args := []string{"-U", db.Owner, "-d", db.Database, "-f", dumpPath}
	cmd := exec.Command("psql", args...)
	cmd.Env = append(os.Environ(), "PGPASSWORD=") // ponytail: no password prompt in MVP
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("psql failed: %w (output: %s)", err, string(out))
	}
	return nil
}

func restoreMySQL(db RecoveryDatabase, dumpPath string) error {
	args := []string{"-u", db.Owner, db.Database}
	cmd := exec.Command("mysql", args...)
	dump, err := os.Open(dumpPath)
	if err != nil {
		return err
	}
	defer dump.Close()
	cmd.Stdin = dump
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mysql failed: %w (output: %s)", err, string(out))
	}
	return nil
}

func printRecoveryPlan(c Config) {
	p := recoveryPlan(c)
	fmt.Println("Recovery plan (read-only)")
	if !p.Configured {
		fmt.Println("  recovery: not configured")
		return
	}
	fmt.Printf("  apt packages: %d\n", p.Packages)
	fmt.Printf("  users: %d\n", p.Users)
	fmt.Printf("  databases: %d\n", p.Databases)
	fmt.Printf("  downloads: %d\n", p.Downloads)
	fmt.Printf("  restore staging: %s\n", c.Recovery.Restore.Staging)
	fmt.Println("  no changes made")
}

func usage() { printHelp("") }

func install(configPath string) error {
	baseDir := filepath.Dir(configPath)
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	passwordFile := filepath.Join(baseDir, "password")
	if _, err := os.Stat(passwordFile); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(passwordFile, []byte(""), 0o600); err != nil {
			return fmt.Errorf("create password file: %w", err)
		}
		fmt.Printf("created %s (empty)\n", passwordFile)
	}
	if err := os.Chmod(passwordFile, 0o600); err != nil {
		return fmt.Errorf("chmod password file: %w", err)
	}
	if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
		example := Config{
			Version:      2,
			Repositories: []RepositoryConfig{{Name: "local", URL: "local:/var/backups/restic", PasswordFile: passwordFile, Required: boolPtr(true)}},
			Repository:   RepositoryConfig{},
			Backup:       BackupConfig{Paths: []string{"/etc", "/home"}, Exclude: []string{"/proc", "/sys", "/dev", "/run", "/tmp", "/var/cache", "/var/tmp", "/mnt", "/media"}},
			Retention:    RetentionConfig{Daily: 14, Weekly: 8, Monthly: 6, Prune: false},
			Verify:       VerifyConfig{AfterBackup: true},
			Schedule:     ScheduleConfig{Enabled: true, OnCalendar: "*-*-* 02:00:00", Persistent: true, RandomizedDelay: "15m"},
		}
		data, err := yaml.Marshal(example)
		if err != nil {
			return fmt.Errorf("render config template: %w", err)
		}
		if err := os.WriteFile(configPath, data, 0o600); err != nil {
			return fmt.Errorf("write config: %w", err)
		}
		fmt.Printf("created %s\n", configPath)
	}
	fmt.Println("install complete")
	fmt.Printf("next steps:\n  1) edit %s\n  2) set password in %s\n  3) run: backup-system -config %s init\n", configPath, passwordFile, configPath)
	return nil
}
func main() {
	configPath := "/etc/backup-system/config.yml"
	args := os.Args[1:]
	if len(args) >= 2 && args[0] == "-config" {
		configPath = args[1]
		args = args[2:]
	}
	if absolute, err := filepath.Abs(configPath); err == nil {
		configPath = absolute
	}
	if len(args) == 0 || args[0] == "--help" || args[0] == "help" {
		if len(args) > 1 {
			printHelp(args[1])
		} else {
			usage()
		}
		return
	}
	command := args[0]
	if suggestion := suggestion(command); suggestion != "" && !contains(commands, command) {
		fmt.Fprintf(os.Stderr, "backup-system: unknown command %q\n\nDid you mean %q?\n\n", command, suggestion)
		usage()
		os.Exit(2)
	}
	if !contains(commands, command) {
		fmt.Fprintf(os.Stderr, "backup-system: unknown command %q\n\n", command)
		usage()
		os.Exit(2)
	}
	if len(args) > 1 && args[1] == "--help" {
		printHelp(command)
		return
	}
	if command == "version" {
		fmt.Printf("backup-system %s\n", version)
		return
	}
	if command == "update" {
		checkOnly := len(args) > 1 && args[1] == "--check"
		if err := selfUpdate(checkOnly); err != nil {
			fmt.Fprintln(os.Stderr, "backup-system:", err)
			os.Exit(1)
		}
		return
	}
	if command == "install" {
		if err := install(configPath); err != nil {
			fmt.Fprintln(os.Stderr, "backup-system:", err)
			os.Exit(1)
		}
		return
	}
	if command == "config-check" || command == "repositories" {
		c, err := loadConfig(configPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "backup-system:", err)
			os.Exit(1)
		}
		if command == "repositories" {
			for _, repo := range c.Repositories {
				required := "optional"
				if repo.isRequired() {
					required = "required"
				}
				fmt.Printf("%-16s %-9s %s\n", repo.displayName(), required, repo.URL)
			}
			return
		}
		restic, err := resticPath()
		if err != nil {
			fmt.Fprintln(os.Stderr, "backup-system:", err)
			os.Exit(1)
		}
		fmt.Println("config: OK")
		fmt.Printf("restic: OK (%s)\n", restic)
		return
	}
	if command == "schedule" && len(args) == 2 && (args[1] == "status" || args[1] == "remove") {
		var err error
		if args[1] == "status" {
			err = scheduleStatus()
		} else {
			err = scheduleRemove()
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "backup-system:", err)
			os.Exit(1)
		}
		return
	}
	c, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "backup-system:", err)
		os.Exit(1)
	}
	if command == "recovery" {
		if len(args) != 2 || (args[1] != "plan" && args[1] != "check" && args[1] != "run") {
			fmt.Fprintln(os.Stderr, "backup-system: usage: recovery <plan|check|run>")
			os.Exit(2)
		}
		if args[1] == "plan" {
			printRecoveryPlan(c)
		} else if args[1] == "check" {
			stagingPath := c.Recovery.Restore.Staging
			if stagingPath == "" {
				stagingPath = "/recovery/staging"
			}
			repoPath := strings.TrimPrefix(c.Repository.URL, "local:")
			if !strings.HasPrefix(c.Repository.URL, "local:") {
				repoPath = filepath.Dir(filepath.Dir(c.Repository.URL))
			}
			errs := recoveryCheck(c, stagingPath, repoPath)
			if len(errs) > 0 {
				fmt.Println("Recovery check FAILED:")
				for _, err := range errs {
					fmt.Printf("  ✗ %s\n", err)
				}
				os.Exit(1)
			}
			fmt.Println("Recovery check OK")
		} else {
			errs := recoveryRun(c, true)
			if len(errs) > 0 {
				fmt.Println("Recovery run FAILED:")
				for _, err := range errs {
					fmt.Printf("  ✗ %s\n", err)
				}
				os.Exit(1)
			}
			fmt.Println("Recovery run OK (staging)")
		}
		return
	}
	a := &app{cfg: c}
	lockPath := filepath.Join(filepath.Dir(configPath), ".lock")
	if command != "install" && command != "config-check" && command != "version" {
		a.lock, err = acquireLock(lockPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "backup-system:", err)
			os.Exit(1)
		}
		defer a.lock.Close()
	}
	selector := ""
	prune := false
	var selectorErr error
	if command == "retention" {
		selector, prune, selectorErr = retentionArgs(args[1:])
	} else if command != "restore" && command != "schedule" {
		selector, selectorErr = repositorySelector(args[1:])
	}
	if selectorErr != nil {
		err = selectorErr
	}
	switch command {
	case "repositories":
		if len(args) != 1 {
			err = errors.New("usage: repositories")
		}
	case "init":
		if err == nil && (len(args) != 1 && len(args) != 3) {
			err = errors.New("usage: init [--repository name]")
		} else if err == nil {
			err = runRepositories(c, selector, func(r RepositoryConfig) error { return a.runRepo(r, "init") })
		}
	case "backup":
		if err == nil && (len(args) != 1 && len(args) != 3) {
			err = errors.New("usage: backup [--repository name]")
		} else if err == nil {
			// Safety checks before backup
			if safeErr := checkSafety(c.Safety); safeErr != nil {
				err = safeErr
				break
			}

			repos, _ := selectRepositories(c, selector)

			// Auto-unlock stale locks
			unlockStaleLocks(a, repos, c.Safety.LockTimeoutMin)

			// Before hooks
			if hookErr := runHooks(c.Backup.Hooks.Before, "before"); hookErr != nil {
				err = hookErr
				_ = runHooks(c.Backup.Hooks.OnError, "on_error")
				break
			}

			start := time.Now()
			var results []BackupResult
			for _, repo := range repos {
				backupArgs := append([]string{"backup"}, a.cfg.Backup.Paths...)
				for _, p := range a.cfg.Backup.Exclude {
					backupArgs = append(backupArgs, "--exclude", p)
				}
				out, _, berr := a.runRepoCapture(repo, backupArgs...)
				sid, fn, fc, fu, added, stored, noParent := parseBackupOutput(out)
				r := BackupResult{
					Repo: repo.displayName(), Required: repo.isRequired(),
					SnapshotID: sid, FilesNew: fn, FilesChanged: fc, FilesUnchanged: fu,
					Added: added, Stored: stored, NoParent: noParent, Err: berr,
				}
				results = append(results, r)
				if berr != nil {
					fmt.Fprintf(os.Stderr, "repository %s: FAILED: %v\n", repo.displayName(), berr)
					if repo.isRequired() {
						err = fmt.Errorf("required repository %s failed", repo.displayName())
					}
				} else {
					fmt.Printf("repository %s: OK\n", repo.displayName())
				}
			}
			dur := time.Since(start)

			// After/error hooks
			if err != nil {
				_ = runHooks(c.Backup.Hooks.OnError, "on_error")
			} else {
				_ = runHooks(c.Backup.Hooks.After, "after")
			}

			if err == nil && c.Retention.Prune {
				err = runRepositories(c, selector, func(r RepositoryConfig) error { return a.retentionRepo(r, true) })
			}

			// Write status file
			writeStatusFile(c.Status.File, results, err == nil, dur)

			// Telegram notification
			if c.Notify.Telegram != nil {
				hostname, _ := os.Hostname()
				_, notifText := formatBackupNotif(results, hostname)
				if notifyErr := sendTelegramNotify(c.Notify.Telegram, err == nil, notifText); notifyErr != nil {
					fmt.Fprintf(os.Stderr, "warn: %v\n", notifyErr)
				}
			}
		}
	case "snapshots":
		if err == nil && (len(args) != 1 && len(args) != 3) {
			err = errors.New("usage: snapshots [--repository name]")
		} else if err == nil {
			err = runRepositories(c, selector, func(r RepositoryConfig) error { return a.runRepo(r, "snapshots") })
		}
	case "verify":
		if err == nil && (len(args) != 1 && len(args) != 3) {
			err = errors.New("usage: verify [--repository name]")
		} else if err == nil {
			err = runRepositories(c, selector, func(r RepositoryConfig) error { return a.runRepo(r, "check") })
		}
	case "retention":
		if err == nil && len(args) > 4 {
			err = errors.New("usage: retention [--dry-run|--prune] [--repository name]")
		} else if err == nil {
			err = runRepositories(c, selector, func(r RepositoryConfig) error { return a.retentionRepo(r, prune) })
		}
	case "restore":
		restoreSelector, positional, parseErr := restoreArgs(args[1:])
		if parseErr != nil {
			err = parseErr
		} else if len(c.Repositories) > 1 && restoreSelector == "" {
			err = errors.New("multiple repositories configured; choose one with --repository")
		} else if !filepath.IsAbs(positional[1]) {
			err = errors.New("restore target must be absolute")
		} else if err = validateRestoreTarget(positional[1]); err == nil {
			repos, selectErr := selectRepositories(c, restoreSelector)
			if selectErr != nil {
				err = selectErr
			} else {
				err = a.runRepo(repos[0], "restore", positional[0], "--target", positional[1])
			}
		}
	case "schedule":
		if len(args) != 2 {
			err = errors.New("usage: schedule <render|install|status|remove>")
		} else {
			switch args[1] {
			case "render":
				if !c.Schedule.Enabled {
					err = errors.New("schedule is disabled in config.yml")
				} else {
					binaryPath, pathErr := executablePath()
					if pathErr != nil {
						err = pathErr
					} else if calendarErr := validateCalendar(c.Schedule.OnCalendar); calendarErr != nil {
						err = calendarErr
					} else {
						fmt.Printf("# %s\n%s# %s\n%s", serviceUnitName, serviceUnit(configPath, binaryPath), timerUnitName, timerUnit(c.Schedule))
					}
				}
			case "install":
				err = scheduleInstall(c, configPath)
			case "status":
				err = scheduleStatus()
			case "remove":
				err = scheduleRemove()
			default:
				err = fmt.Errorf("unknown schedule action %q; choose render, install, status, or remove", args[1])
			}
		}
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "backup-system:", strings.TrimSpace(err.Error()))
		os.Exit(1)
	}
}
