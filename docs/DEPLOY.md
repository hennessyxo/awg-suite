# awg-deploy

> Cross-platform SSH deploy tool — install & manage AmneziaWG on a remote server
> from your own machine (Windows `.exe`, macOS, Linux). One binary, nothing to
> pre-install on the server.
> Кросс-платформенный инструмент: ставит и управляет AmneziaWG на сервере по SSH.

![go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go)
![platform](https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux-orange)

`awg-deploy` embeds the installer script and pipes it to the server over SSH,
runs it non-interactively, then pulls back the client config, saves it as a
`.conf`, and opens a scannable QR image. No need to SSH in by hand or know any Linux.

## Download

Grab the binary for your OS from [Releases](https://github.com/hennessyxo/amneziawg-installer/releases):

| Your computer / Твой компьютер | File / Файл |
|--------------------------------|-------------|
| Windows | `awg-deploy-windows-amd64.exe` |
| macOS — Apple Silicon (M1–M5) | `awg-deploy-darwin-arm64` |
| macOS — Intel | `awg-deploy-darwin-amd64` |
| Linux — x86_64 | `awg-deploy-linux-amd64` |
| Linux — ARM | `awg-deploy-linux-arm64` |

> `darwin` = macOS. Качай под **свой** компьютер, а не под сервер — сервер
> настраивается автоматически по SSH.

### macOS: Gatekeeper

Бинарник неподписанный, поэтому при первом запуске macOS его заблокирует. Сними
карантин один раз:

```bash
chmod +x ./awg-deploy-darwin-arm64
xattr -dr com.apple.quarantine ./awg-deploy-darwin-arm64
```

Либо: правый клик по файлу в Finder → **Открыть** → **Открыть**. (То же в
`System Settings → Privacy & Security → Open Anyway`.)

## Wizard (самый простой путь)

Запусти бинарник **без аргументов** (на Windows — двойной клик по `.exe`):

```bash
./awg-deploy-darwin-arm64
```

Мастер спросит адрес сервера и пароль, проверит, установлен ли AmneziaWG
(предложит поставить), сохранит первый конфиг + QR и откроет меню управления.
Команды ниже — для тех, кто хочет вызывать действия напрямую (скрипты/автоматизация).

## Usage / Использование

```bash
# Установить VPN на сервер (спросит SSH-пароль, если не указан ключ):
awg-deploy install root@YOUR_SERVER_IP --preset mobile --client phone

# С SSH-ключом и нестандартным SSH-портом:
awg-deploy install root@YOUR_SERVER_IP:2222 --identity ~/.ssh/id_ed25519

# Добавить ещё клиента (печатает его конфиг + QR):
awg-deploy add-client root@YOUR_SERVER_IP laptop

# Список клиентов:
awg-deploy list root@YOUR_SERVER_IP

# Удалить клиента:
awg-deploy remove-client root@YOUR_SERVER_IP laptop

# Интерактивное меню сервера прямо в твоём терминале (по SSH):
awg-deploy menu root@YOUR_SERVER_IP

# Живой мониторинг сервера прямо из своего терминала:
awg-deploy monitor root@YOUR_SERVER_IP

# Полностью удалить AmneziaWG с сервера (спросит подтверждение):
awg-deploy uninstall root@YOUR_SERVER_IP
```

> Повторный `install` на уже настроенном сервере ничего не ломает — он
> распознаёт это и подсказывает команды управления (`add-client`, `list`,
> `remove-client`, `monitor`). Интерактивное меню есть на самом сервере:
> `sudo bash amneziawg-install.sh`.

On Windows just run the `.exe` from a terminal (PowerShell/Windows Terminal):

```powershell
.\awg-deploy-windows-amd64.exe install root@YOUR_SERVER_IP --preset mobile
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
   └─ saves <name>.conf + <name>.png (QR) locally and opens the image
```

`monitor` runs `awg show <iface> dump` over SSH on each tick and renders the same
TUI as [`awg-monitor`](MONITOR.md), reusing `internal/awg` and `internal/ui`.

## License

MIT — see [../LICENSE](../LICENSE).
