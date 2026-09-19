package main

import (
	"os"
	"path/filepath"
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
