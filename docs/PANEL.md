# awg-panel

> Web management panel for a self-hosted **AmneziaWG** VPN — Go + htmx, single binary.
> Веб-панель управления VPN на AmneziaWG.

![go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go)
![ui](https://img.shields.io/badge/UI-htmx-3366cc)

A session-authenticated dashboard for viewing live client traffic and managing
clients in the browser. Built on the same `awg` parsing core as `awg-monitor`.

## Features

- 🔐 **Авторизация**: пароль администратора (bcrypt), сессии в HttpOnly-куках, CSRF на формах
- 🔒 **HTTPS**: работает по TLS (самоподписанный серт ставится автоматически)
- 📊 **Живой дашборд**: онлайн-статус, скорость ↑↓, суммарный трафик по клиентам (htmx-поллинг)
- ➕ **Управление**: добавить / удалить / отключить / включить клиента, скачать `.conf`, QR — без перезапуска VPN
- ⏳ **Квоты и срок действия**: при создании клиента задаёшь лимит трафика (ГБ) и/или срок (дней);
  фоновый enforcer считает трафик (с учётом сброса счётчиков при перезапуске) и сам
  **отключает** истёкших и превысивших квоту клиентов — они сохраняются и их можно
  **включить обратно** (при включении истёкший срок снимается, переполненная квота обнуляется)
- 🐢 **Ограничение скорости**: задаёшь лимит в Мбит/с — фоновый шейпер на `tc`
  режет отдачу (HTB-класс на IP клиента) и приём (ingress-police), вместо отключения
- 📦 **Один бинарник**: HTML/CSS/htmx вшиты через `embed` — нечего деплоить отдельно

## Install / Установка

Через меню установщика (рекомендуется):

```bash
sudo bash amneziawg-install.sh   # → пункт 7 «Веб-панель»
```

Установщик скачает бинарник, спросит пароль администратора, сгенерирует
самоподписанный TLS-сертификат и поднимет systemd-службу на `https://<ip>:8443`.

Вручную из исходников:

```bash
go build -o awg-panel ./cmd/awg-panel
echo 'мой-пароль' | ./awg-panel hash > /etc/amnezia/amneziawg/panel.hash
sudo ./awg-panel \
  --password-hash-file /etc/amnezia/amneziawg/panel.hash \
  --tls-cert cert.pem --tls-key key.pem
```

## Flags

| Flag | Default | Назначение |
|------|---------|-----------|
| `--listen` | `:8443` | адрес прослушивания |
| `--iface` | `awg0` | интерфейс AmneziaWG |
| `--conf` | `/etc/amnezia/amneziawg/awg0.conf` | конфиг сервера |
| `--params` | `/etc/amnezia/amneziawg/params` | параметры установщика |
| `--client-dir` | `/etc/amnezia/amneziawg/clients` | где хранятся конфиги, созданные панелью |
| `--store` | `/etc/amnezia/amneziawg/clients.json` | метаданные жизненного цикла (квоты/срок) |
| `--password-hash-file` | `/etc/amnezia/amneziawg/panel.hash` | bcrypt-хеш пароля админа |
| `--tls-cert` / `--tls-key` | — | включают HTTPS |

`awg-panel hash` читает пароль из stdin и печатает bcrypt-хеш (plaintext нигде не хранится).

## Security notes

- Панель запускается под root (нужно для `awg`/`awg-quick`). Не открывай её в интернет
  без необходимости; лучший вариант — доступ через SSH-туннель или доверенную сеть.
- Куки `HttpOnly` + `SameSite=Lax`, флаг `Secure` ставится при HTTPS.
- На формах — CSRF-токен, привязанный к сессии.
- QR/`.conf` отдаются только для клиентов, созданных самой панелью (приватные ключи
  не хранятся на сервере для клиентов, созданных через CLI — это by design WireGuard).

## Architecture

```
cmd/awg-panel/main.go        # flags, TLS, `hash` subcommand
internal/
├── awgctl/                  # control plane (params, peer add/remove, FileController)
├── auth/                    # bcrypt + in-memory sessions + CSRF
├── lifecycle/               # quota/expiry store, usage accounting, rule engine
├── shaper/                  # tc command planner (per-client bandwidth caps)
├── server/                  # routing, middleware, handlers, rate tracker, enforcer
└── web/                     # embedded templates + static (htmx, CSS)
```

The enforcer (in `server`) reconciles every 30s: accounts traffic into the
`lifecycle` store, then disables over-quota and expired clients. Bandwidth caps
are re-applied via `shaper` on every change and at startup.

Pure logic (`awgctl`, `auth`) and HTTP handlers (against a fake `Controller`) are
unit-tested; run `go test ./...`.

## License

MIT — see [../LICENSE](../LICENSE).
