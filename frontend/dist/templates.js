"use strict";
/* Templates: named sets of P-values, applied to phones or exported as provisioning files. Uses helpers from app.js. */

const TPL = { list: [] };

// ---------- page ----------
async function renderTemplates() {
  TPL.list = await call("Templates");
  const phones = S.dash?.phones || [];
  $("#main").innerHTML = `<div class="page"><div class="head"><h1>Templates</h1><span class="sub">${TPL.list.length}</span><span class="grow"></span>
      <button class="btn primary" onclick="openTemplateEditor()">${ICON.plus}New template</button></div>
    <p class="muted" style="margin-top:-6px">A template is a list of settings, one <code>P-value = value</code> per line. Apply it to any number of phones, or export it as <code>cfg&lt;mac&gt;.xml</code> files for your provisioning server.</p>
    ${TPL.list.length ? `<div class="panel"><table><thead><tr><th>Name</th><th>Settings</th><th>Notes</th><th>Updated</th><th></th></tr></thead><tbody>
      ${TPL.list.map((t) => `<tr><td><b style="font-weight:500">${esc(t.name)}</b></td><td class="muted">${plural(Object.keys(t.values || {}).length, "value")}</td><td class="muted" style="max-width:360px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">${esc(t.notes || "")}</td><td class="faint">${ago(t.updatedAt)}</td>
        <td style="white-space:nowrap;text-align:right"><button class="btn small primary" onclick="openApplyDialog([], '${t.id}')" ${phones.length ? "" : "disabled"}>Apply…</button> <button class="btn small" onclick="openExportDialog('${t.id}')" ${phones.length ? "" : "disabled"}>Provisioning files…</button> <button class="btn small" onclick="editTemplate('${t.id}')">Edit</button> <button class="btn small danger" onclick="deleteTemplate('${t.id}')">Delete</button></td></tr>`).join("")}
    </tbody></table></div>` : `<div class="panel"><div class="empty"><h3>No templates yet</h3><p>Make one from scratch, or open a phone that's set up the way you like and use <b>Save as template</b> on its Settings tab.</p></div></div>`}</div>`;
}

async function editTemplate(id) {
  const t = TPL.list.find((x) => x.id === id);
  if (!t) return;
  openTemplateEditor(t, await call("TemplateLines", id));
}

async function deleteTemplate(id) {
  const t = TPL.list.find((x) => x.id === id);
  if (!confirm(`Delete template “${t?.name}”? Phones it was applied to keep their settings.`)) return;
  try { await call("DeleteTemplate", id); renderTemplates(); } catch (e) { toast(e.message, true); }
}

// ---------- editor ----------
function paramPicker(id) {
  const groups = S.params || [];
  return `<select id="${id}" style="max-width:300px"><option value="">Insert a setting…</option>${groups.map((g) => `<optgroup label="${esc(g.group)}">${g.params.map((p) => `<option value="${p.key}">${esc(p.label)} (${p.key})</option>`).join("")}</optgroup>`).join("")}</select>`;
}

function insertParam(sel, ta) {
  const key = sel.value;
  if (!key) return;
  const p = (S.params || []).flatMap((g) => g.params).find((x) => x.key === key);
  const hint = p?.options ? Object.entries(p.options).map(([v, l]) => `${v} = ${l}`).join(", ") : p?.help || "";
  const line = `# ${p ? (p.group.startsWith("Account") ? p.group + " " + p.label.toLowerCase() : p.label) : key}${hint ? " (" + hint + ")" : ""}\n${key} = `;
  const cur = ta.value;
  ta.value = cur + (cur && !cur.endsWith("\n") ? "\n" : "") + line;
  ta.focus();
  ta.setSelectionRange(ta.value.length, ta.value.length);
  sel.value = "";
}

function openTemplateEditor(t, lines) {
  t = t || {};
  const m = modal(`<h2>${t.id ? "Edit template" : "New template"}</h2><div class="body">
    <div class="row"><label class="f"><span>Name</span><input type="text" id="tn" value="${esc(t.name || "")}" placeholder="Office phones"></label>
      <label class="f"><span>Notes</span><input type="text" id="tnotes" value="${esc(t.notes || "")}"></label></div>
    <div class="row" style="align-items:center;margin-bottom:6px"><span class="muted" style="font-size:13px">One setting per line: <code>P47 = pbx.example.com</code>. Lines starting with # are comments.</span><span class="grow"></span><span style="flex:none">${paramPicker("tpick")}</span></div>
    <textarea id="tl" class="lines" rows="14" spellcheck="false" placeholder="# SIP server for account 1\nP47 = pbx.example.com\n# NTP server\nP30 = pool.ntp.org">${esc(lines || "")}</textarea>
    <div id="tmsg"></div></div>
    <div class="foot"><span class="grow"></span><button class="btn" data-close>Cancel</button><button class="btn primary" id="tsave">Save template</button></div>`, true);
  $("#tpick", m).onchange = () => insertParam($("#tpick", m), $("#tl", m));
  $("#tsave", m).onclick = async () => {
    try {
      await call("SaveTemplate", { id: t.id || "", name: $("#tn", m).value.trim(), notes: $("#tnotes", m).value.trim() }, $("#tl", m).value);
      closeModal();
      toast("Template saved");
      location.hash = "#/templates";
      if (S.route[0] === "templates") renderTemplates();
    } catch (e) { $("#tmsg", m).innerHTML = `<div class="msg err">${esc(e.message).replace(/\n/g, "<br>")}</div>`; }
  };
}

// ---------- phone picker shared by apply and export ----------
function phonePicker(ids, q = "") {
  const sel = new Set(ids);
  const phones = (S.dash?.phones || []).filter((v) => !v.disabled);
  const shown = phones.filter((v) => phoneMatches(v, q));
  const all = shown.length > 0 && shown.every((v) => sel.has(v.id));
  return `<div class="row" style="align-items:center;margin-bottom:8px"><span class="searchbox">${ICON.search}<input type="text" id="ppq" placeholder="Filter phones" value="${esc(q)}"></span>
      <span class="grow"></span><label class="check" style="flex:none;font-size:13px"><input type="checkbox" id="ppall" ${all ? "checked" : ""}> All shown</label><span class="faint" id="ppn" style="flex:none;font-size:13px">${sel.size} selected</span></div>
    <div class="pick" id="pplist">${shown.map((v) => `<label><input type="checkbox" value="${v.id}" ${sel.has(v.id) ? "checked" : ""}>${statusDot(v)}<span>${esc(nameOfView(v))}</span><span class="faint">${esc(v.summary?.model || "")}</span><span class="sub">${esc(v.host)}</span></label>`).join("") || `<div class="empty">No phones match.</div>`}</div>`;
}

function wirePicker(m, ids, onChange) {
  const sel = new Set(ids);
  let q = "";
  const redraw = () => {
    $("#pphold", m).innerHTML = phonePicker([...sel], q);
    wire();
  };
  const wire = () => {
    $("#ppq", m).oninput = (e) => { q = e.target.value; redraw(); $("#ppq", m).focus(); $("#ppq", m).setSelectionRange(q.length, q.length); };
    $("#ppall", m).onchange = (e) => {
      for (const v of (S.dash?.phones || []).filter((v) => !v.disabled && phoneMatches(v, q))) e.target.checked ? sel.add(v.id) : sel.delete(v.id);
      redraw();
      onChange([...sel]);
    };
    $("#pplist", m).onchange = (e) => {
      const cb = e.target;
      if (cb.type !== "checkbox") return;
      cb.checked ? sel.add(cb.value) : sel.delete(cb.value);
      $("#ppn", m).textContent = `${sel.size} selected`;
      onChange([...sel]);
    };
  };
  redraw();
  return () => [...sel];
}

// ---------- apply ----------
// openApplyDialog(ids, templateId): pick a template (or use the given one), pick phones, review, apply.
async function openApplyDialog(ids, templateId) {
  if (!TPL.list.length) TPL.list = await call("Templates");
  if (!TPL.list.length) { toast("Make a template first (Templates page)", true); return; }
  const tpl = TPL.list.find((t) => t.id === templateId) || TPL.list[0];
  const lines = await call("TemplateLines", tpl.id);
  openValuesDialog(ids, lines, tpl);
}

// openValuesDialog(ids, lines, tpl): the generic "write these values to these phones" flow.
function openValuesDialog(ids, lines, tpl) {
  const m = modal(`<h2>${tpl ? `Apply “${esc(tpl.name)}”` : "Write values"}</h2><div class="body">
    ${tpl && TPL.list.length > 1 ? `<label class="f"><span>Template</span><select id="vtpl">${TPL.list.map((t) => `<option value="${t.id}" ${t.id === tpl.id ? "selected" : ""}>${esc(t.name)}</option>`).join("")}</select></label>` : ""}
    <div class="row" style="align-items:center;margin-bottom:6px"><span class="muted" style="font-size:13px">Settings to write, one per line.</span><span class="grow"></span><span style="flex:none">${paramPicker("vpick")}</span></div>
    <textarea id="vl" class="lines" rows="${tpl ? 6 : 9}" spellcheck="false" placeholder="P30 = pool.ntp.org">${esc(lines || "")}</textarea>
    <div style="height:12px"></div>
    <div id="pphold"></div>
    <div style="height:12px"></div>
    <label class="check"><input type="checkbox" id="vreboot"> Reboot each phone afterwards <span class="faint">(needed for network and some provisioning changes)</span></label>
    <div id="vmsg"></div></div>
    <div class="foot"><span class="grow"></span><button class="btn" data-close>Cancel</button><button class="btn primary" id="vprev">Review changes</button></div>`, true);
  $("#vpick", m).onchange = () => insertParam($("#vpick", m), $("#vl", m));
  const get = wirePicker(m, ids, () => {});
  if ($("#vtpl", m)) $("#vtpl", m).onchange = async () => { $("#vl", m).value = await call("TemplateLines", $("#vtpl", m).value); };
  $("#vprev", m).onclick = async () => {
    const chosen = get();
    const text = $("#vl", m).value;
    if (!chosen.length) { $("#vmsg", m).innerHTML = `<div class="msg err">Tick at least one phone.</div>`; return; }
    try {
      const pv = await call("PreviewValues", chosen, text);
      closeModal();
      confirmApply(chosen, text, pv, $("#vreboot", m).checked);
    } catch (e) { $("#vmsg", m).innerHTML = `<div class="msg err">${esc(e.message).replace(/\n/g, "<br>")}</div>`; }
  };
}

function changeRows(changes) {
  return changes.map((c) => `<div class="chg${c.changed ? "" : " same"}"><span class="k" title="${esc(c.label)}">${c.key}</span><span class="from" title="${esc(c.label)}">${c.secret ? "" : esc(c.from === undefined ? "" : c.from) || `<span class="faint">(empty)</span>`}</span><span class="to">${c.changed ? esc(c.to) || `<span class="faint">(cleared)</span>` : `<span class="faint">unchanged</span>`}</span></div>`).join("");
}

function confirmApply(ids, text, previews, reboot) {
  const total = previews.reduce((n, p) => n + p.changed, 0);
  const m = modal(`<h2>Review changes</h2><div class="body">
    <p class="muted" style="margin-top:0">${total ? `${plural(total, "value")} will be written across ${phonesWord(previews.length)}.` : "Every phone already has these values; nothing will be written."}${reboot ? " Each phone is rebooted afterwards." : ""} Values a phone already has are skipped.</p>
    <div style="max-height:50vh;overflow:auto">${previews.map((p) => `<section class="panel" style="margin-bottom:10px"><h2 style="padding:9px 14px"><span>${esc(p.phoneName)}</span><span class="grow"></span><span class="muted" style="font-weight:400;font-size:13px">${p.changed ? plural(p.changed, "change") : "no changes"}${p.note ? " · " + esc(p.note) : ""}</span></h2>
      <div class="body" style="padding:6px 14px">${changeRows(p.changes)}</div></section>`).join("")}</div>
    <div id="ares"></div></div>
    <div class="foot"><span class="grow"></span><button class="btn" data-close>Cancel</button><button class="btn primary" id="ago" ${total ? "" : "disabled"}>Write to ${phonesWord(previews.filter((p) => p.changed).length)}</button></div>`, true);
  $("#ago", m).onclick = async () => {
    const b = $("#ago", m);
    b.disabled = true;
    b.textContent = "Writing…";
    try {
      const res = await call("ApplyValues", previews.filter((p) => p.changed).map((p) => p.phoneId), text, reboot);
      const okN = res.filter((r) => r.ok).length;
      $("#ares", m).innerHTML = `<div class="msg ${okN === res.length ? "okm" : "err"}" style="margin-top:12px">${okN} of ${phonesWord(res.length)} updated.</div>
        <div class="panel" style="margin-top:10px;max-height:40vh;overflow:auto">${res.map((r) => `<div class="result"><b style="font-weight:500">${esc(r.phoneName)}</b> <span class="${r.ok ? "ok" : "bad"}">${r.ok ? `${r.written} written` : "failed"}</span>${r.error ? `<div class="bad" style="font-size:13px">${esc(r.error)}</div>` : ""}<pre class="code mono" style="margin-top:6px;max-height:140px;font-size:12px">${esc((r.transcript || []).join("\n"))}</pre></div>`).join("")}</div>`;
      b.remove();
      $("[data-close]", m).textContent = "Close";
    } catch (e) {
      $("#ares", m).innerHTML = `<div class="msg err">${esc(e.message)}</div>`;
      b.disabled = false;
      b.textContent = "Try again";
    }
  };
}

// ---------- provisioning export ----------
async function openExportDialog(templateId) {
  const tpl = TPL.list.find((t) => t.id === templateId);
  const lines = await call("TemplateLines", templateId);
  const m = modal(`<h2>Provisioning files for “${esc(tpl.name)}”</h2><div class="body">
    <p class="muted" style="margin-top:0;font-size:13px">Writes one <code>cfg&lt;mac&gt;.xml</code> per phone, in Grandstream's XML format, for a provisioning server of your own. The phone's MAC comes from its last reading.</p>
    <div id="pphold"></div><div id="xmsg"></div></div>
    <div class="foot"><span class="grow"></span><button class="btn" data-close>Cancel</button><button class="btn primary" id="xgo">Save files…</button></div>`, true);
  const get = wirePicker(m, [], () => {});
  $("#xgo", m).onclick = async () => {
    const chosen = get();
    if (!chosen.length) { $("#xmsg", m).innerHTML = `<div class="msg err">Tick at least one phone.</div>`; return; }
    try {
      const path = await call("ExportProvisioning", chosen, lines);
      closeModal();
      toast("Saved to " + path);
    } catch (e) {
      if (e.message === "cancelled") return;
      if (e.message.startsWith("saved")) { closeModal(); toast(e.message, true); return; }
      $("#xmsg", m).innerHTML = `<div class="msg err">${esc(e.message)}</div>`;
    }
  };
}
