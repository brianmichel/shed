/* ============================================================
   Shed UI — app
   Master/detail IA, hash-based routing, theme toggle.
   ============================================================ */

const $ = (id) => document.getElementById(id);

const TABS = ["overview", "run", "files", "events"];

const state = {
  sandboxes: [],
  current: null,           // { view: "list" } | { view: "detail", id, tab }
  detailSandbox: null,     // hydrated sandbox object for the detail view
  tabTimer: null,          // setInterval handle for tab-specific polling
};

function stopTabPolling() {
  if (state.tabTimer) { clearInterval(state.tabTimer); state.tabTimer = null; }
}
function startTabPolling(fn, everyMs) {
  stopTabPolling();
  fn();
  state.tabTimer = setInterval(fn, everyMs);
}

/* ───────────────────────── fetch helpers ───────────────────────── */

async function json(url, opts) {
  const r = await fetch(url, opts);
  const ct = r.headers.get("content-type") || "";
  const body = ct.includes("application/json") ? await r.json() : { error: { message: await r.text() } };
  if (!r.ok) throw new Error(body.error?.message || r.statusText);
  return body;
}

/* ───────────────────────── theme ───────────────────────── */

const THEME_CYCLE = ["auto", "light", "dark"];
const THEME_GLYPH = { auto: "A", light: "☀", dark: "☾" };

function currentTheme() {
  return localStorage.getItem("shed.theme") || "auto";
}
function applyTheme(t) {
  if (t === "auto") document.documentElement.removeAttribute("data-theme");
  else document.documentElement.setAttribute("data-theme", t);
  const btn = $("theme-toggle");
  const icon = $("theme-icon");
  if (icon) icon.textContent = THEME_GLYPH[t];
  if (btn) btn.title = `Theme: ${t}`;
}
function cycleTheme() {
  const next = THEME_CYCLE[(THEME_CYCLE.indexOf(currentTheme()) + 1) % THEME_CYCLE.length];
  try { localStorage.setItem("shed.theme", next); } catch (e) {}
  applyTheme(next);
}

/* ───────────────────────── formatting ───────────────────────── */

function fmtAge(iso) {
  const ms = Date.now() - new Date(iso).getTime();
  if (Number.isNaN(ms) || ms < 0) return "—";
  const s = Math.floor(ms / 1000);
  if (s < 60) return s + "s";
  const m = Math.floor(s / 60);
  if (m < 60) return m + "m";
  const h = Math.floor(m / 60);
  if (h < 48) return h + "h";
  return Math.floor(h / 24) + "d";
}
function fmtTTL(iso) {
  const ms = new Date(iso).getTime() - Date.now();
  if (Number.isNaN(ms)) return "—";
  if (ms <= 0) return "expired";
  const s = Math.floor(ms / 1000);
  if (s < 60) return "in " + s + "s";
  const m = Math.floor(s / 60);
  if (m < 60) return "in " + m + "m";
  const h = Math.floor(m / 60);
  return "in " + h + "h " + (m % 60) + "m";
}
function fmtTimestamp(iso) {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso || "—";
  return d.toLocaleString(undefined, { hour12: false });
}
function capsRow(caps) {
  return `<span class="caps">${[
    ["commands", !!caps?.commands],
    ["files", !!caps?.files],
    ["pty", !!caps?.pty],
  ].map(([k, on]) => `<span class="cap ${on ? "on" : "off"}">${k}</span>`).join("")}</span>`;
}
function stateBadge(s) {
  const cls = { ready: "ready", pending: "pending", released: "released", failed: "failed" }[s] || "muted";
  return `<span class="pill ${cls}">${s}</span>`;
}
function escapeHTML(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}

/* ───────────────────────── health ───────────────────────── */

function setHealth(status, ok) {
  const el = $("health");
  el.classList.remove("ok", "warn", "err");
  el.classList.add(ok ? "ok" : "err");
  $("health-text").textContent = status;
}

async function pollHealth() {
  try {
    const h = await json("/v1/health");
    setHealth(h.status, true);
  } catch (e) {
    setHealth("offline", false);
  }
}

/* ───────────────────────── list view ───────────────────────── */

function renderStats(list) {
  $("subbar-inner").querySelector("#stat-total")?.replaceChildren(document.createTextNode(list.length));
  $("subbar-inner").querySelector("#stat-ready")?.replaceChildren(document.createTextNode(list.filter((x) => x.state === "ready").length));
  $("subbar-inner").querySelector("#stat-pending")?.replaceChildren(document.createTextNode(list.filter((x) => x.state === "pending").length));
  $("subbar-inner").querySelector("#stat-released")?.replaceChildren(document.createTextNode(list.filter((x) => x.state === "released").length));
}

function renderSandboxList() {
  const tbody = $("sandboxes");
  const wrap = $("sandbox-tbl-wrap");
  const empty = $("sandbox-empty");
  const list = state.sandboxes;
  if (!list.length) {
    tbody.innerHTML = "";
    if (wrap) wrap.hidden = true;
    if (empty) empty.hidden = false;
    return;
  }
  if (wrap) wrap.hidden = false;
  if (empty) empty.hidden = true;
  tbody.innerHTML = list.map((x) => `
    <tr data-id="${x.id}">
      <td class="col-id"><span class="id">${x.id}</span></td>
      <td>${stateBadge(x.state)}</td>
      <td><span class="meta">${escapeHTML(x.environment ?? "—")}</span></td>
      <td><span class="meta">${escapeHTML(x.template ?? "—")}</span></td>
      <td>${capsRow(x.capabilities)}</td>
      <td><span class="meta" title="${x.lease?.expires_at ?? ""}">${x.lease ? fmtTTL(x.lease.expires_at) : "—"}</span></td>
      <td class="col-age"><span class="meta">${fmtAge(x.inserted_at)}</span></td>
      <td class="col-chev"><span class="chev" aria-hidden="true">›</span></td>
    </tr>`).join("");

  tbody.querySelectorAll("tr[data-id]").forEach((tr) => {
    tr.addEventListener("click", () => {
      location.hash = `#/sandboxes/${tr.dataset.id}`;
    });
  });
}

async function refreshSandboxes() {
  try {
    const s = await json("/v1/sandboxes");
    state.sandboxes = s.data || [];
    if (state.current?.view === "list") {
      renderStats(state.sandboxes);
      renderSandboxList();
    }
    if (state.current?.view === "detail") {
      const sb = state.sandboxes.find((x) => x.id === state.current.id);
      if (sb) {
        state.detailSandbox = sb;
        renderDetailChrome();
        if (state.current.tab === "overview") renderOverview();
      } else if (state.detailSandbox) {
        // The sandbox we were viewing is gone (released externally). Bounce
        // back to the list so the UI never sits on stale context.
        state.detailSandbox = null;
        location.hash = "#/sandboxes";
      }
    }
  } catch (e) {
    if (state.current?.view === "list") {
      $("sandboxes").innerHTML = `<tr class="empty"><td colspan="7">Failed to load: ${escapeHTML(e.message)}</td></tr>`;
    }
  }
}

async function createSandbox() {
  const btns = [$("create"), $("create-empty")].filter(Boolean);
  const olds = btns.map((b) => b.textContent);
  btns.forEach((b) => { b.disabled = true; b.textContent = "Creating…"; });
  try {
    const r = await json("/v1/sandboxes", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: "{}",
    });
    await refreshSandboxes();
    location.hash = `#/sandboxes/${r.data.id}`;
  } catch (e) {
    alert("Create failed: " + e.message);
  } finally {
    btns.forEach((b, i) => { b.disabled = false; b.textContent = olds[i]; });
  }
}

/* ───────────────────────── detail view ───────────────────────── */

function renderDetailChrome() {
  const sb = state.detailSandbox;
  const id = state.current.id;
  const tab = state.current.tab;
  const sub = $("subbar-inner");
  const released = sb?.state === "released";
  sub.innerHTML = `
    <div class="crumbs">
      <a class="crumb-back" href="#/sandboxes" title="Back to sandboxes">‹ Sandboxes</a>
      <span class="crumb-sep">/</span>
      <span class="crumb current crumb-id">${id}</span>
      ${sb ? stateBadge(sb.state) : ""}
    </div>
    <nav class="tabs" aria-label="Sandbox tabs">
      ${TABS.map((t) => `<a class="tab ${t === tab ? "active" : ""}" href="#/sandboxes/${id}/${t}">${t}</a>`).join("")}
    </nav>
    <div class="subbar-actions">
      <button class="btn danger" id="release" ${released ? "disabled" : ""}>${released ? "Released" : "Release"}</button>
    </div>
  `;
}

async function releaseCurrent() {
  const id = state.current?.id;
  if (!id) return;
  if (!confirm(`Release ${id}? This ends the session and frees the sandbox.`)) return;
  const btn = $("release");
  if (btn) { btn.disabled = true; btn.textContent = "Releasing…"; }
  try {
    await json(`/v1/sandboxes/${id}/release`, { method: "POST", headers: { "content-type": "application/json" }, body: "{}" });
    state.sandboxes = state.sandboxes.filter((x) => x.id !== id);
    location.hash = "#/sandboxes";
  } catch (e) {
    alert("Release failed: " + e.message);
    if (btn) { btn.disabled = false; btn.textContent = "Release"; }
  }
}

function showTab(tab) {
  document.querySelectorAll("#view-detail [data-tab]").forEach((el) => {
    el.hidden = el.dataset.tab !== tab;
  });
}

function renderOverview() {
  const sb = state.detailSandbox;
  const grid = $("overview-kv");
  if (!sb) {
    grid.innerHTML = `<div class="empty-state"><div class="empty-title">Sandbox not found</div><div class="empty-body">It may have been released. <a href="#/sandboxes">Back to sandboxes</a>.</div></div>`;
    return;
  }
  const rows = [
    ["Environment", `<span class="mono">${escapeHTML(sb.environment)}</span>`],
    ["Template", `<span class="mono">${escapeHTML(sb.template)}</span>`],
    ["Capabilities", capsRow(sb.capabilities)],
    ["Lease TTL", `<span class="mono">${sb.lease ? Math.round(sb.lease.ttl_ms / 1000) + "s" : "—"}</span>`],
    ["Lease expires", `<span class="mono">${sb.lease ? fmtTimestamp(sb.lease.expires_at) + " (" + fmtTTL(sb.lease.expires_at) + ")" : "—"}</span>`],
    ["Created", `<span class="mono">${fmtTimestamp(sb.inserted_at)} (${fmtAge(sb.inserted_at)} ago)</span>`],
    ["Updated", `<span class="mono">${fmtTimestamp(sb.updated_at)}</span>`],
    ["Metadata", `<pre class="kv-pre mono">${escapeHTML(JSON.stringify(sb.metadata || {}, null, 2))}</pre>`],
  ];
  grid.innerHTML = rows.map(([k, v]) => `<div class="kv-row"><div class="kv-k">${k}</div><div class="kv-v">${v}</div></div>`).join("");
}

/* — Run tab — */

function setTermStatus(label, cls) {
  const el = $("term-status");
  el.className = "term-status mono " + (cls || "");
  el.textContent = label;
}
function setTermTitle(t) { $("term-title").textContent = t; }
function clearTerm() { $("output").innerHTML = ""; }
function appendTerm(text, cls) {
  const span = document.createElement("span");
  if (cls) span.className = cls;
  span.textContent = text;
  const out = $("output");
  out.appendChild(span);
  out.scrollTop = out.scrollHeight;
}

async function runCommand() {
  const sid = state.current?.id;
  const cmd = $("command").value;
  if (!sid) return;
  clearTerm();
  setTermTitle(`${sid} $ ${cmd}`);
  setTermStatus("starting", "running");
  let r;
  try {
    r = await json(`/v1/sandboxes/${sid}/commands`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ command: cmd }),
    });
  } catch (e) {
    setTermStatus("failed", "failed");
    appendTerm(`[start failed] ${e.message}\n`, "ev-err");
    return;
  }
  const cmdId = r.data.id;
  setTermStatus("running", "running");
  appendTerm(`[command ${cmdId}]\n`, "ev");

  let after = 0;
  let done = false;
  for (let i = 0; i < 240 && !done; i++) {
    await new Promise((r) => setTimeout(r, 400));
    if (state.current?.tab !== "run" || state.current?.id !== sid) return;
    let ev;
    try {
      ev = await json(`/v1/sandboxes/${sid}/commands/${cmdId}/events?after=${after}`);
    } catch (e) {
      setTermStatus("error", "failed");
      appendTerm(`\n[poll error] ${e.message}\n`, "ev-err");
      return;
    }
    after = ev.next_cursor || after;
    for (const e of ev.data) {
      if (e.type === "command.stdout") appendTerm(e.data.chunk);
      else if (e.type === "command.stderr") appendTerm(e.data.chunk, "ev-err");
      else if (e.type === "command.exit") {
        const code = e.data?.exit_code ?? 0;
        appendTerm(`\n[exit ${code}]\n`, code === 0 ? "ev-ok" : "ev-err");
        setTermStatus(`exited ${code}`, code === 0 ? "exited" : "failed");
        done = true;
      } else if (e.type === "command.failed") {
        appendTerm(`\n[failed]\n`, "ev-err"); setTermStatus("failed", "failed"); done = true;
      } else if (e.type === "command.killed") {
        appendTerm(`\n[killed]\n`, "ev-err"); setTermStatus("killed", "failed"); done = true;
      }
    }
  }
  if (!done) setTermStatus("timeout", "failed");
}

/* — Events tab — */

async function refreshEvents() {
  const sid = state.current?.id;
  if (!sid) return;
  const tbody = $("events-body");
  try {
    const ev = await json(`/v1/sandboxes/${sid}/events?after=0`);
    // API returns events in ascending seq order; show newest on top.
    const list = (ev.data || []).slice().sort((a, b) => (b.seq ?? 0) - (a.seq ?? 0));
    if (!list.length) {
      tbody.innerHTML = `<tr class="empty"><td colspan="5">No events yet.</td></tr>`;
      return;
    }
    tbody.innerHTML = list.map((e) => `
      <tr>
        <td class="col-seq mono">${e.seq ?? "—"}</td>
        <td class="col-ts mono">${fmtTimestamp(e.occurred_at || e.time || e.inserted_at)}</td>
        <td>${escapeHTML(e.type || "—")}</td>
        <td><span class="meta">${escapeHTML(e.source || "—")}</span></td>
        <td><span class="mono" style="color:var(--color-text-muted)">${escapeHTML(JSON.stringify(e.data || {}))}</span></td>
      </tr>`).join("");
  } catch (e) {
    tbody.innerHTML = `<tr class="empty"><td colspan="5">Failed to load events: ${escapeHTML(e.message)}</td></tr>`;
  }
}

/* ───────────────────────── router ───────────────────────── */

function parseHash() {
  const h = (location.hash || "#/sandboxes").replace(/^#\/?/, "");
  const parts = h.split("/").filter(Boolean);
  if (parts[0] !== "sandboxes") return { view: "list" };
  if (parts.length === 1) return { view: "list" };
  const id = parts[1];
  const tab = TABS.includes(parts[2]) ? parts[2] : "overview";
  return { view: "detail", id, tab };
}

function renderListChrome() {
  $("subbar-inner").innerHTML = `
    <div class="crumbs"><span class="crumb current">sandboxes</span></div>
    <div class="stats">
      <span class="stat"><span class="stat-k">total</span><span class="stat-v mono" id="stat-total">0</span></span>
      <span class="stat"><span class="stat-k">ready</span><span class="stat-v mono" id="stat-ready">0</span></span>
      <span class="stat"><span class="stat-k">pending</span><span class="stat-v mono" id="stat-pending">0</span></span>
      <span class="stat"><span class="stat-k">released</span><span class="stat-v mono" id="stat-released">0</span></span>
    </div>
    <div class="subbar-actions">
      <button class="btn ghost" id="refresh">Refresh</button>
      <button class="btn primary" id="create">+ New sandbox</button>
    </div>
  `;
  renderStats(state.sandboxes);
}

async function route() {
  const next = parseHash();
  state.current = next;

  document.querySelectorAll(".topnav a").forEach((a) => a.classList.toggle("active", a.dataset.route === "sandboxes"));

  if (next.view === "list") {
    stopTabPolling();
    $("view-list").hidden = false;
    $("view-detail").hidden = true;
    renderListChrome();
    await refreshSandboxes();
    return;
  }

  // detail
  $("view-list").hidden = true;
  $("view-detail").hidden = false;

  // ensure we have sandbox data
  if (!state.sandboxes.length) {
    try { state.sandboxes = (await json("/v1/sandboxes")).data || []; } catch (e) {}
  }
  state.detailSandbox = state.sandboxes.find((x) => x.id === next.id) || null;
  if (!state.detailSandbox) {
    // try direct fetch
    try {
      const r = await json(`/v1/sandboxes/${next.id}`);
      state.detailSandbox = r.data;
    } catch (e) { /* leave null; overview will render not-found */ }
  }

  renderDetailChrome();
  showTab(next.tab);

  stopTabPolling();
  switch (next.tab) {
    case "overview": renderOverview(); break;
    case "events": startTabPolling(refreshEvents, 3000); break;
    case "run": setTermStatus("idle", ""); break;
    case "files": /* static empty state for now */ break;
  }

  window.scrollTo({ top: 0 });
}

/* ───────────────────────── boot ───────────────────────── */

document.addEventListener("click", (e) => {
  const t = e.target;
  if (t.id === "refresh") refreshSandboxes();
  else if (t.id === "create" || t.id === "create-empty") createSandbox();
  else if (t.id === "run") runCommand();
  else if (t.id === "release") releaseCurrent();
  else if (t.id === "theme-toggle" || t.closest("#theme-toggle")) cycleTheme();
});
document.addEventListener("keydown", (e) => {
  if (e.key === "Enter" && e.target.id === "command") runCommand();
});
window.addEventListener("hashchange", route);

applyTheme(currentTheme());
route();
pollHealth();
setInterval(pollHealth, 5000);
setInterval(refreshSandboxes, 5000);
