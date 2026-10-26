const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const { test } = require("node:test");
const ts = require("typescript");

const compiled = ts.transpileModule(fs.readFileSync(path.join(__dirname, "../src/lib/auth-background.ts"), "utf8"), {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 },
}).outputText;
const exportsObject = {};
vm.runInNewContext(compiled, { exports: exportsObject, URL });
const { resolveAuthBackgroundUrl } = exportsObject;

test("uploaded backgrounds accept legacy and V2 paths without repeating the prefix", () => {
  for (const prefix of ["", "/api/v1", "/api/v2"]) {
    const raw = `${prefix}/system/auth-background?v=2`;
    assert.equal(resolveAuthBackgroundUrl(raw), "/api/v2/system/auth-background?v=2");
    assert.equal(resolveAuthBackgroundUrl(raw, "https://api.example/"), "https://api.example/api/v2/system/auth-background?v=2");
  }
});

test("external backgrounds preserve the URL including spaces and parentheses", () => {
  assert.equal(resolveAuthBackgroundUrl(" https://images.example/my background(1).jpg "), "https://images.example/my%20background(1).jpg");
});

test("invalid paths, protocols and credentials never become background images", () => {
  for (const raw of [null, "", "//images.example/a.jpg", "/srv/uploads/a.jpg", "C:\\uploads\\a.jpg", "javascript:alert(1)", "https://user:secret@images.example/a.jpg", "https://images.example/\na.jpg"]) {
    assert.equal(resolveAuthBackgroundUrl(raw), undefined, String(raw));
  }
});
