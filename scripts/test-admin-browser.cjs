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
        const r = await fetch(base + "/healthz");
        if (r.ok) {
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
        viewport: { width: 520, height: 900 },
      }),
      page = await owner.newPage(),
      purchase = await buyer.newPage();
    const errors = [];
    page.on("pageerror", (e) => errors.push(e.message));
    purchase.on("pageerror", (e) => errors.push(e.message));
    await page.goto(base + "/admin");
    await page.locator('[name="password"]').fill(password);
    await page.locator("#login-form button").click();
    await page.locator("#app").waitFor({ state: "visible" });
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
    const ticket = await create("a".repeat(64));
    await purchase.goto(base + ticket.purchase_path);
    await purchase.locator("#order").waitFor({ state: "visible" });
    assert.equal(new URL(purchase.url()).hash, "");
    await purchase
      .locator("#customer-contact")
      .fill("测试用户 · buyer@example.com");
    await purchase.locator("#payment-reference").fill("USER-20260929-0001");
    await purchase
      .locator("#evidence")
      .setInputFiles({
        name: "payment.png",
        mimeType: "image/png",
        buffer: png(),
      });
    await purchase.locator("#submit-form button").click();
    await purchase
      .locator("#order-status")
      .filter({ hasText: "待人工核实" })
      .waitFor();
    await page.locator('[data-tab="orders"]').click();
    await page.getByRole("button", { name: "核实付款", exact: true }).click();
    await page.locator("#proof").waitFor({ state: "visible" });
    await page.locator("#decision-note").fill("请补充付款时间");
    await page.locator("#reject").click();
    await page.locator("#detail").waitFor({ state: "hidden" });
    await purchase.locator("#refresh").click();
    await purchase
      .locator("#review-note")
      .filter({ hasText: "请补充付款时间" })
      .waitFor();
    await purchase.locator("#customer-note").fill("微信付款，已补充付款时间。");
    await purchase.locator("#submit-form button").click();
    await purchase
      .locator("#order-status")
      .filter({ hasText: "待人工核实" })
      .waitFor();
    await page.locator("#refresh").click();
    await page.getByRole("button", { name: "核实付款", exact: true }).click();
    await page.locator("#received").fill("19.80");
    await page.locator("#receipt").fill("OWNER-ACTUAL-20260929-001");
    await page.locator("#confirmed").check();
    await page.locator("#approve").click();
    await page.waitForFunction(() =>
      document.querySelector("#notice").textContent.includes("实际到账"),
    );
    assert.equal(await page.locator("#detail").evaluate((e) => e.open), true);
    await page.locator("#received").fill("19.90");
    await page.locator("#approve").click();
    await page.locator("#detail").waitFor({ state: "hidden" });
    await purchase.locator("#refresh").click();
    await purchase.locator("#approved").waitFor({ state: "visible" });
    assert.ok(
      (await purchase.locator("#license-code").inputValue()).includes("."),
    );
    const second = await create("b".repeat(64));
    await purchase.goto(base + second.purchase_path);
    await purchase.locator("#order").waitFor({ state: "visible" });
    await purchase.locator("#customer-contact").fill("另一位测试用户");
    await purchase.locator("#payment-reference").fill("USER-SECOND");
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
    await purchase.locator("#submit-form button").click();
    await purchase
      .locator("#order-status")
      .filter({ hasText: "待人工核实" })
      .waitFor();
    await page.locator("#refresh").click();
    await page.getByRole("button", { name: "核实付款", exact: true }).click();
    await page
      .locator("#proof-warning")
      .filter({ hasText: "2 个订单" })
      .waitFor();
    assert.equal(await page.evaluate(() => window.bad), undefined);
    await page.locator("#received").fill("19.90");
    await page.locator("#receipt").fill("OWNER-ACTUAL-20260929-001");
    await page.locator("#confirmed").check();
    await page.locator("#approve").click();
    await page.waitForFunction(() =>
      document
        .querySelector("#notice")
        .textContent.includes("已经用于其他订单"),
    );
    await page.locator("#close-detail").click();
    await page.locator("#status").selectOption("");
    await page.waitForFunction(() =>
      document.querySelector("#total").textContent.includes("共 2 条"),
    );
    await fs.mkdir("dist", { recursive: true });
    await page.locator("#notice").waitFor({ state: "hidden" });
    await page.screenshot({
      path: "dist/workmate-admin-preview.png",
      fullPage: true,
    });
    await purchase.goto(base + ticket.purchase_path);
    await purchase.locator("#approved").waitFor({ state: "visible" });
    await purchase.setViewportSize({ width: 390, height: 844 });
    await purchase.screenshot({
      path: "dist/workmate-purchase-preview.png",
      fullPage: true,
    });
    assert.deepEqual(errors, [], "browser JavaScript errors");
    console.log(
      "PASS: browser login, QR/settings, proof submission, rejection/resubmission, payment amount validation, approval, duplicate receipt, evidence warning, safe text rendering and mobile page.",
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
