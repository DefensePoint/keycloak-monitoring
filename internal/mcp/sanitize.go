package mcp

import (
	"strings"
	"unicode"
)

// strippedRunes are invisible characters that can smuggle instructions or
// reorder displayed text inside strings handed to an MCP client: Unicode
// bidi controls, zero-width characters and the BOM.
var strippedRunes = map[rune]bool{
	'\u061c': true, // arabic letter mark
	'\u200b': true, // zero width space
	'\u200c': true, // zero width non-joiner
	'\u200d': true, // zero width joiner
	'\u200e': true, // left-to-right mark
	'\u200f': true, // right-to-left mark
	'\u202a': true, // left-to-right embedding
	'\u202b': true, // right-to-left embedding
	'\u202c': true, // pop directional formatting
	'\u202d': true, // left-to-right override
	'\u202e': true, // right-to-left override
	'\u2060': true, // word joiner
	'\u2066': true, // left-to-right isolate
	'\u2067': true, // right-to-left isolate
	'\u2068': true, // first strong isolate
	'\u2069': true, // pop directional isolate
	'\ufeff': true, // byte order mark
}

// CleanString removes control characters (keeping newline and tab), bidi
// overrides and zero-width characters from a DTO string.
func CleanString(s string) string {
	return strings.Map(func(r rune) rune {
		if strippedRunes[r] {
			return -1
		}
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
}

// CapString truncates a string to at most max runes.
func CapString(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

// Clean applies CleanString then CapString.
func Clean(s string, max int) string {
	return CapString(CleanString(s), max)
}

// CleanSlice applies Clean to every element.
func CleanSlice(values []string, max int) []string {
	if values == nil {
		return nil
	}
	cleaned := make([]string, len(values))
	for i, v := range values {
		cleaned[i] = Clean(v, max)
	}
	return cleaned
}
