package docs

import (
	"strings"
	"testing"
)

func TestListAtLeastOnePage(t *testing.T) {
	pages := List()
	if len(pages) == 0 {
		t.Fatal("expected at least one embedded doc page")
	}
	for _, p := range pages {
		if p.Slug == "" || p.Title == "" {
			t.Errorf("malformed page: %+v", p)
		}
	}
}

func TestListIsSorted(t *testing.T) {
	pages := List()
	for i := 1; i < len(pages); i++ {
		if pages[i-1].Order >= pages[i].Order {
			t.Errorf("not sorted by Order: %+v then %+v", pages[i-1], pages[i])
		}
	}
}

func TestRenderKnownPage(t *testing.T) {
	html := Render("overview")
	if html == "" {
		t.Fatal("overview should render")
	}
	for _, want := range []string{"<h1>", "Überblick", "Tablet"} {
		if !strings.Contains(html, want) {
			t.Errorf("overview missing %q", want)
		}
	}
}

func TestRenderUnknownPageEmpty(t *testing.T) {
	if Render("does-not-exist") != "" {
		t.Error("unknown slug should return empty string")
	}
}

func TestRenderInlineCode(t *testing.T) {
	got := toHTML("This is `inline code` here.\n")
	if !strings.Contains(got, "<code>inline code</code>") {
		t.Errorf("inline code not rendered: %s", got)
	}
}

func TestRenderBold(t *testing.T) {
	got := toHTML("This is **important**.\n")
	if !strings.Contains(got, "<strong>important</strong>") {
		t.Errorf("bold not rendered: %s", got)
	}
}

func TestRenderLink(t *testing.T) {
	got := toHTML("See [the rules](https://example.com/rules).\n")
	if !strings.Contains(got, `<a href="https://example.com/rules">the rules</a>`) {
		t.Errorf("link not rendered: %s", got)
	}
}

func TestRenderCodeFence(t *testing.T) {
	got := toHTML("```go\nfn := func() {}\n```\n")
	if !strings.Contains(got, `<pre><code class="lang-go">`) {
		t.Errorf("code fence missing class: %s", got)
	}
	if !strings.Contains(got, "fn := func() {}") {
		t.Errorf("code fence content missing: %s", got)
	}
}

func TestRenderHeadings(t *testing.T) {
	got := toHTML("# H1\n\n## H2\n\n### H3\n")
	for _, want := range []string{"<h1>H1</h1>", "<h2>H2</h2>", "<h3>H3</h3>"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in: %s", want, got)
		}
	}
}

func TestRenderList(t *testing.T) {
	got := toHTML("- first\n- second\n- third\n")
	if !strings.Contains(got, "<ul>") || !strings.Contains(got, "<li>first</li>") {
		t.Errorf("list malformed: %s", got)
	}
}

func TestRenderRawHTMLPassThrough(t *testing.T) {
	// Tables can also be hand-written as raw HTML when finer control is
	// needed.
	got := toHTML("<table><tr><td>x</td></tr></table>\n")
	if !strings.Contains(got, "<table>") {
		t.Errorf("raw HTML stripped: %s", got)
	}
}

func TestRenderPipeTable(t *testing.T) {
	md := "| GoR | Rang |\n|---|---|\n| 2100 | 1 Dan |\n| 100 | 20 Kyu |\n"
	got := toHTML(md)
	for _, want := range []string{
		"<table>", "<thead>", "<th>GoR</th>", "<th>Rang</th>",
		"<tbody>", "<td>2100</td>", "<td>1 Dan</td>", "<td>20 Kyu</td>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("pipe table missing %q in: %s", want, got)
		}
	}
	// Ensure we did not emit the raw `|` text as a paragraph.
	if strings.Contains(got, "<p>| GoR") {
		t.Errorf("table fell through as paragraph: %s", got)
	}
}

func TestRenderPipeTableLinkInCell(t *testing.T) {
	md := "| Quelle |\n|---|\n| [EGF](https://example.com) |\n"
	got := toHTML(md)
	if !strings.Contains(got, `<a href="https://example.com">EGF</a>`) {
		t.Errorf("link inside table cell not rendered: %s", got)
	}
}

func TestRenderLangEnglish(t *testing.T) {
	html := RenderLang("en", "overview")
	if html == "" {
		t.Fatal("english overview should render")
	}
	if !strings.Contains(html, "Overview") {
		t.Errorf("english overview missing English heading: %s", html)
	}
	if strings.Contains(html, "Überblick") {
		t.Errorf("english overview still shows German heading")
	}
}

func TestListLangFallsBackToDefault(t *testing.T) {
	// An unknown language yields the same chapters/slugs as the default,
	// so navigation stays complete.
	de := ListLang("de")
	xx := ListLang("xx")
	if len(de) != len(xx) || len(de) == 0 {
		t.Fatalf("expected same page count, got de=%d xx=%d", len(de), len(xx))
	}
	for i := range de {
		if de[i].Slug != xx[i].Slug {
			t.Errorf("slug mismatch at %d: %q vs %q", i, de[i].Slug, xx[i].Slug)
		}
	}
}
