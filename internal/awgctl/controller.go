package awgctl

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hennessyxo/amneziawg-installer/internal/awg"
	"github.com/hennessyxo/amneziawg-installer/internal/lifecycle"
)

// Client is a generated VPN client and its full configuration text.
type Client struct {
	Name   string
	Config string
}

// AddOptions carries optional lifecycle limits for a new client.
type AddOptions struct {
	ExpiresIn  time.Duration // 0 = never expires
	QuotaBytes uint64        // 0 = unlimited
	SpeedMbit  int           // bandwidth cap in Mbit/s (0 = unlimited)
}

// Controller is the panel's view of the running AmneziaWG server. It is an
// interface so HTTP handlers can be tested against a fake.
type Controller interface {
	Snapshot() (awg.Snapshot, error)
	AddClient(name string, opts AddOptions) (Client, error)
	RevokeClient(name string) error
	DisableClient(name string) error
	EnableClient(name string) error
	ClientConfig(name string) (string, error)
}

// FileController is the production Controller: it shells out to `awg`, edits the
// server config on disk, and records lifecycle metadata in the store. All the
// config-rewriting logic lives in the unit-tested pure functions above.
type FileController struct {
	Iface     string           // e.g. "awg0"
	ConfPath  string           // /etc/amnezia/amneziawg/awg0.conf
	ParamPath string           // /etc/amnezia/amneziawg/params
	ClientDir string           // where panel-generated client .conf files are stored
	Store     *lifecycle.Store // lifecycle metadata (quotas, expiry)
}

func (c FileController) Snapshot() (awg.Snapshot, error) {
	out, err := exec.Command("awg", "show", c.Iface, "dump").Output()
	if err != nil {
		return awg.Snapshot{}, fmt.Errorf("awg show: %w", err)
	}
	snap, err := awg.ParseDump(c.Iface, string(out), time.Now())
	if err != nil {
		return snap, err
	}
	if data, e := os.ReadFile(c.ConfPath); e == nil {
		awg.ApplyNames(snap.Peers, awg.ParseNames(string(data)))
	}
	return snap, nil
}

func (c FileController) AddClient(name string, opts AddOptions) (Client, error) {
	confBytes, err := os.ReadFile(c.ConfPath)
	if err != nil {
		return Client{}, err
	}
	conf := string(confBytes)
	if HasPeer(conf, name) {
		return Client{}, fmt.Errorf("client %q already exists", name)
	}

	paramBytes, err := os.ReadFile(c.ParamPath)
	if err != nil {
		return Client{}, err
	}
	params := ParseParams(string(paramBytes))

	priv, err := c.genKey()
	if err != nil {
		return Client{}, err
	}
	pub, err := c.pubKey(priv)
	if err != nil {
		return Client{}, err
	}
	psk, err := c.genPSK()
	if err != nil {
		return Client{}, err
	}
	octet, err := FreeOctet(conf, c.reservedOctets())
	if err != nil {
		return Client{}, err
	}

	clientCfg := RenderClientConfig(params, priv, psk, octet)
	block := PeerBlock(name, pub, psk, octet)

	if err := os.WriteFile(c.ConfPath, []byte(AppendBlock(conf, block)), 0o600); err != nil {
		return Client{}, err
	}
	if err := os.MkdirAll(c.ClientDir, 0o700); err != nil {
		return Client{}, err
	}
	if err := os.WriteFile(c.clientFile(name), []byte(clientCfg), 0o600); err != nil {
		return Client{}, err
	}
	if err := c.syncConf(); err != nil {
		return Client{}, err
	}

	c.record(name, pub, octet, block, opts)
	return Client{Name: name, Config: clientCfg}, nil
}

func (c FileController) RevokeClient(name string) error {
	confBytes, err := os.ReadFile(c.ConfPath)
	if err != nil {
		return err
	}
	newConf, _ := RemovePeerBlock(string(confBytes), name)
	if err := os.WriteFile(c.ConfPath, []byte(newConf), 0o600); err != nil {
		return err
	}
	_ = os.Remove(c.clientFile(name))
	if c.Store != nil {
		_ = c.Store.Delete(name)
	}
	return c.syncConf()
}

// DisableClient cuts a client off the live interface (and config) but keeps its
// record and stored peer block so it can be re-enabled.
func (c FileController) DisableClient(name string) error {
	confBytes, err := os.ReadFile(c.ConfPath)
	if err != nil {
		return err
	}
	newConf, _ := RemovePeerBlock(string(confBytes), name)
	if err := os.WriteFile(c.ConfPath, []byte(newConf), 0o600); err != nil {
		return err
	}
	if c.Store != nil {
		if rec, ok := c.Store.Get(name); ok {
			rec.Disabled = true
			_ = c.Store.Put(rec)
		}
	}
	return c.syncConf()
}

// EnableClient re-adds a previously disabled client from its stored peer block.
func (c FileController) EnableClient(name string) error {
	if c.Store == nil {
		return fmt.Errorf("no lifecycle store configured")
	}
	rec, ok := c.Store.Get(name)
	if !ok {
		return fmt.Errorf("client %q not found", name)
	}
	confBytes, err := os.ReadFile(c.ConfPath)
	if err != nil {
		return err
	}
	conf := string(confBytes)
	if !HasPeer(conf, name) {
		if err := os.WriteFile(c.ConfPath, []byte(AppendBlock(conf, rec.PeerBlock)), 0o600); err != nil {
			return err
		}
	}
	// Clear whatever caused the auto-disable so the enforcer doesn't immediately
	// re-disable: drop a past expiry, and reset usage if it was over quota.
	now := time.Now()
	if rec.ExpiresAt != nil && now.After(*rec.ExpiresAt) {
		rec.ExpiresAt = nil
	}
	if rec.QuotaBytes > 0 && rec.UsedBytes >= rec.QuotaBytes {
		rec.UsedBytes = 0
		rec.LastRx, rec.LastTx = 0, 0
	}
	rec.Disabled = false
	_ = c.Store.Put(rec)
	return c.syncConf()
}

func (c FileController) ClientConfig(name string) (string, error) {
	data, err := os.ReadFile(c.clientFile(name))
	if err != nil {
		return "", fmt.Errorf("config for %q unavailable (only panel-created clients are stored): %w", name, err)
	}
	return string(data), nil
}

// --- helpers ---------------------------------------------------------------

func (c FileController) reservedOctets() map[int]bool {
	if c.Store == nil {
		return nil
	}
	return c.Store.UsedOctets()
}

func (c FileController) record(name, pub string, octet int, block string, opts AddOptions) {
	if c.Store == nil {
		return
	}
	rec := lifecycle.Record{
		Name:       name,
		PubKey:     pub,
		Octet:      octet,
		PeerBlock:  block,
		CreatedAt:  time.Now(),
		QuotaBytes: opts.QuotaBytes,
		SpeedMbit:  opts.SpeedMbit,
	}
	if opts.ExpiresIn > 0 {
		exp := time.Now().Add(opts.ExpiresIn)
		rec.ExpiresAt = &exp
	}
	_ = c.Store.Put(rec)
}

func (c FileController) clientFile(name string) string {
	return filepath.Join(c.ClientDir, c.Iface+"-client-"+name+".conf")
}

func (c FileController) genKey() (string, error) { return runOut("awg", "genkey") }
func (c FileController) genPSK() (string, error) { return runOut("awg", "genpsk") }

func (c FileController) pubKey(priv string) (string, error) {
	cmd := exec.Command("awg", "pubkey")
	cmd.Stdin = strings.NewReader(priv + "\n")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("awg pubkey: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// syncConf applies config changes to the live interface without dropping peers.
func (c FileController) syncConf() error {
	stripped, err := exec.Command("awg-quick", "strip", c.Iface).Output()
	if err != nil {
		return fmt.Errorf("awg-quick strip: %w", err)
	}
	tmp, err := os.CreateTemp("", "awg-sync-*.conf")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(stripped); err != nil {
		return err
	}
	tmp.Close()
	if err := exec.Command("awg", "syncconf", c.Iface, tmp.Name()).Run(); err != nil {
		return fmt.Errorf("awg syncconf: %w", err)
	}
	return nil
}

func runOut(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return strings.TrimSpace(string(out)), nil
}
