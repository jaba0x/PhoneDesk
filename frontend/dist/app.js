"use strict";
/* PhoneDesk UI. Talks to the Go side through window.go.main.App (Wails bindings). */

let api = null;
const S = { dash: null, detail: null, creds: [], route: [], tab: "overview", filter: "all", query: "", modalOpen: false, consoleOut: "", consoleHist: [] };

// ---------- helpers ----------
const $ = (sel, root = document) => root.querySelector(sel);
const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];
const esc = (v) => String(v ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
const plural = (n, w) => `${n} ${w}${n === 1 ? "" : "s"}`;

function ago(t) {
  if (!t || t.startsWith("0001")) return "never";
  const s = Math.round((Date.now() - new Date(t)) / 1000);
  if (s < 60) return "just now";
  if (s < 3600) return `${Math.round(s / 60)} min ago`;
  if (s < 86400) return `${Math.round(s / 3600)} h ago`;
  return new Date(t).toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

const ICON = {
  dash: '<svg viewBox="0 0 24 24"><rect x="3" y="4" width="18" height="6" rx="1.5"/><rect x="3" y="14" width="18" height="6" rx="1.5"/><path d="M7 7h.01M10 7h.01M7 17h.01M10 17h.01"/></svg>',
  list: '<svg viewBox="0 0 24 24"><path d="M8 6h13M8 12h13M8 18h13M3.5 6h.01M3.5 12h.01M3.5 18h.01"/></svg>',
  phone: '<svg viewBox="0 0 24 24"><rect x="6" y="2.5" width="12" height="19" rx="2"/><path d="M9 6h6M9 9h6M9 12h2M13 12h2M9 15h2M13 15h2M9 18h6"/></svg>',
  ext: '<svg viewBox="0 0 24 24"><path d="M5 4h4l2 5-2.5 1.5a11 11 0 0 0 5 5L15 13l5 2v4a2 2 0 0 1-2 2A16 16 0 0 1 3 6a2 2 0 0 1 2-2z"/></svg>',
  tpl: '<svg viewBox="0 0 24 24"><path d="M4 5h16v4H4zM4 11h7v8H4zM13 11h7v3h-7zM13 16h7v3h-7z"/></svg>',
  gear: '<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.6 1.6 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.6 1.6 0 0 0-1.8-.3 1.6 1.6 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.6 1.6 0 0 0-1-1.5 1.6 1.6 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.6 1.6 0 0 0 .3-1.8 1.6 1.6 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.6 1.6 0 0 0 1.5-1 1.6 1.6 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.6 1.6 0 0 0 1.8.3H9a1.6 1.6 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.6 1.6 0 0 0 1 1.5 1.6 1.6 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.6 1.6 0 0 0-.3 1.8V9a1.6 1.6 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.6 1.6 0 0 0-1.5 1z"/></svg>',
  refresh: '<svg viewBox="0 0 24 24"><path d="M21 12a9 9 0 1 1-2.6-6.4M21 4v5h-5"/></svg>',
  plus: '<svg viewBox="0 0 24 24"><path d="M12 5v14M5 12h14"/></svg>',
  back: '<svg viewBox="0 0 24 24"><path d="M15 18l-6-6 6-6"/></svg>',
  pause: '<svg viewBox="0 0 24 24"><path d="M9 5v14M15 5v14"/></svg>',
  play: '<svg viewBox="0 0 24 24"><path d="M7 5l12 7-12 7z"/></svg>',
  trash: '<svg viewBox="0 0 24 24"><path d="M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3"/></svg>',
  chip: '<svg viewBox="0 0 24 24"><rect x="6" y="6" width="12" height="12" rx="1.5"/><path d="M9 2v4M15 2v4M9 18v4M15 18v4M2 9h4M2 15h4M18 9h4M18 15h4"/></svg>',
  up: '<svg viewBox="0 0 24 24"><path d="M12 19V5M6 11l6-6 6 6"/></svg>',
  search: '<svg viewBox="0 0 24 24"><circle cx="11" cy="11" r="6.5"/><path d="M20 20l-4.2-4.2"/></svg>',
  power: '<svg viewBox="0 0 24 24"><path d="M12 3v9M18.4 6.6a9 9 0 1 1-12.8 0"/></svg>',
  radar: '<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="4.5"/><path d="M12 12l6-6"/></svg>',
  ext2: '<svg viewBox="0 0 24 24"><path d="M14 3h7v7M21 3l-9 9M19 14v5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2h5"/></svg>',
  edit: '<svg viewBox="0 0 24 24"><path d="M4 20h4l10.5-10.5a2.1 2.1 0 0 0-3-3L5 17v3zM13 7l3 3"/></svg>',
};

function toast(text, err) {
  const t = document.createElement("div");
  t.className = "toast" + (err ? " err" : "");
  t.textContent = text;
  $("#toasts").appendChild(t);
  setTimeout(() => t.remove(), err ? 7000 : 3500);
}

async function call(fn, ...args) {
  try {
    return await api[fn](...args);
  } catch (e) {
    throw new Error(typeof e === "string" ? e : e?.message || String(e));
  }
}

const phonesWord = (n) => `${n} phone${n === 1 ? "" : "s"}`;

// ---------- line keys ----------
// A phone drawn as its line keys: one LED per account. Green registered,
// red failed, grey off, muted when registration isn't known. Blue blinks
// while a line is in a call.
function keys(sum, big) {
  if (!sum?.accounts?.length) return `<div class="keys none">No account data yet</div>`;
  const busy = new Set((sum.lines || []).filter((l) => ["incall", "ringing", "calling", "hold", "dialing"].includes(l.state)).map((l) => l.account));
  const cells = sum.accounts.map((a) => {
    const cls = !a.active ? "x" : busy.has(a.index - 1) ? "c" : a.registered === 1 ? "u" : a.registered === 0 ? "b" : "q";
    const state = !a.active ? "off" : busy.has(a.index - 1) ? "in a call" : a.registered === 1 ? "registered" : a.registered === 0 ? "not registered" : "registration unknown";
    const tip = `Account ${a.index}: ${a.userId || "no user ID"}${a.server ? " at " + a.server : ""}, ${state}`;
    return `<span class="k" title="${esc(tip)}"><i class="${cls}"></i>${big || a.active ? esc(a.userId || a.index) : ""}</span>`;
  }).join("");
  return `<div class="keys${big ? " big" : ""}">${cells}</div>`;
}

function regChip(a) {
  if (!a.active) return `<span class="reg off">off</span>`;
  if (a.registered === 1) return `<span class="reg ok">registered</span>`;
  if (a.registered === 0) return `<span class="reg bad">not registered</span>`;
  return `<span class="reg unk" title="Turn on Action URI support on the phone to see registration state">unknown</span>`;
}

// ---------- shell ----------
function renderSide() {
  const r = S.route[0] || "dashboard";
  const crit = S.dash?.critical || 0;
  const fwCount = (S.dash?.phones || []).filter((v) => !v.disabled && v.firmware?.available).length;
  const item = (key, icon, label, extra = "") =>
    `<button class="nav${r === key || (key === "phones" && r === "phone") ? " on" : ""}" data-go="#/${key}">${ICON[icon]}<span>${label}</span>${extra}</button>`;
  $("#side").innerHTML = `
    <div class="brand"><img class="brand-logo" src="logo.svg" alt="" width="26" height="26"><b>PhoneDesk</b></div>
    ${item("dashboard", "dash", "Dashboard", crit ? `<span class="count">${crit}</span>` : "")}
    ${item("phones", "phone", "Phones")}
    ${item("extensions", "ext", "Extensions")}
    ${item("templates", "tpl", "Templates")}
    ${item("firmware", "chip", "Firmware", fwCount ? `<span class="count info">${fwCount}</span>` : "")}
    ${item("settings", "gear", "Settings")}
    <div class="side-foot">${S.dash ? `${S.dash.online} of ${S.dash.total} online` : ""}${S.dash?.accounts ? `<br>${S.dash.registered} of ${plural(S.dash.accounts, "account")} registered` : ""}${S.dash?.collecting ? `<br>Reading ${phonesWord(S.dash.collecting)}` : ""}
      <a class="credit" onclick="openURL('https://clihero.com')" title="clihero.com">
        <img src="clihero-logo-white.png" alt="clihero.com">
        <span>Built by Jaba Macharashvili</span></a></div>`;
}

document.addEventListener("click", (e) => {
  if (e.target.closest(".nogo")) return; // checkboxes and row buttons don't open the phone
  const go = e.target.closest("[data-go]");
  if (go) { e.preventDefault(); location.hash = go.dataset.go; }
});

window.addEventListener("hashchange", route);

async function route() {
  if (S.modalOpen) { $(".veil")?.remove(); S.modalOpen = false; }
  const h = location.hash.replace(/^#\/?/, "") || "dashboard";
  S.route = h.split("/").map(decodeURIComponent);
  renderSide();
  const [page, a, b] = S.route;
  $("#main").scrollTop = 0;
  if (page === "phone") {
    S.tab = b || "overview";
    await loadDetail(a);
  } else if (page === "phones") renderPhones();
  else if (page === "extensions") renderExtensions();
  else if (page === "templates") renderTemplates();
  else if (page === "firmware") renderFirmware();
  else if (page === "settings") renderSettings();
  else renderDashboard();
}

async function refreshData() {
  S.dash = await call("GetDashboard");
  renderSide();
  if (S.modalOpen) return;
  const page = S.route[0] || "dashboard";
  if (page === "dashboard") renderDashboard();
  if (page === "phones") renderPhones(true);
  if (page === "phone" && ["overview", "accounts", "settings"].includes(S.tab)) loadDetail(S.route[1], true);
  if (page === "phone" && S.detail) updateDetailHead();
  if (page === "extensions") renderExtensions();
  if (page === "firmware") renderFirmware(true);
}

// ---------- dashboard ----------
function renderDashboard() {
  const d = S.dash;
  if (!d) return;
  const main = $("#main");
  if (!d.total) {
    main.innerHTML = `<div class="page"><div class="head"><h1>Dashboard</h1></div>
      <div class="panel"><div class="empty"><h3>No phones yet</h3>
      <p>Add your Grandstream phones by address, or let PhoneDesk find them on the network. It logs in to each phone's web UI, reads its accounts and settings, and keeps checking.</p>
      <button class="btn primary" onclick="openPhoneModal()">${ICON.plus}Add phone</button>
      <button class="btn" onclick="openScanModal()">${ICON.radar}Scan network</button>
      <button class="btn" onclick="openBulkModal()">Add many</button></div></div></div>`;
    return;
  }
  const pct = (n) => (d.total ? (n / d.total) * 100 : 0);
  const issues = d.issues.filter((i) => S.filter === "all" ? i.severity !== "info" : i.severity === S.filter);
  const chip = (k, label, n) => `<button class="chip${S.filter === k ? " on" : ""}" onclick="S.filter='${k}';renderDashboard()">${label}${n !== undefined ? ` ${n}` : ""}</button>`;
  const bySev = { critical: 0, warning: 1, info: 2, "": 3 };
  const rank = (v) => (v.disabled ? 2 : v.status.checked && !v.status.online ? 0 : 1);
  const plates = [...d.phones].sort((a, b) => rank(a) - rank(b) || bySev[a.worst || ""] - bySev[b.worst || ""]);
  main.innerHTML = `<div class="page">
    <div class="head"><h1>Dashboard</h1><span class="grow"></span>
      <button class="btn" onclick="refreshAll(this)">${ICON.refresh}Read all now</button></div>
    <div class="fleet">
      <div class="fleet-line">
        <span class="stat" data-go="#/phones"><span class="big">${d.online}</span><span class="lbl">online</span></span>
        <span class="stat" data-go="#/phones"><span class="big ${d.offline ? "bad" : ""}">${d.offline}</span><span class="lbl">offline</span></span>
        ${d.unknown ? `<span><span class="big faint">${d.unknown}</span><span class="lbl">not checked yet</span></span>` : ""}
        ${d.disabled ? `<span class="stat" data-go="#/phones"><span class="big faint">${d.disabled}</span><span class="lbl">disabled</span></span>` : ""}
        ${d.accounts ? `<span class="stat" data-go="#/extensions"><span class="big ${d.registered < d.accounts ? "warnc" : ""}">${d.registered}<span class="faint" style="font-size:18px">/${d.accounts}</span></span><span class="lbl">accounts registered</span></span>` : ""}
        <span><span class="big ${d.critical ? "bad" : ""}">${d.critical}</span><span class="lbl">critical</span></span>
        <span><span class="big ${d.warning ? "warnc" : ""}">${d.warning}</span><span class="lbl">warnings</span></span>
      </div>
      <div class="bar" title="${d.online} online, ${d.offline} offline"><i style="width:${pct(d.online)}%;background:var(--up)"></i><i style="width:${pct(d.offline)}%;background:var(--crit)"></i></div>
    </div>
    <div class="dash">
      <section class="panel">
        <h2>Problems<span class="grow"></span><span class="chips">${chip("all", "Critical and warnings")}${chip("critical", "Critical", d.critical)}${chip("warning", "Warnings", d.warning)}${chip("info", "Notes", d.info)}</span></h2>
        <div class="issues-scroll">${issues.length ? issues.map(issueRow).join("") : `<div class="empty"><h3>Nothing to fix</h3><p>${S.filter === "all" ? "No critical problems or warnings across your phones." : "No items in this category."}</p></div>`}</div>
      </section>
      <section class="panel">
        <h2>Phones<span class="grow"></span><a data-go="#/phones" class="muted" style="font-weight:400;font-size:13px">All phones</a></h2>
        <div class="rack issues-scroll">${plates.map(plate).join("")}</div>
      </section>
    </div></div>`;
}

function issueRow(i) {
  const go = i.phoneId ? `data-go="#/phone/${i.phoneId}"` : "";
  return `<div class="issue" ${go}><div><span class="sev ${i.severity}">${{ critical: "Critical", warning: "Warning", info: "Note" }[i.severity]}</span></div>
    <div><div><span class="t">${esc(i.title)}</span> <span class="w">on ${esc(i.phoneName)}</span></div>${i.detail ? `<div class="d">${esc(i.detail)}</div>` : ""}</div></div>`;
}

function statusDot(v) {
  if (!v.status.checked) return `<span class="dot" title="Not checked yet"></span>`;
  return v.status.online ? `<span class="dot up" title="Online"></span>` : `<span class="dot down" title="Offline"></span>`;
}

const nameOfView = (v) => v.name || (v.summary?.display ? `${v.summary.display} (${v.summary.extension})` : v.summary?.extension) || v.host;

function plate(v) {
  const s = v.summary;
  const meta = v.disabled ? `<span class="faint">disabled</span>` : !v.status.online && v.status.checked ? `<span class="bad">offline ${ago(v.status.since)}</span>` :
    `${esc(s?.model || v.host)}${v.status.latencyMs ? ` <span class="faint">${v.status.latencyMs} ms</span>` : ""}`;
  return `<div class="plate${v.disabled ? " off" : ""}" data-go="#/phone/${v.id}">
    <div class="nm">${v.disabled ? `<span class="dot"></span>` : statusDot(v)}<b>${esc(nameOfView(v))}</b>${v.worst ? `<span class="sev ${v.worst}">${v.issues.length}</span>` : ""}${v.status.collecting ? `<span class="faint" style="font-size:12px">reading</span>` : ""}</div>
    <div class="meta">${meta}</div>
    <div style="grid-column:1/-1">${s ? keys(s) : `<div class="faint" style="font-size:12.5px">No data read yet</div>`}</div>
  </div>`;
}

async function refreshAll(btn) {
  btn?.classList.add("spin");
  await call("RefreshAll");
  toast("Reading all online phones");
  setTimeout(() => btn?.classList.remove("spin"), 1500);
}

// ---------- phones list ----------
function phoneMatches(v, q) {
  if (!q) return true;
  const s = v.summary || {};
  const state = v.disabled ? "disabled paused" : !v.status.checked ? "unknown" : v.status.online ? "online up" : "offline down";
  const reg = s.registrationKnown ? (s.registered < s.activeAccounts ? "unregistered" : "registered") : "";
  const hay = [v.name, v.host, v.site, v.notes, s.model, s.firmware, s.mac, s.extension, s.display, s.server, (s.accounts || []).map((a) => a.userId).join(" "), state, reg, v.worst, v.worst ? "problem" : "", v.firmware?.available ? "update" : ""].join(" ").toLowerCase();
  return q.toLowerCase().split(/\s+/).filter(Boolean).every((w) => hay.includes(w));
}

S.sel = new Set();

function renderPhones(soft) {
  const d = S.dash;
  if (!d) return;
  const rows = d.phones.filter((v) => phoneMatches(v, S.query));
  const ids = new Set(d.phones.map((v) => v.id));
  for (const id of S.sel) if (!ids.has(id)) S.sel.delete(id);
  const allOn = rows.length > 0 && rows.every((v) => S.sel.has(v.id));
  const table = `<table class="plist"><thead><tr>
      <th class="cb nogo"><input type="checkbox" title="Select all shown" ${allOn ? "checked" : ""} onchange="selectAll(this.checked)"></th>
      <th></th><th>Name</th><th>Extension</th><th>Address</th><th>Site</th><th>Model</th><th>Firmware</th><th>Accounts</th><th>Problems</th><th>Read</th><th></th></tr></thead><tbody>
    ${rows.map((v) => {
      const s = v.summary || {};
      const accts = !s.accounts ? "" : !s.activeAccounts ? `<span class="reg off">none</span>` : s.registrationKnown ? `<span class="reg ${s.registered === s.activeAccounts ? "ok" : s.registered ? "bad" : "bad"}">${s.registered}/${s.activeAccounts} registered</span>` : `<span class="reg unk">${s.activeAccounts} active</span>`;
      return `<tr class="click${v.disabled ? " off" : ""}${S.sel.has(v.id) ? " picked" : ""}" data-go="#/phone/${v.id}">
      <td class="cb nogo"><input type="checkbox" ${S.sel.has(v.id) ? "checked" : ""} onchange="toggleSel('${v.id}', this.checked)"></td>
      <td>${v.disabled ? `<span class="dot" title="Disabled"></span>` : statusDot(v)}</td>
      <td>${esc(v.name || s.display || "")}${v.disabled ? ` <span class="tag">disabled</span>` : ""}</td><td class="mono">${esc(s.extension || "")}</td><td class="mono">${esc(v.host)}</td><td>${esc(v.site)}</td>
      <td title="${esc(s.mac || v.mac || "")}">${esc(s.model || "")}</td><td><span class="mono">${esc(s.firmware || "")}</span> ${fwCell(v)}</td>
      <td>${accts}</td>
      <td>${v.worst ? `<span class="sev ${v.worst}">${v.issues.length}</span>` : ""}</td><td class="faint">${v.status.collecting ? "now" : ago(s.collectedAt)}</td>
      <td class="acts nogo">${canUpdate(v) ? `<button class="ib" title="Update firmware to ${esc(v.firmware.available)}" onclick="openUpgradeDialog(['${v.id}'])">${ICON.up}</button>` : ""}<button class="ib" title="${v.disabled ? "Enable" : "Disable"}" onclick="setDisabled(['${v.id}'], ${!v.disabled})">${v.disabled ? ICON.play : ICON.pause}</button><button class="ib danger" title="Delete" onclick="deletePhones(['${v.id}'])">${ICON.trash}</button></td></tr>`;
    }).join("")}</tbody></table>
    ${rows.length ? "" : `<div class="empty">No phones match “${esc(S.query)}”. <a onclick="clearSearch()">Clear search</a></div>`}`;
  const n = S.sel.size;
  const bar = n ? `<div class="selbar"><b>${n} selected</b><span class="grow"></span>
      <button class="btn small" onclick="refreshSel()">${ICON.refresh}Read now</button>
      <button class="btn small" onclick="openApplyDialog([...S.sel])">${ICON.tpl}Apply template</button>
      <button class="btn small" onclick="openUpgradeDialog([...S.sel])">${ICON.up}Update firmware</button>
      <button class="btn small" onclick="rebootPhones([...S.sel])">${ICON.power}Reboot</button>
      <button class="btn small" onclick="setDisabled([...S.sel], true)">${ICON.pause}Disable</button>
      <button class="btn small" onclick="setDisabled([...S.sel], false)">${ICON.play}Enable</button>
      <button class="btn small danger" onclick="deletePhones([...S.sel])">${ICON.trash}Delete</button>
      <button class="btn small" onclick="S.sel.clear();renderPhones(true)">Clear</button></div>` : "";
  const counts = `${d.total}${d.disabled ? ` <span class="faint">(${d.disabled} disabled)</span>` : ""}${S.query ? ` <span class="faint">${rows.length} shown</span>` : ""}`;
  if (soft && $("#ptable")) {
    $("#ptable").innerHTML = d.total ? table : emptyList();
    $("#selbar").innerHTML = bar;
    $("#pcount").innerHTML = counts;
    return;
  }
  $("#main").innerHTML = `<div class="page">
    <div class="head"><h1>Phones</h1><span class="sub" id="pcount">${counts}</span><span class="grow"></span>
      <span class="searchbox">${ICON.search}<input type="text" id="psearch" placeholder="Search phones  /" value="${esc(S.query)}" oninput="S.query=this.value;renderPhones(true)" title="Name, extension, address, site, model, firmware or MAC. Also: online, offline, disabled, problem, unregistered, update"></span>
      <button class="btn" onclick="openScanModal()">${ICON.radar}Scan</button>
      <button class="btn" onclick="openBulkModal()">Add many</button>
      <button class="btn primary" onclick="openPhoneModal()">${ICON.plus}Add phone</button></div>
    <div id="selbar">${bar}</div>
    <div class="panel" id="ptable">${d.total ? table : emptyList()}</div></div>`;
}

function emptyList() {
  return `<div class="empty"><h3>No phones yet</h3><p>Add one by address, scan a network range, or paste a list with Add many.</p></div>`;
}

function clearSearch() {
  S.query = "";
  if ($("#psearch")) $("#psearch").value = "";
  renderPhones(true);
}

function toggleSel(id, on) {
  on ? S.sel.add(id) : S.sel.delete(id);
  renderPhones(true);
}

function selectAll(on) {
  for (const v of S.dash.phones.filter((v) => phoneMatches(v, S.query))) on ? S.sel.add(v.id) : S.sel.delete(v.id);
  renderPhones(true);
}

function nameOf(id) {
  const v = S.dash.phones.find((x) => x.id === id);
  return v ? nameOfView(v) : id;
}

async function setDisabled(ids, disabled) {
  if (!ids.length) return;
  try {
    await call("SetDisabled", ids, disabled);
    toast(`${disabled ? "Disabled" : "Enabled"} ${ids.length === 1 ? nameOf(ids[0]) : phonesWord(ids.length)}`);
    S.dash = await call("GetDashboard");
    renderSide();
    if ((S.route[0] || "") === "phones") renderPhones(true);
    else if (S.route[0] === "phone") loadDetail(S.route[1]);
  } catch (e) { toast(e.message, true); }
}

async function deletePhones(ids) {
  if (!ids.length) return;
  const what = ids.length === 1 ? nameOf(ids[0]) : phonesWord(ids.length);
  if (!confirm(`Delete ${what} from PhoneDesk, with the saved copies of their settings? The phones themselves aren't changed.`)) return;
  try {
    await call("DeletePhones", ids);
    ids.forEach((id) => S.sel.delete(id));
    toast(`Deleted ${what}`);
    S.dash = await call("GetDashboard");
    renderSide();
    if (S.route[0] === "phone") location.hash = "#/phones";
    else renderPhones(true);
  } catch (e) { toast(e.message, true); }
}

async function refreshSel() {
  const ids = [...S.sel].filter((id) => !S.dash.phones.find((v) => v.id === id)?.disabled);
  for (const id of ids) await call("Refresh", id);
  toast(`Reading ${phonesWord(ids.length)}`);
}

async function rebootPhones(ids) {
  if (!ids.length) return;
  const what = ids.length === 1 ? nameOf(ids[0]) : phonesWord(ids.length);
  if (!confirm(`Reboot ${what}? Calls in progress will drop.`)) return;
  let ok = 0;
  const errs = [];
  for (const id of ids) {
    try { await call("Reboot", id); ok++; } catch (e) { errs.push(`${nameOf(id)}: ${e.message}`); }
  }
  if (ok) toast(`Rebooting ${ok === 1 ? what : phonesWord(ok)}`);
  if (errs.length) toast(errs.join("\n"), true);
}

// ---------- phone detail ----------
async function loadDetail(id, soft) {
  try {
    S.detail = await call("GetPhone", id);
  } catch (e) {
    $("#main").innerHTML = `<div class="page"><div class="empty"><h3>Phone not found</h3><a data-go="#/phones">Back to phones</a></div></div>`;
    return;
  }
  renderDetail(soft);
}

function updateDetailHead() {
  const v = S.dash?.phones.find((x) => x.id === S.detail.id);
  if (v && $("#detail-status")) $("#detail-status").innerHTML = detailStatus(v);
}

function detailStatus(v) {
  if (v.disabled) return `<span class="faint">Disabled</span>`;
  if (v.status.collecting) return `<span class="faint">Reading…</span>`;
  if (!v.status.checked) return `<span class="faint">Checking…</span>`;
  return v.status.online ? `<span class="ok">Online</span> <span class="faint">${v.status.latencyMs} ms</span>` : `<span class="bad">Offline since ${new Date(v.status.since).toLocaleString()}</span>`;
}

const TABS = [["overview", "Overview"], ["accounts", "Accounts"], ["settings", "Settings"], ["copies", "Copies"], ["console", "Console"], ["raw", "Raw output"]];

function renderDetail(soft) {
  const d = S.detail, snap = d.snapshot, s = d.summary;
  const name = nameOfView(d);
  const counts = { accounts: snap?.accounts?.length, copies: d.backups?.length };
  const body = snap ? tabBody(S.tab, d, snap) : noDataYet(d);
  if (soft && $("#tabbody")) {
    $("#tabbody").innerHTML = body;
    $("#detail-status").innerHTML = detailStatus(d);
    afterTab();
    return;
  }
  const banners = [];
  if (d.disabled) banners.push(`<div class="banner" style="background:rgba(139,149,167,.08);border-color:var(--line)"><div class="grow"><b>This phone is disabled.</b> PhoneDesk doesn't check or read it and raises no problems for it. The data below is from its last reading.</div><button class="btn" onclick="setDisabled(['${d.id}'], false)">${ICON.play}Enable</button></div>`);
  if (d.status.authFailed) banners.push(`<div class="banner"><div class="grow"><b>Login failed.</b> ${esc(d.status.collectError)}. Fix the credentials, then read again.</div><button class="btn" onclick="openPhoneModal(S.detail)">Edit phone</button></div>`);
  $("#main").innerHTML = `<div class="page">
    <div class="head"><button class="btn small" data-go="#/phones" title="Back">${ICON.back}</button><h1>${esc(name)}</h1><span id="detail-status">${detailStatus(d)}</span><span class="grow"></span>
      ${d.disabled ? "" : `<button class="btn" onclick="collectOne(this)">${ICON.refresh}Read now</button>`}
      ${d.disabled ? "" : `<button class="btn" onclick="rebootPhones(['${d.id}'])">${ICON.power}Reboot</button>`}
      <button class="btn" onclick="call('OpenPhoneWeb', '${d.id}')" title="Open the phone's own web UI">${ICON.ext2}Web UI</button>
      <button class="btn" onclick="setDisabled(['${d.id}'], ${!d.disabled})">${d.disabled ? ICON.play + "Enable" : ICON.pause + "Disable"}</button>
      <button class="btn" onclick="openPhoneModal(S.detail)">Edit</button></div>
    <div class="idline"><span class="mono"><b>${esc(d.host)}</b>${d.port ? ":" + d.port : ""}</span>
      ${s ? `<span><b>${esc(s.model || "Unknown model")}</b></span><span class="mono">${esc(s.firmware)}</span>${s.mac ? `<span class="mono">${esc(s.mac)}</span>` : ""}${s.uptime ? `<span>Up ${esc(s.uptime)}</span>` : ""}` : ""}
      ${d.site ? `<span>Site <b>${esc(d.site)}</b></span>` : ""}
      ${s ? `<span class="faint">Read ${ago(s.collectedAt)}${snap?.duration ? ` in ${snap.duration}` : ""} over ${(s.scheme || "").toUpperCase()}</span>` : ""}</div>
    ${banners.join("")}
    <div class="tabs">${TABS.map(([k, l]) => `<button class="tab${S.tab === k ? " on" : ""}" data-go="#/phone/${d.id}/${k}">${l}${counts[k] ? `<span class="n">${counts[k]}</span>` : ""}</button>`).join("")}</div>
    <div id="tabbody">${body}</div></div>`;
  afterTab();
}

function noDataYet(d) {
  return `<div class="panel"><div class="empty"><h3>No data read yet</h3>
    <p>${d.status.collectError ? esc(d.status.collectError) : d.status.online ? "The first read starts automatically. Use Read now to start it immediately." : "The phone isn't answering on its web port right now."}</p>
    <button class="btn" onclick="collectOne()">${ICON.refresh}Read now</button></div></div>`;
}

async function collectOne(btn) {
  btn?.classList.add("spin");
  await call("Refresh", S.detail.id);
  toast("Reading " + nameOfView(S.detail));
  setTimeout(() => btn?.classList.remove("spin"), 1500);
}

function tabBody(tab, d, snap) {
  switch (tab) {
    case "accounts": return tabAccounts(d, snap);
    case "settings": return tabSettings(d, snap);
    case "copies": return tabCopies(d, snap);
    case "console": return tabConsole(d);
    case "raw": return tabRaw(snap);
    default: return tabOverview(d, snap);
  }
}

function checkRow(good, text, detail) {
  const ico = good === null ? `<span class="ico faint">–</span>` : good ? `<span class="ico ok">✓</span>` : `<span class="ico bad">✕</span>`;
  return `<div class="check-row">${ico}<span>${text}${detail ? ` <span class="faint">${esc(detail)}</span>` : ""}</span></div>`;
}

function portSeen(state, port) {
  return { open: `port ${port} answers`, closed: `port ${port} refused`, filtered: `no answer on port ${port}` }[state] || "";
}

function tabOverview(d, snap) {
  const f = snap.facts, sec = snap.security, up = snap.upgrade, t = snap.time;
  const b = (v) => (v === null || v === undefined ? null : v);
  return `${keys(d.summary, true)}<div style="height:16px"></div>
  <div class="grid2">
    <section class="panel"><h2>Problems on this phone</h2>
      ${d.issues.length ? d.issues.map(issueRow).join("") : `<div class="empty" style="padding:22px"><h3>Nothing to fix</h3></div>`}</section>
    <section class="panel"><h2>System</h2><div class="body"><dl class="kv">
      <dt>Model</dt><dd>${esc(f.model || "?")}${f.vendor ? ` <span class="faint">${esc(f.vendor)}</span>` : ""}</dd>
      <dt>Firmware</dt><dd class="mono">${esc(f.firmware || "?")}</dd>
      ${f.boot ? `<dt>Boot / core</dt><dd class="mono">${esc(f.boot)}${f.core ? " / " + esc(f.core) : ""}</dd>` : ""}
      ${f.base || f.dsp ? `<dt>Base / DSP</dt><dd class="mono">${esc(f.base || "–")} / ${esc(f.dsp || "–")}</dd>` : ""}
      <dt>MAC</dt><dd class="mono">${esc(f.mac || d.mac || "?")}</dd>
      ${f.ip ? `<dt>IP address</dt><dd class="mono">${esc(f.ip)}</dd>` : ""}
      ${f.hardware ? `<dt>Hardware</dt><dd>${esc(f.hardware)}</dd>` : ""}${f.pn ? `<dt>Part number</dt><dd class="mono">${esc(f.pn)}</dd>` : ""}
      ${f.uptime ? `<dt>Uptime</dt><dd>${esc(f.uptime)}</dd>` : ""}
      <dt>Address type</dt><dd>${esc(snap.network.addressType || "?")}${snap.network.vlan && snap.network.vlan !== "0" ? ` <span class="faint">VLAN ${esc(snap.network.vlan)}</span>` : ""}</dd>
      ${snap.network.lldp !== null && snap.network.lldp !== undefined ? `<dt>LLDP</dt><dd>${snap.network.lldp ? "On" : "Off"}</dd>` : ""}
    </dl></div></section>
    <section class="panel"><h2>Accounts<span class="grow"></span><a data-go="#/phone/${d.id}/accounts" style="font-weight:400;font-size:13px">Details</a></h2>
      ${snap.accounts?.length ? `<table><tbody>${snap.accounts.map((a) => `<tr><td class="faint" style="width:34px">${a.index}</td><td class="mono">${esc(a.userId || "–")}</td><td>${esc(a.display || a.name || "")}</td><td class="mono muted">${esc(a.server || "")}</td><td style="text-align:right">${regChip(a)}</td></tr>`).join("")}</tbody></table>` : `<div class="empty">No accounts configured.</div>`}
      ${!snap.registrationKnown ? `<div class="faint" style="padding:8px 16px 10px;font-size:12.5px">Registration state needs Action URI support on the phone (Settings → Remote control). Until then only the configuration is shown.</div>` : ""}</section>
    ${firmwareCard(d)}
    <section class="panel"><h2>Management access</h2><div class="body">
      ${checkRow(!sec.defaultAdmin, sec.defaultAdmin ? "Default admin password (admin/admin)" : "Admin password changed from the default")}
      ${checkRow(sec.httpsPort === "open" ? true : sec.httpsPort ? false : null, sec.httpsPort === "open" ? "HTTPS web access" : "HTTPS not answering", portSeen(sec.httpsPort, 443))}
      ${checkRow(sec.httpPort !== "open", sec.httpPort === "open" ? "Plain HTTP web access is open" : "Plain HTTP closed", portSeen(sec.httpPort, 80))}
      ${checkRow(sec.sshPort !== "open" ? true : null, sec.sshPort === "open" ? "SSH is open" : "SSH closed", portSeen(sec.sshPort, 22))}
      ${checkRow(sec.telnetPort !== "open", sec.telnetPort === "open" ? "Telnet is open" : "Telnet closed", portSeen(sec.telnetPort, 23))}
      ${checkRow(!!t.ntpServer, t.ntpServer ? "Time sync" : "No NTP server", t.ntpServer ? `${t.ntpServer}${t.zone ? ", " + t.zone : ""}` : "")}
      ${checkRow(snap.logging.server ? true : null, snap.logging.server ? "Remote syslog" : "No syslog server", snap.logging.server ? `${snap.logging.server} (${snap.logging.level})` : "")}
      ${checkRow(up.configPath ? true : null, up.configPath ? "Provisioning server" : "No provisioning server", up.configPath ? `${up.configPath} over ${up.configVia}` : "")}
      ${checkRow(null, "Firmware server", up.firmwarePath ? `${up.firmwarePath} over ${up.firmwareVia}` : "none")}
      ${b(up.option66) !== null ? checkRow(!up.option66, up.option66 ? "DHCP option 66/43 may override the servers" : "DHCP option 66/43 ignored") : ""}
    </div></section>
    ${snap.lines?.length ? `<section class="panel"><h2>Lines</h2><table><tbody>${snap.lines.map((l) => `<tr><td class="faint" style="width:34px">${l.line}</td><td class="${l.state === "failed" ? "bad" : l.state === "idle" ? "muted" : "ok"}">${esc(l.state)}</td><td class="faint">account ${l.account + 1}</td><td class="mono">${esc(l.remoteNumber || "")}${l.remoteName ? " " + esc(l.remoteName) : ""}</td></tr>`).join("")}</tbody></table></section>` : ""}
    ${snap.errors?.length ? `<section class="panel"><h2>Reading notes</h2><div class="body faint" style="font-size:13px">${snap.errors.map(esc).join("<br>")}</div></section>` : ""}
  </div>`;
}

function tabAccounts(d, snap) {
  const a = snap.accounts || [];
  return `<section class="panel"><h2>SIP accounts<span class="grow"></span><button class="btn small primary" onclick="openValuesDialog([S.detail.id], accountLines(1))">${ICON.edit}Change account settings</button></h2>
    ${a.length ? `<table><thead><tr><th>#</th><th>State</th><th>Account name</th><th>Display name</th><th>SIP user ID</th><th>Auth ID</th><th>SIP server</th><th>Outbound proxy</th><th></th></tr></thead><tbody>
    ${a.map((x) => `<tr><td class="faint">${x.index}</td><td>${regChip(x)}</td><td>${esc(x.name)}</td><td>${esc(x.display)}</td><td class="mono">${esc(x.userId)}</td><td class="mono">${esc(x.authId)}</td><td class="mono">${esc(x.server)}</td><td class="mono">${esc(x.proxy)}</td>
      <td style="text-align:right"><button class="btn small" onclick="openValuesDialog([S.detail.id], accountLines(${x.index}))">Edit</button></td></tr>`).join("")}
    </tbody></table>` : `<div class="empty">No accounts configured.</div>`}
    <div class="faint" style="padding:10px 16px;font-size:12.5px">Passwords are never read back from the phone, so they don't show here. Editing an account writes only the fields you change.</div></section>`;
}

// template lines for one account, prefilled from the phone
function accountLines(index) {
  const A = [
    ["P271", "P270", "P47", "P48", "P35", "P36", "P34", "P3"],
    ["P401", "P417", "P402", "P403", "P404", "P405", "P406", "P407"],
    ["P501", "P517", "P502", "P503", "P504", "P505", "P506", "P507"],
    ["P601", "P617", "P602", "P603", "P604", "P605", "P606", "P607"],
    ["P1701", "P1717", "P1702", "P1703", "P1704", "P1705", "P1706", "P1707"],
    ["P1801", "P1817", "P1802", "P1803", "P1804", "P1805", "P1806", "P1807"],
  ][index - 1];
  const labels = ["Active (0/1)", "Account name", "SIP server", "Outbound proxy", "SIP user ID", "Authenticate ID", "Authenticate password", "Display name"];
  const vals = S.detail?.snapshot?.values || {};
  return A.map((k, i) => `# Account ${index}: ${labels[i]}\n${k} = ${k === A[6] ? "" : vals[k] ?? ""}`).join("\n") + "\n";
}

function tabSettings(d, snap) {
  const groups = S.params || [];
  const v = snap.values || {};
  const panels = groups.filter((g) => !g.group.startsWith("Account")).map((g) => `<section class="panel"><h2>${esc(g.group)}</h2><div class="body"><dl class="kv">
    ${g.params.map((p) => `<dt title="${p.key}">${esc(p.label)}</dt><dd class="val">${p.secret ? `<span class="unset">not shown</span>` : v[p.key] === undefined ? `<span class="unset">not reported</span>` : v[p.key] === "" ? `<span class="unset">empty</span>` : esc(displayValue(p, v[p.key]))}</dd>`).join("")}
  </dl></div></section>`).join("");
  return `<div class="row" style="margin-bottom:14px;align-items:center"><span class="muted" style="font-size:13px">Settings PhoneDesk knows by name. Everything the phone reported is under Raw output.</span><span class="grow"></span>
    <span style="flex:none;display:flex;gap:8px"><button class="btn" onclick="saveAsTemplate(S.detail.id)">${ICON.tpl}Save as template</button><button class="btn primary" onclick="openValuesDialog([S.detail.id], '')">${ICON.edit}Change settings</button></span></div>
    <div class="grid2">${panels}</div>`;
}

function displayValue(p, v) {
  if (p.options && p.options[v] !== undefined) return p.options[v];
  return v;
}

async function saveAsTemplate(id) {
  try {
    const lines = await call("PhoneValuesText", id);
    openTemplateEditor({ name: `From ${nameOfView(S.detail)}`, notes: `Settings read from ${S.detail.host} on ${new Date().toLocaleDateString()}` }, lines);
  } catch (e) { toast(e.message, true); }
}

let cfgView = { name: "", text: "", compare: "", search: "" };

function currentLines(snap) {
  const v = snap.values || {};
  return Object.keys(v).filter((k) => /^P\d+$/.test(k)).sort((a, b) => +a.slice(1) - +b.slice(1)).map((k) => `${k} = ${v[k]}`).join("\n");
}

function tabCopies(d, snap) {
  if (!cfgView.text || cfgView.forId !== d.id) cfgView = { forId: d.id, name: "", text: currentLines(snap), compare: "", search: "" };
  const list = [`<div class="${!cfgView.name ? "on" : ""}" onclick="showBackup('')">Current settings<div class="faint" style="font-size:12px">${ago(snap.collectedAt)}</div></div>`]
    .concat((d.backups || []).map((b) => `<div class="${cfgView.name === b.name ? "on" : ""}" onclick="showBackup('${b.name}')">${new Date(b.at).toLocaleString()}<div class="faint" style="font-size:12px">${(b.size / 1024).toFixed(1)} KB</div></div>`));
  return `<div class="split"><section class="panel"><h2>Saved copies</h2><div class="list-pick">${list.join("")}</div>
    <div class="faint" style="padding:10px 12px;font-size:12.5px">A copy of the settings is kept each time one changes.</div></section>
    <section><div class="row" style="margin-bottom:10px;align-items:center">
      <input type="text" id="cfgsearch" placeholder="Find in settings" value="${esc(cfgView.search)}" oninput="cfgView.search=this.value;drawConfig()">
      <select id="cfgcmp" onchange="compareBackup(this.value)" style="max-width:260px"><option value="">Compare with…</option>${(d.backups || []).filter((b) => b.name !== cfgView.name).map((b) => `<option value="${b.name}" ${cfgView.compare === b.name ? "selected" : ""}>${new Date(b.at).toLocaleString()}</option>`).join("")}</select>
      <span style="flex:none;display:flex;gap:6px"><button class="btn" onclick="restoreCopy()" title="Write this copy back to the phone">Restore to phone</button><button class="btn" onclick="exportConfig()">Save as file</button></span></div>
      <pre class="code mono" id="cfgpre"></pre></section></div>`;
}

async function showBackup(name) {
  cfgView.name = name;
  cfgView.compare = "";
  cfgView.text = name ? await call("GetBackup", S.detail.id, name) : currentLines(S.detail.snapshot);
  $("#tabbody").innerHTML = tabCopies(S.detail, S.detail.snapshot);
  afterTab();
}

async function compareBackup(name) {
  cfgView.compare = name;
  cfgView.cmpText = name ? await call("GetBackup", S.detail.id, name) : "";
  drawConfig();
}

function drawConfig() {
  const pre = $("#cfgpre");
  if (!pre) return;
  const strip = (t) => t.split("\n").filter((l) => !l.startsWith("#")).join("\n");
  if (cfgView.compare) {
    const ops = diffLines(strip(cfgView.cmpText).split("\n"), strip(cfgView.text).split("\n"));
    const changed = ops.filter((o) => o[0] !== " ").length;
    pre.innerHTML = changed ? ops.map(([t, l]) => t === "+" ? `<span class="add">+ ${esc(l)}</span>` : t === "-" ? `<span class="del">- ${esc(l)}</span>` : `  ${esc(l)}\n`).join("") : "No differences.";
    return;
  }
  const q = cfgView.search.trim();
  let html = esc(cfgView.text || "No settings read.");
  if (q) html = html.replace(new RegExp(esc(q).replace(/[.*+?^${}()|[\]\\]/g, "\\$&"), "gi"), (m) => `<mark>${m}</mark>`);
  pre.innerHTML = html;
  pre.querySelector("mark")?.scrollIntoView({ block: "center" });
}

function diffLines(a, b) {
  const n = a.length, m = b.length;
  if (n * m > 9e6) return [["-", `(${n} lines)`], ["+", `(${m} lines) too large to compare here`]];
  const dp = Array.from({ length: n + 1 }, () => new Uint16Array(m + 1));
  for (let i = n - 1; i >= 0; i--) for (let j = m - 1; j >= 0; j--) dp[i][j] = a[i] === b[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
  const out = [];
  let i = 0, j = 0;
  while (i < n && j < m) {
    if (a[i] === b[j]) { out.push([" ", a[i]]); i++; j++; }
    else if (dp[i + 1][j] >= dp[i][j + 1]) out.push(["-", a[i++]]);
    else out.push(["+", b[j++]]);
  }
  while (i < n) out.push(["-", a[i++]]);
  while (j < m) out.push(["+", b[j++]]);
  return out;
}

function restoreCopy() {
  if (!cfgView.name) { toast("Pick a saved copy to restore"); return; }
  openValuesDialog([S.detail.id], cfgView.text);
}

async function exportConfig() {
  const d = S.detail;
  const name = `${(d.name || d.summary?.extension || d.host).replace(/[^\w.-]/g, "_")}-${cfgView.name ? cfgView.name.replace(".cfg", "") : "current"}.cfg`;
  try {
    const p = await call("ExportText", name, cfgView.text);
    toast("Saved to " + p);
  } catch (e) { if (e.message !== "cancelled") toast(e.message, true); }
}

function tabConsole(d) {
  return `<section class="panel" style="margin-bottom:16px"><h2>Read values<span class="grow"></span><span class="faint" style="font-weight:400;font-size:13px">Any P-value or status key, straight from the phone</span></h2><div class="body">
    <form class="row" onsubmit="runGet(event)" style="align-items:center"><input type="text" id="cmd" class="mono" placeholder="P47 P35 prog mac" autocomplete="off" spellcheck="false" onkeydown="cmdHistory(event)"><span style="flex:none"><button class="btn primary" id="runbtn">Read</button></span></form>
    <div style="height:12px"></div><pre class="code mono console-out" id="cmdout">${esc(S.consoleOut || "Values appear here.")}</pre></div></section>
    <div class="grid2">
    <section class="panel"><h2>Write values</h2><div class="body">
      <p class="muted" style="margin-top:0;font-size:13px">Lines like <code>P30 = pool.ntp.org</code>. You'll see what changes before anything is sent.</p>
      <button class="btn primary" onclick="openValuesDialog([S.detail.id], '')">${ICON.edit}Write values…</button></div></section>
    <section class="panel"><h2>Dial</h2><div class="body">
      <form class="row" onsubmit="dialNumber(event)" style="align-items:center"><input type="text" id="dialnum" class="mono" placeholder="Number to call" autocomplete="off"><span style="flex:none"><button class="btn">Call from this phone</button></span></form>
      <p class="faint" style="font-size:12.5px;margin:8px 0 0">Needs click-to-dial and Action URI support on the phone.</p></div></section></div>`;
}

let histPos = -1;
function cmdHistory(e) {
  if (e.key !== "ArrowUp" && e.key !== "ArrowDown") return;
  e.preventDefault();
  const h = S.consoleHist;
  if (!h.length) return;
  histPos = e.key === "ArrowUp" ? Math.min(histPos + 1, h.length - 1) : Math.max(histPos - 1, -1);
  e.target.value = histPos < 0 ? "" : h[histPos];
}

async function runGet(e) {
  e.preventDefault();
  const cmd = $("#cmd").value.trim();
  if (!cmd) return;
  S.consoleHist = [cmd, ...S.consoleHist.filter((c) => c !== cmd)].slice(0, 50);
  histPos = -1;
  const btn = $("#runbtn");
  btn.disabled = true;
  $("#cmdout").textContent = "Reading " + cmd + " …";
  try {
    const v = await call("GetValues", S.detail.id, cmd);
    const keys = Object.keys(v).sort((a, b) => (/^P\d+$/.test(a) && /^P\d+$/.test(b) ? +a.slice(1) - +b.slice(1) : a.localeCompare(b)));
    S.consoleOut = `# ${cmd}\n` + (keys.length ? keys.map((k) => `${k} = ${v[k]}`).join("\n") : "(the phone knows none of these keys)");
  } catch (err) {
    S.consoleOut = `# ${cmd}\n${err.message}`;
  }
  btn.disabled = false;
  if ($("#cmdout")) $("#cmdout").textContent = S.consoleOut;
}

async function dialNumber(e) {
  e.preventDefault();
  const num = $("#dialnum").value.trim();
  if (!num) return;
  try { await call("Dial", S.detail.id, num, -1); toast("Dialing " + num); } catch (err) { toast(err.message, true); }
}

function tabRaw(snap) {
  const keys = Object.keys(snap.raw || {}).sort();
  const cur = S.rawKey && keys.includes(S.rawKey) ? S.rawKey : keys[0];
  return `<div class="split"><section class="panel"><div class="list-pick">${keys.map((k) => `<div class="${k === cur ? "on" : ""}" onclick="S.rawKey='${esc(k)}';$('#tabbody').innerHTML=tabRaw(S.detail.snapshot)">${esc(k)}</div>`).join("")}</div></section>
    <pre class="code mono">${esc(pretty(snap.raw?.[cur]) || "(empty)")}</pre></div>`;
}

function pretty(t) {
  try { return JSON.stringify(JSON.parse(t), null, 2); } catch { return t; }
}

function afterTab() {
  if (S.tab === "copies") drawConfig();
  if (S.tab === "console") $("#cmd")?.focus();
}

// ---------- modals ----------
function modal(html, wide) {
  closeModal();
  const v = document.createElement("div");
  v.className = "veil";
  v.innerHTML = `<div class="modal${wide ? " wide" : ""}" role="dialog">${html}</div>`;
  v.addEventListener("click", (e) => { if (e.target === v || e.target.closest("[data-close]")) closeModal(); });
  document.body.appendChild(v);
  S.modalOpen = true;
  setTimeout(() => v.querySelector("input:not([disabled]):not([type=checkbox]):not([type=radio]),select,textarea")?.focus(), 30);
  return v;
}
function closeModal() {
  $(".veil")?.remove();
  if (S.modalOpen) { S.modalOpen = false; refreshData(); }
}
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape" && S.modalOpen) closeModal();
  if (e.key === "/" && !S.modalOpen && !/INPUT|TEXTAREA|SELECT/.test(document.activeElement?.tagName) && $("#psearch")) { e.preventDefault(); $("#psearch").focus(); }
});

function credOptions(sel) {
  return S.creds.map((c) => `<option value="${c.id}" ${c.id === sel ? "selected" : ""}>${esc(c.name)} (${esc(c.username)})</option>`).join("") + `<option value="__new">New credentials…</option>`;
}
const SCHEMES = [["auto", "HTTPS, then HTTP"], ["https", "HTTPS only"], ["http", "HTTP only"]];

async function openPhoneModal(ph) {
  S.creds = await call("Credentials");
  const e = ph || { port: 0, scheme: "auto", credentialId: S.creds[0]?.id };
  const m = modal(`<h2>${ph ? "Edit phone" : "Add phone"}</h2><div class="body">
    <div class="row"><label class="f"><span>Address</span><input type="text" id="sh" class="mono" placeholder="10.0.5.21" value="${esc(e.host || "")}"></label>
      <label class="f" style="flex:0 0 150px"><span>Web access</span><select id="ssch">${SCHEMES.map(([k, l]) => `<option value="${k}" ${k === (e.scheme || "auto") ? "selected" : ""}>${l}</option>`).join("")}</select></label>
      <label class="f" style="flex:0 0 90px"><span>Port <span class="faint">(default)</span></span><input type="number" id="sp" value="${e.port || ""}" placeholder="443"></label></div>
    <label class="f"><span>Name <span class="faint">(optional; the extension is used otherwise)</span></span><input type="text" id="sn" value="${esc(e.name || "")}"></label>
    <div class="row"><label class="f"><span>Credentials <span class="faint">(the phone's admin login)</span></span><select id="sc">${credOptions(e.credentialId)}</select></label>
      <label class="f"><span>Site</span><input type="text" id="ss" value="${esc(e.site || "")}"></label></div>
    <div id="newcred" style="display:none" class="row"><label class="f"><span>Username</span><input type="text" id="cu" value="admin"></label><label class="f"><span>Password</span><input type="password" id="cp"></label></div>
    <div class="row"><label class="f"><span>Notes</span><input type="text" id="so" value="${esc(e.notes || "")}"></label>
      <label class="f" style="flex:0 0 190px"><span>Firmware family <span class="faint">(blank = from model)</span></span><input type="text" id="sfam" class="mono" placeholder="grp2610" value="${esc(e.family || "")}"></label></div>
    <div id="smsg"></div></div>
    <div class="foot">${ph ? `<button class="btn danger" id="sdel">Remove</button>` : ""}<span class="grow"></span><button class="btn" id="stest">Test connection</button><button class="btn" data-close>Cancel</button><button class="btn primary" id="ssave">${ph ? "Save" : "Add phone"}</button></div>`);
  const sc = $("#sc", m);
  const toggle = () => ($("#newcred", m).style.display = sc.value === "__new" || !S.creds.length ? "flex" : "none");
  sc.onchange = toggle;
  if (!S.creds.length) sc.value = "__new";
  toggle();
  const msg = (t, ok) => ($("#smsg", m).innerHTML = `<div class="msg ${ok ? "okm" : "err"}">${esc(t)}</div>`);
  const collect = async () => {
    let cid = sc.value;
    if (cid === "__new") {
      const c = await call("SaveCredential", { name: $("#cu", m).value.trim(), username: $("#cu", m).value.trim(), password: $("#cp", m).value });
      S.creds = await call("Credentials");
      sc.innerHTML = credOptions(c.id);
      sc.value = c.id;
      toggle();
      cid = c.id;
    }
    return { ...(ph || {}), host: $("#sh", m).value.trim(), port: +$("#sp", m).value || 0, scheme: $("#ssch", m).value, name: $("#sn", m).value.trim(), credentialId: cid, site: $("#ss", m).value.trim(), notes: $("#so", m).value.trim(), family: $("#sfam", m).value.trim() };
  };
  $("#stest", m).onclick = async () => {
    const b = $("#stest", m);
    b.disabled = true;
    b.textContent = "Connecting…";
    try { msg(await call("TestPhone", await collect()), true); } catch (err) { msg(err.message); }
    b.disabled = false;
    b.textContent = "Test connection";
  };
  $("#ssave", m).onclick = async () => {
    try {
      const saved = await call("SavePhone", await collect());
      closeModal();
      toast(ph ? "Saved" : "Added " + saved.host + ". Reading it now.");
      if (!ph) location.hash = "#/phone/" + saved.id;
      else loadDetail(saved.id);
    } catch (err) { msg(err.message); }
  };
  if (ph) $("#sdel", m).onclick = async () => {
    if (!confirm(`Remove ${nameOfView(ph)} and its saved copies from PhoneDesk? The phone itself isn't changed.`)) return;
    await call("DeletePhone", ph.id);
    closeModal();
    location.hash = "#/phones";
  };
}

async function openBulkModal() {
  S.creds = await call("Credentials");
  const m = modal(`<h2>Add many phones</h2><div class="body">
    <label class="f"><span>One phone per line: an address, "name, address", or a range like 10.0.5.20-10.0.5.60</span>
    <textarea id="bt" rows="9" placeholder="10.0.5.21\nReception, 10.0.5.22\n10.0.5.30-10.0.5.49"></textarea></label>
    <div class="row"><label class="f"><span>Credentials</span><select id="bc">${credOptions(S.creds[0]?.id)}</select></label>
      <label class="f"><span>Web access</span><select id="bsch">${SCHEMES.map(([k, l]) => `<option value="${k}">${l}</option>`).join("")}</select></label></div>
    <div id="bnew" class="row" style="display:none"><label class="f"><span>Username</span><input type="text" id="bu" value="admin"></label><label class="f"><span>Password</span><input type="password" id="bp"></label></div>
    <label class="f"><span>Site</span><input type="text" id="bs"></label><div id="bmsg"></div></div>
    <div class="foot"><span class="grow"></span><button class="btn" data-close>Cancel</button><button class="btn primary" id="bgo">Add phones</button></div>`);
  const bc = $("#bc", m);
  const toggle = () => ($("#bnew", m).style.display = bc.value === "__new" || !S.creds.length ? "flex" : "none");
  if (!S.creds.length) bc.value = "__new";
  bc.onchange = toggle;
  toggle();
  $("#bgo", m).onclick = async () => {
    try {
      let cid = bc.value;
      if (cid === "__new") cid = (await call("SaveCredential", { name: $("#bu", m).value.trim(), username: $("#bu", m).value.trim(), password: $("#bp", m).value })).id;
      const r = await call("BulkAdd", $("#bt", m).value, cid, $("#bs", m).value.trim(), $("#bsch", m).value);
      if (r.errors?.length) {
        $("#bmsg", m).innerHTML = `<div class="msg err">Added ${r.added}. Skipped:<br>${r.errors.map(esc).join("<br>")}</div>`;
      } else {
        closeModal();
        toast(`Added ${phonesWord(r.added)}. Reading them now.`);
        location.hash = "#/dashboard";
      }
    } catch (err) { $("#bmsg", m).innerHTML = `<div class="msg err">${esc(err.message)}</div>`; }
  };
}

// ---------- scan ----------
async function openScanModal() {
  S.creds = await call("Credentials");
  const m = modal(`<h2>Scan for phones</h2><div class="body">
    <label class="f"><span>Networks or ranges to scan, separated by commas or lines</span><input type="text" id="scr" class="mono" placeholder="10.0.5.0/24, 10.0.6.20-80"></label>
    <p class="faint" style="font-size:12.5px;margin:-4px 0 12px">PhoneDesk knocks on each address's web port and keeps the ones that answer like a Grandstream phone. Nothing is logged in to.</p>
    <div id="scprog"></div>
    <div id="scfound"></div>
    <div id="scadd" style="display:none"><div class="row" style="margin-top:12px"><label class="f"><span>Credentials for the new phones</span><select id="scc">${credOptions(S.creds[0]?.id)}</select></label><label class="f"><span>Site</span><input type="text" id="scs"></label></div>
      <div id="scnew" class="row" style="display:none"><label class="f"><span>Username</span><input type="text" id="scu" value="admin"></label><label class="f"><span>Password</span><input type="password" id="scp"></label></div></div>
    <div id="scmsg"></div></div>
    <div class="foot"><span class="grow"></span><button class="btn" data-close id="sccancel">Cancel</button><button class="btn primary" id="scgo">Scan</button><button class="btn primary" id="scaddbtn" style="display:none">Add selected</button></div>`, true);
  const found = new Map();
  const draw = (p) => {
    $("#scprog", m).innerHTML = p ? `<div class="scanprog"><span>${p.done} of ${p.total} addresses checked</span><div class="bar" style="flex:1;max-width:320px"><i style="width:${p.total ? (p.done / p.total) * 100 : 0}%;background:var(--info)"></i></div></div>` : "";
    for (const f of p?.found || []) found.set(f.host, f);
    const rows = [...found.values()].sort((a, b) => ipKey(a.host) - ipKey(b.host));
    $("#scfound", m).innerHTML = rows.length ? `<div class="pick">${rows.map((f) => `<label><input type="checkbox" value="${esc(f.host)}" ${f.known ? "disabled" : "checked"}><span class="mono">${esc(f.host)}</span><span class="faint">${f.scheme.toUpperCase()}</span><span class="sub">${f.known ? "already added as " + esc(f.name) : "new"}</span></label>`).join("")}</div>` : (p && p.done === p.total ? `<div class="empty">No Grandstream phones answered.</div>` : "");
  };
  const scc = $("#scc", m);
  const toggle = () => ($("#scnew", m).style.display = scc.value === "__new" || !S.creds.length ? "flex" : "none");
  scc.onchange = toggle;
  if (!S.creds.length) scc.value = "__new";
  toggle();
  const off = window.runtime?.EventsOn?.("scan", draw);
  $("#scgo", m).onclick = async () => {
    const b = $("#scgo", m);
    b.disabled = true; b.textContent = "Scanning…";
    $("#sccancel", m).onclick = (e) => { e.stopPropagation(); call("CancelScan"); };
    $("#sccancel", m).removeAttribute("data-close");
    $("#sccancel", m).textContent = "Stop";
    try {
      const res = await call("ScanNetwork", $("#scr", m).value);
      draw({ done: 1, total: 1, found: res });
      $("#scprog", m).innerHTML = `<div class="scanprog">Found ${phonesWord(res.length)}${res.filter((f) => !f.known).length ? "" : ", none new"}.</div>`;
      if (res.some((f) => !f.known)) { $("#scadd", m).style.display = "block"; $("#scaddbtn", m).style.display = "inline-flex"; }
    } catch (e) { $("#scmsg", m).innerHTML = `<div class="msg err">${esc(e.message)}</div>`; }
    b.disabled = false; b.textContent = "Scan again"; b.className = "btn";
    $("#sccancel", m).textContent = "Close";
    $("#sccancel", m).setAttribute("data-close", "");
    $("#sccancel", m).onclick = null;
  };
  $("#scaddbtn", m).onclick = async () => {
    const hosts = $$("#scfound input:checked", m).map((i) => i.value);
    if (!hosts.length) { $("#scmsg", m).innerHTML = `<div class="msg err">Tick the phones to add.</div>`; return; }
    try {
      let cid = scc.value;
      if (cid === "__new") cid = (await call("SaveCredential", { name: $("#scu", m).value.trim(), username: $("#scu", m).value.trim(), password: $("#scp", m).value })).id;
      const r = await call("AddFound", hosts, cid, $("#scs", m).value.trim());
      off?.();
      closeModal();
      toast(`Added ${phonesWord(r.added)}. Reading them now.`);
      location.hash = "#/phones";
    } catch (e) { $("#scmsg", m).innerHTML = `<div class="msg err">${esc(e.message)}</div>`; }
  };
  m.addEventListener("click", (e) => { if (e.target === m) off?.(); });
}

function ipKey(h) {
  const p = h.split(".").map(Number);
  return p.length === 4 ? ((p[0] * 256 + p[1]) * 256 + p[2]) * 256 + p[3] : 0;
}

// ---------- extensions ----------
async function renderExtensions() {
  const rows = await call("Extensions");
  const q = (S.extQuery || "").toLowerCase();
  const shown = rows.filter((r) => !q || [r.userId, r.display, r.server, r.phoneName].join(" ").toLowerCase().includes(q));
  const dups = {};
  for (const r of rows) if (r.active) dups[r.userId + "@" + r.server] = (dups[r.userId + "@" + r.server] || 0) + 1;
  $("#main").innerHTML = `<div class="page"><div class="head"><h1>Extensions</h1><span class="sub">${rows.filter((r) => r.active).length} active across ${phonesWord(new Set(rows.map((r) => r.phoneId)).size)}</span><span class="grow"></span>
      <span class="searchbox">${ICON.search}<input type="text" placeholder="Search extensions" value="${esc(S.extQuery || "")}" oninput="S.extQuery=this.value;renderExtensions()"></span></div>
    ${rows.length ? `<div class="panel"><table><thead><tr><th>Extension</th><th>Display name</th><th>State</th><th>Phone</th><th>Account</th><th>SIP server</th></tr></thead><tbody>
    ${shown.map((r) => `<tr class="click" data-go="#/phone/${r.phoneId}/accounts"><td class="mono">${esc(r.userId || "–")}${dups[r.userId + "@" + r.server] > 1 && r.active ? ` <span class="sev warning" title="Registered from more than one phone">×${dups[r.userId + "@" + r.server]}</span>` : ""}</td><td>${esc(r.display)}</td><td>${regChip(r)}</td><td>${esc(r.phoneName)}</td><td class="faint">${r.account}</td><td class="mono muted">${esc(r.server)}</td></tr>`).join("")}</tbody></table>
    ${shown.length ? "" : `<div class="empty">Nothing matches.</div>`}</div>` : `<div class="panel"><div class="empty"><h3>No extensions yet</h3><p>Accounts appear here once phones have been read.</p></div></div>`}</div>`;
}

// ---------- settings ----------
async function renderSettings() {
  S.creds = await call("Credentials");
  S.settings = await call("GetSettings");
  const set = S.settings;
  const ver = await call("Version");
  const df = await call("DataFolder");
  const fws = await call("FirmwareServer").catch(() => ({}));
  const checks = await call("Checks").catch(() => []);
  const off = new Set(set.checksOff || []);
  const sevWord = { critical: "Critical", warning: "Warning", info: "Note" };
  const checkRows = () => {
    const groups = [...new Set(checks.map((c) => c.group))];
    return groups.map((g) => `<div class="checkgrp"><div class="faint" style="font-size:12px;margin:8px 0 4px">${esc(g)}</div>${checks.filter((c) => c.group === g).map((c) =>
      `<label class="check" style="margin-bottom:5px" title="${esc(c.detail)}"><input type="checkbox" data-check="${c.id}" ${off.has(c.id) ? "" : "checked"}> <span class="sev ${c.severity}" style="min-width:64px;text-align:center">${sevWord[c.severity]}</span> ${esc(c.label)}</label>`).join("")}</div>`).join("");
  };
  $("#main").innerHTML = `<div class="page" style="max-width:900px"><div class="head"><h1>Settings</h1></div>
    <section class="panel" style="margin-bottom:18px"><h2>Credentials<span class="grow"></span><button class="btn small" onclick="editCred()">${ICON.plus}Add</button></h2>
      ${S.creds.length ? `<table><tbody>${S.creds.map((c) => `<tr><td>${esc(c.name)}</td><td class="mono">${esc(c.username)}</td><td style="text-align:right"><button class="btn small" onclick="editCred('${c.id}')">Edit</button> <button class="btn small danger" onclick="delCred('${c.id}')">Remove</button></td></tr>`).join("")}</tbody></table>` : `<div class="empty">No credentials yet. They're added when you add your first phone.</div>`}
      <div class="faint" style="padding:10px 16px;font-size:12.5px">The phone's admin login. Passwords are encrypted on this computer and never shown again.</div></section>
    <section class="panel"><h2>Monitoring</h2><div class="body">
      <div class="row"><label class="f"><span>Reachability check every (seconds)</span><input type="number" id="pi" min="5" value="${set.pingInterval}"></label>
        <label class="f"><span>Read phones every (minutes, 0 = only on demand)</span><input type="number" id="ci" min="0" value="${set.collectInterval}"></label></div>
      <div class="row"><label class="f"><span>Phones read at the same time</span><input type="number" id="pa" min="1" max="16" value="${set.parallel}"></label>
        <label class="f"><span>Settings copies kept per phone</span><input type="number" id="kb" min="1" value="${set.keepBackups}"></label>
        <label class="f"><span>Note when data is older than (hours)</span><input type="number" id="sh" min="1" value="${set.staleHours}"></label></div>
      <div style="margin-bottom:14px"><b style="font-weight:600">Checks</b>
        <p class="faint" style="font-size:12.5px;margin:4px 0 2px">Every rule the dashboard applies. Switch off the ones that don't apply to your network; hover one for what it looks at.</p>
        <div class="checkgrid">${checkRows()}</div></div>
      <div style="border-top:1px solid var(--line-soft);padding-top:14px;margin-bottom:16px"><b style="font-weight:600">Where phones fetch firmware from</b>
        <p class="faint" style="font-size:12.5px;margin:4px 0 10px">During an update the phone is pointed here, rebooted, and the setting is put back afterwards.</p>
        <label class="check" style="margin-bottom:6px"><input type="radio" name="fs" value="builtin" ${set.firmwareSource === "builtin" ? "checked" : ""}> This computer: PhoneDesk serves the library over HTTP <span class="faint">(the phone must reach this computer)</span></label>
        <div class="row" style="max-width:360px;margin:0 0 8px 24px"><label class="f" style="margin-bottom:6px"><span>Port</span><input type="number" id="fp" min="1" max="65535" value="${set.firmwarePort}"></label>
          <label class="f" style="margin-bottom:6px"><span>Server now</span><div style="padding:6px 0;font-size:13px">${fws.running ? `<span class="ok">Running</span> on ${esc(fws.example || ":" + fws.port)} <a onclick="call('StopFirmwareServer').then(renderSettings)">Stop</a>` : `<span class="muted">Stopped</span> <a onclick="call('StartFirmwareServer').then(renderSettings).catch((e)=>toast(e.message,true))">Start</a>`}</div></label></div>
        <label class="check" style="margin-bottom:6px"><input type="radio" name="fs" value="grandstream" ${set.firmwareSource === "grandstream" ? "checked" : ""}> Grandstream's public server (firmware.grandstream.com) <span class="faint">(the phone needs internet access; installs whatever is current there)</span></label>
        <label class="check" style="margin-bottom:6px"><input type="radio" name="fs" value="custom" ${set.firmwareSource === "custom" ? "checked" : ""}> Another server</label>
        <div class="row" style="max-width:480px;margin-left:24px"><label class="f"><span>Server path <span class="faint">(host or host/folder)</span></span><input type="text" id="fsrv" class="mono" value="${esc(set.firmwareServer || "")}" placeholder="fw.example.com/grandstream"></label>
          <label class="f" style="flex:0 0 120px"><span>Via</span><select id="fvia">${["http", "https", "tftp"].map((v) => `<option value="${v}" ${set.firmwareVia === v ? "selected" : ""}>${v.toUpperCase()}</option>`).join("")}</select></label></div></div>
      <button class="btn primary" onclick="saveSettings()">Save settings</button></div></section>
    <section class="panel" style="margin-top:18px"><h2>Data</h2><div class="body">
      <label class="f"><span>Data folder${df.custom ? "" : ' <span class="faint">(default)</span>'}</span>
        <div class="row" style="align-items:center"><input type="text" class="mono" readonly value="${esc(df.path)}"><span style="flex:none;display:flex;gap:6px">
        <button class="btn" onclick="pickDataFolder()">Change…</button><button class="btn" onclick="call('OpenDataFolder')">Open</button>
        ${df.custom ? `<button class="btn" onclick="resetDataFolder()">Use default</button>` : ""}</span></div></label>
      <p class="faint" style="font-size:12.5px;margin:-4px 0 18px">Holds your phones, encrypted credentials, templates, saved settings copies and the firmware library. A folder on another disk or a synced drive works; the encryption key travels with it.</p>
      <div class="row" style="align-items:stretch">
        <div class="panel" style="padding:14px 16px;background:var(--panel-2)"><b style="font-weight:600">Export to another computer</b>
          <p class="muted" style="font-size:13px;margin:6px 0 12px">Saves everything to one .pdbak file. Passwords inside are protected by a passphrase you choose here and need again when importing.</p>
          <button class="btn primary" onclick="openExport()">Export…</button></div>
        <div class="panel" style="padding:14px 16px;background:var(--panel-2)"><b style="font-weight:600">Import from a file</b>
          <p class="muted" style="font-size:13px;margin:6px 0 12px">Add the phones and templates from a .pdbak file to this computer, or replace everything here with its contents.</p>
          <button class="btn" onclick="openImport()">Import…</button></div>
      </div></div></section>
    <section class="panel about" style="margin-top:18px"><div class="body">
      <a class="about-logo" onclick="openURL('https://clihero.com')" title="clihero.com"><img src="clihero-logo-white.png" alt="clihero.com"></a>
      <div><b>PhoneDesk ${esc(ver)}</b>
        <div class="muted" style="font-size:13px;margin-top:2px">Built by Jaba Macharashvili · <a onclick="openURL('https://clihero.com')">clihero.com</a> · <a onclick="openURL('https://jaba0x.github.io/PhoneDesk/')">Documentation</a> · <a onclick="openURL('https://github.com/jaba0x/PhoneDesk')">GitHub</a> · <a onclick="openURL('https://github.com/jaba0x/SwitchDesk')">SwitchDesk</a></div>
        <div style="margin-top:10px;display:flex;gap:10px;align-items:center"><button class="btn small" onclick="checkUpdate(this)">${ICON.refresh}Check for updates</button><span id="updres" class="muted" style="font-size:13px"></span></div></div>
    </div></section></div>`;
}

async function saveSettings() {
  try {
    await call("SaveSettings", { ...S.settings, pingInterval: +$("#pi").value, collectInterval: +$("#ci").value, parallel: +$("#pa").value, keepBackups: +$("#kb").value, staleHours: +$("#sh").value,
      checksOff: $$("input[data-check]").filter((x) => !x.checked).map((x) => x.dataset.check),
      firmwareSource: $("input[name=fs]:checked").value, firmwarePort: +$("#fp").value, firmwareServer: $("#fsrv").value.trim(), firmwareVia: $("#fvia").value });
    S.settings = await call("GetSettings");
    toast("Settings saved");
  } catch (e) { toast(e.message, true); }
}

// ---------- data folder, export, import ----------
async function pickDataFolder() {
  let info;
  try { info = await call("PickDataFolder"); } catch (e) { toast(e.message, true); return; }
  if (!info) return;
  confirmFolder(info, false);
}

async function resetDataFolder() {
  confirmFolder(await call("DefaultFolderInfo"), true);
}

function confirmFolder(info, isDefault) {
  if (info.same) { toast("That's already the data folder"); return; }
  if (!info.writable) { toast("PhoneDesk can't write to " + info.path, true); return; }
  const where = isDefault ? "the default folder" : "this folder";
  const choices = info.hasData
    ? `<p>${isDefault ? "The default folder" : "This folder"} already has PhoneDesk data with ${phonesWord(info.phones)}.</p>
       <label class="check" style="margin-bottom:8px"><input type="radio" name="fm" value="use" checked> Use the data that's there</label>
       <label class="check"><input type="radio" name="fm" value="move"> Replace it with my current data</label>`
    : `<p>Your current data will be copied to ${where}.</p>
       <label class="check" style="margin-bottom:8px"><input type="radio" name="fm" value="move" checked> Move it, and remove it from the old folder</label>
       <label class="check"><input type="radio" name="fm" value="copy"> Copy it, and leave the old folder as it is</label>`;
  const m = modal(`<h2>Change data folder</h2><div class="body"><p class="mono" style="margin-top:0;user-select:text;word-break:break-all">${esc(info.path)}</p>${choices}<div id="fmsg"></div></div>
    <div class="foot"><span class="grow"></span><button class="btn" data-close>Cancel</button><button class="btn primary" id="fgo">Switch folder</button></div>`);
  $("#fgo", m).onclick = async () => {
    const mode = $("input[name=fm]:checked", m).value;
    const b = $("#fgo", m);
    b.disabled = true; b.textContent = "Switching…";
    try {
      await call("ChangeDataFolder", info.path, mode);
      closeModal();
      toast("Now using " + info.path);
      renderSettings();
    } catch (e) {
      $("#fmsg", m).innerHTML = `<div class="msg err">${esc(e.message)}</div>`;
      b.disabled = false; b.textContent = "Switch folder";
    }
  };
}

function openExport() {
  const m = modal(`<h2>Export to another computer</h2><div class="body">
    <div class="row"><label class="f"><span>Passphrase</span><input type="password" id="ep1" autocomplete="new-password"></label>
    <label class="f"><span>Repeat passphrase</span><input type="password" id="ep2" autocomplete="new-password"></label></div>
    <p class="faint" style="font-size:12.5px;margin:-4px 0 14px">At least 8 characters. Without it, the passwords in the file can't be read, so keep it somewhere safe.</p>
    <label class="check"><input type="checkbox" id="eh" checked> Include saved copies and last readings</label><div id="emsg"></div></div>
    <div class="foot"><span class="grow"></span><button class="btn" data-close>Cancel</button><button class="btn primary" id="ego">Export…</button></div>`);
  $("#ego", m).onclick = async () => {
    const p1 = $("#ep1", m).value, p2 = $("#ep2", m).value;
    const err = (t) => ($("#emsg", m).innerHTML = `<div class="msg err">${esc(t)}</div>`);
    if (p1.length < 8) return err("Use at least 8 characters.");
    if (p1 !== p2) return err("The passphrases don't match.");
    try {
      const path = await call("ExportData", p1, $("#eh", m).checked);
      closeModal();
      toast("Exported to " + path);
    } catch (e) { if (e.message !== "cancelled") err(e.message); }
  };
}

async function openImport() {
  let path;
  try { path = await call("PickImportFile"); } catch (e) { toast(e.message, true); return; }
  if (!path) return;
  const m = modal(`<h2>Import from a file</h2><div class="body">
    <p class="mono" style="margin-top:0;user-select:text;word-break:break-all">${esc(path)}</p>
    <label class="f"><span>Passphrase used for the export</span><input type="password" id="ip" autocomplete="off"></label>
    <div id="iinfo"></div>
    <label class="check" style="margin-bottom:8px"><input type="radio" name="im" value="merge" checked> Add its phones and templates to the ones here <span class="faint">(phones already here are skipped)</span></label>
    <label class="check"><input type="radio" name="im" value="replace"> Replace everything here with the file's contents</label>
    <div id="imsg"></div></div>
    <div class="foot"><span class="grow"></span><button class="btn" data-close>Cancel</button><button class="btn primary" id="igo">Import</button></div>`);
  const err = (t) => ($("#imsg", m).innerHTML = `<div class="msg err">${esc(t)}</div>`);
  $("#ip", m).onchange = async () => {
    try {
      const i = await call("InspectImport", path, $("#ip", m).value);
      $("#iinfo", m).innerHTML = `<p class="muted" style="font-size:13px;margin:-4px 0 14px">${phonesWord(i.phones)}, exported ${new Date(i.createdAt).toLocaleString()}${i.history ? ", with saved copies" : ""}.</p>`;
      $("#imsg", m).innerHTML = "";
    } catch (e) { err(e.message); }
  };
  $("#igo", m).onclick = async () => {
    const mode = $("input[name=im]:checked", m).value;
    if (mode === "replace" && !confirm("Replace all phones, credentials, templates and settings on this computer with the file's contents?")) return;
    try {
      const r = await call("ImportData", path, $("#ip", m).value, mode);
      closeModal();
      toast(`Imported ${phonesWord(r.added)}${r.skipped?.length ? `, skipped ${r.skipped.length} already here` : ""}`);
      location.hash = "#/dashboard";
    } catch (e) { err(e.message === "wrong passphrase" ? "That passphrase doesn't open this file." : e.message); }
  };
}

function editCred(id) {
  const c = S.creds.find((x) => x.id === id) || {};
  const m = modal(`<h2>${id ? "Edit credentials" : "Add credentials"}</h2><div class="body">
    <label class="f"><span>Name</span><input type="text" id="cn" value="${esc(c.name || "")}" placeholder="Office phones"></label>
    <div class="row"><label class="f"><span>Username</span><input type="text" id="cu2" value="${esc(c.username || "admin")}"></label>
    <label class="f"><span>Password${id ? ' <span class="faint">(leave empty to keep)</span>' : ""}</span><input type="password" id="cp2"></label></div><div id="cmsg"></div></div>
    <div class="foot"><span class="grow"></span><button class="btn" data-close>Cancel</button><button class="btn primary" id="csave">Save</button></div>`);
  $("#csave", m).onclick = async () => {
    try {
      await call("SaveCredential", { id: id || "", name: $("#cn", m).value.trim(), username: $("#cu2", m).value.trim(), password: $("#cp2", m).value });
      closeModal();
      renderSettings();
    } catch (e) { $("#cmsg", m).innerHTML = `<div class="msg err">${esc(e.message)}</div>`; }
  };
}

async function delCred(id) {
  try { await call("DeleteCredential", id); renderSettings(); } catch (e) { toast(e.message, true); }
}

// ---------- app updates ----------
function openURL(u) { call("OpenURL", u).catch(() => {}); }

function skippedVersion() {
  try { return localStorage.getItem("pd-skip-version") || ""; } catch { return ""; }
}

function showUpdate(u, force) {
  const bar = $("#updbar");
  if (!bar || !u?.available) return;
  if (!force && skippedVersion() === u.latest) return;
  S.update = u;
  bar.innerHTML = `<div class="updbar"><b>PhoneDesk ${esc(u.latest)} is available.</b><span>You have ${esc(u.current)}.</span><span class="grow"></span>
    <button class="ubtn" onclick="openURL('${esc(u.url)}')">Download update</button>
    <button class="ulink" onclick="hideUpdate()">Later</button></div>`;
  $("#content").classList.add("hasupd");
}

function hideUpdate() {
  try { localStorage.setItem("pd-skip-version", S.update?.latest || ""); } catch {}
  $("#updbar").innerHTML = "";
  $("#content").classList.remove("hasupd");
}

async function checkUpdate(btn) {
  btn?.classList.add("spin");
  const res = $("#updres");
  try {
    const u = await call("CheckForUpdate");
    if (u.available) {
      showUpdate(u, true);
      if (res) res.innerHTML = `Version ${esc(u.latest)} is available.`;
    } else if (res) res.textContent = `You're on the latest version (${u.current}).`;
  } catch (e) { if (res) res.textContent = e.message; }
  btn?.classList.remove("spin");
}

// ---------- boot ----------
async function boot() {
  for (let i = 0; i < 100 && !window.go?.main?.App; i++) await new Promise((r) => setTimeout(r, 30));
  api = window.go?.main?.App;
  if (!api) {
    $("#main").innerHTML = `<div class="page"><div class="empty"><h3>Can't reach the PhoneDesk backend</h3><p>Open this page through the PhoneDesk app.</p></div></div>`;
    return;
  }
  const plat = await call("Platform");
  if (plat === "darwin") document.documentElement.style.setProperty("--top", "22px");
  S.settings = await call("GetSettings");
  S.params = await call("KnownParams").catch(() => []);
  S.dash = await call("GetDashboard");
  window.runtime?.EventsOn?.("changed", () => refreshData().catch(() => {}));
  window.runtime?.EventsOn?.("firmware-download", (p) => { if (FW.dl) { FW.dl = p; drawDl(); } });
  window.runtime?.EventsOn?.("update-available", showUpdate);
  call("LastUpdateCheck").then((u) => u && showUpdate(u)).catch(() => {});
  const notice = await call("StartupNotice").catch(() => "");
  if (notice) toast(notice, true);
  setInterval(() => { if (!S.modalOpen && (S.route[0] || "dashboard") === "dashboard") renderDashboard(); }, 30000);
  route();
}
boot();
