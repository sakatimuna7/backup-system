package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"gopkg.in/yaml.v3"
)

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
	Daily, Weekly, Monthly int
	Prune                  bool
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
		return Config{}, err
	}
	var c Config
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&c); err != nil {
		return c, fmt.Errorf("invalid config: %w", err)
	}
	if c.Version != 1 {
		return c, fmt.Errorf("version must be 1")
	}
	if c.Repository.URL == "" || c.Repository.PasswordFile == "" {
		return c, errors.New("repository.url and repository.password_file are required")
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
	st, err := os.Stat(c.Repository.PasswordFile)
	if err != nil {
		return c, fmt.Errorf("password file: %w", err)
	}
	if st.IsDir() {
		return c, errors.New("password_file must be a file")
	}
	if st.Mode().Perm()&0o077 != 0 {
		return c, errors.New("password_file must not be group/world-readable")
	}
	return c, nil
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
	cmd := exec.Command("restic", args...)
	cmd.Env = append(os.Environ(), "RESTIC_REPOSITORY="+a.cfg.Repository.URL, "RESTIC_PASSWORD_FILE="+a.cfg.Repository.PasswordFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
	fmt.Fprintln(os.Stderr, "usage: backup-system [-config path] <init|backup|snapshots|verify|retention|restore>")
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
	c, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "backup-system:", err)
		os.Exit(1)
	}
	a := &app{cfg: c}
	lockPath := filepath.Join(filepath.Dir(configPath), ".lock")
	if command != "init" {
		a.lock, err = acquireLock(lockPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "backup-system:", err)
			os.Exit(1)
		}
		defer a.lock.Close()
	}
	switch command {
	case "init":
		err = a.run("init")
	case "backup":
		err = a.backup()
		if err == nil && c.Retention.Prune {
			err = a.retention(true)
		}
	case "snapshots":
		err = a.run("snapshots")
	case "verify":
		err = a.run("check")
	case "retention":
		prune := len(args) > 1 && args[1] == "--prune"
		err = a.retention(prune)
	case "restore":
		if len(args) < 3 {
			err = errors.New("usage: restore <snapshot> <target>")
		} else if !filepath.IsAbs(args[2]) {
			err = errors.New("restore target must be absolute")
		} else {
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
