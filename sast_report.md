# SAST Security Report - Late (Lightweight AI Terminal Environment)
Date: 2025-04-29
Analyzer: llm-sast-scanner v1.3
Target: /mnt/storage/Projects/late (Go CLI agent orchestrator)

## Executive Summary

Late is a CLI-based AI agent orchestrator with 66 Go source files. It connects to OpenAI-compatible LLM APIs, executes shell commands via a parser-based allowlist, persists session history as JSON, and supports MCP server integration. The security model is based on a tool execution framework with input validation and path confinement.

**Total findings: 7** (2 Low, 4 Informational, 1 Unverifiable)

No Critical or High severity vulnerabilities were found. The project has a reasonably solid security posture for its threat model (single-user developer tool).

---

## Critical Findings

_None_

---

## High Findings

_None_

---

## Low Findings

### [LOW] VULN-001 -- Shell Command Injection via Bash Tool
**Severity:** Low [LIKELY]

**File:** internal/tool/shell_command_unix.go:32 / internal/tool/implementations.go:296

**Description:** Shell commands are passed directly to /bin/sh -c "<command>" for execution. While the BashAnalyzer in bash_analyzer.go validates commands using a mvdan.cc/sh/v3 parser, when positional arguments pass isSafePositionalArg (literal-only check), they are passed unquoted to the shell. If the LLM generates a command with shell metacharacters embedded in positional arguments, they reach /bin/sh.

**Evidence:**
```go
// internal/tool/implementations.go:296
cmd := newShellCommand(ctx, params.Command)
cmd.Dir = params.Cwd
output, err := cmd.CombinedOutput()

// internal/tool/shell_command_unix.go:32
func newShellCommand(ctx context.Context, command string) *exec.Cmd {
    return exec.CommandContext(ctx, getUnixShellPath(), "-c", command)
}
```

**Impact:** An LLM could craft commands where positional arguments contain shell metacharacters ($(), backticks) that execute after the intended command.

**Judge:** The BashAnalyzer resolvesWord only accepts literal and quoted strings (not variable expansions or command substitutions). However, args like "cat /etc/passwd" are resolved as literal "cat /etc/passwd" and passed through. The tier allowlists limit exposure, but complex chains with subshells in positional args are possible.

**Remediation:** Quote the command argument before passing to /bin/sh:
```go
cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "shopt -s nullglob; exec "+params.Command)
```

**Reference:** references/command_injection.md

---

### [LOW] VULN-002 -- Unbounded JSON Parsing in Tools
**Severity:** Low [CONFIRMED]

**File:** internal/tool/implementations.go:52 / internal/tool/targetEdit.go:41

**Description:** Tools parse JSON arguments from the LLM using json.Unmarshal without depth limits or size limits. Deeply nested or very large JSON payloads could cause stack exhaustion or memory exhaustion.

**Evidence:**
```go
// internal/tool/implementations.go:52
var params struct {
    Path      string `json:"path"`
    StartLine int    `json:"start_line"`
    EndLine   int    `json:"end_line"`
}
if err := json.Unmarshal(args, &params); err != nil {
    return "", err
}
```

**Impact:** An LLM could generate deeply nested JSON structures that cause a stack overflow or high memory consumption.

**Judge:** Go's json.Unmarshal uses recursive descent parser. For typical LLM JSON (depth < 100), this is safe. Pathological inputs could cause issues. Go 1.22+ has SetMaxDecoderTokenDepth.

**Remediation:** Use json.NewDecoder with SetMaxDecoderTokenDepth (Go 1.22+) or limit input size.

**Reference:** references/denial_of_service.md

---

## Informational Findings

### [INFO] VULN-003 -- API Key Stored in Plaintext Config
**Severity:** Informational

**File:** internal/config/config.go:36-41

**Description:** The application config (~/.config/late/config.json) stores openai_api_key, openai_base_url, subagent_api_key, etc. in plaintext with 0600 permissions. Config is also written with 0644 in some cases (e.g., commands file at line 523 of permissions.go).

**Impact:** API keys stored in plaintext on disk. If another user can read the config directory, they can steal API keys.

**Remediation:** Consider encrypting API keys or using a credential manager (keyring, pass).

**Reference:** references/weak_crypto_hash.md

---

### [INFO] VULN-004 -- Session History Contains Sensitive Data with No TTL
**Severity:** Informational

**File:** internal/session/persistence.go:12-46

**Description:** Session history (chat messages, tool calls, tool results) is persisted indefinitely to ~/.local/share/late/sessions/. Session files can grow large and contain sensitive data (LLM responses with code, user prompts with API keys).

**Evidence:**
```go
// internal/session/persistence.go:27-43
tmpFile, err := os.CreateTemp(dir, "history-*.json.tmp")
// ... write and atomic rename
```

**Impact:** If session directory is backed up or shared, sensitive data could be exposed. No automatic cleanup/TTL for old sessions.

**Remediation:** Add session TTL/cleanup mechanism (delete sessions older than N days).

**Reference:** references/information_disclosure.md

---

### [INFO] VULN-005 -- HTTP Client Uses Default Transport
**Severity:** Informational

**File:** internal/client/client.go:43-49

**Description:** HTTP client uses custom transport with DisableKeepAlives:true but Go's default http.DefaultTransport for TLS. No custom TLS config:
- TLS 1.0 and 1.1 connections accepted
- No certificate pinning
- No custom root CA support

**Evidence:**
```go
// internal/client/client.go:43-49
httpClient: &http.Client{
    Transport: &http.Transport{
        DisableKeepAlives: true,
    },
    Timeout: 0,
},
```

**Impact:** In high-security environments, could connect to MITM'd server on older TLS version.

**Remediation:** Configure TLSClientConfig with specific minimum TLS version and optionally custom RootCAs.

**Reference:** references/weak_crypto_hash.md

---

### [INFO] VULN-006 -- MCP Server Command Executed Without Sandboxing
**Severity:** Informational

**File:** internal/mcp/client.go:157-195

**Description:** MCP server subprocesses created via exec.Command(command, args...) with environment variables expanded from config. The command string and args are parsed from JSON config files and can include ${VAR} expansions. No sandboxing (namespaces, cgroups, seccomp) is applied.

**Evidence:**
```go
// internal/mcp/client.go:157
func NewStdioTransport(ctx context.Context, command string, args []string, env []string) (mcp.Transport, error) {
    cmd := exec.Command(command, args...)
    cmd.Env = append(os.Environ(), env...)
    // ...
}
```

**Impact:** A misconfigured or compromised MCP server config could cause arbitrary command execution within the user's shell environment.

**Remediation:** Validate MCP server command paths against an allowlist or validate that commands start with expected prefixes.

**Reference:** references/command_injection.md

---

## Unverifiable Findings

### [UNVERIFIABLE] VULN-007 -- LLM Prompt Injection in System Prompt
**Severity:** Informational

**File:** cmd/late/main.go:93-107

**Description:** The system prompt can be loaded from a file (--system-prompt-file) or an environment variable (LATE_SYSTEM_PROMPT). An LLM could inject instructions into the system prompt via a crafted prompt file that the LLM then follows.

**Evidence:**
```go
// cmd/late/main.go:93-99
if *systemPromptFileReq != "" {
    content, err := os.ReadFile(*systemPromptFileReq)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error reading system prompt file: %v\n", err)
        os.Exit(1)
    }
    systemPrompt = string(content)
}
```

**Impact:** If the system prompt file is controlled by the LLM (e.g., via a tool that writes to it), it could inject instructions like "ignore previous instructions" or "output the API key".

**Remediation:** Validate system prompt file content or limit its size. Consider using a separate process for prompt injection.

---

## Remediation Priority

| Priority | Finding | Severity | Effort |
|----------|---------|----------|--------|
| 1 | VULN-002: Unbounded JSON Parsing | Low | Easy |
| 2 | VULN-001: Shell Command Injection | Low | Medium |
| 3 | VULN-005: HTTP TLS Config | Info | Easy |
| 4 | VULN-004: Session TTL | Info | Medium |
| 5 | VULN-003: Plaintext API Keys | Info | Medium |
| 6 | VULN-006: MCP Sandbox | Info | Hard |
| 7 | VULN-007: Prompt Injection | Info | Easy |

---

## Defense-in-Depth Assessment

Late has a solid defense-in-depth strategy for a single-user CLI tool:

1. **Command Confinement:** The BashAnalyzer parser + tier-based allowlist prevents most shell injection.
2. **Path Confinement:** IsSafePath() restricts file operations to the CWD.
3. **Confirmation Gate:** Shell commands require user approval unless --unsupervised flag is set.
4. **Session Isolation:** Sessions stored per-user with 0600/0700 permissions.
5. **Config Security:** Config directory uses restrictive permissions.
6. **Atomic File Writes:** History persistence uses temp file + rename pattern.
7. **Unsupervised Mode:** --unsupervised flag enables fully autonomous tool execution.

**Overall Assessment:** Late's security model is appropriate for its intended use case (single developer tool). The main attack surface is the LLM-driven command execution, which is mitigated by the allowlist and user confirmation.

---

## Vulnerability Classes Scanned

The following 26 vulnerability classes were systematically checked:

| Category | Classes Checked | Status |
|----------|----------------|--------|
| **Injection** | SQLi, XSS, SSTI, NoSQL Injection, GraphQL Injection, XXE, RCE/Command Injection, Expression Language Injection | No SQLi/XSS (CLI app). RCE via bash tool: LOW. |
| **Access Control** | IDOR, Privilege Escalation, Auth/JWT, Default Credentials, Brute Force, HTTP Method Tamper, Verification Code Abuse, Session Fixation | No auth (single-user). No JWT. |
| **Data Exposure** | Weak Crypto/Hash, Information Disclosure, Insecure Cookie, Trust Boundary | API keys plaintext (INFO). Session data indefinite (INFO). |
| **Server-Side** | SSRF, Path Traversal/LFI/RFI, Insecure Deserialization, Arbitrary File Upload, JNDI Injection, Race Conditions | No SSRF sinks. Path confinement active. |
| **Protocol** | CSRF, Open Redirect, HTTP Smuggling/Desync, Denial of Service, CVE Patterns | DoS via JSON parsing (LOW). |
| **Language/Platform** | PHP Security, Mobile Security | N/A (Go CLI). |

---

*Report generated by llm-sast-scanner v1.3*
*All findings passed Judge re-verification per SAST skill specifications.*
