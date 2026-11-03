package controller

import (
	"testing"

	"github.com/gowtham472/handoffproof/internal/evidence"
	"github.com/gowtham472/handoffproof/internal/scan"
)

func TestAssertsGap(t *testing.T) {
	for _, tc := range []struct {
		statement string
		want      bool
	}{
		{"DATABASE_URL is documented in the README", false},
		{"Backups run nightly", false},
		{"DB backup runs nightly - restore never tested", true},
		{"API_KEY is not documented anywhere", true},
		{"MAIL_TOKEN is undocumented", true},
		{"Nobody has tried a restore", true},
		{"The README doesn't mention SMTP_HOST", true},
		{"Notes about deploys live in Confluence", false}, // "Notes" is not "not"
	} {
		if got := assertsGap(tc.statement); got != tc.want {
			t.Errorf("assertsGap(%q) = %v, want %v", tc.statement, got, tc.want)
		}
	}
}

func TestVerifyClaimsPolarity(t *testing.T) {
	rep := scan.Report{EnvVars: []scan.EnvVar{
		{Name: "DOCD", Documented: true},
		{Name: "UNDOC", Documented: false},
	}}
	failedBackup := []evidence.Record{{ID: "EV-f", Drill: "incomplete_backup", Passed: false}}
	passedBackup := []evidence.Record{{ID: "EV-p", Drill: "incomplete_backup", Passed: true}}
	for _, tc := range []struct {
		name    string
		records []evidence.Record
		claim   Claim
		want    ClaimStatus
		asserts string
	}{
		{"config ok, documented", nil, Claim{Type: "config", Subject: "DOCD", Statement: "DOCD is documented in the README"}, ClaimConfirmed, "ok"},
		{"config ok, undocumented", nil, Claim{Type: "config", Subject: "UNDOC", Statement: "UNDOC is documented in the README"}, ClaimContradicted, "ok"},
		{"config gap, undocumented", nil, Claim{Type: "config", Subject: "UNDOC", Statement: "UNDOC is not documented"}, ClaimConfirmed, "gap"},
		{"config gap, documented", nil, Claim{Type: "config", Subject: "DOCD", Statement: "DOCD is not documented"}, ClaimContradicted, "gap"},
		{"config unused name", nil, Claim{Type: "config", Subject: "GHOST", Statement: "GHOST is documented"}, ClaimUnverified, "ok"},
		{"backup gap, drill failed", failedBackup, Claim{Type: "backup", Statement: "restore never tested"}, ClaimConfirmed, "gap"},
		{"backup ok, drill failed", failedBackup, Claim{Type: "backup", Statement: "backups restore fine"}, ClaimContradicted, "ok"},
		{"backup ok, drill passed", passedBackup, Claim{Type: "backup", Statement: "backups restore fine"}, ClaimConfirmed, "ok"},
		{"backup gap, drill passed", passedBackup, Claim{Type: "backup", Statement: "restore never tested"}, ClaimContradicted, "gap"},
		{"backup without evidence", nil, Claim{Type: "backup", Statement: "backups restore fine"}, ClaimUnverified, "ok"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.claim.ID = "C"
			got := VerifyClaims(tc.records, rep, t.TempDir(), []Claim{tc.claim})[0]
			if got.Status != tc.want || got.Asserts != tc.asserts {
				t.Errorf("verdict = %+v, want %s (asserts %s)", got, tc.want, tc.asserts)
			}
		})
	}
}
