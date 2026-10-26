// Run against a local WebUI with Playwright's Firefox installed. All API calls
// are intercepted with synthetic data; this test never logs into a real account.
const { firefox } = require(process.env.PLAYWRIGHT_MODULE || "playwright");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const baseURL = process.env.WEBUI_TEST_URL || "http://127.0.0.1:3100";
const output = path.join(__dirname, "../test-results/responsive");
const user = { uid: 1, username: "LongAdministratorName_abcdefghijklmnopqrstuvwxyz", role: 0, role_name: "Administrator", active: true, emby_id: "emby-123", expired_at: -1, telegram_id: 123 };
const code = { code: "LONG-REGISTRATION-CODE-ABCDEFGHIJKLMNOPQRSTUVWXYZ-123456", type: 1, days: 30, use_count: 1, use_count_limit: 10, active: true, status: "available", source: "admin", note: "A long registration note for narrow screens", created_time: 1790000000, created_at: 1790000000, validity_time: 72, target_username: "VeryLongTargetUsernameWithoutSpacesABCDEFGHIJKLMNOPQRSTUVWXYZ" };
const fixtures = {
  "/auth/me": user,
  "/system/info": { name: "Twilight", icon: "", version: "test", auth_background_url: "/api/v2/system/auth-background", features: { register: true, telegram: true, emby_stats: true, signin: true, media_request: true, invite: true }, limits: {}, setup: { available: false } },
  "/telegram/status": { bound: true },
  "/emby/status": { online: true, server_name: "Long Emby server name", version: "4.9", movie_count: 12345, series_count: 12345, episode_count: 123456 },
  "/system/emby-urls": { lines: [{ name: "A very long line name", url: "https://media.example/" + "longpath".repeat(20) }] },
  "/system/emby-stats": { enabled: true, configured: true, movie_count: 12345, series_count: 12345, episode_count: 123456 },
  "/system/emby-viewers": { viewers: 12345 },
  "/signin/summary": { summary: { enabled: true, current_points: 123456789, currency_name: "Reward points", current_streak: 365, next_bonus_in_days: 2, next_bonus_points: 10 } },
  "/registration/availability": { enabled: true, register_enabled: true, emby_direct_register_enabled: false },
  "/admin/regcodes": { items: [code, { ...code, code: "SECOND-CODE" }], pagination: { total: 2, page: 1, per_page: 20 } },
  "/settings/appearance": { background: "" },
  "/announcements": { announcements: [], total: 0 },
  "/me/announcements": { announcements: [], force_read: [] },
  "/media/requests": { items: [], pagination: { total: 0 } },
};

async function assertFits(page, selector) {
  const overflow = await page.locator(selector).evaluate((root) => {
    return [root, ...root.querySelectorAll("*")].filter((el) => {
      const rect = el.getBoundingClientRect();
      const style = getComputedStyle(el);
      if (!rect.width || !rect.height || style.position === "absolute" || style.position === "fixed") return false;
      // Internal table/tab scrolling and clipped text inputs are intentional.
      for (let p = el.parentElement; p && p !== root.parentElement; p = p.parentElement) {
        if (["auto", "scroll"].includes(getComputedStyle(p).overflowX)) return false;
      }
      const spills = el.clientWidth > 0 && el.scrollWidth > el.clientWidth + 2
        && !["hidden", "clip", "auto", "scroll"].includes(style.overflowX)
        && !["INPUT", "TEXTAREA", "SELECT"].includes(el.tagName);
      return rect.right > document.documentElement.clientWidth + 1 || spills;
    }).map((el) => ({ tag: el.tagName, class: el.className, text: el.textContent.slice(0, 60) }));
  });
  assert.deepEqual(overflow, [], `Overflow in ${selector}: ${JSON.stringify(overflow)}`);
}

async function checkDialog(page) {
  const dialog = page.getByRole("dialog");
  await dialog.waitFor();
  // Allow the shared Radix entrance transition to finish before geometry checks.
  await page.waitForTimeout(250);
  await assertFits(page, '[role="dialog"]');
  const box = await dialog.boundingBox();
  assert.ok(box.y >= -1 && box.y + box.height <= page.viewportSize().height + 1, "Dialog exceeds viewport height");
  await page.keyboard.press("Escape");
  await dialog.waitFor({ state: "hidden" });
}

(async () => {
  fs.mkdirSync(output, { recursive: true });
  const browser = await firefox.launch({ headless: true });
  let cases = 0;
  try {
    for (const locale of ["zh-Hans", "zh-Hant", "en-US"]) {
      for (const width of [360, 768, 1024, 1280, 1536]) {
        const context = await browser.newContext({ viewport: { width, height: width === 360 ? 640 : 900 }, locale });
        await context.addInitScript(({ locale }) => {
          localStorage.setItem("twilight-auth", JSON.stringify({ state: { isAuthenticated: true }, version: 0 }));
          localStorage.setItem("twilight:locale", locale);
        }, { locale });
        let backgroundRequests = 0;
        await context.route("**/api/**", (route) => {
          assert.equal(route.request().method(), "GET", "Layout checks must not mutate data");
          const resource = new URL(route.request().url()).pathname.replace(/^\/api\/v[12]/, "");
          if (resource === "/system/auth-background") {
            backgroundRequests++;
            return route.fulfill({ contentType: "image/png", body: Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAgAAAAICAIAAABLbSncAAAAf0lEQVR4nA3JIQFFMQhA0R/n1cAQghAYGkxPo6+ZWQhCYKjzd+z5fYoopoSylaOUMsrvM8QwI4xtHKOMsReOOOaEs53jlDP+YiELW8RiL86iFrNeJJJYEslOTlLJ5IuLXOwSl305l7rMfVFIYUUUuzhFFVMvGmmsiWY3p6lmmj/HsmrhvzdtGgAAAABJRU5ErkJggg==", "base64") });
          }
          return route.fulfill({ json: { success: true, message: "OK", data: fixtures[resource] || {} } });
        });
        const page = await context.newPage();
        const errors = [];
        page.on("pageerror", (error) => errors.push(error.message));
        for (const route of ["/dashboard", "/admin/regcodes", "/login"]) {
          await page.goto(baseURL + route);
          const root = route === "/login" ? ".auth-panel" : "#main-content";
          await page.locator(root).waitFor();
          await page.waitForFunction((locale) => document.documentElement.lang === locale, locale);
          await page.waitForTimeout(600);
          await assertFits(page, root);
          if (width >= 1024 && route !== "/login") await assertFits(page, ".sidebar-surface");
          if (route === "/login") {
            await page.locator(".auth-has-bg").waitFor();
            assert.ok(backgroundRequests > 0, "Login must request the configured background image");
            const imageURL = await page.locator(".auth-has-bg").evaluate((el) => getComputedStyle(el).backgroundImage);
            assert.ok(imageURL.includes("/api/v2/system/auth-background"));
            assert.ok(!imageURL.includes("/api/v2/api/v2"));
            const decoded = await page.evaluate(async (cssURL) => {
              const image = new Image();
              image.src = JSON.parse(cssURL.slice(4, -1));
              await image.decode();
              return image.naturalWidth > 0;
            }, imageURL);
            assert.ok(decoded, "Configured background must decode as an image");
          }
          if (locale === "en-US" && width <= 1024) {
            if (route === "/dashboard") {
              await page.getByRole("button", { name: /View lines/ }).click();
              await checkDialog(page);
            }
            if (route === "/admin/regcodes") {
              await page.getByRole("button", { name: "Generate Codes", exact: true }).click();
              await checkDialog(page);
              await page.getByRole("combobox").first().click();
              await page.getByRole("listbox").waitFor();
              await assertFits(page, '[role="listbox"]');
              await page.keyboard.press("Escape");
            }
          }
          await page.screenshot({ path: path.join(output, `${locale}-${width}-${route.split("/").pop()}.png`), fullPage: true });
          cases++;
        }
        assert.deepEqual(errors, []);
        await context.close();
      }
    }
    console.log(`Firefox: ${cases} page/locale/viewport cases passed, including background loading and narrow-screen dialogs.`);
  } finally {
    await browser.close();
  }
})().catch((error) => { console.error(error); process.exitCode = 1; });
