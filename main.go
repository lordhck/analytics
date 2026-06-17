package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"analytics/internal/store"
)

var version = "dev"

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed assets/*
var assetsFS embed.FS

type Config struct {
	Port       string
	DBPath     string
	AppName    string
	SiteDomain string
	Loc        *time.Location
	Secure     bool
}

type App struct {
	cfg    Config
	store  *store.Store
	secret []byte
	tmpl   *template.Template
}

func main() {
	cfg := loadConfig()

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	secret, err := st.SessionSecret()
	if err != nil {
		log.Fatalf("session secret: %v", err)
	}

	app := &App{
		cfg:    cfg,
		store:  st,
		secret: []byte(secret),
		tmpl:   template.Must(template.ParseFS(templatesFS, "templates/*.html")),
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           app.routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("analytics %s listening on :%s (%s)", version, cfg.Port, cfg.Loc)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func loadConfig() Config {
	domain := strings.TrimRight(getenv("SITE_DOMAIN", "http://localhost:8080"), "/")
	return Config{
		Port:       getenv("PORT", "8080"),
		DBPath:     getenv("DB_PATH", "data/analytics.db"),
		AppName:    getenv("APP_NAME", "Analytics"),
		SiteDomain: domain,
		Loc:        loadLocation(),
		Secure:     strings.HasPrefix(domain, "https://"),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func loadLocation() *time.Location {
	if tz := os.Getenv("TZ"); tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return loc
		}
	}
	return time.Local
}

func (a *App) sign(value string) string {
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte(value))
	return value + "." + hex.EncodeToString(mac.Sum(nil))
}

func (a *App) verify(token string) bool {
	i := strings.LastIndex(token, ".")
	if i < 0 {
		return false
	}
	value, sig := token[:i], token[i+1:]
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte(value))
	expected := hex.EncodeToString(mac.Sum(nil))
	return value == "auth" && hmac.Equal([]byte(sig), []byte(expected))
}
