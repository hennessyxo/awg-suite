// Command awg-deploy installs and manages a self-hosted AmneziaWG VPN on a
// remote server over SSH — a single cross-platform binary (incl. Windows .exe).
//
// Usage:
//
//	awg-deploy install   user@host[:sshport] [flags]
//	awg-deploy add-client user@host[:sshport] <name> [flags]
//	awg-deploy monitor   user@host[:sshport] [flags]
//
// The installer script is embedded, so nothing needs to be present on the
// server beforehand — awg-deploy pipes it over SSH and runs it non-interactively.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/skip2/go-qrcode"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/term"

	amneziawg "github.com/hennessyxo/amneziawg-installer"
	"github.com/hennessyxo/amneziawg-installer/internal/awg"
	"github.com/hennessyxo/amneziawg-installer/internal/deploy"
	"github.com/hennessyxo/amneziawg-installer/internal/ui"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "install":
		err = runInstall(os.Args[2:])
	case "add-client":
		err = runAddClient(os.Args[2:])
	case "monitor":
		err = runMonitor(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "awg-deploy:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`awg-deploy — установка и управление AmneziaWG по SSH

  awg-deploy install    user@host[:port] [--preset mobile] [--port 51820] [--client phone]
  awg-deploy add-client user@host[:port] <name>
  awg-deploy monitor    user@host[:port] [--iface awg0] [--interval 2s]

Аутентификация: --identity <ключ> или пароль (спросит). Общие флаги: --identity,
--known-hosts, --accept-new.
`)
}

// authFlags holds the SSH auth/host-key flags shared by all subcommands.
type authFlags struct {
	identity   *string
	knownHosts *string
	acceptNew  *bool
}

func registerAuthFlags(fs *flag.FlagSet) authFlags {
	return authFlags{
		identity:   fs.String("identity", "", "SSH private key file (else password prompt)"),
		knownHosts: fs.String("known-hosts", defaultKnownHosts(), "known_hosts path"),
		acceptNew:  fs.Bool("accept-new", false, "trust an unknown host key without prompting"),
	}
}

func runInstall(args []string) error {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	af := registerAuthFlags(fs)
	port := fs.String("port", "", "AmneziaWG UDP port (default: random on server)")
	preset := fs.String("preset", "default", "obfuscation preset: default | mobile")
	client := fs.String("client", "phone", "first client name")
	serverIP := fs.String("server-ip", "", "public IP/host clients connect to (default: autodetect)")
	dns1 := fs.String("dns1", "", "client DNS 1 (default 1.1.1.1)")
	dns2 := fs.String("dns2", "", "client DNS 2 (default 1.0.0.1)")
	out := fs.String("out", "", "where to save the client .conf (default <client>.conf)")
	_ = fs.Parse(args)

	t, err := targetArg(fs)
	if err != nil {
		return err
	}
	cl, err := connect(t, af)
	if err != nil {
		return err
	}
	defer cl.Close()

	env := map[string]string{"AWG_PRESET": *preset, "AWG_CLIENT": *client}
	putIf(env, "AWG_PORT", *port)
	putIf(env, "AWG_SERVER_IP", *serverIP)
	putIf(env, "AWG_DNS1", *dns1)
	putIf(env, "AWG_DNS2", *dns2)

	fmt.Printf("→ Устанавливаю AmneziaWG на %s ...\n\n", t.Addr())
	output, err := cl.RunScript(deploy.InstallCommand(deploy.Sudo(t.User), env), amneziawg.InstallerScript, os.Stdout)
	if err != nil {
		return fmt.Errorf("установка не удалась: %w", err)
	}
	return saveAndShow(output, defaultOut(*out, *client))
}

func runAddClient(args []string) error {
	fs := flag.NewFlagSet("add-client", flag.ExitOnError)
	af := registerAuthFlags(fs)
	out := fs.String("out", "", "where to save the client .conf (default <name>.conf)")
	_ = fs.Parse(args)

	rest := fs.Args()
	if len(rest) < 2 {
		return errors.New("usage: awg-deploy add-client user@host <name>")
	}
	t, err := deploy.ParseTarget(rest[0])
	if err != nil {
		return err
	}
	name := rest[1]

	cl, err := connect(t, af)
	if err != nil {
		return err
	}
	defer cl.Close()

	fmt.Printf("→ Создаю клиента %q на %s ...\n\n", name, t.Addr())
	output, err := cl.RunScript(deploy.AddClientCommand(deploy.Sudo(t.User), name), amneziawg.InstallerScript, os.Stdout)
	if err != nil {
		return fmt.Errorf("создание клиента не удалось: %w", err)
	}
	return saveAndShow(output, defaultOut(*out, name))
}

func runMonitor(args []string) error {
	fs := flag.NewFlagSet("monitor", flag.ExitOnError)
	af := registerAuthFlags(fs)
	iface := fs.String("iface", "awg0", "AmneziaWG interface")
	interval := fs.Duration("interval", 2*time.Second, "refresh interval")
	_ = fs.Parse(args)

	t, err := targetArg(fs)
	if err != nil {
		return err
	}
	cl, err := connect(t, af)
	if err != nil {
		return err
	}
	defer cl.Close()

	sudo := deploy.Sudo(t.User)
	names := map[string]string{}
	if confOut, e := cl.Run(deploy.ReadConfCommand(sudo, *iface)); e == nil {
		names = awg.ParseNames(confOut)
	}

	src := sshSource{cl: cl, sudo: sudo, iface: *iface, names: names}
	prog := tea.NewProgram(ui.New(src, *iface, *interval), tea.WithAltScreen())
	_, err = prog.Run()
	return err
}

// sshSource feeds the TUI from a remote `awg show ... dump` over SSH.
type sshSource struct {
	cl    *deploy.Client
	sudo  string
	iface string
	names map[string]string
}

func (s sshSource) Fetch() (awg.Snapshot, error) {
	out, err := s.cl.Run(deploy.MonitorDumpCommand(s.sudo, s.iface))
	if err != nil {
		return awg.Snapshot{}, fmt.Errorf("remote awg show: %w", err)
	}
	snap, err := awg.ParseDump(s.iface, out, time.Now())
	if err == nil {
		awg.ApplyNames(snap.Peers, s.names)
	}
	return snap, err
}

// --- connection helpers ----------------------------------------------------

func targetArg(fs *flag.FlagSet) (deploy.Target, error) {
	if fs.NArg() < 1 {
		return deploy.Target{}, errors.New("missing target: user@host[:port]")
	}
	return deploy.ParseTarget(fs.Arg(0))
}

func connect(t deploy.Target, af authFlags) (*deploy.Client, error) {
	auth, err := authMethods(*af.identity)
	if err != nil {
		return nil, err
	}
	hk, err := hostKeyCallback(*af.knownHosts, *af.acceptNew)
	if err != nil {
		return nil, err
	}
	return deploy.Dial(t, auth, hk, 15*time.Second)
}

func authMethods(identity string) ([]ssh.AuthMethod, error) {
	if identity != "" {
		key, err := os.ReadFile(identity)
		if err != nil {
			return nil, fmt.Errorf("reading key %s: %w", identity, err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("parsing key (passphrase-protected keys aren't supported; use ssh-agent): %w", err)
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
	}
	pw, err := promptPassword("SSH password: ")
	if err != nil {
		return nil, err
	}
	return []ssh.AuthMethod{ssh.Password(pw)}, nil
}

// hostKeyCallback verifies against known_hosts, with trust-on-first-use for
// unknown hosts and a hard failure on a changed key (possible MITM).
func hostKeyCallback(path string, acceptNew bool) (ssh.HostKeyCallback, error) {
	if err := ensureFile(path); err != nil {
		return nil, err
	}
	base, err := knownhosts.New(path)
	if err != nil {
		return nil, fmt.Errorf("loading known_hosts: %w", err)
	}
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		if err := base(hostname, remote, key); err == nil {
			return nil
		} else {
			var ke *knownhosts.KeyError
			if !errors.As(err, &ke) {
				return err
			}
			if len(ke.Want) > 0 {
				return fmt.Errorf("REMOTE HOST KEY CHANGED for %s — possible MITM, refusing to connect", hostname)
			}
		}
		fp := ssh.FingerprintSHA256(key)
		if !acceptNew {
			if !promptYesNo(fmt.Sprintf("Неизвестный хост %s\n  отпечаток: %s\nДоверять и продолжить?", hostname, fp)) {
				return errors.New("host key rejected")
			}
		}
		return appendKnownHost(path, hostname, key)
	}, nil
}

func appendKnownHost(path, hostname string, key ssh.PublicKey) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	line := knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key)
	_, err = f.WriteString(line + "\n")
	return err
}

// --- small utilities -------------------------------------------------------

func saveAndShow(installerOutput, outPath string) error {
	conf, err := deploy.ExtractConfig(installerOutput)
	if err != nil {
		return fmt.Errorf("не нашёл конфиг клиента в выводе: %w", err)
	}
	if err := os.WriteFile(outPath, []byte(conf), 0o600); err != nil {
		return err
	}
	fmt.Printf("\n✓ Конфиг сохранён: %s\n\nОтсканируй QR в приложении AmneziaVPN:\n\n", outPath)
	q, err := qrcode.New(conf, qrcode.Medium)
	if err != nil {
		return err
	}
	fmt.Println(q.ToSmallString(false))
	return nil
}

func putIf(m map[string]string, k, v string) {
	if strings.TrimSpace(v) != "" {
		m[k] = v
	}
}

func defaultOut(out, name string) string {
	if out != "" {
		return out
	}
	return name + ".conf"
}

func defaultKnownHosts() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "known_hosts"
	}
	return filepath.Join(home, ".ssh", "known_hosts")
}

func ensureFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	return f.Close()
}

func promptPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func promptYesNo(prompt string) bool {
	fmt.Print(prompt + " [y/N]: ")
	sc := bufio.NewScanner(os.Stdin)
	if !sc.Scan() {
		return false
	}
	ans := strings.ToLower(strings.TrimSpace(sc.Text()))
	return ans == "y" || ans == "yes"
}
