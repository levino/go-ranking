package web

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// TestTournamentFlowRendersAndRecords drives the tournament UI end-to-end:
// create → pick players → start → record a result → see it in the
// standings. It doubles as a template-execution smoke test for both
// tournament pages.
func TestTournamentFlowRendersAndRecords(t *testing.T) {
	ts, svc, signer := newTestWebServer(t)
	ctx := context.Background()
	g, _ := svc.CreateGroupWithSlug(ctx, "g", "G")
	u, _ := svc.Store.UpsertUserByOIDC(ctx, "u-sub", "u@example.com", "U")
	_ = svc.Store.AddGroupAdmin(ctx, u.ID, g.ID)
	var pids []int64
	for _, r := range []float64{1500, 1100, 700, 300} {
		p, _ := svc.Store.CreatePlayer(ctx, g.ID, "P"+strconv.FormatInt(int64(r), 10), r)
		pids = append(pids, p.ID)
	}
	c := loggedInClient(t, ts, signer, u.ID)

	// The list page renders and offers the create form.
	body := getBody(t, c, ts.URL+"/g/g/tournaments")
	if !strings.Contains(body, "Neues Turnier") {
		t.Fatalf("tournaments page missing create form")
	}

	// Create a round-robin handicap tournament.
	resp := postForm(t, c, ts.URL+"/g/g/tournaments", url.Values{
		"name": {"Weihnachtsturnier"}, "format": {"round_robin"},
		"board": {"9"}, "handicap": {"on"}, "rounds": {"0"},
	})
	loc := resp.Header.Get("Location")
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound || !strings.HasPrefix(loc, "/g/g/t/") {
		t.Fatalf("create -> %d %q", resp.StatusCode, loc)
	}

	// Detail page in setup shows the player picker.
	body = getBody(t, c, ts.URL+loc)
	if !strings.Contains(body, "Spieler auswählen") {
		t.Fatalf("setup page missing player picker: %s", body)
	}

	// Start with all four players.
	form := url.Values{}
	for _, id := range pids {
		form.Add("player", strconv.FormatInt(id, 10))
	}
	resp = postForm(t, c, ts.URL+loc+"/start", form)
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("start -> %d", resp.StatusCode)
	}

	// Now the detail page shows standings and round 1.
	body = getBody(t, c, ts.URL+loc)
	for _, want := range []string{"Tabelle", "Runde 1", "Weihnachtsturnier"} {
		if !strings.Contains(body, want) {
			t.Errorf("running page missing %q", want)
		}
	}

	// Record one pairing's result and confirm it surfaces as a win.
	tr, _ := svc.Store.ListTournaments(ctx, g.ID)
	pairings, _ := svc.Store.ListPairings(ctx, tr[0].ID)
	var first int64
	for _, p := range pairings {
		if !p.IsBye() {
			first = p.ID
			break
		}
	}
	resp = postForm(t, c, ts.URL+loc+"/result", url.Values{
		"pairing": {strconv.FormatInt(first, 10)}, "winner": {"black"},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("record result -> %d", resp.StatusCode)
	}
	body = getBody(t, c, ts.URL+loc)
	if !strings.Contains(body, "🏆") {
		t.Errorf("standings/result should show a trophy after a recorded win")
	}
}

func getBody(t *testing.T, c *http.Client, url string) string {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET %s -> %d: %s", url, resp.StatusCode, b)
	}
	return string(b)
}

func postForm(t *testing.T, c *http.Client, url string, form url.Values) *http.Response {
	t.Helper()
	resp, err := c.PostForm(url, form)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}
