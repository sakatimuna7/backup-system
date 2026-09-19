package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigRejectsRelativePathsAndWeakPassword(t *testing.T) {
	d := t.TempDir()
	pw := filepath.Join(d, "password")
	if err := os.WriteFile(pw, []byte("secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(d, "config.yml")
	data := []byte("version: 1\nrepository:\n  url: local\n  password_file: " + pw + "\nbackup:\n  paths: [relative]\n")
	if err := os.WriteFile(cfg, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(cfg); err == nil {
		t.Fatal("expected relative path rejection")
	}
	if err := os.Chmod(pw, 0644); err != nil {
		t.Fatal(err)
	}
	data = []byte("version: 1\nrepository:\n  url: local\n  password_file: " + pw + "\nbackup:\n  paths: [/tmp]\n")
	if err := os.WriteFile(cfg, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(cfg); err == nil {
		t.Fatal("expected weak password rejection")
	}
}

func TestLoadConfigRejectsPasswordSymlink(t *testing.T) {
	d := t.TempDir()
	target := filepath.Join(d, "real-password")
	link := filepath.Join(d, "password")
	if err := os.WriteFile(target, []byte("secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(d, "config.yml")
	data := []byte("version: 1\nrepository:\n  url: local\n  password_file: " + link + "\nbackup:\n  paths: [/tmp]\n")
	if err := os.WriteFile(cfg, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(cfg); err == nil {
		t.Fatal("expected password symlink rejection")
	}
}

func TestAcquireLockConflicts(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".lock")
	first, err := acquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if _, err := acquireLock(path); err == nil {
		t.Fatal("expected lock conflict")
	}
}

func TestConfigAndUX(t *testing.T) {
	if version != "0.2.0" {
		t.Fatalf("unexpected version: %s", version)
	}
	missing := filepath.Join(t.TempDir(), "missing.yml")
	if _, err := loadConfig(missing); err == nil || !strings.Contains(err.Error(), "config not found") {
		t.Fatalf("unexpected missing config error: %v", err)
	}
}

func TestSuggestion(t *testing.T) {
	if got := suggestion("bakcup"); got != "backup" {
		t.Fatalf("suggestion = %q, want backup", got)
	}
	if got := suggestion("xyz"); got != "" {
		t.Fatalf("unexpected suggestion: %q", got)
	}
}

func TestContains(t *testing.T) {
	if !contains(commands, "backup") || contains(commands, "bakcup") {
		t.Fatal("contains returned unexpected result")
	}
}

func TestScheduleUnits(t *testing.T) {
	c := ScheduleConfig{Enabled: true, OnCalendar: "daily", Persistent: true, RandomizedDelay: "15m"}
	service := serviceUnit("/etc/backup-system/config.yml", "/usr/local/bin/backup-system")
	if !strings.Contains(service, `ExecStart="/usr/local/bin/backup-system" -config "/etc/backup-system/config.yml" backup`) || !strings.Contains(service, "NoNewPrivileges=true") {
		t.Fatal("service unit missing expected settings")
	}
	timer := timerUnit(c)
	for _, want := range []string{"OnCalendar=daily", "Persistent=true", "Unit=backup-system.service", "RandomizedDelaySec=15m"} {
		if !strings.Contains(timer, want) {
			t.Fatalf("timer unit missing %q", want)
		}
	}
}

func TestScheduleValidation(t *testing.T) {
	d := t.TempDir()
	pw := filepath.Join(d, "password")
	if err := os.WriteFile(pw, []byte("secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, extra := range []string{
		"schedule:\n  enabled: true\n  on_calendar: \"\"\n",
		"schedule:\n  randomized_delay: nope\n",
	} {
		cfg := filepath.Join(d, "config.yml")
		data := []byte("version: 1\nrepository:\n  url: local\n  password_file: " + pw + "\nbackup:\n  paths: [/tmp]\n" + extra)
		if err := os.WriteFile(cfg, data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadConfig(cfg); err == nil {
			t.Fatalf("expected schedule validation error for %q", extra)
		}
	}
}

func TestScheduleDisabledRenderGuard(t *testing.T) {
	if err := scheduleInstall(Config{}, "/tmp/config.yml"); err == nil || !strings.Contains(err.Error(), "schedule is disabled") {
		t.Fatalf("unexpected disabled schedule error: %v", err)
	}
}

func TestValidateRestoreTarget(t *testing.T) {
	if err := validateRestoreTarget("/"); err == nil {
		t.Fatal("expected root rejection")
	}
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "existing"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateRestoreTarget(d); err == nil {
		t.Fatal("expected non-empty target rejection")
	}
	missing := filepath.Join(t.TempDir(), "new")
	if err := validateRestoreTarget(missing); err != nil {
		t.Fatalf("expected missing target to be accepted: %v", err)
	}
}
