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
	// Tables in the manual are hand-written as HTML.
	got := toHTML("<table><tr><td>x</td></tr></table>\n")
	if !strings.Contains(got, "<table>") {
		t.Errorf("raw HTML stripped: %s", got)
	}
}
