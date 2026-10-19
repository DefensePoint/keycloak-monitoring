package local

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// The base local stack gives the API server the development database
// password. Compose merges a service's environment from every file rather
// than replacing it, so the demo inherited that password, which beat the
// demo config's own and left the API server unable to reach the demo's
// database. config.bundled.yaml looked right on its own: only the merged
// Compose view shows what a container really gets.

// serviceEnvironments returns each service's environment in a Compose file.
// A !reset or !override environment replaces, rather than merges with, the
// base file's.
func serviceEnvironments(t *testing.T, path string) map[string]struct {
	env     map[string]string
	replace bool
} {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Services map[string]struct {
			Environment yaml.Node `yaml:"environment"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	out := map[string]struct {
		env     map[string]string
		replace bool
	}{}
	for name, svc := range doc.Services {
		n := svc.Environment
		env := map[string]string{}
		switch n.Kind {
		case yaml.MappingNode:
			for i := 0; i+1 < len(n.Content); i += 2 {
				env[n.Content[i].Value] = n.Content[i+1].Value
			}
		case yaml.SequenceNode:
			for _, item := range n.Content {
				k, v, _ := strings.Cut(item.Value, "=")
				env[k] = v
			}
		}
		out[name] = struct {
			env     map[string]string
			replace bool
		}{env, n.Tag == "!reset" || n.Tag == "!override"}
	}
	return out
}

var databasePasswordKey = regexp.MustCompile(`^MONITORING_(MCP_)?DATABASE_PASSWORD$`)

func TestBundledDemo_ServicesGetTheDemoDatabasePassword(t *testing.T) {
	base := serviceEnvironments(t, "docker-compose.yml")
	demo := serviceEnvironments(t, "docker-compose.bundled.yml")

	checked := 0
	for name, overlay := range demo {
		merged := map[string]string{}
		if !overlay.replace {
			for k, v := range base[name].env {
				merged[k] = v
			}
		}
		for k, v := range overlay.env {
			merged[k] = v
		}
		for k, v := range merged {
			if !databasePasswordKey.MatchString(k) {
				continue
			}
			checked++
			if !strings.Contains(v, "${BUNDLED_DB_PASSWORD") {
				t.Errorf("the demo's %s gets %s=%q, not the demo database's password: "+
					"it overrides config.bundled.yaml and the server cannot connect", name, k, v)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no demo service sets a database password; this test checks nothing")
	}
}
