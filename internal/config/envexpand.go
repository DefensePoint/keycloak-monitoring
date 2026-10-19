package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"
)

// placeholderPattern matches ${...} and its escaped form $${...}. Only the
// braced form is expanded: a bare $ is common in passwords and secrets, and
// expanding $NAME would silently corrupt them.
var (
	placeholderPattern = regexp.MustCompile(`\$?\$\{[^}]*\}`)
	envNamePattern     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// expandEnvPlaceholders replaces ${NAME} in the YAML document's scalar values
// with the NAME environment variable. $${NAME} is written as a literal
// ${NAME}. Keys and comments are left alone, so a commented-out example line
// never needs its variable set.
//
// A placeholder that cannot be expanded, because its variable is unset or it
// is not a plain name (shell forms such as ${NAME:-default}), keeps its
// literal text, as before expansion existed, and is reported in the returned
// warnings with its line. Refusing to start instead would take every service
// sharing the file down over a value that may not even be used.
//
// An expanded value is always a string, even unquoted: a secret of 0123 or
// true must not be re-read as a number or boolean. Fields that are numbers,
// booleans or durations still decode from that string.
// reportedPlaceholder is how an unsupported placeholder appears in a warning:
// the variable name it starts with, and an ellipsis for anything after it.
// What follows the name, such as a shell-style ${NAME:-default}, may be a
// secret, and the warning is printed at startup and logged.
func reportedPlaceholder(inner string) string {
	end := strings.IndexFunc(inner, func(r rune) bool { return !isEnvNameRune(r) })
	if end < 0 {
		return "${" + inner + "}"
	}
	return "${" + inner[:end] + "…}"
}

// isEnvNameRune reports whether r can appear in an environment variable name.
func isEnvNameRune(r rune) bool {
	return r == '_' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z'
}

func expandEnvPlaceholders(raw []byte) ([]byte, []string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, nil, fmt.Errorf("parse config file: %w", err)
	}
	if len(doc.Content) == 0 {
		return raw, nil, nil
	}

	missing := map[string][]int{}
	var invalid []string
	changed := false
	var walk func(n *yaml.Node)
	walk = func(n *yaml.Node) {
		switch n.Kind {
		case yaml.MappingNode:
			for i := 1; i < len(n.Content); i += 2 {
				walk(n.Content[i])
			}
		case yaml.SequenceNode, yaml.DocumentNode:
			for _, c := range n.Content {
				walk(c)
			}
		case yaml.ScalarNode:
			if !strings.Contains(n.Value, "${") {
				return
			}
			n.Value = placeholderPattern.ReplaceAllStringFunc(n.Value, func(m string) string {
				if strings.HasPrefix(m, "$$") {
					return m[1:]
				}
				name := m[2 : len(m)-1]
				if !envNamePattern.MatchString(name) {
					invalid = append(invalid, fmt.Sprintf("%s (line %d)", reportedPlaceholder(name), n.Line))
					return m
				}
				value, ok := os.LookupEnv(name)
				if !ok {
					missing[name] = append(missing[name], n.Line)
					return m
				}
				return value
			})
			n.Tag = "!!str"
			changed = true
		}
	}
	walk(&doc)

	var warnings []string
	if len(invalid) > 0 {
		warnings = append(warnings, fmt.Sprintf("config file has unsupported placeholders, kept as literal text; only ${NAME} is expanded (write $${...} for a literal): %s", strings.Join(invalid, ", ")))
	}
	if len(missing) > 0 {
		names := make([]string, 0, len(missing))
		for name := range missing {
			names = append(names, name)
		}
		sort.Strings(names)
		parts := make([]string, len(names))
		for i, name := range names {
			parts[i] = fmt.Sprintf("%s (line %s)", name, joinInts(missing[name]))
		}
		warnings = append(warnings, fmt.Sprintf("config file references environment variables that are not set, kept as literal ${NAME} text: %s", strings.Join(parts, ", ")))
	}
	if !changed {
		return raw, warnings, nil
	}

	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	if err := enc.Encode(&doc); err != nil {
		return nil, nil, fmt.Errorf("re-encode config file: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, nil, fmt.Errorf("re-encode config file: %w", err)
	}
	return out.Bytes(), warnings, nil
}

func joinInts(xs []int) string {
	s := make([]string, len(xs))
	for i, x := range xs {
		s[i] = fmt.Sprint(x)
	}
	return strings.Join(s, ", ")
}

// rereadWithEnvPlaceholders reloads the config file viper just read, with its
// ${NAME} placeholders expanded, and returns the placeholders it could not
// expand. Only YAML files are expanded.
func rereadWithEnvPlaceholders(v *viper.Viper) ([]string, error) {
	path := v.ConfigFileUsed()
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".yaml" && ext != ".yml" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	expanded, warnings, err := expandEnvPlaceholders(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	v.SetConfigType("yaml")
	if err := v.ReadConfig(bytes.NewReader(expanded)); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	return warnings, nil
}
