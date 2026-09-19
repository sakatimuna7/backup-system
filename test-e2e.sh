#!/bin/bash
set -e

echo "=== E2E Test: backup-system v0.7.0 ==="

TESTDIR=$(mktemp -d)
trap "rm -rf $TESTDIR" EXIT

echo "[1/10] Setup test environment..."
mkdir -p $TESTDIR/{repo,staging,data,pwd}
echo "test-password-12345" > $TESTDIR/pwd/password
chmod 600 $TESTDIR/pwd/password

echo "[2/10] Create test config..."
cat > $TESTDIR/config.yml << 'CONFIG'
version: 3
repositories:
  - name: local
    url: "local:REPO_PATH"
    password_file: "PWD_PATH"
    required: true
backup:
  paths: [DATA_PATH]
  exclude: ["/tmp"]
retention:
  daily: 1
  weekly: 0
  monthly: 0
  prune: false
verify:
  after_backup: true
recovery:
  packages:
    apt:
      - curl
  users:
    - name: testuser
      shell: "/bin/bash"
      home: "/home/testuser"
      create_home: true
      groups: [sudo]
  restore:
    staging: "STAGING_PATH"
CONFIG
sed -i "s|REPO_PATH|$TESTDIR/repo|g; s|PWD_PATH|$TESTDIR/pwd/password|g; s|DATA_PATH|$TESTDIR/data|g; s|STAGING_PATH|$TESTDIR/staging|g" $TESTDIR/config.yml

echo "[3/10] Create test data..."
mkdir -p $TESTDIR/data/etc
echo "test-backup-data" > $TESTDIR/data/etc/hostname

echo "[4/10] Test: config-check..."
./backup-system -config $TESTDIR/config.yml config-check

echo "[5/10] Test: init repository..."
./backup-system -config $TESTDIR/config.yml init

echo "[6/10] Test: create backup..."
./backup-system -config $TESTDIR/config.yml backup

echo "[7/10] Test: snapshots..."
./backup-system -config $TESTDIR/config.yml snapshots

echo "[8/10] Test: verify repository..."
./backup-system -config $TESTDIR/config.yml verify

echo "[9/10] Test: recovery plan..."
./backup-system -config $TESTDIR/config.yml recovery plan

echo "[10/10] Test: recovery check..."
./backup-system -config $TESTDIR/config.yml recovery check

echo ""
echo "✅ All E2E tests PASSED"
echo "Test artifacts: $TESTDIR"
ls -la $TESTDIR/repo/ | head -5
