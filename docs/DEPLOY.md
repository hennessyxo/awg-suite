# awg-deploy

> Cross-platform SSH deploy tool — install & manage AmneziaWG on a remote server
> from your own machine (Windows `.exe`, macOS, Linux). One binary, nothing to
> pre-install on the server.
> Кросс-платформенный инструмент: ставит и управляет AmneziaWG на сервере по SSH.

![go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go)
![platform](https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux-orange)

`awg-deploy` embeds the installer script and pipes it to the server over SSH,
runs it non-interactively, then pulls back the client config and prints a QR code
in your terminal. No need to SSH in by hand or know any Linux.

## Download

Grab the binary for your OS from [Releases](https://github.com/hennessyxo/amneziawg-installer/releases):

- Windows: `awg-deploy-windows-amd64.exe`
- macOS: `awg-deploy-darwin-arm64` (Apple Silicon) / `-amd64` (Intel)
- Linux: `awg-deploy-linux-amd64` / `-arm64`

## Usage / Использование

```bash
# Установить VPN на сервер (спросит SSH-пароль, если не указан ключ):
awg-deploy install root@203.0.113.7 --preset mobile --client phone

# С SSH-ключом и нестандартным SSH-портом:
awg-deploy install root@203.0.113.7:2222 --identity ~/.ssh/id_ed25519

# Добавить ещё клиента (печатает его конфиг + QR):
awg-deploy add-client root@203.0.113.7 laptop

# Живой мониторинг сервера прямо из своего терминала:
awg-deploy monitor root@203.0.113.7
```

On Windows just run the `.exe` from a terminal (PowerShell/Windows Terminal):

```powershell
.\awg-deploy-windows-amd64.exe install root@203.0.113.7 --preset mobile
```

### install flags

| Flag | Назначение |
|------|-----------|
| `--preset` | `default` или `mobile` (MTU 1280, Jc=3 для 4G/LTE) |
| `--port` | UDP-порт AmneziaWG (по умолчанию случайный) |
| `--client` | имя первого клиента |
| `--server-ip` | публичный IP/домен для клиентов (по умолчанию автоопределение) |
| `--dns1`, `--dns2` | DNS клиентов |
| `--out` | куда сохранить `.conf` локально |
| `--identity` | SSH-приватный ключ (иначе спросит пароль) |
| `--known-hosts` | путь к known_hosts |
| `--accept-new` | довериться новому хосту без вопроса |

## Security

- Ключ хоста проверяется по `known_hosts`. Неизвестный хост → запрос подтверждения
  (TOFU) с показом отпечатка SHA256; **изменившийся** ключ → отказ (возможный MITM).
- Пароль читается скрытым вводом и не сохраняется.
- Скрипт установщика **встроен в бинарник** (`embed`), отдельно ничего качать не нужно.

## How it works

```
awg-deploy ──SSH──> server
   │  pipes embedded amneziawg-install.sh to `bash -s -- --yes`
   │  passes settings via AWG_* env vars (non-interactive mode)
   │  captures the fenced client config from stdout
   └─ saves <name>.conf locally + renders a QR in the terminal
```

`monitor` runs `awg show <iface> dump` over SSH on each tick and renders the same
TUI as [`awg-monitor`](MONITOR.md), reusing `internal/awg` and `internal/ui`.

## License

MIT — see [../LICENSE](../LICENSE).
