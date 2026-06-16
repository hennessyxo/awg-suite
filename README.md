# AmneziaWG Installer

> One-command installer & client manager for a self-hosted **AmneziaWG** VPN on Ubuntu/Debian.
> Установщик и менеджер клиентов для собственного VPN на **AmneziaWG** — в одну команду.

![shell](https://img.shields.io/badge/shell-bash-1f425f)
![platform](https://img.shields.io/badge/platform-Ubuntu%20%7C%20Debian-orange)
![license](https://img.shields.io/badge/license-MIT-green)

AmneziaWG is a fork of **WireGuard** with built-in traffic obfuscation. Regular
WireGuard is fast but easy for DPI systems to fingerprint and block; AmneziaWG
disguises the handshake and packet headers so the traffic looks like noise.
This script removes the manual work: it installs AmneziaWG, sets up NAT and the
firewall, generates randomized obfuscation parameters, and manages clients with
ready-to-scan QR codes — no Linux networking knowledge required.

AmneziaWG — это форк **WireGuard** со встроенной обфускацией трафика. Обычный
WireGuard быстрый, но его легко вычисляют и блокируют по DPI; AmneziaWG
маскирует рукопожатие и заголовки пакетов, чтобы трафик выглядел «шумом».
Скрипт берёт всю ручную работу на себя: установка, NAT, firewall, случайные
параметры обфускации и управление клиентами с QR-кодами — без знаний Linux.

---

## ✨ Features / Возможности

- 🚀 **Установка в одну команду** — от чистого сервера до рабочего VPN за пару минут
- 🛡️ **Обфускация трафика** — случайные `Jc/Jmin/Jmax/S1/S2/H1–H4`, синхронные у сервера и клиента
- 📶 **Мобильный пресет** — `MTU 1280` + `Jc=3` для 4G/LTE (Yota, МТС, Билайн, Мегафон, Tele2); лечит «подключено, но нет интернета» на сотовых
- 📱 **QR-коды** — импорт конфига в мобильное приложение сканированием
- 👥 **Меню управления** — добавить / удалить клиента, список, статус, QR
- 📊 **TUI-монитор** ([`monitor/`](monitor/)) — живой терминальный дашборд: трафик, скорости, handshake, online
- 🔁 **Hot-reload** — клиенты добавляются без разрыва текущих соединений (`awg syncconf`)
- 🌐 **IPv4 + IPv6**, автоопределение публичного IP и сетевого интерфейса
- ♻️ **Идемпотентность** — повторный запуск открывает меню, а не ломает установку

---

## ⚡ Quick start / Быстрый старт

На сервере (Ubuntu 22.04+/24.04 или Debian 12+), под root:

```bash
git clone https://github.com/hennessyxo/amneziawg-installer.git
cd amneziawg-installer
sudo bash amneziawg-install.sh
```

Или одной строкой:

```bash
curl -fsSL https://raw.githubusercontent.com/hennessyxo/amneziawg-installer/main/amneziawg-install.sh -o awg-install.sh
sudo bash awg-install.sh
```

> ⚠️ Перед `curl | bash` всегда читайте, что запускаете. Здесь скрипт скачивается
> в файл, чтобы вы могли его просмотреть.

---

## 📋 Step-by-step / Пошагово

1. **Арендуйте VPS** с Ubuntu/Debian (подойдёт самый дешёвый — 1 vCPU / 512 МБ).
2. **Подключитесь по SSH:** `ssh root@ВАШ_IP`.
3. **Запустите скрипт** (см. Quick start). Он спросит:
   - публичный IP/домен (определяется автоматически),
   - внешний интерфейс (определяется автоматически),
   - UDP-порт (по умолчанию случайный),
   - DNS для клиентов (по умолчанию Cloudflare),
   - имя первого клиента,
   - **мобильный пресет** — включи, если будешь пользоваться с сотовой сети.
4. **Отсканируйте QR-код** в приложении **AmneziaWG** / **Amnezia VPN**
   (Android, iOS, Windows, macOS, Linux) — или импортируйте `.conf` файл.
5. **Подключайтесь.** Готово.

Чтобы добавить ещё клиента — просто запустите скрипт снова:

```bash
sudo bash amneziawg-install.sh
```

Появится меню управления.

---

## 📱 Client apps / Клиентские приложения

| Платформа | Приложение |
|-----------|-----------|
| Android / iOS | **AmneziaVPN** (Google Play / App Store) или **AmneziaWG** |
| Windows / macOS / Linux | **AmneziaVPN** desktop |
| CLI (другой Linux) | `amneziawg-tools` → `awg-quick up <config>` |

Импортируйте сгенерированный `awg0-client-<name>.conf` или отсканируйте QR.

---

## 🔧 What the script configures / Что настраивает скрипт

| Шаг | Действие |
|-----|----------|
| Пакеты | Подключает Amnezia PPA, ставит `amneziawg` (DKMS-модуль + tools), `qrencode` |
| Ядро | Включает `net.ipv4.ip_forward` и IPv6 forwarding (`/etc/sysctl.d/99-amneziawg.conf`) |
| Интерфейс | `awg0` с адресами `10.66.66.1/24` и `fd42:42:42::1/64` |
| Firewall/NAT | Правила `iptables`/`ip6tables` MASQUERADE и FORWARD в `PostUp/PostDown` |
| Служба | Включает и запускает `awg-quick@awg0` (автозагрузка) |
| Клиенты | Ключи, preshared-key, выдача IP, конфиг + QR |

Все настройки сохраняются в `/etc/amnezia/amneziawg/params` (права `600`).

---

## 🔐 Security notes / Безопасность

- Приватные ключи и `params` хранятся с правами `600`, под `umask 077`.
- Каждый клиент получает уникальный **preshared key** (дополнительный слой к ключам).
- Параметры обфускации генерируются случайно при установке — не используйте
  «дефолтные из интернета», иначе обфускация теряет смысл.
- Скрипт открывает в firewall только выбранный UDP-порт.
- Для боевого использования смените SSH на ключи и закройте лишние порты.

---

## 📊 Monitoring / Мониторинг

В каталоге [`monitor/`](monitor/) — `awg-monitor`, живой терминальный дашборд на Go:
трафик и скорость по каждому клиенту, время последнего handshake, online-статус,
спарклайны нагрузки. Подтягивает имена клиентов прямо из конфига установщика.

```bash
cd monitor && go build -o awg-monitor .
sudo ./awg-monitor            # мониторинг awg0
./awg-monitor --demo          # демо без сервера
```

Подробнее — в [`monitor/README.md`](monitor/README.md).

---

## 🗺️ Roadmap

- [x] Установщик + меню управления (CLI)
- [x] Мобильные пресеты (MTU/обфускация под 4G/LTE)
- [x] TUI-монитор на Go (с тестами и CI)
- [x] Пункт меню «мониторинг» + готовые бинарники (GitHub Releases, без Go на сервере)
- [ ] **Веб-панель** (фаза 3): красивый UI, управление клиентами, лимиты трафика,
      авто-отключение при превышении квоты, ограничение скорости (`tc`), авторизация

---

## 🩺 Troubleshooting

Частые проблемы и решения — в [`docs/TROUBLESHOOTING.md`](docs/TROUBLESHOOTING.md).

Быстрая диагностика:

```bash
systemctl status awg-quick@awg0
journalctl -u awg-quick@awg0 -n 50
awg show awg0
```

---

## ⚠️ Disclaimer

Проект создан для **законного** использования: приватности, доступа к собственным
ресурсам и обучения сетевым технологиям. Соблюдайте законы своей юрисдикции.
Автор не несёт ответственности за неправомерное использование.

This project is for **lawful** use — privacy, accessing your own resources, and
learning networking. Follow the laws of your jurisdiction.

---

## 📄 License

MIT © contributors. See [LICENSE](LICENSE).

Логика установки и управления клиентами адаптирована из проверенного
[`angristan/wireguard-install`](https://github.com/angristan/wireguard-install)
и портирована на AmneziaWG с поддержкой обфускации.
