package evidence

import (
	"strings"
	"testing"
)

func TestNewIDStableAndDistinct(t *testing.T) {
	a := NewID("run-1", "withdrawn_test_token", "registration", 0)
	if a != NewID("run-1", "withdrawn_test_token", "registration", 0) {
		t.Fatal("NewID must be deterministic")
	}
	if !strings.HasPrefix(a, "EV-") || len(a) != 11 {
		t.Fatalf("NewID shape = %q, want EV-xxxxxxxx", a)
	}
	if NewID("run-1", "withdrawn_test_token", "registration", 1) == a {
		t.Fatal("sequence must change the ID")
	}
	if NewID("run-2", "withdrawn_test_token", "registration", 0) == a {
		t.Fatal("run must change the ID")
	}
}

func TestClassificationsAreClosed(t *testing.T) {
	for _, c := range []Classification{ObservedFailure, ObservedPass, InferredRisk, UntestedDependency} {
		if c == "" {
			t.Fatal("empty classification")
		}
	}
}
