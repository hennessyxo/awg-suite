// Package server implements the AmneziaWG web panel: a session-authenticated,
// htmx-driven dashboard for viewing live client traffic and managing clients.
package server

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/skip2/go-qrcode"

	"github.com/hennessyxo/amneziawg-installer/internal/auth"
	"github.com/hennessyxo/amneziawg-installer/internal/awg"
	"github.com/hennessyxo/amneziawg-installer/internal/awgctl"
	"github.com/hennessyxo/amneziawg-installer/internal/format"
	"github.com/hennessyxo/amneziawg-installer/internal/lifecycle"
	"github.com/hennessyxo/amneziawg-installer/internal/shaper"
	"github.com/hennessyxo/amneziawg-installer/internal/web"
)

const (
	cookieName = "awgsess"
	subnetBase = "10.66.66." // VPN subnet prefix used for tc filters
)

// Server holds the panel's dependencies and HTTP routes.
type Server struct {
	ctrl     awgctl.Controller
	sessions *auth.Store
	store    *lifecycle.Store // lifecycle metadata (may be nil in tests)
	pwHash   string
	iface    string
	secure   bool // set the Secure flag on cookies (true behind HTTPS)
	tmpl     *template.Template
	mux      *http.ServeMux
	rates    *rateTracker
}

// New builds a Server. It takes the Controller interface (so tests can inject a
// fake) and returns the concrete struct. store may be nil (lifecycle disabled).
func New(ctrl awgctl.Controller, sessions *auth.Store, store *lifecycle.Store, pwHash, iface string, secure bool) (*Server, error) {
	tmpl, err := template.ParseFS(web.Templates, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parsing templates: %w", err)
	}
	s := &Server{
		ctrl:     ctrl,
		sessions: sessions,
		store:    store,
		pwHash:   pwHash,
		iface:    iface,
		secure:   secure,
		tmpl:     tmpl,
		rates:    newRateTracker(),
	}
	s.routes()
	return s, nil
}

// StartEnforcer runs the quota/expiry reconciliation loop until ctx is done.
// Each tick it accounts traffic, disables over-quota clients, and deletes
// expired ones. No-op when no lifecycle store is configured.
func (s *Server) StartEnforcer(ctx context.Context, interval time.Duration) {
	if s.store == nil {
		return
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			s.enforceOnce()
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}

func (s *Server) enforceOnce() {
	snap, err := s.ctrl.Snapshot()
	if err != nil {
		return
	}
	transfers := make(map[string]lifecycle.Transfer, len(snap.Peers))
	for _, p := range snap.Peers {
		transfers[p.PublicKey] = lifecycle.Transfer{Rx: p.RxBytes, Tx: p.TxBytes}
	}
	_ = s.store.ApplyUsage(transfers)

	now := time.Now()
	changed := false
	for _, rec := range s.store.List() {
		switch lifecycle.Evaluate(rec, now) {
		case lifecycle.ActionDelete:
			_ = s.ctrl.RevokeClient(rec.Name)
			changed = true
		case lifecycle.ActionDisable:
			_ = s.ctrl.DisableClient(rec.Name)
			changed = true
		}
	}
	if changed {
		s.ReconcileShaper() // drop tc rules for clients just disabled/removed
	}
}

// ReconcileShaper rebuilds tc bandwidth caps from the lifecycle store. It is
// best-effort (logged, not fatal): tc needs root and the kernel HTB module.
func (s *Server) ReconcileShaper() {
	if s.store == nil {
		return
	}
	var limits []shaper.Limit
	for _, r := range s.store.List() {
		if !r.Disabled && r.SpeedMbit > 0 {
			limits = append(limits, shaper.Limit{Octet: r.Octet, Mbit: r.SpeedMbit})
		}
	}
	if err := shaper.Apply(shaper.Plan(s.iface, subnetBase, limits)); err != nil {
		log.Printf("awg-panel: shaper reconcile: %v", err)
	}
}

// Handler returns the HTTP handler for the panel.
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /login", s.loginPage)
	mux.HandleFunc("POST /login", s.doLogin)
	mux.HandleFunc("POST /logout", s.doLogout)
	mux.HandleFunc("GET /{$}", s.requireAuth(s.dashboard))
	mux.HandleFunc("GET /partials/clients", s.requireAuth(s.clientsPartial))
	mux.HandleFunc("POST /clients", s.requireAuth(s.addClient))
	mux.HandleFunc("POST /clients/{name}/revoke", s.requireAuth(s.revokeClient))
	mux.HandleFunc("POST /clients/{name}/disable", s.requireAuth(s.toggleClient(false)))
	mux.HandleFunc("POST /clients/{name}/enable", s.requireAuth(s.toggleClient(true)))
	mux.HandleFunc("GET /clients/{name}/qr.png", s.requireAuth(s.qrPNG))
	mux.HandleFunc("GET /clients/{name}/config", s.requireAuth(s.downloadConfig))

	staticFS, _ := fs.Sub(web.Static, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	s.mux = mux
}

// --- auth plumbing ---------------------------------------------------------

func (s *Server) session(r *http.Request) (auth.Session, bool) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return auth.Session{}, false
	}
	return s.sessions.Valid(c.Value)
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.session(r); !ok {
			// htmx requests can't follow a 303; tell htmx to redirect instead.
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", "/login")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

// checkCSRF validates the form's csrf token against the session.
func (s *Server) checkCSRF(r *http.Request) bool {
	sess, ok := s.session(r)
	if !ok {
		return false
	}
	return r.FormValue("csrf") != "" && r.FormValue("csrf") == sess.CSRF
}

// --- handlers --------------------------------------------------------------

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.session(r); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.render(w, "login", map[string]any{})
}

func (s *Server) doLogin(w http.ResponseWriter, r *http.Request) {
	pw := r.FormValue("password")
	if !auth.CheckPassword(s.pwHash, pw) {
		w.WriteHeader(http.StatusUnauthorized)
		s.render(w, "login", map[string]any{"Error": "Неверный пароль"})
		return
	}
	token, _ := s.sessions.Create()
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) doLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		s.sessions.Delete(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	sess, _ := s.session(r)
	s.render(w, "dashboard", map[string]any{"Iface": s.iface, "CSRF": sess.CSRF})
}

func (s *Server) clientsPartial(w http.ResponseWriter, r *http.Request) {
	sess, _ := s.session(r)
	data, err := s.buildClientsData(sess.CSRF)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(w, `<p class="err">Не удалось получить данные: %s</p>`, template.HTMLEscapeString(err.Error()))
		return
	}
	s.render(w, "clients", data)
}

func (s *Server) addClient(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		http.Error(w, "bad csrf token", http.StatusForbidden)
		return
	}
	name, ok := awgctl.SanitizeName(r.FormValue("name"))
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `<div class="created"><div class="created-head">Некорректное имя клиента</div></div>`)
		return
	}
	opts := awgctl.AddOptions{
		ExpiresIn:  daysToDuration(r.FormValue("expires_days")),
		QuotaBytes: gbToBytes(r.FormValue("quota_gb")),
		SpeedMbit:  atoiNonNeg(r.FormValue("speed_mbit")),
	}
	client, err := s.ctrl.AddClient(name, opts)
	if err != nil {
		w.WriteHeader(http.StatusConflict)
		fmt.Fprintf(w, `<div class="created"><div class="created-head">Ошибка: %s</div></div>`, template.HTMLEscapeString(err.Error()))
		return
	}
	s.ReconcileShaper() // apply the new client's speed cap, if any
	s.render(w, "created", map[string]any{"Name": client.Name, "Config": client.Config})
}

func (s *Server) revokeClient(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		http.Error(w, "bad csrf token", http.StatusForbidden)
		return
	}
	name := r.PathValue("name")
	if err := s.ctrl.RevokeClient(name); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `<p class="err">%s</p>`, template.HTMLEscapeString(err.Error()))
		return
	}
	s.ReconcileShaper()
	s.renderClients(w, r)
}

// toggleClient returns a handler that enables (true) or disables (false) a client.
func (s *Server) toggleClient(enable bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.checkCSRF(r) {
			http.Error(w, "bad csrf token", http.StatusForbidden)
			return
		}
		name := r.PathValue("name")
		var err error
		if enable {
			err = s.ctrl.EnableClient(name)
		} else {
			err = s.ctrl.DisableClient(name)
		}
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `<p class="err">%s</p>`, template.HTMLEscapeString(err.Error()))
			return
		}
		s.ReconcileShaper() // (re)apply or drop the client's speed cap
		s.renderClients(w, r)
	}
}

// renderClients re-renders the clients table partial (used after mutations).
func (s *Server) renderClients(w http.ResponseWriter, r *http.Request) {
	sess, _ := s.session(r)
	data, err := s.buildClientsData(sess.CSRF)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	s.render(w, "clients", data)
}

func (s *Server) qrPNG(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.ctrl.ClientConfig(r.PathValue("name"))
	if err != nil {
		http.Error(w, "config unavailable", http.StatusNotFound)
		return
	}
	png, err := qrcode.Encode(cfg, qrcode.Medium, 256)
	if err != nil {
		http.Error(w, "qr error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(png)
}

func (s *Server) downloadConfig(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	cfg, err := s.ctrl.ClientConfig(name)
	if err != nil {
		http.Error(w, "config unavailable", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-client-%s.conf"`, s.iface, name))
	fmt.Fprint(w, cfg)
}

// --- view assembly ---------------------------------------------------------

type peerView struct {
	Name, Endpoint               string
	RateRx, RateTx, RxStr, TxStr string
	HandshakeAgo                 string
	Usage, Expires, Speed        string // lifecycle: "" when unlimited/never
	Online, HasConfig, Disabled  bool
}

type clientsData struct {
	Peers                     []peerView
	Online, Total             int
	TotalRx, TotalTx, TimeStr string
	CSRF                      string
}

func (s *Server) buildClientsData(csrf string) (clientsData, error) {
	snap, err := s.ctrl.Snapshot()
	if err != nil {
		return clientsData{}, err
	}
	s.rates.update(snap)
	now := snap.Time

	// Lifecycle records, keyed by name, to enrich the live peers.
	recs := map[string]lifecycle.Record{}
	if s.store != nil {
		for _, r := range s.store.List() {
			recs[r.Name] = r
		}
	}

	peers := make([]awg.Peer, len(snap.Peers))
	copy(peers, snap.Peers)
	sort.SliceStable(peers, func(i, j int) bool {
		oi, oj := peers[i].Online(now), peers[j].Online(now)
		if oi != oj {
			return oi
		}
		return displayName(peers[i]) < displayName(peers[j])
	})

	views := make([]peerView, 0, len(peers)+len(recs))
	live := map[string]bool{}
	for _, p := range peers {
		name := displayName(p)
		live[name] = true
		rx, tx := s.rates.rate(p.PublicKey)
		endpoint := p.Endpoint
		if endpoint == "" {
			endpoint = "—"
		}
		rec := recs[name]
		views = append(views, peerView{
			Name:         name,
			Endpoint:     endpoint,
			RateRx:       format.HumanRate(rx),
			RateTx:       format.HumanRate(tx),
			RxStr:        format.HumanBytes(p.RxBytes),
			TxStr:        format.HumanBytes(p.TxBytes),
			HandshakeAgo: format.Ago(p.LatestHandshake, now),
			Usage:        usageStr(rec),
			Expires:      expiresStr(rec, now),
			Speed:        speedStr(rec),
			Online:       p.Online(now),
			HasConfig:    true,
		})
	}

	// Disabled clients are absent from the live config; list them too.
	for _, rec := range recs {
		if rec.Disabled && !live[rec.Name] {
			views = append(views, peerView{
				Name:      rec.Name,
				Endpoint:  "—",
				RateRx:    format.HumanRate(0),
				RateTx:    format.HumanRate(0),
				RxStr:     "—",
				TxStr:     "—",
				Usage:     usageStr(rec),
				Expires:   expiresStr(rec, now),
				Speed:     speedStr(rec),
				Disabled:  true,
				HasConfig: true,
			})
		}
	}

	return clientsData{
		Peers:   views,
		Online:  snap.OnlineCount(),
		Total:   len(views),
		TotalRx: format.HumanBytes(snap.TotalRx()),
		TotalTx: format.HumanBytes(snap.TotalTx()),
		TimeStr: now.Format("15:04:05"),
		CSRF:    csrf,
	}, nil
}

// usageStr renders "used / quota" or "" when unlimited.
func usageStr(rec lifecycle.Record) string {
	if rec.QuotaBytes == 0 {
		return ""
	}
	return format.HumanBytes(rec.UsedBytes) + " / " + format.HumanBytes(rec.QuotaBytes)
}

// expiresStr renders remaining time ("5д", "3ч"), "истёк", or "" when no expiry.
func expiresStr(rec lifecycle.Record, now time.Time) string {
	if rec.ExpiresAt == nil {
		return ""
	}
	d := rec.ExpiresAt.Sub(now)
	if d <= 0 {
		return "истёк"
	}
	if days := int(d.Hours() / 24); days >= 1 {
		return fmt.Sprintf("%dд", days)
	}
	return fmt.Sprintf("%dч", int(d.Hours()))
}

// speedStr renders the bandwidth cap, or "" when unlimited.
func speedStr(rec lifecycle.Record) string {
	if rec.SpeedMbit <= 0 {
		return ""
	}
	return fmt.Sprintf("%d Мбит/с", rec.SpeedMbit)
}

// atoiNonNeg parses a non-negative integer from a form field (0 on error).
func atoiNonNeg(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// daysToDuration parses a positive integer day count into a Duration (0 = none).
func daysToDuration(s string) time.Duration {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n <= 0 {
		return 0
	}
	return time.Duration(n) * 24 * time.Hour
}

// gbToBytes parses a GB amount (float) into bytes (0 = unlimited).
func gbToBytes(s string) uint64 {
	g, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || g <= 0 {
		return 0
	}
	return uint64(g * (1 << 30))
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func displayName(p awg.Peer) string {
	if p.Name != "" {
		return p.Name
	}
	if len(p.PublicKey) > 8 {
		return p.PublicKey[:8]
	}
	return p.PublicKey
}

// --- rate tracker ----------------------------------------------------------

type rateTracker struct {
	mu       sync.Mutex
	prev     map[string]awg.Peer
	prevTime time.Time
	rx, tx   map[string]float64
}

func newRateTracker() *rateTracker {
	return &rateTracker{rx: map[string]float64{}, tx: map[string]float64{}}
}

func (rt *rateTracker) update(s awg.Snapshot) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.prev != nil {
		dt := s.Time.Sub(rt.prevTime).Seconds()
		if dt > 0 {
			for _, p := range s.Peers {
				old, ok := rt.prev[p.PublicKey]
				if !ok {
					continue
				}
				rt.rx[p.PublicKey] = deltaRate(p.RxBytes, old.RxBytes, dt)
				rt.tx[p.PublicKey] = deltaRate(p.TxBytes, old.TxBytes, dt)
			}
		}
	}
	rt.prev = make(map[string]awg.Peer, len(s.Peers))
	for _, p := range s.Peers {
		rt.prev[p.PublicKey] = p
	}
	rt.prevTime = s.Time
}

func (rt *rateTracker) rate(pub string) (rx, tx float64) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.rx[pub], rt.tx[pub]
}

func deltaRate(cur, old uint64, dt float64) float64 {
	if cur < old || dt <= 0 {
		return 0
	}
	return float64(cur-old) / dt
}
