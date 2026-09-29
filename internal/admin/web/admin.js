"use strict";
const $ = (id) => document.getElementById(id),
  labels = {
    created: "尚未提交",
    pending: "待核实",
    approved: "已开通 Pro",
    rejected: "已驳回",
  };
let csrf = "",
  tab = "orders",
  offset = 0,
  selected = null,
  total = 0,
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
    contact.textContent = order.contact || "用户尚未提交联系方式";
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
    cell(tr, date(order.created_at));
    const action = cell(tr, "");
    const button = document.createElement("button");
    button.className = "secondary";
    button.textContent = order.status === "pending" ? "核实付款" : "查看";
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
  $("license-box").hidden = order.status !== "approved";
  $("license-code").value = order.license_code || "";
  $("detail").showModal();
}
async function decide(action) {
  if (!selected) return;
  const d = {
    revision: selected.revision,
    confirmed: $("confirmed").checked,
    received_cents: 0,
    receipt_reference: $("receipt").value,
    note: $("decision-note").value,
  };
  if (action === "approve") {
    d.received_cents = cents($("received"));
    if (!d.confirmed || !d.receipt_reference.trim())
      throw Error("请先核实到账并填写实际交易单号");
  } else if (!d.note.trim()) throw Error("请填写驳回原因");
  $("approve").disabled = $("reject").disabled = true;
  try {
    await api(
      "/api/admin/orders/" + encodeURIComponent(selected.id) + "/" + action,
      { method: "POST", body: JSON.stringify(d) },
    );
    $("detail").close();
    notice(
      action === "approve"
        ? "已开通 Pro，用户联网后会自动领取授权"
        : "已驳回，用户可补充信息后重新提交",
    );
    await loadOrders();
  } finally {
    $("approve").disabled = $("reject").disabled = false;
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
async function switchTab(next) {
  tab = next;
  for (const name of ["orders", "settings", "audit"])
    $(name + "-tab").hidden = name !== tab;
  document
    .querySelectorAll("[data-tab]")
    .forEach((b) => b.classList.toggle("active", b.dataset.tab === tab));
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
bindAsync("approve", "click", () => decide("approve"));
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
  if (csrf && tab === "orders" && !$("detail").open && !document.hidden)
    loadOrders().catch((e) => notice(e.message, true));
}, 30000);
