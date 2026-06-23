package main

import (
	"html/template"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"analytics/internal/store"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	secret, err := st.SessionSecret()
	if err != nil {
		t.Fatalf("SessionSecret: %v", err)
	}
	return &App{
		cfg:    Config{AppName: "Test", SiteDomain: "http://localhost:8080", Loc: time.UTC},
		store:  st,
		secret: []byte(secret),
		tmpl:   template.Must(template.ParseFS(templatesFS, "templates/*.html")),
	}
}

func testServer(t *testing.T, app *App) (*httptest.Server, *http.Client) {
	t.Helper()
	srv := httptest.NewServer(app.routes())
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return srv, client
}

func get(t *testing.T, c *http.Client, u string) *http.Response {
	t.Helper()
	resp, err := c.Get(u)
	if err != nil {
		t.Fatalf("GET %s: %v", u, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func postForm(t *testing.T, c *http.Client, u string, v url.Values) *http.Response {
	t.Helper()
	resp, err := c.PostForm(u, v)
	if err != nil {
		t.Fatalf("POST %s: %v", u, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func assertRedirect(t *testing.T, resp *http.Response, loc string) {
	t.Helper()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d; want 303", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != loc {
		t.Fatalf("Location = %q; want %q", got, loc)
	}
}

func TestReferrerHost(t *testing.T) {
	cases := []struct {
		ref, site, want string
	}{
		{"", "example.com", "Direct"},
		{"https://www.google.com/search?q=x", "example.com", "google.com"},
		{"https://example.com/page", "example.com", "Direct"},
		{"https://www.example.com/page", "https://example.com", "Direct"},
		{"https://news.ycombinator.com/", "example.com", "news.ycombinator.com"},
	}
	for _, c := range cases {
		if got := referrerHost(c.ref, c.site); got != c.want {
			t.Errorf("referrerHost(%q, %q) = %q; want %q", c.ref, c.site, got, c.want)
		}
	}
}

func TestSessionRoundTrip(t *testing.T) {
	app := newTestApp(t)
	tok := app.sign("auth")
	if !app.verify(tok) {
		t.Fatal("verify(sign(auth)) = false; want true")
	}
	if app.verify(tok + "x") {
		t.Fatal("verify(tampered) = true; want false")
	}
	if app.verify("auth.deadbeef") {
		t.Fatal("verify(forged) = true; want false")
	}
}

func TestLoginResetFlow(t *testing.T) {
	app := newTestApp(t)
	srv, client := testServer(t, app)

	assertRedirect(t, get(t, client, srv.URL+"/dash"), "/auth")

	resp := postForm(t, client, srv.URL+"/auth", url.Values{"password": {"nope"}})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong login = %d; want 401", resp.StatusCode)
	}

	assertRedirect(t, postForm(t, client, srv.URL+"/auth", url.Values{"password": {"admin"}}), "/reset")
	assertRedirect(t, get(t, client, srv.URL+"/dash"), "/reset")
	assertRedirect(t, postForm(t, client, srv.URL+"/reset", url.Values{"password": {"hunter2"}}), "/dash")

	if resp := get(t, client, srv.URL+"/dash"); resp.StatusCode != http.StatusOK {
		t.Fatalf("dash after reset = %d; want 200", resp.StatusCode)
	}
}

func TestCollectorStoresEvent(t *testing.T) {
	app := newTestApp(t)
	srv, client := testServer(t, app)
	site, err := app.store.CreateSite("example.com")
	if err != nil {
		t.Fatalf("CreateSite: %v", err)
	}

	body := `{"site":"` + site.ID + `","path":"/","referrer":"https://www.google.com/"}`
	resp, err := client.Post(srv.URL+"/api/event", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST event: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("event = %d; want 204", resp.StatusCode)
	}

	now := time.Now()
	if n, _ := app.store.VisitsToday(site.ID, now, time.UTC); n != 1 {
		t.Fatalf("VisitsToday = %d; want 1", n)
	}
	refs, _ := app.store.TopReferrers(site.ID, now, time.UTC, 5)
	if len(refs) != 1 || refs[0].Referrer != "google.com" {
		t.Fatalf("TopReferrers = %+v; want one google.com (www stripped)", refs)
	}

	resp2, err := client.Post(srv.URL+"/api/event", "application/json",
		strings.NewReader(`{"site":"unknown","path":"/","referrer":""}`))
	if err != nil {
		t.Fatalf("POST unknown event: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusNoContent {
		t.Fatalf("unknown site = %d; want 204", resp2.StatusCode)
	}
}

func TestTrackerServed(t *testing.T) {
	app := newTestApp(t)
	srv, client := testServer(t, app)
	resp := get(t, client, srv.URL+"/atag.js")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/atag.js = %d; want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/javascript") {
		t.Fatalf("Content-Type = %q; want application/javascript", ct)
	}
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), "sendBeacon") {
		t.Fatal("tracker body missing sendBeacon")
	}
}
