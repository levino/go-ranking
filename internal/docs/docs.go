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

//go:embed pages/*.md pages/en/*.md
var pagesFS embed.FS

// defaultLang is the language whose pages live directly under pages/.
// Other languages live in pages/<lang>/ and fall back to the default
// file when a translation is missing.
const defaultLang = "de"

// Page is a single chapter of the manual.
type Page struct {
	Slug  string
	Title string
	Order int
}

// dirFor returns the embedded directory holding a language's pages.
func dirFor(lang string) string {
	if lang == "" || lang == defaultLang {
		return "pages"
	}
	return "pages/" + lang
}

// List returns the available pages in the default language. See
// ListLang for language-aware listing.
func List() []Page { return ListLang(defaultLang) }

// ListLang returns the available pages for a language, sorted by their
// numeric prefix. Files are named `NN-slug.md`; the numeric prefix
// controls navigation order, the slug becomes the URL, and the first
// level-1 heading becomes the displayed title. Pages missing in a
// translation fall back to the default-language file (so the navigation
// is always complete and slugs line up across languages).
func ListLang(lang string) []Page {
	// Drive ordering and slugs off the default set so every language
	// exposes the same chapters in the same order.
	entries, err := fs.ReadDir(pagesFS, "pages")
	if err != nil {
		return nil
	}
	dir := dirFor(lang)
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
		content, err := fs.ReadFile(pagesFS, dir+"/"+e.Name())
		if err != nil {
			content, err = fs.ReadFile(pagesFS, "pages/"+e.Name())
			if err != nil {
				continue
			}
		}
		out = append(out, Page{Slug: slug, Title: firstHeading(string(content)), Order: order})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out
}

// Render returns the HTML body for the given slug in the default
// language, or empty string if no such page exists.
func Render(slug string) string { return RenderLang(defaultLang, slug) }

// RenderLang returns the HTML body for a slug in the given language,
// falling back to the default-language file when the translation is
// absent. Empty string if no such page exists in any language.
func RenderLang(lang, slug string) string {
	entries, _ := fs.ReadDir(pagesFS, "pages")
	dir := dirFor(lang)
	for _, e := range entries {
		stem := strings.TrimSuffix(e.Name(), ".md")
		s := stem
		if i := strings.Index(stem, "-"); i > 0 {
			s = stem[i+1:]
		}
		if s == slug {
			content, err := fs.ReadFile(pagesFS, dir+"/"+e.Name())
			if err != nil {
				content, _ = fs.ReadFile(pagesFS, "pages/"+e.Name())
			}
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
	for idx := 0; idx < len(lines); idx++ {
		raw := lines[idx]
		line := strings.TrimRight(raw, " \t\r")

		// Pipe tables: header row + separator (---) + zero or more data
		// rows. Recognised only outside code blocks. Cell contents go
		// through inline rendering so links and bold work inside tables.
		if !inCode && isTableHeader(line) && idx+1 < len(lines) && isTableSeparator(lines[idx+1]) {
			flushPara()
			closeList()
			headers := splitTableRow(line)
			b.WriteString("<table>\n<thead><tr>")
			for _, h := range headers {
				b.WriteString("<th>")
				b.WriteString(inlineHTML(h))
				b.WriteString("</th>")
			}
			b.WriteString("</tr></thead>\n<tbody>\n")
			j := idx + 2
			for j < len(lines) && isTableHeader(strings.TrimRight(lines[j], " \t\r")) {
				cells := splitTableRow(strings.TrimRight(lines[j], " \t\r"))
				b.WriteString("<tr>")
				for _, c := range cells {
					b.WriteString("<td>")
					b.WriteString(inlineHTML(c))
					b.WriteString("</td>")
				}
				b.WriteString("</tr>\n")
				j++
			}
			b.WriteString("</tbody>\n</table>\n")
			idx = j - 1
			continue
		}

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

// isTableHeader returns true for a line that looks like a pipe-table
// row: starts with `|` and contains at least one more `|`.
func isTableHeader(line string) bool {
	if !strings.HasPrefix(line, "|") {
		return false
	}
	return strings.Count(line, "|") >= 2
}

// isTableSeparator returns true for a line like `|---|---|` or
// `| :---: | ---: |` (alignment markers are accepted but ignored).
func isTableSeparator(line string) bool {
	line = strings.TrimRight(line, " \t\r")
	if !strings.HasPrefix(line, "|") {
		return false
	}
	for _, c := range line {
		switch c {
		case '|', '-', ':', ' ', '\t':
		default:
			return false
		}
	}
	return strings.Contains(line, "---")
}

// splitTableRow splits `| a | b | c |` into ["a", "b", "c"]. Trims
// outer pipes and whitespace; empty trailing cell from the closing `|`
// is dropped.
func splitTableRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	parts := strings.Split(line, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.TrimSpace(p))
	}
	return out
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

// inlineNonCode handles bold, italic, and links in a non-code segment.
// Math is left as raw $...$ / $$...$$ markers so KaTeX can find them
// after the page hits the browser (see layout.html for the auto-render
// script tag). We escape HTML BEFORE applying markdown so user-written
// `<` doesn't get treated as a tag, but math runs in its own pass
// before escaping so $a < b$ stays valid TeX.
func inlineNonCode(s string) string {
	s, math := extractMath(s)
	s = html.EscapeString(s)
	s = boldRepl(s)
	s = italicRepl(s)
	s = linkRepl(s)
	s = restoreMath(s, math)
	return s
}

// extractMath pulls out $...$ inline and $$...$$ display math
// segments, replacing them with placeholders. Returns the cleaned
// string and the segments (in order of appearance).
func extractMath(s string) (string, []string) {
	var out strings.Builder
	var math []string
	for i := 0; i < len(s); {
		if i+1 < len(s) && s[i] == '$' && s[i+1] == '$' {
			j := strings.Index(s[i+2:], "$$")
			if j >= 0 {
				math = append(math, "$$"+s[i+2:i+2+j]+"$$")
				out.WriteString("\x00MATH" + fmt.Sprintf("%d", len(math)-1) + "\x00")
				i += 2 + j + 2
				continue
			}
		}
		if s[i] == '$' {
			j := strings.Index(s[i+1:], "$")
			if j >= 0 {
				math = append(math, "$"+s[i+1:i+1+j]+"$")
				out.WriteString("\x00MATH" + fmt.Sprintf("%d", len(math)-1) + "\x00")
				i += 1 + j + 1
				continue
			}
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String(), math
}

func restoreMath(s string, math []string) string {
	for idx, m := range math {
		marker := "\x00MATH" + fmt.Sprintf("%d", idx) + "\x00"
		s = strings.Replace(s, marker, m, 1)
	}
	return s
}

// italicRepl turns *x* into <em>x</em>. Single asterisks only — must
// not be confused with the bold markers (**…**), which boldRepl has
// already consumed. Greedy left-to-right matching.
func italicRepl(s string) string {
	var b strings.Builder
	for {
		i := strings.Index(s, "*")
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		j := strings.Index(s[i+1:], "*")
		if j < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		b.WriteString("<em>")
		b.WriteString(s[i+1 : i+1+j])
		b.WriteString("</em>")
		s = s[i+1+j+1:]
	}
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
