package deploy

import (
	"strings"
	"testing"
)

func TestParseTarget(t *testing.T) {
	cases := []struct {
		in         string
		user, host string
		port       int
		wantErr    bool
	}{
		{"203.0.113.7", "root", "203.0.113.7", 22, false},
		{"alice@example.com", "alice", "example.com", 22, false},
		{"alice@example.com:2222", "alice", "example.com", 2222, false},
		{"root@10.0.0.1:22", "root", "10.0.0.1", 22, false},
		{"", "", "", 0, true},
		{"user@", "", "", 0, true},
		{"host:0", "", "", 0, true},
		{"host:99999", "", "", 0, true},
		{"host:abc", "", "", 0, true},
	}
	for _, c := range cases {
		got, err := ParseTarget(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseTarget(%q) expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseTarget(%q) error: %v", c.in, err)
			continue
		}
		if got.User != c.user || got.Host != c.host || got.Port != c.port {
			t.Errorf("ParseTarget(%q) = %+v, want {%s %s %d}", c.in, got, c.user, c.host, c.port)
		}
	}
}

func TestAddr(t *testing.T) {
	if got := (Target{Host: "h", Port: 2222}).Addr(); got != "h:2222" {
		t.Errorf("Addr = %q, want h:2222", got)
	}
}

func TestSudo(t *testing.T) {
	if Sudo("root") != "" {
		t.Error("root should need no sudo prefix")
	}
	if Sudo("alice") != "sudo " {
		t.Error("non-root should get a sudo prefix")
	}
}

func TestInstallCommand(t *testing.T) {
	cmd := InstallCommand("sudo ", map[string]string{"AWG_SERVER_IP": "203.0.113.7", "AWG_PRESET": "mobile"})
	// Always non-interactive, always captures config, env is sorted+quoted.
	for _, want := range []string{
		"sudo env ",
		"AWG_PRESET='mobile'",
		"AWG_PRINT_CONFIG='1'",
		"AWG_SERVER_IP='203.0.113.7'",
		"bash -s -- --yes",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("InstallCommand missing %q in %q", want, cmd)
		}
	}
	// Deterministic ordering (sorted keys).
	if strings.Index(cmd, "AWG_PRESET") > strings.Index(cmd, "AWG_PRINT_CONFIG") {
		t.Errorf("env args not sorted: %q", cmd)
	}
	// root → no sudo prefix.
	if got := InstallCommand("", nil); !strings.HasPrefix(got, "env ") {
		t.Errorf("root install command should start with env, got %q", got)
	}
}

func TestAddClientCommand_QuotesName(t *testing.T) {
	cmd := AddClientCommand("sudo ", "weird name; rm -rf /")
	if !strings.Contains(cmd, "--add-client 'weird name; rm -rf /'") {
		t.Errorf("AddClientCommand should single-quote the name: %q", cmd)
	}
}

func TestRemoveAndListCommands(t *testing.T) {
	if got := RemoveClientCommand("sudo ", "laptop"); got != "sudo bash -s -- --remove-client 'laptop'" {
		t.Errorf("RemoveClientCommand = %q", got)
	}
	if got := ListClientsCommand(""); got != "bash -s -- --list" {
		t.Errorf("ListClientsCommand = %q", got)
	}
	if got := UninstallCommand("sudo "); got != "sudo env AWG_CONFIRM=yes bash -s -- --uninstall" {
		t.Errorf("UninstallCommand = %q", got)
	}
}

func TestAlreadyInstalled(t *testing.T) {
	if !AlreadyInstalled("...\nAWG_ALREADY_INSTALLED\n...") {
		t.Error("should detect the already-installed marker")
	}
	if AlreadyInstalled("fresh install output") {
		t.Error("false positive on fresh output")
	}
}

func TestCheckInstalled(t *testing.T) {
	cmd := CheckInstalledCommand("sudo ")
	if !strings.Contains(cmd, "test -f /etc/amnezia/amneziawg/params") || !strings.Contains(cmd, "AWG_INSTALLED") {
		t.Errorf("CheckInstalledCommand = %q", cmd)
	}
	if !IsInstalled("foo\nAWG_INSTALLED\n") || IsInstalled("nope") {
		t.Error("IsInstalled mismatch")
	}
}

func TestShellQuote_EscapesQuotes(t *testing.T) {
	if got := shellQuote("a'b"); got != `'a'\''b'` {
		t.Errorf("shellQuote(a'b) = %q", got)
	}
}

func TestExtractConfig(t *testing.T) {
	out := "noise\n" + beginMarker + "\n[Interface]\nPrivateKey = X\n" + endMarker + "\ntail"
	conf, err := ExtractConfig(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(conf, "[Interface]") || !strings.Contains(conf, "PrivateKey = X") {
		t.Errorf("unexpected extracted config: %q", conf)
	}
	if _, err := ExtractConfig("no markers here"); err == nil {
		t.Error("expected error when markers absent")
	}
}

func TestParseServerInfo(t *testing.T) {
	out := "PORT=51820\nVER=amneziawg-tools v1.0\nUP=93784\nPEERS=3\n"
	info := ParseServerInfo(out)
	if info.Port != "51820" {
		t.Errorf("Port = %q, want 51820", info.Port)
	}
	if info.Version != "amneziawg-tools v1.0" {
		t.Errorf("Version = %q", info.Version)
	}
	if info.UptimeSeconds != 93784 {
		t.Errorf("UptimeSeconds = %d, want 93784", info.UptimeSeconds)
	}
	if info.Peers != 3 {
		t.Errorf("Peers = %d, want 3", info.Peers)
	}
}

func TestParseServerInfo_Empty(t *testing.T) {
	info := ParseServerInfo("PORT=\nVER=\nUP=\nPEERS=\n")
	if info.Port != "" || info.Version != "" || info.UptimeSeconds != 0 || info.Peers != 0 {
		t.Errorf("expected zero values, got %+v", info)
	}
}

func TestChangePanelPasswordCommand_NoPasswordInArgs(t *testing.T) {
	cmd := ChangePanelPasswordCommand("sudo ")
	if !strings.Contains(cmd, "awg-panel hash") || !strings.Contains(cmd, "read -r __pw") {
		t.Errorf("command missing stdin-hash pipeline: %s", cmd)
	}
	if !strings.HasPrefix(cmd, "sudo bash -c ") {
		t.Errorf("unexpected prefix: %s", cmd)
	}
}
