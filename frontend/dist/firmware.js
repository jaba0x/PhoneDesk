"use strict";
/* Firmware: library, available updates, update jobs. Uses helpers from app.js. */

const FW = { images: [], jobs: [], dl: null, sel: new Set(), openLog: new Set(), server: {} };

const mb = (n) => (n >= 1 << 20 ? (n / (1 << 20)).toFixed(n > 100 << 20 ? 0 : 1) + " MB" : Math.max(1, Math.round(n / 1024)) + " KB");

// an update can be started when something newer exists and nothing is running
function canUpdate(v) {
  const j = v.firmware?.job;
  return !v.disabled && v.firmware?.available && !(j && (j.state === "running" || j.state === "queued"));
}

// small status cell used in the phone list and on the phone page
function fwCell(v) {
  const f = v.firmware || {};
  const j = f.job;
  if (j && (j.state === "running" || j.state === "queued")) {
    const pct = j.state === "running" && j.progress > 0 && j.progress < 1 ? ` ${Math.round(j.progress * 100)}%` : "";
    return `<span class="fwchip busy" title="${esc(j.step)}">${j.state === "queued" ? "queued" : "updating" + pct}</span>`;
  }
  if (f.available) return `<span class="fwchip avail" title="${esc(f.available)} is in your library">↑ ${esc(f.available)}</span>`;
  if (j && j.state === "failed") return `<span class="fwchip failed" title="${esc(j.error)}">update failed</span>`;
  return "";
}

// ---------- page ----------
async function renderFirmware(soft) {
  try {
    [FW.images, FW.jobs, FW.server] = await Promise.all([call("FirmwareImages"), call("UpgradeJobs"), call("FirmwareServer")]);
  } catch (e) {
    $("#main").innerHTML = `<div class="page"><div class="head"><h1>Firmware</h1></div><div class="panel"><div class="empty">${esc(e.message)}</div></div></div>`;
    return;
  }
  const set = S.settings || (S.settings = await call("GetSettings"));
  if (soft && $("#fw-updates")) {
    $("#fw-updates").innerHTML = updatesPanel();
    $("#fw-jobs").innerHTML = jobsPanel();
    $("#fw-lib").innerHTML = libraryTable();
    $("#fw-src").innerHTML = sourceLine(set);
    return;
  }
  $("#main").innerHTML = `<div class="page">
    <div class="head"><h1>Firmware</h1><span class="grow"></span><span class="muted" style="font-size:13px" id="fw-src">${sourceLine(set)}</span></div>
    <div id="fw-updates">${updatesPanel()}</div>
    <div id="fw-jobs">${jobsPanel()}</div>
    <section class="panel"><h2>Library<span class="grow"></span>
        <button class="btn small" onclick="call('OpenFirmwareFolder')">Open folder</button>
        <button class="btn small primary" onclick="addFirmwareFiles()">${ICON.plus}Add files</button></h2>
      <div class="body" style="padding-bottom:6px">
        <form class="row" style="align-items:center" onsubmit="downloadFirmware(event)">
          <input type="text" id="fw-url" placeholder="Paste a download link: a Release_….zip from grandstream.com/support/firmware, or a …fw.bin" autocomplete="off" spellcheck="false">
          <span style="flex:none;display:flex;gap:6px"><button class="btn" id="fw-dl">Download</button></span></form>
        <div id="fw-dlstate" style="margin-top:8px"></div>
        <p class="faint" style="font-size:12.5px;margin:8px 0 6px">Grandstream publishes firmware freely. Add the release zip or the .bin image; the version comes from the file name, and you're asked for it when it can't be read. Each phone is offered the newest image for its family (a GRP2613 uses grp2610fw.bin, a GXP2170 uses gxp2130fw.bin).</p>
      </div>
      <div id="fw-lib">${libraryTable()}</div></section></div>`;
}

function sourceLine(set) {
  const s = FW.server || {};
  const src = { builtin: `this computer${s.running ? ` (serving on ${esc(s.example || ":" + s.port)})` : ` on port ${s.port || set.firmwarePort}`}`, grandstream: "firmware.grandstream.com", custom: esc(set.firmwareServer || "another server"), relay: `${esc(set.relayServerPath || "the relay server")}, filled over SSH from here` }[set.firmwareSource || "builtin"];
  return `Phones fetch from: <a data-go="#/settings">${src}</a>`;
}

function updatesPanel() {
  const vs = (S.dash?.phones || []).filter((v) => !v.disabled && v.firmware?.supported);
  const avail = vs.filter((v) => v.firmware.available);
  for (const id of FW.sel) if (!avail.find((v) => v.id === id)) FW.sel.delete(id);
  const upToDate = vs.filter((v) => !v.firmware.available && v.firmware.running).length;
  if (!avail.length) {
    return `<section class="panel" style="margin-bottom:18px"><h2>Updates</h2><div class="empty" style="padding:22px"><h3>${vs.length ? "Every phone runs the newest firmware in your library" : "No phones read yet"}</h3>
      <p>${vs.length ? `${phonesWord(upToDate)} checked. Add newer images to the library below to see updates here.` : "Firmware shows up once phones have been read."}</p></div></section>`;
  }
  const all = avail.every((v) => FW.sel.has(v.id));
  return `<section class="panel" style="margin-bottom:18px"><h2>Updates available<span class="sub muted" style="font-weight:400">${avail.length}</span><span class="grow"></span>
      ${FW.sel.size ? `<button class="btn small primary" onclick="openUpgradeDialog([...FW.sel])">${ICON.up}Update ${FW.sel.size} selected</button>` : `<span class="faint" style="font-weight:400;font-size:13px">Tick phones to update them</span>`}</h2>
    <table class="plist"><thead><tr><th class="cb nogo"><input type="checkbox" ${all ? "checked" : ""} onchange="fwSelAll(this.checked)"></th><th>Phone</th><th>Model</th><th>Family</th><th>Running</th><th>Available</th><th>Status</th></tr></thead><tbody>
    ${avail.map((v) => `<tr class="click${FW.sel.has(v.id) ? " picked" : ""}" data-go="#/phone/${v.id}"><td class="cb nogo"><input type="checkbox" ${FW.sel.has(v.id) ? "checked" : ""} onchange="fwSel('${v.id}', this.checked)"></td>
        <td title="${esc(v.host)}">${statusDot(v)} ${esc(nameOfView(v))}</td><td>${esc(v.summary?.model || "")}</td><td class="mono muted">${esc(v.firmware.family)}</td>
        <td class="mono">${esc(v.firmware.running)}</td><td class="mono ok">${esc(v.firmware.available)}</td><td>${fwCell(v)}</td></tr>`).join("")}</tbody></table></section>`;
}

function fwSel(id, on) { on ? FW.sel.add(id) : FW.sel.delete(id); $("#fw-updates").innerHTML = updatesPanel(); }
function fwSelAll(on) {
  for (const v of S.dash.phones.filter((v) => !v.disabled && v.firmware?.available)) on ? FW.sel.add(v.id) : FW.sel.delete(v.id);
  $("#fw-updates").innerHTML = updatesPanel();
}

function jobsPanel() {
  if (!FW.jobs.length) return "";
  const label = { queued: ["Queued", "info"], running: ["Running", "info"], done: ["Done", "ok"], failed: ["Failed", "critical"], cancelled: ["Cancelled", ""] };
  return `<section class="panel" style="margin-bottom:18px"><h2>Update jobs<span class="grow"></span>
      ${FW.jobs.some((j) => !["queued", "running"].includes(j.state)) ? `<button class="btn small" onclick="call('ClearFinishedUpgrades').then(()=>renderFirmware(true))">Clear finished</button>` : ""}</h2>
    ${FW.jobs.map((j) => {
      const [txt, cls] = label[j.state] || [j.state, ""];
      const pct = j.state === "running" && j.progress > 0 && j.progress < 1 ? Math.round(j.progress * 100) : null;
      const open = FW.openLog.has(j.id);
      return `<div class="job">
        <div class="jobline"><span class="sev ${cls === "ok" ? "" : cls} ${cls === "ok" ? "okchip" : ""}">${txt}</span>
          <b>${esc(j.phoneName)}</b><span class="mono muted">${esc(j.from)} → ${esc(j.to)}</span>
          ${j.source ? `<span class="faint mono" style="font-size:12px">from ${esc(j.source)}</span>` : ""}<span class="grow"></span>
          ${j.state === "queued" ? `<button class="btn small" onclick="cancelUpgrade('${j.id}')">Cancel</button>` : ""}
          <a onclick="toggleLog('${j.id}')" style="font-size:13px">${open ? "Hide log" : "Log"}</a></div>
        <div class="jobstep ${j.state === "failed" ? "bad" : "muted"}">${esc(j.state === "failed" ? j.error : j.step)}</div>
        ${pct !== null ? `<div class="bar" style="margin-top:6px;max-width:420px"><i style="width:${pct}%;background:var(--info)"></i></div>` : ""}
        ${open ? `<pre class="code mono" style="margin-top:8px;max-height:220px">${esc((j.log || []).join("\n") || "No entries yet.")}</pre>` : ""}</div>`;
    }).join("")}</section>`;
}

function toggleLog(id) { FW.openLog.has(id) ? FW.openLog.delete(id) : FW.openLog.add(id); $("#fw-jobs").innerHTML = jobsPanel(); }

async function cancelUpgrade(id) {
  try { await call("CancelUpgrade", id); renderFirmware(true); } catch (e) { toast(e.message, true); }
}

function libraryTable() {
  if (!FW.images.length) return `<div class="empty" style="padding:22px">No images yet.</div>`;
  const used = {};
  for (const v of S.dash?.phones || []) if (v.firmware?.family) used[v.firmware.family] = (used[v.firmware.family] || 0) + 1;
  return `<table><thead><tr><th>File</th><th>Family</th><th>Version</th><th>For</th><th class="num">Size</th><th>Added</th><th>From</th><th></th></tr></thead><tbody>
    ${FW.images.map((i) => `<tr><td class="mono">${esc(i.file)}</td><td class="mono">${esc(i.family)}</td><td class="mono">${esc(i.version.text)}</td>
      <td class="muted">${used[i.family] ? phonesWord(used[i.family]) + " of yours" : `<span class="faint">none of your phones</span>`}</td>
      <td class="num">${mb(i.size)}</td><td class="faint">${ago(i.addedAt)}</td><td class="faint">${esc(i.source === "file" ? "added file" : i.source === "zip" ? "release zip" : i.source)}</td>
      <td style="text-align:right"><button class="ib danger" title="Delete" onclick="deleteFirmware('${esc(i.file)}')">${ICON.trash}</button></td></tr>`).join("")}
    </tbody></table>`;
}

async function addFirmwareFiles() {
  try {
    const r = await call("AddFirmwareFiles");
    if (!r) return;
    if (r.added?.length) toast(`Added ${r.added.map((i) => `${i.family} ${i.version.text}`).join(", ")}`);
    if (r.errors?.length) toast(r.errors.join("\n"), true);
    for (const p of r.needVersion || []) await askVersion(p);
  } catch (e) { toast(e.message, true); }
  S.dash = await call("GetDashboard");
  renderFirmware(true);
}

// Grandstream's .bin files don't carry their version in the name, so ask.
function askVersion(path, family) {
  return new Promise((resolve) => {
    const base = path.split(/[\\/]/).pop();
    const m = modal(`<h2>Which version is this?</h2><div class="body">
      <p class="mono" style="margin-top:0;user-select:text;word-break:break-all">${esc(base)}</p>
      <p class="muted" style="font-size:13px">The file name doesn't say. Type the version from Grandstream's download page or release notes.</p>
      <div class="row"><label class="f"><span>Version</span><input type="text" id="av" class="mono" placeholder="1.0.13.31"></label>
        <label class="f"><span>Family <span class="faint">(blank = from the file name)</span></span><input type="text" id="af" class="mono" value="${esc(family || "")}" placeholder="grp2610"></label></div>
      <div id="amsg"></div></div>
      <div class="foot"><span class="grow"></span><button class="btn" data-close>Skip</button><button class="btn primary" id="ago">Add</button></div>`);
    m.addEventListener("click", (e) => { if (e.target === m || e.target.closest("[data-close]")) resolve(false); });
    $("#ago", m).onclick = async () => {
      try {
        const img = await call("AddFirmwareFileAs", path, $("#af", m).value.trim(), $("#av", m).value.trim());
        closeModal();
        toast(`Added ${img.family} ${img.version.text}`);
        resolve(true);
      } catch (e) { $("#amsg", m).innerHTML = `<div class="msg err">${esc(e.message)}</div>`; }
    };
  });
}

async function downloadFirmware(e) {
  e.preventDefault();
  const url = $("#fw-url").value.trim();
  if (!url) return;
  const btn = $("#fw-dl");
  if (FW.dl) { call("CancelDownload"); return; }
  FW.dl = { done: 0, total: -1 };
  btn.textContent = "Cancel";
  drawDl();
  try {
    const img = await call("DownloadFirmware", url, "", "");
    toast(`Downloaded ${img.family} ${img.version.text}`);
    $("#fw-url").value = "";
  } catch (err) {
    if (err.message === "version unknown") {
      // the file is in the library's download folder; ask and add it from there
      FW.dl = null; drawDl();
      if ($("#fw-dl")) $("#fw-dl").textContent = "Download";
      const ver = prompt("The link doesn't say which version this is. Type it (like 1.0.13.31):");
      if (ver) {
        FW.dl = { done: 0, total: -1 }; drawDl();
        try { const img = await call("DownloadFirmware", url, "", ver); toast(`Downloaded ${img.family} ${img.version.text}`); $("#fw-url").value = ""; } catch (e2) { toast(e2.message, true); }
      }
    } else if (err.message !== "cancelled") toast(err.message, true);
  }
  FW.dl = null;
  if ($("#fw-dl")) $("#fw-dl").textContent = "Download";
  drawDl();
  S.dash = await call("GetDashboard");
  renderFirmware(true);
}

function drawDl() {
  const el = $("#fw-dlstate");
  if (!el) return;
  const d = FW.dl;
  if (!d) { el.innerHTML = ""; return; }
  const pct = d.total > 0 ? Math.round((d.done / d.total) * 100) : null;
  el.innerHTML = `<div class="muted" style="font-size:13px;margin-bottom:4px">Downloading… ${mb(d.done)}${d.total > 0 ? ` of ${mb(d.total)}` : ""}</div>
    <div class="bar" style="max-width:420px"><i style="width:${pct ?? 100}%;background:var(--info);${pct === null ? "opacity:.4" : ""}"></i></div>`;
}

async function deleteFirmware(file) {
  if (!confirm(`Delete ${file} from the library?`)) return;
  try { await call("DeleteFirmware", file); } catch (e) { toast(e.message, true); }
  S.dash = await call("GetDashboard");
  renderFirmware(true);
}

// ---------- update dialog ----------
function openUpgradeDialog(ids) {
  const vs = ids.map((id) => S.dash.phones.find((v) => v.id === id)).filter(Boolean);
  const ok = vs.filter((v) => v.firmware?.available && !v.disabled);
  const skipped = vs.length - ok.length;
  if (!ok.length) { toast("None of these phones has a newer image in the library", true); return; }
  const set = S.settings || {};
  const how = { builtin: `Each phone is pointed at this computer (port ${set.firmwarePort}), rebooted, and fetches the image on the way up. The phone has to reach this computer, so keep PhoneDesk open and the firewall out of the way.`,
    grandstream: "Each phone is pointed at firmware.grandstream.com, rebooted, and fetches whatever Grandstream currently publishes for it. The phone needs internet access.",
    custom: `Each phone is pointed at ${esc(set.firmwareServer)}, rebooted, and fetches the image from there.`,
    relay: `The image is first copied over SSH to ${esc(set.relayHost)} (${esc(set.relayDir)}/phonedesk/…), then each phone is pointed at ${esc(set.relayServerPath)} over ${esc((set.relayVia || "tftp").toUpperCase())}, rebooted, and fetches it from there. Nothing has to reach this computer.` }[set.firmwareSource || "builtin"];
  const m = modal(`<h2>Update firmware</h2><div class="body">
    <table style="margin-bottom:14px"><tbody>${ok.map((v) => `<tr><td>${esc(nameOfView(v))}</td><td class="muted">${esc(v.summary?.model || "")}</td><td class="mono muted">${esc(v.firmware.running)}</td><td class="mono ok">→ ${esc(v.firmware.available)}</td></tr>`).join("")}</tbody></table>
    ${skipped ? `<p class="faint" style="font-size:13px;margin-top:-6px">${skipped} selected phone${skipped === 1 ? " has" : "s have"} nothing newer and will be skipped.</p>` : ""}
    <p class="muted" style="font-size:13px;margin-top:0">${how} Phones are updated one at a time, and the upgrade server setting is put back afterwards. Calls in progress drop on reboot.</p>
    <div id="umsg"></div></div>
    <div class="foot"><span class="grow"></span><a data-go="#/settings" style="font-size:13px">Change the source</a><button class="btn" data-close>Cancel</button><button class="btn primary" id="ugo">${ICON.up}Start update</button></div>`, true);
  $("#ugo", m).onclick = async () => {
    try {
      const jobs = await call("StartUpgrade", ok.map((v) => v.id), "");
      closeModal();
      FW.sel.clear();
      toast(`Started ${jobs.length} update${jobs.length === 1 ? "" : "s"}`);
      location.hash = "#/firmware";
    } catch (e) { $("#umsg", m).innerHTML = `<div class="msg err">${esc(e.message)}</div>`; }
  };
}

// firmware card on the phone page
function firmwareCard(d) {
  const f = d.firmware || {};
  if (!f.supported) return `<section class="panel"><h2>Firmware</h2><div class="body muted" style="font-size:13px">Firmware details appear once the phone has been read.</div></section>`;
  const j = f.job;
  return `<section class="panel"><h2>Firmware<span class="grow"></span>${canUpdate(d) ? `<button class="btn small primary" onclick="openUpgradeDialog(['${d.id}'])">${ICON.up}Update</button>` : ""}</h2><div class="body"><dl class="kv">
    <dt>Running</dt><dd class="mono">${esc(f.running || "?")}</dd>
    <dt>Family</dt><dd class="mono">${esc(f.family || "?")} <span class="faint" style="font-family:var(--sans)">${f.family ? f.family + "fw.bin" : ""}</span></dd>
    <dt>Available</dt><dd>${f.available ? `<span class="mono ok">${esc(f.available)}</span>` : `<span class="faint">Up to date with your library</span>`}</dd>
    ${d.snapshot?.upgrade?.firmwarePath ? `<dt>Phone's own server</dt><dd class="mono">${esc(d.snapshot.upgrade.firmwarePath)} <span class="faint" style="font-family:var(--sans)">over ${esc(d.snapshot.upgrade.firmwareVia)}</span></dd>` : ""}
    ${j ? `<dt>Last update</dt><dd>${fwCell(d) || esc(j.state)} <span class="faint">${esc(j.state === "failed" ? j.error : j.step)}</span> <a data-go="#/firmware" style="font-size:13px">Details</a></dd>` : ""}
  </dl></div></section>`;
}
