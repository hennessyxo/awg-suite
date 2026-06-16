# AmneziaWG Installer

[English](README.md) · **Русский**

> Установщик, менеджер клиентов, монитор и веб-панель для собственного VPN на
> **AmneziaWG** (Ubuntu/Debian) — в одну команду.

![shell](https://img.shields.io/badge/shell-bash-1f425f)
![go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go)
![platform](https://img.shields.io/badge/platform-Ubuntu%20%7C%20Debian-orange)
![ci](https://github.com/hennessyxo/amneziawg-installer/actions/workflows/ci.yml/badge.svg)
![license](https://img.shields.io/badge/license-MIT-green)

AmneziaWG — форк **WireGuard** со встроенной обфускацией трафика. Обычный
WireGuard быстрый, но его легко вычисляют и блокируют по DPI; AmneziaWG маскирует
рукопожатие и заголовки пакетов, чтобы трафик выглядел «шумом». Проект берёт всю
ручную работу на себя — установка, NAT/firewall, случайная обфускация, управление
клиентами с QR — без знаний Linux.

## ✨ Что внутри

| Компонент | Что делает |
|-----------|------------|
| `amneziawg-install.sh` | Установка в одну команду + меню (добавить/удалить клиентов, QR, статус) |
| **Мобильный пресет** | `MTU 1280` + `Jc=3` для 4G/LTE — лечит «подключено, но нет интернета» на сотовых |
| `cmd/awg-monitor` | Живой терминальный дашборд (Go): трафик, скорости, handshake, online |
| `cmd/awg-panel` | Веб-панель (Go + htmx): авторизация, HTTPS, живой статус, управление, **квоты, срок, лимит скорости** |
| `cmd/awg-deploy` | Кросс-платформенный SSH-установщик — **`.exe` для Windows** (+ macOS/Linux), ставит всё по SSH |

## 📥 Установка

Нужен дешёвый VPS (Ubuntu 22.04+/24.04 или Debian 12+) — это сервер, на котором
крутится VPN. Выбери **один** из двух способов.

### Способ A — со своего компьютера (проще всего, без Linux)

1. Скачай бинарник `awg-deploy` **под свой компьютер** из
   [Releases](https://github.com/hennessyxo/amneziawg-installer/releases/latest):

   | Твой компьютер | Какой файл качать |
   |----------------|-------------------|
   | Windows | `awg-deploy-windows-amd64.exe` |
   | macOS — Apple Silicon (M1–M5) | `awg-deploy-darwin-arm64` |
   | macOS — Intel | `awg-deploy-darwin-amd64` |
   | Linux — x86_64 | `awg-deploy-linux-amd64` |
   | Linux — ARM | `awg-deploy-linux-arm64` |

2. **Просто запусти без аргументов** — мастер сам спросит адрес сервера и пароль,
   проверит, установлен ли AmneziaWG (предложит поставить), сохранит первый
   конфиг + QR и откроет меню управления (добавить / удалить / список клиентов,
   мониторинг, полное меню сервера, удаление):

   **Windows** — двойной клик по `.exe`, либо в PowerShell:
   ```powershell
   .\awg-deploy-windows-amd64.exe
   ```

   **macOS / Linux:**
   ```bash
   chmod +x ./awg-deploy-darwin-arm64
   xattr -dr com.apple.quarantine ./awg-deploy-darwin-arm64   # только macOS: снять карантин Gatekeeper
   ./awg-deploy-darwin-arm64
   ```

   > macOS может сказать «не удаётся проверить разработчика» (бинарник неподписанный).
   > Выполни команду `xattr` выше, либо правый клик по файлу в Finder → **Открыть**.

3. (Для продвинутых) Те же действия есть и отдельными командами — для скриптов:
   ```bash
   awg-deploy install      root@IP_СЕРВЕРА --preset mobile
   awg-deploy add-client   root@IP_СЕРВЕРА laptop
   awg-deploy list         root@IP_СЕРВЕРА
   awg-deploy remove-client root@IP_СЕРВЕРА laptop
   awg-deploy monitor      root@IP_СЕРВЕРА
   awg-deploy uninstall    root@IP_СЕРВЕРА
   ```

Все флаги — в [`docs/DEPLOY.md`](docs/DEPLOY.md).

### Способ B — прямо на сервере

Зайди на сервер по SSH и под root:

```bash
git clone https://github.com/hennessyxo/amneziawg-installer.git
cd amneziawg-installer
sudo bash amneziawg-install.sh        # --lang en для английского интерфейса
```

Ответь на несколько вопросов (публичный IP, порт, DNS, первый клиент, мобильный
пресет) и отсканируй QR в приложении **AmneziaVPN**. Запусти скрипт снова в любой
момент — откроется меню управления: добавить/удалить клиентов, **мониторинг**
(пункт 6) и **веб-панель** (пункт 7).

### Автоматизация / без вопросов

```bash
AWG_SERVER_IP=IP_СЕРВЕРА AWG_PORT=51820 AWG_PRESET=mobile AWG_CLIENT=phone \
  sudo -E bash amneziawg-install.sh --yes
sudo bash amneziawg-install.sh --add-client laptop    # один клиент и выход
```

Переменные: `AWG_SERVER_IP`, `AWG_SERVER_NIC`, `AWG_PORT`, `AWG_DNS1/2`,
`AWG_CLIENT`, `AWG_PRESET` (`default|mobile`), `AWG_LANG` (`ru|en`).

## 📊 Мониторинг

`awg-monitor` ([`cmd/awg-monitor`](cmd/awg-monitor)) — живой терминальный дашборд:
трафик и скорость по клиентам, время handshake, статус online, спарклайны.
Ставится из меню (пункт 6) или собирается:

```bash
go build -o awg-monitor ./cmd/awg-monitor && sudo ./awg-monitor
```

Подробнее — [`docs/MONITOR.md`](docs/MONITOR.md).

## 🖥️ Веб-панель

`awg-panel` ([`cmd/awg-panel`](cmd/awg-panel)) — панель в браузере (Go + htmx):
вход по паролю (bcrypt + сессии, HTTPS), живой трафик, добавление/удаление
клиентов с QR, плюс **квоты трафика, срок действия и лимит скорости на клиента**.
Ставится из меню (пункт 7): задаёт пароль, генерирует TLS-сертификат и
systemd-службу на `https://<ip>:8443`. В интерфейсе есть переключатель EN/RU.

Подробнее — [`docs/PANEL.md`](docs/PANEL.md).

### Жизненный цикл клиента (квоты / срок / скорость)

При создании клиента можно задать **квоту трафика (ГБ)**, **срок (дней)** и
**лимит скорости (Мбит/с)**. Фоновый enforcer считает трафик и:

- **истёк срок** или **превышена квота** → клиент **отключается** (сохраняется,
  можно включить обратно);
- **лимит скорости** → клиент режется через `tc` (HTB на отдачу, ingress-police
  на приём), а не отключается.

## 🗺️ Роадмап

- [x] Установщик + меню управления + мобильные пресеты
- [x] TUI-монитор (Go, тесты, CI)
- [x] Веб-панель (авторизация/HTTPS/htmx)
- [x] Квоты + срок действия (авто-отключение, возобновление)
- [x] Ограничение скорости на клиента (`tc`)
- [x] Кросс-платформенный SSH-установщик (`.exe` для Windows)
- [x] Локализация EN/RU (доки, интерфейс установщика, веб-панель)

## 🔐 Безопасность

- Приватные ключи, `params` и хеш пароля панели хранятся с правами `600`, под `umask 077`.
- У каждого клиента уникальный preshared-key; параметры обфускации случайны при установке.
- Веб-панель: bcrypt + сессии (HttpOnly-кука) + CSRF, HTTPS; работает под root
  (нужно для `awg`) — не открывай её в интернет без необходимости (SSH-туннель / доверенная сеть).
- SSH-инструмент проверяет host key по `known_hosts` (TOFU для новых, отказ при смене ключа).

## 🩺 Решение проблем

См. [`docs/TROUBLESHOOTING.md`](docs/TROUBLESHOOTING.md). Быстрая диагностика:

```bash
systemctl status awg-quick@awg0
journalctl -u awg-quick@awg0 -n 50
awg show awg0
```

## ⚠️ Дисклеймер

Только для **законного** использования: приватность, доступ к своим ресурсам,
обучение сетям. Соблюдай законы своей юрисдикции.

## 📄 Лицензия

MIT. См. [LICENSE](LICENSE). Логика установки адаптирована из проверенного
[`angristan/wireguard-install`](https://github.com/angristan/wireguard-install)
и портирована на AmneziaWG с обфускацией.
