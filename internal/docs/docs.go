// Package docs serves the user-facing manual: a small set of Markdown
// files embedded into the binary, rendered to HTML at request time.
//
// The renderer is intentionally minimal — we control every input file,
// so it only needs to handle the Markdown features we actually use.
// Pulling in goldmark/blackfriday for this would mean adding a heavy
// dependency for what's essentially a static-site-generator's worth of
// work (~ 200 lines of code).
package docs

import (
	"embed"
	"fmt"
	"html"
	"io/fs"
	"sort"
	"strings"
)

//go:embed pages/*.md
var pagesFS embed.FS

// Page is a single chapter of the manual.
type Page struct {
	Slug  string
	Title string
	Order int
}

// List returns the available pages, sorted by their numeric prefix.
// Files are named `NN-slug.md`; the numeric prefix controls navigation
// order, the slug becomes the URL, and the first level-1 heading
// becomes the displayed title.
func List() []Page {
	entries, err := fs.ReadDir(pagesFS, "pages")
	if err != nil {
		return nil
	}
	var out []Page
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		stem := strings.TrimSuffix(e.Name(), ".md")
		// "NN-slug" → order NN, slug "slug"
		order := 0
		slug := stem
		if i := strings.Index(stem, "-"); i > 0 {
			fmt.Sscanf(stem[:i], "%d", &order)
			slug = stem[i+1:]
		}
		content, err := fs.ReadFile(pagesFS, "pages/"+e.Name())
		if err != nil {
			continue
		}
		out = append(out, Page{Slug: slug, Title: firstHeading(string(content)), Order: order})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out
}

// Render returns the HTML body for the given slug, or empty string if
// no such page exists.
func Render(slug string) string {
	entries, _ := fs.ReadDir(pagesFS, "pages")
	for _, e := range entries {
		stem := strings.TrimSuffix(e.Name(), ".md")
		s := stem
		if i := strings.Index(stem, "-"); i > 0 {
			s = stem[i+1:]
		}
		if s == slug {
			content, _ := fs.ReadFile(pagesFS, "pages/"+e.Name())
			return toHTML(string(content))
		}
	}
	return ""
}

// firstHeading returns the text of the first `# ...` line in the
// document, used for navigation labels.
func firstHeading(md string) string {
	for _, line := range strings.Split(md, "\n") {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(line[2:])
		}
	}
	return ""
}

// toHTML walks the document line by line. We don't aim for CommonMark
// completeness — just the constructs we actually use in the manual.
func toHTML(md string) string {
	var b strings.Builder
	lines := strings.Split(md, "\n")
	inCode := false
	inList := false
	var paraBuf []string
	flushPara := func() {
		if len(paraBuf) == 0 {
			return
		}
		b.WriteString("<p>")
		b.WriteString(inlineHTML(strings.Join(paraBuf, " ")))
		b.WriteString("</p>\n")
		paraBuf = nil
	}
	closeList := func() {
		if inList {
			b.WriteString("</ul>\n")
			inList = false
		}
	}
	for _, raw := range lines {
		line := strings.TrimRight(raw, " \t\r")

		// Code fence open/close.
		if strings.HasPrefix(line, "```") {
			flushPara()
			closeList()
			if inCode {
				b.WriteString("</code></pre>\n")
				inCode = false
			} else {
				lang := strings.TrimSpace(strings.TrimPrefix(line, "```"))
				b.WriteString(`<pre><code class="lang-`)
				b.WriteString(html.EscapeString(lang))
				b.WriteString(`">`)
				inCode = true
			}
			continue
		}
		if inCode {
			b.WriteString(html.EscapeString(raw))
			b.WriteString("\n")
			continue
		}

		// Blank line — paragraph/list separator.
		if line == "" {
			flushPara()
			closeList()
			continue
		}

		// Headings.
		if h, level := parseHeading(line); level > 0 {
			flushPara()
			closeList()
			fmt.Fprintf(&b, "<h%d>%s</h%d>\n", level, inlineHTML(h), level)
			continue
		}

		// Lists (- or *).
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			flushPara()
			if !inList {
				b.WriteString("<ul>\n")
				inList = true
			}
			b.WriteString("<li>")
			b.WriteString(inlineHTML(strings.TrimSpace(line[2:])))
			b.WriteString("</li>\n")
			continue
		}

		// Raw HTML pass-through (for tables, which we hand-write).
		if strings.HasPrefix(line, "<") {
			flushPara()
			closeList()
			b.WriteString(line)
			b.WriteString("\n")
			continue
		}

		// Otherwise it's part of a paragraph.
		closeList()
		paraBuf = append(paraBuf, line)
	}
	flushPara()
	closeList()
	if inCode {
		b.WriteString("</code></pre>\n")
	}
	return b.String()
}

// parseHeading returns the heading text and level (1-3), or 0 if the
// line is not a heading.
func parseHeading(line string) (string, int) {
	n := 0
	for n < 3 && n < len(line) && line[n] == '#' {
		n++
	}
	if n == 0 || n >= len(line) || line[n] != ' ' {
		return "", 0
	}
	return strings.TrimSpace(line[n+1:]), n
}

// inlineHTML transforms inline Markdown to HTML. Order matters: code
// spans are extracted first so their contents are not further parsed.
func inlineHTML(s string) string {
	// Pull out code spans so we don't re-process their bodies.
	var (
		out      strings.Builder
		i        int
		segments []string // alternating: code, text, code, text...
		isCode   []bool
	)
	for i < len(s) {
		if s[i] == '`' {
			// Find closing backtick.
			j := strings.IndexByte(s[i+1:], '`')
			if j < 0 {
				break
			}
			segments = append(segments, s[i+1:i+1+j])
			isCode = append(isCode, true)
			i += j + 2
			continue
		}
		// Collect text up to the next backtick (or end).
		next := strings.IndexByte(s[i:], '`')
		if next < 0 {
			segments = append(segments, s[i:])
			isCode = append(isCode, false)
			break
		}
		segments = append(segments, s[i:i+next])
		isCode = append(isCode, false)
		i += next
	}
	for k, seg := range segments {
		if isCode[k] {
			out.WriteString("<code>")
			out.WriteString(html.EscapeString(seg))
			out.WriteString("</code>")
		} else {
			out.WriteString(inlineNonCode(seg))
		}
	}
	return out.String()
}

// inlineNonCode handles bold and links in a non-code segment.
func inlineNonCode(s string) string {
	s = html.EscapeString(s)
	s = boldRepl(s)
	s = linkRepl(s)
	return s
}

// boldRepl turns **x** into <strong>x</strong>. Greedy left-to-right
// matching, doesn't handle nested or partial markers.
func boldRepl(s string) string {
	var b strings.Builder
	for {
		i := strings.Index(s, "**")
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		j := strings.Index(s[i+2:], "**")
		if j < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		b.WriteString("<strong>")
		b.WriteString(s[i+2 : i+2+j])
		b.WriteString("</strong>")
		s = s[i+2+j+2:]
	}
}

// linkRepl turns [text](href) into <a href="href">text</a>.
func linkRepl(s string) string {
	var b strings.Builder
	for {
		open := strings.Index(s, "[")
		if open < 0 {
			b.WriteString(s)
			return b.String()
		}
		close := strings.Index(s[open:], "]")
		if close < 0 {
			b.WriteString(s)
			return b.String()
		}
		close += open
		if close+1 >= len(s) || s[close+1] != '(' {
			b.WriteString(s[:close+1])
			s = s[close+1:]
			continue
		}
		parenClose := strings.Index(s[close+2:], ")")
		if parenClose < 0 {
			b.WriteString(s)
			return b.String()
		}
		text := s[open+1 : close]
		href := s[close+2 : close+2+parenClose]
		b.WriteString(s[:open])
		b.WriteString(`<a href="`)
		b.WriteString(href)
		b.WriteString(`">`)
		b.WriteString(text)
		b.WriteString("</a>")
		s = s[close+2+parenClose+1:]
	}
}
