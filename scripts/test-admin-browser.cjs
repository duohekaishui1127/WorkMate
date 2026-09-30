// Optional browser regression: install Playwright in a temporary development
// directory and set NODE_PATH. This is not a production dependency.
"use strict";
const { chromium } = require("playwright");
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const os = require("node:os");
const net = require("node:net");
const zlib = require("node:zlib");
const { spawn } = require("node:child_process");
const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
function png() {
  const crc = (b) => {
    let value = 0xffffffff;
    for (const n of b) {
      value ^= n;
      for (let i = 0; i < 8; i++)
        value = (value >>> 1) ^ (value & 1 ? 0xedb88320 : 0);
    }
    return (value ^ 0xffffffff) >>> 0;
  };
  const chunk = (name, data) => {
    const type = Buffer.from(name),
      len = Buffer.alloc(4),
      sum = Buffer.alloc(4);
    len.writeUInt32BE(data.length);
    sum.writeUInt32BE(crc(Buffer.concat([type, data])));
    return Buffer.concat([len, type, data, sum]);
  };
  const head = Buffer.alloc(13);
  head.writeUInt32BE(64, 0);
  head.writeUInt32BE(64, 4);
  head[8] = 8;
  head[9] = 2;
  const pixels = Buffer.alloc(64 * (1 + 64 * 3));
  for (let y = 0; y < 64; y++) {
    for (let x = 0; x < 64; x++) {
      const i = y * 193 + 1 + x * 3;
      pixels[i] = 42;
      pixels[i + 1] = (x + y) % 16 < 8 ? 130 : 225;
      pixels[i + 2] = 110;
    }
  }
  return Buffer.concat([
    Buffer.from("89504e470d0a1a0a", "hex"),
    chunk("IHDR", head),
    chunk("IDAT", zlib.deflateSync(pixels)),
    chunk("IEND", Buffer.alloc(0)),
  ]);
}
async function freePort() {
  const server = net.createServer();
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = server.address().port;
  await new Promise((resolve) => server.close(resolve));
  return port;
}
async function main() {
  const dir = await fs.mkdtemp(path.join(os.tmpdir(), "workmate-browser-")),
    port = await freePort(),
    base = "http://127.0.0.1:" + port;
  const binary =
    process.env.WORKMATE_ADMIN_TEST_BINARY ||
    path.resolve(
      "dist",
      process.platform === "win32" ? "WorkMateAdmin.exe" : "workmate-admin",
    );
  const child = spawn(
    binary,
    ["-listen", "127.0.0.1:" + port, "-data-dir", dir],
    { stdio: "ignore" },
  );
  let browser;
  try {
    let ready = false;
    for (let i = 0; i < 100; i++) {
      try {
        if ((await fetch(base + "/healthz")).ok) {
          ready = true;
          break;
        }
      } catch {}
      await delay(100);
    }
    assert.ok(ready, "test server did not start");
    const password = (
      await fs.readFile(path.join(dir, "initial-password.txt"), "utf8")
    ).trim();
    browser = await chromium.launch({ headless: true });
    const owner = await browser.newContext({
        viewport: { width: 1360, height: 980 },
      }),
      buyer = await browser.newContext({
        viewport: { width: 390, height: 844 },
      }),
      page = await owner.newPage(),
      purchase = await buyer.newPage(),
      errors = [];
    page.on("pageerror", (e) => errors.push(e.message));
    purchase.on("pageerror", (e) => errors.push(e.message));
    await fs.mkdir("dist", { recursive: true });
    await page.goto(base + "/admin");
    await page.locator('[name="password"]').fill(password);
    await page.locator("#login-form button").click();
    await page.locator("#app").waitFor({ state: "visible" });
    await page.waitForFunction(() => document.querySelector("#overview-stats").children.length > 0);
    assert.match(await page.locator("#overview-stats").innerText(), /今日首次启动/);
    const observedDevice = "d".repeat(64);
    const observed = await fetch(base + "/api/telemetry", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ device_id: observedDevice, version: "0.8.0", trial_started_at: new Date().toISOString(), pro: false }),
    });
    assert.equal(observed.status, 204);
    await page.locator("#refresh-overview").click();
    await page.waitForFunction(() => document.querySelector("#user-list").textContent.includes("d".repeat(64)));
    assert.match(await page.locator("#overview-stats").innerText(), /今日活跃\s+1/);
    assert.equal(await page.locator("#active-trend svg").count(), 1);
    await page.setViewportSize({ width: 390, height: 844 });
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth), true, "mobile overview should not overflow");
    await page.screenshot({ path: "dist/workmate-overview-preview.png", fullPage: true });
    await page.setViewportSize({ width: 1360, height: 980 });
    await page.locator('[data-tab="settings"]').click();
    await page
      .locator("#qr-file")
      .setInputFiles({
        name: "test-qr.png",
        mimeType: "image/png",
        buffer: png(),
      });
    await page.locator("#qr-form button").click();
    await page.locator("#qr-status").filter({ hasText: "已配置" }).waitFor();
    await page.locator("#price").fill("19.90");
    await page.locator("#contact").fill("开发者：workmate@example.com");
    await page.locator("#trial-hours").fill("48");
    await page.locator("#enabled").check();
    await page.locator("#settings-form button").click();
    await page.waitForFunction(
      () => document.querySelector("#notice").textContent === "购买设置已保存",
    );
    const cfg = await (await fetch(base + "/api/config")).json();
    assert.equal(cfg.trial_hours, 48);
    assert.equal(cfg.enabled, true);
    async function create(device) {
      const r = await fetch(base + "/api/orders", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ device_id: device }),
      });
      assert.equal(r.status, 201);
      return r.json();
    }
    async function openBuyer(ticket) {
      await purchase.goto(base + ticket.purchase_path);
      await purchase.locator("#order").waitFor({ state: "visible" });
      assert.equal(new URL(purchase.url()).hash, "");
      await purchase.locator(".plan-details summary").click();
      assert.match(await purchase.locator(".plan-card").first().innerText(), /普通版.*免费/);
      assert.match(await purchase.locator(".plan-card.pro").innerText(), /工时账本/);
      assert.equal(await purchase.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, "feature comparison overflows mobile viewport");
      await purchase.locator(".plan-details summary").click();
    }
    async function pending() {
      await purchase.locator("#pending").waitFor({ state: "visible" });
    }
    async function review() {
      await page.locator("#refresh").click();
      await page
        .getByRole("button", { name: "核实并开通", exact: true })
        .first()
        .click();
      await page.locator("#detail").waitFor({ state: "visible" });
    }
    const ticket = await create("a".repeat(64));
    await openBuyer(ticket);
    assert.equal(
      await purchase.locator("#submit-form input:visible").count(),
      1,
      "only the screenshot picker should be visible by default",
    );
    assert.equal(
      await purchase.locator("#customer-contact").getAttribute("required"),
      null,
    );
    await purchase.locator("#submit-payment").click();
    await purchase
      .locator("#notice")
      .filter({ hasText: "请选择付款截图" })
      .waitFor();
    await purchase.locator("#notice").waitFor({ state: "hidden" });
    await purchase.screenshot({
      path: "dist/workmate-purchase-form-preview.png",
      fullPage: true,
    });
    await purchase
      .locator("#evidence")
      .setInputFiles({
        name: "payment.png",
        mimeType: "image/png",
        buffer: png(),
      });
    await purchase.locator("#evidence-preview").waitFor({ state: "visible" });
    // A failed submit must retain the already uploaded screenshot for one-click retry.
    const submitURL = base + "/api/orders/" + ticket.order.id + "/submit";
    await purchase.route(submitURL, (route) => route.abort());
    await purchase.locator("#submit-payment").click();
    await purchase
      .locator("#evidence-status")
      .filter({ hasText: "付款截图已保存" })
      .waitFor();
    await purchase.waitForFunction(
      () => !document.querySelector("#submit-payment").disabled,
    );
    await purchase.unroute(submitURL);
    await purchase.locator("#submit-payment").click();
    await pending();
    assert.equal(await purchase.locator("#submission").isVisible(), false);
    assert.equal(await purchase.locator("#payment").isVisible(), false);
    const submitted = await (
      await fetch(base + "/api/orders/" + ticket.order.id, {
        headers: { Authorization: "Bearer " + ticket.token },
      })
    ).json();
    assert.equal(submitted.contact, "");
    assert.equal(submitted.payment_method, "other");
    assert.equal(submitted.status, "pending");
    assert.equal(submitted.license_code, undefined);
    await purchase.locator("#edit-submission").click();
    await purchase.locator("#extra-information summary").click();
    await purchase.locator("#customer-note").fill("付款时间补充草稿");
    await purchase.locator("#refresh").click();
    assert.equal(
      await purchase.locator("#customer-note").inputValue(),
      "付款时间补充草稿",
      "polling must not erase form drafts",
    );
    await purchase.locator("#cancel-edit").click();
    await pending();
    await page.locator('[data-tab="orders"]').click();
    await review();
    await page.locator("#proof").waitFor({ state: "visible" });
    assert.equal(await page.locator("#received").inputValue(), "19.90");
    assert.equal(
      await page.locator("#decision-form input:visible").count(),
      1,
      "receipt should be the only visible review input",
    );
    assert.equal(await page.locator("#approve").isDisabled(), true);
    await page.locator("#reject-details summary").click();
    await page.locator("#reject-reason").selectOption("unclear");
    await page.locator("#reject").click();
    await page.locator("#detail").waitFor({ state: "hidden" });
    await purchase.locator("#refresh").click();
    await purchase
      .locator("#review-note")
      .filter({ hasText: "付款截图不清晰" })
      .waitFor();
    assert.equal(
      await purchase.locator("#payment").isVisible(),
      false,
      "rejected buyers must not be prompted to pay again",
    );
    await purchase
      .locator("#customer-note")
      .fill("已补充付款时间。保留原截图重新提交。");
    await purchase.locator("#submit-payment").click();
    await pending();
    await review();
    await page.locator("#receipt").fill("OWNER-ACTUAL-20260929-001");
    await page.locator("#amount-details summary").click();
    await page.locator("#received").fill("19.80");
    await page.locator("#approve").click();
    await page
      .locator("#notice")
      .filter({ hasText: "实际到账金额与订单不一致" })
      .waitFor();
    assert.equal(await page.locator("#detail").evaluate((e) => e.open), true);
    await page.locator("#received").fill("19.90");
    await page.locator("#amount-details summary").click();
    await page.locator("#notice").waitFor({ state: "hidden" });
    await page.screenshot({
      path: "dist/workmate-review-preview.png",
      fullPage: true,
    });
    await page.locator("#receipt").press("Enter");
    await page.locator("#detail").waitFor({ state: "hidden" });
    // Verify automatic status polling without clicking Refresh.
    await purchase
      .locator("#approved")
      .waitFor({ state: "visible", timeout: 16000 });
    assert.ok(
      (await purchase.locator("#license-code").inputValue()).includes("."),
    );
    const second = await create("b".repeat(64));
    await openBuyer(second);
    await purchase.locator("#extra-information summary").click();
    await purchase
      .locator("#customer-note")
      .fill('<img src=x onerror="window.bad=true">');
    await purchase
      .locator("#evidence")
      .setInputFiles({
        name: "same-payment.png",
        mimeType: "image/png",
        buffer: png(),
      });
    await purchase.locator("#submit-payment").click();
    await pending();
    await review();
    await page
      .locator("#proof-warning")
      .filter({ hasText: "2 个订单" })
      .waitFor();
    assert.equal(await page.evaluate(() => window.bad), undefined);
    await page.locator("#receipt").fill("OWNER-ACTUAL-20260929-001");
    await page.locator("#approve").click();
    await page
      .locator("#notice")
      .filter({ hasText: "已经用于其他订单" })
      .waitFor();
    await page.locator("#reject-details summary").click();
    await page.locator("#reject-reason").selectOption("other");
    await page.locator("#reject").click();
    await page
      .locator("#notice")
      .filter({ hasText: "请填写需要用户补充" })
      .waitFor();
    await page
      .locator("#decision-note")
      .fill("请补充实际付款时间，无需重复付款。");
    await page.locator("#reject").click();
    await page.locator("#detail").waitFor({ state: "hidden" });
    const third = await create("c".repeat(64));
    await openBuyer(third);
    await purchase.locator("#extra-information summary").click();
    await purchase.locator("#payment-reference").fill("USER-REFERENCE-ONLY");
    await purchase.locator("#submit-payment").click();
    await pending();
    await review();
    await page.locator("#no-proof").waitFor({ state: "visible" });
    assert.ok(
      (await page.locator("#payment-detail").textContent()).includes(
        "USER-REFERENCE-ONLY",
      ),
    );
    await page.setViewportSize({ width: 390, height: 844 });
    assert.equal(
      await page.evaluate(
        () =>
          document.documentElement.scrollWidth <=
          document.documentElement.clientWidth,
      ),
      true,
      "mobile review should not overflow",
    );
    await page.locator("#receipt").fill("OWNER-REFERENCE-ONLY");
    await page.locator("#approve").click();
    await page.locator("#detail").waitFor({ state: "hidden" });
    await purchase.locator("#refresh").click();
    await purchase.locator("#approved").waitFor({ state: "visible" });
    await page.setViewportSize({ width: 1360, height: 980 });
    await page.locator("#status").selectOption("");
    await page.waitForFunction(() =>
      document.querySelector("#total").textContent.includes("共 3 条"),
    );
    await page.locator("#notice").waitFor({ state: "hidden" });
    await page.screenshot({
      path: "dist/workmate-admin-preview.png",
      fullPage: true,
    });
    await purchase.screenshot({
      path: "dist/workmate-purchase-preview.png",
      fullPage: true,
    });
    assert.deepEqual(errors, [], "browser JavaScript errors");
    console.log(
      "PASS: screenshot-only checkout, upload retry, optional fields, clear pending state, draft preservation, quick/custom rejection, prefilled amount, single-field review, keyboard approval, automatic status, duplicate receipt/evidence protection, transaction-only alternative, safe text rendering and mobile layout.",
    );
  } finally {
    if (browser) await browser.close();
    child.kill("SIGINT");
    await Promise.race([
      new Promise((resolve) => child.once("exit", resolve)),
      delay(5000),
    ]);
    await fs.rm(dir, { recursive: true, force: true });
  }
}
main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
