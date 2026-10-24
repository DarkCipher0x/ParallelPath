package tone

import (
	"testing"
	"time"
)

func TestModelLoadsAndScores(t *testing.T) {
	t0 := time.Now()
	m := Default()
	if m == nil {
		t.Fatal("the built-in model didn't load")
	}
	t.Logf("load: %s", time.Since(t0))
	calm := []string{
		"Pick-up from school is at 3:30 tomorrow.",
		"Can we swap Saturday for Sunday this week?",
		"Mia has a dentist appointment on the 4th at 9am, I'll take her.",
		"Thanks for sorting the school trip money.",
	}
	hostile := []string{
		"You are a pathetic excuse for a parent and everyone knows it.",
		"Shut up you stupid idiot",
		"You disgusting piece of garbage",
	}
	for _, s := range calm {
		if sc := m.Scores(s); sc[0] > 0.3 || sc[4] > 0.3 {
			t.Errorf("calm message scored high: %q -> toxicity %.2f insult %.2f", s, sc[0], sc[4])
		}
	}
	for _, s := range hostile {
		if sc := m.Scores(s); sc[0] < 0.5 && sc[4] < 0.5 {
			t.Errorf("hostile message scored low: %q -> toxicity %.2f insult %.2f", s, sc[0], sc[4])
		}
	}
	// Disguised spelling is undone before scoring.
	if a, b := m.Scores("you are an @sshole")[4], m.Scores("you are an asshole")[4]; a < b-0.05 {
		t.Errorf("disguise not undone: %.2f vs %.2f", a, b)
	}
	if c := m.Culprits("You are a pathetic excuse for a parent", 4, 3); len(c) == 0 || c[0] != "pathetic" {
		t.Errorf("culprits: %v", c)
	}
}

func BenchmarkScores(b *testing.B) {
	m := Default()
	msg := "I told you three times the pick-up is at 5, you never listen and it's always the same with you."
	for b.Loop() {
		m.Scores(msg)
	}
}
