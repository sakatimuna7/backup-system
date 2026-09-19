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
