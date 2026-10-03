package main

import (
	"regexp"
	"sort"
	"testing"
)

var fmtVerb = regexp.MustCompile(`%[+#0-9.]*[a-zA-Z%]`)

// TestMessageSetsMatch checks that every language has every English key with
// the same formatting verbs, so a translated message can never break
// fmt.Sprintf or silently fall back to English.
func TestMessageSetsMatch(t *testing.T) {
	en := messages["en"]
	if len(en) == 0 {
		t.Fatal("no English messages")
	}
	langs := make([]string, 0, len(messages))
	for lang := range messages {
		langs = append(langs, lang)
	}
	sort.Strings(langs)
	if len(langs) < 9 {
		t.Fatalf("expected at least 9 languages, got %v", langs)
	}
	for _, lang := range langs {
		set := messages[lang]
		for key, english := range en {
			text, ok := set[key]
			if !ok {
				t.Errorf("%s: missing %q", lang, key)
				continue
			}
			if text == "" {
				t.Errorf("%s: empty %q", lang, key)
			}
			want, got := fmtVerb.FindAllString(english, -1), fmtVerb.FindAllString(text, -1)
			if len(want) != len(got) {
				t.Errorf("%s: %q has verbs %v, English has %v", lang, key, got, want)
			}
		}
		for key := range set {
			if _, ok := en[key]; !ok {
				t.Errorf("%s: extra key %q", lang, key)
			}
		}
	}
	// Switching to a known language takes effect; unknown ones are ignored.
	a := &App{}
	a.SetLanguage("de")
	if got := T("notConnected"); got != messages["de"]["notConnected"] {
		t.Fatalf("German message: %q", got)
	}
	a.SetLanguage("xx")
	if got := T("notConnected"); got != messages["de"]["notConnected"] {
		t.Fatalf("unknown language changed messages: %q", got)
	}
	a.SetLanguage("en")
}
