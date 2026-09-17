#!/usr/bin/env python3
"""Cross-check every webui API call against the registered /api/v2 routes.

Extracts (method, path) pairs from webui/src/lib/api.ts request call sites and
from internal/api/routes_v2.go, then reports calls that have no V2 route.
Template interpolations are normalised to :param so they match route patterns.
"""

import pathlib
import re
import sys

ROOT = pathlib.Path(".")
API_TS = ROOT / "webui/src/lib/api.ts"
ROUTES = ROOT / "internal/api/routes_v2.go"

METHOD_CONST = {
    "MethodGet": "GET",
    "MethodPost": "POST",
    "MethodPut": "PUT",
    "MethodPatch": "PATCH",
    "MethodDelete": "DELETE",
    "MethodHead": "HEAD",
}

ROUTE_RE = re.compile(
    r'a\.add\(http\.Method(\w+),\s*"([^"]+)"'
)
PATH_LITERAL_RE = re.compile(r'["`](/(?:[^"`\s]|(?:\$\{[^}]*\}))+)["`]')
PARAM_RE = re.compile(r"\$\{[^}]*\}")
METHOD_RE = re.compile(r'method:\s*"([A-Z]+)"')
# `${enable ? "enable" : "disable"}` — two literal alternatives, no interpolation.
TERNARY_RE = re.compile(r'\$\{[^}]*?\?\s*["\'`]([^"\'`]*)["\'`]\s*:\s*["\'`]([^"\'`]*)["\'`]\s*\}')


def expand_interpolations(path: str) -> list[str]:
    """Expand a template literal into the concrete paths it can produce.

    Two shapes occur in api.ts:
      * `${cond ? "enable" : "disable"}` -> two registered routes
      * `${suffix ? "?a=1" : ""}`        -> an optional query string, i.e. no path text
    Everything else collapses to a `:param` placeholder.
    """
    slots: list[tuple[str, list[str]]] = []
    cursor = 0
    for match in PARAM_RE.finditer(path):
        token = match.group(0)
        if ternary := TERNARY_RE.fullmatch(token):
            left, right = ternary.group(1), ternary.group(2)
            if left.startswith("?") or right.startswith("?"):
                options = ["", ""] if left and right else [""]
            else:
                options = [left, right]
        elif match.start() == 0 or path[match.start() - 1] != "/":
            # Glued to the end of a segment (`/tickets${suffix}`); it can only
            # append a query string, never introduce a path segment.
            options = [""]
        else:
            options = [":param"]
        slots.append((token, options))
        cursor = match.end()

    variants = [path]
    for token, options in slots:
        nxt = []
        for variant in variants:
            for option in options:
                nxt.append(variant.replace(token, option, 1))
        variants = nxt
    return variants


def normalise(path: str) -> str:
    # Route params carry descriptive names (:uid vs :log_id); collapse them.
    resolved = re.sub(r":\w+", ":param", path)
    # A placeholder that is not a whole segment (query suffix) is noise.
    resolved = re.sub(r":param[^/]*", ":param", resolved)
    return resolved.split("?", 1)[0].rstrip("/") or "/"


def normalised_variants(path: str) -> list[str]:
    seen: list[str] = []
    for variant in expand_interpolations(path):
        resolved = normalise(variant)
        if resolved not in seen:
            seen.append(resolved)
    return seen


def load_v2_routes():
    routes = set()
    text = ROUTES.read_text(encoding="utf-8")
    for const, path in ROUTE_RE.findall(text):
        method = METHOD_CONST.get("Method" + const)
        if not method or not path.startswith("/api/v2"):
            continue
        routes.add((method, normalise(path[len("/api/v2"):])))
    return routes


def load_frontend_calls():
    calls = []
    lines = API_TS.read_text(encoding="utf-8").splitlines()
    for index, line in enumerate(lines, start=1):
        if "request" not in line:
            continue
        if "/api/" in line:
            continue
        # The verb is frequently on a following line; look ahead a little.
        window = "\n".join(lines[index - 1 : index + 5])
        if 'apiVersion: "v1"' in window:
            continue
        method_match = METHOD_RE.search(window)
        method = method_match.group(1) if method_match else "GET"
        # The endpoint literal often sits on its own line below the call. The
        # window covers a multi-line response generic plus the init object, so a
        # call whose path lands a few lines down is still checked (a `/usage`
        # vs `/users` drift once hid behind a too-narrow window).
        for offset, candidate in enumerate(lines[index - 1 : index + 4]):
                for raw in PATH_LITERAL_RE.findall(candidate):
                    for path in normalised_variants(raw):
                        if len(path) < 2 or path.startswith("/_next") or "uploads" in path:
                            continue
                        calls.append((method, path, index + offset, candidate.strip()[:110]))
    return calls


def main() -> int:
    routes = load_v2_routes()
    calls = load_frontend_calls()
    print(f"v2 routes: {len(routes)}   frontend call sites: {len(calls)}")

    missing = []
    for method, path, lineno, snippet in calls:
        if (method, path) in routes:
            continue
        # Fall back to a method-agnostic match: some helpers infer the verb.
        if any(existing == path for _, existing in routes):
            continue
        missing.append((method, path, lineno, snippet))

    if not missing:
        print("all extracted frontend calls resolve to a registered v2 route")
        return 0

    print(f"\n{len(missing)} call sites without a v2 route:")
    for method, path, lineno, snippet in missing:
        print(f"  api.ts:{lineno}  {method} {path}")
        print(f"      {snippet}")
    return 1


if __name__ == "__main__":
    sys.exit(main())
