---
name: deploy-test-device
description: Cross-compile the goodwe-daemon Go binary for ARM and deploy it to a test Pi via SSH (configured via GOODWE_TEST_HOST env var), then restart the goodwe systemd service.
---

# Deploy to Test Device

Cross-compiles the `goodwe-daemon` binary for ARMv8, transfers it to the test
Raspberry Pi, and restarts the daemon service.

## Configuration

Set the `GOODWE_TEST_HOST` environment variable to your Pi's SSH hostname.

One-off:
```bash
export GOODWE_TEST_HOST=some-pi.tailnet.ts.net
```

Persistent (add to `~/.bashrc` or create a gitignored `.env` file):
```bash
echo 'export GOODWE_TEST_HOST=some-pi.tailnet.ts.net' >> ~/.bashrc
# or create a .env file (this repo ignores .env):
echo 'GOODWE_TEST_HOST=some-pi.tailnet.ts.net' > .env
```
Then run `.env` once before using the script:
```bash
set -a; source .env; set +a
```

## Usage

```bash
.pi/skills/deploy-test-device/deploy.sh
```

What it does:
1. `GOARCH=arm GOARM=8 go build -o goodwe-daemon-arm ./cmd/goodwe-daemon`
2. `tar -cj goodwe-daemon-arm | ssh $GOODWE_TEST_HOST 'tar xjvf -'`
3. `ssh $GOODWE_TEST_HOST 'sudo systemctl restart goodwe'`

## Prerequisites

- SSH key configured for the `$GOODWE_TEST_HOST` host
- `sudo systemctl restart goodwe` works without a password prompt
  (add to sudoers: `goodwe ALL=(ALL) NOPASSWD: /usr/bin/systemctl restart goodwe`)
- Go 1.26 toolchain with ARM cross-compilation support
