package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	version       = "0.1.0"
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
	Version    int              `yaml:"version"`
	Repository RepositoryConfig `yaml:"repository"`
	Backup     BackupConfig     `yaml:"backup"`
	Retention  RetentionConfig  `yaml:"retention"`
	Verify     VerifyConfig     `yaml:"verify"`
}
type RepositoryConfig struct {
	URL          string `yaml:"url"`
	PasswordFile string `yaml:"password_file"`
}
type BackupConfig struct {
	Paths   []string `yaml:"paths"`
	Exclude []string `yaml:"exclude"`
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
	var c Config
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&c); err != nil {
		return c, fmt.Errorf("invalid config: %w", err)
	}
	if c.Version != 1 {
		return c, fmt.Errorf("unsupported config version %d (expected 1)", c.Version)
	}
	if c.Repository.URL == "" {
		return c, errors.New("repository.url is required")
	}
	if c.Repository.PasswordFile == "" {
		return c, errors.New("repository.password_file is required")
	}
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
	lst, err := os.Lstat(c.Repository.PasswordFile)
	if err != nil {
		return c, fmt.Errorf("password file: %w", err)
	}
	if lst.Mode()&os.ModeSymlink != 0 {
		return c, errors.New("password_file must not be a symlink")
	}
	if lst.IsDir() {
		return c, errors.New("password_file must be a file")
	}
	if lst.Size() == 0 {
		return c, errors.New("password_file must not be empty")
	}
	if lst.Mode().Perm()&0o077 != 0 {
		return c, errors.New("password_file must not be group/world-readable")
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
func (a *app) run(args ...string) error {
	restic, err := resticPath()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), resticTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, restic, args...)
	cmd.Env = append(os.Environ(), "RESTIC_REPOSITORY="+a.cfg.Repository.URL, "RESTIC_PASSWORD_FILE="+a.cfg.Repository.PasswordFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("restic timed out after %s; check repository connectivity and retry", resticTimeout)
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf("restic failed (exit %d): check the repository URL, password file, and restic output above", exitErr.ExitCode())
		}
		return fmt.Errorf("run restic: %w", err)
	}
	return nil
}
func (a *app) backup() error {
	args := []string{"backup"}
	args = append(args, a.cfg.Backup.Paths...)
	for _, p := range a.cfg.Backup.Exclude {
		args = append(args, "--exclude", p)
	}
	if err := a.run(args...); err != nil {
		return err
	}
	if a.cfg.Verify.AfterBackup {
		return a.run("check")
	}
	return nil
}
func (a *app) retention(prune bool) error {
	args := []string{"forget", "--keep-daily", strconv.Itoa(a.cfg.Retention.Daily), "--keep-weekly", strconv.Itoa(a.cfg.Retention.Weekly), "--keep-monthly", strconv.Itoa(a.cfg.Retention.Monthly), "--dry-run"}
	if prune {
		args[len(args)-1] = "--prune"
	}
	return a.run(args...)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: backup-system [-config path] <install|version|config-check|init|backup|snapshots|verify|retention|restore>")
}

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
			Version:    1,
			Repository: RepositoryConfig{URL: "local:/var/backups/restic", PasswordFile: passwordFile},
			Backup:     BackupConfig{Paths: []string{"/etc", "/home"}, Exclude: []string{"/proc", "/sys", "/dev", "/run", "/tmp", "/var/cache", "/var/tmp", "/mnt", "/media"}},
			Retention:  RetentionConfig{Daily: 14, Weekly: 8, Monthly: 6, Prune: false},
			Verify:     VerifyConfig{AfterBackup: true},
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
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}
	command := args[0]
	known := map[string]bool{"install": true, "version": true, "config-check": true, "init": true, "backup": true, "snapshots": true, "verify": true, "retention": true, "restore": true}
	if !known[command] {
		fmt.Fprintf(os.Stderr, "backup-system: unknown command %q\n", command)
		usage()
		os.Exit(2)
	}
	if command == "version" {
		fmt.Printf("backup-system %s\n", version)
		return
	}
	if command == "install" {
		if err := install(configPath); err != nil {
			fmt.Fprintln(os.Stderr, "backup-system:", err)
			os.Exit(1)
		}
		return
	}
	if command == "config-check" {
		if _, err := loadConfig(configPath); err != nil {
			fmt.Fprintln(os.Stderr, "backup-system:", err)
			os.Exit(1)
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
	c, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "backup-system:", err)
		os.Exit(1)
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
	switch command {
	case "init":
		if len(args) != 1 {
			err = errors.New("usage: init")
		} else {
			err = a.run("init")
		}
	case "backup":
		if len(args) != 1 {
			err = errors.New("usage: backup")
		} else {
			err = a.backup()
			if err == nil && c.Retention.Prune {
				err = a.retention(true)
			}
		}
	case "snapshots":
		if len(args) != 1 {
			err = errors.New("usage: snapshots")
		} else {
			err = a.run("snapshots")
		}
	case "verify":
		if len(args) != 1 {
			err = errors.New("usage: verify")
		} else {
			err = a.run("check")
		}
	case "retention":
		if len(args) > 2 || (len(args) == 2 && args[1] != "--prune" && args[1] != "--dry-run") {
			err = errors.New("usage: retention [--dry-run|--prune]")
		} else {
			err = a.retention(len(args) == 2 && args[1] == "--prune")
		}
	case "restore":
		if len(args) != 3 {
			err = errors.New("usage: restore <snapshot> <target>")
		} else if !filepath.IsAbs(args[2]) {
			err = errors.New("restore target must be absolute")
		} else if err = validateRestoreTarget(args[2]); err == nil {
			err = a.run("restore", args[1], "--target", args[2])
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
