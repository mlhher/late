#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT

mkdir -p "$TEST_ROOT/bin" "$TEST_ROOT/project/.late" "$TEST_ROOT/config/late"
cp "$ROOT/late-podman" "$TEST_ROOT/bin/late-podman"
printf '#!/usr/bin/env bash\nexit 0\n' > "$TEST_ROOT/bin/late"
chmod +x "$TEST_ROOT/bin/late" "$TEST_ROOT/bin/late-podman"

cat > "$TEST_ROOT/bin/podman" <<'EOF'
#!/usr/bin/env bash
if [[ "${1:-}" == "image" && "${2:-}" == "exists" ]]; then
    if [[ -f "$TEST_ROOT/mock-image-exists" ]]; then
        exit 0
    fi
    exit 1
fi
if [[ "${1:-}" == "build" ]]; then
    printf '%s\0' "$@" > "$PODMAN_BUILD_CAPTURE"
    touch "$TEST_ROOT/mock-image-exists"
    exit 0
fi
printf '%s\0' "$@" > "$PODMAN_CAPTURE"
EOF
chmod +x "$TEST_ROOT/bin/podman"

assert_arg() {
    local expected=$1
    local file="${2:-$TEST_ROOT/args}"
    grep -Fx -- "$expected" "$file" >/dev/null || {
        printf 'missing Podman argument: %s in %s\n' "$expected" "$file" >&2
        exit 1
    }
}

refute_arg() {
    local unexpected=$1
    local file="${2:-$TEST_ROOT/args}"
    if grep -Fx -- "$unexpected" "$file" >/dev/null; then
        printf 'unexpected Podman argument found: %s in %s\n' "$unexpected" "$file" >&2
        exit 1
    fi
}

# Test 1: Basic project with .late/podman-image and --exec
printf '%s\n' 'registry.example/dev:1' > "$TEST_ROOT/project/.late/podman-image"
(
    cd "$TEST_ROOT/project"
    PATH="$TEST_ROOT/bin:$PATH" \
        TEST_ROOT="$TEST_ROOT" \
        XDG_CONFIG_HOME="$TEST_ROOT/config" \
        PODMAN_CAPTURE="$TEST_ROOT/capture" \
        PODMAN_BUILD_CAPTURE="$TEST_ROOT/capture-build" \
        OPENAI_BASE_URL='http://localhost:8080' \
        late-podman --exec 'dnf install -y golang' --exec 'npm ci' -- --continue 'two words'
)

tr '\0' '\n' < "$TEST_ROOT/capture" > "$TEST_ROOT/args"
assert_arg run
assert_arg --rm
assert_arg '--network=host'
assert_arg 'label=disable'
assert_arg "type=bind,src=$TEST_ROOT/project,target=/workspace"
assert_arg "late-podman-cache-project:/root/.cache"
assert_arg 'registry.example/dev:1'
assert_arg 'OPENAI_BASE_URL=http://localhost:8080'
assert_arg $'dnf install -y golang\nnpm ci'
assert_arg --continue
assert_arg 'two words'
grep -F -- '--i-promise-i-have-backups-and-will-not-file-issues' "$TEST_ROOT/args" >/dev/null || {
    echo "expected container script to execute late with --i-promise-i-have-backups-and-will-not-file-issues" >&2
    exit 1
}

# Test 2: devcontainer.json image, containerEnv, postCreateCommand on first run
mkdir -p "$TEST_ROOT/dc-project/.devcontainer"
cat > "$TEST_ROOT/dc-project/.devcontainer/devcontainer.json" <<'EOF'
{
  "name": "Node Dev Container",
  // Image with comment
  "image": "ghcr.io/devcontainers/javascript-node:20",
  "containerEnv": {
    "NODE_ENV": "development",
    "CUSTOM_PORT": "4000",
  },
  "postCreateCommand": [
    "npm",
    "install"
  ],
  "postStartCommand": "echo started"
}
EOF

(
    cd "$TEST_ROOT/dc-project"
    PATH="$TEST_ROOT/bin:$PATH" \
        TEST_ROOT="$TEST_ROOT" \
        XDG_CONFIG_HOME="$TEST_ROOT/config" \
        PODMAN_CAPTURE="$TEST_ROOT/capture-dc-1" \
        PODMAN_BUILD_CAPTURE="$TEST_ROOT/capture-build" \
        late-podman --exec 'npm run build' -- --prompt 'hello'
)

tr '\0' '\n' < "$TEST_ROOT/capture-dc-1" > "$TEST_ROOT/args-dc-1"
assert_arg 'ghcr.io/devcontainers/javascript-node:20' "$TEST_ROOT/args-dc-1"
assert_arg 'NODE_ENV=development' "$TEST_ROOT/args-dc-1"
assert_arg 'CUSTOM_PORT=4000' "$TEST_ROOT/args-dc-1"
assert_arg 'npm install' "$TEST_ROOT/args-dc-1"
assert_arg $'echo started\nnpm run build' "$TEST_ROOT/args-dc-1"
assert_arg --prompt "$TEST_ROOT/args-dc-1"
assert_arg 'hello' "$TEST_ROOT/args-dc-1"

# Test 3: Second run with post-create-hash already present -> skips postCreateCommand
mkdir -p "$TEST_ROOT/dc-project/.late"
# Calculate hash of "npm install"
hash_val=$(printf 'npm install' | sha256sum | awk '{print $1}')
printf '%s\n' "$hash_val" > "$TEST_ROOT/dc-project/.late/post-create-hash"

(
    cd "$TEST_ROOT/dc-project"
    PATH="$TEST_ROOT/bin:$PATH" \
        TEST_ROOT="$TEST_ROOT" \
        XDG_CONFIG_HOME="$TEST_ROOT/config" \
        PODMAN_CAPTURE="$TEST_ROOT/capture-dc-2" \
        PODMAN_BUILD_CAPTURE="$TEST_ROOT/capture-build" \
        late-podman --exec 'npm run build' -- --prompt 'hello'
)

tr '\0' '\n' < "$TEST_ROOT/capture-dc-2" > "$TEST_ROOT/args-dc-2"
refute_arg 'npm install' "$TEST_ROOT/args-dc-2"
assert_arg $'echo started\nnpm run build' "$TEST_ROOT/args-dc-2"

# Test 4: Third run with --rebuild -> forces postCreateCommand to run again
(
    cd "$TEST_ROOT/dc-project"
    PATH="$TEST_ROOT/bin:$PATH" \
        TEST_ROOT="$TEST_ROOT" \
        XDG_CONFIG_HOME="$TEST_ROOT/config" \
        PODMAN_CAPTURE="$TEST_ROOT/capture-dc-3" \
        PODMAN_BUILD_CAPTURE="$TEST_ROOT/capture-build" \
        late-podman --rebuild --exec 'npm run build' -- --prompt 'hello'
)

tr '\0' '\n' < "$TEST_ROOT/capture-dc-3" > "$TEST_ROOT/args-dc-3"
assert_arg 'npm install' "$TEST_ROOT/args-dc-3"

# Test 5: Dockerfile devcontainer build and caching
mkdir -p "$TEST_ROOT/build-project/.devcontainer"
cat > "$TEST_ROOT/build-project/.devcontainer/devcontainer.json" <<'EOF'
{
  "name": "Build Dev Container",
  "build": {
    "dockerfile": "Dockerfile",
    "context": ".."
  }
}
EOF
cat > "$TEST_ROOT/build-project/.devcontainer/Dockerfile" <<'EOF'
FROM alpine:latest
RUN apk add --no-cache bash
EOF

# 5a: First run: image doesn't exist -> builds image
rm -f "$TEST_ROOT/mock-image-exists"
rm -f "$TEST_ROOT/capture-build"
(
    cd "$TEST_ROOT/build-project"
    PATH="$TEST_ROOT/bin:$PATH" \
        TEST_ROOT="$TEST_ROOT" \
        XDG_CONFIG_HOME="$TEST_ROOT/config" \
        PODMAN_CAPTURE="$TEST_ROOT/capture-build-run-1" \
        PODMAN_BUILD_CAPTURE="$TEST_ROOT/capture-build-1" \
        late-podman
)

[[ -f "$TEST_ROOT/capture-build-1" ]] || {
    echo "expected podman build to be called on first run" >&2
    exit 1
}
tr '\0' '\n' < "$TEST_ROOT/capture-build-1" > "$TEST_ROOT/args-build-1"
assert_arg build "$TEST_ROOT/args-build-1"
assert_arg 'localhost/late-devcontainer-build-project:latest' "$TEST_ROOT/args-build-1"

# 5b: Second run: image exists & hash matches -> skips build
rm -f "$TEST_ROOT/capture-build-2"
(
    cd "$TEST_ROOT/build-project"
    PATH="$TEST_ROOT/bin:$PATH" \
        TEST_ROOT="$TEST_ROOT" \
        XDG_CONFIG_HOME="$TEST_ROOT/config" \
        PODMAN_CAPTURE="$TEST_ROOT/capture-build-run-2" \
        PODMAN_BUILD_CAPTURE="$TEST_ROOT/capture-build-2" \
        late-podman
)
if [[ -f "$TEST_ROOT/capture-build-2" ]]; then
    echo "podman build should have been skipped on second run" >&2
    exit 1
fi

# 5c: Third run with --rebuild -> forces build with --no-cache
(
    cd "$TEST_ROOT/build-project"
    PATH="$TEST_ROOT/bin:$PATH" \
        TEST_ROOT="$TEST_ROOT" \
        XDG_CONFIG_HOME="$TEST_ROOT/config" \
        PODMAN_CAPTURE="$TEST_ROOT/capture-build-run-3" \
        PODMAN_BUILD_CAPTURE="$TEST_ROOT/capture-build-3" \
        late-podman --rebuild
)
[[ -f "$TEST_ROOT/capture-build-3" ]] || {
    echo "expected podman build to be called on --rebuild" >&2
    exit 1
}
tr '\0' '\n' < "$TEST_ROOT/capture-build-3" > "$TEST_ROOT/args-build-3"
assert_arg --no-cache "$TEST_ROOT/args-build-3"

# Test 6: Precedence: .late/podman-image over devcontainer.json
mkdir -p "$TEST_ROOT/dc-project/.late"
printf '%s\n' 'override.example/app:latest' > "$TEST_ROOT/dc-project/.late/podman-image"
(
    cd "$TEST_ROOT/dc-project"
    PATH="$TEST_ROOT/bin:$PATH" \
        TEST_ROOT="$TEST_ROOT" \
        XDG_CONFIG_HOME="$TEST_ROOT/config" \
        PODMAN_CAPTURE="$TEST_ROOT/capture-override" \
        PODMAN_BUILD_CAPTURE="$TEST_ROOT/capture-build" \
        late-podman
)
tr '\0' '\n' < "$TEST_ROOT/capture-override" > "$TEST_ROOT/args"
assert_arg 'override.example/app:latest'

# Test 7: Precedence: CLI --image over everything
(
    cd "$TEST_ROOT/dc-project"
    PATH="$TEST_ROOT/bin:$PATH" \
        TEST_ROOT="$TEST_ROOT" \
        XDG_CONFIG_HOME="$TEST_ROOT/config" \
        PODMAN_CAPTURE="$TEST_ROOT/capture-cli" \
        PODMAN_BUILD_CAPTURE="$TEST_ROOT/capture-build" \
        late-podman --image 'cli.example/app:1'
)
tr '\0' '\n' < "$TEST_ROOT/capture-cli" > "$TEST_ROOT/args"
assert_arg 'cli.example/app:1'

# Test 8: Error handling on missing argument
if PATH="$TEST_ROOT/bin:$PATH" HOME="$TEST_ROOT" TEST_ROOT="$TEST_ROOT" \
    PODMAN_CAPTURE="$TEST_ROOT/capture" PODMAN_BUILD_CAPTURE="$TEST_ROOT/capture-build" \
    "$TEST_ROOT/bin/late-podman" --image 2> "$TEST_ROOT/error"; then
    echo 'expected a missing --image value to fail' >&2
    exit 1
fi
grep -F -- '--image requires an argument' "$TEST_ROOT/error" >/dev/null

# Test 9: CLI -e, --env, remoteEnv, and TZ pass-through
mkdir -p "$TEST_ROOT/env-project/.devcontainer"
cat > "$TEST_ROOT/env-project/.devcontainer/devcontainer.json" <<'EOF'
{
  "name": "Env Dev Container",
  "image": "registry.example/env:1",
  "remoteEnv": {
    "DEVCONTAINER_REMOTE_VAR": "remote_value"
  }
}
EOF

(
    cd "$TEST_ROOT/env-project"
    MY_HOST_VAR="host_value" \
    TZ="Europe/Berlin" \
    PATH="$TEST_ROOT/bin:$PATH" \
        TEST_ROOT="$TEST_ROOT" \
        XDG_CONFIG_HOME="$TEST_ROOT/config" \
        PODMAN_CAPTURE="$TEST_ROOT/capture-env" \
        PODMAN_BUILD_CAPTURE="$TEST_ROOT/capture-build" \
        late-podman -e CUSTOM_EXPLICIT=explicit_val -e MY_HOST_VAR --env=ANOTHER_VAR=another_val
)

tr '\0' '\n' < "$TEST_ROOT/capture-env" > "$TEST_ROOT/args-env"
assert_arg 'DEVCONTAINER_REMOTE_VAR=remote_value' "$TEST_ROOT/args-env"
assert_arg 'CUSTOM_EXPLICIT=explicit_val' "$TEST_ROOT/args-env"
assert_arg 'MY_HOST_VAR=host_value' "$TEST_ROOT/args-env"
assert_arg 'ANOTHER_VAR=another_val' "$TEST_ROOT/args-env"
assert_arg 'TZ=Europe/Berlin' "$TEST_ROOT/args-env"

# Test 10: Git identity, safe.directory, and SSH agent socket pass-through
python3 -c "import socket; s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM); s.bind('$TEST_ROOT/ssh-mock.sock')"
cat > "$TEST_ROOT/mock-gitconfig" <<'EOF'
[user]
	name = Test Committer
	email = committer@example.com
	signingkey = ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGitKey
[gpg]
	format = ssh
[commit]
	gpgsign = true
EOF

(
    cd "$TEST_ROOT/env-project"
    SSH_AUTH_SOCK="$TEST_ROOT/ssh-mock.sock" \
    GIT_CONFIG_GLOBAL="$TEST_ROOT/mock-gitconfig" \
    PATH="$TEST_ROOT/bin:$PATH" \
        TEST_ROOT="$TEST_ROOT" \
        XDG_CONFIG_HOME="$TEST_ROOT/config" \
        PODMAN_CAPTURE="$TEST_ROOT/capture-git-ssh" \
        PODMAN_BUILD_CAPTURE="$TEST_ROOT/capture-build" \
        late-podman
)

tr '\0' '\n' < "$TEST_ROOT/capture-git-ssh" > "$TEST_ROOT/args-git-ssh"
assert_arg 'GIT_AUTHOR_NAME=Test Committer' "$TEST_ROOT/args-git-ssh"
assert_arg 'GIT_AUTHOR_EMAIL=committer@example.com' "$TEST_ROOT/args-git-ssh"
assert_arg "type=bind,src=$TEST_ROOT/ssh-mock.sock,target=/tmp/ssh-agent.sock" "$TEST_ROOT/args-git-ssh"
assert_arg 'SSH_AUTH_SOCK=/tmp/ssh-agent.sock' "$TEST_ROOT/args-git-ssh"

grep -F -- 'git config --global user.name' "$TEST_ROOT/args-git-ssh" >/dev/null || {
    echo "expected git config --global user.name in container args" >&2
    exit 1
}
grep -F -- 'git config --global gpg.format ssh' "$TEST_ROOT/args-git-ssh" >/dev/null || {
    echo "expected git config --global gpg.format ssh in container args" >&2
    exit 1
}
grep -F -- 'git config --global --add safe.directory /workspace' "$TEST_ROOT/args-git-ssh" >/dev/null || {
    echo "expected safe.directory in container args" >&2
    exit 1
}

echo 'late-podman tests passed'
