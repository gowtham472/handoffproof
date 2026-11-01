package secretpath

import "testing"

func TestDenied(t *testing.T) {
	denied := []string{
		".env", ".env.local", ".env.production",
		"server.pem", "tls.key", "cert.p12", "store.keystore",
		"id_rsa", "id_ed25519", ".npmrc", ".netrc", ".pgpass",
		"credentials.json", "gcp-service-account.json", "secrets.json",
		"client.ovpn", "app.keytab",
	}
	for _, n := range denied {
		if !Denied(n) {
			t.Errorf("Denied(%q) = false, want true", n)
		}
	}
	allowed := []string{".env.example", ".env.sample", "index.ts", "README.md", "docker-compose.yml", "backup.sh", "id_rsa.pub"}
	for _, n := range allowed {
		if Denied(n) {
			t.Errorf("Denied(%q) = true, want false", n)
		}
	}
}
