// Package report writes ParallelPath's court-ready reports as PDF: plain, paginated A4 text in the standard Helvetica
// fonts (every PDF reader has them, so nothing is embedded and the file is easy to inspect). No dependencies.
package report

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	pageW, pageH = 595.28, 841.89 // A4 in points
	margin       = 56.0
	bottom       = 64.0
)

// Colours (RGB 0-1): ParallelPath's accent and text, chosen to print well in black and white too.
var (
	ink    = [3]float64{0.07, 0.16, 0.17}
	dim    = [3]float64{0.32, 0.40, 0.41}
	accent = [3]float64{0.04, 0.47, 0.45}
	warn   = [3]float64{0.60, 0.30, 0.02}
)

// PDF is a document being written.
type PDF struct {
	pages []*bytes.Buffer
	cur   *bytes.Buffer
	y     float64
	title string
}

// New starts a document.
func New(title string) *PDF {
	p := &PDF{title: title}
	p.newPage()
	return p
}

func (p *PDF) newPage() {
	p.cur = &bytes.Buffer{}
	p.pages = append(p.pages, p.cur)
	p.y = pageH - margin
}

func (p *PDF) need(h float64) {
	if p.y-h < bottom {
		p.newPage()
	}
}

func (p *PDF) text(x, y float64, font string, size float64, c [3]float64, s string) {
	fmt.Fprintf(p.cur, "BT %.3f %.3f %.3f rg /%s %.1f Tf %.2f %.2f Td (%s) Tj ET\n", c[0], c[1], c[2], font, size, x, y, escape(s))
}

// line writes wrapped text in the given font and size, indented by indent points.
func (p *PDF) line(s, font string, size float64, c [3]float64, indent float64) {
	lead := size * 1.35
	for _, l := range wrap(s, font, size, pageW-2*margin-indent) {
		p.need(lead)
		p.y -= lead
		p.text(margin+indent, p.y, font, size, c, l)
	}
}

// Title is the document's main heading.
func (p *PDF) Title(s string) { p.line(s, "F2", 20, accent, 0); p.Gap(6) }

// H1 starts a section (on a new page if little room is left).
func (p *PDF) H1(s string) {
	p.need(80)
	p.Gap(10)
	p.line(s, "F2", 15, accent, 0)
	p.Rule()
}

// H2 is a sub-heading.
func (p *PDF) H2(s string) { p.need(40); p.Gap(4); p.line(s, "F2", 11.5, ink, 0) }

// Para is body text.
func (p *PDF) Para(s string) { p.line(s, "F1", 10, ink, 0) }

// Indent is body text indented under an item.
func (p *PDF) Indent(s string) { p.line(s, "F1", 9.5, ink, 14) }

// Small is secondary text (times, who, reasons).
func (p *PDF) Small(s string) { p.line(s, "F1", 8.5, dim, 14) }

// Warn is text that needs attention (withdrawn, declined, failed checks).
func (p *PDF) Warn(s string) { p.line(s, "F2", 9.5, warn, 14) }

// KV is a label and a value on one line.
func (p *PDF) KV(k, v string) {
	lead := 10 * 1.4
	lines := wrap(v, "F1", 10, pageW-2*margin-150)
	for i, l := range lines {
		p.need(lead)
		p.y -= lead
		if i == 0 {
			p.text(margin, p.y, "F2", 9.5, dim, k)
		}
		p.text(margin+150, p.y, "F1", 10, ink, l)
	}
}

// Mono is fixed-width text (hashes, signatures).
func (p *PDF) Mono(s string) { p.line(s, "F3", 8.5, ink, 14) }

// Gap adds vertical space.
func (p *PDF) Gap(h float64) { p.y -= h }

// Rule draws a thin line across the page.
func (p *PDF) Rule() {
	p.need(8)
	p.y -= 5
	fmt.Fprintf(p.cur, "%.3f %.3f %.3f RG 0.6 w %.2f %.2f m %.2f %.2f l S\n", accent[0], accent[1], accent[2], margin, p.y, pageW-margin, p.y)
	p.y -= 4
}

// Bytes finishes the document; footer gives each page's footer text ("Page 2 of 9 · report 1a2b…").
func (p *PDF) Bytes(footer func(page, total int) string) []byte {
	n := len(p.pages)
	var out bytes.Buffer
	var offs []int
	obj := func(body string) {
		offs = append(offs, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", len(offs), body)
	}
	out.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	// 1 catalog, 2 pages, 3-5 fonts, then (page, content) pairs, then info.
	kids := make([]string, n)
	for i := range kids {
		kids[i] = fmt.Sprintf("%d 0 R", 6+2*i)
	}
	obj("<< /Type /Catalog /Pages 2 0 R >>")
	obj(fmt.Sprintf("<< /Type /Pages /Count %d /Kids [%s] >>", n, strings.Join(kids, " ")))
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Courier /Encoding /WinAnsiEncoding >>")
	for i, pg := range p.pages {
		var c bytes.Buffer
		c.Write(pg.Bytes())
		f := footer(i+1, n)
		fmt.Fprintf(&c, "BT %.3f %.3f %.3f rg /F1 7.5 Tf %.2f %.2f Td (%s) Tj ET\n", dim[0], dim[1], dim[2], margin, 34.0, escape(f))
		fmt.Fprintf(&c, "%.3f %.3f %.3f RG 0.4 w %.2f %.2f m %.2f %.2f l S\n", dim[0], dim[1], dim[2], margin, 46.0, pageW-margin, 46.0)
		obj(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.2f %.2f] /Resources << /Font << /F1 3 0 R /F2 4 0 R /F3 5 0 R >> >> /Contents %d 0 R >>",
			pageW, pageH, 7+2*i))
		obj(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", c.Len(), c.String()))
	}
	obj(fmt.Sprintf("<< /Title (%s) /Producer (ParallelPath) /Creator (ParallelPath) >>", escape(p.title)))
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offs)+1)
	for _, o := range offs {
		fmt.Fprintf(&out, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R /Info %d 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offs)+1, len(offs), xref)
	return out.Bytes()
}

// winAnsi maps the characters the standard fonts have (WinAnsiEncoding); anything else becomes "?".
var winAnsiExtra = map[rune]byte{
	'€': 0x80, '‚': 0x82, 'ƒ': 0x83, '„': 0x84, '…': 0x85, '†': 0x86, '‡': 0x87, 'ˆ': 0x88, '‰': 0x89, 'Š': 0x8a, '‹': 0x8b,
	'Œ': 0x8c, 'Ž': 0x8e, '‘': 0x91, '’': 0x92, '“': 0x93, '”': 0x94, '•': 0x95, '–': 0x96, '—': 0x97, '˜': 0x98, '™': 0x99,
	'š': 0x9a, '›': 0x9b, 'œ': 0x9c, 'ž': 0x9e, 'Ÿ': 0x9f,
}

func toWinAnsi(s string) []byte {
	var b []byte
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
			b = append(b, ' ')
		case r >= 0x20 && r < 0x7f, r >= 0xa0 && r <= 0xff:
			b = append(b, byte(r))
		case winAnsiExtra[r] != 0:
			b = append(b, winAnsiExtra[r])
		case r == 0xfe0f || r == 0x200d: // emoji presentation selectors and joiners: drop
		default:
			b = append(b, '?')
		}
	}
	return b
}

func escape(s string) string {
	var out strings.Builder
	for _, c := range toWinAnsi(s) {
		switch c {
		case '(', ')', '\\':
			out.WriteByte('\\')
			out.WriteByte(c)
		default:
			if c < 0x20 || c >= 0x7f {
				fmt.Fprintf(&out, "\\%03o", c)
			} else {
				out.WriteByte(c)
			}
		}
	}
	return out.String()
}

// Helvetica's advance widths (per 1000 units) for ASCII 32..126; Courier is 600 for everything.
var helv = [95]int{278, 278, 355, 556, 556, 889, 667, 191, 333, 333, 389, 584, 278, 333, 278, 278, 556, 556, 556, 556, 556, 556, 556,
	556, 556, 556, 278, 278, 584, 584, 584, 556, 1015, 667, 667, 722, 722, 667, 611, 778, 722, 278, 500, 667, 556, 833, 722, 778,
	667, 778, 722, 667, 611, 722, 667, 944, 667, 667, 611, 278, 278, 278, 469, 556, 333, 556, 556, 500, 556, 556, 278, 556, 556,
	222, 222, 500, 222, 833, 556, 556, 556, 556, 333, 500, 278, 556, 500, 722, 500, 500, 500, 334, 260, 334, 584}

func width(s, font string, size float64) float64 {
	w := 0
	for _, c := range toWinAnsi(s) {
		switch {
		case font == "F3":
			w += 600
		case c >= 32 && c <= 126:
			v := helv[c-32]
			if font == "F2" {
				v = v * 106 / 100 // bold is a little wider
			}
			w += v
		default:
			w += 556
		}
	}
	return float64(w) * size / 1000
}

// wrap breaks text into lines that fit max points (long words are split).
func wrap(s, font string, size, max float64) []string {
	var lines []string
	for _, para := range strings.Split(strings.ReplaceAll(s, "\r", ""), "\n") {
		cur := ""
		for _, word := range strings.Fields(para) {
			for width(word, font, size) > max && utf8.RuneCountInString(word) > 1 {
				cut := len(word)
				for cut > 1 && width(word[:cut], font, size) > max {
					_, sz := utf8.DecodeLastRuneInString(word[:cut])
					cut -= sz
				}
				if cur != "" {
					lines = append(lines, cur)
					cur = ""
				}
				lines = append(lines, word[:cut])
				word = word[cut:]
			}
			try := word
			if cur != "" {
				try = cur + " " + word
			}
			if width(try, font, size) > max && cur != "" {
				lines = append(lines, cur)
				cur = word
			} else {
				cur = try
			}
		}
		lines = append(lines, cur)
	}
	return lines
}
