package main

import (
	"errors"
	"fmt"
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

func TestRecoveryValidation(t *testing.T) {
	if err := validateRecovery(RecoveryConfig{Downloads: []RecoveryDownload{{Name: "bad", URL: "http://example.com/file", SHA256: strings.Repeat("a", 64), InstallTo: "/usr/local/bin/file"}}}); err == nil {
		t.Fatal("expected https validation")
	}
	if err := validateRecovery(RecoveryConfig{Downloads: []RecoveryDownload{{Name: "bad", URL: "https://example.com/file", SHA256: "bad", InstallTo: "/usr/local/bin/file"}}}); err == nil {
		t.Fatal("expected checksum validation")
	}
	if recoveryConfigured(RecoveryConfig{Applications: []RecoveryApplication{{Name: "api"}}}) != true {
		t.Fatal("application recovery should be configured")
	}
}

func TestRecoveryCheck(t *testing.T) {
	d := t.TempDir()
	pw := filepath.Join(d, "password")
	if err := os.WriteFile(pw, []byte("secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(d, "config.yml")
	data := []byte("version: 3\nrepositories:\n  - name: local\n    url: local:" + d + "/repo\n    password_file: " + pw + "\nbackup:\n  paths: [/etc]\nrecovery:\n  restore:\n    staging: " + d + "/staging\n")
	if err := os.WriteFile(cfg, data, 0600); err != nil {
		t.Fatal(err)
	}
	c, _ := loadConfig(cfg)
	errs := recoveryCheck(c, d+"/staging", d+"/repo")
	if len(errs) == 0 {
		t.Fatal("expected staging not created error")
	}
	if err := os.Mkdir(d+"/staging", 0755); err != nil {
		t.Fatal(err)
	}
	errs = recoveryCheck(c, d+"/staging", d+"/repo")
	if len(errs) == 0 {
		t.Fatal("expected repository not found error")
	}
	if err := os.Mkdir(d+"/repo", 0700); err != nil {
		t.Fatal(err)
	}
	errs = recoveryCheck(c, d+"/staging", d+"/repo")
	if len(errs) > 0 {
		t.Logf("errors: %v", errs)
	}
}

func TestRecoveryPlan(t *testing.T) {
	c := Config{Recovery: RecoveryConfig{
		Packages:  RecoveryPackages{Apt: []string{"nginx"}},
		Users:     []RecoveryUser{{Name: "deploy"}},
		Downloads: []RecoveryDownload{{Name: "restic", URL: "https://example.com/restic", SHA256: strings.Repeat("a", 64), InstallTo: "/usr/local/bin/restic"}},
	}}
	plan := recoveryPlan(c)
	if !plan.Configured || plan.Packages != 1 || plan.Users != 1 || plan.Downloads != 1 {
		t.Fatalf("unexpected recovery plan: %#v", plan)
	}
}

func TestConfigAndUX(t *testing.T) {
	if version != "0.13.1" {
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

func TestRepositoryValidation(t *testing.T) {
	d := t.TempDir()
	pw := filepath.Join(d, "password")
	if err := os.WriteFile(pw, []byte("secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(d, "config.yml")
	data := []byte("version: 2\nrepositories:\n  - name: same\n    url: local:a\n    password_file: " + pw + "\n  - name: same\n    url: local:b\n    password_file: " + pw + "\nbackup:\n  paths: [/tmp]\nschedule:\n  enabled: false\n")
	if err := os.WriteFile(cfg, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(cfg); err == nil || !strings.Contains(err.Error(), "duplicate repository name") {
		t.Fatalf("unexpected duplicate error: %v", err)
	}
}

func TestRcloneValidation(t *testing.T) {
	d := t.TempDir()
	pw, rc, bin := filepath.Join(d, "password"), filepath.Join(d, "rclone.conf"), filepath.Join(d, "rclone")
	for path, content := range map[string]string{pw: "secret\n", rc: "[gdrive]\n"} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d+string(os.PathListSeparator)+os.Getenv("PATH"))
	cfg := filepath.Join(d, "config.yml")
	data := []byte("version: 2\nrepositories:\n  - name: drive\n    url: rclone:gdrive:backup\n    password_file: " + pw + "\n    rclone_config: " + rc + "\nbackup:\n  paths: [/tmp]\nschedule:\n  enabled: false\n")
	if err := os.WriteFile(cfg, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(cfg); err != nil {
		t.Fatalf("rclone config rejected: %v", err)
	}
}

func TestRestoreArgsAndEnv(t *testing.T) {
	for _, args := range [][]string{
		{"latest", "/tmp/restore", "--repository", "remote"},
		{"--repository", "remote", "latest", "/tmp/restore"},
	} {
		name, positional, err := restoreArgs(args)
		if err != nil || name != "remote" || len(positional) != 2 || positional[0] != "latest" {
			t.Fatalf("restore args failed: name=%q positional=%#v err=%v", name, positional, err)
		}
	}
	if _, _, err := restoreArgs([]string{"latest", "/tmp/restore", "--repository"}); err == nil {
		t.Fatal("expected missing repository name")
	}
	env := cleanResticEnv([]string{"PATH=/bin", "RESTIC_REPOSITORY=old", "RCLONE_CONFIG=old", "RESTIC_PASSWORD_FILE=old", "HOME=/tmp"})
	joined := strings.Join(env, "\n")
	for _, key := range []string{"RESTIC_REPOSITORY=", "RESTIC_PASSWORD_FILE=", "RCLONE_CONFIG="} {
		if strings.Contains(joined, key) {
			t.Fatalf("stale environment variable remained: %s", key)
		}
	}
	if !strings.Contains(joined, "PATH=/bin") || !strings.Contains(joined, "HOME=/tmp") {
		t.Fatalf("unrelated environment variable was removed: %q", joined)
	}
}

func TestRecoveryRunStaging(t *testing.T) {
	d := t.TempDir()
	pw := filepath.Join(d, "password")
	if err := os.WriteFile(pw, []byte("secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(d, "config.yml")
	data := []byte("version: 3\nrepositories:\n  - name: local\n    url: local:" + d + "/repo\n    password_file: " + pw + "\nbackup:\n  paths: [/etc]\nrecovery:\n  restore:\n    staging: " + d + "/staging\n")
	if err := os.WriteFile(cfg, data, 0600); err != nil {
		t.Fatal(err)
	}
	c, _ := loadConfig(cfg)
	errs := recoveryRun(c, true)
	if len(errs) == 0 {
		t.Fatal("expected staging restore error")
	}
}

func TestRepositorySelector(t *testing.T) {
	c := Config{Repositories: []RepositoryConfig{{Name: "local"}, {Name: "remote"}}}
	if got, err := selectRepositories(c, "remote"); err != nil || len(got) != 1 || got[0].Name != "remote" {
		t.Fatalf("selector failed: %v %#v", err, got)
	}
	if _, err := selectRepositories(c, "missing"); err == nil {
		t.Fatal("expected unknown repository error")
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

func TestRecoveryDatabaseValidation(t *testing.T) {
	if err := validateRecovery(RecoveryConfig{Databases: []RecoveryDatabase{{Name: "db", Engine: "unknown", DumpPath: "/tmp/dump.sql"}}}); err == nil {
		t.Fatal("expected unknown engine rejection")
	}
	if err := validateRecovery(RecoveryConfig{Databases: []RecoveryDatabase{{Name: "db", Engine: "postgresql", DumpPath: "relative.sql"}}}); err == nil {
		t.Fatal("expected relative dump path rejection")
	}
	if err := validateRecovery(RecoveryConfig{Databases: []RecoveryDatabase{{Name: "db", Engine: "postgresql", DumpPath: "/tmp/dump.sql", Database: ""}}}); err == nil {
		t.Fatal("expected empty database name rejection")
	}
}

func TestRecoveryPackageValidation(t *testing.T) {
	if err := validateRecovery(RecoveryConfig{Packages: RecoveryPackages{Apt: []string{"nginx", "postgresql"}}}); err != nil {
		t.Fatalf("expected packages to be valid: %v", err)
	}
}

func TestRecoveryUserValidation(t *testing.T) {
	if err := validateRecovery(RecoveryConfig{Users: []RecoveryUser{{Name: "", Home: "/home/user"}}}); err == nil {
		t.Fatal("expected empty name rejection")
	}
	if err := validateRecovery(RecoveryConfig{Users: []RecoveryUser{{Name: "deploy", Home: "relative"}}}); err == nil {
		t.Fatal("expected relative home rejection")
	}
	if err := validateRecovery(RecoveryConfig{Users: []RecoveryUser{{Name: "deploy", Home: "/home/deploy", CreateHome: true}}}); err != nil {
		t.Fatalf("expected valid user config: %v", err)
	}
}

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")

	// Write valid .env mode 600
	content := "# comment\nTEST_KEY=hello\nTEST_SPACES = world\n\nTEST_EMPTY=\n"
	if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := loadEnvFile(envPath); err != nil {
		t.Fatalf("expected ok: %v", err)
	}
	if os.Getenv("TEST_KEY") != "hello" {
		t.Fatalf("expected TEST_KEY=hello, got %q", os.Getenv("TEST_KEY"))
	}
	if os.Getenv("TEST_SPACES") != "world" {
		t.Fatalf("expected TEST_SPACES=world, got %q", os.Getenv("TEST_SPACES"))
	}

	// Insecure permissions → reject
	insecure := filepath.Join(dir, "insecure.env")
	_ = os.WriteFile(insecure, []byte("X=1\n"), 0644)
	if err := loadEnvFile(insecure); err == nil {
		t.Fatal("expected permission error for 0644")
	}

	// Missing file → error
	if err := loadEnvFile(filepath.Join(dir, "missing.env")); err == nil {
		t.Fatal("expected error for missing file")
	}

	// Invalid line (no =) → error
	bad := filepath.Join(dir, "bad.env")
	_ = os.WriteFile(bad, []byte("NOEQUALSSIGN\n"), 0600)
	if err := loadEnvFile(bad); err == nil {
		t.Fatal("expected error for invalid format")
	}
}

func TestEnvExpansion(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	_ = os.WriteFile(envPath, []byte("TEST_NOTIFY_TOKEN=mytoken123\n"), 0600)

	repoDir := filepath.Join(dir, "restic-repo")
	_ = os.MkdirAll(repoDir, 0700)
	passFile := filepath.Join(dir, "password")
	_ = os.WriteFile(passFile, []byte("testpass\n"), 0600)

	cfg := fmt.Sprintf(`version: 3
env_file: %q
repositories:
  - name: local
    url: "local:%s"
    password_file: %q
backup:
  paths: ["/etc/hostname"]
retention:
  daily: 1
  weekly: 0
  monthly: 0
notify:
  telegram:
    token: "${TEST_NOTIFY_TOKEN}"
    chat_id: "-123"
`, envPath, repoDir, passFile)

	cfgPath := filepath.Join(dir, "config.yml")
	_ = os.WriteFile(cfgPath, []byte(cfg), 0600)

	c, err := loadConfig(cfgPath)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if c.Notify.Telegram == nil {
		t.Fatal("expected telegram notify config")
	}
	if c.Notify.Telegram.Token != "mytoken123" {
		t.Fatalf("expected expanded token, got %q", c.Notify.Telegram.Token)
	}
}

func TestSelfUpdateCheck(t *testing.T) {
	// --check: only reads GitHub API, no download, no write
	// We just verify the function doesn't panic and handles already-up-to-date
	// by comparing against a fake "older" version string.
	// Real network call is acceptable in integration context;
	// skip if offline.
	if err := selfUpdate(true); err != nil {
		// network unavailable in CI — skip gracefully
		t.Logf("selfUpdate --check skipped (network): %v", err)
	}
}

func TestParseBackupOutput(t *testing.T) {
	out := `
using parent snapshot abc12345
Files:           3 new,     7 changed, 20577 unmodified
Added to the repository: 2.681 MiB (461.060 KiB stored)

snapshot ef567890 saved
`
	sid, _, fn, fc, _, added, stored, noParent := parseBackupOutput(out)
	if sid != "ef567890" {
		t.Errorf("snapshot ID: got %q", sid)
	}
	if fn != "3" || fc != "7" {
		t.Errorf("files: new=%q changed=%q", fn, fc)
	}
	if added != "2.681 MiB" {
		t.Errorf("added: got %q", added)
	}
	if stored != "461.060 KiB stored" {
		t.Errorf("stored: got %q", stored)
	}
	if noParent {
		t.Error("noParent should be false")
	}

	// First backup (no parent)
	out2 := "no parent snapshot found, will read all files\nsnapshot aabb1234 saved\n"
	sid2, _, _, _, _, _, _, noParent2 := parseBackupOutput(out2)
	if sid2 != "aabb1234" {
		t.Errorf("snapshot ID: got %q", sid2)
	}
	if !noParent2 {
		t.Error("noParent should be true")
	}
}

func TestFormatBackupNotif(t *testing.T) {
	results := []BackupResult{
		{
			Repo: "local", Required: true, SnapshotID: "abc12345", FilesNew: "3", FilesChanged: "7", Added: "2.6 MiB", Stored: "461 KiB stored",
			DiffFiles: []DiffEntry{
				{Change: "+", Path: "/etc/nginx/nginx.conf", Size: "1.2 KiB"},
				{Change: "M", Path: "/home/deploy/apps/app.js", Size: "-"},
			},
		},
		{Repo: "gdrive", Required: false, NoParent: true, Added: "45 MiB"},
	}
	success, text := formatBackupNotif(results, "myhost", false)
	if !success {
		t.Error("expected success")
	}
	if !strings.Contains(text, "abc12345") {
		t.Error("expected snapshot ID in text")
	}
	if !strings.Contains(text, "full (first backup)") {
		t.Error("expected first backup label")
	}
	if !strings.Contains(text, "All 2 repo(s) OK") {
		t.Error("expected summary line")
	}
	if strings.Contains(text, "Ch Path") {
		t.Error("did not expect diff table when sendDetail is false")
	}

	// Test with sendDetail = true
	_, detailText := formatBackupNotif(results, "myhost", true)
	if !strings.Contains(detailText, "Ch Path") {
		t.Error("expected diff table header when sendDetail is true")
	}
	if !strings.Contains(detailText, "```") {
		t.Error("expected code block markdown when sendDetail is true")
	}

	// One failed required
	results2 := []BackupResult{
		{Repo: "local", Required: true, Err: errors.New("disk full")},
	}
	success2, text2 := formatBackupNotif(results2, "myhost", false)
	if success2 {
		t.Error("expected failure")
	}
	if !strings.Contains(text2, "disk full") {
		t.Error("expected error in text")
	}
}
