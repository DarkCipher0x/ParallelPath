package help

import (
	"os"
	"strings"
	"testing"
)

// The embedded knowledge must be exactly the published guide and FAQ (tools/build-release.ps1 copies them).
func TestKnowledgeMatchesDocs(t *testing.T) {
	for _, f := range []string{"USER-GUIDE.md", "FAQ.md"} {
		doc, err := os.ReadFile("../../docs/" + f)
		if err != nil {
			t.Fatal(err)
		}
		kbf, err := kb.ReadFile("kb/" + f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.ReplaceAll(string(doc), "\r\n", "\n") != strings.ReplaceAll(string(kbf), "\r\n", "\n") {
			t.Errorf("engine/help/kb/%s differs from docs/%s: copy it again", f, f)
		}
	}
}

// Everyday questions find the right answer first.
func TestAsk(t *testing.T) {
	for q, want := range map[string]string{
		"how do I message just my ex privately": "message just one person",
		"who can my kid call":                   "Who can my child call",
		"forgot my password":                    "forgot my passphrase",
		"make a pdf for the judge":              "report for court",
		"add my phone":                          "add my phone",
		"can the server read my messages":       "server or its owner read",
		"invite a lawyer":                       "mediator or solicitor",
		"why did it say my message was rude":    "tone check",
		"my phone was stolen":                   "phone was stolen",
		"browser shows a certificate warning":   "certificate warning",
	} {
		a := Ask(q, 3)
		if len(a) == 0 {
			t.Errorf("%q: no answer", q)
			continue
		}
		if !strings.Contains(strings.ToLower(a[0].Title), strings.ToLower(want)) {
			t.Errorf("%q: first answer %q, want one about %q (others: %v)", q, a[0].Title, want, titles(a))
		}
	}
	if a := Ask("the of and", 3); len(a) != 0 {
		t.Errorf("stop words alone found %v", titles(a))
	}
}

func titles(a []Answer) []string {
	var out []string
	for _, x := range a {
		out = append(out, x.Title)
	}
	return out
}

func BenchmarkAsk(b *testing.B) {
	Ask("warm up", 1)
	for b.Loop() {
		Ask("how do I message just the other parent privately", 4)
	}
}
