package mcp

import "testing"

func TestCleanStringStripsControlCharacters(t *testing.T) {
	in := "a\x00b\x1bc\rd\x7fe"
	if got := CleanString(in); got != "abcde" {
		t.Fatalf("got %q, want %q", got, "abcde")
	}
}

func TestCleanStringKeepsNewlineAndTab(t *testing.T) {
	in := "line1\nline2\tend"
	if got := CleanString(in); got != in {
		t.Fatalf("got %q, want %q", got, in)
	}
}

func TestCleanStringStripsBidiOverrides(t *testing.T) {
	in := "user\u202ename\u202c\u2066x\u2069\u061c"
	if got := CleanString(in); got != "usernamex" {
		t.Fatalf("got %q, want %q", got, "usernamex")
	}
}

func TestCleanStringStripsZeroWidthCharacters(t *testing.T) {
	in := "a\u200bb\u200cc\u200dd\u2060e\ufefff"
	if got := CleanString(in); got != "abcdef" {
		t.Fatalf("got %q, want %q", got, "abcdef")
	}
}

func TestCapStringTruncatesRunes(t *testing.T) {
	if got := CapString("héllö wörld", 5); got != "héllö" {
		t.Fatalf("got %q, want %q", got, "héllö")
	}
	if got := CapString("short", 100); got != "short" {
		t.Fatalf("got %q, want %q", got, "short")
	}
	if got := CapString("anything", 0); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestClean(t *testing.T) {
	in := "ab\u202ecd\x00efgh"
	if got := Clean(in, 4); got != "abcd" {
		t.Fatalf("got %q, want %q", got, "abcd")
	}
}

func TestCleanSlice(t *testing.T) {
	got := CleanSlice([]string{"a\u200bb", "c\x00d"}, 10)
	if len(got) != 2 || got[0] != "ab" || got[1] != "cd" {
		t.Fatalf("got %v", got)
	}
	if CleanSlice(nil, 10) != nil {
		t.Fatal("nil input should stay nil")
	}
}
