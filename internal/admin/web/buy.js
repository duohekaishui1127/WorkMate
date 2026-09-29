"use strict";
const $ = (id) => document.getElementById(id),
  id = decodeURIComponent(location.pathname.split("/").pop()),
  key = "workmate-order-" + id;
let token = location.hash.slice(1) || sessionStorage.getItem(key) || "",
  order = null,
  cfg = null,
  busy = false;
if (location.hash) {
  sessionStorage.setItem(key, token);
  history.replaceState(null, "", location.pathname);
}
function notice(text, error = false) {
  $("notice").textContent = text;
  $("notice").className = error ? "error" : "";
  $("notice").hidden = false;
  setTimeout(() => ($("notice").hidden = true), 6000);
}
async function api(path, opts = {}) {
  const headers = new Headers(opts.headers || {});
  headers.set("Authorization", "Bearer " + token);
  if (opts.body && !(opts.body instanceof Blob))
    headers.set("Content-Type", "application/json");
  const r = await fetch(path, { ...opts, headers, credentials: "omit" }),
    body = await r.json();
  if (!r.ok) throw Error(body.error || "读取订单失败");
  return body;
}
const path = "/api/orders/" + encodeURIComponent(id);
function render(initial = false) {
  $("loading").hidden = true;
  $("order").hidden = false;
  $("order-id").textContent = order.id;
  $("device-id").textContent = order.device_id;
  $("amount").textContent = new Intl.NumberFormat("zh-CN", {
    style: "currency",
    currency: "CNY",
  }).format(order.amount_cents / 100);
  const status = $("order-status");
  status.className = "badge " + order.status;
  status.textContent =
    {
      created: "尚未提交付款",
      pending: "待人工核实",
      approved: "已开通 Pro",
      rejected: "请补充付款信息",
    }[order.status] || order.status;
  $("approved").hidden = order.status !== "approved";
  $("submission").hidden = order.status === "approved";
  $("payment").hidden = order.status === "approved";
  $("review-note").hidden = order.status !== "rejected";
  $("review-note").textContent = order.admin_note || "请补充付款信息后重新提交";
  $("license-code").value = order.license_code || "";
  $("evidence-status").textContent = order.has_evidence
    ? "已保存付款截图；重新上传将替换原截图。"
    : "";
  if (cfg) {
    const available = cfg.enabled && cfg.qr_available;
    $("qr").hidden = !available;
    $("payment-unavailable").hidden = available;
    if (available && !$("qr").getAttribute("src"))
      $("qr").src = "/api/payment-qr";
    $("developer-contact").textContent = cfg.contact
      ? "联系开发者：" + cfg.contact
      : "";
  }
  if (initial) {
    $("customer-contact").value = order.contact || "";
    $("payment-method").value = order.payment_method || "wechat";
    $("payment-reference").value = order.payment_reference || "";
    $("customer-note").value = order.customer_note || "";
  }
}
async function refresh() {
  if (!token)
    throw Error("缺少订单访问凭证，请从 WorkMate 购买窗口重新打开订单。");
  order = await api(path);
  render();
}
(async () => {
  try {
    if (!token) throw Error("请从 WorkMate 的购买窗口打开订单。");
    [order, cfg] = await Promise.all([api(path), api("/api/config")]);
    render(true);
  } catch (e) {
    $("loading").textContent = e.message;
    notice(e.message, true);
  }
})();
$("submit-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  if (busy || !order) return;
  busy = true;
  const button = $("submit-form").querySelector("button");
  button.disabled = true;
  try {
    const file = $("evidence").files[0],
      reference = $("payment-reference").value.trim();
    if (!file && !order.has_evidence && reference.length < 3)
      throw Error("请上传付款截图或填写付款交易单号");
    if (file) {
      if (file.size > 4 * 1024 * 1024) throw Error("图片不能超过 4 MB");
      await api(path + "/evidence", { method: "POST", body: file });
    }
    order = await api(path + "/submit", {
      method: "POST",
      body: JSON.stringify({
        contact: $("customer-contact").value,
        payment_method: $("payment-method").value,
        payment_reference: reference,
        customer_note: $("customer-note").value,
      }),
    });
    $("evidence").value = "";
    render();
    notice("已提交，请等待开发者核实到账。无需重复付款。");
  } catch (error) {
    notice(error.message, true);
  } finally {
    busy = false;
    button.disabled = false;
  }
});
$("refresh").addEventListener("click", () =>
  refresh().catch((e) => notice(e.message, true)),
);
$("copy-license").addEventListener("click", async () => {
  try {
    await navigator.clipboard.writeText($("license-code").value);
    notice("激活码已复制");
  } catch {
    $("license-code").select();
    notice("请按 Ctrl+C 复制激活码");
  }
});
setInterval(() => {
  if (token && order && !busy && !document.hidden) refresh().catch(() => {});
}, 10000);
