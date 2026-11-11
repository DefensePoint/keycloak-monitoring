package config

import (
	"strings"
	"unicode"
)

// Default branding: the names KMT shows when the config file sets none.
const (
	DefaultBrandingName      = "Keycloak Monitoring Tool"
	DefaultBrandingShortName = "KMT"
	DefaultBrandingLogoText  = "Keycloak"
)

// BrandingConfig names the product in everything a user sees: the web UI,
// alert emails, Slack messages, GitLab issues, PDF reports and the MCP
// server. A deployment that ships KMT under its own name sets it here.
type BrandingConfig struct {
	// Name is the full product name, as in "Keycloak Monitoring Tool".
	Name string `mapstructure:"name" json:"name"`
	// ShortName is used where the full name is too long, as in "Can't
	// reach the KMT server.", and, lower-cased, in report file names.
	ShortName string `mapstructure:"short_name" json:"short_name"`
	// LogoText is the word shown next to the logo.
	LogoText string `mapstructure:"logo_text" json:"logo_text"`
}

// WithDefaults returns the names trimmed, with the default back for any
// left empty, so an empty value in the file or the environment cannot
// blank the product name out.
func (b BrandingConfig) WithDefaults() BrandingConfig {
	return BrandingConfig{
		Name:      oneLineOr(b.Name, DefaultBrandingName),
		ShortName: oneLineOr(b.ShortName, DefaultBrandingShortName),
		LogoText:  oneLineOr(b.LogoText, DefaultBrandingLogoText),
	}
}

// applyBranding settles the branding after loading and hands the product
// name to the settings that default to it.
func applyBranding(c *AppConfig) {
	c.Branding = c.Branding.WithDefaults()
	c.Notifications.ProductName = c.Branding.Name
	if strings.TrimSpace(c.Notifications.Slack.Username) == "" {
		c.Notifications.Slack.Username = c.Branding.Name
	}
}

// oneLineOr returns s on one line and trimmed, or def when nothing is left.
// The names go into email headers, so a line break must not survive.
func oneLineOr(s, def string) string {
	s = strings.Join(strings.FieldsFunc(s, func(r rune) bool { return r == '\r' || r == '\n' }), " ")
	if s = strings.TrimSpace(s); s == "" {
		return def
	}
	return s
}

// Slug is the product name as an identifier: "keycloak-monitoring-tool".
func (b BrandingConfig) Slug() string {
	return slugify(oneLineOr(b.Name, DefaultBrandingName), "keycloak-monitoring-tool")
}

// FilePrefix starts the names of the files KMT produces: "kmt", as in
// kmt-report-<tenant>-<from>-to-<to>.pdf.
func (b BrandingConfig) FilePrefix() string {
	return slugify(oneLineOr(b.ShortName, DefaultBrandingShortName), "kmt")
}

// slugify lower-cases s and joins its letters and digits with single
// dashes. A name with no letter or digit at all gives def.
func slugify(s, def string) string {
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return r > unicode.MaxASCII || !(unicode.IsLetter(r) || unicode.IsDigit(r))
	})
	if len(words) == 0 {
		return def
	}
	return strings.Join(words, "-")
}
