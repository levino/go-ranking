// Package i18n holds the UI translation catalog and a tiny localizer.
//
// The catalog is a flat map of message id → per-language string. There
// are only two languages for now (German, the default, and English), so
// a hand-maintained map beats pulling in a full ICU/message-format
// dependency. Messages may carry fmt verbs; pass the arguments to T.
package i18n

import (
	"fmt"
	"strings"
)

// Lang is a supported UI language, identified by its ISO-639-1 code.
type Lang string

const (
	DE Lang = "de"
	EN Lang = "en"
)

// Default is the language used when no preference is known. German,
// because that's the school the app was built for.
const Default = DE

// Supported lists the selectable languages in display order, each with
// its endonym (the name of the language in that language).
var Supported = []struct {
	Lang Lang
	Name string
}{
	{DE, "Deutsch"},
	{EN, "English"},
}

// Parse normalises an arbitrary language string to a supported Lang,
// falling back to Default for anything unrecognised.
func Parse(s string) Lang {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "de":
		return DE
	case "en":
		return EN
	default:
		return Default
	}
}

// FromAcceptLanguage picks a language from an HTTP Accept-Language
// header. It only looks at the primary subtag of each entry and ignores
// q-weights — good enough for a two-language UI. Falls back to Default.
func FromAcceptLanguage(header string) Lang {
	for _, part := range strings.Split(header, ",") {
		tag := strings.TrimSpace(part)
		if i := strings.IndexByte(tag, ';'); i >= 0 {
			tag = tag[:i]
		}
		if i := strings.IndexByte(tag, '-'); i >= 0 {
			tag = tag[:i]
		}
		switch strings.ToLower(strings.TrimSpace(tag)) {
		case "de":
			return DE
		case "en":
			return EN
		}
	}
	return Default
}

// Localizer translates message ids into one language.
type Localizer struct{ lang Lang }

// New returns a Localizer for the given language.
func New(lang Lang) *Localizer { return &Localizer{lang: lang} }

// Lang reports the localizer's language code.
func (l *Localizer) Lang() Lang { return l.lang }

// T returns the message for id in the localizer's language. Missing
// translations fall back to the Default language, then to the id itself
// so a forgotten key is visible rather than silently blank. When args
// are supplied the message is treated as a printf format string.
func (l *Localizer) T(id string, args ...any) string {
	m, ok := messages[id]
	if !ok {
		return id
	}
	s, ok := m[l.lang]
	if !ok || s == "" {
		s = m[Default]
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}
