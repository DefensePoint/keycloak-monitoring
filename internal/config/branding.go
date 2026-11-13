package config

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Default branding: the names KMT shows when the config file sets none.
const (
	DefaultBrandingName      = "Keycloak Monitoring Tool"
	DefaultBrandingShortName = "KMT"
	DefaultBrandingLogoText  = "Keycloak"

	// DefaultBrandingVendor and DefaultBrandingContactEmail are the
	// credits a PDF report carries when the config file sets no vendor.
	DefaultBrandingVendor       = "DefensePoint"
	DefaultBrandingContactEmail = "security@defensepoint.com"
)

// slugFallback names the product in identifiers when its name has no letter
// or digit that can be written in ASCII.
const slugFallback = "monitoring"

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
	// Vendor is credited as the author of PDF reports ("<vendor> Team")
	// and named as their contact ("<vendor> Security Team"). Only reports
	// show it, so /auth/config leaves it out.
	Vendor string `mapstructure:"vendor" json:"-"`
	// ContactEmail is the address PDF reports give for the vendor. Empty
	// with a vendor set leaves the address out of the report.
	ContactEmail string `mapstructure:"contact_email" json:"-"`
}

// WithDefaults returns the names trimmed, with the default back for any
// left empty, so an empty value in the file or the environment cannot
// blank the product name out. The default contact address only goes with
// the default vendor: a deployment that names its own vendor gives its own
// address or none.
func (b BrandingConfig) WithDefaults() BrandingConfig {
	vendor := oneLineOr(b.Vendor, "")
	email := oneLineOr(b.ContactEmail, "")
	if vendor == "" {
		vendor = DefaultBrandingVendor
		if email == "" {
			email = DefaultBrandingContactEmail
		}
	}
	return BrandingConfig{
		Name:         oneLineOr(b.Name, DefaultBrandingName),
		ShortName:    oneLineOr(b.ShortName, DefaultBrandingShortName),
		LogoText:     oneLineOr(b.LogoText, DefaultBrandingLogoText),
		Vendor:       vendor,
		ContactEmail: email,
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
	return slugify(oneLineOr(b.Name, DefaultBrandingName))
}

// FilePrefix starts the names of the files KMT produces: "kmt", as in
// kmt-report-<tenant>-<from>-to-<to>.pdf.
func (b BrandingConfig) FilePrefix() string {
	return slugify(oneLineOr(b.ShortName, DefaultBrandingShortName))
}

// foldedLetters are the letters that do not break down into an ASCII letter
// and accents, written the way they usually are in ASCII. The web UI's
// filePrefix folds the same ones.
var foldedLetters = strings.NewReplacer(
	"ß", "ss", "æ", "ae", "ø", "o", "œ", "oe", "đ", "d", "ł", "l", "þ", "th", "ı", "i",
)

// slugify lower-cases s, drops accents ("Überwachung" gives
// "uberwachung") and joins its ASCII letters and digits with single dashes.
// A name with none, as one in Cyrillic or Chinese, gives "monitoring"
// rather than a name the deployment did not choose.
func slugify(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.Is(unicode.M, r) {
			return -1
		}
		return r
	}, norm.NFD.String(strings.ToLower(s)))
	s = foldedLetters.Replace(s)
	words := strings.FieldsFunc(s, func(r rune) bool {
		return r > unicode.MaxASCII || !(unicode.IsLetter(r) || unicode.IsDigit(r))
	})
	if len(words) == 0 {
		return slugFallback
	}
	return strings.Join(words, "-")
}
