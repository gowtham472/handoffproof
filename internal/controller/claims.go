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
	// Asserts records how a config or backup claim was read: "gap" when
	// the statement says something is missing, undocumented or untested,
	// "ok" otherwise. The status is checked against that reading.
	Asserts string `json:"asserts,omitempty"`
}

// gapMarkers match statements asserting a gap. Reading is deterministic on
// the claim text; a double negative ("no gaps") reads as a gap, a known
// limit of keyword reading.
var gapMarkers = regexp.MustCompile(`(?i)\b(not|never|no|none|nobody|nothing|undocumented|untested|unknown|missing|absent|without|cannot|fails?|failed|broken|lost)\b|n't\b`)

// assertsGap reports whether a claim statement asserts a gap.
func assertsGap(statement string) bool {
	return gapMarkers.MatchString(statement)
}

func assertsLabel(gap bool) string {
	if gap {
		return "gap"
	}
	return "ok"
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
		return verifyBackup(c, backupFailed, backupPassed)
	case "config":
		return verifyConfig(c, used, documented)
	default:
		return ClaimVerdict{ClaimID: c.ID, Status: ClaimUnverified, NeedsRunbook: true,
			Detail: "type " + c.Type + " has no drill or scan check; needs a written runbook"}
	}
}

// verifyBackup checks a backup claim against incomplete-backup drill
// evidence in the direction the claim asserts.
func verifyBackup(c Claim, failed, passed []evidence.Record) ClaimVerdict {
	gap := assertsGap(c.Statement)
	v := ClaimVerdict{ClaimID: c.ID, Asserts: assertsLabel(gap)}
	switch {
	case len(failed) > 0:
		v.EvidenceIDs = recordIDs(failed)
		v.Status = ClaimContradicted
		v.Detail = "incomplete-backup drill evidence shows a gap: restore was not demonstrated"
		if gap {
			v.Status = ClaimConfirmed
			v.Detail = "incomplete-backup drill evidence shows the gap; restore was not demonstrated"
		}
	case len(passed) > 0:
		v.EvidenceIDs = recordIDs(passed)
		v.Status = ClaimConfirmed
		v.Detail = "incomplete-backup drill passed with a restore attempt on record"
		if gap {
			v.Status = ClaimContradicted
		}
	default:
		v.Status = ClaimUnverified
		v.Detail = "no incomplete-backup drill evidence for this claim"
	}
	return v
}

// verifyConfig checks a config claim against static-scan usage and
// documentation of the env name, in the direction the claim asserts.
func verifyConfig(c Claim, used, documented map[string]bool) ClaimVerdict {
	gap := assertsGap(c.Statement)
	v := ClaimVerdict{ClaimID: c.ID, Asserts: assertsLabel(gap)}
	if !used[c.Subject] {
		v.Status = ClaimUnverified
		v.Detail = "env name " + c.Subject + " appears neither in code nor in docs"
		return v
	}
	if documented[c.Subject] {
		v.Detail = "static scan: " + c.Subject + " is used and documented"
	} else {
		v.Detail = "static scan: " + c.Subject + " is used in code but undocumented"
	}
	// The claim holds when its reading matches the scan: a gap claim
	// needs the name undocumented, an ok claim needs it documented.
	if gap != documented[c.Subject] {
		v.Status = ClaimConfirmed
	} else {
		v.Status = ClaimContradicted
	}
	return v
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
