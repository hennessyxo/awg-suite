#!/usr/bin/env bash
#
# amneziawg-install.sh
# One-command installer & client manager for a self-hosted AmneziaWG VPN.
#
# AmneziaWG is a fork of WireGuard with traffic obfuscation that helps bypass
# DPI-based blocking. This script installs it on Ubuntu/Debian, configures NAT
# and firewall rules, generates obfuscation parameters, and manages clients
# (add / revoke / show QR) — without requiring the user to know the internals.
#
# Tested target systems: Ubuntu 22.04/24.04, Debian 12/13.
# Run as root:  sudo bash amneziawg-install.sh
#
# License: MIT
# Approach adapted from the battle-tested angristan/wireguard-install, ported
# to AmneziaWG (awg/awg-quick) with obfuscation support.

set -euo pipefail

# ---------------------------------------------------------------------------
# Constants & paths
# ---------------------------------------------------------------------------
readonly AWG_DIR="/etc/amnezia/amneziawg"
readonly AWG_NIC="awg0"
readonly PARAMS_FILE="${AWG_DIR}/params"
readonly SERVER_CONF="${AWG_DIR}/${AWG_NIC}.conf"
readonly CLIENT_OUT_DIR="${HOME}"
readonly MONITOR_BIN="/usr/local/bin/awg-monitor"
readonly PANEL_BIN="/usr/local/bin/awg-panel"
readonly PANEL_PORT="8443"
readonly PANEL_HASH="${AWG_DIR}/panel.hash"
readonly PANEL_CERT="${AWG_DIR}/panel-cert.pem"
readonly PANEL_KEY="${AWG_DIR}/panel-key.pem"
readonly PANEL_CLIENT_DIR="${AWG_DIR}/clients"
readonly REPO_SLUG="hennessyxo/amneziawg-installer"

# Colors (disabled automatically when output is not a terminal)
if [[ -t 1 ]]; then
	RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
	BLUE='\033[0;34m'; CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'
else
	RED=''; GREEN=''; YELLOW=''; BLUE=''; CYAN=''; BOLD=''; NC=''
fi

msg()  { echo -e "${BLUE}==>${NC} $*"; }
ok()   { echo -e "${GREEN}✓${NC} $*"; }
warn() { echo -e "${YELLOW}!${NC} $*"; }
err()  { echo -e "${RED}✗${NC} $*" >&2; }

# Runtime flags (overridable via CLI args; see parseArgs).
NONINTERACTIVE=0
ADD_CLIENT=""
REMOVE_CLIENT=""
LIST_CLIENTS=0
UNINSTALL=0
INSTALL_PANEL=0
REMOVE_PANEL=0
LANG_CODE="ru"

# detectLang picks the UI language: --lang/AWG_LANG, else $LANG, else Russian.
detectLang() {
	local l="${AWG_LANG:-}"
	if [[ -z "${l}" ]]; then
		case "${LANG:-}" in en*) l="en" ;; *) l="ru" ;; esac
	fi
	case "${l}" in en) LANG_CODE="en" ;; *) LANG_CODE="ru" ;; esac
}

# chooseLang asks the language interactively (so it's not at the mercy of the
# server's $LANG, which arrives inconsistently over SSH).
chooseLang() {
	echo
	echo "Выбери язык / Choose language:"
	echo "  1) Русский"
	echo "  2) English"
	read -rp "[1]: " l
	case "${l}" in 2) LANG_CODE="en" ;; *) LANG_CODE="ru" ;; esac
}

# t prints a localized UI string by key (interactive surface: menu/prompts).
t() {
	if [[ "${LANG_CODE}" == "en" ]]; then
		case "$1" in
			menu_title)   echo "AmneziaWG — management" ;;
			m_add)        echo "Add client" ;;
			m_del)        echo "Remove client" ;;
			m_list)       echo "List clients" ;;
			m_qr)         echo "Show client QR" ;;
			m_status)     echo "Server status" ;;
			m_monitor)    echo "Monitoring (install / run awg-monitor)" ;;
			m_panel)      echo "Web panel (install / run awg-panel)" ;;
			m_uninstall)  echo "Remove AmneziaWG completely" ;;
			m_exit)       echo "Exit" ;;
			choose)       echo "Choice" ;;
			confirm)      echo "Continue? [Y/n]: " ;;
			cancelled)    echo "Cancelled." ;;
			done_title)   echo "Done! AmneziaWG server is up." ;;
			run_again)    echo "Run the script again to add clients, enable monitoring (6) or the web panel (7)." ;;
			run_monitor)  echo "Run monitoring now? [Y/n]: " ;;
			mon_usage)    echo "How to use awg-monitor" ;;
			panel_addr)   echo "Address" ;;
			press_enter)  echo "Press Enter to return to the menu... " ;;
			panel_inst_q) echo "The web panel is already installed. Remove it? [y/N]: " ;;
			panel_removed) echo "Web panel removed." ;;
			mon_action_q) echo "Run monitoring (Enter) or remove it (type d)? " ;;
			mon_removed)  echo "awg-monitor removed." ;;
			panel_offer)  echo "Install the web panel for easy browser management? [Y/n]: " ;;
			one_profile)  echo "One profile = one device. Make a separate client for each phone/PC, or connections will clash." ;;
			qr_note)      echo "Open the AmneziaWG app and scan the QR (or import the .conf file). iOS — App Store: https://apps.apple.com/app/amneziawg/id6478942365 ; Android/Windows: https://amnezia.org/downloads" ;;
			first_conf)   echo "First client config:" ;;
			panel_at)     echo "Web panel:" ;;
			add_more)     echo "Add more clients from the menu or the web panel." ;;
			p_deps)       echo "Installing dependencies..." ;;
			p_repo)       echo "Adding the AmneziaWG repository..." ;;
			p_module)     echo "Building the AmneziaWG kernel module (DKMS, ~2-5 min — this is normal, please wait)..." ;;
			invalid)      echo "Invalid choice." ;;
			*)            echo "$1" ;;
		esac
	else
		case "$1" in
			menu_title)   echo "AmneziaWG — управление" ;;
			m_add)        echo "Добавить клиента" ;;
			m_del)        echo "Удалить клиента" ;;
			m_list)       echo "Список клиентов" ;;
			m_qr)         echo "Показать QR-код клиента" ;;
			m_status)     echo "Статус сервера" ;;
			m_monitor)    echo "Мониторинг (установить / запустить awg-monitor)" ;;
			m_panel)      echo "Веб-панель (установить / запустить awg-panel)" ;;
			m_uninstall)  echo "Удалить AmneziaWG полностью" ;;
			m_exit)       echo "Выход" ;;
			choose)       echo "Выбор" ;;
			confirm)      echo "Продолжить? [Y/n]: " ;;
			cancelled)    echo "Отменено." ;;
			done_title)   echo "Готово! Сервер AmneziaWG развёрнут." ;;
			run_again)    echo "Запусти скрипт снова: добавить клиентов, включить мониторинг (6) или веб-панель (7)." ;;
			run_monitor)  echo "Запустить мониторинг сейчас? [Y/n]: " ;;
			mon_usage)    echo "Как пользоваться awg-monitor" ;;
			panel_addr)   echo "Адрес" ;;
			press_enter)  echo "Нажми Enter, чтобы вернуться в меню... " ;;
			panel_inst_q) echo "Веб-панель уже установлена. Удалить её? [y/N]: " ;;
			panel_removed) echo "Веб-панель удалена." ;;
			mon_action_q) echo "Запустить мониторинг (Enter) или удалить (введи d)? " ;;
			mon_removed)  echo "awg-monitor удалён." ;;
			panel_offer)  echo "Поставить веб-панель для удобного управления в браузере? [Y/n]: " ;;
			one_profile)  echo "Один профиль — одно устройство. Для каждого телефона/ПК создавай отдельного клиента, иначе соединения конфликтуют." ;;
			qr_note)      echo "Открой приложение AmneziaWG и отсканируй QR (или импортируй файл .conf). iOS — App Store: https://apps.apple.com/app/amneziawg/id6478942365 ; Android/Windows: https://amnezia.org/downloads" ;;
			first_conf)   echo "Конфиг первого клиента:" ;;
			panel_at)     echo "Веб-панель:" ;;
			add_more)     echo "Добавляй клиентов в меню или в веб-панели." ;;
			p_deps)       echo "Устанавливаю зависимости..." ;;
			p_repo)       echo "Подключаю репозиторий AmneziaWG..." ;;
			p_module)     echo "Собираю модуль ядра AmneziaWG (DKMS, ~2–5 мин — это нормально, дождись)..." ;;
			invalid)      echo "Неверный выбор." ;;
			*)            echo "$1" ;;
		esac
	fi
}

# ---------------------------------------------------------------------------
# Pre-flight checks
# ---------------------------------------------------------------------------
checkRoot() {
	if [[ "${EUID}" -ne 0 ]]; then
		err "This script must be run as root. Try: sudo bash $0"
		exit 1
	fi
}

checkVirt() {
	if [[ "$(systemd-detect-virt 2>/dev/null || echo none)" == "openvz" ]]; then
		err "OpenVZ is not supported (no kernel module support)."
		exit 1
	fi
	if [[ "$(systemd-detect-virt 2>/dev/null || echo none)" == "lxc" ]]; then
		warn "LXC detected. The kernel module may be unavailable; userspace fallback is not handled by this script."
	fi
}

checkOS() {
	if [[ ! -e /etc/os-release ]]; then
		err "Cannot detect the operating system (/etc/os-release missing)."
		exit 1
	fi
	# shellcheck disable=SC1091
	source /etc/os-release
	OS="${ID}"
	OS_VERSION="${VERSION_ID:-unknown}"

	case "${OS}" in
		ubuntu) ;;
		debian)
			if [[ "${VERSION_ID%%.*}" -lt 11 ]] 2>/dev/null; then
				warn "Debian ${VERSION_ID} is old; Debian 12+ is recommended."
			fi
			;;
		*)
			err "Unsupported OS: ${PRETTY_NAME:-$OS}. This script targets Ubuntu and Debian."
			exit 1
			;;
	esac
	ok "Detected ${PRETTY_NAME:-$OS $OS_VERSION}"
}

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
randInt() { shuf -i "$1"-"$2" -n1; }

# Large random int in [5, ~1.07e9] built from two 15-bit $RANDOM draws.
# Avoids `shuf -i 5-2147483647` which can be slow/memory-hungry on old coreutils.
randBig() { echo $(( ((RANDOM << 15 | RANDOM) % 1073741819) + 5 )); }

# Generate four distinct header magic values (H1..H4) in the valid range.
generateHeaders() {
	local h1 h2 h3 h4
	h1=$(randBig)
	h2=$(randBig); while [[ "$h2" == "$h1" ]]; do h2=$(randBig); done
	h3=$(randBig); while [[ "$h3" == "$h1" || "$h3" == "$h2" ]]; do h3=$(randBig); done
	h4=$(randBig); while [[ "$h4" == "$h1" || "$h4" == "$h2" || "$h4" == "$h3" ]]; do h4=$(randBig); done
	echo "$h1 $h2 $h3 $h4"
}

# Generate the full obfuscation parameter set, respecting AmneziaWG constraints.
# Uses safe values that work on BOTH cellular (4G/LTE) and broadband/PC: MTU=1280
# (RFC 8200 minimum, passes everywhere) and Jc=3. A lower MTU + gentle junk sizing
# is the usual fix for "connected but no internet" on mobile networks and costs
# nothing meaningful on a desktop link — so there is a single, universal profile.
generateObfuscation() {
	JC=3                              # fixed: Jc>3 often fails first connect on cellular
	JMIN=$(randInt 30 50)
	JMAX=$(( JMIN + 20 + RANDOM % 61 ))   # Jmin + 20..80
	CLIENT_MTU=1280
	S1=$(randInt 15 150)          # init packet junk size
	S2=$(randInt 15 150)          # response packet junk size
	# Constraint: S1 + 56 must not equal S2
	while [[ $((S1 + 56)) -eq "$S2" ]]; do S2=$(randInt 15 150); done
	read -r H1 H2 H3 H4 <<<"$(generateHeaders)"
}

# Detect the public-facing IPv4 address.
detectPublicIP() {
	local ip
	ip=$(ip -4 addr | awk '/inet / && !/127.0.0.1/ {print $2}' | cut -d/ -f1 | head -n1 || true)
	# Prefer a real public IP via external lookup; fall back to the local one.
	if command -v curl >/dev/null 2>&1; then
		local pub
		pub=$(curl -fsS --max-time 5 https://api.ipify.org 2>/dev/null || true)
		[[ -n "${pub}" ]] && ip="${pub}"
	fi
	echo "${ip}"
}

# Detect the default outbound network interface.
detectPublicNIC() {
	ip -4 route ls 2>/dev/null | awk '/default/ {for(i=1;i<=NF;i++) if($i=="dev"){print $(i+1); exit}}'
}

# portInUse reports whether a UDP port is already bound on the server.
portInUse() {
	local p="$1"
	if command -v ss >/dev/null 2>&1; then
		ss -lun 2>/dev/null | awk '{print $5}' | grep -qE "[:.]${p}\$"
	elif command -v netstat >/dev/null 2>&1; then
		netstat -lun 2>/dev/null | awk '{print $4}' | grep -qE "[:.]${p}\$"
	else
		return 1   # can't check → assume free
	fi
}

# pickFreePort returns a random free UDP port in [40000,59999], avoiding any that
# are already bound (so we never collide with another service on the server).
pickFreePort() {
	local p tries=0
	while :; do
		p=$((RANDOM % 20000 + 40000))
		portInUse "${p}" || { echo "${p}"; return 0; }
		tries=$((tries + 1))
		[[ "${tries}" -ge 50 ]] && { echo "${p}"; return 0; }
	done
}

# ---------------------------------------------------------------------------
# Installation
# ---------------------------------------------------------------------------
installQuestions() {
	# Non-interactive mode: take everything from AWG_* env vars (with sane
	# defaults). Used by the SSH deploy tool so install runs without prompts.
	if [[ "${NONINTERACTIVE}" == "1" ]]; then
		SERVER_PUB_IP="${AWG_SERVER_IP:-$(detectPublicIP)}"
		SERVER_PUB_NIC="${AWG_SERVER_NIC:-$(detectPublicNIC)}"
		SERVER_PORT="${AWG_PORT:-$(pickFreePort)}"
		if portInUse "${SERVER_PORT}"; then
			warn "Порт ${SERVER_PORT}/udp уже занят — выбираю свободный."
			SERVER_PORT="$(pickFreePort)"
		fi
		CLIENT_DNS_1="${AWG_DNS1:-1.1.1.1}"
		CLIENT_DNS_2="${AWG_DNS2:-1.0.0.1}"
		FIRST_CLIENT="$(sanitizeName "${AWG_CLIENT:-phone}")"
		PRESET="mobile"
		SERVER_WG_IPV4="10.66.66.1"
		SERVER_WG_IPV6="fd42:42:42::1"
		msg "Неинтерактивная установка: ${SERVER_PUB_IP}:${SERVER_PORT}/udp"
		return 0
	fi

	echo
	echo -e "${BOLD}AmneziaWG VPN — установка / installation${NC}"
	echo "Ответь на несколько вопросов (Enter = значение по умолчанию)."
	echo

	local default_ip default_nic
	default_ip=$(detectPublicIP)
	default_nic=$(detectPublicNIC)

	read -rp "Публичный IP/домен сервера [${default_ip}]: " SERVER_PUB_IP
	SERVER_PUB_IP="${SERVER_PUB_IP:-$default_ip}"

	read -rp "Внешний сетевой интерфейс [${default_nic}]: " SERVER_PUB_NIC
	SERVER_PUB_NIC="${SERVER_PUB_NIC:-$default_nic}"

	local default_port; default_port=$(pickFreePort)
	read -rp "Порт AmneziaWG (UDP) [${default_port}]: " SERVER_PORT
	SERVER_PORT="${SERVER_PORT:-$default_port}"
	if portInUse "${SERVER_PORT}"; then
		warn "Порт ${SERVER_PORT}/udp уже занят на сервере."
		read -rp "Выбрать свободный автоматически? [Y/n]: " pchg
		[[ "${pchg,,}" != "n" ]] && SERVER_PORT="$(pickFreePort)" && msg "Использую порт ${SERVER_PORT}/udp."
	fi

	read -rp "DNS для клиентов [1.1.1.1]: " CLIENT_DNS_1
	CLIENT_DNS_1="${CLIENT_DNS_1:-1.1.1.1}"
	read -rp "Резервный DNS [1.0.0.1]: " CLIENT_DNS_2
	CLIENT_DNS_2="${CLIENT_DNS_2:-1.0.0.1}"

	read -rp "Имя первого клиента [phone]: " FIRST_CLIENT
	FIRST_CLIENT="${FIRST_CLIENT:-phone}"
	FIRST_CLIENT=$(sanitizeName "${FIRST_CLIENT}")

	# Single universal profile (MTU 1280 + Jc=3): works on both mobile and PC.
	PRESET="mobile"

	# Internal VPN subnets
	SERVER_WG_IPV4="10.66.66.1"
	SERVER_WG_IPV6="fd42:42:42::1"

	echo
	msg "Будет установлено:"
	echo "    Endpoint : ${SERVER_PUB_IP}:${SERVER_PORT}/udp"
	echo "    Интерфейс: ${AWG_NIC} (${SERVER_WG_IPV4}/24)"
	echo "    Выход    : ${SERVER_PUB_NIC}"
	echo "    DNS      : ${CLIENT_DNS_1}, ${CLIENT_DNS_2}"
	echo
	read -rp "$(t confirm)" confirm
	if [[ "${confirm,,}" == "n" ]]; then
		err "$(t cancelled)"
		exit 0
	fi
}

addRepoAndInstall() {
	export DEBIAN_FRONTEND=noninteractive

	msg "$(t p_deps)"
	apt-get update -qq
	apt-get install -y -qq software-properties-common python3-launchpadlib \
		gnupg2 curl qrencode iptables "linux-headers-$(uname -r)" >/dev/null 2>&1 || {
		warn "Не все заголовки ядра найдены; продолжаю — DKMS попробует собрать модуль."
	}

	msg "$(t p_repo)"
	if [[ "${OS}" == "ubuntu" ]]; then
		add-apt-repository -y ppa:amnezia/ppa >/dev/null 2>&1
	else
		# Debian: add the Amnezia PPA manually (uses the Ubuntu focal pocket).
		apt-key adv --keyserver keyserver.ubuntu.com --recv-keys 57290828 >/dev/null 2>&1 || \
			warn "Не удалось импортировать ключ через apt-key; проверь вручную, если установка упадёт."
		if ! grep -q "ppa.launchpadcontent.net/amnezia" /etc/apt/sources.list 2>/dev/null; then
			echo "deb https://ppa.launchpadcontent.net/amnezia/ppa/ubuntu focal main" >>/etc/apt/sources.list
			echo "deb-src https://ppa.launchpadcontent.net/amnezia/ppa/ubuntu focal main" >>/etc/apt/sources.list
		fi
	fi
	apt-get update -qq

	# Stream the output of this step: the DKMS kernel-module build takes a few
	# minutes, and a silent prompt looks frozen.
	msg "$(t p_module)"
	if ! apt-get install -y amneziawg; then
		err "Установка пакета amneziawg не удалась. Смотри docs/TROUBLESHOOTING.md"
		exit 1
	fi

	if ! command -v awg >/dev/null 2>&1; then
		err "Инструмент 'awg' не найден после установки. Модуль ядра мог не собраться."
		exit 1
	fi
	ok "AmneziaWG установлен ($(awg --version 2>/dev/null | head -n1 || echo awg))"
}

enableForwarding() {
	msg "Включаю IP forwarding..."
	cat >/etc/sysctl.d/99-amneziawg.conf <<-EOF
		net.ipv4.ip_forward = 1
		net.ipv6.conf.all.forwarding = 1
	EOF
	sysctl --system >/dev/null 2>&1 || sysctl -p /etc/sysctl.d/99-amneziawg.conf >/dev/null 2>&1 || true
	ok "Форвардинг включён"
}

writeServerConfig() {
	msg "Генерирую ключи сервера и параметры обфускации..."
	umask 077
	mkdir -p "${AWG_DIR}"

	SERVER_PRIV_KEY=$(awg genkey)
	SERVER_PUB_KEY=$(echo "${SERVER_PRIV_KEY}" | awg pubkey)
	generateObfuscation

	# Persist all settings so the management menu can reuse them later.
	cat >"${PARAMS_FILE}" <<-EOF
		SERVER_PUB_IP=${SERVER_PUB_IP}
		SERVER_PUB_NIC=${SERVER_PUB_NIC}
		SERVER_WG_NIC=${AWG_NIC}
		SERVER_WG_IPV4=${SERVER_WG_IPV4}
		SERVER_WG_IPV6=${SERVER_WG_IPV6}
		SERVER_PORT=${SERVER_PORT}
		SERVER_PRIV_KEY=${SERVER_PRIV_KEY}
		SERVER_PUB_KEY=${SERVER_PUB_KEY}
		CLIENT_DNS_1=${CLIENT_DNS_1}
		CLIENT_DNS_2=${CLIENT_DNS_2}
		JC=${JC}
		JMIN=${JMIN}
		JMAX=${JMAX}
		S1=${S1}
		S2=${S2}
		H1=${H1}
		H2=${H2}
		H3=${H3}
		H4=${H4}
		PRESET=${PRESET}
		CLIENT_MTU=${CLIENT_MTU}
	EOF

	# Server interface config with NAT/forward rules applied on up/down.
	cat >"${SERVER_CONF}" <<-EOF
		[Interface]
		Address = ${SERVER_WG_IPV4}/24,${SERVER_WG_IPV6}/64
		ListenPort = ${SERVER_PORT}
		PrivateKey = ${SERVER_PRIV_KEY}
		MTU = ${CLIENT_MTU}
		Jc = ${JC}
		Jmin = ${JMIN}
		Jmax = ${JMAX}
		S1 = ${S1}
		S2 = ${S2}
		H1 = ${H1}
		H2 = ${H2}
		H3 = ${H3}
		H4 = ${H4}
		PostUp = iptables -I INPUT -p udp --dport ${SERVER_PORT} -j ACCEPT
		PostUp = iptables -I FORWARD -i ${AWG_NIC} -j ACCEPT
		PostUp = iptables -I FORWARD -o ${AWG_NIC} -j ACCEPT
		PostUp = iptables -t nat -A POSTROUTING -o ${SERVER_PUB_NIC} -j MASQUERADE
		PostUp = ip6tables -I FORWARD -i ${AWG_NIC} -j ACCEPT
		PostUp = ip6tables -t nat -A POSTROUTING -o ${SERVER_PUB_NIC} -j MASQUERADE
		PostDown = iptables -D INPUT -p udp --dport ${SERVER_PORT} -j ACCEPT
		PostDown = iptables -D FORWARD -i ${AWG_NIC} -j ACCEPT
		PostDown = iptables -D FORWARD -o ${AWG_NIC} -j ACCEPT
		PostDown = iptables -t nat -D POSTROUTING -o ${SERVER_PUB_NIC} -j MASQUERADE
		PostDown = ip6tables -D FORWARD -i ${AWG_NIC} -j ACCEPT
		PostDown = ip6tables -t nat -D POSTROUTING -o ${SERVER_PUB_NIC} -j MASQUERADE
	EOF

	chmod 600 "${PARAMS_FILE}" "${SERVER_CONF}"
	ok "Конфигурация сервера записана: ${SERVER_CONF}"
}

startService() {
	msg "Запускаю службу awg-quick@${AWG_NIC}..."
	systemctl enable "awg-quick@${AWG_NIC}" >/dev/null 2>&1 || true
	systemctl start "awg-quick@${AWG_NIC}"
	if systemctl is-active --quiet "awg-quick@${AWG_NIC}"; then
		ok "Служба запущена и добавлена в автозагрузку"
	else
		err "Служба не запустилась. Логи: journalctl -u awg-quick@${AWG_NIC} -n 30"
		exit 1
	fi
}

installAmneziaWG() {
	installQuestions
	addRepoAndInstall
	enableForwarding
	writeServerConfig
	startService
	newClient "${FIRST_CLIENT}"

	# Offer the web panel right away (interactive) so users land in a browser GUI.
	if [[ "${NONINTERACTIVE}" != "1" ]]; then
		echo
		read -rp "$(t panel_offer)" p
		[[ "${p,,}" != "n" ]] && installPanel
	fi

	# Final summary screen.
	echo
	echo -e "${BOLD}$(t done_title)${NC}"
	echo "  • $(t first_conf) ${CLIENT_OUT_DIR}/${AWG_NIC}-client-${FIRST_CLIENT}.conf"
	if [[ -f /etc/systemd/system/awg-panel.service ]]; then
		echo -e "  • $(t panel_at) ${CYAN}https://${SERVER_PUB_IP}:${PANEL_PORT}${NC}"
	fi
	echo "  • $(t add_more)"
}

# ---------------------------------------------------------------------------
# Client management
# ---------------------------------------------------------------------------
sanitizeName() {
	# Keep only safe characters for filenames and config sections.
	echo "$1" | sed 's/[^a-zA-Z0-9_-]/_/g' | cut -c1-32
}

loadParams() {
	if [[ ! -f "${PARAMS_FILE}" ]]; then
		err "Параметры не найдены (${PARAMS_FILE}). Сначала выполни установку."
		exit 1
	fi
	# shellcheck disable=SC1090
	source "${PARAMS_FILE}"
}

# Find the next free host octet in 10.66.66.0/24 (server uses .1).
nextClientIP() {
	local octet
	for octet in $(seq 2 254); do
		if ! grep -q "10.66.66.${octet}/32" "${SERVER_CONF}" 2>/dev/null; then
			echo "${octet}"
			return 0
		fi
	done
	err "Свободные адреса в подсети закончились."
	exit 1
}

newClient() {
	loadParams
	local name="${1:-}"
	if [[ -z "${name}" ]]; then
		read -rp "Имя нового клиента: " name
	fi
	name=$(sanitizeName "${name}")
	if [[ -z "${name}" ]]; then
		err "Пустое имя клиента."
		return 1
	fi
	if grep -q "^# BEGIN_PEER ${name}\$" "${SERVER_CONF}" 2>/dev/null; then
		err "Клиент '${name}' уже существует."
		return 1
	fi

	local octet client_ipv4 client_ipv6 priv pub psk client_file
	octet=$(nextClientIP)
	client_ipv4="10.66.66.${octet}"
	client_ipv6="fd42:42:42::${octet}"

	priv=$(awg genkey)
	pub=$(echo "${priv}" | awg pubkey)
	psk=$(awg genpsk)

	# Append the peer to the server config, fenced with markers for easy removal.
	cat >>"${SERVER_CONF}" <<-EOF

		# BEGIN_PEER ${name}
		[Peer]
		PublicKey = ${pub}
		PresharedKey = ${psk}
		AllowedIPs = ${client_ipv4}/32,${client_ipv6}/128
		# END_PEER ${name}
	EOF

	# Build the client config (obfuscation params MUST match the server).
	# CLIENT_MTU defaults for installs created before the mobile-preset feature.
	local client_mtu="${CLIENT_MTU:-1420}"
	client_file="${CLIENT_OUT_DIR}/${SERVER_WG_NIC}-client-${name}.conf"
	umask 077
	cat >"${client_file}" <<-EOF
		[Interface]
		PrivateKey = ${priv}
		Address = ${client_ipv4}/32,${client_ipv6}/128
		DNS = ${CLIENT_DNS_1},${CLIENT_DNS_2}
		MTU = ${client_mtu}
		Jc = ${JC}
		Jmin = ${JMIN}
		Jmax = ${JMAX}
		S1 = ${S1}
		S2 = ${S2}
		H1 = ${H1}
		H2 = ${H2}
		H3 = ${H3}
		H4 = ${H4}

		[Peer]
		PublicKey = ${SERVER_PUB_KEY}
		PresharedKey = ${psk}
		Endpoint = ${SERVER_PUB_IP}:${SERVER_PORT}
		AllowedIPs = 0.0.0.0/0,::/0
		PersistentKeepalive = 25
	EOF

	# Mirror the config into the panel's client dir so the web panel can serve
	# the download/QR even for clients created from the CLI / installer.
	mkdir -p "${PANEL_CLIENT_DIR}" 2>/dev/null && cp "${client_file}" "${PANEL_CLIENT_DIR}/" 2>/dev/null || true

	# Apply live without dropping existing connections.
	if systemctl is-active --quiet "awg-quick@${SERVER_WG_NIC}"; then
		awg syncconf "${SERVER_WG_NIC}" <(awg-quick strip "${SERVER_WG_NIC}") 2>/dev/null || \
			systemctl restart "awg-quick@${SERVER_WG_NIC}"
	fi

	echo
	ok "Клиент '${name}' создан → ${client_file}"
	echo -e "${CYAN}Отсканируй QR-код в приложении AmneziaWG:${NC}"
	echo
	qrencode -t ANSIUTF8 <"${client_file}" || warn "qrencode недоступен — импортируй файл вручную."
	echo
	warn "$(t qr_note)"
	echo -e "Файл конфигурации: ${BOLD}${client_file}${NC}"
	echo
	warn "$(t one_profile)"

	# For automation (the SSH deploy tool): emit the config fenced so it can be
	# captured over SSH without guessing the file path.
	if [[ "${AWG_PRINT_CONFIG:-0}" == "1" ]]; then
		echo "-----BEGIN_AWG_CONF-----"
		cat "${client_file}"
		echo "-----END_AWG_CONF-----"
	fi
}

listClients() {
	loadParams
	local clients
	clients=$(grep -c "^# BEGIN_PEER" "${SERVER_CONF}" 2>/dev/null || true)
	clients=${clients:-0}
	if [[ "${clients}" -eq 0 ]]; then
		warn "Клиентов пока нет."
		return 0
	fi
	echo -e "${BOLD}Текущие клиенты (${clients}):${NC}"
	grep "^# BEGIN_PEER" "${SERVER_CONF}" | awk '{print "  - " $3}'
}

# removeClientByName deletes a client non-interactively (also used by --remove-client).
removeClientByName() {
	loadParams
	local name
	name=$(sanitizeName "${1:-}")
	if [[ -z "${name}" ]]; then
		err "Не указано имя клиента."
		return 1
	fi
	if ! grep -q "^# BEGIN_PEER ${name}\$" "${SERVER_CONF}" 2>/dev/null; then
		err "Клиент '${name}' не найден."
		return 1
	fi

	# Remove the fenced peer block.
	sed -i "/^# BEGIN_PEER ${name}\$/,/^# END_PEER ${name}\$/d" "${SERVER_CONF}"
	# Drop a leftover blank line if present.
	sed -i '/^$/N;/^\n$/D' "${SERVER_CONF}"
	rm -f "${CLIENT_OUT_DIR}/${SERVER_WG_NIC}-client-${name}.conf"
	rm -f "${PANEL_CLIENT_DIR}/${SERVER_WG_NIC}-client-${name}.conf"

	if systemctl is-active --quiet "awg-quick@${SERVER_WG_NIC}"; then
		awg syncconf "${SERVER_WG_NIC}" <(awg-quick strip "${SERVER_WG_NIC}") 2>/dev/null || \
			systemctl restart "awg-quick@${SERVER_WG_NIC}"
	fi
	ok "Клиент '${name}' удалён."
}

revokeClient() {
	loadParams
	if ! grep -q "^# BEGIN_PEER" "${SERVER_CONF}" 2>/dev/null; then
		warn "Нет клиентов для удаления."
		return 0
	fi
	listClients
	echo
	read -rp "Имя клиента для удаления: " name
	removeClientByName "${name}"
}

showClientQR() {
	loadParams
	listClients
	echo
	read -rp "Имя клиента: " name
	name=$(sanitizeName "${name}")
	local f="${CLIENT_OUT_DIR}/${SERVER_WG_NIC}-client-${name}.conf"
	if [[ ! -f "${f}" ]]; then
		err "Файл конфигурации для '${name}' не найден."
		return 1
	fi
	echo
	qrencode -t ANSIUTF8 <"${f}" || warn "qrencode недоступен — открой файл вручную."
	echo
	echo -e "Путь: ${BOLD}${f}${NC}"
}

# doUninstall performs the teardown without prompting (used by --uninstall).
doUninstall() {
	loadParams
	systemctl stop "awg-quick@${SERVER_WG_NIC}" 2>/dev/null || true
	systemctl disable "awg-quick@${SERVER_WG_NIC}" 2>/dev/null || true

	# Tear down the web panel if it was installed.
	if [[ -f /etc/systemd/system/awg-panel.service ]]; then
		systemctl stop awg-panel 2>/dev/null || true
		systemctl disable awg-panel 2>/dev/null || true
		rm -f /etc/systemd/system/awg-panel.service
		systemctl daemon-reload 2>/dev/null || true
		iptables -D INPUT -p tcp --dport "${PANEL_PORT}" -j ACCEPT 2>/dev/null || true
	fi
	rm -f "${PANEL_BIN}" "${MONITOR_BIN}"

	export DEBIAN_FRONTEND=noninteractive
	apt-get remove --purge -y -qq amneziawg amneziawg-tools amneziawg-dkms >/dev/null 2>&1 || true

	rm -rf "${AWG_DIR}"
	rm -f /etc/sysctl.d/99-amneziawg.conf
	rm -f "${CLIENT_OUT_DIR}/${SERVER_WG_NIC}"-client-*.conf
	ok "AmneziaWG удалён."
}

# uninstall is the interactive menu entry (asks for confirmation).
uninstall() {
	echo
	warn "Это полностью удалит AmneziaWG, конфиги и всех клиентов."
	read -rp "Точно удалить? Введи 'yes' для подтверждения: " confirm
	if [[ "${confirm}" != "yes" ]]; then
		msg "Отменено."
		return 0
	fi
	doUninstall
}

showStatus() {
	loadParams
	echo
	echo -e "${BOLD}Статус сервера${NC}"
	echo "  Endpoint : ${SERVER_PUB_IP}:${SERVER_PORT}/udp"
	echo "  Служба   : $(systemctl is-active "awg-quick@${SERVER_WG_NIC}" 2>/dev/null || echo unknown)"
	echo
	awg show "${SERVER_WG_NIC}" 2>/dev/null || warn "Интерфейс ${SERVER_WG_NIC} не поднят."
}

# ---------------------------------------------------------------------------
# Monitoring (awg-monitor — Go TUI dashboard)
# ---------------------------------------------------------------------------
detectArch() {
	case "$(uname -m)" in
		x86_64 | amd64) echo "amd64" ;;
		aarch64 | arm64) echo "arm64" ;;
		*) echo "" ;;
	esac
}

showMonitorUsage() {
	echo
	echo -e "${BOLD}$(t mon_usage)${NC}"
	echo "  awg-monitor                 — открыть дашборд (интерфейс ${AWG_NIC})"
	echo "  awg-monitor --interval 1s   — обновлять раз в секунду"
	echo "  awg-monitor --demo          — демо-режим без сервера"
	echo "  В дашборде: [r] обновить сейчас, [q] выйти"
	echo
}

runMonitor() {
	if [[ ! -x "${MONITOR_BIN}" ]]; then
		err "awg-monitor не установлен."
		return 1
	fi
	"${MONITOR_BIN}" --iface "${AWG_NIC}" --conf "${SERVER_CONF}"
}

# fetchGoBinary installs a component binary (monitor|panel) to a destination,
# preferring a prebuilt release asset and falling back to building from source.
# $1 = component name (without the awg- prefix), $2 = destination path.
fetchGoBinary() {
	local comp="$1" dest="$2" arch
	arch=$(detectArch)

	if [[ -n "${arch}" ]]; then
		local url="https://github.com/${REPO_SLUG}/releases/latest/download/awg-${comp}-linux-${arch}"
		msg "Скачиваю awg-${comp} (${arch})..."
		if curl -fsSL "${url}" -o "${dest}" 2>/dev/null && [[ -s "${dest}" ]]; then
			chmod +x "${dest}"
			ok "Бинарник установлен: ${dest}"
			return 0
		fi
	fi

	local repo_dir
	repo_dir="$(cd "$(dirname "$0")" && pwd)"
	if [[ -d "${repo_dir}/cmd/awg-${comp}" ]] && command -v go >/dev/null 2>&1; then
		msg "Готовый бинарник недоступен — собираю awg-${comp} из исходников..."
		if (cd "${repo_dir}" && go build -o "${dest}" "./cmd/awg-${comp}") 2>/dev/null; then
			chmod +x "${dest}"
			ok "Собрано из исходников: ${dest}"
			return 0
		fi
	fi

	err "Не удалось установить awg-${comp} автоматически."
	echo "Причина: бинарник не скачался (приватный репозиторий?) и нет Go для сборки."
	echo "Решения:"
	echo "  • сделать репозиторий публичным — тогда бинарник скачается одной командой;"
	echo "  • или установить Go и собрать: go build -o ${dest} ./cmd/awg-${comp}"
	return 1
}

installMonitor() {
	loadParams
	if [[ -x "${MONITOR_BIN}" ]]; then
		showMonitorUsage
		read -rp "$(t mon_action_q)" r
		if [[ "${r,,}" == "d" ]]; then
			rm -f "${MONITOR_BIN}"
			ok "$(t mon_removed)"
			return 0
		fi
		runMonitor
		return 0
	fi
	fetchGoBinary monitor "${MONITOR_BIN}" || return 1
	showMonitorUsage
	read -rp "$(t run_monitor)" r
	[[ "${r,,}" != "n" ]] && runMonitor
}

# ---------------------------------------------------------------------------
# Web panel (awg-panel — Go + htmx)
# ---------------------------------------------------------------------------
removePanel() {
	systemctl stop awg-panel 2>/dev/null || true
	systemctl disable awg-panel 2>/dev/null || true
	rm -f /etc/systemd/system/awg-panel.service
	systemctl daemon-reload 2>/dev/null || true
	iptables -D INPUT -p tcp --dport "${PANEL_PORT}" -j ACCEPT 2>/dev/null || true
	rm -f "${PANEL_BIN}" "${PANEL_HASH}" "${PANEL_CERT}" "${PANEL_KEY}"
	ok "$(t panel_removed)"
}

installPanel() {
	loadParams

	# Already installed → show how to reach it and offer removal.
	if [[ -f /etc/systemd/system/awg-panel.service ]]; then
		showPanelUsage
		# Non-interactive (GUI/automation): just report it's already up.
		if [[ "${INSTALL_PANEL}" == "1" || -n "${AWG_PANEL_PASSWORD:-}" ]]; then
			return 0
		fi
		read -rp "$(t panel_inst_q)" r
		[[ "${r,,}" == "y" ]] && removePanel
		return 0
	fi

	command -v openssl >/dev/null 2>&1 || apt-get install -y -qq openssl >/dev/null 2>&1 || true
	# tc (iproute2) is needed for per-client speed limits; usually already present.
	command -v tc >/dev/null 2>&1 || apt-get install -y -qq iproute2 >/dev/null 2>&1 || true

	fetchGoBinary panel "${PANEL_BIN}" || return 1

	# Admin password → bcrypt hash (plaintext never stored).
	if [[ ! -s "${PANEL_HASH}" ]]; then
		local pw pw2
		# Non-interactive path (GUI/automation): password from env.
		if [[ -n "${AWG_PANEL_PASSWORD:-}" ]]; then
			if [[ "${#AWG_PANEL_PASSWORD}" -lt 8 ]]; then
				err "Пароль панели слишком короткий (минимум 8 символов)."
				return 1
			fi
			pw="${AWG_PANEL_PASSWORD}"
		else
			while :; do
				read -rsp "Придумай пароль администратора панели (мин. 8 символов): " pw; echo
				read -rsp "Повтори пароль: " pw2; echo
				if [[ "${pw}" != "${pw2}" ]]; then
					warn "Пароли не совпадают — попробуй снова."
				elif [[ "${#pw}" -lt 8 ]]; then
					warn "Слишком короткий пароль (минимум 8 символов)."
				else
					break
				fi
			done
		fi
		umask 077
		echo "${pw}" | "${PANEL_BIN}" hash >"${PANEL_HASH}"
		chmod 600 "${PANEL_HASH}"
		ok "Пароль сохранён (bcrypt-хеш): ${PANEL_HASH}"
	fi

	# Self-signed TLS certificate (browser will warn once; traffic is encrypted).
	if [[ ! -s "${PANEL_CERT}" ]]; then
		msg "Генерирую самоподписанный TLS-сертификат..."
		umask 077
		openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes \
			-keyout "${PANEL_KEY}" -out "${PANEL_CERT}" -days 3650 \
			-subj "/CN=${SERVER_PUB_IP}" >/dev/null 2>&1 || {
			err "Не удалось создать сертификат (openssl)."
			return 1
		}
		chmod 600 "${PANEL_KEY}" "${PANEL_CERT}"
		ok "Сертификат создан."
	fi

	writePanelService
	systemctl daemon-reload
	systemctl enable awg-panel >/dev/null 2>&1 || true
	systemctl restart awg-panel
	sleep 1
	if systemctl is-active --quiet awg-panel; then
		ok "Веб-панель запущена."
		showPanelUsage
	else
		err "Панель не запустилась. Логи: journalctl -u awg-panel -n 30"
		return 1
	fi
}

writePanelService() {
	cat >/etc/systemd/system/awg-panel.service <<-EOF
		[Unit]
		Description=AmneziaWG web panel
		After=network-online.target awg-quick@${AWG_NIC}.service
		Wants=network-online.target

		[Service]
		Type=simple
		# Open the panel port idempotently before start.
		ExecStartPre=/bin/bash -c 'iptables -C INPUT -p tcp --dport ${PANEL_PORT} -j ACCEPT 2>/dev/null || iptables -I INPUT -p tcp --dport ${PANEL_PORT} -j ACCEPT'
		ExecStart=${PANEL_BIN} --listen :${PANEL_PORT} --iface ${AWG_NIC} \\
		  --conf ${SERVER_CONF} --params ${PARAMS_FILE} --client-dir ${PANEL_CLIENT_DIR} \\
		  --password-hash-file ${PANEL_HASH} --tls-cert ${PANEL_CERT} --tls-key ${PANEL_KEY}
		Restart=on-failure
		RestartSec=3

		[Install]
		WantedBy=multi-user.target
	EOF
}

showPanelUsage() {
	echo
	echo -e "${BOLD}Веб-панель AmneziaWG${NC}"
	echo -e "  Адрес : ${CYAN}https://${SERVER_PUB_IP}:${PANEL_PORT}${NC}"
	echo "  Логин : пароль, который ты задал"
	echo "  Сертификат самоподписанный — браузер предупредит один раз, это нормально."
	echo "  Управление службой: systemctl {status|restart|stop} awg-panel"
	echo
	warn "Открой порт ${PANEL_PORT}/tcp в фаерволе облака (если он есть)."
}

# ---------------------------------------------------------------------------
# Menu (shown when AmneziaWG is already installed)
# ---------------------------------------------------------------------------
manageMenu() {
	echo
	echo -e "${BOLD}$(t menu_title)${NC}"
	echo "  1) $(t m_add)"
	echo "  2) $(t m_del)"
	echo "  3) $(t m_list)"
	echo "  4) $(t m_qr)"
	echo "  5) $(t m_status)"
	echo "  6) $(t m_monitor)"
	echo "  7) $(t m_panel)"
	echo "  8) $(t m_uninstall)"
	echo "  9) $(t m_exit)"
	echo
	read -rp "$(t choose) [1-9]: " choice
	case "${choice}" in
		1) newClient "" ;;
		2) revokeClient ;;
		3) listClients ;;
		4) showClientQR ;;
		5) showStatus ;;
		6) installMonitor ;;
		7) installPanel ;;
		8) uninstall ;;
		9) exit 0 ;;
		*) err "$(t invalid)" ;;
	esac
}

# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------
parseArgs() {
	while [[ $# -gt 0 ]]; do
		case "$1" in
			-y | --yes) NONINTERACTIVE=1; shift ;;
			--add-client) ADD_CLIENT="${2:-}"; shift 2 ;;
			--remove-client) REMOVE_CLIENT="${2:-}"; shift 2 ;;
			--list) LIST_CLIENTS=1; shift ;;
			--uninstall) UNINSTALL=1; shift ;;
			--install-panel) INSTALL_PANEL=1; shift ;;
			--remove-panel) REMOVE_PANEL=1; shift ;;
			--lang) AWG_LANG="${2:-}"; shift 2 ;;
			-h | --help)
				echo "Usage: $0 [-y|--yes] [--lang en|ru] [--add-client NAME] [--remove-client NAME] [--list]"
				echo "  -y, --yes          non-interactive install (settings from AWG_* env)"
				echo "  --lang en|ru       UI language (default: auto from \$LANG)"
				echo "  --add-client N     create client N and exit (for automation/SSH)"
				echo "  --remove-client N  remove client N and exit"
				echo "  --list             list clients and exit"
				echo "  --uninstall        remove everything (needs AWG_CONFIRM=yes)"
				echo "  --install-panel    install the web panel (password via AWG_PANEL_PASSWORD)"
				echo "  --remove-panel     remove the web panel"
				exit 0
				;;
			*) shift ;;
		esac
	done
}

main() {
	parseArgs "$@"
	detectLang
	checkRoot
	checkVirt
	checkOS

	# Non-interactive actions (used by the SSH deploy tool).
	if [[ "${UNINSTALL}" == "1" ]]; then
		if [[ "${AWG_CONFIRM:-}" != "yes" ]]; then
			err "Опасное действие. Для подтверждения задай AWG_CONFIRM=yes."
			exit 1
		fi
		doUninstall
		exit 0
	fi
	if [[ "${LIST_CLIENTS}" == "1" ]]; then
		listClients
		exit 0
	fi
	if [[ -n "${REMOVE_CLIENT}" ]]; then
		removeClientByName "${REMOVE_CLIENT}"
		exit $?
	fi
	if [[ -n "${ADD_CLIENT}" ]]; then
		newClient "${ADD_CLIENT}"
		exit 0
	fi
	if [[ "${INSTALL_PANEL}" == "1" ]]; then
		installPanel
		exit $?
	fi
	if [[ "${REMOVE_PANEL}" == "1" ]]; then
		removePanel
		exit $?
	fi

	# Interactive sessions: let the user pick the language (unless forced via
	# --lang/AWG_LANG), so it doesn't flip based on the server's $LANG.
	if [[ "${NONINTERACTIVE}" != "1" && -z "${AWG_LANG:-}" ]]; then
		chooseLang
	fi

	# Fresh server → install (interactive prompts, or non-interactive via AWG_* env).
	if [[ ! -f "${PARAMS_FILE}" ]]; then
		installAmneziaWG
	elif [[ "${NONINTERACTIVE}" == "1" ]]; then
		# Already installed and called non-interactively: emit a stable marker.
		echo "AWG_ALREADY_INSTALLED"
		ok "AmneziaWG уже установлен."
		exit 0
	fi

	# Interactive sessions stay in the management menu until the user exits
	# (menu option 9 calls `exit`, which ends the script and this loop).
	if [[ "${NONINTERACTIVE}" != "1" ]]; then
		while :; do
			manageMenu
			echo
			read -rp "$(t press_enter)" _ || exit 0
		done
	fi
}

main "$@"
