// Frontend logic for the AmneziaWG Manager. Backend methods are exposed by Wails
// at window.go.main.App.*; runtime events arrive via window.runtime.EventsOn.

const $ = (id) => document.getElementById(id);
const backend = () => window.go.main.App;

const APP_URLS = {
  ios: "https://apps.apple.com/app/amneziawg/id6478942365",
  android: "https://play.google.com/store/apps/details?id=org.amnezia.awg",
  macos: "https://apps.apple.com/app/amneziawg/id6478942365",
  windows: "https://github.com/amnezia-vpn/amneziawg-windows-client/releases",
};

const RENT_URLS = {
  vdsina: "https://www.vdsina.com/?partner=7yhz21p6dkml",
  hshp: "https://hshp.host/?from=144227",
};

// --- i18n ------------------------------------------------------------------

const I18N = {
  ru: {
    switch_server: "Сменить сервер",
    connect_title: "Подключение к серверу",
    connect_sub: "Введите данные вашего Linux-сервера. Приложение подключится по SSH и всё сделает само.",
    howto_summary: "Как это работает? (3 шага)",
    howto_1: "<b>Сервер.</b> Нужен любой VPS на Ubuntu/Debian (например, у хостинг-провайдера) — его IP, логин (обычно <span class=\"mono\">root</span>) и пароль.",
    howto_2: "<b>Установка.</b> Подключитесь здесь — приложение само поставит AmneziaWG и создаст первого клиента.",
    howto_3: "<b>Подключение.</b> Установите приложение AmneziaWG на телефон/ПК и отсканируйте QR (или импортируйте файл .conf).",
    rent_lead: "Нет своего сервера? Можно арендовать VPS:",
    rent_note: "(партнёрские ссылки — поддерживают проект)",
    saved_servers: "Сохранённые серверы",
    lbl_host: "IP-адрес сервера",
    lbl_user: "Пользователь",
    lbl_label: "Название (необязательно)",
    ph_label: "напр. VDSina, Германия",
    auth_password: "Пароль",
    auth_key: "SSH-ключ",
    lbl_password: "Пароль",
    lbl_key: "Путь к приватному ключу",
    remember_pw: "Запомнить пароль (хранится в системном хранилище, не в файле)",
    btn_connect: "Подключиться",
    install_title: "AmneziaWG ещё не установлен",
    install_sub: "Нажмите «Установить» — всё настроится автоматически. Займёт пару минут.",
    lbl_first_client: "Имя первого клиента",
    advanced: "Дополнительно",
    lbl_port: "UDP-порт (необязательно — оставьте пустым для автоподбора)",
    ph_port: "авто",
    btn_install: "Установить",
    tab_clients: "Клиенты",
    tab_monitor: "Мониторинг",
    tab_panel: "Веб-панель",
    clients_title: "Клиенты",
    ph_new_client: "имя нового клиента",
    btn_add: "Добавить",
    hint_profile: "Один профиль = одно устройство. Для каждого устройства создавайте свой профиль, иначе будет конфликт.",
    th_name: "Имя",
    th_actions: "Действия",
    btn_uninstall: "Удалить AmneziaWG полностью",
    uptime_label: "аптайм",
    stat_status: "Статус",
    stat_clients: "Клиентов",
    stat_uptime: "Аптайм",
    traffic: "Трафик",
    th_client: "Клиент",
    th_rx: "↓ принято",
    th_tx: "↑ отдано",
    th_handshake: "рукопожатие",
    panel_title: "Лимиты на клиента",
    panel_desc: "Ограничение скорости, срок действия и квота трафика на каждого клиента настраиваются в веб-панели — у неё для этого есть постоянно работающая на сервере служба. Из приложения это включить нельзя: оно не запущено круглосуточно.",
    ph_panel_pass: "пароль администратора",
    btn_install_panel: "Установить веб-панель",
    panel_pw_rule: "Пароль: минимум 6 символов, строчные и заглавные буквы, цифра и спецсимвол (например <span class=\"mono\">Admin2@</span>).",
    panel_up: "✓ Веб-панель работает:",
    btn_open_panel: "Открыть в браузере",
    btn_remove_panel: "Удалить панель",
    panel_cert_note: "Браузер один раз предупредит про самоподписанный сертификат — это нормально, трафик шифруется.",
    tab_settings: "Настройки",
    settings_server: "Сервер",
    info_host: "Адрес",
    info_port: "UDP-порт",
    info_uptime: "Аптайм",
    info_clients: "Клиентов",
    info_panel: "Панель",
    settings_rename: "Название сервера",
    settings_rename_hint: "Понятное имя для этого подключения в приложении. На сам сервер не влияет.",
    ph_server_name: "например, VDSina DE",
    btn_save: "Сохранить",
    settings_password: "Пароль веб-панели",
    settings_password_hint: "Сменить пароль администратора веб-панели. Применяется сразу.",
    ph_new_panel_pass: "новый пароль",
    btn_change_pass: "Сменить пароль",
    settings_no_panel: "Веб-панель не установлена — её можно установить во вкладке «Веб-панель».",
    settings_danger: "Опасная зона",
    danger_remove_panel_t: "Удалить веб-панель",
    danger_remove_panel_d: "AmneziaWG и клиенты останутся.",
    danger_remove_awg_t: "Удалить AmneziaWG полностью",
    danger_remove_awg_d: "Снесёт VPN, панель, всех клиентов и конфиги с сервера.",
    btn_delete_all: "Удалить всё",
    busy_saving: "Сохраняю…",
    toast_renamed: "Название сохранено",
    e_rename: "Не удалось сохранить название: ",
    e_server_info: "Не удалось получить данные сервера: ",
    log_change_pass: "Смена пароля панели",
    busy_changing_pass: "Меняю пароль…",
    toast_pass_changed: "Пароль панели изменён",
    e_change_pass: "Не удалось сменить пароль: ",
    client_ready_prefix: "Клиент",
    client_ready_suffix: "готов",
    qr_note: "Откройте приложение <b>AmneziaWG</b> и отсканируйте QR — или импортируйте файл .conf.",
    download_awg: "Скачать AmneziaWG:",
    btn_download_conf: "Скачать .conf",
    cancel: "Отмена",

    act_config: "конфиг / QR",
    act_rename: "переименовать",
    act_delete: "удалить",
    delete: "Удалить",
    remove: "Убрать",
    delete_all: "Удалить всё",
    delete_panel: "Удалить панель",
    profile_remove_title: "Убрать из списка",
    clients_empty: "Пока нет клиентов. Добавьте первого выше.",
    traffic_empty: "Пока нет данных",
    vpn_running: "VPN работает",
    vpn_stopped: "VPN остановлен",
    status_unavailable: "статус недоступен",
    clients_count: "{n} клиент(ов)",
    traffic_summary: "(онлайн {online} из {total})",

    log_install: "Установка",
    log_uninstall: "Удаление",
    log_panel: "Установка веб-панели",

    busy_default: "Подождите…",
    busy_connecting: "Подключаюсь к серверу…",
    busy_checking: "Проверяю сервер…",
    busy_installing: "Устанавливаю AmneziaWG… это займёт пару минут",
    busy_creating: "Создаю клиента…",
    busy_loading_conf: "Загружаю конфиг…",
    busy_renaming: "Переименовываю…",
    busy_deleting_client: "Удаляю клиента…",
    busy_uninstalling: "Удаляю AmneziaWG…",
    busy_installing_panel: "Устанавливаю веб-панель…",
    busy_removing_panel: "Удаляю панель…",

    toast_installed: "AmneziaWG установлен",
    toast_uninstalled: "AmneziaWG полностью удалён",
    toast_panel_installed: "Веб-панель установлена",
    toast_panel_removed: "Веб-панель удалена",
    toast_saved: "Сохранено: {path}",
    toast_client_removed: "Клиент «{name}» удалён",
    toast_client_renamed: "Клиент переименован в «{name}»",

    err_no_host: "Укажите IP-адрес сервера",
    err_weak_pw: "Слабый пароль: мин. 6 символов, строчные и заглавные буквы, цифра и спецсимвол (например Admin2@)",
    e_check_server: "Не удалось проверить сервер: ",
    e_install: "Установка не удалась: ",
    e_create_client: "Не удалось создать клиента: ",
    e_load_conf: "Не удалось получить конфиг: ",
    e_rename: "Не удалось переименовать: ",
    e_delete_client: "Не удалось удалить клиента: ",
    e_uninstall: "Не удалось удалить: ",
    e_check_panel: "Не удалось проверить панель: ",
    e_install_panel: "Не удалось установить панель: ",
    e_remove_panel: "Не удалось удалить панель: ",
    e_open_browser: "Не удалось открыть браузер: ",
    e_save: "Не удалось сохранить: ",

    confirm_remove_client: "Удалить клиента «{name}»? Его профиль перестанет работать.",
    confirm_uninstall: "Это ПОЛНОСТЬЮ удалит AmneziaWG, веб-панель, всех клиентов и конфиги с сервера. Продолжить?",
    confirm_remove_panel: "Удалить веб-панель с сервера? AmneziaWG и клиенты останутся.",
    confirm_remove_profile: "Убрать сервер «{name}» из списка? Сам сервер не изменится.",
    prompt_rename: "Новое имя для клиента «{name}»:",
  },
  en: {
    switch_server: "Switch server",
    connect_title: "Connect to a server",
    connect_sub: "Enter your Linux server details. The app connects over SSH and does the rest.",
    howto_summary: "How it works (3 steps)",
    howto_1: "<b>Server.</b> You need any Ubuntu/Debian VPS (from a hosting provider) — its IP, a user (usually <span class=\"mono\">root</span>) and the password.",
    howto_2: "<b>Install.</b> Connect here — the app installs AmneziaWG and creates the first client for you.",
    howto_3: "<b>Connect.</b> Install the AmneziaWG app on your phone/PC and scan the QR (or import the .conf file).",
    rent_lead: "No server yet? You can rent a VPS:",
    rent_note: "(referral links — they support the project)",
    saved_servers: "Saved servers",
    lbl_host: "Server IP address",
    lbl_user: "User",
    lbl_label: "Label (optional)",
    ph_label: "e.g. VDSina, Germany",
    auth_password: "Password",
    auth_key: "SSH key",
    lbl_password: "Password",
    lbl_key: "Private key path",
    remember_pw: "Remember password (stored in the OS keychain, not in a file)",
    btn_connect: "Connect",
    install_title: "AmneziaWG is not installed yet",
    install_sub: "Click Install — everything is configured automatically. Takes a couple of minutes.",
    lbl_first_client: "First client name",
    advanced: "Advanced",
    lbl_port: "UDP port (optional — leave empty for auto)",
    ph_port: "auto",
    btn_install: "Install",
    tab_clients: "Clients",
    tab_monitor: "Monitoring",
    tab_panel: "Web panel",
    clients_title: "Clients",
    ph_new_client: "new client name",
    btn_add: "Add",
    hint_profile: "One profile = one device. Create a separate profile per device, or connections will clash.",
    th_name: "Name",
    th_actions: "Actions",
    btn_uninstall: "Remove AmneziaWG completely",
    uptime_label: "uptime",
    stat_status: "Status",
    stat_clients: "Clients",
    stat_uptime: "Uptime",
    traffic: "Traffic",
    th_client: "Client",
    th_rx: "↓ received",
    th_tx: "↑ sent",
    th_handshake: "handshake",
    panel_title: "Per-client limits",
    panel_desc: "Speed limit, expiry and traffic quota per client are configured in the web panel — it runs an always-on service on the server for that. The desktop app can't do it: it isn't running 24/7.",
    ph_panel_pass: "admin password",
    btn_install_panel: "Install web panel",
    panel_pw_rule: "Password: at least 6 chars with lower- and upper-case letters, a digit and a special character (e.g. <span class=\"mono\">Admin2@</span>).",
    panel_up: "✓ Web panel is running:",
    btn_open_panel: "Open in browser",
    btn_remove_panel: "Remove panel",
    panel_cert_note: "The browser warns once about the self-signed certificate — that's expected; traffic is still encrypted.",
    tab_settings: "Settings",
    settings_server: "Server",
    info_host: "Address",
    info_port: "UDP port",
    info_uptime: "Uptime",
    info_clients: "Clients",
    info_panel: "Panel",
    settings_rename: "Server name",
    settings_rename_hint: "A friendly name for this connection in the app. Does not affect the server itself.",
    ph_server_name: "e.g. VDSina DE",
    btn_save: "Save",
    settings_password: "Web panel password",
    settings_password_hint: "Change the web panel admin password. Applies immediately.",
    ph_new_panel_pass: "new password",
    btn_change_pass: "Change password",
    settings_no_panel: "The web panel is not installed — you can install it on the \"Web panel\" tab.",
    settings_danger: "Danger zone",
    danger_remove_panel_t: "Remove the web panel",
    danger_remove_panel_d: "AmneziaWG and clients stay.",
    danger_remove_awg_t: "Remove AmneziaWG completely",
    danger_remove_awg_d: "Wipes the VPN, panel, all clients and configs from the server.",
    btn_delete_all: "Delete all",
    busy_saving: "Saving…",
    toast_renamed: "Name saved",
    e_rename: "Could not save the name: ",
    e_server_info: "Could not fetch server info: ",
    log_change_pass: "Changing panel password",
    busy_changing_pass: "Changing password…",
    toast_pass_changed: "Panel password changed",
    e_change_pass: "Could not change the password: ",
    client_ready_prefix: "Client",
    client_ready_suffix: "is ready",
    qr_note: "Open the <b>AmneziaWG</b> app and scan the QR — or import the .conf file.",
    download_awg: "Download AmneziaWG:",
    btn_download_conf: "Download .conf",
    cancel: "Cancel",

    act_config: "config / QR",
    act_rename: "rename",
    act_delete: "delete",
    delete: "Delete",
    remove: "Remove",
    delete_all: "Delete everything",
    delete_panel: "Remove panel",
    profile_remove_title: "Remove from list",
    clients_empty: "No clients yet. Add the first one above.",
    traffic_empty: "No data yet",
    vpn_running: "VPN is running",
    vpn_stopped: "VPN is stopped",
    status_unavailable: "status unavailable",
    clients_count: "{n} client(s)",
    traffic_summary: "({online} of {total} online)",

    log_install: "Installation",
    log_uninstall: "Removal",
    log_panel: "Web panel install",

    busy_default: "Please wait…",
    busy_connecting: "Connecting to the server…",
    busy_checking: "Checking the server…",
    busy_installing: "Installing AmneziaWG… this takes a couple of minutes",
    busy_creating: "Creating client…",
    busy_loading_conf: "Loading config…",
    busy_renaming: "Renaming…",
    busy_deleting_client: "Removing client…",
    busy_uninstalling: "Removing AmneziaWG…",
    busy_installing_panel: "Installing web panel…",
    busy_removing_panel: "Removing panel…",

    toast_installed: "AmneziaWG installed",
    toast_uninstalled: "AmneziaWG fully removed",
    toast_panel_installed: "Web panel installed",
    toast_panel_removed: "Web panel removed",
    toast_saved: "Saved: {path}",
    toast_client_removed: "Client \"{name}\" removed",
    toast_client_renamed: "Client renamed to \"{name}\"",

    err_no_host: "Enter the server IP address",
    err_weak_pw: "Weak password: at least 6 chars with lower- and upper-case letters, a digit and a special character (e.g. Admin2@)",
    e_check_server: "Could not check the server: ",
    e_install: "Install failed: ",
    e_create_client: "Could not create the client: ",
    e_load_conf: "Could not get the config: ",
    e_rename: "Could not rename: ",
    e_delete_client: "Could not remove the client: ",
    e_uninstall: "Could not remove: ",
    e_check_panel: "Could not check the panel: ",
    e_install_panel: "Could not install the panel: ",
    e_remove_panel: "Could not remove the panel: ",
    e_open_browser: "Could not open the browser: ",
    e_save: "Could not save: ",

    confirm_remove_client: "Remove client \"{name}\"? Its profile will stop working.",
    confirm_uninstall: "This will COMPLETELY remove AmneziaWG, the web panel, all clients and configs from the server. Continue?",
    confirm_remove_panel: "Remove the web panel from the server? AmneziaWG and clients stay.",
    confirm_remove_profile: "Remove server \"{name}\" from the list? The server itself is not changed.",
    prompt_rename: "New name for client \"{name}\":",
  },
};

let LANG = localStorage.getItem("awg-lang") === "en" ? "en" : "ru";

function t(key, vars) {
  let s = (I18N[LANG] && I18N[LANG][key]) || I18N.ru[key] || key;
  if (vars) for (const k in vars) s = s.replaceAll("{" + k + "}", vars[k]);
  return s;
}

function applyI18n() {
  document.documentElement.lang = LANG;
  document.querySelectorAll("[data-i18n]").forEach((el) => { el.textContent = t(el.dataset.i18n); });
  document.querySelectorAll("[data-i18n-html]").forEach((el) => { el.innerHTML = t(el.dataset.i18nHtml); });
  document.querySelectorAll("[data-i18n-ph]").forEach((el) => { el.placeholder = t(el.dataset.i18nPh); });
  document.querySelectorAll(".langswitch a").forEach((a) => a.classList.toggle("on", a.dataset.lang === LANG));
}

async function setLang(lang) {
  LANG = lang === "en" ? "en" : "ru";
  localStorage.setItem("awg-lang", LANG);
  try { await backend().SetLang(LANG); } catch (_) { /* ignore */ }
  applyI18n();
  // Re-render dynamic text that isn't covered by data-i18n.
  if (!$("view-connect").classList.contains("hidden")) loadProfiles();
  if (!$("view-server").classList.contains("hidden") && !$("block-manage").classList.contains("hidden")) {
    refreshClients();
    refreshHealth();
    refreshTraffic();
  }
}

// openExternal opens a URL in the user's real browser, not the app webview.
function openExternal(url) {
  if (window.runtime && window.runtime.BrowserOpenURL) window.runtime.BrowserOpenURL(url);
}

let authMode = "password";
let lastResult = null;

// --- small UI helpers ------------------------------------------------------

function show(el) { el.classList.remove("hidden"); }
function hide(el) { el.classList.add("hidden"); }

function busy(on, text) {
  $("busy-text").textContent = text || t("busy_default");
  on ? show($("busy")) : hide($("busy"));
}

// confirmDialog shows an in-app modal and resolves true/false. Native confirm()
// is unreliable inside the Wails webview, so we never use it.
function confirmDialog(message, okLabel) {
  return new Promise((resolve) => {
    $("modal-text").textContent = message;
    $("modal-ok").textContent = okLabel || t("delete");
    show($("modal"));
    const done = (val) => {
      hide($("modal"));
      $("modal-ok").onclick = null;
      $("modal-cancel").onclick = null;
      resolve(val);
    };
    $("modal-ok").onclick = () => done(true);
    $("modal-cancel").onclick = () => done(false);
  });
}

let toastTimer = null;
function toast(message, kind = "ok") {
  const el = $("toast");
  el.textContent = message;
  el.className = "toast " + kind;
  show(el);
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => hide(el), 4500);
}

function errMsg(err) {
  return typeof err === "string" ? err : (err && err.message) || String(err);
}

// openLog clears and reveals the collapsible log drawer for an operation.
function openLog(title) {
  $("log").textContent = "";
  $("log-title").textContent = title;
  const d = $("log-panel");
  d.classList.remove("hidden");
  d.open = true;
}

// promptDialog shows an in-app text-input modal and resolves the value (or null).
function promptDialog(message, defaultValue = "") {
  return new Promise((resolve) => {
    $("prompt-text").textContent = message;
    const input = $("prompt-input");
    input.value = defaultValue;
    show($("prompt"));
    input.focus();
    input.select();
    const done = (val) => {
      hide($("prompt"));
      $("prompt-ok").onclick = null;
      input.onkeydown = null;
      resolve(val);
    };
    $("prompt-ok").onclick = () => done(input.value.trim());
    input.onkeydown = (e) => { if (e.key === "Enter") done(input.value.trim()); };
    window.__closePrompt = () => done(null);
  });
}

function closePrompt() {
  if (window.__closePrompt) window.__closePrompt();
}

// switchServer disconnects and returns to the connect screen to pick/add a server.
async function switchServer() {
  stopTraffic();
  try {
    await backend().Disconnect();
  } catch (_) {
    /* ignore */
  }
  hide($("view-server"));
  hide($("conn-pill"));
  hide($("btn-switch"));
  show($("view-connect"));
  $("password").value = "";
  await prefill();
}

function showError(el, err) {
  el.textContent = typeof err === "string" ? err : (err && err.message) || String(err);
  show(el);
}

function appendLog(text) {
  const log = $("log");
  log.textContent += text;
  log.scrollTop = log.scrollHeight;
}

// --- connect ---------------------------------------------------------------

function selectAuth(mode) {
  authMode = mode;
  document.querySelectorAll(".tab").forEach((tab) => tab.classList.toggle("on", tab.dataset.auth === mode));
  $("field-password").classList.toggle("hidden", mode !== "password");
  $("field-key").classList.toggle("hidden", mode !== "key");
}

function initAuthTabs() {
  document.querySelectorAll(".tab").forEach((tab) => {
    tab.addEventListener("click", () => selectAuth(tab.dataset.auth));
  });
}

const MANAGE_TABS = ["clients", "monitor", "advanced", "settings"];

function selectTab(name) {
  document.querySelectorAll(".nav-item").forEach((b) => b.classList.toggle("on", b.dataset.tab === name));
  MANAGE_TABS.forEach((tab) => $("tab-" + tab).classList.toggle("hidden", tab !== name));
  if (name === "settings") loadSettings();
}

function initTabs() {
  document.querySelectorAll(".nav-item").forEach((btn) => {
    btn.addEventListener("click", () => selectTab(btn.dataset.tab));
  });
}

// profileLabel is the display name for a saved server: its label or user@host.
function profileLabel(p) {
  return (p.label && p.label.trim()) || (p.user || "root") + "@" + p.host;
}

// fillForm populates the connect form from a saved Prefs/Profile object.
function fillForm(p) {
  if (!p) return;
  $("host").value = p.host || "";
  $("user").value = p.user || "root";
  $("srv-label").value = p.label || "";
  $("identity").value = p.identityPath || "";
  selectAuth(p.authMode || "password");
  $("remember").checked = !!p.remember;
  $("password").value = p.password || "";
}

// prefill restores the last-used connection (password from the OS secret store)
// and renders the saved-servers list.
async function prefill() {
  try {
    fillForm(await backend().LoadPrefs());
  } catch (_) {
    /* no saved prefs yet — ignore */
  }
  await loadProfiles();
}

// loadProfiles renders saved servers as one-click reconnect rows.
async function loadProfiles() {
  let list = [];
  try {
    list = (await backend().ListProfiles()) || [];
  } catch (_) {
    return;
  }
  const box = $("profiles");
  const ul = $("profiles-list");
  ul.innerHTML = "";
  if (list.length === 0) {
    hide(box);
    return;
  }
  list.forEach((p) => {
    const label = profileLabel(p);
    const row = document.createElement("div");
    row.className = "profile";

    const pick = document.createElement("button");
    pick.className = "pick";
    pick.textContent = label;
    pick.addEventListener("click", () => {
      fillForm(p);
      connect();
    });

    const x = document.createElement("button");
    x.className = "x";
    x.textContent = "×";
    x.title = t("profile_remove_title");
    x.addEventListener("click", async (e) => {
      e.stopPropagation();
      const ok = await confirmDialog(t("confirm_remove_profile", { name: label }), t("remove"));
      if (!ok) return;
      try {
        await backend().DeleteProfile(p.host, p.user);
      } catch (_) {
        /* ignore */
      }
      await loadProfiles();
    });

    row.append(pick, x);
    ul.appendChild(row);
  });
  show(box);
}

async function connect() {
  hide($("connect-err"));
  const host = $("host").value.trim();
  if (!host) { showError($("connect-err"), t("err_no_host")); return; }

  const user = $("user").value.trim() || "root";
  const label = $("srv-label").value.trim();
  const req = {
    host,
    user,
    label,
    password: authMode === "password" ? $("password").value : "",
    identityPath: authMode === "key" ? $("identity").value.trim() : "",
    authMode,
    remember: $("remember").checked,
  };

  busy(true, t("busy_connecting"));
  try {
    await backend().Connect(req);
    $("conn-pill").textContent = label || user + "@" + host;
    show($("conn-pill"));
    show($("btn-switch"));
    hide($("view-connect"));
    show($("view-server"));
    await refreshStatus();
    loadProfiles();
  } catch (err) {
    showError($("connect-err"), err);
  } finally {
    busy(false);
  }
}

// --- server status: install vs manage --------------------------------------

async function refreshStatus() {
  busy(true, t("busy_checking"));
  try {
    const status = await backend().ServerStatus();
    hide($("result"));
    if (status.installed) {
      hide($("block-install"));
      show($("block-manage"));
      selectTab("clients");
      await refreshClients();
      await refreshPanel();
      await refreshHealth();
      startTraffic();
    } else {
      show($("block-install"));
      hide($("block-manage"));
      stopTraffic();
    }
  } catch (err) {
    toast(t("e_check_server") + errMsg(err), "err");
  } finally {
    busy(false);
  }
}

// --- install ---------------------------------------------------------------

async function install() {
  const req = {
    port: $("port").value.trim(),
    client: $("first-client").value.trim() || "phone",
  };
  openLog(t("log_install"));
  $("btn-install").disabled = true;
  busy(true, t("busy_installing"));
  try {
    const res = await backend().Install(req);
    await refreshStatus();
    showResult(res);
    toast(t("toast_installed"), "ok");
  } catch (err) {
    toast(t("e_install") + errMsg(err), "err");
  } finally {
    busy(false);
    $("btn-install").disabled = false;
  }
}

// --- manage clients --------------------------------------------------------

async function refreshClients() {
  const names = (await backend().ListClients()) || [];
  const body = $("clients-body");
  body.innerHTML = "";
  if (names.length === 0) {
    const tr = document.createElement("tr");
    const td = document.createElement("td");
    td.colSpan = 2;
    td.className = "empty";
    td.textContent = t("clients_empty");
    tr.appendChild(td);
    body.appendChild(tr);
    return;
  }
  names.forEach((name) => {
    const tr = document.createElement("tr");
    const nameTd = document.createElement("td");
    nameTd.className = "name";
    nameTd.textContent = name;
    const actTd = document.createElement("td");
    actTd.className = "r";
    actTd.innerHTML = '<div class="row-actions"></div>';
    const actions = actTd.querySelector(".row-actions");

    const conf = document.createElement("button");
    conf.className = "link";
    conf.textContent = t("act_config");
    conf.addEventListener("click", () => showClientConfig(name));
    actions.appendChild(conf);

    const ren = document.createElement("button");
    ren.className = "link";
    ren.textContent = t("act_rename");
    ren.addEventListener("click", () => renameClient(name));
    actions.appendChild(ren);

    const del = document.createElement("button");
    del.className = "link del";
    del.textContent = t("act_delete");
    del.addEventListener("click", () => removeClient(name));
    actions.appendChild(del);

    tr.append(nameTd, actTd);
    body.appendChild(tr);
  });
}

async function addClient(e) {
  e.preventDefault();
  const name = $("new-client").value.trim();
  if (!name) return;
  busy(true, t("busy_creating"));
  try {
    const res = await backend().AddClient(name);
    $("new-client").value = "";
    await refreshClients();
    showResult(res);
  } catch (err) {
    toast(t("e_create_client") + errMsg(err), "err");
  } finally {
    busy(false);
  }
}

async function showClientConfig(name) {
  busy(true, t("busy_loading_conf"));
  try {
    const res = await backend().ClientConfig(name);
    showResult(res);
  } catch (err) {
    toast(t("e_load_conf") + errMsg(err), "err");
  } finally {
    busy(false);
  }
}

async function renameClient(name) {
  const newName = await promptDialog(t("prompt_rename", { name }), name);
  if (!newName || newName === name) return;
  busy(true, t("busy_renaming"));
  try {
    await backend().RenameClient(name, newName);
    await refreshClients();
    toast(t("toast_client_renamed", { name: newName }), "ok");
  } catch (err) {
    toast(t("e_rename") + errMsg(err), "err");
  } finally {
    busy(false);
  }
}

async function removeClient(name) {
  const ok = await confirmDialog(t("confirm_remove_client", { name }));
  if (!ok) return;
  busy(true, t("busy_deleting_client"));
  try {
    await backend().RemoveClient(name);
    await refreshClients();
    toast(t("toast_client_removed", { name }), "ok");
  } catch (err) {
    toast(t("e_delete_client") + errMsg(err), "err");
  } finally {
    busy(false);
  }
}

async function uninstall() {
  const ok = await confirmDialog(t("confirm_uninstall"), t("delete_all"));
  if (!ok) return;
  openLog(t("log_uninstall"));
  busy(true, t("busy_uninstalling"));
  try {
    await backend().Uninstall();
    await refreshStatus();
    toast(t("toast_uninstalled"), "ok");
  } catch (err) {
    toast(t("e_uninstall") + errMsg(err), "err");
  } finally {
    busy(false);
  }
}

// --- server health & live traffic ------------------------------------------

async function refreshHealth() {
  try {
    const h = await backend().ServerHealth();
    $("health-dot").className = "health-dot " + (h.running ? "up" : "down");
    $("health-state").textContent = h.running ? t("vpn_running") : t("vpn_stopped");
    $("health-clients").textContent = h.clients;
    $("health-uptime").textContent = h.uptime || "—";
    $("health-version").textContent = h.version || "—";
  } catch (_) {
    $("health-dot").className = "health-dot";
    $("health-state").textContent = t("status_unavailable");
  }
}

let trafficTimer = null;
let trafficBusy = false;

async function refreshTraffic() {
  if (trafficBusy) return;
  trafficBusy = true;
  try {
    const r = await backend().Traffic();
    $("traffic-summary").textContent = t("traffic_summary", { online: r.online, total: r.total });
    const body = $("traffic-body");
    body.innerHTML = "";
    if (!r.peers || r.peers.length === 0) {
      const tr = document.createElement("tr");
      const td = document.createElement("td");
      td.colSpan = 5;
      td.className = "traffic-empty";
      td.textContent = t("traffic_empty");
      tr.appendChild(td);
      body.appendChild(tr);
      return;
    }
    r.peers.forEach((p) => {
      const tr = document.createElement("tr");
      const dotTd = document.createElement("td");
      dotTd.innerHTML = `<span class="tdot${p.online ? " on" : ""}"></span>`;
      const name = document.createElement("td");
      name.className = "name";
      name.textContent = p.name;
      const rx = document.createElement("td");
      rx.className = "r mono";
      rx.textContent = p.rx;
      const tx = document.createElement("td");
      tx.className = "r mono";
      tx.textContent = p.tx;
      const hs = document.createElement("td");
      hs.className = "r dim";
      hs.textContent = p.handshake;
      tr.append(dotTd, name, rx, tx, hs);
      body.appendChild(tr);
    });
  } catch (_) {
    /* transient — keep last values */
  } finally {
    trafficBusy = false;
  }
}

function startTraffic() {
  refreshTraffic();
  if (!trafficTimer) trafficTimer = setInterval(refreshTraffic, 5000);
}

function stopTraffic() {
  if (trafficTimer) {
    clearInterval(trafficTimer);
    trafficTimer = null;
  }
}

// --- web panel (advanced limits) -------------------------------------------

async function refreshPanel() {
  try {
    const p = await backend().PanelStatus();
    $("panel-url").textContent = p.url;
    $("panel-absent").classList.toggle("hidden", p.installed);
    $("panel-present").classList.toggle("hidden", !p.installed);
  } catch (err) {
    toast(t("e_check_panel") + errMsg(err), "err");
  }
}

function validPanelPassword(p) {
  return p.length >= 6 && /[a-z]/.test(p) && /[A-Z]/.test(p) && /[0-9]/.test(p) && /[^a-zA-Z0-9]/.test(p);
}

async function installPanel() {
  const pass = $("panel-pass").value;
  if (!validPanelPassword(pass)) {
    toast(t("err_weak_pw"), "err");
    return;
  }
  openLog(t("log_panel"));
  busy(true, t("busy_installing_panel"));
  try {
    await backend().InstallPanel(pass);
    $("panel-pass").value = "";
    await refreshPanel();
    toast(t("toast_panel_installed"), "ok");
  } catch (err) {
    toast(t("e_install_panel") + errMsg(err), "err");
  } finally {
    busy(false);
  }
}

async function removePanel() {
  const ok = await confirmDialog(t("confirm_remove_panel"), t("delete_panel"));
  if (!ok) return;
  busy(true, t("busy_removing_panel"));
  try {
    await backend().RemovePanel();
    await refreshPanel();
    toast(t("toast_panel_removed"), "ok");
  } catch (err) {
    toast(t("e_remove_panel") + errMsg(err), "err");
  } finally {
    busy(false);
  }
}

async function openPanel() {
  try {
    await backend().OpenPanel();
  } catch (err) {
    toast(t("e_open_browser") + errMsg(err), "err");
  }
}

// --- settings --------------------------------------------------------------

// loadSettings populates the settings tab: the server-info card, the rename
// field, and toggles the panel-password section by whether the panel exists.
async function loadSettings() {
  $("rename-input").value = ($("srv-label").value || "").trim();
  try {
    const info = await backend().ServerInfo();
    $("info-host").textContent = info.host || "—";
    $("info-port").textContent = info.port || "—";
    $("info-version").textContent = info.version || "—";
    $("info-uptime").textContent = info.uptime || "—";
    $("info-clients").textContent = info.clients != null ? String(info.clients) : "—";
    $("info-panel").textContent = info.panelUrl || "—";
  } catch (err) {
    toast(t("e_server_info") + errMsg(err), "err");
  }
  try {
    const p = await backend().PanelStatus();
    $("settings-panel-pw").classList.toggle("hidden", !p.installed);
    $("settings-panel-absent").classList.toggle("hidden", p.installed);
    $("danger-panel-row").classList.toggle("hidden", !p.installed);
  } catch (_) {
    /* leave defaults if the check fails */
  }
}

async function renameServer() {
  const label = $("rename-input").value.trim();
  busy(true, t("busy_saving"));
  try {
    await backend().RenameServer(label);
    await loadProfiles();
    toast(t("toast_renamed"), "ok");
  } catch (err) {
    toast(t("e_rename") + errMsg(err), "err");
  } finally {
    busy(false);
  }
}

async function changePanelPassword() {
  const pass = $("newpanel-pass").value;
  if (!validPanelPassword(pass)) {
    toast(t("err_weak_pw"), "err");
    return;
  }
  openLog(t("log_change_pass"));
  busy(true, t("busy_changing_pass"));
  try {
    await backend().ChangePanelPassword(pass);
    $("newpanel-pass").value = "";
    toast(t("toast_pass_changed"), "ok");
  } catch (err) {
    toast(t("e_change_pass") + errMsg(err), "err");
  } finally {
    busy(false);
  }
}

// --- client result (QR + conf) ---------------------------------------------

function showResult(res) {
  lastResult = res;
  $("result-name").textContent = res.name;
  $("result-qr").src = res.qr || "";
  $("result-conf").textContent = res.conf || "";
  show($("result"));
  $("result").scrollIntoView({ behavior: "smooth", block: "start" });
}

async function downloadConf() {
  if (!lastResult) return;
  try {
    const path = await backend().SaveConfig(lastResult.name, lastResult.conf);
    if (path) toast(t("toast_saved", { path }), "ok");
  } catch (err) {
    toast(t("e_save") + errMsg(err), "err");
  }
}

// --- wire up ---------------------------------------------------------------

window.addEventListener("DOMContentLoaded", () => {
  applyI18n();
  backend().SetLang(LANG).catch(() => {});
  initAuthTabs();
  initTabs();
  prefill();

  document.querySelectorAll(".langswitch a").forEach((a) => {
    a.addEventListener("click", (e) => { e.preventDefault(); setLang(a.dataset.lang); });
  });

  $("btn-connect").addEventListener("click", connect);
  $("password").addEventListener("keydown", (e) => { if (e.key === "Enter") connect(); });
  $("btn-install").addEventListener("click", install);
  $("add-form").addEventListener("submit", addClient);
  $("btn-uninstall").addEventListener("click", uninstall);
  $("btn-install-panel").addEventListener("click", installPanel);
  $("btn-open-panel").addEventListener("click", openPanel);
  $("btn-remove-panel").addEventListener("click", removePanel);
  $("btn-rename-server").addEventListener("click", renameServer);
  $("btn-change-pass").addEventListener("click", changePanelPassword);
  $("result-close").addEventListener("click", () => hide($("result")));
  $("result-download").addEventListener("click", downloadConf);
  ["ios", "android", "macos", "windows"].forEach((os) => {
    $("link-" + os).addEventListener("click", (e) => { e.preventDefault(); openExternal(APP_URLS[os]); });
  });
  $("rent-vdsina").addEventListener("click", (e) => { e.preventDefault(); openExternal(RENT_URLS.vdsina); });
  $("rent-hshp").addEventListener("click", (e) => { e.preventDefault(); openExternal(RENT_URLS.hshp); });
  $("btn-switch").addEventListener("click", switchServer);
  $("prompt-cancel").addEventListener("click", () => closePrompt(null));

  if (window.runtime) {
    window.runtime.EventsOn("install:log", appendLog);
    window.runtime.EventsOn("client:log", appendLog);
    window.runtime.EventsOn("panel:log", appendLog);
  }
});
