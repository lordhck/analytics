package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"analytics/internal/store"
)

const adminUser = "admin"

type authView struct {
	AppName string
	Error   string
}

type dashView struct {
	AppName string
	Sites   []store.Site
}

type siteView struct {
	AppName string
	Site    *store.Site
	Today   int
	Week    int
	Days    []store.DayCount
	Pages   []store.PathCount
	Refs    []store.RefCount
	Snippet string
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /atag.js", a.handleTracker)
	mux.HandleFunc("POST /api/event", a.handleEvent)
	mux.HandleFunc("OPTIONS /api/event", a.handleEventPreflight)

	mux.HandleFunc("GET /auth", a.handleLoginForm)
	mux.HandleFunc("POST /auth", a.handleLogin)
	mux.HandleFunc("POST /logout", a.handleLogout)

	mux.Handle("GET /reset", a.authed(a.handleResetForm))
	mux.Handle("POST /reset", a.authed(a.handleReset))
	mux.Handle("GET /dash", a.authed(a.handleDash))
	mux.Handle("POST /sites", a.authed(a.handleCreateSite))
	mux.Handle("GET /site/{id}", a.authed(a.handleSite))
	mux.Handle("POST /sites/{id}/delete", a.authed(a.handleDeleteSite))

	mux.HandleFunc("GET /", a.handleRoot)
	return mux
}

func (a *App) authed(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.loggedIn(r) {
			http.Redirect(w, r, "/auth", http.StatusSeeOther)
			return
		}
		if must, _ := a.store.MustChange(); must && r.URL.Path != "/reset" {
			http.Redirect(w, r, "/reset", http.StatusSeeOther)
			return
		}
		h(w, r)
	})
}

func (a *App) loggedIn(r *http.Request) bool {
	c, err := r.Cookie("session")
	if err != nil {
		return false
	}
	return a.verify(c.Value)
}

func (a *App) setSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    a.sign("auth"),
		Path:     "/",
		HttpOnly: true,
		Secure:   a.cfg.Secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   60 * 60 * 24 * 30,
	})
}

func (a *App) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := a.tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("render %s: %v", name, err)
	}
}

func (a *App) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/dash", http.StatusSeeOther)
}

func (a *App) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	if a.loggedIn(r) {
		http.Redirect(w, r, "/dash", http.StatusSeeOther)
		return
	}
	a.render(w, "auth.html", authView{AppName: a.cfg.AppName})
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	ok, err := a.store.CheckPassword(r.FormValue("password"))
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if r.FormValue("username") != adminUser || !ok {
		w.WriteHeader(http.StatusUnauthorized)
		a.render(w, "auth.html", authView{AppName: a.cfg.AppName, Error: "Wrong username or password."})
		return
	}
	must, _ := a.store.MustChange()
	if must {
		if err := a.store.ConsumeTempPassword(); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}
	a.setSession(w)
	if must {
		http.Redirect(w, r, "/reset", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/dash", http.StatusSeeOther)
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	http.Redirect(w, r, "/auth", http.StatusSeeOther)
}

func (a *App) handleResetForm(w http.ResponseWriter, r *http.Request) {
	a.render(w, "reset.html", authView{AppName: a.cfg.AppName})
}

func (a *App) handleReset(w http.ResponseWriter, r *http.Request) {
	pw := r.FormValue("password")
	if pw == "" {
		a.render(w, "reset.html", authView{AppName: a.cfg.AppName, Error: "Password required."})
		return
	}
	if r.FormValue("confirm") != pw {
		a.render(w, "reset.html", authView{AppName: a.cfg.AppName, Error: "Passwords do not match."})
		return
	}
	if err := a.store.SetPassword(pw); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	a.store.SetMustChange(false)
	http.Redirect(w, r, "/dash", http.StatusSeeOther)
}

func (a *App) handleDash(w http.ResponseWriter, r *http.Request) {
	sites, err := a.store.ListSites()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	a.render(w, "dash.html", dashView{AppName: a.cfg.AppName, Sites: sites})
}

func (a *App) handleCreateSite(w http.ResponseWriter, r *http.Request) {
	if domain := strings.TrimSpace(r.FormValue("domain")); domain != "" {
		a.store.CreateSite(domain)
	}
	http.Redirect(w, r, "/dash", http.StatusSeeOther)
}

func (a *App) handleSite(w http.ResponseWriter, r *http.Request) {
	site, err := a.store.GetSite(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	now := time.Now()
	today, _ := a.store.VisitsToday(site.ID, now, a.cfg.Loc)
	week, _ := a.store.VisitsLast7Days(site.ID, now, a.cfg.Loc)
	days, _ := a.store.DailyBreakdown(site.ID, now, a.cfg.Loc)
	pages, _ := a.store.TopPages(site.ID, now, a.cfg.Loc, 10)
	refs, _ := a.store.TopReferrers(site.ID, now, a.cfg.Loc, 10)
	snippet := fmt.Sprintf(`<script defer src="%s/atag.js" data-site="%s"></script>`, a.cfg.SiteDomain, site.ID)
	a.render(w, "site.html", siteView{
		AppName: a.cfg.AppName,
		Site:    site,
		Today:   today,
		Week:    week,
		Days:    days,
		Pages:   pages,
		Refs:    refs,
		Snippet: snippet,
	})
}

func (a *App) handleDeleteSite(w http.ResponseWriter, r *http.Request) {
	a.store.DeleteSite(r.PathValue("id"))
	http.Redirect(w, r, "/dash", http.StatusSeeOther)
}

func (a *App) handleTracker(w http.ResponseWriter, r *http.Request) {
	b, err := assetsFS.ReadFile("assets/tracker.js")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Write(b)
}

func (a *App) handleEventPreflight(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleEvent(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var in struct {
		Site     string `json:"site"`
		Path     string `json:"path"`
		Referrer string `json:"referrer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Site == "" || in.Path == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	site, err := a.store.GetSite(in.Site)
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	a.store.InsertEvent(site.ID, in.Path, referrerHost(in.Referrer, site.Domain))
	w.WriteHeader(http.StatusNoContent)
}

func setCORS(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "Content-Type")
}

func referrerHost(ref, siteDomain string) string {
	rh := hostOf(ref)
	if rh == "" || rh == hostOf(siteDomain) {
		return "Direct"
	}
	return rh
}

func hostOf(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "//") {
		s = "//" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(u.Host, "www.")
}
