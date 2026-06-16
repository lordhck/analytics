package store

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func insertAt(t *testing.T, s *Store, siteID, path, ref string, ts time.Time) {
	t.Helper()
	if _, err := s.db.Exec(`INSERT INTO events(site_id, path, referrer, created_at) VALUES(?, ?, ?, ?)`,
		siteID, path, ref, ts.Unix()); err != nil {
		t.Fatalf("insertAt: %v", err)
	}
}

func TestSeedAndAuth(t *testing.T) {
	s := newTestStore(t)

	must, err := s.MustChange()
	if err != nil || !must {
		t.Fatalf("MustChange after seed = %v, %v; want true", must, err)
	}
	if ok, err := s.CheckPassword("admin"); err != nil || !ok {
		t.Fatalf("CheckPassword(admin) = %v, %v; want true", ok, err)
	}
	if ok, err := s.CheckPassword("wrong"); err != nil || ok {
		t.Fatalf("CheckPassword(wrong) = %v, %v; want false", ok, err)
	}

	if err := s.SetPassword("s3cret"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if err := s.SetMustChange(false); err != nil {
		t.Fatalf("SetMustChange: %v", err)
	}
	if must, _ := s.MustChange(); must {
		t.Fatalf("MustChange after clear = true; want false")
	}
	if ok, _ := s.CheckPassword("s3cret"); !ok {
		t.Fatalf("CheckPassword(s3cret) = false; want true")
	}
}

func TestSessionSecretStable(t *testing.T) {
	s := newTestStore(t)
	a, err := s.SessionSecret()
	if err != nil || a == "" {
		t.Fatalf("SessionSecret first = %q, %v", a, err)
	}
	b, _ := s.SessionSecret()
	if a != b {
		t.Fatalf("SessionSecret not stable: %q != %q", a, b)
	}
}

func TestSitesCascade(t *testing.T) {
	s := newTestStore(t)
	site, err := s.CreateSite("a.com")
	if err != nil {
		t.Fatalf("CreateSite: %v", err)
	}
	if err := s.InsertEvent(site.ID, "/", "Direct"); err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}

	sites, _ := s.ListSites()
	if len(sites) != 1 || sites[0].Domain != "a.com" {
		t.Fatalf("ListSites = %+v; want one a.com", sites)
	}
	if got, err := s.GetSite(site.ID); err != nil || got.Domain != "a.com" {
		t.Fatalf("GetSite = %+v, %v", got, err)
	}

	if err := s.DeleteSite(site.ID); err != nil {
		t.Fatalf("DeleteSite: %v", err)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM events WHERE site_id = ?`, site.ID).Scan(&n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if n != 0 {
		t.Fatalf("events after delete = %d; want 0 (cascade)", n)
	}
	if _, err := s.GetSite(site.ID); err == nil {
		t.Fatalf("GetSite after delete = nil error; want not-found")
	}
}

func TestStatsTimezone(t *testing.T) {
	s := newTestStore(t)
	site, err := s.CreateSite("a.com")
	if err != nil {
		t.Fatalf("CreateSite: %v", err)
	}

	loc := time.FixedZone("test+2", 2*60*60)
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, loc)

	insertAt(t, s, site.ID, "/", "Direct", time.Date(2026, 6, 16, 0, 30, 0, 0, loc))
	insertAt(t, s, site.ID, "/", "Direct", now)
	insertAt(t, s, site.ID, "/about", "google.com", now)
	insertAt(t, s, site.ID, "/", "Direct", time.Date(2026, 6, 15, 23, 30, 0, 0, loc))
	insertAt(t, s, site.ID, "/old", "Direct", time.Date(2026, 6, 8, 12, 0, 0, 0, loc))

	if got, _ := s.VisitsToday(site.ID, now, loc); got != 3 {
		t.Fatalf("VisitsToday = %d; want 3", got)
	}
	if got, _ := s.VisitsLast7Days(site.ID, now, loc); got != 4 {
		t.Fatalf("VisitsLast7Days = %d; want 4", got)
	}

	days, _ := s.DailyBreakdown(site.ID, now, loc)
	if len(days) != 7 {
		t.Fatalf("DailyBreakdown len = %d; want 7", len(days))
	}
	if last := days[6]; last.Day != "2026-06-16" || last.Count != 3 {
		t.Fatalf("DailyBreakdown today = %+v; want 2026-06-16 count 3", last)
	}
	if prev := days[5]; prev.Day != "2026-06-15" || prev.Count != 1 {
		t.Fatalf("DailyBreakdown yesterday = %+v; want 2026-06-15 count 1", prev)
	}

	pages, _ := s.TopPages(site.ID, now, loc, 10)
	if len(pages) != 2 || pages[0].Path != "/" || pages[0].Count != 3 {
		t.Fatalf("TopPages = %+v; want / first with 3", pages)
	}
	refs, _ := s.TopReferrers(site.ID, now, loc, 10)
	if len(refs) != 2 || refs[0].Referrer != "Direct" || refs[0].Count != 3 {
		t.Fatalf("TopReferrers = %+v; want Direct first with 3", refs)
	}
}
