// Package server implements the AmneziaWG web panel: a session-authenticated,
// htmx-driven dashboard for viewing live client traffic and managing clients.
package server

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/skip2/go-qrcode"

	"github.com/hennessyxo/awg-suite/internal/auth"
	"github.com/hennessyxo/awg-suite/internal/awg"
	"github.com/hennessyxo/awg-suite/internal/awgctl"
	"github.com/hennessyxo/awg-suite/internal/format"
	"github.com/hennessyxo/awg-suite/internal/lifecycle"
	"github.com/hennessyxo/awg-suite/internal/shaper"
	"github.com/hennessyxo/awg-suite/internal/sysstat"
	"github.com/hennessyxo/awg-suite/internal/web"
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
	limiter  *auth.Limiter    // login brute-force protection
	pwHash   string
	iface    string
	secure   bool // set the Secure flag on cookies (true behind HTTPS)
	tmpl     *template.Template
	mux      *http.ServeMux
	rates    *rateTracker
	sys      *sysstat.Collector
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
		limiter:  auth.NewLimiter(5, 10*time.Minute, 15*time.Minute),
		pwHash:   pwHash,
		iface:    iface,
		secure:   secure,
		tmpl:     tmpl,
		rates:    newRateTracker(),
		sys:      sysstat.NewCollector(),
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
	_ = s.store.RecordSamples(now) // daily snapshot for day/week/month usage
	for _, rec := range s.store.List() {
		switch lifecycle.Evaluate(rec, now) {
		case lifecycle.ActionDelete:
			_ = s.ctrl.RevokeClient(rec.Name)
		case lifecycle.ActionDisable:
			_ = s.ctrl.DisableClient(rec.Name)
		}
	}
	// Reconcile bandwidth caps every cycle (the plan is idempotent). This drops tc
	// rules for clients just disabled, and applies speed changes made out of band,
	// e.g. via `awg-panel client-set` from the desktop app.
	s.ReconcileShaper()
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
	mux.HandleFunc("GET /lang/{code}", s.setLang)
	mux.HandleFunc("GET /{$}", s.requireAuth(s.dashboard))
	mux.HandleFunc("GET /server", s.requireAuth(s.serverPage))
	mux.HandleFunc("GET /partials/server", s.requireAuth(s.serverPartial))
	mux.HandleFunc("GET /partials/clients", s.requireAuth(s.clientsPartial))
	mux.HandleFunc("POST /clients", s.requireAuth(s.addClient))
	mux.HandleFunc("POST /clients/{name}/revoke", s.requireAuth(s.revokeClient))
	mux.HandleFunc("GET /clients/{name}/edit", s.requireAuth(s.editForm))
	mux.HandleFunc("POST /clients/{name}/update", s.requireAuth(s.updateClient))
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
	lang := s.lang(r)
	s.render(w, "login", map[string]any{"L": tr(lang), "Lang": lang})
}

func (s *Server) doLogin(w http.ResponseWriter, r *http.Request) {
	lang := s.lang(r)
	ip := clientIP(r)

	if locked, until := s.limiter.Locked(ip); locked {
		w.WriteHeader(http.StatusTooManyRequests)
		mins := int(time.Until(until).Minutes()) + 1
		s.render(w, "login", map[string]any{
			"Error": fmt.Sprintf("%s (~%d мин)", tr(lang)["login_locked"], mins),
			"L":     tr(lang), "Lang": lang,
		})
		return
	}

	pw := r.FormValue("password")
	if !auth.CheckPassword(s.pwHash, pw) {
		s.limiter.Fail(ip)
		w.WriteHeader(http.StatusUnauthorized)
		s.render(w, "login", map[string]any{"Error": tr(lang)["login_err"], "L": tr(lang), "Lang": lang})
		return
	}
	s.limiter.Reset(ip)
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
	lang := s.lang(r)
	s.render(w, "dashboard", map[string]any{"Iface": s.iface, "CSRF": sess.CSRF, "L": tr(lang), "Lang": lang, "Nav": "clients"})
}

// serverPage renders the server-overview page shell (the stats load via htmx).
func (s *Server) serverPage(w http.ResponseWriter, r *http.Request) {
	lang := s.lang(r)
	s.render(w, "serverpage", map[string]any{"Iface": s.iface, "L": tr(lang), "Lang": lang, "Nav": "server"})
}

// serverPartial renders the live server stats block (CPU/RAM/disk + traffic).
func (s *Server) serverPartial(w http.ResponseWriter, r *http.Request) {
	s.render(w, "serverstats", s.buildServerData(s.lang(r)))
}

func (s *Server) clientsPartial(w http.ResponseWriter, r *http.Request) {
	sess, _ := s.session(r)
	data, err := s.buildClientsData(sess.CSRF, s.lang(r), r.URL.Query().Get("sort"))
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(w, `<p class="err">%s</p>`, template.HTMLEscapeString(err.Error()))
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
		AllowedIPs: safeConfList(r.FormValue("allowed_ips")),
		DNS:        safeConfList(r.FormValue("dns")),
		MTU:        safeMTU(r.FormValue("mtu")),
	}
	client, err := s.ctrl.AddClient(name, opts)
	if err != nil {
		w.WriteHeader(http.StatusConflict)
		fmt.Fprintf(w, `<div class="created"><div class="created-head">Ошибка: %s</div></div>`, template.HTMLEscapeString(err.Error()))
		return
	}
	s.ReconcileShaper() // apply the new client's speed cap, if any
	lang := s.lang(r)
	s.render(w, "created", map[string]any{"Name": client.Name, "Config": client.Config, "L": tr(lang), "Lang": lang})
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
	data, err := s.buildClientsData(sess.CSRF, s.lang(r), r.URL.Query().Get("sort"))
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	s.render(w, "clients", data)
}

// editForm renders the inline edit form prefilled with the client's current limits.
func (s *Server) editForm(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	sess, _ := s.session(r)
	lang := s.lang(r)
	data := map[string]any{
		"Name": name, "CSRF": sess.CSRF, "L": tr(lang), "Lang": lang,
		"Days": "", "QuotaGB": "", "SpeedMbit": "",
	}
	if s.store != nil {
		if rec, ok := s.store.Get(name); ok {
			if rec.QuotaBytes > 0 {
				data["QuotaGB"] = strconv.FormatUint(rec.QuotaBytes/(1<<30), 10)
			}
			if rec.SpeedMbit > 0 {
				data["SpeedMbit"] = strconv.Itoa(rec.SpeedMbit)
			}
			if rec.ExpiresAt != nil {
				if d := int(time.Until(*rec.ExpiresAt).Hours() / 24); d > 0 {
					data["Days"] = strconv.Itoa(d)
				}
			}
		}
	}
	s.render(w, "edit", data)
}

// updateClient applies a rename (if changed) and new limits to an existing client.
func (s *Server) updateClient(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		http.Error(w, "bad csrf token", http.StatusForbidden)
		return
	}
	current := r.PathValue("name")
	effective := current
	if newName, ok := awgctl.SanitizeName(r.FormValue("name")); ok && newName != current {
		if err := s.ctrl.RenameClient(current, newName); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `<p class="err">%s</p>`, template.HTMLEscapeString(err.Error()))
			return
		}
		effective = newName
	}
	opts := awgctl.UpdateOptions{
		ExpiresIn:  daysToDuration(r.FormValue("expires_days")),
		QuotaBytes: gbToBytes(r.FormValue("quota_gb")),
		SpeedMbit:  atoiNonNeg(r.FormValue("speed_mbit")),
	}
	if err := s.ctrl.UpdateClient(effective, opts); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `<p class="err">%s</p>`, template.HTMLEscapeString(err.Error()))
		return
	}
	s.ReconcileShaper()
	s.renderClients(w, r)
}

func (s *Server) qrPNG(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.ctrl.ClientConfig(r.PathValue("name"))
	if err != nil {
		http.Error(w, "config unavailable", http.StatusNotFound)
		return
	}
	png, err := qrcode.Encode(cfg, qrcode.Low, 512)
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
	Today, Week, Month           string // usage over the last day/week/month
	Online, HasConfig, Disabled  bool

	// sort keys (not rendered)
	totalBytes, todayBytes, weekBytes, monthBytes uint64
}

type clientsData struct {
	Peers                     []peerView
	Online, Total             int
	TotalRx, TotalTx, TimeStr string
	CSRF                      string
	Sort                      string // active sort mode (echoed into the poll URL)
	L                         map[string]string
	Lang                      string
}

// adoptOrphans registers clients that exist in the server config but not yet in
// the lifecycle store (created via the installer/CLI), so the panel can manage
// them too. Idempotent.
func (s *Server) adoptOrphans() {
	if s.store == nil {
		return
	}
	clients, err := s.ctrl.ServerClients()
	if err != nil {
		return
	}
	for _, sc := range clients {
		if _, ok := s.store.Get(sc.Name); !ok {
			_ = s.store.Put(lifecycle.Record{
				Name: sc.Name, PubKey: sc.PubKey, Octet: sc.Octet,
				PeerBlock: sc.Block, CreatedAt: time.Now(),
			})
		}
	}
}

func (s *Server) buildClientsData(csrf, lang, sortMode string) (clientsData, error) {
	L := tr(lang)
	s.adoptOrphans()
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
		today, week, month := rec.Today(now), rec.Last7d(now), rec.Last30d(now)
		views = append(views, peerView{
			Name:         name,
			Endpoint:     endpoint,
			RateRx:       format.HumanRate(rx),
			RateTx:       format.HumanRate(tx),
			RxStr:        format.HumanBytes(p.RxBytes),
			TxStr:        format.HumanBytes(p.TxBytes),
			HandshakeAgo: format.Ago(p.LatestHandshake, now),
			Usage:        usageStr(rec),
			Expires:      expiresStr(rec, now, L),
			Speed:        speedStr(rec, L),
			Today:        format.HumanBytes(today),
			Week:         format.HumanBytes(week),
			Month:        format.HumanBytes(month),
			Online:       p.Online(now),
			HasConfig:    true,
			totalBytes:   p.RxBytes + p.TxBytes,
			todayBytes:   today,
			weekBytes:    week,
			monthBytes:   month,
		})
	}

	// Disabled clients are absent from the live config; list them too.
	for _, rec := range recs {
		if rec.Disabled && !live[rec.Name] {
			today, week, month := rec.Today(now), rec.Last7d(now), rec.Last30d(now)
			views = append(views, peerView{
				Name:       rec.Name,
				Endpoint:   "—",
				RateRx:     format.HumanRate(0),
				RateTx:     format.HumanRate(0),
				RxStr:      "—",
				TxStr:      "—",
				Usage:      usageStr(rec),
				Expires:    expiresStr(rec, now, L),
				Speed:      speedStr(rec, L),
				Today:      format.HumanBytes(today),
				Week:       format.HumanBytes(week),
				Month:      format.HumanBytes(month),
				Disabled:   true,
				HasConfig:  true,
				todayBytes: today,
				weekBytes:  week,
				monthBytes: month,
			})
		}
	}

	sortMode = sortViews(views, sortMode)

	return clientsData{
		Peers:   views,
		Online:  snap.OnlineCount(),
		Total:   len(views),
		TotalRx: format.HumanBytes(snap.TotalRx()),
		TotalTx: format.HumanBytes(snap.TotalTx()),
		TimeStr: now.Format("15:04:05"),
		CSRF:    csrf,
		Sort:    sortMode,
		L:       L,
		Lang:    lang,
	}, nil
}

// sortViews orders the table by the chosen mode and returns the normalized mode
// (default "online" — the online-then-name order already applied above).
func sortViews(v []peerView, mode string) string {
	switch mode {
	case "name":
		sort.SliceStable(v, func(i, j int) bool { return v[i].Name < v[j].Name })
	case "total":
		sort.SliceStable(v, func(i, j int) bool { return v[i].totalBytes > v[j].totalBytes })
	case "today":
		sort.SliceStable(v, func(i, j int) bool { return v[i].todayBytes > v[j].todayBytes })
	case "week":
		sort.SliceStable(v, func(i, j int) bool { return v[i].weekBytes > v[j].weekBytes })
	case "month":
		sort.SliceStable(v, func(i, j int) bool { return v[i].monthBytes > v[j].monthBytes })
	default:
		mode = "online"
	}
	return mode
}

// --- server overview --------------------------------------------------------

type srvTop struct {
	Name  string
	Value string
	Pct   int
}

type srvBar struct {
	Pct   int
	Label string
	Value string
}

type serverData struct {
	CPU       string
	Load      string
	MemUsed   string
	MemTotal  string
	MemPct    int
	DiskUsed  string
	DiskTotal string
	DiskPct   int
	HasDisk   bool
	Uptime    string

	Today   string
	Week    string
	Month   string
	TotalRx string
	TotalTx string

	Top   []srvTop
	Spark []srvBar
	Peak  string // largest single day in the 30-day window

	TimeStr string
	L       map[string]string
	Lang    string
	Nav     string
}

// buildServerData assembles the server overview: host load (CPU/RAM/disk/uptime),
// traffic totals over day/week/month, top clients, and a 30-day daily series.
func (s *Server) buildServerData(lang string) serverData {
	L := tr(lang)
	st := s.sys.Sample()
	now := time.Now()

	d := serverData{
		CPU:      fmt.Sprintf("%.0f", st.CPUPercent),
		Load:     fmt.Sprintf("%.2f / %.2f / %.2f", st.Load1, st.Load5, st.Load15),
		Uptime:   fmtUptime(st.UptimeSeconds, lang),
		MemUsed:  format.HumanBytes(st.MemUsedBytes),
		MemTotal: format.HumanBytes(st.MemTotalBytes),
		MemPct:   pct(st.MemUsedBytes, st.MemTotalBytes),
		TimeStr:  now.Format("15:04:05"),
		L:        L,
		Lang:     lang,
		Nav:      "server",
	}
	if st.DiskTotalBytes > 0 {
		d.HasDisk = true
		d.DiskUsed = format.HumanBytes(st.DiskUsedBytes)
		d.DiskTotal = format.HumanBytes(st.DiskTotalBytes)
		d.DiskPct = pct(st.DiskUsedBytes, st.DiskTotalBytes)
	}

	// Traffic totals + per-client month usage for the top list.
	var today, week, month uint64
	type nameBytes struct {
		name  string
		bytes uint64
	}
	var monthly []nameBytes
	if s.store != nil {
		for _, r := range s.store.List() {
			today += r.Today(now)
			week += r.Last7d(now)
			m := r.Last30d(now)
			month += m
			monthly = append(monthly, nameBytes{r.Name, m})
		}
	}
	d.Today = format.HumanBytes(today)
	d.Week = format.HumanBytes(week)
	d.Month = format.HumanBytes(month)

	sort.Slice(monthly, func(i, j int) bool { return monthly[i].bytes > monthly[j].bytes })
	var maxMonth uint64
	if len(monthly) > 0 {
		maxMonth = monthly[0].bytes
	}
	for i, m := range monthly {
		if i >= 3 || m.bytes == 0 {
			break
		}
		d.Top = append(d.Top, srvTop{Name: m.name, Value: format.HumanBytes(m.bytes), Pct: pct(m.bytes, maxMonth)})
	}

	// 30-day daily traffic series for the sparkline.
	if s.store != nil {
		series := s.store.DailyTotals(now, 30)
		var maxDay uint64
		for _, p := range series {
			if p.Bytes > maxDay {
				maxDay = p.Bytes
			}
		}
		d.Peak = format.HumanBytes(maxDay)
		for _, p := range series {
			h := pct(p.Bytes, maxDay)
			if h < 3 {
				h = 3 // keep an empty day visible as a sliver
			}
			d.Spark = append(d.Spark, srvBar{Pct: h, Label: p.Date, Value: format.HumanBytes(p.Bytes)})
		}
	}

	// All-time totals since the interface came up (live counters).
	if snap, err := s.ctrl.Snapshot(); err == nil {
		d.TotalRx = format.HumanBytes(snap.TotalRx())
		d.TotalTx = format.HumanBytes(snap.TotalTx())
	}
	return d
}

// pct returns used/total as an integer percentage, clamped to [0,100].
func pct(used, total uint64) int {
	if total == 0 {
		return 0
	}
	p := int(used * 100 / total)
	if p > 100 {
		p = 100
	}
	return p
}

// fmtUptime renders seconds as a localized "Nd Nh" / "Nh Nm" / "Nm".
func fmtUptime(sec int64, lang string) string {
	if sec <= 0 {
		return "—"
	}
	d := sec / 86400
	h := (sec % 86400) / 3600
	m := (sec % 3600) / 60
	if lang == "en" {
		switch {
		case d > 0:
			return fmt.Sprintf("%dd %dh", d, h)
		case h > 0:
			return fmt.Sprintf("%dh %dm", h, m)
		default:
			return fmt.Sprintf("%dm", m)
		}
	}
	switch {
	case d > 0:
		return fmt.Sprintf("%d дн %d ч", d, h)
	case h > 0:
		return fmt.Sprintf("%d ч %d мин", h, m)
	default:
		return fmt.Sprintf("%d мин", m)
	}
}

// usageStr renders "used / quota" or "" when unlimited.
func usageStr(rec lifecycle.Record) string {
	if rec.QuotaBytes == 0 {
		return ""
	}
	return format.HumanBytes(rec.UsedBytes) + " / " + format.HumanBytes(rec.QuotaBytes)
}

// expiresStr renders remaining time ("5d", "3h"), the localized "expired", or ""
// when there is no expiry.
func expiresStr(rec lifecycle.Record, now time.Time, L map[string]string) string {
	if rec.ExpiresAt == nil {
		return ""
	}
	d := rec.ExpiresAt.Sub(now)
	if d <= 0 {
		return L["expired"]
	}
	if days := int(d.Hours() / 24); days >= 1 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

// speedStr renders the bandwidth cap, or "" when unlimited.
func speedStr(rec lifecycle.Record, L map[string]string) string {
	if rec.SpeedMbit <= 0 {
		return ""
	}
	return fmt.Sprintf("%d %s", rec.SpeedMbit, L["speed_unit"])
}

// safeConfList accepts an AllowedIPs/DNS value only if it contains nothing but
// the safe charset (IP digits/hex, dots, colons, slashes, commas, spaces) — so a
// newline can't inject extra config lines. Returns "" (use the default) otherwise.
func safeConfList(s string) string {
	s = strings.TrimSpace(s)
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F',
			r == '.', r == ':', r == '/', r == ',', r == ' ':
		default:
			return ""
		}
	}
	return s
}

// safeMTU returns the MTU if it is a plausible value, else "" (use the default).
func safeMTU(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if n, err := strconv.Atoi(s); err != nil || n < 576 || n > 9000 {
		return ""
	}
	return s
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

// clientIP extracts the source IP (without port) from the request.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
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
