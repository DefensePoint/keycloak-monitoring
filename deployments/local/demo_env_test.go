package local

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/auth"
)

// The values make docker-demo-up needs and fills when they are empty.
var demoEnvKeys = []string{
	"MONITORING_AUTH_SIMPLE_DEFAULT_PASS",
	"BUNDLED_DB_PASSWORD",
	"BUNDLED_KC_ADMIN_PASSWORD",
	"BUNDLED_KC1_CLIENT_SECRET",
	"BUNDLED_KC2_CLIENT_SECRET",
	"BUNDLED_SESSION_SECRET",
	// The AMFA tenant's (make docker-demo-up AMFA=1).
	"BUNDLED_KC3_CLIENT_SECRET",
	"BUNDLED_AMFA_DB_PASSWORD",
}

func runDemoEnv(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("sh", "demo-env.sh", filepath.Join(dir, ".env"), "../../.env.example", "kmt-demo-test-none")
	// Without docker on PATH the script skips its retained-volume check.
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("demo-env.sh failed: %v\n%s", err, out)
	}
	return string(out)
}

func readEnv(t *testing.T, path string) (map[string]string, string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		if key, value, ok := strings.Cut(line, "="); ok && !strings.HasPrefix(line, "#") {
			values[key] = value
		}
	}
	return values, string(raw)
}

func TestDemoEnv_CreatesEnvAndFillsEveryDemoValue(t *testing.T) {
	dir := t.TempDir()
	out := runDemoEnv(t, dir)
	values, _ := readEnv(t, filepath.Join(dir, ".env"))

	seen := map[string]string{}
	for _, key := range demoEnvKeys {
		value := values[key]
		if value == "" {
			t.Fatalf("%s left empty", key)
		}
		if strings.ContainsAny(value, "$'\" ") {
			t.Fatalf("%s=%q has a character Compose or the shell would change", key, value)
		}
		if other, dup := seen[value]; dup {
			t.Fatalf("%s reuses the value of %s", key, other)
		}
		seen[value] = key
	}
	if err := auth.ValidatePassword(values["MONITORING_AUTH_SIMPLE_DEFAULT_PASS"], auth.DefaultPasswordRequirements()); err != nil {
		t.Fatalf("generated dashboard password is refused by the server: %v", err)
	}
	if values["MONITORING_AUTH_SIMPLE_DEFAULT_USER"] != "operator" {
		t.Fatal("the rest of .env.example was not copied")
	}
	if !strings.Contains(out, values["MONITORING_AUTH_SIMPLE_DEFAULT_PASS"]) {
		t.Fatal("the generated dashboard password was not shown")
	}
}

func TestDemoEnv_KeepsValuesAlreadySet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	example, err := os.ReadFile("../../.env.example")
	if err != nil {
		t.Fatal(err)
	}
	mine := strings.Replace(string(example), "BUNDLED_DB_PASSWORD=\n", "BUNDLED_DB_PASSWORD='my-$db'\n", 1)
	mine = strings.Replace(mine, "MONITORING_AUTH_SIMPLE_DEFAULT_PASS=\n", "MONITORING_AUTH_SIMPLE_DEFAULT_PASS=Mine!Pass-2026x\n", 1)
	if err := os.WriteFile(path, []byte(mine), 0o600); err != nil {
		t.Fatal(err)
	}

	out := runDemoEnv(t, dir)
	values, raw := readEnv(t, path)
	if values["BUNDLED_DB_PASSWORD"] != "'my-$db'" || values["MONITORING_AUTH_SIMPLE_DEFAULT_PASS"] != "Mine!Pass-2026x" {
		t.Fatal("a value that was already set was changed")
	}
	if strings.Contains(out, "Mine!Pass-2026x") {
		t.Fatal("a password the user set was printed")
	}
	if values["BUNDLED_KC1_CLIENT_SECRET"] == "" {
		t.Fatal("an empty value next to set ones was not filled")
	}

	// Every line other than the filled ones is left exactly as it was.
	before := strings.Split(mine, "\n")
	after := strings.Split(raw, "\n")
	if len(before) != len(after) {
		t.Fatalf("line count changed: %d -> %d", len(before), len(after))
	}
	for i := range before {
		key, _, _ := strings.Cut(before[i], "=")
		if before[i] != after[i] && !strings.HasPrefix(before[i], key+"=") {
			t.Fatalf("line %d changed: %q -> %q", i+1, before[i], after[i])
		}
	}

	again := runDemoEnv(t, dir)
	_, rawAgain := readEnv(t, path)
	if rawAgain != raw {
		t.Fatal("a second run changed .env")
	}
	if strings.Contains(again, "generated") {
		t.Fatalf("a second run reported generating values:\n%s", again)
	}
}

func TestDemoEnv_AddsDemoKeysMissingFromAnOlderEnv(t *testing.T) {
	// The second file has no newline after its last line.
	for _, old := range []string{
		"MONITORING_AUTH_SIMPLE_DEFAULT_USER=operator\nMONITORING_AUTH_SIMPLE_DEFAULT_EMAIL=operator@example.com\nMONITORING_AUTH_SIMPLE_DEFAULT_PASS=Mine!Pass-2026x\n",
		"MONITORING_AUTH_SIMPLE_DEFAULT_USER=operator\nMONITORING_AUTH_SIMPLE_DEFAULT_PASS=Mine!Pass-2026x",
	} {
		dir := t.TempDir()
		path := filepath.Join(dir, ".env")
		if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
			t.Fatal(err)
		}

		runDemoEnv(t, dir)
		values, raw := readEnv(t, path)
		if !strings.HasPrefix(raw, strings.TrimSuffix(old, "\n")+"\n") {
			t.Fatalf("the existing lines were not kept at the top:\n%s", raw)
		}
		if values["MONITORING_AUTH_SIMPLE_DEFAULT_PASS"] != "Mine!Pass-2026x" {
			t.Fatal("the last existing line was changed")
		}
		for _, key := range demoEnvKeys[1:] {
			if values[key] == "" {
				t.Fatalf("%s was not added", key)
			}
		}
	}
}

func TestDemoEnv_FillsValuesWrittenAsEmptyQuotes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	example, err := os.ReadFile("../../.env.example")
	if err != nil {
		t.Fatal(err)
	}
	mine := strings.Replace(string(example), "BUNDLED_DB_PASSWORD=\n", "BUNDLED_DB_PASSWORD=''\n", 1)
	mine = strings.Replace(mine, "BUNDLED_SESSION_SECRET=\n", "BUNDLED_SESSION_SECRET=\"\"\n", 1)
	if err := os.WriteFile(path, []byte(mine), 0o600); err != nil {
		t.Fatal(err)
	}

	runDemoEnv(t, dir)
	values, _ := readEnv(t, path)
	for _, key := range []string{"BUNDLED_DB_PASSWORD", "BUNDLED_SESSION_SECRET"} {
		if v := values[key]; v == "" || v == "''" || v == `""` {
			t.Fatalf("%s written as empty quotes was not filled: %q", key, v)
		}
	}
}

// runDemoEnvWithVolumes runs the script with a docker command that lists the
// given volume names, the only docker call the script makes.
func runDemoEnvWithVolumes(t *testing.T, dir string, volumes ...string) string {
	t.Helper()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	fake := "#!/bin/sh\nprintf '%s\\n' " + strings.Join(append([]string{"''"}, volumes...), " ") + "\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "demo-env.sh", filepath.Join(dir, ".env"), "../../.env.example", "kmt-demo")
	cmd.Env = []string{"PATH=" + bin + ":/usr/bin:/bin"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("demo-env.sh failed: %v\n%s", err, out)
	}
	return string(out)
}

func TestDemoEnv_WarnsOnlyAboutVolumesWhosePasswordsItGenerated(t *testing.T) {
	setAll := func(t *testing.T, dir string, except ...string) {
		raw, err := os.ReadFile("../../.env.example")
		if err != nil {
			t.Fatal(err)
		}
		env := string(raw)
		skip := map[string]bool{}
		for _, key := range except {
			skip[key] = true
		}
		for _, key := range demoEnvKeys {
			if !skip[key] {
				env = strings.Replace(env, key+"=\n", key+"=already-set-1A!\n", 1)
			}
		}
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	demoVolumes := []string{"kmt-demo_kmt-postgres-data", "kmt-demo_kmt-keycloak-db-1-data"}
	amfaVolume := "kmt-demo_kmt-amfa-postgres-data"

	for _, c := range []struct {
		name    string
		missing []string
		volumes []string
		warn    bool
	}{
		// An .env from before the AMFA tenant: its AMFA values are new, but
		// there is no AMFA database for them to disagree with.
		{"new AMFA values, no AMFA volume", demoEnvKeys[6:], demoVolumes, false},
		{"new AMFA values, AMFA volume kept", demoEnvKeys[6:], append(demoVolumes, amfaVolume), true},
		{"new demo database password, demo volumes kept", []string{"BUNDLED_DB_PASSWORD"}, demoVolumes, true},
		{"new Keycloak admin password, only the AMFA volume kept", []string{"BUNDLED_KC_ADMIN_PASSWORD"}, []string{amfaVolume}, true},
		{"new client secret only", []string{"BUNDLED_KC3_CLIENT_SECRET"}, append(demoVolumes, amfaVolume), false},
		{"new demo database password, no volumes", []string{"BUNDLED_DB_PASSWORD"}, nil, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			setAll(t, dir, c.missing...)
			out := runDemoEnvWithVolumes(t, dir, c.volumes...)
			if got := strings.Contains(out, "WARNING"); got != c.warn {
				t.Fatalf("warning = %v, want %v:\n%s", got, c.warn, out)
			}
		})
	}
}
