package app

import (
	"strings"
	"testing"
)

func TestCheckTone(t *testing.T) {
	for _, c := range []struct {
		text, level string
		kinds       []string
	}{
		{"Can we swap Saturday for Sunday this week?", "calm", nil},
		{"I'll drop Mia at school at 8:30. Her PE kit is in the blue bag.", "calm", nil},
		{"The GP appointment is at 9 (UK time).", "calm", nil},
		{"You ALWAYS do this", "careful", nil},
		{"You always do this, it's your fault", "careful", []string{"blame"}},
		{"WHY ARE YOU LATE AGAIN", "careful", []string{"shouting"}},
		{"Where are you???", "careful", []string{"punctuation"}},
		{"You're useless and a liar", "heated", []string{"insult"}},
		{"Pay it or else. See you in court", "heated", []string{"threat"}},
		{"whatever, not my problem", "careful", []string{"dismissive"}},
	} {
		r := CheckTone(c.text)
		if r.Level != c.level {
			t.Errorf("%q: level %s, want %s (%+v)", c.text, r.Level, c.level, r.Issues)
		}
		for _, k := range c.kinds {
			found := false
			for _, i := range r.Issues {
				found = found || i.Kind == k
			}
			if !found {
				t.Errorf("%q: no %s issue in %+v", c.text, k, r.Issues)
			}
		}
	}
}

// The model catches hostility the word lists don't, and the calmer version keeps the arrangements.
func TestToneModel(t *testing.T) {
	r := CheckTone("You are a horrible mother and a complete failure as a human being.")
	if r.Level == "calm" {
		t.Errorf("model missed plain-word hostility: %+v", r)
	}
	r = CheckTone("Pick-up is at 5 on Friday. You're a useless idiot!!! DON'T BE LATE AGAIN!!!")
	if r.Calmer == "" || !strings.Contains(r.Calmer, "Pick-up is at 5 on Friday.") || strings.Contains(r.Calmer, "idiot") ||
		strings.Contains(r.Calmer, "!!!") || strings.Contains(r.Calmer, "DON'T") {
		t.Errorf("calmer: %q", r.Calmer)
	}
	if r := CheckTone("Thanks - I'll bring her PE kit on Monday."); r.Level != "calm" || r.Calmer != "" {
		t.Errorf("calm message flagged: %+v", r)
	}
}

// The live tone bar rises with hostility.
func TestToneMeter(t *testing.T) {
	calm := CheckTone("Thanks - I'll bring her PE kit on Monday.").Meter
	careful := CheckTone("You always forget the PE kit.").Meter
	hot := CheckTone("You're a useless idiot and a liar!!!").Meter
	if !(calm < 0.25 && careful > calm && hot > careful && hot >= 0.75) {
		t.Errorf("meter: calm %.2f careful %.2f heated %.2f", calm, careful, hot)
	}
}
