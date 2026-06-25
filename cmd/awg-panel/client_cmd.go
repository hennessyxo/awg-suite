package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hennessyxo/awg-suite/internal/awgctl"
	"github.com/hennessyxo/awg-suite/internal/lifecycle"
	"github.com/hennessyxo/awg-suite/internal/shaper"
)

// subnetBase is the fixed VPN subnet prefix the installer uses; tc filters match
// clients by their host octet within it. Kept in sync with the installer and the
// panel server.
const subnetBase = "10.66.66."

const bytesPerGB = 1 << 30

// clientLimit is the JSON shape emitted by `client-list` and consumed by the
// desktop app to prefill the per-client limit controls.
type clientLimit struct {
	Name        string `json:"name"`
	Disabled    bool   `json:"disabled"`
	QuotaGB     int    `json:"quotaGB"`     // 0 = unlimited
	UsedBytes   uint64 `json:"usedBytes"`   // accumulated usage
	SpeedMbit   int    `json:"speedMbit"`   // 0 = unlimited
	ExpiresDays int    `json:"expiresDays"` // whole days left, 0 = no expiry
}

// runClientCmd implements the `awg-panel client-*` subcommands. They let another
// process (the desktop app, over SSH) read and change per-client limits through
// the same lifecycle store and shaper the daemon uses, with cross-process locking.
func runClientCmd(cmd string, args []string) {
	// Pull the client name (first positional) before flag parsing, since Go's flag
	// package stops at the first non-flag argument. This lets the name come first:
	// `awg-panel client-set phone --quota-gb 50`.
	var name string
	if cmd != "client-list" {
		if len(args) == 0 || strings.HasPrefix(args[0], "-") {
			fail(fmt.Errorf("client name required: %s <name> [flags]", cmd))
		}
		name = args[0]
		args = args[1:]
	}

	fs := flag.NewFlagSet("awg-panel "+cmd, flag.ExitOnError)
	iface := fs.String("iface", "awg0", "AmneziaWG interface")
	conf := fs.String("conf", "/etc/amnezia/amneziawg/awg0.conf", "server config path")
	params := fs.String("params", "/etc/amnezia/amneziawg/params", "installer params path")
	clientDir := fs.String("client-dir", "/etc/amnezia/amneziawg/clients", "client config dir")
	storePath := fs.String("store", "/etc/amnezia/amneziawg/clients.json", "lifecycle store")
	quotaGB := fs.Int("quota-gb", 0, "traffic quota in GB (0 = unlimited)")
	expiresDays := fs.Int("expires-days", 0, "expire after N days from now (0 = never)")
	speedMbit := fs.Int("speed-mbit", 0, "speed cap in Mbit/s (0 = unlimited)")
	_ = fs.Parse(args)

	store, err := lifecycle.Open(*storePath)
	if err != nil {
		fail(err)
	}
	ctrl := awgctl.FileController{
		Iface:     *iface,
		ConfPath:  *conf,
		ParamPath: *params,
		ClientDir: *clientDir,
		Store:     store,
	}

	// Register any clients that exist in the server config but not yet in the
	// store (created via the installer or the desktop app). The web panel does
	// this when its dashboard loads; doing it here too means the desktop app can
	// manage limits without anyone opening the panel page first.
	adoptOrphans(ctrl, store)

	switch cmd {
	case "client-list":
		listClients(store)
	case "client-set":
		opts := awgctl.UpdateOptions{
			QuotaBytes: uint64(max0(*quotaGB)) * bytesPerGB,
			SpeedMbit:  max0(*speedMbit),
		}
		if d := max0(*expiresDays); d > 0 {
			opts.ExpiresIn = time.Duration(d) * 24 * time.Hour
		}
		if err := ctrl.UpdateClient(name, opts); err != nil {
			fail(err)
		}
		reconcileShaper(store, *iface)
	case "client-enable":
		if err := ctrl.EnableClient(name); err != nil {
			fail(err)
		}
		reconcileShaper(store, *iface)
	case "client-disable":
		if err := ctrl.DisableClient(name); err != nil {
			fail(err)
		}
		reconcileShaper(store, *iface)
	default:
		fail(fmt.Errorf("unknown subcommand %q", cmd))
	}
}

// listClients prints every client's limit metadata as a JSON array.
func listClients(store *lifecycle.Store) {
	now := time.Now()
	out := make([]clientLimit, 0)
	for _, r := range store.List() {
		cl := clientLimit{
			Name:      r.Name,
			Disabled:  r.Disabled,
			QuotaGB:   int(r.QuotaBytes / bytesPerGB),
			UsedBytes: r.UsedBytes,
			SpeedMbit: r.SpeedMbit,
		}
		if r.ExpiresAt != nil {
			if d := int(r.ExpiresAt.Sub(now).Hours() / 24); d > 0 {
				cl.ExpiresDays = d
			}
		}
		out = append(out, cl)
	}
	enc := json.NewEncoder(os.Stdout)
	if err := enc.Encode(out); err != nil {
		fail(err)
	}
}

// reconcileShaper re-applies tc bandwidth caps from the store so a speed change
// takes effect immediately, instead of waiting for the daemon's next cycle.
func reconcileShaper(store *lifecycle.Store, iface string) {
	var limits []shaper.Limit
	for _, r := range store.List() {
		if !r.Disabled && r.SpeedMbit > 0 {
			limits = append(limits, shaper.Limit{Octet: r.Octet, Mbit: r.SpeedMbit})
		}
	}
	// Best-effort: tc needs root and the HTB module. The daemon also reconciles
	// every cycle, so a transient failure here self-heals.
	_ = shaper.Apply(shaper.Plan(iface, subnetBase, limits))
}

// adoptOrphans mirrors the panel's adoption: every peer in the server config
// that has no store record yet gets a minimal one, so it can carry limits.
func adoptOrphans(ctrl awgctl.FileController, store *lifecycle.Store) {
	clients, err := ctrl.ServerClients()
	if err != nil {
		return
	}
	for _, sc := range clients {
		if _, ok := store.Get(sc.Name); !ok {
			_ = store.Put(lifecycle.Record{
				Name:      sc.Name,
				PubKey:    sc.PubKey,
				Octet:     sc.Octet,
				PeerBlock: sc.Block,
				CreatedAt: time.Now(),
			})
		}
	}
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "awg-panel:", err)
	os.Exit(1)
}
