package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func execCmd(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}

// --- drills.Env implementation -------------------------------------------

const fetchScript = `const u='http://127.0.0.1:'+process.env.HP_PORT+'/'+process.env.HP_PATH;const b=process.env.HP_BODY_B64?Buffer.from(process.env.HP_BODY_B64,'base64'):undefined;const h={'content-type':'application/json'};if(process.env.HP_BEARER)h.authorization='Bearer '+process.env.HP_BEARER;fetch(u,{method:process.env.HP_METHOD,headers:h,body:b}).then(async r=>{process.stdout.write(r.status+'\n'+(await r.text()))}).catch(e=>{console.error(e.message);process.exit(1)})`

// execFetch performs an HTTP request from inside a sandbox container, so
// drill sandboxes need no published ports at all.
func (sb *Sandbox) execFetch(ctx context.Context, service string, port int, method, path, bearer string, body []byte) (int, []byte, error) {
	args := []string{"compose", "-p", sb.Project, "exec", "-T",
		"-e", "HP_PORT=" + strconv.Itoa(port),
		"-e", "HP_METHOD=" + method,
		"-e", "HP_PATH=" + path,
		"-e", "HP_BEARER=" + bearer,
		"-e", "HP_BODY_B64=" + base64.StdEncoding.EncodeToString(body),
		service, "node", "-e", fetchScript}
	cmd := execCmd(ctx, "docker", args...)
	cmd.Dir = sb.appDir()
	cmd.Env = composeEnvBase()
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return 0, nil, fmt.Errorf("exec fetch %s %s: %w: %s", method, path, err, tail(errb.String(), 400))
	}
	text := out.String()
	codeStr, rest, _ := strings.Cut(text, "\n")
	code, err := strconv.Atoi(strings.TrimSpace(codeStr))
	if err != nil {
		return 0, nil, fmt.Errorf("exec fetch %s: bad status %q", path, codeStr)
	}
	return code, []byte(rest), nil
}

func (sb *Sandbox) callJSON(ctx context.Context, service string, port int, method, path, bearer string, payload any) (int, []byte, error) {
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			return 0, nil, err
		}
	}
	return sb.execFetch(ctx, service, port, method, path, bearer, body)
}

func (sb *Sandbox) Register(ctx context.Context, email, name string) (int, error) {
	code, raw, err := sb.callJSON(ctx, "app", 3000, "POST", "register", "", map[string]any{"email": email, "name": name})
	if err != nil {
		return 0, err
	}
	if code != 201 {
		return 0, fmt.Errorf("register status %d: %s", code, tail(string(raw), 300))
	}
	var v struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return 0, err
	}
	return v.ID, nil
}

func (sb *Sandbox) Book(ctx context.Context, userID int, service string) (int, error) {
	code, raw, err := sb.callJSON(ctx, "app", 3000, "POST", "bookings", "", map[string]any{"user_id": userID, "service": service})
	if err != nil {
		return 0, err
	}
	if code != 201 {
		return 0, fmt.Errorf("book status %d: %s", code, tail(string(raw), 300))
	}
	var v struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return 0, err
	}
	return v.ID, nil
}

func (sb *Sandbox) SinkCount(ctx context.Context, email string) (int, error) {
	code, raw, err := sb.callJSON(ctx, "mail-sink", 4100, "GET", "messages?to="+email, sb.AdminToken, nil)
	if err != nil {
		return 0, err
	}
	if code != 200 {
		return 0, fmt.Errorf("sink messages status %d: %s", code, tail(string(raw), 300))
	}
	var v struct {
		Messages []any `json:"messages"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return 0, err
	}
	return len(v.Messages), nil
}

func (sb *Sandbox) Upload(ctx context.Context, name string, content []byte) (string, string, error) {
	code, raw, err := sb.callJSON(ctx, "app", 3000, "POST", "uploads", "", map[string]any{
		"name":           name,
		"content_base64": base64.StdEncoding.EncodeToString(content),
	})
	if err != nil {
		return "", "", err
	}
	if code != 201 {
		return "", "", fmt.Errorf("upload status %d: %s", code, tail(string(raw), 300))
	}
	var v struct {
		ID  string `json:"id"`
		SHA string `json:"sha256"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", "", err
	}
	sb.lastUploadID = v.ID
	return v.ID, v.SHA, nil
}

func (sb *Sandbox) DocStored(ctx context.Context, docID string) (bool, string, error) {
	code, raw, err := sb.callJSON(ctx, "app", 3000, "GET", "documents/"+docID, "", nil)
	if err != nil {
		return false, "", err
	}
	if code != 200 {
		return false, "", fmt.Errorf("document status %d: %s", code, tail(string(raw), 300))
	}
	var v struct {
		SHA    string `json:"sha256"`
		Stored bool   `json:"stored"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return false, "", err
	}
	return v.Stored, v.SHA, nil
}

func (sb *Sandbox) psql(ctx context.Context, sql string) (string, error) {
	args := []string{"compose", "-p", sb.Project, "exec", "-T", "db", "psql", "-U", "handoff", "-d", "handoffproof", "-t", "-A", "-c", sql}
	cmd := execCmd(ctx, "docker", args...)
	cmd.Dir = sb.appDir()
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("psql: %w: %s", err, tail(errb.String(), 500))
	}
	return strings.TrimSpace(out.String()), nil
}

func (sb *Sandbox) appDir() string { return filepath.Join(sb.Dir, "app") }

func (sb *Sandbox) UserExists(ctx context.Context, email string) (bool, error) {
	got, err := sb.psql(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE email = '"+strings.ReplaceAll(email, "'", "''")+"')")
	if err != nil {
		return false, err
	}
	return got == "t" || got == "true", nil
}

func (sb *Sandbox) BookingExists(ctx context.Context, bookingID int) (bool, error) {
	got, err := sb.psql(ctx, fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM bookings WHERE id = %d)", bookingID))
	if err != nil {
		return false, err
	}
	return got == "t" || got == "true", nil
}

func (sb *Sandbox) RevokeMailToken(ctx context.Context) error {
	code, raw, err := sb.callJSON(ctx, "mail-sink", 4100, "POST", "admin/revoke", sb.AdminToken, map[string]any{"token": sb.MailToken})
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("revoke status %d: %s", code, tail(string(raw), 300))
	}
	return nil
}

func (sb *Sandbox) AppLogs(ctx context.Context) (string, error) {
	out, err := sb.composeEnv(ctx, "logs", "--tail", "200", "app")
	if err != nil {
		return "", err
	}
	return tail(out, 4000), nil
}

// BackupRestore dumps the database, destroys the stack volumes, restores
// into a fresh stack, and reports row counts plus whether the last uploaded
// file survived. The demo backup script covers the database only, so the
// uploaded file is expected to be lost (that loss is the drill signal).
func (sb *Sandbox) BackupRestore(ctx context.Context) (int, bool, error) {
	dump, err := sb.composeExec(ctx, nil, "db", "pg_dump", "-U", "handoff", "handoffproof")
	if err != nil {
		return 0, false, fmt.Errorf("pg_dump: %w", err)
	}
	if _, err := sb.composeEnv(ctx, "down", "-v"); err != nil {
		return 0, false, fmt.Errorf("down: %w", err)
	}
	if _, err := sb.composeEnv(ctx, "up", "-d"); err != nil {
		return 0, false, fmt.Errorf("fresh up: %w", err)
	}
	if err := sb.waitHealthy(ctx); err != nil {
		return 0, false, err
	}
	if _, err := sb.composeExec(ctx, dump, "db", "psql", "-U", "handoff", "-d", "handoffproof", "-q"); err != nil {
		return 0, false, fmt.Errorf("restore: %w", err)
	}
	got, err := sb.psql(ctx, "SELECT (SELECT count(*) FROM users)+(SELECT count(*) FROM bookings)+(SELECT count(*) FROM documents)")
	if err != nil {
		return 0, false, err
	}
	rows, err := strconv.Atoi(got)
	if err != nil {
		return 0, false, fmt.Errorf("count rows: %w", err)
	}
	fileOK := false
	if sb.lastUploadID != "" {
		stored, _, err := sb.DocStored(ctx, sb.lastUploadID)
		if err == nil {
			fileOK = stored
		}
	}
	return rows, fileOK, nil
}

func (sb *Sandbox) composeExec(ctx context.Context, stdin []byte, svc string, args ...string) ([]byte, error) {
	full := append([]string{"compose", "-p", sb.Project, "exec", "-T", svc}, args...)
	cmd := execCmd(ctx, "docker", full...)
	cmd.Dir = sb.appDir()
	cmd.Env = composeEnvBase()
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, tail(errb.String(), 500))
	}
	return out.Bytes(), nil
}
