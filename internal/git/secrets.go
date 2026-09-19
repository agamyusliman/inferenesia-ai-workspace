package git

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SecretCheckResult is the outcome of a staged-secret scan (VAL-CROSS-007).
type SecretCheckResult struct {
	// OK is true when no staged secrets were detected.
	OK bool `json:"ok"`
	// BlockedPaths lists the staged paths flagged as secret/credential material.
	BlockedPaths []string `json:"blocked_paths,omitempty"`
	// Message is a human-readable summary for the panel / agent tool error.
	Message string `json:"message,omitempty"`
}

// ErrStagedSecrets is returned by Commit when the index holds a secret-looking path.
var ErrStagedSecrets = fmt.Errorf("git: staged secrets detected")

// secretPathGlobs are basename / extension patterns considered secret material.
// A staged path matching any of these is refused at commit time (VAL-CROSS-007).
var secretPathGlobs = []string{
	".env",
	".env.*",
	"*.env",
	"*.env.*",
	".envrc",
	"*.pem",
	"*.key",
	"*.p12",
	"*.pfx",
	"*.jks",
	"*.keystore",
	"*.crt",
	"*.cer",
	"id_rsa",
	"id_rsa.*",
	"id_ed25519",
	"id_ed25519.*",
	"id_dsa",
	"id_dsa.*",
	"id_ecdsa",
	"id_ecdsa.*",
	"*.ppk",
	"*.kdbx",
	"auth.json",
	"token.json",
	"tokens.json",
	"*.token",
	"*.tokens",
	"service-account*.json",
	"*-service-account.json",
	"google-credentials.json",
	"firebase-adminsdk*.json",
	"credentials.yaml",
	"credentials.yml",
	"credentials.toml",
	"credentials.json",
	"secrets.yaml",
	"secrets.yml",
	"secrets.toml",
	"secrets.json",
	"api-keys.*",
	"apikeys.*",
	"api_keys.*",
	".netrc",
	".npmrc",
}

// IsSecretPath reports whether path is likely a credential/secret file
// (VAL-CROSS-007 pre-commit guard). The check is filename-based and conservative:
// false positives are acceptable for safety; false negatives are not.
//
// path may be workspace-relative or absolute. Directory components are ignored —
// only the basename is matched against secret globs.
//
// Allowlist: `.env.example`, `.env.*.example`, and `env.example` are NOT flagged
// so users can track committed example/template env files (the root .gitignore
// explicitly keeps these via `!.env.example` negations).
func IsSecretPath(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	// Normalize separators for cross-platform matching.
	rel := filepath.ToSlash(path)
	base := filepath.Base(rel)
	if base == "" || base == "." || base == "/" {
		return false
	}
	lowBase := strings.ToLower(base)

	// Allowlist: committed example/template env files are safe to track.
	// Covers .env.example, .env.production.example, env.example, etc.
	if isEnvExampleFile(lowBase) {
		return false
	}

	// Exact-name matches first.
	for _, pat := range secretPathGlobs {
		if ok, _ := filepath.Match(pat, base); ok {
			return true
		}
	}

	// Filename contains a credential-ish substring (.env, credential, secret).
	switch {
	case strings.Contains(lowBase, "credentials"),
		strings.Contains(lowBase, ".secret"),
		strings.Contains(lowBase, "_secret"),
		strings.Contains(lowBase, "service-account"),
		strings.Contains(lowBase, "firebase-adminsdk"):
		return true
	}

	// .env with any suffix or prefix (e.g. dev.env, prod.env.local).
	if strings.HasSuffix(lowBase, ".env") || strings.HasSuffix(lowBase, ".env.local") ||
		strings.HasPrefix(lowBase, ".env.") {
		return true
	}

	// Private SSH key prefix (id_*). The glob above already covers common names,
	// but be defensive: any id_* without an extension is treated as a key.
	if strings.HasPrefix(lowBase, "id_") && filepath.Ext(base) == "" {
		return true
	}

	return false
}

// isEnvExampleFile reports whether base is a committed example/template env
// file (.env.example, .env.<x>.example, env.example) that is intentionally
// tracked and should NOT be flagged by the secret guard.
func isEnvExampleFile(lowBase string) bool {
	switch {
	case lowBase == ".env.example",
		lowBase == "env.example",
		lowBase == ".envrc.example":
		return true
	case strings.HasPrefix(lowBase, ".env.") && strings.HasSuffix(lowBase, ".example"):
		return true
	}
	return false
}

// CheckStagedSecrets scans the git index for paths that look like secrets and
// returns a SecretCheckResult. OK=true means it is safe to proceed with commit.
//
// Uses `git diff --cached --name-only` so it only inspects what is actually
// staged (not the broader working tree). Paths that are gitignored are not in
// the index, so the check is robust to a stray .env in the working tree that
// was never `git add`-ed.
func (s *Service) CheckStagedSecrets() (SecretCheckResult, error) {
	res := SecretCheckResult{OK: true}
	if s == nil || !s.isRepo() {
		// Not a repo → nothing staged → vacuously OK.
		return res, nil
	}
	out, err := s.runCapture("diff", "--cached", "--name-only")
	if err != nil {
		// If the diff command fails for some reason (e.g. no HEAD yet on a fresh
		// repo), fall back to ls-files --stage so we still catch staged secrets
		// on the initial commit. Both paths only read index contents.
		out2, err2 := s.runCapture("ls-files", "--stage")
		if err2 != nil {
			res.OK = false
			res.Message = fmt.Sprintf("git: staged-secret scan failed: %v", err)
			return res, err
		}
		out = extractPathsFromLsFiles(out2)
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if IsSecretPath(line) {
			res.BlockedPaths = append(res.BlockedPaths, line)
		}
	}
	if len(res.BlockedPaths) > 0 {
		res.OK = false
		res.Message = fmt.Sprintf("staged secrets detected: %s", strings.Join(res.BlockedPaths, ", "))
	}
	return res, nil
}

// extractPathsFromLsFiles pulls the path column out of `git ls-files --stage`
// output lines shaped like "<mode> <sha> <stage>\t<path>".
func extractPathsFromLsFiles(out string) string {
	var b strings.Builder
	for _, line := range strings.Split(out, "\n") {
		if i := strings.IndexByte(line, '\t'); i >= 0 {
			b.WriteString(line[i+1:])
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// ensureCommitSafe runs the pre-commit secret guard. Returns the blocking
// SecretCheckResult (with OK=false) and ErrStagedSecrets when staged secrets
// are detected; otherwise returns OK=true with a nil error.
func (s *Service) ensureCommitSafe() (SecretCheckResult, error) {
	res, err := s.CheckStagedSecrets()
	if err != nil {
		// Conservative: surface the error but allow caller to decide. We map
		// a scan failure to a hard block so secrets never slip through on
		// a misconfigured repo.
		return res, err
	}
	if !res.OK {
		return res, fmt.Errorf("%w: %s", ErrStagedSecrets, res.Message)
	}
	return res, nil
}
