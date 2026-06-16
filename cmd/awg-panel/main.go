// Command awg-panel is the web management panel for a self-hosted AmneziaWG VPN.
// It serves a session-authenticated dashboard (live traffic + client management)
// built on the same `awg` parsing core as awg-monitor.
//
// Usage:
//
//	awg-panel --password-hash-file /etc/amnezia/amneziawg/panel.hash \
//	          --tls-cert /etc/.../cert.pem --tls-key /etc/.../key.pem
//
//	echo 'mysecret' | awg-panel hash    # print a bcrypt hash for the installer
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/hennessyxo/amneziawg-installer/internal/auth"
	"github.com/hennessyxo/amneziawg-installer/internal/awgctl"
	"github.com/hennessyxo/amneziawg-installer/internal/lifecycle"
	"github.com/hennessyxo/amneziawg-installer/internal/server"
)

const (
	sessionTTL      = 12 * time.Hour
	enforceInterval = 30 * time.Second // how often quotas/expiry are reconciled
)

func main() {
	// Subcommand: `awg-panel hash` reads a password from stdin and prints its
	// bcrypt hash, so the installer can write a hash file without storing the
	// plaintext password anywhere.
	if len(os.Args) > 1 && os.Args[1] == "hash" {
		makeHash()
		return
	}

	listen := flag.String("listen", ":8443", "address to listen on")
	iface := flag.String("iface", "awg0", "AmneziaWG interface")
	conf := flag.String("conf", "/etc/amnezia/amneziawg/awg0.conf", "server config path")
	params := flag.String("params", "/etc/amnezia/amneziawg/params", "installer params path")
	clientDir := flag.String("client-dir", "/etc/amnezia/amneziawg/clients", "where panel-generated client configs are stored")
	storePath := flag.String("store", "/etc/amnezia/amneziawg/clients.json", "lifecycle metadata store (quotas/expiry)")
	hashFile := flag.String("password-hash-file", "/etc/amnezia/amneziawg/panel.hash", "file containing the admin bcrypt hash")
	password := flag.String("password", "", "admin password (dev only; prefer --password-hash-file)")
	tlsCert := flag.String("tls-cert", "", "TLS certificate (enables HTTPS)")
	tlsKey := flag.String("tls-key", "", "TLS private key (enables HTTPS)")
	flag.Parse()

	pwHash, err := loadHash(*password, *hashFile)
	if err != nil {
		log.Fatalf("awg-panel: %v", err)
	}

	store, err := lifecycle.Open(*storePath)
	if err != nil {
		log.Fatalf("awg-panel: %v", err)
	}

	ctrl := awgctl.FileController{
		Iface:     *iface,
		ConfPath:  *conf,
		ParamPath: *params,
		ClientDir: *clientDir,
		Store:     store,
	}

	useTLS := *tlsCert != "" && *tlsKey != ""
	srv, err := server.New(ctrl, auth.NewStore(sessionTTL), store, pwHash, *iface, useTLS)
	if err != nil {
		log.Fatalf("awg-panel: %v", err)
	}
	srv.ReconcileShaper() // re-apply bandwidth caps after a restart
	srv.StartEnforcer(context.Background(), enforceInterval)

	httpSrv := &http.Server{
		Addr:              *listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	if useTLS {
		log.Printf("awg-panel: listening on https://%s (iface %s)", *listen, *iface)
		log.Fatal(httpSrv.ListenAndServeTLS(*tlsCert, *tlsKey))
	}
	log.Printf("awg-panel: listening on http://%s (iface %s)", *listen, *iface)
	log.Printf("awg-panel: WARNING — no TLS configured; cookies are not Secure. Use --tls-cert/--tls-key or put a reverse proxy with HTTPS in front.")
	log.Fatal(httpSrv.ListenAndServe())
}

// loadHash resolves the admin password hash from --password (hashed in memory)
// or from the hash file.
func loadHash(password, hashFile string) (string, error) {
	if password != "" {
		return auth.HashPassword(password)
	}
	data, err := os.ReadFile(hashFile)
	if err != nil {
		return "", fmt.Errorf("no admin password configured: set --password or create %s (echo PW | awg-panel hash > file): %w", hashFile, err)
	}
	hash := strings.TrimSpace(string(data))
	if hash == "" {
		return "", fmt.Errorf("hash file %s is empty", hashFile)
	}
	return hash, nil
}

func makeHash() {
	sc := bufio.NewScanner(os.Stdin)
	if !sc.Scan() {
		fmt.Fprintln(os.Stderr, "awg-panel hash: read password from stdin")
		os.Exit(1)
	}
	pw := strings.TrimSpace(sc.Text())
	if pw == "" {
		fmt.Fprintln(os.Stderr, "awg-panel hash: empty password")
		os.Exit(1)
	}
	hash, err := auth.HashPassword(pw)
	if err != nil {
		fmt.Fprintln(os.Stderr, "awg-panel hash:", err)
		os.Exit(1)
	}
	fmt.Println(hash)
}
