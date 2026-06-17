// Frontend logic for the AmneziaWG Manager. Backend methods are exposed by Wails
// at window.go.main.App.*; runtime events arrive via window.runtime.EventsOn.

const $ = (id) => document.getElementById(id);
const backend = () => window.go.main.App;

let authMode = "password";
let lastResult = null;

// --- small UI helpers ------------------------------------------------------

function show(el) { el.classList.remove("hidden"); }
function hide(el) { el.classList.add("hidden"); }

function busy(on, text) {
  $("busy-text").textContent = text || "Подождите…";
  on ? show($("busy")) : hide($("busy"));
}

// confirmDialog shows an in-app modal and resolves true/false. Native confirm()
// is unreliable inside the Wails webview, so we never use it.
function confirmDialog(message, okLabel = "Удалить") {
  return new Promise((resolve) => {
    $("modal-text").textContent = message;
    $("modal-ok").textContent = okLabel;
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
  const t = $("toast");
  t.textContent = message;
  t.className = "toast " + kind;
  show(t);
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => hide(t), 4500);
}

function errMsg(err) {
  return typeof err === "string" ? err : (err && err.message) || String(err);
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

function initAuthTabs() {
  document.querySelectorAll(".tab").forEach((tab) => {
    tab.addEventListener("click", () => {
      document.querySelectorAll(".tab").forEach((t) => t.classList.remove("on"));
      tab.classList.add("on");
      authMode = tab.dataset.auth;
      $("field-password").classList.toggle("hidden", authMode !== "password");
      $("field-key").classList.toggle("hidden", authMode !== "key");
    });
  });
}

async function connect() {
  hide($("connect-err"));
  const host = $("host").value.trim();
  if (!host) { showError($("connect-err"), "Укажите IP-адрес сервера"); return; }

  const req = {
    host,
    user: $("user").value.trim() || "root",
    password: authMode === "password" ? $("password").value : "",
    identityPath: authMode === "key" ? $("identity").value.trim() : "",
  };

  busy(true, "Подключаюсь к серверу…");
  try {
    await backend().Connect(req);
    $("conn-pill").textContent = req.user + "@" + host;
    show($("conn-pill"));
    hide($("view-connect"));
    show($("view-server"));
    await refreshStatus();
  } catch (err) {
    showError($("connect-err"), err);
  } finally {
    busy(false);
  }
}

// --- server status: install vs manage --------------------------------------

async function refreshStatus() {
  busy(true, "Проверяю сервер…");
  try {
    const status = await backend().ServerStatus();
    hide($("result"));
    if (status.installed) {
      hide($("block-install"));
      show($("block-manage"));
      await refreshClients();
      await refreshPanel();
    } else {
      show($("block-install"));
      hide($("block-manage"));
    }
  } catch (err) {
    toast("Не удалось проверить сервер: " + errMsg(err), "err");
  } finally {
    busy(false);
  }
}

// --- install ---------------------------------------------------------------

async function install() {
  const req = {
    preset: $("preset").value,
    port: $("port").value.trim(),
    client: $("first-client").value.trim() || "phone",
  };
  $("log").textContent = "";
  $("log-title").textContent = "Установка";
  show($("log-panel"));
  $("btn-install").disabled = true;
  busy(true, "Устанавливаю AmneziaWG… это займёт пару минут");
  try {
    const res = await backend().Install(req);
    await refreshStatus();
    showResult(res);
    toast("AmneziaWG установлен", "ok");
  } catch (err) {
    toast("Установка не удалась: " + errMsg(err), "err");
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
    tr.innerHTML = '<td colspan="2" class="empty">Пока нет клиентов. Добавьте первого выше.</td>';
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
    const del = document.createElement("button");
    del.className = "link del";
    del.textContent = "удалить";
    del.addEventListener("click", () => removeClient(name));
    actTd.querySelector(".row-actions").appendChild(del);
    tr.append(nameTd, actTd);
    body.appendChild(tr);
  });
}

async function addClient(e) {
  e.preventDefault();
  const name = $("new-client").value.trim();
  if (!name) return;
  busy(true, "Создаю клиента…");
  try {
    const res = await backend().AddClient(name);
    $("new-client").value = "";
    await refreshClients();
    showResult(res);
  } catch (err) {
    toast("Не удалось создать клиента: " + errMsg(err), "err");
  } finally {
    busy(false);
  }
}

async function removeClient(name) {
  const ok = await confirmDialog(`Удалить клиента «${name}»? Его профиль перестанет работать.`);
  if (!ok) return;
  busy(true, "Удаляю клиента…");
  try {
    await backend().RemoveClient(name);
    await refreshClients();
    toast(`Клиент «${name}» удалён`, "ok");
  } catch (err) {
    toast("Не удалось удалить клиента: " + errMsg(err), "err");
  } finally {
    busy(false);
  }
}

async function uninstall() {
  const ok = await confirmDialog(
    "Это ПОЛНОСТЬЮ удалит AmneziaWG, веб-панель, всех клиентов и конфиги с сервера. Продолжить?",
    "Удалить всё"
  );
  if (!ok) return;
  $("log").textContent = "";
  $("log-title").textContent = "Удаление";
  show($("log-panel"));
  busy(true, "Удаляю AmneziaWG…");
  try {
    await backend().Uninstall();
    await refreshStatus();
    toast("AmneziaWG полностью удалён", "ok");
  } catch (err) {
    toast("Не удалось удалить: " + errMsg(err), "err");
  } finally {
    busy(false);
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
    toast("Не удалось проверить панель: " + errMsg(err), "err");
  }
}

async function installPanel() {
  const pass = $("panel-pass").value;
  if (pass.length < 8) {
    toast("Пароль панели — минимум 8 символов", "err");
    return;
  }
  $("log").textContent = "";
  $("log-title").textContent = "Установка веб-панели";
  show($("log-panel"));
  busy(true, "Устанавливаю веб-панель…");
  try {
    await backend().InstallPanel(pass);
    $("panel-pass").value = "";
    await refreshPanel();
    toast("Веб-панель установлена", "ok");
  } catch (err) {
    toast("Не удалось установить панель: " + errMsg(err), "err");
  } finally {
    busy(false);
  }
}

async function removePanel() {
  const ok = await confirmDialog("Удалить веб-панель с сервера? AmneziaWG и клиенты останутся.", "Удалить панель");
  if (!ok) return;
  busy(true, "Удаляю панель…");
  try {
    await backend().RemovePanel();
    await refreshPanel();
    toast("Веб-панель удалена", "ok");
  } catch (err) {
    toast("Не удалось удалить панель: " + errMsg(err), "err");
  } finally {
    busy(false);
  }
}

async function openPanel() {
  try {
    await backend().OpenPanel();
  } catch (err) {
    toast("Не удалось открыть браузер: " + errMsg(err), "err");
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

function downloadConf() {
  if (!lastResult) return;
  const blob = new Blob([lastResult.conf], { type: "text/plain" });
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = lastResult.name + ".conf";
  a.click();
  URL.revokeObjectURL(a.href);
}

// --- wire up ---------------------------------------------------------------

window.addEventListener("DOMContentLoaded", () => {
  initAuthTabs();
  $("btn-connect").addEventListener("click", connect);
  $("password").addEventListener("keydown", (e) => { if (e.key === "Enter") connect(); });
  $("btn-install").addEventListener("click", install);
  $("add-form").addEventListener("submit", addClient);
  $("btn-uninstall").addEventListener("click", uninstall);
  $("btn-install-panel").addEventListener("click", installPanel);
  $("btn-open-panel").addEventListener("click", openPanel);
  $("btn-remove-panel").addEventListener("click", removePanel);
  $("result-close").addEventListener("click", () => hide($("result")));
  $("result-download").addEventListener("click", downloadConf);

  if (window.runtime) {
    window.runtime.EventsOn("install:log", appendLog);
    window.runtime.EventsOn("client:log", appendLog);
    window.runtime.EventsOn("panel:log", appendLog);
  }
});
