package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gowtham472/handoffproof/internal/byoa"
)

// Applications registry: user applications imported for Bring Your Own App
// rehearsals. Profiles are stored in plain JSON because they contain no
// secret values; every environment value supplied in the wizard is encrypted
// as one AES-GCM blob per application.

type applicationRecord struct {
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	SourceKind string       `json:"source_kind"`
	Repository string       `json:"repository,omitempty"`
	Branch     string       `json:"branch,omitempty"`
	SourceDir  string       `json:"-"`
	Profile    byoa.Profile `json:"profile"`
	EnvNames   []string     `json:"env_names"`
	CreatedAt  time.Time    `json:"created_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
	LastRun    string       `json:"last_run,omitempty"`
	LastKind   string       `json:"last_kind,omitempty"`
}

type storedApplication struct {
	applicationRecord
	SourceDir string `json:"source_dir"`
	EnvCipher string `json:"env_cipher"`
	Nonce     string `json:"nonce"`
}

type applicationsFile struct {
	Version int                 `json:"version"`
	Apps    []storedApplication `json:"apps"`
}

var applicationIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}$`)

func applicationsStorePath() (string, error) {
	if v := os.Getenv("HANDOFF_APPLICATIONS_STORE"); v != "" {
		return v, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "HandoffProof", "applications.json"), nil
}

func applicationsEncryptionKey(storePath string) ([]byte, error) {
	if v := os.Getenv("HANDOFF_APPLICATIONS_ENCRYPTION_KEY"); v != "" {
		if raw, err := base64.StdEncoding.DecodeString(v); err == nil && (len(raw) == 16 || len(raw) == 24 || len(raw) == 32) {
			return raw, nil
		}
		if raw, err := base64.RawStdEncoding.DecodeString(v); err == nil && (len(raw) == 16 || len(raw) == 24 || len(raw) == 32) {
			return raw, nil
		}
		sum := sha256.Sum256([]byte(v))
		return sum[:], nil
	}
	keyPath := storePath + ".key"
	if raw, err := os.ReadFile(keyPath); err == nil {
		if decoded, derr := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw))); derr == nil && len(decoded) == 32 {
			return decoded, nil
		}
		if len(raw) == 32 {
			return raw, nil
		}
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, []byte(base64.StdEncoding.EncodeToString(key)), 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func encryptEnv(storePath string, values map[string]string) (string, string, error) {
	key, err := applicationsEncryptionKey(storePath)
	if err != nil {
		return "", "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", "", err
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(nonce), base64.StdEncoding.EncodeToString(gcm.Seal(nil, nonce, raw, nil)), nil
}

func decryptEnv(storePath, nonceB64, cipherB64 string) (map[string]string, error) {
	if nonceB64 == "" && cipherB64 == "" {
		return map[string]string{}, nil
	}
	key, err := applicationsEncryptionKey(storePath)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce, err := base64.StdEncoding.DecodeString(nonceB64)
	if err != nil {
		return nil, err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(cipherB64)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	if err := json.Unmarshal(plain, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *server) loadApplications() ([]storedApplication, string, error) {
	path, err := applicationsStorePath()
	if err != nil {
		return nil, "", err
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, path, nil
	}
	if err != nil {
		return nil, path, err
	}
	var file applicationsFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, path, err
	}
	return file.Apps, path, nil
}

func (s *server) saveApplications(apps []storedApplication) error {
	path, err := applicationsStorePath()
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(applicationsFile{Version: 1, Apps: apps}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func publicApplication(a storedApplication) applicationRecord {
	rec := a.applicationRecord
	names := append([]string(nil), rec.EnvNames...)
	sort.Strings(names)
	rec.EnvNames = names
	return rec
}

func findApplication(apps []storedApplication, id string) (storedApplication, int, bool) {
	for i, a := range apps {
		if a.ID == id {
			return a, i, true
		}
	}
	return storedApplication{}, -1, false
}
