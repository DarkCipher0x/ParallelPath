// Command guides turns the Markdown guides in docs/ into printable HTML pages (ParallelPath theme), which
// build-release.ps1 prints to PDF with Edge. It handles the subset of Markdown the guides use: headings, paragraphs,
// lists (and task lists), tables, fenced code, links, **bold**, `code`, and horizontal rules. No dependencies.
//
//	go run ./tools/guides OUT_DIR docs/USER-GUIDE.md docs/ADMIN-GUIDE.md ...
//	go run ./tools/guides -template site.html OUT_DIR docs/... (web pages: {{TITLE}} and {{BODY}} filled in)
package main

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func main() {
	args := os.Args[1:]
	tmpl := ""
	if len(args) > 1 && args[0] == "-template" {
		b, err := os.ReadFile(args[1])
		if err != nil {
			fail(err)
		}
		tmpl, args = string(b), args[2:]
	}
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: guides [-template FILE] OUT_DIR FILE.md...")
		os.Exit(2)
	}
	out := args[0]
	if err := os.MkdirAll(out, 0o755); err != nil {
		fail(err)
	}
	for _, in := range args[1:] {
		src, err := os.ReadFile(in)
		if err != nil {
			fail(err)
		}
		title, body := render(string(src))
		name := strings.TrimSuffix(filepath.Base(in), ".md") + ".html"
		doc := page(title, body)
		if tmpl != "" {
			doc = strings.NewReplacer("{{TITLE}}", html.EscapeString(title), "{{BODY}}", body).Replace(tmpl)
		}
		if err := os.WriteFile(filepath.Join(out, name), []byte(doc), 0o644); err != nil {
			fail(err)
		}
		fmt.Println(filepath.Join(out, name))
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "guides:", err)
	os.Exit(1)
}

var (
	reCode   = regexp.MustCompile("`([^`]+)`")
	reBold   = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	reLink   = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)
	reOrder  = regexp.MustCompile(`^\d+\. `)
	reHeader = regexp.MustCompile(`^(#{1,4}) (.*)$`)
)

// inline formats one line of text: escape first, then code spans (their content stays literal), links and bold.
func inline(s string) string {
	var codes []string
	s = reCode.ReplaceAllStringFunc(s, func(m string) string {
		codes = append(codes, "<code>"+html.EscapeString(m[1:len(m)-1])+"</code>")
		return fmt.Sprintf("\x00%d\x00", len(codes)-1)
	})
	s = html.EscapeString(s)
	s = reLink.ReplaceAllStringFunc(s, func(m string) string {
		p := reLink.FindStringSubmatch(m)
		return `<a href="` + p[2] + `">` + p[1] + `</a>`
	})
	s = reBold.ReplaceAllString(s, "<strong>$1</strong>")
	for i, c := range codes {
		s = strings.ReplaceAll(s, fmt.Sprintf("\x00%d\x00", i), c)
	}
	return s
}

func anchor(h string) string {
	h = strings.ToLower(h)
	var b strings.Builder
	for _, r := range h {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		}
	}
	return b.String()
}

func cells(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(strings.TrimSuffix(line, "|"), "|")
	// A pipe inside a code span is escaped as \| in the source.
	parts := strings.Split(strings.ReplaceAll(line, `\|`, "\x01"), "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(strings.ReplaceAll(parts[i], "\x01", "|"))
	}
	return parts
}

func render(md string) (title, body string) {
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	var b strings.Builder
	var para []string
	list := "" // "ul" or "ol" while a list is open
	flush := func() {
		if len(para) > 0 {
			b.WriteString("<p>" + inline(strings.Join(para, " ")) + "</p>\n")
			para = nil
		}
	}
	closeList := func() {
		if list != "" {
			b.WriteString("</" + list + ">\n")
			list = ""
		}
	}
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		t := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(t, "```"):
			flush()
			closeList()
			var code []string
			for i++; i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```"); i++ {
				code = append(code, lines[i])
			}
			b.WriteString("<pre><code>" + html.EscapeString(strings.Join(code, "\n")) + "</code></pre>\n")
		case t == "":
			flush()
			closeList()
		case t == "---":
			flush()
			closeList()
			b.WriteString("<hr>\n")
		case reHeader.MatchString(t):
			flush()
			closeList()
			m := reHeader.FindStringSubmatch(t)
			lvl := len(m[1])
			if lvl == 1 && title == "" {
				title = m[2]
			}
			fmt.Fprintf(&b, "<h%d id=\"%s\">%s</h%d>\n", lvl, anchor(m[2]), inline(m[2]), lvl)
		case strings.HasPrefix(t, "|") && i+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i+1]), "|---"):
			flush()
			closeList()
			b.WriteString("<table><thead><tr>")
			for _, c := range cells(t) {
				b.WriteString("<th>" + inline(c) + "</th>")
			}
			b.WriteString("</tr></thead><tbody>\n")
			for i += 2; i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|"); i++ {
				b.WriteString("<tr>")
				for _, c := range cells(lines[i]) {
					b.WriteString("<td>" + inline(c) + "</td>")
				}
				b.WriteString("</tr>\n")
			}
			i--
			b.WriteString("</tbody></table>\n")
		case strings.HasPrefix(t, "- ") || reOrder.MatchString(t):
			flush()
			kind, item := "ul", strings.TrimPrefix(t, "- ")
			if reOrder.MatchString(t) {
				kind, item = "ol", reOrder.ReplaceAllString(t, "")
			}
			if list != kind {
				closeList()
				b.WriteString("<" + kind + ">\n")
				list = kind
			}
			// Continuation lines (indented) belong to the item.
			for i+1 < len(lines) && strings.HasPrefix(lines[i+1], "  ") && strings.TrimSpace(lines[i+1]) != "" &&
				!strings.HasPrefix(strings.TrimSpace(lines[i+1]), "- ") {
				i++
				item += " " + strings.TrimSpace(lines[i])
			}
			box := ""
			if strings.HasPrefix(item, "[ ] ") {
				box, item = `<span class="box"></span>`, item[4:]
			}
			b.WriteString("<li>" + box + inline(item) + "</li>\n")
		default:
			closeList()
			para = append(para, t)
		}
	}
	flush()
	closeList()
	return title, b.String()
}

func page(title, body string) string {
	return `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>` + html.EscapeString(title) + `</title>
<style>
@page { size: A4; margin: 18mm 16mm; }
body { font-family: "Segoe UI", Inter, Roboto, Arial, sans-serif; color: #10282b; font-size: 10.5pt; line-height: 1.5; }
h1 { font-size: 22pt; color: #0b1517; border-bottom: 3px solid #3dd6d0; padding-bottom: 6px; }
h1::before { content: "ParallelPath"; display: block; font-size: 10pt; color: #1aa39d; letter-spacing: .08em; text-transform: uppercase; }
h2 { font-size: 15pt; color: #0b1517; margin-top: 22px; border-bottom: 1px solid #cfe3e2; padding-bottom: 3px; break-after: avoid; }
h3 { font-size: 12pt; color: #1a4a4a; margin-top: 16px; break-after: avoid; }
a { color: #137a76; text-decoration: none; }
code { font-family: "JetBrains Mono", Consolas, monospace; font-size: 9pt; background: #eef6f5; padding: 1px 4px; border-radius: 4px; }
pre { background: #0b1517; color: #d9f2f0; padding: 10px 12px; border-radius: 8px; break-inside: avoid; }
pre code { background: none; color: inherit; padding: 0; }
table { border-collapse: collapse; width: 100%; margin: 8px 0 12px; font-size: 9.5pt; break-inside: auto; }
th { background: #132427; color: #e6f7f6; text-align: left; padding: 5px 7px; }
td { border-bottom: 1px solid #d7e7e6; padding: 5px 7px; vertical-align: top; }
tr { break-inside: avoid; }
hr { border: 0; border-top: 1px solid #d7e7e6; margin: 16px 0; }
li { margin: 2px 0; }
.box { display: inline-block; width: 9pt; height: 9pt; border: 1.2px solid #1aa39d; border-radius: 2px; margin-right: 6px; vertical-align: -1px; }
footer { margin-top: 28px; font-size: 8.5pt; color: #6b8a89; }
</style></head><body>
` + body + `<footer>ParallelPath - built by DarkCipher0x at DarkCipher Systems - free and open source (AGPL-3.0)</footer>
</body></html>
`
}
