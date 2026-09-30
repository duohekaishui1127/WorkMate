"use strict";
const $ = (id) => document.getElementById(id),
  id = decodeURIComponent(location.pathname.split("/").pop()),
  key = "workmate-order-" + id;
let token = location.hash.slice(1) || sessionStorage.getItem(key) || "",
  order = null,
  cfg = null,
  busy = false,
  editing = false,
  previewURL = "",
  noticeTimer;
if (location.hash) {
  sessionStorage.setItem(key, token);
  history.replaceState(null, "", location.pathname);
}
function notice(text, error = false) {
  $("notice").textContent = text;
  $("notice").className = error ? "error" : "";
  $("notice").hidden = false;
  clearTimeout(noticeTimer);
  noticeTimer = setTimeout(() => ($("notice").hidden = true), 6000);
}
async function api(path, opts = {}) {
  const headers = new Headers(opts.headers || {});
  headers.set("Authorization", "Bearer " + token);
  if (opts.body && !(opts.body instanceof Blob))
    headers.set("Content-Type", "application/json");
  const r = await fetch(path, { ...opts, headers, credentials: "omit" });
  const body = await r.json();
  if (!r.ok) throw Error(body.error || "读取订单失败");
  return body;
}
const path = "/api/orders/" + encodeURIComponent(id);
function updateEvidence() {
  const file = $("evidence").files[0];
  $("evidence-status").textContent = file
    ? "已选择：" + file.name + "，点击下方按钮提交。"
    : order?.has_evidence
      ? "付款截图已保存，可以直接提交补充信息；需要更换时再选一张。"
      : "";
}
function resetPreview() {
  if (previewURL) URL.revokeObjectURL(previewURL);
  previewURL = "";
  $("evidence-preview").hidden = true;
  $("evidence-preview").removeAttribute("src");
}
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
      created: "等待付款",
      pending: "等待开通",
      approved: "已开通 Pro",
      rejected: "需要补充信息",
    }[order.status] || order.status;
  const pending = order.status === "pending",
    approved = order.status === "approved";
  if (!pending) editing = false;
  $("approved").hidden = !approved;
  $("pending").hidden = !pending || editing;
  $("submission").hidden = approved || (pending && !editing);
  $("payment").hidden = order.status !== "created";
  $("cancel-edit").hidden = !pending || !editing;
  $("submission-title").textContent =
    order.status === "created" ? "2. 上传付款截图" : "补充付款信息";
  $("submit-payment").textContent = busy
    ? "正在提交…"
    : order.status === "created"
      ? "我已付款，提交核实"
      : "提交补充信息";
  $("review-note").hidden = order.status !== "rejected";
  $("review-note").textContent =
    order.admin_note || "请补充付款信息后重新提交，无需再次付款。";
  $("license-code").value = order.license_code || "";
  updateEvidence();
  if (cfg) {
    const available = cfg.enabled && cfg.qr_available;
    $("qr").hidden = !available;
    $("payment-unavailable").hidden = available;
    if (available && !$("qr").getAttribute("src"))
      $("qr").src = "/api/payment-qr";
    $("developer-contact").textContent = cfg.contact
      ? "需要帮助？联系开发者：" + cfg.contact
      : "";
  }
  if (initial) {
    $("customer-contact").value = order.contact || "";
    $("payment-method").value =
      order.payment_method === "other" ? "" : order.payment_method || "";
    $("payment-reference").value = order.payment_reference || "";
    $("customer-note").value = order.customer_note || "";
  }
}
async function refresh() {
  if (busy) return;
  if (!token)
    throw Error("缺少订单访问凭证，请从 WorkMate 购买窗口重新打开订单。");
  const latest = await api(path);
  if (busy || (order && latest.revision < order.revision)) return;
  order = latest;
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
$("evidence").addEventListener("change", () => {
  resetPreview();
  const file = $("evidence").files[0];
  if (
    file &&
    ["image/png", "image/jpeg"].includes(file.type) &&
    file.size <= 4 * 1024 * 1024
  ) {
    previewURL = URL.createObjectURL(file);
    $("evidence-preview").src = previewURL;
    $("evidence-preview").hidden = false;
  }
  updateEvidence();
});
$("submit-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  if (busy || !order) return;
  busy = true;
  const button = $("submit-payment");
  button.disabled = true;
  $("cancel-edit").disabled = true;
  button.textContent = "正在提交…";
  try {
    const file = $("evidence").files[0],
      reference = $("payment-reference").value.trim();
    if (!file && !order.has_evidence && reference.length < 3)
      throw Error(
        "请选择付款截图；没有截图时，也可以展开可选信息填写交易单号。",
      );
    if (file) {
      if (file.size > 4 * 1024 * 1024)
        throw Error("图片不能超过 4 MB，请换一张截图。");
      await api(path + "/evidence", { method: "POST", body: file });
      order = await api(path);
      $("evidence").value = "";
      resetPreview();
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
    editing = false;
    notice("已提交，等待开通。无需重复付款。");
  } catch (error) {
    notice(error.message, true);
  } finally {
    busy = false;
    button.disabled = false;
    $("cancel-edit").disabled = false;
    render();
  }
});
$("edit-submission").addEventListener("click", () => {
  editing = true;
  render();
});
$("cancel-edit").addEventListener("click", () => {
  editing = false;
  render();
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
