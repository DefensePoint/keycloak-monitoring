package logger

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestPinLevelSurvivesStricterGlobalLevel(t *testing.T) {
	prev := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })

	var plain bytes.Buffer
	New(zerolog.New(&plain)).Info("filtered")
	if plain.Len() != 0 {
		t.Fatalf("unpinned info line should be dropped at global error level, got %q", plain.String())
	}

	var buf bytes.Buffer
	log := New(zerolog.New(&buf)).PinLevel("info").WithComponent("audit")
	log.Info("kept", Str("k", "v"))
	log.Debug("below the pin")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected exactly one line, got %q", buf.String())
	}
	var line map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &line); err != nil {
		t.Fatalf("unparseable line %q: %v", lines[0], err)
	}
	if line["level"] != "info" || line["message"] != "kept" || line["k"] != "v" || line["component"] != "audit" {
		t.Fatalf("unexpected line: %v", line)
	}
}

func TestPinLevelUsesNormalPathWhenGlobalAllows(t *testing.T) {
	prev := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })

	var buf bytes.Buffer
	New(zerolog.New(&buf)).PinLevel("info").Warn("w")

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("unparseable line %q: %v", buf.String(), err)
	}
	if line["level"] != "warn" {
		t.Fatalf("level = %v, want warn", line["level"])
	}
}
