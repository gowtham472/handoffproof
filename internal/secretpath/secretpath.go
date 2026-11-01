// Package secretpath classifies file names that must never be read into
// model context, reports, or sandbox copies. Classification is by the
// meaning of the name (credential material), not by one fixed list:
// any .env variant, key material, or credential store is denied.
package secretpath

import (
	"path/filepath"
	"strings"
)

// Denied reports whether a file with this base name holds credential
// material and must not be read, listed, or copied. Only explicit
// documentation variants (.env.example/.sample/.template/.dist) pass.
func Denied(name string) bool {
	base := strings.ToLower(filepath.Base(name))
	switch base {
	case ".env.example", ".env.sample", ".env.template", ".env.dist":
		return false
	}
	if base == ".env" || strings.HasPrefix(base, ".env.") || strings.HasSuffix(base, ".env") {
		return true
	}
	switch base {
	case ".npmrc", ".netrc", ".git-credentials", ".pgpass", ".my.cnf",
		"credentials", "credentials.json", "id_rsa", "id_dsa", "id_ecdsa",
		"id_ed25519", "id_xmss", ".htpasswd", "secrets.json", "secret.json":
		return true
	}
	if strings.Contains(base, "service-account") || strings.Contains(base, "serviceaccount") {
		return true
	}
	switch filepath.Ext(base) {
	case ".pem", ".key", ".p12", ".pfx", ".keystore", ".jks", ".kdbx", ".ovpn", ".keytab":
		return true
	}
	return false
}
