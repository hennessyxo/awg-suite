# AmneziaWG Installer

**English** · [Русский](README.ru.md)

> One-command installer, client manager, monitor and web panel for a self-hosted
> **AmneziaWG** VPN on Ubuntu/Debian.

![shell](https://img.shields.io/badge/shell-bash-1f425f)
![go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go)
![platform](https://img.shields.io/badge/platform-Ubuntu%20%7C%20Debian-orange)
![ci](https://github.com/hennessyxo/amneziawg-installer/actions/workflows/ci.yml/badge.svg)
![license](https://img.shields.io/badge/license-MIT-green)

AmneziaWG is a fork of **WireGuard** with built-in traffic obfuscation. Plain
WireGuard is fast but easy for DPI systems to fingerprint and block; AmneziaWG
disguises the handshake and packet headers so the traffic looks like noise. This
project removes all the manual work — install, NAT/firewall, randomized
obfuscation, client management with QR codes — no Linux knowledge required.

## ✨ What's inside

| Component | What it does |
|-----------|--------------|
| `amneziawg-install.sh` | One-command install + interactive menu (add/revoke clients, QR, status) |
| **Mobile preset** | `MTU 1280` + `Jc=3` for 4G/LTE carriers — fixes "connected but no internet" on cellular |
| `cmd/awg-monitor` | Live terminal dashboard (Go/Bubble Tea): traffic, rates, handshake, online status |
| `cmd/awg-panel` | Web panel (Go + htmx): auth, HTTPS, live dashboard, client management, **quotas, expiry, speed limits** |
| `cmd/awg-deploy` | Cross-platform SSH installer — a **Windows `.exe`** (+ macOS/Linux) that sets everything up remotely |

## 📥 Installation

You need a cheap VPS (Ubuntu 22.04+/24.04 or Debian 12+) — that's the server the
VPN runs on. Pick **one** of the two ways to set it up.

### Option A — from your own computer (easiest, no Linux needed)

1. Download the `awg-deploy` binary for **your computer** from
   [Releases](https://github.com/hennessyxo/amneziawg-installer/releases/latest):

   | Your computer | File to download |
   |---------------|------------------|
   | Windows | `awg-deploy-windows-amd64.exe` |
   | macOS — Apple Silicon (M1–M5) | `awg-deploy-darwin-arm64` |
   | macOS — Intel | `awg-deploy-darwin-amd64` |
   | Linux — x86_64 | `awg-deploy-linux-amd64` |
   | Linux — ARM | `awg-deploy-linux-arm64` |

2. Run it from a terminal, pointing at your server:

   **Windows** (PowerShell / Windows Terminal):
   ```powershell
   .\awg-deploy-windows-amd64.exe install root@YOUR_SERVER_IP --preset mobile
   ```

   **macOS / Linux:**
   ```bash
   chmod +x ./awg-deploy-darwin-arm64
   xattr -dr com.apple.quarantine ./awg-deploy-darwin-arm64   # macOS only: clear Gatekeeper
   ./awg-deploy-darwin-arm64 install root@YOUR_SERVER_IP --preset mobile
   ```

   It asks for your SSH password (or use `--identity ~/.ssh/id_ed25519`), installs
   AmneziaWG over SSH, saves the client `.conf` and prints a QR code. The installer
   script is **embedded in the binary** — nothing to download on the server.

   > macOS may say "cannot verify the developer" (the binary is unsigned). Either
   > run the `xattr` command above, or right-click the file in Finder → **Open**.

3. Manage it the same way:
   ```bash
   awg-deploy add-client root@YOUR_SERVER_IP laptop   # new client + QR
   awg-deploy monitor   root@YOUR_SERVER_IP           # live dashboard in your terminal
   ```

See [`docs/DEPLOY.md`](docs/DEPLOY.md) for all flags.

### Option B — directly on the server

SSH into the server and run, as root:

```bash
git clone https://github.com/hennessyxo/amneziawg-installer.git
cd amneziawg-installer
sudo bash amneziawg-install.sh        # add --lang en for English UI
```

Answer a few questions (public IP, port, DNS, first client, mobile preset) and
scan the QR in the **AmneziaVPN** app. Re-run the script anytime for the
management menu: add/remove clients, **monitoring** (option 6) and the
**web panel** (option 7).

### Automation / non-interactive

```bash
AWG_SERVER_IP=YOUR_SERVER_IP AWG_PORT=51820 AWG_PRESET=mobile AWG_CLIENT=phone \
  sudo -E bash amneziawg-install.sh --yes
sudo bash amneziawg-install.sh --add-client laptop    # one client, then exit
```

Vars: `AWG_SERVER_IP`, `AWG_SERVER_NIC`, `AWG_PORT`, `AWG_DNS1/2`, `AWG_CLIENT`,
`AWG_PRESET` (`default|mobile`), `AWG_LANG` (`ru|en`).

## 📊 Monitoring

`awg-monitor` ([`cmd/awg-monitor`](cmd/awg-monitor)) — a live terminal dashboard:
per-client traffic and rates, handshake age, online status, throughput
sparklines. Install it from the menu (option 6) or build it:

```bash
go build -o awg-monitor ./cmd/awg-monitor && sudo ./awg-monitor
```

See [`docs/MONITOR.md`](docs/MONITOR.md).

## 🖥️ Web panel

`awg-panel` ([`cmd/awg-panel`](cmd/awg-panel)) — a browser dashboard (Go + htmx):
password auth (bcrypt + sessions, HTTPS), live client traffic, add/remove
clients with QR, plus **traffic quotas, time-based expiry and per-client speed
limits**. Install from the menu (option 7); it sets a password, generates a TLS
cert and a systemd service on `https://<ip>:8443`. EN/RU toggle in the UI.

See [`docs/PANEL.md`](docs/PANEL.md).

### Client lifecycle (quotas / expiry / speed)

When adding a client you can set a **traffic quota (GB)**, an **expiry (days)**
and a **speed limit (Mbit/s)**. A background enforcer accounts traffic and:

- **expired** or **over quota** → the client is **disabled** (kept; re-enable any time);
- **speed limit** → the client is throttled with `tc` (HTB on download, ingress
  policer on upload) instead of being cut off.

## 🗺️ Roadmap

- [x] Installer + management menu + mobile presets
- [x] TUI monitor (Go, tested, CI)
- [x] Web panel (auth/HTTPS/htmx)
- [x] Quotas + time-based expiry (auto-disable, re-enable)
- [x] Per-client speed limiting (`tc`)
- [x] Cross-platform SSH installer (Windows `.exe`)
- [x] EN/RU localization (docs, installer UI, web panel)

## 🔐 Security notes

- Private keys, params and the panel password hash are stored `600` under `umask 077`.
- Each client gets a unique preshared key; obfuscation parameters are randomized per install.
- The web panel uses bcrypt + sessions (HttpOnly cookie) + CSRF, and HTTPS; it runs
  as root (needs `awg`) — don't expose it publicly without need (SSH tunnel / trusted network).
- The SSH deploy tool verifies host keys via `known_hosts` (TOFU for new hosts,
  hard-fail on a changed key).

## 🩺 Troubleshooting

See [`docs/TROUBLESHOOTING.md`](docs/TROUBLESHOOTING.md). Quick checks:

```bash
systemctl status awg-quick@awg0
journalctl -u awg-quick@awg0 -n 50
awg show awg0
```

## ⚠️ Disclaimer

For **lawful** use — privacy, accessing your own resources, and learning
networking. Follow the laws of your jurisdiction.

## 📄 License

MIT © contributors. See [LICENSE](LICENSE). Install logic adapted from the
battle-tested [`angristan/wireguard-install`](https://github.com/angristan/wireguard-install)
and ported to AmneziaWG with obfuscation support.
