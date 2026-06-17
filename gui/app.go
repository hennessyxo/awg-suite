package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	amneziawg "github.com/hennessyxo/amneziawg-installer"
	"github.com/hennessyxo/amneziawg-installer/internal/awgctl"
	"github.com/hennessyxo/amneziawg-installer/internal/deploy"
	"github.com/skip2/go-qrcode"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// serverConf is the AmneziaWG server config path; the interface name is a fixed
// constant in the installer (awg0).
const serverConf = "/etc/amnezia/amneziawg/awg0.conf"

// App is the Wails-bound backend. Its exported methods are callable from the
// frontend as window.go.main.App.*.
type App struct {
	ctx context.Context

	mu     sync.Mutex
	client *deploy.Client
	target deploy.Target
}

// NewApp constructs the backend in its disconnected state.
func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) { a.ctx = ctx }
func (a *App) shutdown(_ context.Context)  { a.closeClient() }

func (a *App) closeClient() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.client != nil {
		_ = a.client.Close()
		a.client = nil
	}
}

// --- request / response types (JSON-bound to the frontend) -----------------

// ConnectRequest carries the SSH connection parameters from the UI.
type ConnectRequest struct {
	Host         string `json:"host"`
	User         string `json:"user"`
	Password     string `json:"password"`
	IdentityPath string `json:"identityPath"`
}

// InstallRequest carries the install options from the UI.
type InstallRequest struct {
	Preset string `json:"preset"` // "default" | "mobile"
	Port   string `json:"port"`   // optional UDP port
	Client string `json:"client"` // first client name
}

// StatusResult reports whether the connected server already has AmneziaWG.
type StatusResult struct {
	Installed bool `json:"installed"`
}

// PanelResult reports the web panel state and the URL to reach it.
type PanelResult struct {
	Installed bool   `json:"installed"`
	URL       string `json:"url"`
}

// ClientResult is a freshly created client's config plus a scannable QR image
// rendered as a data URI for direct use in an <img src>.
type ClientResult struct {
	Name string `json:"name"`
	Conf string `json:"conf"`
	QR   string `json:"qr"`
}

// --- bound methods ---------------------------------------------------------

// Connect opens an SSH session to the server and stores it for later calls.
func (a *App) Connect(req ConnectRequest) error {
	host := strings.TrimSpace(req.Host)
	if host == "" {
		return fmt.Errorf("укажите адрес сервера")
	}
	user := strings.TrimSpace(req.User)
	if user == "" {
		user = "root"
	}
	t, err := deploy.ParseTarget(user + "@" + host)
	if err != nil {
		return err
	}

	cl, err := dial(t, strings.TrimSpace(req.IdentityPath), req.Password)
	if err != nil {
		return err
	}

	a.closeClient()
	a.mu.Lock()
	a.client = cl
	a.target = t
	a.mu.Unlock()
	return nil
}

// Disconnect closes the SSH session.
func (a *App) Disconnect() error {
	a.closeClient()
	return nil
}

// ServerStatus reports whether the server already has AmneziaWG installed.
func (a *App) ServerStatus() (StatusResult, error) {
	cl, t, err := a.conn()
	if err != nil {
		return StatusResult{}, err
	}
	out, err := cl.Run(deploy.CheckInstalledCommand(deploy.Sudo(t.User)))
	if err != nil {
		return StatusResult{}, fmt.Errorf("проверка сервера не удалась: %w", err)
	}
	return StatusResult{Installed: deploy.IsInstalled(out)}, nil
}

// Install runs the installer on the server, streaming progress to the UI via
// the "install:log" event, and returns the first client's config + QR.
func (a *App) Install(req InstallRequest) (ClientResult, error) {
	cl, t, err := a.conn()
	if err != nil {
		return ClientResult{}, err
	}
	client := strings.TrimSpace(req.Client)
	if client == "" {
		client = "phone"
	}
	preset := req.Preset
	if preset != "mobile" {
		preset = "default"
	}

	env := map[string]string{"AWG_PRESET": preset, "AWG_CLIENT": client}
	if p := strings.TrimSpace(req.Port); p != "" {
		env["AWG_PORT"] = p
	}

	out, err := cl.RunScript(deploy.InstallCommand(deploy.Sudo(t.User), env), amneziawg.InstallerScript, a.logWriter("install:log"))
	if err != nil {
		return ClientResult{}, fmt.Errorf("установка не удалась: %w", err)
	}
	if deploy.AlreadyInstalled(out) {
		return ClientResult{}, fmt.Errorf("сервер уже настроен — используйте управление клиентами")
	}
	return buildClientResult(client, out)
}

// AddClient creates a new client on the server and returns its config + QR.
func (a *App) AddClient(name string) (ClientResult, error) {
	cl, t, err := a.conn()
	if err != nil {
		return ClientResult{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return ClientResult{}, fmt.Errorf("укажите имя клиента")
	}
	out, err := cl.RunScript(deploy.AddClientCommand(deploy.Sudo(t.User), name), amneziawg.InstallerScript, a.logWriter("client:log"))
	if err != nil {
		return ClientResult{}, fmt.Errorf("создание клиента не удалось: %w", err)
	}
	return buildClientResult(name, out)
}

// RemoveClient deletes a client from the server.
func (a *App) RemoveClient(name string) error {
	cl, t, err := a.conn()
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("укажите имя клиента")
	}
	out, err := cl.RunScript(deploy.RemoveClientCommand(deploy.Sudo(t.User), name), amneziawg.InstallerScript, a.logWriter("client:log"))
	if err != nil {
		return fmt.Errorf("удаление не удалось: %w\n%s", err, out)
	}
	return nil
}

// ListClients returns the names of all clients configured on the server.
func (a *App) ListClients() ([]string, error) {
	cl, t, err := a.conn()
	if err != nil {
		return nil, err
	}
	out, err := cl.Run(deploy.Sudo(t.User) + "cat " + serverConf + " 2>/dev/null || true")
	if err != nil {
		return nil, fmt.Errorf("чтение конфигурации не удалось: %w", err)
	}
	clients := awgctl.ParseServerClients(out)
	names := make([]string, 0, len(clients))
	for _, c := range clients {
		names = append(names, c.Name)
	}
	sort.Strings(names)
	return names, nil
}

// panelURL builds the https URL the web panel is reachable at (the host the
// user connected to, on the panel port).
func (a *App) panelURL() string {
	return fmt.Sprintf("https://%s:%d", a.target.Host, deploy.PanelPort)
}

// PanelStatus reports whether the web panel is installed and its URL.
func (a *App) PanelStatus() (PanelResult, error) {
	cl, t, err := a.conn()
	if err != nil {
		return PanelResult{}, err
	}
	out, err := cl.Run(deploy.PanelInstalledCommand(deploy.Sudo(t.User)))
	if err != nil {
		return PanelResult{}, fmt.Errorf("проверка панели не удалась: %w", err)
	}
	return PanelResult{Installed: deploy.IsPanelInstalled(out), URL: a.panelURL()}, nil
}

// InstallPanel installs the web panel non-interactively with the given admin
// password (min 8 chars) and returns its URL. The panel carries the advanced
// per-client limits (speed/quota/expiry) and the enforcer daemon.
func (a *App) InstallPanel(password string) (PanelResult, error) {
	cl, t, err := a.conn()
	if err != nil {
		return PanelResult{}, err
	}
	if len(password) < 8 {
		return PanelResult{}, fmt.Errorf("пароль панели — минимум 8 символов")
	}
	out, err := cl.RunScript(deploy.InstallPanelCommand(deploy.Sudo(t.User), password), amneziawg.InstallerScript, a.logWriter("panel:log"))
	if err != nil {
		return PanelResult{}, fmt.Errorf("установка панели не удалась: %w\n%s", err, out)
	}
	return PanelResult{Installed: true, URL: a.panelURL()}, nil
}

// RemovePanel removes the web panel from the server.
func (a *App) RemovePanel() error {
	cl, t, err := a.conn()
	if err != nil {
		return err
	}
	out, err := cl.RunScript(deploy.RemovePanelCommand(deploy.Sudo(t.User)), amneziawg.InstallerScript, a.logWriter("panel:log"))
	if err != nil {
		return fmt.Errorf("удаление панели не удалось: %w\n%s", err, out)
	}
	return nil
}

// OpenPanel opens the web panel URL in the user's default browser.
func (a *App) OpenPanel() error {
	if a.ctx == nil {
		return fmt.Errorf("приложение не готово")
	}
	wruntime.BrowserOpenURL(a.ctx, a.panelURL())
	return nil
}

// Uninstall removes AmneziaWG and everything it set up from the server.
func (a *App) Uninstall() error {
	cl, t, err := a.conn()
	if err != nil {
		return err
	}
	out, err := cl.RunScript(deploy.UninstallCommand(deploy.Sudo(t.User)), amneziawg.InstallerScript, a.logWriter("install:log"))
	if err != nil {
		return fmt.Errorf("удаление не удалось: %w\n%s", err, out)
	}
	return nil
}

// --- helpers ---------------------------------------------------------------

// conn returns the live client and target, or an error if not connected.
func (a *App) conn() (*deploy.Client, deploy.Target, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.client == nil {
		return nil, deploy.Target{}, fmt.Errorf("нет подключения к серверу")
	}
	return a.client, a.target, nil
}

// logWriter returns an io.Writer that streams installer output to the UI as
// events under the given name.
func (a *App) logWriter(event string) *eventWriter {
	return &eventWriter{ctx: a.ctx, event: event}
}

// eventWriter forwards written bytes to the frontend as Wails runtime events.
type eventWriter struct {
	ctx   context.Context
	event string
}

func (w *eventWriter) Write(p []byte) (int, error) {
	if w.ctx != nil {
		wruntime.EventsEmit(w.ctx, w.event, string(p))
	}
	return len(p), nil
}

// buildClientResult extracts the client config from installer output and renders
// a scannable QR PNG as a data URI.
func buildClientResult(name, output string) (ClientResult, error) {
	conf, err := deploy.ExtractConfig(output)
	if err != nil {
		return ClientResult{}, fmt.Errorf("не нашёл конфиг клиента в выводе: %w", err)
	}
	res := ClientResult{Name: name, Conf: conf}
	// Low EC + a large image keeps the long AmneziaWG config QR scannable.
	png, err := qrcode.Encode(conf, qrcode.Low, 512)
	if err == nil {
		res.QR = "data:image/png;base64," + encodeBase64(png)
	}
	return res, nil
}
