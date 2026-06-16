const $ = (id) => document.getElementById(id);

const state = {
  sandboxes: [],
  selectedId: null,
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
  const cls = { ready: "ready", pending: "pending", released: "released", failed: "failed" }[s] || "muted";
  return `<span class="pill ${cls}">${s}</span>`;
}

function renderStats(list) {
  $("stat-total").textContent = list.length;
  $("stat-ready").textContent = list.filter((x) => x.state === "ready").length;
  $("stat-pending").textContent = list.filter((x) => x.state === "pending").length;
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
        <td class="col-act"><button class="btn tiny" data-act="select">Select</button></td>
      </tr>`
    )
    .join("");

  tbody.querySelectorAll("tr[data-id]").forEach((tr) => {
    const id = tr.dataset.id;
    tr.addEventListener("click", (e) => {
      if (e.target.closest("button")) return;
      selectSandbox(id);
    });
    tr.querySelector('[data-act="select"]').addEventListener("click", (e) => {
      e.stopPropagation();
      selectSandbox(id);
    });
  });
}

function selectSandbox(id) {
  state.selectedId = id;
  $("sandbox-id").value = id;
  const sb = state.sandboxes.find((x) => x.id === id);
  $("sel-summary").textContent = sb
    ? `${sb.id} · ${sb.environment}/${sb.template} · ${sb.state}`
    : id;
  renderSandboxes();
  document.getElementById("panel-run").scrollIntoView({ behavior: "smooth", block: "nearest" });
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
      body: "{}",
    });
    await refresh();
    selectSandbox(r.data.id);
  } catch (e) {
    appendTerm(`\n[create failed] ${e.message}\n`, "ev-err");
  } finally {
    btn.disabled = false;
    btn.textContent = old;
  }
}

function setTermStatus(label, cls) {
  const el = $("term-status");
  el.className = "term-status mono " + (cls || "");
  el.textContent = label;
}
function setTermTitle(t) {
  $("term-title").textContent = t;
}
function clearTerm() {
  $("output").innerHTML = "";
}
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
        appendTerm(`\n[failed]\n`, "ev-err");
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
}

function setRoute(name) {
  document.querySelectorAll(".topnav a").forEach((a) => {
    a.classList.toggle("active", a.dataset.route === name);
  });
  document.getElementById("crumbs").innerHTML = `<span class="crumb">${name}</span>`;
  if (name === "run") {
    document.getElementById("panel-run").scrollIntoView({ behavior: "smooth", block: "start" });
  } else if (name === "sandboxes") {
    document.getElementById("panel-sandboxes").scrollIntoView({ behavior: "smooth", block: "start" });
  } else if (name === "events") {
    document.getElementById("panel-sandboxes").scrollIntoView({ behavior: "smooth", block: "start" });
  }
}

function handleHash() {
  const h = (location.hash || "#/sandboxes").replace(/^#\//, "");
  setRoute(h || "sandboxes");
}

$("refresh").addEventListener("click", refresh);
$("create").addEventListener("click", createSandbox);
$("run").addEventListener("click", runCommand);
$("command").addEventListener("keydown", (e) => {
  if (e.key === "Enter") runCommand();
});
window.addEventListener("hashchange", handleHash);

handleHash();
refresh();
setInterval(refresh, 5000);
