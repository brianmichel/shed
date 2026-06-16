const $ = (id) => document.getElementById(id);

const state = {
  sandboxes: [],
  selectedId: null,
  lastEvents: [],
};

async function json(url, opts) {
  const r = await fetch(url, opts);
  const ct = r.headers.get("content-type") || "";
  const body = ct.includes("application/json") ? await r.json() : { error: { message: await r.text() } };
  if (!r.ok) throw new Error(body.error?.message || r.statusText);
  return body;
}

function setHealth(status, ok) {
  const el = $("health");
  el.classList.remove("ok", "warn", "err");
  el.classList.add(ok ? "ok" : "err");
  $("health-text").textContent = status;
}

function fmtAge(iso) {
  const ms = Date.now() - new Date(iso).getTime();
  if (ms < 0) return "—";
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
  if (ms <= 0) return "expired";
  const s = Math.floor(ms / 1000);
  if (s < 60) return "in " + s + "s";
  const m = Math.floor(s / 60);
  if (m < 60) return "in " + m + "m";
  const h = Math.floor(m / 60);
  return "in " + h + "h " + (m % 60) + "m";
}

function capsRow(caps) {
  const items = [
    ["commands", !!caps?.commands],
    ["files", !!caps?.files],
    ["pty", !!caps?.pty],
  ];
  return `<span class="caps">${items
    .map(([k, on]) => `<span class="cap ${on ? "on" : "off"}">${k}</span>`)
    .join("")}</span>`;
}

function stateBadge(s) {
  const cls = {
    ready: "ready",
    pending: "pending",
    pending_client: "pending",
    released: "released",
    releasing: "released",
    degraded: "warn",
    failed: "failed",
  }[s] || "muted";
  return `<span class="pill ${cls}">${s}</span>`;
}

function renderStats(list) {
  $("stat-total").textContent = list.length;
  $("stat-ready").textContent = list.filter((x) => x.state === "ready").length;
  $("stat-pending").textContent = list.filter((x) => x.state === "pending" || x.state === "pending_client").length;
  $("stat-released").textContent = list.filter((x) => x.state === "released").length;
}

function renderSandboxes() {
  const tbody = $("sandboxes");
  const list = state.sandboxes;
  if (!list.length) {
    tbody.innerHTML = `<tr class="empty"><td colspan="8">No sandboxes. Click <strong>+ New sandbox</strong> to create one.</td></tr>`;
    return;
  }
  tbody.innerHTML = list
    .map(
      (x) => `
      <tr data-id="${x.id}" class="${x.id === state.selectedId ? "selected" : ""}">
        <td class="col-id"><span class="id">${x.id}</span></td>
        <td>${stateBadge(x.state)}</td>
        <td><span class="meta">${x.environment ?? "—"}</span></td>
        <td><span class="meta">${x.template ?? "—"}</span></td>
        <td>${capsRow(x.capabilities)}</td>
        <td><span class="meta" title="${x.lease?.expires_at ?? ""}">${x.lease ? fmtTTL(x.lease.expires_at) : "—"}</span></td>
        <td class="col-age"><span class="meta">${fmtAge(x.inserted_at)}</span></td>
        <td class="col-act">
          <span class="row-actions">
            <button class="btn tiny" data-act="select">Select</button>
            <button class="btn tiny" data-act="run">Run</button>
            <button class="btn tiny" data-act="files">Files</button>
            <button class="btn tiny" data-act="events">Events</button>
            <button class="btn tiny" data-act="extend">+30m</button>
            <button class="btn tiny danger" data-act="release">Release</button>
          </span>
        </td>
      </tr>`
    )
    .join("");

  tbody.querySelectorAll("tr[data-id]").forEach((tr) => {
    const id = tr.dataset.id;
    tr.addEventListener("click", (e) => {
      if (e.target.closest("button")) return;
      selectSandbox(id, { scroll: false });
    });
    tr.querySelectorAll("button[data-act]").forEach((button) => {
      button.addEventListener("click", async (e) => {
        e.stopPropagation();
        await handleSandboxAction(id, button.dataset.act, button);
      });
    });
  });
}

async function handleSandboxAction(id, action, button) {
  if (action === "select") return selectSandbox(id);
  if (action === "run") {
    selectSandbox(id, { route: "run" });
    return;
  }
  if (action === "files") {
    selectSandbox(id, { route: "files" });
    await loadFiles();
    return;
  }
  if (action === "events") {
    selectSandbox(id, { route: "events" });
    await loadEvents(id);
    return;
  }
  if (action === "extend") return withBusy(button, "+30m…", async () => extendLease(id));
  if (action === "release") return withBusy(button, "Releasing…", async () => releaseSandbox(id));
}

async function withBusy(button, label, fn) {
  const old = button.textContent;
  button.disabled = true;
  button.textContent = label;
  try {
    await fn();
  } catch (e) {
    appendTerm(`\n[action failed] ${e.message}\n`, "ev-err");
  } finally {
    button.disabled = false;
    button.textContent = old;
  }
}

function selectSandbox(id, opts = {}) {
  state.selectedId = id;
  $("sandbox-id").value = id;
  $("events-sandbox-id").value = id;
  const sb = state.sandboxes.find((x) => x.id === id);
  $("sel-summary").textContent = sb
    ? `${sb.id} · ${sb.environment}/${sb.template} · ${sb.state}`
    : id;
  renderSandboxes();
  if (opts.route) {
    location.hash = `#/${opts.route}`;
    setRoute(opts.route);
  } else if (opts.scroll !== false) {
    document.getElementById("panel-run").scrollIntoView({ behavior: "smooth", block: "nearest" });
  }
}

async function refresh() {
  try {
    const h = await json("/v1/health");
    setHealth(h.status, true);
  } catch (e) {
    setHealth("offline", false);
  }
  try {
    const s = await json("/v1/sandboxes");
    state.sandboxes = s.data || [];
    renderStats(state.sandboxes);
    renderSandboxes();
    if (state.selectedId && !state.sandboxes.some((x) => x.id === state.selectedId)) {
      state.selectedId = null;
      $("sandbox-id").value = "";
      $("events-sandbox-id").value = "";
      $("sel-summary").textContent = "no sandbox selected";
    }
  } catch (e) {
    $("sandboxes").innerHTML = `<tr class="empty"><td colspan="8">Failed to load: ${e.message}</td></tr>`;
  }
}

async function createSandbox() {
  const btn = $("create");
  btn.disabled = true;
  const old = btn.textContent;
  btn.textContent = "Creating…";
  try {
    const r = await json("/v1/sandboxes", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ environment: "local", template: "dev" }),
    });
    await refreshUntilReady(r.data.id);
    selectSandbox(r.data.id, { route: "run" });
    appendTerm(`\n[created sandbox ${r.data.id}]\n`, "ev-ok");
  } catch (e) {
    appendTerm(`\n[create failed] ${e.message}\n`, "ev-err");
  } finally {
    btn.disabled = false;
    btn.textContent = old;
  }
}

async function refreshUntilReady(id) {
  for (let i = 0; i < 20; i++) {
    await refresh();
    const sb = state.sandboxes.find((x) => x.id === id);
    if (!sb || sb.state === "ready" || sb.state === "released" || sb.state === "failed") return;
    await new Promise((resolve) => setTimeout(resolve, 200));
  }
}

async function extendLease(id) {
  await json(`/v1/sandboxes/${id}/lease`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ ttl_ms: 30 * 60 * 1000 }),
  });
  await refresh();
  if (state.selectedId === id) await loadEvents(id);
}

async function releaseSandbox(id) {
  if (!confirm(`Release sandbox ${id}? Running commands will be disconnected.`)) return;
  await json(`/v1/sandboxes/${id}/release`, { method: "POST", headers: { "content-type": "application/json" }, body: "{}" });
  await refresh();
  if (state.selectedId === id) await loadEvents(id);
}

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
  const sid = $("sandbox-id").value.trim();
  const cmd = $("command").value;
  if (!sid) {
    setTermStatus("no sandbox", "failed");
    clearTerm();
    appendTerm("Select a sandbox first.\n", "ev-err");
    return;
  }
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
        appendTerm(`\n[failed] ${e.data?.message || ""}\n`, "ev-err");
        setTermStatus("failed", "failed");
        done = true;
      } else if (e.type === "command.killed") {
        appendTerm(`\n[killed]\n`, "ev-err");
        setTermStatus("killed", "failed");
        done = true;
      }
    }
  }
  if (!done) setTermStatus("timeout", "failed");
  await loadEvents(sid);
}

async function loadEvents(id = $("events-sandbox-id").value.trim() || state.selectedId) {
  if (!id) {
    $("events-output").textContent = "Select a sandbox to inspect its event stream.";
    return;
  }
  $("events-sandbox-id").value = id;
  try {
    const r = await json(`/v1/sandboxes/${id}/events`);
    state.lastEvents = r.data || [];
    $("events-output").textContent = state.lastEvents.length
      ? state.lastEvents.map(formatEvent).join("\n")
      : `No events for ${id}.`;
  } catch (e) {
    $("events-output").textContent = `[event load failed] ${e.message}`;
  }
}

function formatEvent(e) {
  const data = e.data && Object.keys(e.data).length ? " " + JSON.stringify(e.data) : "";
  return `${String(e.seq).padStart(4, "0")} ${e.timestamp} ${e.type}${data}`;
}

async function copyEvents() {
  const text = state.lastEvents.length ? JSON.stringify(state.lastEvents, null, 2) : $("events-output").textContent;
  await navigator.clipboard.writeText(text);
  const btn = $("copy-events");
  const old = btn.textContent;
  btn.textContent = "Copied";
  setTimeout(() => (btn.textContent = old), 1000);
}

async function loadFiles() {
  const sid = state.selectedId || $("sandbox-id").value.trim();
  const dir = $("files-path").value.trim() || "/workspace";
  if (!sid) {
    $("files-list").textContent = "Select a sandbox first.";
    return;
  }
  try {
    const r = await json(`/v1/sandboxes/${sid}/files?path=${encodeURIComponent(dir)}`);
    const entries = r.data.entries || [];
    $("files-list").innerHTML = entries.length
      ? entries.map(fileRow).join("")
      : `<div class="file-row"><span class="file-name">Empty directory</span></div>`;
    $("files-list").querySelectorAll(".file-row[data-path]").forEach((row) => {
      row.addEventListener("click", async () => {
        if (row.dataset.type === "dir") {
          $("files-path").value = row.dataset.path;
          await loadFiles();
        } else {
          await openFile(row.dataset.path);
        }
      });
    });
  } catch (e) {
    $("files-list").textContent = `[file list failed] ${e.message}`;
  }
}

function fileRow(entry) {
  const icon = entry.type === "dir" ? "▸" : "•";
  return `<div class="file-row" data-path="${entry.path}" data-type="${entry.type}"><span>${icon}</span><span class="file-name">${entry.name}</span><span class="file-meta">${entry.type} ${entry.size ?? 0}b</span></div>`;
}

async function openFile(path) {
  const sid = state.selectedId || $("sandbox-id").value.trim();
  if (!sid) return;
  try {
    const r = await json(`/v1/sandboxes/${sid}/files/content?path=${encodeURIComponent(path)}`);
    $("file-path").value = r.data.path;
    $("file-content").value = r.data.content || "";
  } catch (e) {
    $("file-content").value = `[file read failed] ${e.message}`;
  }
}

async function saveFile() {
  const sid = state.selectedId || $("sandbox-id").value.trim();
  const path = $("file-path").value.trim();
  if (!sid || !path) {
    alert("Select a sandbox and enter a file path.");
    return;
  }
  const btn = $("save-file");
  const old = btn.textContent;
  btn.disabled = true;
  btn.textContent = "Saving…";
  try {
    await json(`/v1/sandboxes/${sid}/files/content`, {
      method: "PUT",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ path, content: $("file-content").value }),
    });
    const parent = path.includes("/") ? path.slice(0, path.lastIndexOf("/")) || "/workspace" : "/workspace";
    $("files-path").value = parent;
    await loadFiles();
    await loadEvents(sid);
    btn.textContent = "Saved";
    setTimeout(() => (btn.textContent = old), 1000);
  } catch (e) {
    alert(`Save failed: ${e.message}`);
    btn.textContent = old;
  } finally {
    btn.disabled = false;
  }
}

function setRoute(name) {
  document.querySelectorAll(".topnav a").forEach((a) => {
    a.classList.toggle("active", a.dataset.route === name);
  });
  document.getElementById("crumbs").innerHTML = `<span class="crumb">${name}</span>`;
  const target = document.getElementById(`panel-${name}`) || document.getElementById("panel-sandboxes");
  target.scrollIntoView({ behavior: "smooth", block: "start" });
  if (name === "events") loadEvents();
  if (name === "files") loadFiles();
}

function handleHash() {
  const h = (location.hash || "#/sandboxes").replace(/^#\//, "");
  setRoute(h || "sandboxes");
}

$("refresh").addEventListener("click", refresh);
$("create").addEventListener("click", createSandbox);
$("run").addEventListener("click", runCommand);
$("load-files").addEventListener("click", loadFiles);
$("refresh-files").addEventListener("click", loadFiles);
$("save-file").addEventListener("click", saveFile);
$("files-path").addEventListener("keydown", (e) => {
  if (e.key === "Enter") loadFiles();
});
$("load-events").addEventListener("click", () => loadEvents());
$("refresh-events").addEventListener("click", () => loadEvents());
$("copy-events").addEventListener("click", copyEvents);
$("events-sandbox-id").addEventListener("keydown", (e) => {
  if (e.key === "Enter") loadEvents();
});
$("command").addEventListener("keydown", (e) => {
  if (e.key === "Enter") runCommand();
});
window.addEventListener("hashchange", handleHash);

handleHash();
refresh();
setInterval(refresh, 5000);
