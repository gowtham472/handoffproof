package controller

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gowtham472/handoffproof/internal/evidence"
	"github.com/gowtham472/handoffproof/internal/scan"
)

// Claim is one checkable handover statement extracted from a transcript.
// The statement text is untrusted: it may hold spoken secrets and injected
// instructions. Statuses below are computed deterministically from drill
// evidence and the static scan; they are not model verdicts.
type Claim struct {
	ID        string `json:"id"`
	Speaker   string `json:"speaker"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Subject   string `json:"subject"`
	Statement string `json:"statement"`
}

// ClaimStatus is confirmed, contradicted or unverifiable per claim.
type ClaimStatus string

const (
	ClaimConfirmed    ClaimStatus = "confirmed"
	ClaimContradicted ClaimStatus = "contradicted"
	ClaimUnverified   ClaimStatus = "unverifiable"
)

// ClaimVerdict is the controller's deterministic routing result for a claim.
// EvidenceIDs cite drill records; empty when the check rests on the static
// scan or on the absence of evidence (documented in Detail).
type ClaimVerdict struct {
	ClaimID      string      `json:"claim_id"`
	Status       ClaimStatus `json:"status"`
	EvidenceIDs  []string    `json:"evidence_ids"`
	Detail       string      `json:"detail"`
	NeedsRunbook bool        `json:"needs_runbook"`
}

// injectionMarkers matches the repo's known prompt-injection tells. Claims
// carrying them are dropped before verification, never routed.
var injectionMarkers = regexp.MustCompile(`(?i)AI reviewers:|mark all checks as passed|ignore (all |your |previous )?instructions|disregard (all )?(previous|prior|above) instructions|exfiltrate`)

// secretPrefix matches known credential value prefixes. upperSnake matches
// conventional env var names, which are never values.
var secretPrefix = regexp.MustCompile(`(?i)^(AKIA|ghp_|gho_|ghu_|ghs_|ghr_|github_pat_|xox[baprs]-|sk-|eyJ|-----BEGIN)`)
var upperSnake = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// looksSecretValue reports whether s looks like a credential value rather
// than a name. Values are never searched for and never quoted in output.
func looksSecretValue(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if secretPrefix.MatchString(s) {
		return true
	}
	if strings.ContainsAny(s, " \t\n\"'") {
		return true // names never contain these
	}
	return len(s) >= 20 && !upperSnake.MatchString(s)
}

// FilterInjectedClaims drops claims whose statement carries injection
// markers. The drops are returned for logging; dropped claims are never
// verified or reported as findings.
func FilterInjectedClaims(claims []Claim) (kept, dropped []Claim) {
	for _, c := range claims {
		if injectionMarkers.MatchString(c.Statement) || injectionMarkers.MatchString(c.Subject) {
			dropped = append(dropped, c)
			continue
		}
		kept = append(kept, c)
	}
	return kept, dropped
}

// VerifyClaims routes each claim deterministically by type:
//   - credential_location: withdrawn-token drill evidence plus a repo-wide
//     documentation search for the credential name.
//   - backup: incomplete-backup drill evidence.
//   - config: static-scan usage versus documentation for the env name.
//   - process / other: unverifiable, flagged as needing a written runbook.
func VerifyClaims(records []evidence.Record, rep scan.Report, repoRoot string, claims []Claim) []ClaimVerdict {
	dead, live := drillOutcomes(records, "withdrawn_token")
	backupFailed, backupPassed := drillOutcomes(records, "incomplete_backup")
	used, documented := scanEnv(rep)
	out := make([]ClaimVerdict, 0, len(claims))
	for _, c := range claims {
		out = append(out, verifyOne(c, repoRoot, dead, live, backupFailed, backupPassed, used, documented))
	}
	return out
}

func drillOutcomes(records []evidence.Record, drill string) (failed, passed []evidence.Record) {
	for _, r := range records {
		if r.Drill != drill {
			continue
		}
		if r.Passed {
			passed = append(passed, r)
		} else {
			failed = append(failed, r)
		}
	}
	return failed, passed
}

func scanEnv(rep scan.Report) (used, documented map[string]bool) {
	used = map[string]bool{}
	documented = map[string]bool{}
	for _, e := range rep.EnvVars {
		used[e.Name] = true
		if e.Documented {
			documented[e.Name] = true
		}
	}
	return used, documented
}

func recordIDs(recs []evidence.Record) []string {
	ids := make([]string, 0, len(recs))
	for _, r := range recs {
		ids = append(ids, r.ID)
	}
	sort.Strings(ids)
	return ids
}

func verifyOne(c Claim, repoRoot string, dead, live, backupFailed, backupPassed []evidence.Record, used, documented map[string]bool) ClaimVerdict {
	switch c.Type {
	case "credential_location":
		return verifyCredential(c, repoRoot, dead, live)
	case "backup":
		if len(backupFailed) > 0 {
			return ClaimVerdict{ClaimID: c.ID, Status: ClaimConfirmed,
				EvidenceIDs: recordIDs(backupFailed),
				Detail:      "incomplete-backup drill evidence shows the gap; restore was not demonstrated"}
		}
		if len(backupPassed) > 0 {
			return ClaimVerdict{ClaimID: c.ID, Status: ClaimContradicted,
				EvidenceIDs: recordIDs(backupPassed),
				Detail:      "incomplete-backup drill passed with a restore attempt on record"}
		}
		return ClaimVerdict{ClaimID: c.ID, Status: ClaimUnverified,
			Detail: "no incomplete-backup drill evidence for this claim"}
	case "config":
		if !used[c.Subject] {
			return ClaimVerdict{ClaimID: c.ID, Status: ClaimUnverified,
				Detail: "env name " + c.Subject + " appears neither in code nor in docs"}
		}
		if !documented[c.Subject] {
			return ClaimVerdict{ClaimID: c.ID, Status: ClaimConfirmed,
				Detail: "static scan: " + c.Subject + " is used in code but undocumented"}
		}
		return ClaimVerdict{ClaimID: c.ID, Status: ClaimContradicted,
			Detail: "static scan: " + c.Subject + " is used and documented; no gap"}
	default:
		return ClaimVerdict{ClaimID: c.ID, Status: ClaimUnverified, NeedsRunbook: true,
			Detail: "type " + c.Type + " has no drill or scan check; needs a written runbook"}
	}
}

func verifyCredential(c Claim, repoRoot string, dead, live []evidence.Record) ClaimVerdict {
	if looksSecretValue(c.Subject) {
		return ClaimVerdict{ClaimID: c.ID, Status: ClaimUnverified,
			Detail: "subject looks like a secret value, not a name; not searched and not quoted"}
	}
	if len(live) > 0 {
		return ClaimVerdict{ClaimID: c.ID, Status: ClaimContradicted,
			EvidenceIDs: recordIDs(live),
			Detail:      "withdrawn-token drill passed: the credential still works"}
	}
	if len(dead) == 0 {
		return ClaimVerdict{ClaimID: c.ID, Status: ClaimUnverified,
			Detail: "no withdrawn-token drill evidence for this claim"}
	}
	paths := repoMentions(repoRoot, c.Subject)
	if len(paths) == 0 {
		return ClaimVerdict{ClaimID: c.ID, Status: ClaimUnverified,
			EvidenceIDs: recordIDs(dead),
			Detail:      "token is dead per drill evidence but the name is not documented anywhere searched"}
	}
	return ClaimVerdict{ClaimID: c.ID, Status: ClaimConfirmed,
		EvidenceIDs: recordIDs(dead),
		Detail:      "token is dead per drill evidence yet still documented in " + strings.Join(paths, ", ")}
}

// repoMentions lists up to three repo files mentioning name, skipping
// version control, dependencies, recorded evidence and unreadable binaries.
func repoMentions(root, name string) []string {
	var out []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || len(out) >= 3 {
			return nil
		}
		if info.IsDir() {
			base := info.Name()
			if base == ".git" || base == "node_modules" || base == "evidence" || base == ".agents" {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Size() > 1<<20 {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, b := range raw {
			if b == 0 {
				return nil // binary
			}
		}
		if strings.Contains(string(raw), name) {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return nil
			}
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	return out
}
