package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedactVariants(t *testing.T) {
	cases := []struct{ in, leaked string }{
		{`"MAIL_API_TOKEN": "short7"`, "short7"},                 // quoted JSON key, short value, long field name
		{`mailApiToken = 'zx81-qwerty-99'`, "zx81-qwerty-99"},    // camelCase long form
		{`Authorization: plainvalue123`, "plainvalue123"},        // no Bearer [REDACTED]
		{`aws_key=AKIAIOSFODNN7EXAMPLE`, "AKIAIOSFODNN7EXAMPLE"}, // AWS access key id
		{`token: ghp_1234567890abcdefghij12`, "ghp_1234567890"},  // GitHub PAT
		{`jwt eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.signaturepart`, "eyJhbGciOiJIUzI1NiJ9"},
		{`mysql://root:hunter2pw@db:3306/app`, "hunter2pw"}, // non-postgres connection string
		{`PASSWORD=hunter2`, "hunter2"},                     // short password
		{`client_secret: 'a1b2c3d4e5'`, "a1b2c3d4e5"},       // quoted secret value
	}
	for _, c := range cases {
		out := Redact(c.in)
		if strings.Contains(out, c.leaked) {
			t.Errorf("Redact(%q) leaked %q: %q", c.in, c.leaked, out)
		}
		if !strings.Contains(out, "[REDACTED:") {
			t.Errorf("Redact(%q) has no marker: %q", c.in, out)
		}
	}
	// Ordinary prose must survive.
	if out := Redact("the token endpoint returns tokens for booking"); strings.Contains(out, "[REDACTED:") {
		t.Errorf("prose over-redacted: %q", out)
	}
}

func TestExecutorRefusesSecretAndSymlinkPaths(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, ".env"), []byte("MAIL_API_TOKEN=supersecret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "real.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ex := &Executor{Workspace: ws}
	if _, err := ex.Execute(t.Context(), "read_file", `{"path":".env"}`); err == nil {
		t.Fatal("read_file .env should be refused")
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(ws, "link.txt")); err == nil {
		if _, err := ex.Execute(t.Context(), "read_file", `{"path":"link.txt"}`); err == nil {
			t.Fatal("read_file through symlink should be refused")
		}
	}
	out, err := ex.Execute(t.Context(), "list_files", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, ".env") {
		t.Fatalf("list_files leaked secret file name: %s", out)
	}
}
