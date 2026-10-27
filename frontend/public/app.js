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

const RULE_LABELS = {
  beyond_3_sigma: "Nokta 3 sigma sınırının dışında",
  same_side_8: "8 ardışık nokta merkezin aynı tarafında",
  trend_6: "6 ardışık nokta sürekli artıyor ya da azalıyor",
  two_of_three_beyond_2_sigma: "3 noktadan 2'si aynı tarafta, 2 sigma dışında",
  four_of_five_beyond_1_sigma: "5 noktadan 4'ü aynı tarafta, 1 sigma dışında",
};

// Bu listeler yalnızca görünümü kolaylaştırır; asıl yetki kontrolü sunucudadır.
const MEASURE_ROLES = ["operator", "hat_sefi", "kalite_muhendisi"];
const RECOMPUTE_ROLES = ["kalite_muhendisi"];

const VIEWS = [
  "login-view", "app-view", "detail-view",
  "work-orders-view", "oee-view", "quality-view", "ncr-view", "trace-view",
];

// Ek ekranlar modules.js içinde tanımlıdır; adları burada eşlenir.
const MODULE_ROUTES = {
  "#/work-orders": "showWorkOrders",
  "#/oee": "showOEE",
  "#/quality": "showQuality",
  "#/ncrs": "showNCRs",
  "#/trace": "showTrace",
};

const BASELINE_SIZE = 25;
const SESSION_KEY = "hatkontrol.session";
const SVG_NS = "http://www.w3.org/2000/svg";

const session = { token: null, user: null };
let currentDetailId = null;
let currentChart = null;
let detailTimer = null;

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

// --- Otomatik yenileme ---

// Grafik ekranı açıkken 5 saniyede bir yenilenir; simülatörün verisi canlı görünür.
function stopDetailRefresh() {
  if (detailTimer) {
    clearInterval(detailTimer);
    detailTimer = null;
  }
}

function stopAllRefresh() {
  stopDetailRefresh();
  if (typeof stopModuleRefresh === "function") {
    stopModuleRefresh();
  }
}

// --- Oturum ---

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
  stopAllRefresh();
  session.token = null;
  session.user = null;
  currentDetailId = null;
  currentChart = null;
  try {
    sessionStorage.removeItem(SESSION_KEY);
  } catch (_) {
    // yoksay
  }
  showLogin(message);
}

// --- Ekran yönetimi ---

function setViews(active) {
  for (const id of VIEWS) {
    $(id).hidden = id !== active;
  }
  const loggedOut = active === "login-view";
  $("session-info").hidden = loggedOut;
  $("main-nav").hidden = loggedOut;
  for (const link of document.querySelectorAll("#main-nav a")) {
    const current = link.dataset.view === active ||
      (active === "detail-view" && link.dataset.view === "app-view");
    link.classList.toggle("active", current);
    if (current) {
      link.setAttribute("aria-current", "page");
    } else {
      link.removeAttribute("aria-current");
    }
  }
}

function setMessage(id, text, kind) {
  const node = $(id);
  node.textContent = text || "";
  node.className = kind || "";
  node.hidden = !text;
}

function showLogin(message) {
  setViews("login-view");
  $("password").value = "";
  setMessage("login-error", message, "error");
  if (location.hash) {
    history.replaceState(null, "", location.pathname);
  }
}

function enterApp() {
  const roleLabel = ROLE_LABELS[session.user.role] || session.user.role;
  $("user-name").textContent = session.user.full_name + " (" + roleLabel + ")";
  for (const link of document.querySelectorAll("#main-nav a")) {
    link.hidden = !canSee(link);
  }
  route();
}

// Menü bağlantısının data-roles listesi yoksa herkes görür. Asıl yetki kontrolü sunucudadır.
function canSee(link) {
  const roles = link && link.dataset.roles;
  return !roles || roles.split(" ").includes(session.user.role);
}

function navLinkFor(hash) {
  return [...document.querySelectorAll("#main-nav a")].find((link) => link.getAttribute("href") === hash);
}
function route() {
  if (!session.token) {
    return;
  }
  stopAllRefresh();

  const match = /^#\/charts\/(\d+)$/.exec(location.hash);
  if (match) {
    showDetail(Number(match[1]));
    return;
  }
  const handler = window[MODULE_ROUTES[location.hash]];
    if (typeof handler === "function" && canSee(navLinkFor(location.hash))) {
    handler();
    return;
  }
  showList();
}

// --- Grafik listesi ---

function showList() {
  currentDetailId = null;
  currentChart = null;
  setViews("app-view");
  loadCharts();
}

// Sunucudan gelen metinler textContent ile yazılır; HTML olarak yorumlanmaz.
function chartItem(chart) {
  const item = document.createElement("li");

  const name = document.createElement("strong");
  const link = document.createElement("a");
  link.href = "#/charts/" + chart.id;
  link.textContent = chart.name;
  name.append(link);

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

// --- Grafik detayı ---

function showDetail(id) {
  currentDetailId = id;
  currentChart = null;
  setViews("detail-view");
  $("detail-title").textContent = "Grafik #" + id;
  $("detail-meta").textContent = "";
  $("detail-content").hidden = true;
  setMessage("measure-message", "");
  loadDetail(id, false);

  stopDetailRefresh();
  detailTimer = setInterval(() => {
    if (currentDetailId === id && session.token && !document.hidden) {
      loadDetail(id, true);
    }
  }, 5000);
}

async function loadDetail(id, silent) {
  const status = $("detail-status");
  if (!silent) {
    status.hidden = false;
    status.className = "";
    status.textContent = "Grafik yükleniyor...";
  }

  try {
    const detail = await api("/charts/" + id);
    if (currentDetailId !== id) {
      return;
    }
    status.hidden = true;
    renderDetail(detail);
  } catch (error) {
    if (currentDetailId !== id || !session.token) {
      return;
    }
    if (silent) {
      setMessage("measure-message", error.message, "error");
      return;
    }
    $("detail-content").hidden = true;
    status.hidden = false;
    status.className = "error";
    status.textContent = error.status === 404
      ? "Bu kontrol grafiği bulunamadı."
      : error.message;
  }
}

function formatNumber(value) {
  return value.toLocaleString("tr-TR", { maximumFractionDigits: 3 });
}

function renderDetail(detail) {
  const { chart, limits, points, alarms } = detail;
  currentChart = chart;

  $("detail-title").textContent = chart.name;
  const typeLabel = CHART_TYPE_LABELS[chart.type] || chart.type;
  $("detail-meta").textContent = chart.subgroup_size > 1
    ? typeLabel + ", alt grup büyüklüğü: " + chart.subgroup_size
    : typeLabel;
  $("detail-content").hidden = false;

  // Grafik
  const svg = $("spc-chart");
  const alarmSeqs = new Set(alarms.map((alarm) => alarm.point_seq));
  if (points.length === 0) {
    svg.replaceChildren();
    svg.hidden = true;
    $("chart-empty").hidden = false;
  } else {
    svg.hidden = false;
    $("chart-empty").hidden = true;
    svg.setAttribute("aria-label", "Kontrol grafiği, son " + points.length + " nokta");
    drawChart(svg, points, limits, alarmSeqs);
  }

  // Limit bilgisi
  const total = points.length ? points[points.length - 1].seq : 0;
  if (limits) {
    let text = "Limit sürümü " + limits.version + ": merkez " + formatNumber(limits.center) +
      ", UCL " + formatNumber(limits.ucl) + ", LCL " + formatNumber(limits.lcl) + ".";
    if (limits.range) {
      text += " R grafiği: merkez " + formatNumber(limits.range.center) +
        ", UCL " + formatNumber(limits.range.ucl) + ", LCL " + formatNumber(limits.range.lcl) + ".";
    }
    $("limits-info").textContent = text;
  } else {
    $("limits-info").textContent = "Limitler ilk " + BASELINE_SIZE +
      " ölçümden sonra otomatik hesaplanır. Şu an: " + total + "/" + BASELINE_SIZE + ".";
  }

  // Ölçüm formu ve limit butonu (yalnızca ilgili rollerde görünür)
  const role = session.user.role;
  const size = chart.type === "xbar_r" ? chart.subgroup_size : 1;
  $("measure-form").hidden = !MEASURE_ROLES.includes(role);
  $("recompute-button").hidden = !RECOMPUTE_ROLES.includes(role);
  $("measure-label").textContent = size === 1
    ? "Ölçüm değeri"
    : "Alt grup (" + size + " değer, boşluk ya da ; ile ayırın)";

  // Alarm listesi
  const list = $("alarms-list");
  list.replaceChildren();
  $("alarms-empty").hidden = alarms.length > 0;
  for (const alarm of alarms) {
    list.append(alarmItem(alarm));
  }
}

function alarmItem(alarm) {
  const item = document.createElement("li");

  const title = document.createElement("strong");
  title.textContent = "Nokta " + alarm.point_seq + ": " + (RULE_LABELS[alarm.rule] || alarm.rule);

  const meta = document.createElement("span");
  meta.textContent = "Değer " + formatNumber(alarm.value) + ", limit sürümü " + alarm.limit_version;

  item.append(title, meta);
  return item;
}

// --- SVG grafik ---

function el(name, attributes, text) {
  const node = document.createElementNS(SVG_NS, name);
  for (const [key, value] of Object.entries(attributes || {})) {
    node.setAttribute(key, String(value));
  }
  if (text !== undefined) {
    node.textContent = text;
  }
  return node;
}

function drawChart(svg, points, limits, alarmSeqs) {
  const width = 640;
  const height = 300;
  const left = 76;
  const right = 16;
  const top = 16;
  const bottom = 28;

  svg.replaceChildren();
  svg.setAttribute("viewBox", "0 0 " + width + " " + height);

  const statistics = points.map((point) => point.statistic);
  const dataMin = Math.min(...statistics);
  const dataMax = Math.max(...statistics);

  const scaleValues = statistics.slice();
  if (limits) {
    scaleValues.push(limits.ucl, limits.lcl);
  }
  let min = Math.min(...scaleValues);
  let max = Math.max(...scaleValues);
  if (min === max) {
    min -= 1;
    max += 1;
  }
  const margin = (max - min) * 0.08;
  min -= margin;
  max += margin;

  const plotWidth = width - left - right;
  const x = (index) => points.length === 1
    ? left + plotWidth / 2
    : left + (index * plotWidth) / (points.length - 1);
  const y = (value) => top + ((max - value) * (height - top - bottom)) / (max - min);

  const label = (value, text) => svg.append(el("text", {
    x: left - 6,
    y: y(value) + 4,
    "text-anchor": "end",
    class: "axis-text",
  }, text));

  const horizontalLine = (value, className, name) => {
    svg.append(el("line", { x1: left, x2: width - right, y1: y(value), y2: y(value), class: className }));
    label(value, name + " " + formatNumber(value));
  };

  if (limits) {
    horizontalLine(limits.ucl, "line-limit", "UCL");
    horizontalLine(limits.center, "line-center", "CL");
    horizontalLine(limits.lcl, "line-limit", "LCL");
  } else {
    label(dataMax, formatNumber(dataMax));
    if (dataMin !== dataMax) {
      label(dataMin, formatNumber(dataMin));
    }
  }

  svg.append(el("polyline", {
    class: "series",
    points: points
      .map((point, index) => x(index).toFixed(1) + "," + y(point.statistic).toFixed(1))
      .join(" "),
  }));

  points.forEach((point, index) => {
    const isAlarm = alarmSeqs.has(point.seq);
    const dot = el("circle", {
      cx: x(index),
      cy: y(point.statistic),
      r: isAlarm ? 6 : 3.5,
      class: isAlarm ? "dot alarm" : "dot",
    });
    dot.append(el("title", {}, "Nokta " + point.seq + ": " + formatNumber(point.statistic)));
    svg.append(dot);
  });

  svg.append(el("text", { x: left, y: height - 8, class: "axis-text" }, "#" + points[0].seq));
  svg.append(el("text", {
    x: width - right,
    y: height - 8,
    "text-anchor": "end",
    class: "axis-text",
  }, "#" + points[points.length - 1].seq));
}

// --- Ölçüm ve limit işlemleri ---

const NUMBER_PATTERN = /^-?\d+([.,]\d+)?$/;

function parseValues(text) {
  const parts = text.trim().split(/[\s;]+/).filter(Boolean);
  const valid = parts.length > 0 && parts.every((part) => NUMBER_PATTERN.test(part));
  const values = valid ? parts.map((part) => Number(part.replace(",", "."))) : [];
  return { values, valid };
}

$("measure-form").addEventListener("submit", async (event) => {
  event.preventDefault();

  const id = currentDetailId;
  const expected = currentChart && currentChart.type === "xbar_r" ? currentChart.subgroup_size : 1;
  const parsed = parseValues($("measure-values").value);
  if (!parsed.valid || parsed.values.length !== expected) {
    setMessage(
      "measure-message",
      expected === 1 ? "Geçerli bir sayı girin." : expected + " adet geçerli sayı girin.",
      "error"
    );
    return;
  }

  const button = $("measure-button");
  button.disabled = true;

  try {
    const result = await api("/charts/" + id + "/measurements", {
      method: "POST",
      body: { values: parsed.values },
    });
    if (currentDetailId !== id) {
      return;
    }

    let text = "Ölçüm #" + result.seq + " eklendi.";
    if (result.limits_computed) {
      text += " Limitler hesaplandı (sürüm " + result.limit_version + ").";
    }
    if (result.alarms.length > 0) {
      text += " " + result.alarms.length + " alarm üretildi.";
    }
    setMessage("measure-message", text, result.alarms.length > 0 ? "error" : "success");
    $("measure-values").value = "";
    await loadDetail(id, true);
  } catch (error) {
    if (session.token) {
      setMessage("measure-message", error.message, "error");
    }
  } finally {
    button.disabled = false;
  }
});

$("recompute-button").addEventListener("click", async () => {
  const id = currentDetailId;
  const button = $("recompute-button");
  button.disabled = true;

  try {
    const set = await api("/charts/" + id + "/limits/recompute", { method: "POST" });
    if (currentDetailId !== id) {
      return;
    }
    setMessage("measure-message", "Yeni limit sürümü oluşturuldu: " + set.version + ".", "success");
    await loadDetail(id, true);
  } catch (error) {
    if (session.token) {
      setMessage("measure-message", error.message, "error");
    }
  } finally {
    button.disabled = false;
  }
});

// --- Giriş ve çıkış ---

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
    enterApp();
  } catch (error) {
    setMessage("login-error", error.message, "error");
  } finally {
    button.disabled = false;
    button.textContent = "Giriş yap";
  }
});

$("logout-button").addEventListener("click", () => endSession(""));
$("refresh-button").addEventListener("click", loadCharts);
window.addEventListener("hashchange", route);

// modules.js de yüklendikten sonra başla.
window.addEventListener("DOMContentLoaded", () => {
  if (loadSession()) {
    enterApp();
  } else {
    showLogin("");
  }
});