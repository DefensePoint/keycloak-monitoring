package mcp

import (
	"context"
	"strings"
	"testing"
)

func TestToolOutputSchemasHaveNoCredentialFields(t *testing.T) {
	ts := newTestServer(t, acceptingValidator("pat_good"))
	session := connectClient(t, ts, "pat_good")

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list failed: %v", err)
	}
	if len(res.Tools) == 0 {
		t.Fatal("tools/list returned no tools")
	}

	seen := map[string]bool{}
	for _, tool := range res.Tools {
		if tool.OutputSchema == nil {
			t.Errorf("tool %s declares no output schema, so its fields go unchecked", tool.Name)
		}
		visitSchemaProperties(tool.OutputSchema, func(name string) {
			seen[name] = true
			lower := strings.ToLower(name)
			for _, marker := range []string{"secret", "password", "credential", "private"} {
				if strings.Contains(lower, marker) {
					t.Errorf("tool %s returns a %q field", tool.Name, name)
				}
			}
		})
	}
	if !seen["alert_id"] {
		t.Fatalf("schema walk never reached the alert tools' alert_id, so it checked nothing")
	}
}

func visitSchemaProperties(node any, visit func(name string)) {
	switch n := node.(type) {
	case map[string]any:
		for key, value := range n {
			if properties, ok := value.(map[string]any); ok && key == "properties" {
				for name := range properties {
					visit(name)
				}
			}
			visitSchemaProperties(value, visit)
		}
	case []any:
		for _, value := range n {
			visitSchemaProperties(value, visit)
		}
	}
}
