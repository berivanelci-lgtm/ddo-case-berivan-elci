"use strict";

// Ek modül ekranları: iş emirleri, OEE panosu, kalite formları, uygunsuzluklar (NCR), izlenebilirlik.
// app.js'teki api(), session, $ ve setMessage yardımcılarını kullanır. Sunucudan gelen metinler
// her zaman textContent ile yazılır. Rol kontrolleri yalnızca görünümü kolaylaştırır; yetki sunucudadır.

const WO_STATUS_LABELS = {
  planned: "Planlandı", released: "Serbest bırakıldı", in_progress: "Üretimde",
  completed: "Tamamlandı", cancelled: "İptal edildi",
};
const NCR_STATUS_LABELS = { open: "Açık", investigating: "İncelemede", closed: "Kapalı" };
const SEVERITY_LABELS = { minor: "Düşük", major: "Önemli", critical: "Kritik" };
const KIND_LABELS = { raw: "Hammadde", intermediate: "Ara ürün", serial: "Seri no" };

let moduleTimer = null;
let qualityForms = [];

function stopModuleRefresh() {
  if (moduleTimer) {
    clearInterval(moduleTimer);
    moduleTimer = null;
  }
}

function startModuleRefresh(fn, ms) {
  stopModuleRefresh();
  moduleTimer = setInterval(() => {
    if (session.token && !document.hidden) {
      fn();
    }
  }, ms);
}

function hasRole(...roles) {
  return Boolean(session.user) && roles.includes(session.user.role);
}

// h, güvenli biçimde DOM düğümü üretir: metinler textContent ile, olaylar addEventListener ile eklenir.
function h(tag, props, ...children) {
  const node = document.createElement(tag);
  for (const [key, value] of Object.entries(props || {})) {
    if (value === undefined || value === null || value === false) {
      continue;
    }
    if (key === "text") {
      node.textContent = value;
    } else if (key === "class") {
      node.className = value;
    } else if (key.startsWith("on")) {
      node.addEventListener(key.slice(2), value);
    } else {
      node.setAttribute(key, value === true ? "" : String(value));
    }
  }
  for (const child of children) {
    if (child) {
      node.append(child);
    }
  }
  return node;
}

function fmt(value, digits) {
  return Number(value).toLocaleString("tr-TR", { maximumFractionDigits: digits === undefined ? 2 : digits });
}

function pct(value) {
  return fmt(value * 100, 1) + " %";
}

function dateTime(value) {
  return value ? new Date(value).toLocaleString("tr-TR") : "-";
}

function card(title, ...lines) {
  const item = h("li", {}, h("strong", { text: title }));
  for (const line of lines) {
    if (typeof line === "string") {
      item.append(h("span", { text: line }));
    } else if (line) {
      item.append(line);
    }
  }
  return item;
}

function toNumber(text) {
  const clean = String(text).trim().replace(",", ".");
  return clean === "" ? NaN : Number(clean);
}

async function guarded(messageId, fn) {
  try {
    await fn();
  } catch (error) {
    if (session.token) {
      setMessage(messageId, error.message, "error");
    }
  }
}

// --- İş emirleri ---

async function showWorkOrders() {
  setViews("work-orders-view");
  setMessage("wo-message", "");
  const canCreate = hasRole("uretim_planlama", "fabrika_muduru");
  $("wo-create").hidden = !canCreate;
  if (canCreate) {
    await loadProductOptions();
  }
  await loadWorkOrders();
}

async function loadProductOptions() {
  await guarded("wo-message", async () => {
    const products = await api("/products");
    const select = $("wo-product");
    select.replaceChildren(...products.map((p) => h("option", { value: p.id, text: p.code + " - " + p.name })));
    if (products.length === 0) {
      select.append(h("option", { value: "", text: "Önce ürün tanımlanmalı" }));
    }
  });
}

async function loadWorkOrders() {
  await guarded("wo-message", async () => {
    const page = await api("/work-orders?limit=20");
    $("wo-list").replaceChildren(...page.items.map(workOrderItem));
    $("wo-empty").hidden = page.items.length > 0;
  });
}

function countInput(placeholder) {
  return h("input", { type: "number", min: "0", step: "1", placeholder, "aria-label": placeholder, class: "small-input" });
}

function countValue(input) {
  const value = toNumber(input.value || "0");
  return Number.isInteger(value) && value >= 0 ? value : -1;
}

function workOrderItem(order) {
  const item = card(
    "İş emri #" + order.id + " · " + (WO_STATUS_LABELS[order.status] || order.status),
    "Ürün #" + order.product_id + " · plan " + order.planned_qty + " · başlayan " + order.started_qty +
      " · sağlam " + order.good_qty + " · fire " + order.scrap_qty + " · yeniden işlemde " + order.rework_pending
  );
  const actions = h("div", { class: "actions" });
  const button = (text, action, body, secondary) => actions.append(h("button", {
    type: "button", class: secondary ? "secondary small" : "small", text,
    onclick: () => workOrderAction(order.id, action, body),
  }));

  if (order.status === "planned" && hasRole("uretim_planlama", "hat_sefi", "fabrika_muduru")) {
    button("Serbest bırak", "release");
  }
  if (order.status === "planned" && hasRole("uretim_planlama", "fabrika_muduru")) {
    button("İptal et", "cancel", undefined, true);
  }
  if ((order.status === "released" || order.status === "in_progress") && hasRole("operator", "hat_sefi")) {
    const good = countInput("Sağlam");
    const scrap = countInput("Fire");
    const rework = countInput("Yeniden işlem");
    actions.append(h("form", {
      class: "inline-form",
      onsubmit: (event) => {
        event.preventDefault();
        workOrderAction(order.id, "production", { good: countValue(good), scrap: countValue(scrap), rework: countValue(rework) });
      },
    }, good, scrap, rework, h("button", { type: "submit", class: "small", text: "Üretim kaydet" })));
  }
  if (order.status === "in_progress" && order.rework_pending > 0 && hasRole("operator", "hat_sefi", "kalite_muhendisi")) {
    const good = countInput("Sağlam");
    const scrap = countInput("Fire");
    actions.append(h("form", {
      class: "inline-form",
      onsubmit: (event) => {
        event.preventDefault();
        workOrderAction(order.id, "rework", { good: countValue(good), scrap: countValue(scrap) });
      },
    }, good, scrap, h("button", { type: "submit", class: "small secondary", text: "Yeniden işlem sonucu" })));
  }
  if (order.status === "in_progress" && hasRole("hat_sefi", "fabrika_muduru")) {
    button("Kapat", "close", { allow_partial: false });
    button("Kısmi kapat", "close", { allow_partial: true }, true);
  }

  if (actions.childNodes.length > 0) {
    item.append(actions);
  }
  return item;
}

async function workOrderAction(id, action, body) {
  setMessage("wo-message", "");
  await guarded("wo-message", async () => {
    const order = await api("/work-orders/" + id + "/" + action, { method: "POST", body });
    setMessage("wo-message", "İş emri #" + order.id + ": " + (WO_STATUS_LABELS[order.status] || order.status) + ".", "success");
    await loadWorkOrders();
  });
}

$("wo-create").addEventListener("submit", async (event) => {
  event.preventDefault();
  await guarded("wo-message", async () => {
    const order = await api("/work-orders", {
      method: "POST",
      body: { product_id: Number($("wo-product").value), planned_qty: toNumber($("wo-qty").value) },
    });
    setMessage("wo-message", "İş emri #" + order.id + " oluşturuldu.", "success");
    $("wo-qty").value = "";
    await loadWorkOrders();
  });
});

$("wo-refresh").addEventListener("click", loadWorkOrders);

// --- OEE panosu (F-6) ---

async function showOEE() {
  setViews("oee-view");
  setMessage("oee-message", "");
  if (!$("oee-date").value) {
    $("oee-date").value = new Date().toISOString().slice(0, 10);
  }
  await guarded("oee-message", async () => {
    const lines = await api("/lines");
    const select = $("oee-line");
    const previous = select.value;
    select.replaceChildren(...lines.map((line) => h("option", { value: line.code, text: line.code + " - " + line.name })));
    if (lines.length === 0) {
      setMessage("oee-message", "Henüz hat tanımlı değil.", "");
      return;
    }
    const simulated = lines.find((line) => line.code.startsWith("SIM-HAT"));
    if (previous && lines.some((line) => line.code === previous)) {
      select.value = previous;
    } else {
      select.value = simulated ? simulated.code : lines[0].code;
    }
    await loadOEE();
  });
  startModuleRefresh(loadOEE, 10000);
}

async function loadOEE() {
  const code = $("oee-line").value;
  const date = $("oee-date").value;
  if (!code || !date) {
    return;
  }
  await guarded("oee-message", async () => {
    const report = await api("/lines/" + encodeURIComponent(code) + "/oee?date=" + encodeURIComponent(date));
    const rows = report.shifts.map((shift) => ["Vardiya " + shift.shift, shift]);
    rows.push(["Gün toplamı", report.day]);
    $("oee-cards").replaceChildren(...rows.map(([name, result]) => oeeCard(name, result)));
    $("oee-updated").textContent = "Son güncelleme " + new Date().toLocaleTimeString("tr-TR") +
      ". Pano 10 saniyede bir yenilenir. Vardiyalar UTC: A 06-14, B 14-22, C 22-06.";
  });
}

function oeeCard(name, result) {
  const row = (label, value) => [h("dt", { text: label }), h("dd", { text: value })];
  return h("div", { class: "oee-card" },
    h("div", { class: "oee-head" }, h("strong", { text: name }), h("span", { class: "oee-value", text: pct(result.oee) })),
    h("dl", {},
      ...row("Kullanılabilirlik", pct(result.availability)),
      ...row("Performans", pct(result.performance)),
      ...row("Kalite", pct(result.quality)),
      ...row("Planlı duruş", fmt(result.planned_downtime_min, 1) + " dk"),
      ...row("Plansız duruş", fmt(result.unplanned_downtime_min, 1) + " dk"),
      ...row("Sağlam / fire", result.good + " / " + result.scrap)));
}

$("oee-line").addEventListener("change", loadOEE);
$("oee-date").addEventListener("change", loadOEE);

// --- Kalite formları (F-3) ---

async function showQuality() {
  setViews("quality-view");
  setMessage("qf-message", "");
  $("qf-define").hidden = !hasRole("kalite_muhendisi");
  $("qf-entry").hidden = true;
  $("qf-result").replaceChildren();
  $("qf-history").replaceChildren();
  await guarded("qf-message", async () => {
    qualityForms = await api("/quality-forms");
    $("qf-select").replaceChildren(
      h("option", { value: "", text: "Form seçin" }),
      ...qualityForms.map((form) => h("option", { value: form.id, text: form.code + " - " + form.name })));
    $("qf-empty").hidden = qualityForms.length > 0;
  });
}

function selectedForm() {
  return qualityForms.find((form) => String(form.id) === $("qf-select").value);
}

function renderQualityEntry() {
  const form = selectedForm();
  $("qf-entry").hidden = !form;
  $("qf-result").replaceChildren();
  $("qf-history").replaceChildren();
  setMessage("qf-message", "");
  if (!form) {
    return;
  }
  const canEnter = hasRole("operator", "hat_sefi", "kalite_muhendisi");
  $("qf-info").textContent = "Ölçüm cihazı " + (form.gauge_code || "-") +
    ", kalibrasyon bitişi " + (form.calibration_due || "tanımsız") + ".";
  $("qf-items").replaceChildren(...form.items.map((item) => h("label", { class: "qf-item" },
    h("span", {
      text: item.characteristic + " (" + fmt(item.lsl, 3) + " ile " + fmt(item.usl, 3) + " " + item.unit + ")" +
        (item.chart_id ? ", SPC grafiği #" + item.chart_id : ""),
    }),
    h("input", { type: "text", inputmode: "decimal", autocomplete: "off", required: true, disabled: !canEnter }))));
  $("qf-submit").hidden = !canEnter;
  loadInspections(form.id);
}

async function loadInspections(formId) {
  await guarded("qf-message", async () => {
    const inspections = await api("/quality-forms/" + formId + "/inspections");
    $("qf-history").replaceChildren(...inspections.slice(0, 10).map((inspection) => card(
      "Muayene #" + inspection.id + " · " + dateTime(inspection.inspected_at),
      (inspection.all_in_spec ? "Tümü tolerans içinde" : "Tolerans dışı değer var") + " · " +
        inspection.values.map((v) => v.characteristic + " " + fmt(v.value, 3) + (v.in_spec ? "" : " (dışarıda)")).join(", "))));
  });
}

$("qf-select").addEventListener("change", renderQualityEntry);

$("qf-entry").addEventListener("submit", async (event) => {
  event.preventDefault();
  const form = selectedForm();
  if (!form) {
    return;
  }
  const inputs = [...$("qf-items").querySelectorAll("input")];
  const values = inputs.map((input) => toNumber(input.value));
  if (values.some((value) => !Number.isFinite(value))) {
    setMessage("qf-message", "Tüm değerleri sayı olarak girin.", "error");
    return;
  }
  await guarded("qf-message", async () => {
    const result = await api("/quality-forms/" + form.id + "/inspections", { method: "POST", body: { values } });
    setMessage("qf-message",
      result.all_in_spec ? "Muayene kaydedildi, tüm değerler tolerans içinde." : "Muayene kaydedildi, tolerans dışı değer var.",
      result.all_in_spec ? "success" : "error");
    $("qf-result").replaceChildren(
      ...result.values.map((v) => card(v.characteristic + ": " + fmt(v.value, 3), v.in_spec ? "Tolerans içinde" : "TOLERANS DIŞI")),
      ...result.spc.map((s) => card("SPC grafiği #" + s.chart_id,
        s.error ? "SPC'ye yazılamadı: " + s.error
          : "Nokta #" + s.seq + (s.alarms.length > 0 ? ", " + s.alarms.length + " alarm üretildi" : ", alarm yok"))));
    for (const input of inputs) {
      input.value = "";
    }
    await loadInspections(form.id);
  });
});

$("qf-define").addEventListener("submit", async (event) => {
  event.preventDefault();
  const items = [];
  const lines = $("qf-def-items").value.split("\n").map((line) => line.trim()).filter(Boolean);
  for (const line of lines) {
    const parts = line.split(";").map((part) => part.trim());
    const numbers = parts.slice(2, 5).map(toNumber);
    if (parts.length < 5 || numbers.some((n) => !Number.isFinite(n))) {
      setMessage("qf-message", "Her satır: ad;birim;nominal;alt;üst;grafik no (son alan isteğe bağlı).", "error");
      return;
    }
    items.push({
      characteristic: parts[0], unit: parts[1], nominal: numbers[0], lsl: numbers[1], usl: numbers[2],
      chart_id: parts[5] ? Number(parts[5]) : null,
    });
  }
  await guarded("qf-message", async () => {
    const created = await api("/quality-forms", {
      method: "POST",
      body: {
        code: $("qf-def-code").value, name: $("qf-def-name").value, gauge_code: $("qf-def-gauge").value,
        calibration_due: $("qf-def-due").value, items,
      },
    });
    $("qf-define").reset();
    await showQuality();
    $("qf-select").value = String(created.id);
    renderQualityEntry();
    setMessage("qf-message", "Form " + created.code + " kaydedildi.", "success");
  });
});

// --- Uygunsuzluklar (F-5) ---

async function showNCRs() {
  setViews("ncr-view");
  setMessage("ncr-message", "");
  $("ncr-create").hidden = !hasRole("operator", "hat_sefi", "kalite_muhendisi");
  await loadNCRs();
}

async function loadNCRs() {
  const status = $("ncr-filter").value;
  await guarded("ncr-message", async () => {
    const page = await api("/ncrs?limit=30" + (status ? "&status=" + encodeURIComponent(status) : ""));
    $("ncr-list").replaceChildren(...page.items.map(ncrItem));
    $("ncr-empty").hidden = page.items.length > 0;
  });
}

function ncrItem(ncr) {
  const item = card(
    "NCR #" + ncr.id + " · " + ncr.title,
    (SEVERITY_LABELS[ncr.severity] || ncr.severity) + " · " + (NCR_STATUS_LABELS[ncr.status] || ncr.status) +
      " · açılış " + dateTime(ncr.created_at) + (ncr.work_order_id ? " · iş emri #" + ncr.work_order_id : ""));
  if (ncr.description) {
    item.append(h("span", { text: ncr.description }));
  }
  if (ncr.status === "closed") {
    item.append(h("span", { text: "Kök neden: " + ncr.root_cause + " · Düzeltici faaliyet: " + ncr.corrective_action }));
  }
  if (hasRole("kalite_muhendisi") && ncr.status === "open") {
    item.append(h("div", { class: "actions" },
      h("button", { type: "button", class: "small", text: "İncelemeye al", onclick: () => ncrAction(ncr.id, "investigate") })));
  }
  if (hasRole("kalite_muhendisi") && ncr.status === "investigating") {
    const cause = h("input", { type: "text", placeholder: "Kök neden", "aria-label": "Kök neden" });
    const action = h("input", { type: "text", placeholder: "Düzeltici faaliyet", "aria-label": "Düzeltici faaliyet" });
    item.append(h("form", {
      class: "stack-form",
      onsubmit: (event) => {
        event.preventDefault();
        ncrAction(ncr.id, "close", { root_cause: cause.value, corrective_action: action.value });
      },
    }, cause, action, h("button", { type: "submit", class: "small", text: "Kapat" })));
  }
  return item;
}

async function ncrAction(id, action, body) {
  setMessage("ncr-message", "");
  await guarded("ncr-message", async () => {
    const ncr = await api("/ncrs/" + id + "/" + action, { method: "POST", body });
    setMessage("ncr-message", "NCR #" + ncr.id + ": " + (NCR_STATUS_LABELS[ncr.status] || ncr.status) + ".", "success");
    await loadNCRs();
  });
}

$("ncr-create").addEventListener("submit", async (event) => {
  event.preventDefault();
  await guarded("ncr-message", async () => {
    const ncr = await api("/ncrs", {
      method: "POST",
      body: { title: $("ncr-title").value, description: $("ncr-description").value, severity: $("ncr-severity").value },
    });
    $("ncr-create").reset();
    setMessage("ncr-message", "NCR #" + ncr.id + " açıldı.", "success");
    await loadNCRs();
  });
});

$("ncr-filter").addEventListener("change", loadNCRs);

// --- İzlenebilirlik (F-7) ---

function showTrace() {
  setViews("trace-view");
  setMessage("trace-message", "");
}

async function runTrace(kind) {
  const code = $("trace-code").value.trim();
  if (!code) {
    setMessage("trace-message", "Parti ya da seri kodu girin.", "error");
    return;
  }
  setMessage("trace-message", "");
  $("trace-result").replaceChildren();
  await guarded("trace-message", async () => {
    const data = await api("/traceability/lots/" + encodeURIComponent(code) + "/" + kind);
    const lotLabel = data.lot.code + " (" + (KIND_LABELS[data.lot.kind] || data.lot.kind) + ")";
    if (kind === "recall") {
      $("trace-result").replaceChildren(
        h("p", { text: lotLabel + ": " + data.affected_serial_count + " seri, " + data.shipments.length +
          " sevkiyat ve " + data.customers.length + " müşteri etkileniyor." }),
        h("h3", { text: "Müşteriler" }),
        h("p", { class: "muted", text: data.customers.join(", ") || "-" }),
        h("h3", { text: "Sevkiyatlar" }),
        h("ul", { class: "cards" }, ...data.shipments.map((s) => card(s.code + " · " + s.customer,
          dateTime(s.shipped_at) + " · " + s.affected_units + " etkilenen birim"))),
        h("h3", { text: "Etkilenen seri numaraları" }),
        h("p", { class: "muted", text: data.affected_serials.join(", ") || "-" }));
    } else {
      $("trace-result").replaceChildren(
        h("p", { text: lotLabel + ": " + data.lots.length + " parti bulundu (" + (kind === "backward" ? "geri" : "ileri") + " izleme)." }),
        h("ul", { class: "cards" }, ...data.lots.map((lot) => card(lot.code,
          (KIND_LABELS[lot.kind] || lot.kind) + " · seviye " + lot.depth))));
    }
  });
}

$("trace-backward").addEventListener("click", () => runTrace("backward"));
$("trace-forward").addEventListener("click", () => runTrace("forward"));
$("trace-recall").addEventListener("click", () => runTrace("recall"));