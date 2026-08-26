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
printf '%s\0' "$@" > "$PODMAN_CAPTURE"
EOF
chmod +x "$TEST_ROOT/bin/podman"

assert_arg() {
    local expected=$1
    grep -Fx -- "$expected" "$TEST_ROOT/args" >/dev/null || {
        printf 'missing Podman argument: %s\n' "$expected" >&2
        exit 1
    }
}

printf '%s\n' 'registry.example/dev:1' > "$TEST_ROOT/project/.late/podman-image"
(
    cd "$TEST_ROOT/project"
    PATH="$TEST_ROOT/bin:$PATH" \
        XDG_CONFIG_HOME="$TEST_ROOT/config" \
        PODMAN_CAPTURE="$TEST_ROOT/capture" \
        OPENAI_BASE_URL='http://localhost:8080' \
        late-podman --exec 'dnf install -y golang' --exec 'npm ci' -- --continue 'two words'
)

tr '\0' '\n' < "$TEST_ROOT/capture" > "$TEST_ROOT/args"
assert_arg run
assert_arg --rm
assert_arg '--network=host'
assert_arg 'label=disable'
assert_arg "type=bind,src=$TEST_ROOT/project,target=/workspace"
assert_arg 'registry.example/dev:1'
assert_arg 'OPENAI_BASE_URL=http://localhost:8080'
assert_arg $'dnf install -y golang\nnpm ci'
assert_arg --continue
assert_arg 'two words'

# Test devcontainer.json support
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
}
EOF

(
    cd "$TEST_ROOT/dc-project"
    PATH="$TEST_ROOT/bin:$PATH" \
        XDG_CONFIG_HOME="$TEST_ROOT/config" \
        PODMAN_CAPTURE="$TEST_ROOT/capture-dc" \
        late-podman --exec 'npm run build' -- --prompt 'hello'
)

tr '\0' '\n' < "$TEST_ROOT/capture-dc" > "$TEST_ROOT/args"
assert_arg 'ghcr.io/devcontainers/javascript-node:20'
assert_arg 'NODE_ENV=development'
assert_arg 'CUSTOM_PORT=4000'
assert_arg $'npm install\nnpm run build'
assert_arg --prompt
assert_arg 'hello'

# Test precedence: .late/podman-image over devcontainer.json
mkdir -p "$TEST_ROOT/dc-project/.late"
printf '%s\n' 'override.example/app:latest' > "$TEST_ROOT/dc-project/.late/podman-image"
(
    cd "$TEST_ROOT/dc-project"
    PATH="$TEST_ROOT/bin:$PATH" \
        XDG_CONFIG_HOME="$TEST_ROOT/config" \
        PODMAN_CAPTURE="$TEST_ROOT/capture-override" \
        late-podman
)
tr '\0' '\n' < "$TEST_ROOT/capture-override" > "$TEST_ROOT/args"
assert_arg 'override.example/app:latest'

# Test precedence: CLI --image over everything
(
    cd "$TEST_ROOT/dc-project"
    PATH="$TEST_ROOT/bin:$PATH" \
        XDG_CONFIG_HOME="$TEST_ROOT/config" \
        PODMAN_CAPTURE="$TEST_ROOT/capture-cli" \
        late-podman --image 'cli.example/app:1'
)
tr '\0' '\n' < "$TEST_ROOT/capture-cli" > "$TEST_ROOT/args"
assert_arg 'cli.example/app:1'

if PATH="$TEST_ROOT/bin:$PATH" HOME="$TEST_ROOT" PODMAN_CAPTURE="$TEST_ROOT/capture" \
    "$TEST_ROOT/bin/late-podman" --image 2> "$TEST_ROOT/error"; then
    echo 'expected a missing --image value to fail' >&2
    exit 1
fi
grep -F -- '--image requires an argument' "$TEST_ROOT/error" >/dev/null

echo 'late-podman tests passed'
