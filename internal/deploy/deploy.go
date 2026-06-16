// Package deploy contains the logic for the cross-platform SSH deploy tool
// (cmd/awg-deploy): parsing the target, building the remote commands that drive
// the non-interactive installer, and extracting the generated client config.
//
// The command/parsing logic is pure and unit-tested; the SSH transport lives in
// ssh.go and is exercised against a real server.
package deploy

import (
	"fmt"
	"sort"
	"strings"
)

const (
	beginMarker = "-----BEGIN_AWG_CONF-----"
	endMarker   = "-----END_AWG_CONF-----"
)

// Target identifies the server to deploy to.
type Target struct {
	User string
	Host string
	Port int
}

// Addr returns the host:port dial address.
func (t Target) Addr() string { return fmt.Sprintf("%s:%d", t.Host, t.Port) }

// ParseTarget parses "host", "user@host", or "user@host:port".
// User defaults to root, port to 22.
func ParseTarget(s string) (Target, error) {
	t := Target{User: "root", Port: 22}
	s = strings.TrimSpace(s)
	if s == "" {
		return t, fmt.Errorf("empty target")
	}
	if at := strings.LastIndex(s, "@"); at >= 0 {
		t.User = s[:at]
		s = s[at+1:]
		if t.User == "" {
			return t, fmt.Errorf("empty user in target")
		}
	}
	if colon := strings.LastIndex(s, ":"); colon >= 0 {
		host, portStr := s[:colon], s[colon+1:]
		p, err := parsePort(portStr)
		if err != nil {
			return t, err
		}
		t.Host, t.Port = host, p
	} else {
		t.Host = s
	}
	if t.Host == "" {
		return t, fmt.Errorf("empty host in target")
	}
	return t, nil
}

func parsePort(s string) (int, error) {
	p := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("invalid port %q", s)
		}
		p = p*10 + int(r-'0')
	}
	if p < 1 || p > 65535 {
		return 0, fmt.Errorf("port out of range: %q", s)
	}
	return p, nil
}

// Sudo returns the command prefix for privilege escalation: empty for root,
// "sudo " otherwise.
func Sudo(user string) string {
	if user == "root" {
		return ""
	}
	return "sudo "
}

// InstallCommand builds the remote shell command that runs the installer
// (piped to bash via stdin) non-interactively with the given AWG_* env vars.
func InstallCommand(sudo string, env map[string]string) string {
	full := map[string]string{"AWG_PRINT_CONFIG": "1"}
	for k, v := range env {
		full[k] = v
	}
	return sudo + "env " + envArgs(full) + " bash -s -- --yes"
}

// AddClientCommand builds the remote command to create a single client.
func AddClientCommand(sudo, name string) string {
	return sudo + "env AWG_PRINT_CONFIG=1 bash -s -- --add-client " + shellQuote(name)
}

// MonitorDumpCommand returns the command that dumps live interface state.
func MonitorDumpCommand(sudo, iface string) string {
	return sudo + "awg show " + shellQuote(iface) + " dump"
}

// ReadConfCommand returns the command that prints the server config (for client
// name resolution in monitor mode).
func ReadConfCommand(sudo, iface string) string {
	return sudo + "cat /etc/amnezia/amneziawg/" + iface + ".conf"
}

// ExtractConfig pulls the fenced client config out of installer output.
func ExtractConfig(output string) (string, error) {
	start := strings.Index(output, beginMarker)
	end := strings.Index(output, endMarker)
	if start < 0 || end < 0 || end < start {
		return "", fmt.Errorf("client config markers not found in output")
	}
	conf := output[start+len(beginMarker) : end]
	return strings.TrimSpace(conf) + "\n", nil
}

// envArgs renders a deterministic "K=quoted ..." string for `env`.
func envArgs(env map[string]string) string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+shellQuote(env[k]))
	}
	return strings.Join(parts, " ")
}

// shellQuote single-quotes a value for safe use in a remote shell command.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
