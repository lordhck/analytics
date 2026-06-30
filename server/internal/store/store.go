package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB

	// TempPassword holds the one-time admin password generated on first init,
	// kept only in memory so the operator can read it from the startup logs;
	// it is never persisted in clear and is empty once a real password exists.
	TempPassword string
}

type Site struct {
	ID        string
	Domain    string
	CreatedAt time.Time
}

type DayCount struct {
	Day   string
	Count int
}

type PathCount struct {
	Path  string
	Count int
}

type RefCount struct {
	Referrer string
	Count    int
}

func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.init(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) init() error {
	const schema = `
CREATE TABLE IF NOT EXISTS settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sites (
	id         TEXT PRIMARY KEY,
	domain     TEXT NOT NULL,
	created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS events (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	site_id    TEXT NOT NULL,
	path       TEXT NOT NULL,
	referrer   TEXT NOT NULL,
	created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_events_site_time ON events(site_id, created_at);
`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	return s.seed()
}

func (s *Store) seed() error {
	if _, err := s.SessionSecret(); err != nil {
		return err
	}
	has, err := s.hasSetting("password_hash")
	if err != nil {
		return err
	}
	if !has {
		otp, err := randomToken(16)
		if err != nil {
			return err
		}
		if err := s.SetPassword(otp); err != nil {
			return err
		}
		if err := s.SetMustChange(true); err != nil {
			return err
		}
		s.TempPassword = otp
	}
	return nil
}

func (s *Store) getSetting(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	return v, err
}

func (s *Store) hasSetting(key string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(1) FROM settings WHERE key = ?`, key).Scan(&n)
	return n > 0, err
}

func (s *Store) setSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings(key, value) VALUES(?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (s *Store) SetPassword(pw string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.setSetting("password_hash", string(hash))
}

func (s *Store) CheckPassword(pw string) (bool, error) {
	hash, err := s.getSetting("password_hash")
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw))
	if err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ConsumeTempPassword removes the stored password so the generated one-time
// password can authenticate only once. The caller already holds a session and
// is forced to set a real password; if that is abandoned, a restart re-seeds a
// fresh temporary password rather than locking the instance out.
func (s *Store) ConsumeTempPassword() error {
	_, err := s.db.Exec(`DELETE FROM settings WHERE key = ?`, "password_hash")
	return err
}

func (s *Store) MustChange() (bool, error) {
	v, err := s.getSetting("must_change")
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return v == "1", nil
}

func (s *Store) SetMustChange(b bool) error {
	v := "0"
	if b {
		v = "1"
	}
	return s.setSetting("must_change", v)
}

func (s *Store) SessionSecret() (string, error) {
	v, err := s.getSetting("session_secret")
	if err == nil {
		return v, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	secret, err := randomToken(32)
	if err != nil {
		return "", err
	}
	if err := s.setSetting("session_secret", secret); err != nil {
		return "", err
	}
	return secret, nil
}

func (s *Store) CreateSite(domain string) (*Site, error) {
	id, err := randomToken(8)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	if _, err := s.db.Exec(`INSERT INTO sites(id, domain, created_at) VALUES(?, ?, ?)`, id, domain, now); err != nil {
		return nil, err
	}
	return &Site{ID: id, Domain: domain, CreatedAt: time.Unix(now, 0)}, nil
}

func (s *Store) ListSites() ([]Site, error) {
	rows, err := s.db.Query(`SELECT id, domain, created_at FROM sites ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Site
	for rows.Next() {
		var site Site
		var ts int64
		if err := rows.Scan(&site.ID, &site.Domain, &ts); err != nil {
			return nil, err
		}
		site.CreatedAt = time.Unix(ts, 0)
		out = append(out, site)
	}
	return out, rows.Err()
}

func (s *Store) GetSite(id string) (*Site, error) {
	var site Site
	var ts int64
	err := s.db.QueryRow(`SELECT id, domain, created_at FROM sites WHERE id = ?`, id).Scan(&site.ID, &site.Domain, &ts)
	if err != nil {
		return nil, err
	}
	site.CreatedAt = time.Unix(ts, 0)
	return &site, nil
}

func (s *Store) DeleteSite(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM events WHERE site_id = ?`, id); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`DELETE FROM sites WHERE id = ?`, id); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) InsertEvent(siteID, path, referrer string) error {
	_, err := s.db.Exec(`INSERT INTO events(site_id, path, referrer, created_at) VALUES(?, ?, ?, ?)`,
		siteID, path, referrer, time.Now().Unix())
	return err
}

func (s *Store) visitsBetween(siteID string, start, end time.Time) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(1) FROM events
		WHERE site_id = ? AND created_at >= ? AND created_at < ?`,
		siteID, start.Unix(), end.Unix()).Scan(&n)
	return n, err
}

func startOfDay(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

func (s *Store) VisitsToday(siteID string, now time.Time, loc *time.Location) (int, error) {
	start := startOfDay(now, loc)
	return s.visitsBetween(siteID, start, start.AddDate(0, 0, 1))
}

func (s *Store) VisitsLast7Days(siteID string, now time.Time, loc *time.Location) (int, error) {
	end := startOfDay(now, loc).AddDate(0, 0, 1)
	return s.visitsBetween(siteID, end.AddDate(0, 0, -7), end)
}

func (s *Store) DailyBreakdown(siteID string, now time.Time, loc *time.Location) ([]DayCount, error) {
	today := startOfDay(now, loc)
	out := make([]DayCount, 0, 7)
	for i := 6; i >= 0; i-- {
		day := today.AddDate(0, 0, -i)
		n, err := s.visitsBetween(siteID, day, day.AddDate(0, 0, 1))
		if err != nil {
			return nil, err
		}
		out = append(out, DayCount{Day: day.Format("2006-01-02"), Count: n})
	}
	return out, nil
}

func (s *Store) TopPages(siteID string, now time.Time, loc *time.Location, limit int) ([]PathCount, error) {
	end := startOfDay(now, loc).AddDate(0, 0, 1)
	start := end.AddDate(0, 0, -7)
	rows, err := s.db.Query(`SELECT path, COUNT(1) AS c FROM events
		WHERE site_id = ? AND created_at >= ? AND created_at < ?
		GROUP BY path ORDER BY c DESC LIMIT ?`,
		siteID, start.Unix(), end.Unix(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PathCount
	for rows.Next() {
		var pc PathCount
		if err := rows.Scan(&pc.Path, &pc.Count); err != nil {
			return nil, err
		}
		out = append(out, pc)
	}
	return out, rows.Err()
}

func (s *Store) TopReferrers(siteID string, now time.Time, loc *time.Location, limit int) ([]RefCount, error) {
	end := startOfDay(now, loc).AddDate(0, 0, 1)
	start := end.AddDate(0, 0, -7)
	rows, err := s.db.Query(`SELECT referrer, COUNT(1) AS c FROM events
		WHERE site_id = ? AND created_at >= ? AND created_at < ?
		GROUP BY referrer ORDER BY c DESC LIMIT ?`,
		siteID, start.Unix(), end.Unix(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RefCount
	for rows.Next() {
		var rc RefCount
		if err := rows.Scan(&rc.Referrer, &rc.Count); err != nil {
			return nil, err
		}
		out = append(out, rc)
	}
	return out, rows.Err()
}

func randomToken(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
