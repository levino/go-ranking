package i18n

import "testing"

func TestParse(t *testing.T) {
	cases := map[string]Lang{
		"de": DE, "DE": DE, " en ": EN, "EN": EN,
		"fr": Default, "": Default, "english": Default,
	}
	for in, want := range cases {
		if got := Parse(in); got != want {
			t.Errorf("Parse(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFromAcceptLanguage(t *testing.T) {
	cases := map[string]Lang{
		"en-US,en;q=0.9":    EN,
		"de-DE,de;q=0.8":    DE,
		"fr-FR,fr;q=0.9,en": EN,
		"":                  Default,
		"*":                 Default,
	}
	for in, want := range cases {
		if got := FromAcceptLanguage(in); got != want {
			t.Errorf("FromAcceptLanguage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTFallbackAndFormat(t *testing.T) {
	en := New(EN)
	if got := en.T("action.save"); got != "Save" {
		t.Errorf("EN action.save = %q", got)
	}
	// Unknown key returns the key itself.
	if got := en.T("nope.missing"); got != "nope.missing" {
		t.Errorf("missing key = %q", got)
	}
	// Format args are applied.
	if got := en.T("play.steps", 3); got != "3 steps" {
		t.Errorf("play.steps = %q", got)
	}
}
