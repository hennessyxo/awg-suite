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
# Honors the chosen preset (default | mobile). Mobile networks (4G/LTE) often
# have a lower effective MTU and are picky about junk-packet sizing, which is the
# usual cause of "connected but no internet" on cellular — so the mobile preset
# uses MTU=1280 (RFC 8200 minimum, passes everywhere) and Jc=3.
generateObfuscation() {
	if [[ "${PRESET:-default}" == "mobile" ]]; then
		JC=3                            # fixed: Jc>3 often fails first connect on cellular
		JMIN=$(randInt 30 50)
		JMAX=$(( JMIN + 20 + RANDOM % 61 ))   # Jmin + 20..80
		CLIENT_MTU=1280
	else
		JC=$(randInt 4 12)              # junk packet count
		JMIN=$(randInt 40 80)
		JMAX=$(( JMIN + 80 + RANDOM % 121 ))  # Jmin + 80..200
		CLIENT_MTU=1420                 # standard WireGuard MTU
	fi
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

# ---------------------------------------------------------------------------
# Installation
# ---------------------------------------------------------------------------
installQuestions() {
	# Non-interactive mode: take everything from AWG_* env vars (with sane
	# defaults). Used by the SSH deploy tool so install runs without prompts.
	if [[ "${NONINTERACTIVE}" == "1" ]]; then
		SERVER_PUB_IP="${AWG_SERVER_IP:-$(detectPublicIP)}"
		SERVER_PUB_NIC="${AWG_SERVER_NIC:-$(detectPublicNIC)}"
		SERVER_PORT="${AWG_PORT:-$((RANDOM % 20000 + 40000))}"
		CLIENT_DNS_1="${AWG_DNS1:-1.1.1.1}"
		CLIENT_DNS_2="${AWG_DNS2:-1.0.0.1}"
		FIRST_CLIENT="$(sanitizeName "${AWG_CLIENT:-phone}")"
		PRESET="${AWG_PRESET:-default}"
		SERVER_WG_IPV4="10.66.66.1"
		SERVER_WG_IPV6="fd42:42:42::1"
		msg "Неинтерактивная установка: ${SERVER_PUB_IP}:${SERVER_PORT}/udp, пресет ${PRESET}"
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

	local default_port=$((RANDOM % 20000 + 40000))
	read -rp "Порт AmneziaWG (UDP) [${default_port}]: " SERVER_PORT
	SERVER_PORT="${SERVER_PORT:-$default_port}"

	read -rp "DNS для клиентов [1.1.1.1]: " CLIENT_DNS_1
	CLIENT_DNS_1="${CLIENT_DNS_1:-1.1.1.1}"
	read -rp "Резервный DNS [1.0.0.1]: " CLIENT_DNS_2
	CLIENT_DNS_2="${CLIENT_DNS_2:-1.0.0.1}"

	read -rp "Имя первого клиента [phone]: " FIRST_CLIENT
	FIRST_CLIENT="${FIRST_CLIENT:-phone}"
	FIRST_CLIENT=$(sanitizeName "${FIRST_CLIENT}")

	echo
	echo "Будешь подключаться с мобильного интернета (4G/LTE: Yota, МТС, Билайн, Мегафон, Tele2)?"
	echo "Мобильный пресет включает MTU 1280 + щадящую обфускацию (Jc=3) — лечит"
	echo "'подключено, но нет интернета' на сотовых сетях."
	read -rp "Включить мобильный пресет? [y/N]: " mob
	if [[ "${mob,,}" == "y" ]]; then PRESET="mobile"; else PRESET="default"; fi

	# Internal VPN subnets
	SERVER_WG_IPV4="10.66.66.1"
	SERVER_WG_IPV6="fd42:42:42::1"

	echo
	msg "Будет установлено:"
	echo "    Endpoint : ${SERVER_PUB_IP}:${SERVER_PORT}/udp"
	echo "    Интерфейс: ${AWG_NIC} (${SERVER_WG_IPV4}/24)"
	echo "    Выход    : ${SERVER_PUB_NIC}"
	echo "    DNS      : ${CLIENT_DNS_1}, ${CLIENT_DNS_2}"
	echo "    Пресет   : ${PRESET}$([[ "${PRESET}" == "mobile" ]] && echo ' (MTU 1280, Jc=3)')"
	echo
	read -rp "Продолжить? [Y/n]: " confirm
	if [[ "${confirm,,}" == "n" ]]; then
		err "Отменено пользователем."
		exit 0
	fi
}

addRepoAndInstall() {
	msg "Устанавливаю зависимости и AmneziaWG (это может занять пару минут)..."
	export DEBIAN_FRONTEND=noninteractive

	apt-get update -qq
	apt-get install -y -qq software-properties-common python3-launchpadlib \
		gnupg2 curl qrencode iptables "linux-headers-$(uname -r)" >/dev/null 2>&1 || {
		warn "Не все заголовки ядра найдены; продолжаю — DKMS попробует собрать модуль."
	}

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
	apt-get install -y -qq amneziawg >/dev/null 2>&1 || {
		err "Установка пакета amneziawg не удалась. Смотри docs/TROUBLESHOOTING.md"
		exit 1
	}

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
	echo
	ok "Готово! Сервер AmneziaWG развёрнут."
	echo -e "Запусти ${BOLD}sudo bash $0${NC} снова: добавить клиентов, включить мониторинг (6) или веб-панель (7)."
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

	# Apply live without dropping existing connections.
	if systemctl is-active --quiet "awg-quick@${SERVER_WG_NIC}"; then
		awg syncconf "${SERVER_WG_NIC}" <(awg-quick strip "${SERVER_WG_NIC}") 2>/dev/null || \
			systemctl restart "awg-quick@${SERVER_WG_NIC}"
	fi

	echo
	ok "Клиент '${name}' создан → ${client_file}"
	echo -e "${CYAN}Отсканируй QR-код в приложении AmneziaWG / Amnezia VPN:${NC}"
	echo
	qrencode -t ANSIUTF8 <"${client_file}" || warn "qrencode недоступен — импортируй файл вручную."
	echo
	echo -e "Файл конфигурации: ${BOLD}${client_file}${NC}"
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

revokeClient() {
	loadParams
	if ! grep -q "^# BEGIN_PEER" "${SERVER_CONF}" 2>/dev/null; then
		warn "Нет клиентов для удаления."
		return 0
	fi
	listClients
	echo
	read -rp "Имя клиента для удаления: " name
	name=$(sanitizeName "${name}")
	if ! grep -q "^# BEGIN_PEER ${name}\$" "${SERVER_CONF}"; then
		err "Клиент '${name}' не найден."
		return 1
	fi

	# Remove the fenced peer block.
	sed -i "/^# BEGIN_PEER ${name}\$/,/^# END_PEER ${name}\$/d" "${SERVER_CONF}"
	# Drop a leftover blank line if present.
	sed -i '/^$/N;/^\n$/D' "${SERVER_CONF}"
	rm -f "${CLIENT_OUT_DIR}/${SERVER_WG_NIC}-client-${name}.conf"

	if systemctl is-active --quiet "awg-quick@${SERVER_WG_NIC}"; then
		awg syncconf "${SERVER_WG_NIC}" <(awg-quick strip "${SERVER_WG_NIC}") 2>/dev/null || \
			systemctl restart "awg-quick@${SERVER_WG_NIC}"
	fi
	ok "Клиент '${name}' удалён."
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

uninstall() {
	loadParams
	echo
	warn "Это полностью удалит AmneziaWG, конфиги и всех клиентов."
	read -rp "Точно удалить? Введи 'yes' для подтверждения: " confirm
	if [[ "${confirm}" != "yes" ]]; then
		msg "Отменено."
		return 0
	fi
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
	echo -e "${BOLD}Как пользоваться awg-monitor${NC}"
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
	if [[ ! -x "${MONITOR_BIN}" ]]; then
		fetchGoBinary monitor "${MONITOR_BIN}" || return 1
	else
		ok "awg-monitor уже установлен (${MONITOR_BIN})."
	fi
	showMonitorUsage
	read -rp "Запустить мониторинг сейчас? [Y/n]: " r
	[[ "${r,,}" != "n" ]] && runMonitor
}

# ---------------------------------------------------------------------------
# Web panel (awg-panel — Go + htmx)
# ---------------------------------------------------------------------------
installPanel() {
	loadParams
	command -v openssl >/dev/null 2>&1 || apt-get install -y -qq openssl >/dev/null 2>&1 || true
	# tc (iproute2) is needed for per-client speed limits; usually already present.
	command -v tc >/dev/null 2>&1 || apt-get install -y -qq iproute2 >/dev/null 2>&1 || true

	if [[ ! -x "${PANEL_BIN}" ]]; then
		fetchGoBinary panel "${PANEL_BIN}" || return 1
	else
		ok "awg-panel уже установлен (${PANEL_BIN})."
	fi

	# Admin password → bcrypt hash (plaintext never stored).
	if [[ ! -s "${PANEL_HASH}" ]]; then
		local pw pw2
		while :; do
			read -rsp "Придумай пароль администратора панели: " pw; echo
			read -rsp "Повтори пароль: " pw2; echo
			[[ -n "${pw}" && "${pw}" == "${pw2}" ]] && break
			warn "Пароли пусты или не совпадают — попробуй снова."
		done
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
	echo -e "${BOLD}AmneziaWG — управление${NC}"
	echo "  1) Добавить клиента"
	echo "  2) Удалить клиента"
	echo "  3) Список клиентов"
	echo "  4) Показать QR-код клиента"
	echo "  5) Статус сервера"
	echo "  6) Мониторинг (установить / запустить awg-monitor)"
	echo "  7) Веб-панель (установить / запустить awg-panel)"
	echo "  8) Удалить AmneziaWG полностью"
	echo "  9) Выход"
	echo
	read -rp "Выбор [1-9]: " choice
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
		*) err "Неверный выбор." ;;
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
			-h | --help)
				echo "Usage: $0 [-y|--yes] [--add-client NAME]"
				echo "  -y, --yes        неинтерактивная установка (настройки из AWG_* env)"
				echo "  --add-client N   создать клиента N и выйти (для автоматизации/SSH)"
				exit 0
				;;
			*) shift ;;
		esac
	done
}

main() {
	parseArgs "$@"
	checkRoot
	checkVirt
	checkOS

	# Non-interactive client creation (used by the SSH deploy tool).
	if [[ -n "${ADD_CLIENT}" ]]; then
		newClient "${ADD_CLIENT}"
		exit 0
	fi

	if [[ -f "${PARAMS_FILE}" ]]; then
		if [[ "${NONINTERACTIVE}" == "1" ]]; then
			ok "AmneziaWG уже установлен — пропускаю."
			exit 0
		fi
		manageMenu
	else
		installAmneziaWG
	fi
}

main "$@"
