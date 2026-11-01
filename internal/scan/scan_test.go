package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestScanFixtures(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		undoc []string
		gaps  []string
		ask   []string
	}{
		{
			name: "undocumented env var",
			files: map[string]string{
				"src/index.js": "const k = process.env.SECRET_X;\n",
				".env.example": "OTHER=1\n",
			},
			undoc: []string{"SECRET_X"},
		},
		{
			name: "documented env var",
			files: map[string]string{
				"src/index.js": "const k = process.env.KNOWN_KEY;\n",
				".env.example": "KNOWN_KEY=\n",
			},
			undoc: nil,
		},
		{
			name: "backup covers database only",
			files: map[string]string{
				"src/index.js": "const { Pool } = require('pg');\nconst dir = '/data/uploads';\n",
				"backup.sh":    "#!/bin/sh\npg_dump \"$DATABASE_URL\" > db.sql\n",
				".env.example": "DATABASE_URL=\n",
			},
			gaps: []string{"file_storage"},
		},
		{
			name: "ask phrase in docs",
			files: map[string]string{
				"docs/guide.md": "Ask Maria for the gateway keys.\n",
				"src/index.js":  "console.log('hi');\n",
			},
			ask: []string{"ask Maria"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rep, err := Scan(writeFixture(t, tc.files))
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.undoc {
				if !contains(rep.UndocumentedEnvVars, want) {
					t.Errorf("undocumented = %v, want %q", rep.UndocumentedEnvVars, want)
				}
			}
			if tc.undoc == nil && len(tc.files) > 0 && tc.name == "documented env var" {
				if len(rep.UndocumentedEnvVars) != 0 {
					t.Errorf("undocumented = %v, want none", rep.UndocumentedEnvVars)
				}
			}
			for _, want := range tc.gaps {
				if !contains(rep.BackupGaps, want) {
					t.Errorf("gaps = %v, want %q (covers %v)", rep.BackupGaps, want, rep.BackupCovers)
				}
			}
			for _, want := range tc.ask {
				found := false
				for _, f := range rep.AskPhrases {
					if f.Value == want {
						found = true
					}
				}
				if !found {
					t.Errorf("ask phrases = %v, want %q", rep.AskPhrases, want)
				}
			}
		})
	}
}

// The scan must catch the demo app's planted static faults (ground truth F1, F3, F4).
// F2 (silent mail failure) is expected to stay invisible to static analysis.
func TestScanDemoApp(t *testing.T) {
	rep, err := Scan(filepath.Join("..", "..", "demo-app"))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(rep.UndocumentedEnvVars, "SCHEDULER_TOKEN") {
		t.Errorf("undocumented = %v, want SCHEDULER_TOKEN (F1)", rep.UndocumentedEnvVars)
	}
	if !contains(rep.BackupGaps, "file_storage") {
		t.Errorf("gaps = %v, want file_storage (F3)", rep.BackupGaps)
	}
	found := false
	for _, f := range rep.AskPhrases {
		if f.Value == "ask Ravi" {
			found = true
		}
	}
	if !found {
		t.Errorf("ask phrases = %v, want 'ask Ravi' (F4)", rep.AskPhrases)
	}
}
