"use strict";
const $ = (id) => document.getElementById(id),
  labels = {
    created: "尚未提交",
    pending: "待核实",
    approved: "已开通 Pro",
    rejected: "已驳回",
  };
let csrf = "",
  tab = "overview",
  offset = 0,
  userOffset = 0,
  userTotal = 0,
  selected = null,
  total = 0,
  decisionBusy = false,
  noticeTimer;
function notice(text, error = false) {
  $("notice").textContent = text;
  $("notice").className = error ? "error" : "";
  $("notice").hidden = false;
  clearTimeout(noticeTimer);
  noticeTimer = setTimeout(() => ($("notice").hidden = true), 6000);
}
function loggedOut() {
  $("app").hidden = true;
  $("login").hidden = false;
  csrf = "";
}
async function api(path, opts = {}) {
  const headers = new Headers(opts.headers || {});
  if (opts.body && !(opts.body instanceof Blob))
    headers.set("Content-Type", "application/json");
  if (opts.method && opts.method !== "GET") headers.set("X-CSRF-Token", csrf);
  const r = await fetch(path, { ...opts, headers, credentials: "same-origin" });
  const body = await r.json();
  if (!r.ok) {
    if (r.status === 401) loggedOut();
    throw Error(body.error || "操作失败，请稍后再试");
  }
  return body;
}
function money(cents) {
  return new Intl.NumberFormat("zh-CN", {
    style: "currency",
    currency: "CNY",
  }).format(cents / 100);
}
function cents(input) {
  const value = input.value.trim();
  if (!/^\d+(?:\.\d{1,2})?$/.test(value)) throw Error("金额最多保留两位小数");
  const [whole, part = ""] = value.split(".");
  return Number(whole) * 100 + Number(part.padEnd(2, "0"));
}
function date(value) {
  return new Date(value).toLocaleString("zh-CN", { hour12: false });
}
function cell(row, text) {
  const td = document.createElement("td");
  td.textContent = text;
  row.append(td);
  return td;
}
async function loadOrders() {
  const p = new URLSearchParams({
    status: $("status").value,
    search: $("search").value,
    offset: String(offset),
  });
  const data = await api("/api/admin/orders?" + p);
  total = data.total;
  $("summary").replaceChildren();
  for (const key of ["pending", "approved", "rejected", "created"]) {
    const item = document.createElement("div");
    item.className = "stat";
    const count = document.createElement("strong");
    count.textContent = data.summary[key];
    const label = document.createElement("span");
    label.textContent = labels[key];
    item.append(count, label);
    $("summary").append(item);
  }
  $("order-list").replaceChildren();
  for (const order of data.orders) {
    const tr = document.createElement("tr");
    const first = cell(tr, order.id);
    const contact = document.createElement("small");
    contact.textContent = order.contact || "未留联系方式（可选）";
    first.append(contact);
    cell(tr, money(order.amount_cents));
    const status = cell(tr, "");
    const badge = document.createElement("span");
    badge.className = "badge " + order.status;
    badge.textContent = labels[order.status];
    status.append(badge);
    const proof = cell(tr, order.payment_reference || "—");
    const extra = document.createElement("small");
    extra.textContent =
      (order.has_evidence ? "有付款截图" : "无截图") +
      (order.evidence_uses > 1 ? " · 凭证被多次使用" : "");
    proof.append(extra);
    cell(
      tr,
      date(order.status === "created" ? order.created_at : order.updated_at),
    );
    const action = cell(tr, "");
    const button = document.createElement("button");
    button.className = "secondary";
    button.textContent = order.status === "pending" ? "核实并开通" : "查看";
    button.addEventListener("click", () => showOrder(order));
    action.append(button);
    $("order-list").append(tr);
  }
  if (!data.orders.length) {
    const tr = document.createElement("tr"),
      td = cell(tr, "暂无符合条件的订单");
    td.colSpan = 6;
    $("order-list").append(tr);
  }
  $("total").textContent =
    `共 ${total} 条 · 第 ${Math.floor(offset / 50) + 1} 页`;
  $("previous").disabled = offset === 0;
  $("next").disabled = offset + 50 >= total;
}
function showOrder(order) {
  selected = order;
  $("review-amount").textContent = money(order.amount_cents);
  $("payment-detail").replaceChildren();
  for (const [name, value] of [
    ["提交时间", date(order.updated_at)],
    ["用户交易单号", order.payment_reference],
    ["联系方式", order.contact],
    ["用户说明", order.customer_note],
  ]) {
    if (!value) continue;
    const dt = document.createElement("dt"),
      dd = document.createElement("dd");
    dt.textContent = name;
    dd.textContent = value;
    $("payment-detail").append(dt, dd);
  }
  $("order-detail").replaceChildren();
  for (const [name, value] of [
    ["订单号", order.id],
    ["设备码", order.device_id],
    ["订单金额", money(order.amount_cents)],
    ["当前状态", labels[order.status]],
    ["联系方式", order.contact || "—"],
    [
      "付款方式",
      { wechat: "微信", alipay: "支付宝", other: "其他" }[
        order.payment_method
      ] || "—",
    ],
    ["用户交易单号", order.payment_reference || "—"],
    ["用户说明", order.customer_note || "—"],
    ["处理备注", order.admin_note || "—"],
    ["到账交易单号", order.receipt_reference || "—"],
  ]) {
    const dt = document.createElement("dt"),
      dd = document.createElement("dd");
    dt.textContent = name;
    dd.textContent = value;
    $("order-detail").append(dt, dd);
  }
  $("proof").hidden = !order.has_evidence;
  $("no-proof").hidden = order.has_evidence;
  if (order.has_evidence) {
    const path =
      "/api/admin/orders/" +
      encodeURIComponent(order.id) +
      "/evidence?v=" +
      order.revision;
    $("proof").src = path;
    $("proof-link").href = path;
  } else {
    $("proof").removeAttribute("src");
    $("proof-link").removeAttribute("href");
  }
  $("proof-warning").hidden = order.evidence_uses < 2;
  $("proof-warning").textContent =
    `这张付款凭证出现在 ${order.evidence_uses} 个订单中，请核实是否重复提交。`;
  $("decision-form").hidden = order.status !== "pending";
  $("decision-form").reset();
  $("decision-fields").disabled = false;
  $("received").value = (order.amount_cents / 100).toFixed(2);
  $("custom-reason").hidden = true;
  for (const name of ["amount-details", "reject-details", "technical-detail"])
    $(name).open = false;
  updateApproveButton();
  $("license-box").hidden = order.status !== "approved";
  $("license-code").value = order.license_code || "";
  if (!$("detail").open) $("detail").showModal();
}
function updateApproveButton() {
  $("approve").disabled = decisionBusy || $("receipt").value.trim().length < 3;
}
const rejectionReasons = {
  missing: "未找到对应收款，请补充付款时间或付款交易单号，无需重复付款。",
  unclear: "付款截图不清晰，请重新上传包含金额、时间和交易信息的完整截图。",
  amount: "付款金额与订单不一致，请联系开发者核对处理，无需重复付款。",
};
async function decide(action) {
  if (!selected || decisionBusy) return;
  const d = {
    revision: selected.revision,
    confirmed: action === "approve",
    received_cents: 0,
    receipt_reference: $("receipt").value.trim(),
    note: "",
  };
  if (action === "approve") {
    if (!$("decision-form").reportValidity()) return;
    d.received_cents = cents($("received"));
    if (d.received_cents !== selected.amount_cents)
      throw Error("实际到账金额与订单不一致，请核对或让用户补充信息。");
    if (d.receipt_reference.length < 3)
      throw Error("请从收款记录复制到账交易单号。");
  } else {
    d.note =
      $("reject-reason").value === "other"
        ? $("decision-note").value.trim()
        : rejectionReasons[$("reject-reason").value];
    if (!d.note) throw Error("请填写需要用户补充的内容。");
  }
  decisionBusy = true;
  $("decision-fields").disabled = true;
  $("close-detail").disabled = $("refresh-detail").disabled = true;
  try {
    await api(
      "/api/admin/orders/" + encodeURIComponent(selected.id) + "/" + action,
      { method: "POST", body: JSON.stringify(d) },
    );
    $("detail").close();
    notice(
      action === "approve"
        ? "已开通 Pro，用户会自动领取授权"
        : "已通知用户补充信息，无需再次付款",
    );
    await loadOrders();
  } finally {
    decisionBusy = false;
    $("decision-fields").disabled = false;
    $("close-detail").disabled = $("refresh-detail").disabled = false;
    updateApproveButton();
  }
}
async function loadSettings() {
  const data = await api("/api/admin/settings"),
    cfg = data.settings;
  $("price").value = (cfg.price_cents / 100).toFixed(2);
  $("contact").value = cfg.contact;
  $("trial-hours").value = cfg.trial_hours;
  $("enabled").checked = cfg.enabled;
  $("qr-status").textContent = cfg.qr_available
    ? "已配置收款码。"
    : "尚未上传收款码，购买入口不能开放。";
  $("qr-preview").hidden = !cfg.qr_available;
  if (cfg.qr_available)
    $("qr-preview").src = "/api/admin/payment-qr?v=" + Date.now();
}
async function loadAudit() {
  const items = await api("/api/admin/audit");
  $("audit-list").replaceChildren();
  const actions = {
    created: "创建订单",
    submitted: "提交付款",
    approve: "确认开通",
    reject: "驳回付款",
    settings: "修改购买设置",
    password_changed: "修改管理员密码",
  };
  for (const item of items) {
    const row = document.createElement("tr");
    cell(row, date(item.at));
    cell(row, actions[item.action] || item.action);
    cell(row, item.order_id || "—");
    cell(row, item.note || "—");
    $("audit-list").append(row);
  }
}
function formatCount(value) {
  return new Intl.NumberFormat("zh-CN").format(value);
}
function formatPercent(value, denominator) {
  return denominator ? value.toFixed(1) + "%" : "—";
}
function renderTrend(points) {
  const root = $("active-trend");
  root.replaceChildren();
  if (!points.length || points.every((point) => point.active === 0)) {
    root.textContent = "尚无已同意匿名统计的活跃设备";
    return;
  }
  const ns = "http://www.w3.org/2000/svg";
  const svg = document.createElementNS(ns, "svg");
  svg.setAttribute("viewBox", "0 0 700 220");
  svg.setAttribute("aria-hidden", "true");
  const max = Math.max(1, ...points.map((point) => point.active));
  const xy = points.map((point, i) => ({
    x: 48 + (i * 620) / Math.max(1, points.length - 1),
    y: 174 - (point.active * 145) / max,
  }));
  for (let i = 0; i <= 4; i++) {
    const y = 174 - (i * 145) / 4;
    const line = document.createElementNS(ns, "line");
    line.setAttribute("x1", "48");
    line.setAttribute("x2", "668");
    line.setAttribute("y1", String(y));
    line.setAttribute("y2", String(y));
    line.setAttribute("class", "chart-grid");
    svg.append(line);
    const tick = document.createElementNS(ns, "text");
    tick.setAttribute("x", "39");
    tick.setAttribute("y", String(y + 4));
    tick.setAttribute("text-anchor", "end");
    tick.textContent = String(Math.round((max * i) / 4));
    svg.append(tick);
  }
  const path = document.createElementNS(ns, "path");
  path.setAttribute("d", xy.map((p, i) => `${i ? "L" : "M"}${p.x},${p.y}`).join(" "));
  path.setAttribute("class", "chart-line");
  svg.append(path);
  points.forEach((point, i) => {
    const dot = document.createElementNS(ns, "circle");
    dot.setAttribute("cx", String(xy[i].x));
    dot.setAttribute("cy", String(xy[i].y));
    dot.setAttribute("r", "4");
    dot.setAttribute("class", "chart-dot");
    const title = document.createElementNS(ns, "title");
    title.textContent = `${point.day}：${point.active} 台活跃，${point.new} 台新增`;
    dot.append(title);
    svg.append(dot);
    if (i === 0 || i === points.length - 1 || i % 3 === 0) {
      const label = document.createElementNS(ns, "text");
      label.setAttribute("x", String(xy[i].x));
      label.setAttribute("y", "205");
      label.setAttribute("text-anchor", "middle");
      label.textContent = point.day.slice(5).replace("-", "/");
      svg.append(label);
    }
  });
  root.append(svg);
}
function renderMiniList(id, items, format) {
  const root = $(id);
  root.replaceChildren();
  if (!items.length) {
    root.textContent = "暂无数据";
    return;
  }
  for (const item of items) {
    const row = document.createElement("div");
    const [label, count] = format(item);
    const name = document.createElement("span");
    name.textContent = label;
    const value = document.createElement("strong");
    value.textContent = formatCount(count);
    row.append(name, value);
    root.append(row);
  }
}
async function loadOverview() {
  const data = await api("/api/admin/analytics");
  const cards = [
    ["今日首次启动", data.today_first_starts, "已同意统计且今日首次使用"],
    ["今日新增记录", data.today_new, "今日首次接入统计的设备"],
    ["今日活跃", data.today_active, "今天至少启动或运行一次"],
    ["累计已知设备", data.total_known_devices, "参与统计或创建过线上订单"],
    ["今日已付款订单", data.today_paid_orders, "已核实到账并开通"],
    ["今日订单金额", money(data.today_revenue_cents), "未扣退款和手续费"],
    ["已知 Pro 设备", data.known_pro_devices, "在线订单或同意统计的离线授权"],
    ["试用 → Pro", formatPercent(data.trial_to_paid_percent, data.trial_devices), `试用设备 ${data.trial_devices} 台`],
    ["激活回执率", formatPercent(data.activation_percent, data.approved_orders), `${data.activated_orders} / ${data.approved_orders} 个新版已开通订单`],
    ["今日下载入口点击", data.download_clicks == null ? "—" : data.download_clicks, "尚未配置可统计的下载入口"],
  ];
  $("overview-stats").replaceChildren();
  for (const [label, value, hint] of cards) {
    const card = document.createElement("div");
    card.className = "stat";
    const name = document.createElement("span");
    name.textContent = label;
    const number = document.createElement("strong");
    number.textContent = typeof value === "number" ? formatCount(value) : value;
    const detail = document.createElement("small");
    detail.textContent = hint;
    card.append(name, number, detail);
    $("overview-stats").append(card);
  }
  renderTrend(data.trend || []);
  renderMiniList("version-flows", data.version_flows || [], (item) => [`${item.from} → ${item.to}`, item.count]);
  renderMiniList("version-errors", data.version_errors || [], (item) => [item.version, item.reports]);
  await loadAnalyticsUsers();
}
async function loadAnalyticsUsers() {
  const data = await api("/api/admin/users?" + new URLSearchParams({
    search: $("user-search").value,
    offset: String(userOffset),
  }));
  userTotal = data.total;
  $("user-list").replaceChildren();
  for (const user of data.users) {
    const row = document.createElement("tr");
    const device = cell(row, user.device_id);
    device.className = "mono device-id";
    cell(row, date(user.first_seen));
    cell(row, date(user.last_seen));
    cell(row, `${user.first_version} → ${user.current_version}`);
    cell(row, user.edition === "pro" ? "Pro" : "普通版");
    cell(row, user.trial_started_at ? date(user.trial_started_at) : "—");
    cell(row, user.pro_activated_at ? date(user.pro_activated_at) : "—");
    cell(row, { received: "已领取", pending: "待客户端领取", offline: "离线授权", none: "无" }[user.auth_status] || "—");
    $("user-list").append(row);
  }
  if (!data.users.length) {
    const row = document.createElement("tr"), td = cell(row, "暂无匿名设备记录");
    td.colSpan = 8;
    $("user-list").append(row);
  }
  $("user-total").textContent = `共 ${userTotal} 台 · 第 ${Math.floor(userOffset / 50) + 1} 页`;
  $("user-previous").disabled = userOffset === 0;
  $("user-next").disabled = userOffset + 50 >= userTotal;
}

async function switchTab(next) {
  tab = next;
  for (const name of ["overview", "orders", "settings", "audit"])
    $(name + "-tab").hidden = name !== tab;
  document
    .querySelectorAll("[data-tab]")
    .forEach((b) => b.classList.toggle("active", b.dataset.tab === tab));
  if (tab === "overview") await loadOverview();
  if (tab === "orders") await loadOrders();
  if (tab === "settings") await loadSettings();
  if (tab === "audit") await loadAudit();
}
function bindAsync(id, event, fn) {
  $(id).addEventListener(event, async (e) => {
    e.preventDefault();
    try {
      await fn(e);
    } catch (error) {
      notice(error.message, true);
    }
  });
}
bindAsync("login-form", "submit", async () => {
  const button = $("login-form").querySelector("button");
  button.disabled = true;
  try {
    const f = new FormData($("login-form")),
      data = await api("/api/admin/login", {
        method: "POST",
        body: JSON.stringify({
          username: f.get("username"),
          password: f.get("password"),
        }),
      });
    csrf = data.csrf;
    $("login-form").reset();
    $("login").hidden = true;
    $("app").hidden = false;
    await switchTab(tab);
  } finally {
    button.disabled = false;
  }
});
bindAsync("logout", "click", async () => {
  await api("/api/admin/logout", { method: "POST" });
  loggedOut();
});
bindAsync("search-form", "submit", async () => {
  offset = 0;
  await loadOrders();
});
bindAsync("refresh", "click", loadOrders);
bindAsync("refresh-overview", "click", loadOverview);
bindAsync("user-search-form", "submit", async () => {
  userOffset = 0;
  await loadAnalyticsUsers();
});
bindAsync("user-previous", "click", async () => {
  userOffset = Math.max(0, userOffset - 50);
  await loadAnalyticsUsers();
});
bindAsync("user-next", "click", async () => {
  userOffset += 50;
  await loadAnalyticsUsers();
});
bindAsync("status", "change", async () => {
  offset = 0;
  await loadOrders();
});
bindAsync("previous", "click", async () => {
  offset = Math.max(0, offset - 50);
  await loadOrders();
});
bindAsync("next", "click", async () => {
  offset += 50;
  await loadOrders();
});
bindAsync("close-detail", "click", async () => $("detail").close());
bindAsync("decision-form", "submit", () => decide("approve"));
$("receipt").addEventListener("input", updateApproveButton);
$("reject-reason").addEventListener("change", () => {
  $("custom-reason").hidden = $("reject-reason").value !== "other";
});
$("detail").addEventListener("cancel", (event) => {
  if (decisionBusy) event.preventDefault();
});
bindAsync("refresh-detail", "click", async () => {
  if (!selected || decisionBusy) return;
  const data = await api(
    "/api/admin/orders?" + new URLSearchParams({ search: selected.id }),
  );
  const latest = data.orders.find((order) => order.id === selected.id);
  if (!latest) throw Error("找不到此订单，请刷新订单列表。");
  showOrder(latest);
});
bindAsync("reject", "click", () => decide("reject"));
bindAsync("settings-form", "submit", async () => {
  if (!Number.isInteger(Number($("trial-hours").value)))
    throw Error("试用小时数必须为整数");
  await api("/api/admin/settings", {
    method: "POST",
    body: JSON.stringify({
      enabled: $("enabled").checked,
      price_cents: cents($("price")),
      contact: $("contact").value,
      trial_hours: Number($("trial-hours").value),
    }),
  });
  notice("购买设置已保存");
  await loadSettings();
});
bindAsync("qr-form", "submit", async () => {
  const file = $("qr-file").files[0];
  if (!file) throw Error("请选择收款码");
  if (file.size > 4 * 1024 * 1024) throw Error("图片不能超过 4 MB");
  await api("/api/admin/payment-qr", { method: "POST", body: file });
  $("qr-form").reset();
  notice("收款码已上传");
  await loadSettings();
});
bindAsync("password-form", "submit", async () => {
  const f = new FormData($("password-form"));
  if (f.get("new") !== f.get("repeat")) throw Error("两次新密码不一致");
  await api("/api/admin/password", {
    method: "POST",
    body: JSON.stringify({
      old_password: f.get("old"),
      new_password: f.get("new"),
    }),
  });
  $("password-form").reset();
  loggedOut();
  notice("密码已修改，请重新登录");
});
bindAsync("copy-license", "click", async () => {
  try {
    await navigator.clipboard.writeText($("license-code").value);
    notice("激活码已复制");
  } catch {
    $("license-code").select();
    notice("请按 Ctrl+C 复制激活码");
  }
});
document
  .querySelectorAll("[data-tab]")
  .forEach((button) =>
    button.addEventListener("click", () =>
      switchTab(button.dataset.tab).catch((e) => notice(e.message, true)),
    ),
  );
(async () => {
  try {
    const data = await api("/api/admin/me");
    csrf = data.csrf;
    $("login").hidden = true;
    $("app").hidden = false;
    await switchTab(tab);
  } catch {
    loggedOut();
  }
})();
setInterval(() => {
  if (!csrf || document.hidden) return;
  if (tab === "orders" && !$("detail").open)
    loadOrders().catch((e) => notice(e.message, true));
  if (tab === "overview")
    loadOverview().catch((e) => notice(e.message, true));
}, 30000);
