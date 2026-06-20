package server

import "net/http"

const (
	defaultLang = "ru"
	langCookie  = "lang"
)

// langs holds the UI string catalog per language. Keys are referenced from the
// templates as {{.L.key}} and from a few server-side strings.
var langs = map[string]map[string]string{
	"ru": {
		"login_sub":     "Управление self-hosted VPN",
		"login_pw":      "Пароль администратора",
		"login_btn":     "Войти",
		"login_err":     "Неверный пароль",
		"login_locked":  "Слишком много попыток — вход временно заблокирован",
		"logout":        "Выйти",
		"clients":       "Клиенты",
		"nav_clients":   "Клиенты",
		"nav_server":    "Сервер",
		"server_hint":   "Нагрузка сервера и сводный трафик по всем клиентам.",
		"srv_cpu":       "Процессор",
		"srv_ram":       "Память",
		"srv_disk":      "Диск",
		"srv_uptime":    "Аптайм",
		"srv_traffic":   "Трафик клиентов (↓+↑)",
		"srv_alltime":   "За всё время",
		"srv_30d":       "Трафик за 30 дней",
		"srv_peak":      "пик/день:",
		"srv_top":       "Топ по трафику (месяц)",
		"srv_notraffic": "Пока нет данных о трафике.",
		"ph_name":       "имя клиента",
		"ph_days":       "срок, дней",
		"ph_quota":      "квота, ГБ",
		"ph_speed":      "скорость, Мбит/с",
		"add":           "Добавить",
		"add_client":    "Добавить клиента",
		"new_client":    "Новый клиент",
		"f_name":        "Имя клиента",
		"f_days":        "Срок, дней",
		"f_quota":       "Квота, ГБ",
		"f_speed":       "Скорость, Мбит/с",
		"f_limits":      "Лимиты",
		"optional":      "необязательно",
		"online":        "онлайн",
		"stat_dn":       "↓ всего",
		"stat_up":       "↑ всего",
		"stat_updated":  "обновлено",
		"h_client":      "Клиент",
		"h_speed_dn":    "↓ скорость",
		"h_speed_up":    "↑ скорость",
		"h_limit":       "лимит",
		"h_quota":       "квота",
		"h_today":       "сегодня",
		"h_week":        "неделя",
		"h_month":       "месяц",
		"h_expiry":      "срок",
		"h_handshake":   "handshake",
		"disabled":      "отключён",
		"conf":          "конфиг",
		"on":            "вкл",
		"off":           "выкл",
		"confirm_del":   "Удалить клиента",
		"no_clients":    "пока нет клиентов — добавь первого выше",
		"created_msg":   "создан — скачай .conf (надёжно) или отсканируй QR в приложении AmneziaWG",
		"download":      "Скачать .conf",
		"expired":       "истёк",
		"speed_unit":    "Мбит/с",
		"edit":          "изм.",
		"edit_title":    "Изменить клиента",
		"new_name":      "новое имя",
		"save":          "Сохранить",
		"cancel":        "Отмена",
		"hint_limits":   "пусто = без лимита; срок — через сколько дней отключить",
		"one_profile":   "Каждому устройству — свой клиент. Один конфиг на несколько устройств конфликтует.",
		"qr_note":       "Открой приложение AmneziaWG и отсканируй QR (или импортируй файл .conf). iOS — App Store: apps.apple.com/app/amneziawg/id6478942365",
	},
	"en": {
		"login_sub":     "Self-hosted VPN management",
		"login_pw":      "Admin password",
		"login_btn":     "Sign in",
		"login_err":     "Wrong password",
		"login_locked":  "Too many attempts — login temporarily locked",
		"logout":        "Sign out",
		"clients":       "Clients",
		"nav_clients":   "Clients",
		"nav_server":    "Server",
		"server_hint":   "Server load and aggregate traffic across all clients.",
		"srv_cpu":       "CPU",
		"srv_ram":       "Memory",
		"srv_disk":      "Disk",
		"srv_uptime":    "Uptime",
		"srv_traffic":   "Client traffic (↓+↑)",
		"srv_alltime":   "All time",
		"srv_30d":       "Traffic over 30 days",
		"srv_peak":      "peak/day:",
		"srv_top":       "Top by traffic (month)",
		"srv_notraffic": "No traffic data yet.",
		"ph_name":       "client name",
		"ph_days":       "days",
		"ph_quota":      "quota, GB",
		"ph_speed":      "speed, Mbit/s",
		"add":           "Add",
		"add_client":    "Add client",
		"new_client":    "New client",
		"f_name":        "Client name",
		"f_days":        "Expiry, days",
		"f_quota":       "Quota, GB",
		"f_speed":       "Speed, Mbit/s",
		"f_limits":      "Limits",
		"optional":      "optional",
		"online":        "online",
		"stat_dn":       "↓ total",
		"stat_up":       "↑ total",
		"stat_updated":  "updated",
		"h_client":      "Client",
		"h_speed_dn":    "↓ speed",
		"h_speed_up":    "↑ speed",
		"h_limit":       "limit",
		"h_quota":       "quota",
		"h_today":       "today",
		"h_week":        "week",
		"h_month":       "month",
		"h_expiry":      "expires",
		"h_handshake":   "handshake",
		"disabled":      "disabled",
		"conf":          "conf",
		"on":            "on",
		"off":           "off",
		"confirm_del":   "Delete client",
		"no_clients":    "no clients yet — add the first one above",
		"created_msg":   "created — download .conf (reliable) or scan the QR in the AmneziaWG app",
		"download":      "Download .conf",
		"expired":       "expired",
		"speed_unit":    "Mbit/s",
		"edit":          "edit",
		"edit_title":    "Edit client",
		"new_name":      "new name",
		"save":          "Save",
		"cancel":        "Cancel",
		"hint_limits":   "empty = no limit; expiry = disable after N days",
		"one_profile":   "Each device needs its own client. Sharing one config across devices clashes.",
		"qr_note":       "Open the AmneziaWG app and scan the QR (or import the .conf file). iOS — App Store: apps.apple.com/app/amneziawg/id6478942365",
	},
}

// normLang clamps an arbitrary value to a supported language code.
func normLang(s string) string {
	if _, ok := langs[s]; ok {
		return s
	}
	return defaultLang
}

// tr returns the string catalog for a language.
func tr(lang string) map[string]string { return langs[normLang(lang)] }

// Catalog returns the UI string catalog for a language (exported for tooling such
// as the preview generator). Unknown languages fall back to the default.
func Catalog(lang string) map[string]string { return tr(lang) }

// lang reads the preferred language from the cookie (default Russian).
func (s *Server) lang(r *http.Request) string {
	if c, err := r.Cookie(langCookie); err == nil {
		return normLang(c.Value)
	}
	return defaultLang
}

// setLang stores the language preference and redirects back.
func (s *Server) setLang(w http.ResponseWriter, r *http.Request) {
	code := normLang(r.PathValue("code"))
	http.SetCookie(w, &http.Cookie{
		Name:     langCookie,
		Value:    code,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   365 * 24 * 3600,
	})
	dest := r.Header.Get("Referer")
	if dest == "" {
		dest = "/"
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}
