"use strict";

const ROLE_LABELS = {
  operator: "Operatör",
  hat_sefi: "Hat şefi",
  kalite_muhendisi: "Kalite mühendisi",
  uretim_planlama: "Üretim planlama",
  bakim: "Bakım",
  fabrika_muduru: "Fabrika müdürü",
};

const CHART_TYPE_LABELS = {
  individual: "Bireysel (I-MR)",
  xbar_r: "X̄-R",
};

const SESSION_KEY = "hatkontrol.session";

const session = { token: null, user: null };

const $ = (id) => document.getElementById(id);

class ApiError extends Error {
  constructor(status, message) {
    super(message);
    this.status = status;
  }
}

// api, backend'e /api öneki üzerinden istek atar ve hataları ApiError olarak fırlatır.
async function api(path, { method = "GET", body } = {}) {
  const headers = {};
  if (session.token) {
    headers.Authorization = "Bearer " + session.token;
  }
  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
  }

  let response;
  try {
    response = await fetch("/api" + path, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch (_) {
    throw new ApiError(0, "Sunucuya ulaşılamıyor.");
  }

  let data = null;
  try {
    data = await response.json();
  } catch (_) {
    // gövde yok ya da JSON değil
  }

  if (!response.ok) {
    if (response.status === 401 && session.token) {
      endSession("Oturum süresi doldu, lütfen yeniden giriş yapın.");
    }
    const message = data && data.error && data.error.message
      ? data.error.message
      : "Beklenmeyen bir hata oluştu.";
    throw new ApiError(response.status, message);
  }
  return data;
}

function saveSession(token, user) {
  session.token = token;
  session.user = user;
  try {
    sessionStorage.setItem(SESSION_KEY, JSON.stringify({ token, user }));
  } catch (_) {
    // depolama kapalıysa oturum yalnızca bellekte kalır
  }
}

function loadSession() {
  try {
    const raw = sessionStorage.getItem(SESSION_KEY);
    if (!raw) {
      return false;
    }
    const saved = JSON.parse(raw);
    if (!saved.token || !saved.user) {
      return false;
    }
    session.token = saved.token;
    session.user = saved.user;
    return true;
  } catch (_) {
    return false;
  }
}

function endSession(message) {
  session.token = null;
  session.user = null;
  try {
    sessionStorage.removeItem(SESSION_KEY);
  } catch (_) {
    // yoksay
  }
  showLogin(message);
}

function showLogin(message) {
  $("login-view").hidden = false;
  $("app-view").hidden = true;
  $("session-info").hidden = true;
  $("password").value = "";

  const error = $("login-error");
  error.textContent = message || "";
  error.hidden = !message;
}

function showApp() {
  $("login-view").hidden = true;
  $("app-view").hidden = false;
  $("session-info").hidden = false;

  const roleLabel = ROLE_LABELS[session.user.role] || session.user.role;
  $("user-name").textContent = session.user.full_name + " (" + roleLabel + ")";

  loadCharts();
}

// Sunucudan gelen metinler textContent ile yazılır; HTML olarak yorumlanmaz.
function chartItem(chart) {
  const item = document.createElement("li");

  const name = document.createElement("strong");
  name.textContent = chart.name;

  const meta = document.createElement("span");
  const typeLabel = CHART_TYPE_LABELS[chart.type] || chart.type;
  meta.textContent = chart.subgroup_size > 1
    ? typeLabel + ", alt grup büyüklüğü: " + chart.subgroup_size
    : typeLabel;

  item.append(name, meta);
  return item;
}

async function loadCharts() {
  const status = $("charts-status");
  const list = $("charts-list");

  list.replaceChildren();
  status.hidden = false;
  status.className = "";
  status.textContent = "Grafikler yükleniyor...";

  try {
    const charts = await api("/charts");
    if (charts.length === 0) {
      status.textContent = "Henüz kontrol grafiği yok.";
      return;
    }
    status.hidden = true;
    for (const chart of charts) {
      list.append(chartItem(chart));
    }
  } catch (error) {
    if (!session.token) {
      return; // oturum sonlandı, giriş ekranı zaten gösteriliyor
    }
    status.className = "error";
    status.textContent = error.message;
  }
}

$("login-form").addEventListener("submit", async (event) => {
  event.preventDefault();

  const button = $("login-button");
  button.disabled = true;
  button.textContent = "Giriş yapılıyor...";

  try {
    const data = await api("/auth/login", {
      method: "POST",
      body: { email: $("email").value, password: $("password").value },
    });
    saveSession(data.access_token, data.user);
    showApp();
  } catch (error) {
    const message = $("login-error");
    message.textContent = error.message;
    message.hidden = false;
  } finally {
    button.disabled = false;
    button.textContent = "Giriş yap";
  }
});

$("logout-button").addEventListener("click", () => endSession(""));
$("refresh-button").addEventListener("click", loadCharts);

if (loadSession()) {
  showApp();
} else {
  showLogin("");
}