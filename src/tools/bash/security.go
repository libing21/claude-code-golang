package bash

import (
	"regexp"
	"strings"
)

type destructivePattern struct {
	re      *regexp.Regexp
	warning string
}

var destructivePatterns = []destructivePattern{
	{regexp.MustCompile(`\bgit\s+reset\s+--hard\b`), "may discard uncommitted changes"},
	{regexp.MustCompile(`\bgit\s+push\b[^;&|\n]*[ \t](--force|--force-with-lease|-f)\b`), "may overwrite remote history"},
	// NOTE: Go regexp doesn't support negative lookahead; handle -n/--dry-run exclusion in code.
	{regexp.MustCompile(`\bgit\s+clean\b[^;&|\n]*-[a-zA-Z]*f`), "may permanently delete untracked files"},
	{regexp.MustCompile(`\bgit\s+checkout\s+(--\s+)?\.[ \t]*($|[;&|\n])`), "may discard all working tree changes"},
	{regexp.MustCompile(`\bgit\s+restore\s+(--\s+)?\.[ \t]*($|[;&|\n])`), "may discard all working tree changes"},
	{regexp.MustCompile(`\bgit\s+stash[ \t]+(drop|clear)\b`), "may permanently remove stashed changes"},
	{regexp.MustCompile(`\bgit\s+branch\s+(-D[ \t]|--delete\s+--force|--force\s+--delete)\b`), "may force-delete a branch"},
	{regexp.MustCompile(`\bgit\s+(commit|push|merge)\b[^;&|\n]*--no-verify\b`), "may skip safety hooks"},
	{regexp.MustCompile(`\bgit\s+commit\b[^;&|\n]*--amend\b`), "may rewrite the last commit"},
	// rm -rf family
	{regexp.MustCompile(`(^|[;&|\n]\s*)rm\s+-[a-zA-Z]*[rR][a-zA-Z]*f|(^|[;&|\n]\s*)rm\s+-[a-zA-Z]*f[a-zA-Z]*[rR]`), "may recursively force-remove files"},
	{regexp.MustCompile(`(^|[;&|\n]\s*)rm\s+-[a-zA-Z]*[rR]`), "may recursively remove files"},
	{regexp.MustCompile(`(^|[;&|\n]\s*)rm\s+-[a-zA-Z]*f`), "may force-remove files"},
	// DB
	{regexp.MustCompile(`\b(DROP|TRUNCATE)\s+(TABLE|DATABASE|SCHEMA)\b`), "may drop or truncate database objects"},
	{regexp.MustCompile(`\bDELETE\s+FROM\s+\w+[ \t]*(;|"|'|\n|$)`), "may delete all rows from a database table"},
	// Infra
	{regexp.MustCompile(`\bkubectl\s+delete\b`), "may delete Kubernetes resources"},
	{regexp.MustCompile(`\bterraform\s+destroy\b`), "may destroy Terraform infrastructure"},
}

var (
	segmentSplitRE   = regexp.MustCompile(`[;&|\n]`)
	gitCleanDryRunRE = regexp.MustCompile(`(^|[\s\t])-[a-zA-Z]*n`)
)

func commandSegmentContaining(command string, idx int) string {
	// Best-effort: find the single "segment" between separators ; & | or newline.
	start := 0
	end := len(command)
	for _, loc := range segmentSplitRE.FindAllStringIndex(command, -1) {
		if loc[0] <= idx {
			start = loc[1]
			continue
		}
		end = loc[0]
		break
	}
	seg := strings.TrimSpace(command[start:end])
	return seg
}

func DestructiveCommandWarning(command string) string {
	for _, p := range destructivePatterns {
		loc := p.re.FindStringIndex(command)
		if loc != nil {
			// Special-case git clean: ignore dry-run.
			if strings.Contains(p.re.String(), `\bgit\s+clean\b`) {
				seg := commandSegmentContaining(command, loc[0])
				low := strings.ToLower(seg)
				if strings.Contains(low, "--dry-run") || gitCleanDryRunRE.MatchString(seg) {
					continue
				}
			}
			return p.warning
		}
	}
	return ""
}

// A minimal subset of TS bashSecurity "dangerous patterns" that can bypass naive
// checks. This is intentionally conservative.
var dangerousSubstitutionPatterns = []destructivePattern{
	{regexp.MustCompile(`<\(`), "process substitution <()"},
	{regexp.MustCompile(`>\(`), "process substitution >()"},
	{regexp.MustCompile(`(?:^|[\s;&|])=[a-zA-Z_]`), "Zsh equals expansion (=cmd)"},
	{regexp.MustCompile(`\$\(`), "$() command substitution"},
	{regexp.MustCompile("`"), "backticks command substitution"},
	{regexp.MustCompile(`\$\{`), "${} parameter substitution"},
	{regexp.MustCompile(`\$\[`), "$[] legacy arithmetic expansion"},
	{regexp.MustCompile(`~\[`), "zsh-style parameter expansion"},
}

func DangerousSubstitutionReason(command string) string {
	for _, p := range dangerousSubstitutionPatterns {
		if p.re.MatchString(command) {
			return p.warning
		}
	}
	return ""
}

// Additional policy checks inspired by TS bashSecurity.ts.
var extraSecurityPatterns = []destructivePattern{
	// Redirections (excluding /dev/null cases, which we don't fully parse here).
	{regexp.MustCompile(`(^|[\s;&|])[012]?\s*(>>?|<)\s*[^ \n]`), "input/output redirection"},
	// Brace expansion can explode argument lists or hide tokens.
	{regexp.MustCompile(`\{[^}]*\}`), "brace expansion"},
	// Newlines are suspicious in single-command contexts.
	{regexp.MustCompile(`\n`), "contains newline"},
	// Control chars / non-ascii whitespace (very conservative).
	{regexp.MustCompile(`[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]`), "contains control characters"},
	{regexp.MustCompile(`[^\x09\x0A\x0D\x20-\x7E]`), "contains non-ascii characters"},
}

func ExtraSecurityReason(command string) string {
	for _, p := range extraSecurityPatterns {
		if p.re.MatchString(command) {
			return p.warning
		}
	}
	return ""
}
