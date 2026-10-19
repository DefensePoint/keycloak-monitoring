package auth

import (
	"crypto/rand"
	"encoding/base64"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A session cookie is signed with the session secret, so a secret printed in
// this repository lets anyone sign a cookie for any user.
func TestValidateSessionSecret_RefusesEmptyAndPublished(t *testing.T) {
	refused := []string{"", "   ", "\t\n", "${MONITORING_AUTH_SESSION_SECRET}", "  ${SESSION_SECRET}  ", "prefix-${X}"}
	for _, p := range publishedSessionSecrets {
		refused = append(refused, p, strings.ToUpper(p), "  "+p+"\n")
	}
	for _, s := range refused {
		err := ValidateSessionSecret(s)
		if err == nil {
			t.Errorf("secret %q was accepted", s)
			continue
		}
		if !strings.Contains(err.Error(), "MONITORING_AUTH_SESSION_SECRET") {
			t.Errorf("error for %q does not say how to fix it: %v", s, err)
		}
	}
}

func TestValidateSessionSecret_AcceptsAGeneratedSecret(t *testing.T) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSessionSecret(base64.StdEncoding.EncodeToString(buf)); err != nil {
		t.Fatalf("generated secret refused: %v", err)
	}
	// Placeholder-only by design: a short private secret still starts.
	if err := ValidateSessionSecret("short-but-private"); err != nil {
		t.Fatalf("short private secret refused: %v", err)
	}
}

// The repository must not publish a usable session secret again. Every value
// on the refusal list was once shown as an example; a new example should tell
// the reader to generate one instead.
func TestRepositoryPublishesNoSessionSecret(t *testing.T) {
	root := filepath.Join("..")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "dist", "build", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		ext := filepath.Ext(name)
		if ext != ".md" && ext != ".yaml" && ext != ".yml" && ext != ".example" && !strings.HasPrefix(name, ".env") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, p := range publishedSessionSecrets {
			if strings.Contains(string(data), p) {
				t.Errorf("%s publishes the session secret %q", path, p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
