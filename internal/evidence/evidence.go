// Package evidence defines controller-issued evidence records and finding classes.
package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// Classification is the only set of classes a finding may carry.
type Classification string

const (
	ObservedFailure    Classification = "observed_failure"
	ObservedPass       Classification = "observed_pass"
	InferredRisk       Classification = "inferred_risk"
	UntestedDependency Classification = "untested_dependency"
)

// Record is written only by the controller. The agent can read records, never create them.
type Record struct {
	ID             string    `json:"evidence_id"`
	RunID          string    `json:"run_id"`
	Drill          string    `json:"drill"`
	Workflow       string    `json:"workflow"`
	Assertion      string    `json:"assertion"`
	Passed         bool      `json:"passed"`
	LogRef         string    `json:"log_ref"`
	ArtifactSHA256 string    `json:"artifact_sha256"`
	CreatedAt      time.Time `json:"created_at"`
}

// NewID returns a stable short ID such as EV-1a2b3c4d.
func NewID(runID, drill, workflow string, seq int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%d", runID, drill, workflow, seq)))
	return "EV-" + hex.EncodeToString(sum[:])[:8]
}
