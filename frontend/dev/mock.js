// Fake backend for previewing the UI in a plain browser (not shipped in the app).
(function () {
  const mins = (m) => new Date(Date.now() - m * 60000).toISOString();
  const acct = (index, userId, display, server, registered, active = true) => ({ index, active, name: userId, display, userId, authId: userId, server, proxy: "", registered });
  const ACC = [
    ["P271", "P270", "P47", "P48", "P35", "P36", "P34", "P3"], ["P401", "P417", "P402", "P403", "P404", "P405", "P406", "P407"], ["P501", "P517", "P502", "P503", "P504", "P505", "P506", "P507"],
    ["P601", "P617", "P602", "P603", "P604", "P605", "P606", "P607"], ["P1701", "P1717", "P1702", "P1703", "P1704", "P1705", "P1706", "P1707"], ["P1801", "P1817", "P1802", "P1803", "P1804", "P1805", "P1806", "P1807"],
  ];
  const phones = [
    { id: "p1", name: "", host: "10.0.5.21", model: "GRP2613", family: "grp2610", fw: "1.0.13.31", mac: "c0:74:ad:11:22:33", ip: "10.0.5.21", site: "HQ", accounts: [acct(1, "201", "Nino K.", "pbx.lan", 1), acct(2, "251", "Nino K. (mobile)", "pbx.lan", 1)], lines: [{ line: 1, state: "idle", account: 0 }, { line: 2, state: "idle", account: 0 }, { line: 3, state: "idle", account: 1 }], regKnown: true, ntp: "pool.ntp.org", syslog: "", https: true, def: false, uptime: "12 days 4:21" },
    { id: "p2", name: "Reception", host: "10.0.5.22", model: "GXP2170", family: "gxp2130", fw: "1.0.11.64", mac: "00:0b:82:aa:bb:cc", ip: "10.0.5.22", site: "HQ", accounts: [acct(1, "100", "Reception", "pbx.lan", 1), acct(2, "101", "Reception 2", "pbx.lan", 0), acct(3, "900", "Paging", "pbx.lan", 1)], lines: [{ line: 1, state: "incall", account: 0, remoteNumber: "+995 32 2 123456", remoteName: "Tbilisi Energy" }, { line: 2, state: "idle", account: 0 }, { line: 3, state: "failed", account: 1 }, { line: 4, state: "idle", account: 2 }], regKnown: true, ntp: "pool.ntp.org", syslog: "10.0.0.5", https: true, def: false, uptime: "41 days 2:05", avail: "1.0.11.74" },
    { id: "p3", name: "", host: "10.0.5.23", model: "GRP2602W", family: "grp2600", fw: "1.0.5.99", mac: "c0:74:ad:00:00:01", ip: "10.0.5.23", site: "HQ", accounts: [acct(1, "202", "Giorgi M.", "pbx.lan", 1)], lines: [{ line: 1, state: "idle", account: 0 }, { line: 2, state: "idle", account: 0 }], regKnown: true, ntp: "", syslog: "", https: false, def: true, uptime: "3 days 0:12" },
    { id: "p4", name: "Warehouse", host: "10.0.6.40", model: "GXP1628", family: "gxp1600", fw: "1.0.7.81", mac: "00:0b:82:01:02:03", ip: "10.0.6.40", site: "Warehouse", accounts: [acct(1, "310", "Warehouse", "pbx.lan", -1)], lines: [], regKnown: true, ntp: "pool.ntp.org", syslog: "", https: true, def: false, offline: true, uptime: "" },
    { id: "p5", name: "", host: "10.0.5.60", model: "WP822", family: "wp822", fw: "1.0.7.47", mac: "c0:74:ad:55:66:77", ip: "10.0.5.60", site: "HQ", accounts: [acct(1, "205", "Lika B.", "pbx.lan", -1), acct(2, "", "", "", -1, false)], lines: [], regKnown: false, ntp: "pool.ntp.org", syslog: "", https: true, def: false, uptime: "7 days 11:40" },
    { id: "p6", name: "Old meeting room", host: "10.0.5.99", model: "GXP1625", family: "gxp1600", fw: "1.0.4.152", mac: "00:0b:82:99:99:99", ip: "10.0.5.99", site: "HQ", accounts: [acct(1, "399", "Meeting", "pbx.lan", -1)], lines: [], regKnown: false, ntp: "", syslog: "", https: true, def: false, disabled: true, uptime: "" },
  ];
  const values = (p) => {
    const v = { Pphone_model: p.model, prog: p.fw, mac: p.mac, ip: p.ip, boot: "1.0.0.4", core: "1.0.0.9", base: "1.0.13.31", dsp: "1.0.0.5", uptime: p.uptime, vendor_fullname: "Grandstream Networks, Inc.", P30: p.ntp, P64: "TZR-4", P207: p.syslog, P208: "2", P8: "0", P51: "0", P212: "1", P192: "fm.grandstream.com/gs", P240: "2", P237: "prov.lan/gs", P194: "1", P193: "10080", P238: "1", P145: "1", P1414: "0", P1684: "1", P290: "{ x+ | \\+x+ | *x+ | *xx*x+ }" };
    p.accounts.forEach((a) => { const k = ACC[a.index - 1]; v[k[0]] = a.active ? "1" : "0"; v[k[1]] = a.name; v[k[2]] = a.server; v[k[3]] = a.proxy; v[k[4]] = a.userId; v[k[5]] = a.authId; v[k[7]] = a.display; });
    return v;
  };
  const issuesFor = (p) => {
    const name = p.name || `${p.accounts[0].display} (${p.accounts[0].userId})`;
    const out = [];
    const add = (severity, category, title, detail) => out.push({ phoneId: p.id, phoneName: name, severity, category, title, detail });
    if (p.offline) add("critical", "Reachability", "Phone is offline", "No answer on the web port since Oct 23 09:41 (timeout)");
    p.accounts.filter((a) => a.active && a.registered === 0).forEach((a) => add("critical", "Accounts", `Account ${a.index} (${a.userId}) is not registered`, `The phone can't register with ${a.server}. Check the credentials on the PBX and the phone.`));
    if (p.def) add("critical", "Security", "Default admin password", "The phone still accepts admin/admin. Anyone on the network can reconfigure it.");
    if (!p.https) add("warning", "Security", "Web access over plain HTTP", "PhoneDesk reached the web UI over plain HTTP; the admin password travels in clear text. Turn on HTTPS web access on the phone.");
    if (!p.ntp) add("warning", "Settings", "No NTP server", "Without time sync the call history and log timestamps drift.");
    if (!p.syslog) add("info", "Settings", "No syslog server", "Events only live in the phone's own buffer until it reboots.");
    if (p.avail) add("info", "Firmware", `Firmware ${p.avail} is in your library`, `Running ${p.fw}. Update from the Firmware page.`);
    const order = { critical: 0, warning: 1, info: 2 };
    out.sort((a, b) => order[a.severity] - order[b.severity]);
    return out;
  };
  const view = (p) => {
    const issues = p.disabled ? [] : issuesFor(p);
    const sum = { model: p.model, family: p.family, firmware: p.fw, mac: p.mac, ip: p.ip, uptime: p.uptime, extension: p.accounts[0].userId, display: p.accounts[0].display, server: p.accounts[0].server,
      accounts: p.accounts.map((a) => ({ index: a.index, active: a.active, registered: p.regKnown ? a.registered : -1, userId: a.userId, name: a.name, server: a.server })),
      activeAccounts: p.accounts.filter((a) => a.active).length, registered: p.accounts.filter((a) => a.registered === 1).length, registrationKnown: p.regKnown, lines: p.lines, inCall: p.lines.some((l) => l.state === "incall"), collectedAt: mins(p.offline ? 300 : 4), scheme: p.https ? "https" : "http" };
    return { id: p.id, name: p.name, host: p.host, port: 0, scheme: "auto", credentialId: "c1", site: p.site, notes: "", mac: p.mac, family: "", disabled: !!p.disabled, createdAt: mins(5000),
      status: { online: !p.offline, checked: true, latencyMs: p.offline ? 0 : 2 + (p.host.length % 4), since: mins(p.offline ? 47 : 3000), collecting: false, lastCollect: mins(4), collectError: "", authFailed: false, scheme: sum.scheme },
      summary: sum, issues, worst: issues[0]?.severity || "", firmware: { supported: true, family: p.family, running: p.fw, available: p.avail || "", image: p.avail ? `${p.family}fw-${p.avail}.bin` : "", job: p.id === "p2" ? { id: "j1", phoneId: "p2", phoneName: "Reception", from: "1.0.11.64", to: "1.0.11.74", image: "gxp2130fw-1.0.11.74.bin", family: "gxp2130", source: "10.0.5.2:6970/k3x9z", state: "running", step: "Waiting for the phone to download the image", progress: 0.45, log: ["09:40:02  Updating Reception from 1.0.11.64 to 1.0.11.74", "09:40:03  Firmware server set to 10.0.5.2:6970/k3x9z (HTTP), always check for new firmware", "09:40:04  Rebooting; the phone fetches gxp2130fw.bin on the way up"], error: "", startedAt: mins(1), endedAt: "0001-01-01T00:00:00Z" } : null } };
  };
  const views = phones.map(view);
  const dash = () => {
    const live = views.filter((v) => !v.disabled);
    const issues = views.flatMap((v) => v.issues);
    issues.push({ phoneId: "", phoneName: "Fleet", severity: "warning", category: "Accounts", title: "Extension 201 is on 2 phones", detail: "Nino K. (201), Old meeting room all register 201 with pbx.lan. Unless the PBX allows multiple registrations, they'll knock each other off." });
    const order = { critical: 0, warning: 1, info: 2 };
    issues.sort((a, b) => order[a.severity] - order[b.severity]);
    return { total: views.length, online: live.filter((v) => v.status.online).length, offline: live.filter((v) => !v.status.online).length, unknown: 0, disabled: views.filter((v) => v.disabled).length,
      critical: issues.filter((i) => i.severity === "critical").length, warning: issues.filter((i) => i.severity === "warning").length, info: issues.filter((i) => i.severity === "info").length, collecting: 0,
      registered: live.reduce((n, v) => n + v.summary.registered, 0), accounts: live.reduce((n, v) => n + v.summary.activeAccounts, 0), issues, phones: views, sites: ["HQ", "Warehouse"] };
  };
  const creds = [{ id: "c1", name: "Office phones", username: "admin" }];
  const templates = [
    { id: "t1", name: "Office defaults", notes: "NTP, time zone, syslog and provisioning for every phone", values: { P30: "pool.ntp.org", P64: "TZR-4", P207: "10.0.0.5", P208: "2", P237: "prov.lan/gs", P240: "2", P145: "0" }, createdAt: mins(9000), updatedAt: mins(300) },
    { id: "t2", name: "Harden", notes: "New admin password, HTTPS only", values: { P2: "S3cure!Phone", P196: "user-pass-1" }, createdAt: mins(5000), updatedAt: mins(5000) },
  ];
  const images = [
    { file: "grp2610fw-1.0.13.31.bin", family: "grp2610", version: { nums: [1, 0, 13, 31], text: "1.0.13.31" }, size: 62 << 20, sha256: "", addedAt: mins(1500), source: "zip" },
    { file: "gxp2130fw-1.0.11.74.bin", family: "gxp2130", version: { nums: [1, 0, 11, 74], text: "1.0.11.74" }, size: 71 << 20, sha256: "", addedAt: mins(60), source: "firmware.grandstream.com" },
  ];
  const labels = { P2: "Admin password", P196: "User password", P30: "NTP server", P64: "Time zone", P207: "Syslog server", P208: "Syslog level", P237: "Config server path", P240: "Config upgrade via", P145: "Allow DHCP option 43/66 to override server" };
  const lines = (vals) => Object.keys(vals).sort((a, b) => +a.slice(1) - +b.slice(1)).map((k) => `${labels[k] ? "# " + labels[k] + "\n" : ""}${k} = ${vals[k]}`).join("\n") + "\n";
  const parse = (text) => { const out = {}; for (const l of text.split("\n")) { const t = l.trim(); if (!t || t.startsWith("#")) continue; const i = t.search(/[=:]/); if (i > 0) out[t.slice(0, i).trim().toUpperCase()] = t.slice(i + 1).trim(); } return out; };
  const grouped = () => {
    const yesNo = { 0: "No", 1: "Yes" };
    const out = [];
    for (let i = 0; i < 6; i++) out.push({ group: `Account ${i + 1}`, params: [["Active", 0, "bool", yesNo], ["Account name", 1], ["SIP server", 2], ["Outbound proxy", 3], ["SIP user ID", 4], ["Authenticate ID", 5], ["Authenticate password", 6, "password", null, true], ["Display name", 7]].map(([label, j, kind, options, secret]) => ({ key: ACC[i][j], label, group: `Account ${i + 1}`, kind: kind || "text", options, secret })) });
    out.push({ group: "Web access", params: [{ key: "P2", label: "Admin password", group: "Web access", kind: "password", secret: true }, { key: "P196", label: "User password", group: "Web access", kind: "password", secret: true }] });
    out.push({ group: "Network", params: [{ key: "P8", label: "Address type", group: "Network", kind: "enum", options: { 0: "DHCP", 1: "Static", 2: "PPPoE" } }, { key: "P51", label: "802.1Q VLAN tag", group: "Network", kind: "number" }, { key: "P1684", label: "LLDP", group: "Network", kind: "bool", options: yesNo }] });
    out.push({ group: "Time", params: [{ key: "P30", label: "NTP server", group: "Time" }, { key: "P64", label: "Time zone", group: "Time" }, { key: "P6767", label: "Self-defined time zone", group: "Time" }] });
    out.push({ group: "Firmware upgrade", params: [{ key: "P212", label: "Upgrade via", group: "Firmware upgrade", kind: "enum", options: { 0: "TFTP", 1: "HTTP", 2: "HTTPS" } }, { key: "P192", label: "Firmware server path", group: "Firmware upgrade" }, { key: "P194", label: "Automatic upgrade", group: "Firmware upgrade", kind: "bool", options: yesNo }, { key: "P193", label: "Check every (minutes)", group: "Firmware upgrade" }, { key: "P238", label: "Check for new firmware", group: "Firmware upgrade", kind: "enum", options: { 0: "Always", 1: "Only when the prefix or postfix changes", 2: "Never" } }] });
    out.push({ group: "Provisioning", params: [{ key: "P240", label: "Config upgrade via", group: "Provisioning", kind: "enum", options: { 0: "TFTP", 1: "HTTP", 2: "HTTPS" } }, { key: "P237", label: "Config server path", group: "Provisioning" }, { key: "P145", label: "Allow DHCP option 43/66 to override server", group: "Provisioning", kind: "bool", options: yesNo }, { key: "P1414", label: "3CX auto provision (PnP)", group: "Provisioning", kind: "bool", options: yesNo }] });
    out.push({ group: "Logging", params: [{ key: "P207", label: "Syslog server", group: "Logging" }, { key: "P208", label: "Syslog level", group: "Logging", kind: "enum", options: { 0: "None", 1: "Debug", 2: "Info", 3: "Warning", 4: "Error" } }] });
    out.push({ group: "Calls", params: [{ key: "P290", label: "Dial plan", group: "Calls" }] });
    return out;
  };
  const detail = (id) => {
    const p = phones.find((x) => x.id === id), v = views.find((x) => x.id === id);
    const vals = values(p);
    const snapshot = { collectedAt: v.summary.collectedAt, duration: "2.1s", scheme: v.summary.scheme, facts: { vendor: "Grandstream Networks, Inc.", model: p.model, family: p.family, firmware: p.fw, boot: "1.0.0.4", core: "1.0.0.9", base: "1.0.13.31", dsp: "1.0.0.5", mac: p.mac, ip: p.ip, hardware: "V1.4A", pn: "9620004514A", uptime: p.uptime },
      accounts: p.accounts.map((a) => ({ ...a, registered: p.regKnown ? a.registered : -1 })), lines: p.lines, network: { addressType: "DHCP", vlan: "0", lldp: true }, time: { ntpServer: p.ntp, zone: "TZR-4", selfZone: "" },
      upgrade: { firmwareVia: "HTTP", firmwarePath: "fm.grandstream.com/gs", configVia: "HTTPS", configPath: "prov.lan/gs", auto: true, checkInterval: "10080", checkMode: "Only when the prefix or postfix changes", option66: true },
      logging: { server: p.syslog, level: "Info" }, security: { httpPort: p.https ? "closed" : "open", httpsPort: "open", sshPort: "closed", telnetPort: "closed", defaultAdmin: p.def, plainWeb: !p.https },
      values: vals, raw: { "dologin (https)": '{"response":"success","body":{"sid":"…","role":"admin"}}', "api.values.get P271…P1807": JSON.stringify({ response: "success", body: vals }), "api-get_line_status": JSON.stringify({ response: "success", body: p.lines.map((l) => ({ line: l.line, state: l.state, acct: l.account, active: 0, remotename: l.remoteName || "", remotenumber: l.remoteNumber || "" })) }) },
      errors: p.regKnown ? [] : ["Line status not available: phone answered: unauthorized. Turn on Action URI support under Settings → Remote control on the phone to see registration state."], linesKnown: p.regKnown, registrationKnown: p.regKnown };
    return { ...v, snapshot, backups: [{ name: "20261023-091500.cfg", at: mins(1500), size: 2800 }, { name: "20261012-180300.cfg", at: mins(17000), size: 2790 }] };
  };
  const api = {
    Platform: async () => "browser", Version: async () => "0.1.0", StartupNotice: async () => "", LastUpdateCheck: async () => null,
    CheckForUpdate: async () => ({ current: "0.1.0", latest: "0.2.0", url: "https://github.com/jaba0x/PhoneDesk/releases", available: true }),
    GetSettings: async () => ({ pingInterval: 30, collectInterval: 15, parallel: 4, keepBackups: 20, staleHours: 6, firmwareSource: "builtin", firmwareServer: "", firmwareVia: "http", firmwarePort: 6970, relayHost: "10.0.18.100", relayPort: 22, relayUser: "root", relayHasPassword: true, relayKeyFile: "", relayHostKey: "SHA256:Qm9ndXNGaW5nZXJwcmludA", relayDir: "/tftpboot", relayServerPath: "10.0.18.100", relayVia: "tftp", checksOff: ["option66"] }),
    TestRelay: async (s) => { await new Promise((r) => setTimeout(r, 600)); return `Connected to ${s.relayHost} (pbx, 41G free in ${s.relayDir}). Host key Qm9ndXNGaW5n…`; },
    PickRelayKeyFile: async () => "/Users/jaba/.ssh/id_ed25519", ForgetRelayHostKey: async () => {},
    Checks: async () => [
      { id: "offline", label: "Phone is offline", detail: "The web port doesn't answer.", severity: "critical", group: "Reachability" },
      { id: "login-failed", label: "Web login failed", detail: "The phone refused the credentials.", severity: "critical", group: "Reachability" },
      { id: "read-failed", label: "Reading the phone failed", detail: "The phone is up but the read didn't finish.", severity: "warning", group: "Reachability" },
      { id: "unregistered", label: "Account not registered", detail: "", severity: "critical", group: "Accounts" },
      { id: "no-server", label: "Account has a user ID but no SIP server", detail: "", severity: "warning", group: "Accounts" },
      { id: "no-account", label: "No usable SIP account", detail: "", severity: "warning", group: "Accounts" },
      { id: "duplicate-ext", label: "Same extension on two phones", detail: "", severity: "warning", group: "Accounts" },
      { id: "default-password", label: "Factory admin password still accepted", detail: "", severity: "critical", group: "Security" },
      { id: "plain-http", label: "Web access over plain HTTP", detail: "", severity: "warning", group: "Security" },
      { id: "telnet", label: "Telnet open", detail: "", severity: "critical", group: "Security" },
      { id: "ssh", label: "SSH open", detail: "", severity: "info", group: "Security" },
      { id: "no-ntp", label: "No NTP server", detail: "", severity: "warning", group: "Settings" },
      { id: "no-syslog", label: "No syslog server", detail: "", severity: "info", group: "Settings" },
      { id: "option66", label: "DHCP option 66/43 can override the servers", detail: "", severity: "info", group: "Settings", defaultOff: true },
      { id: "firmware", label: "Newer firmware in the library", detail: "", severity: "info", group: "Firmware" },
      { id: "stale", label: "Data older than the limit", detail: "", severity: "info", group: "Freshness" },
    ],
    SaveSettings: async () => {}, KnownParams: async () => grouped(), GetDashboard: async () => dash(), GetPhone: async (id) => detail(id), Extensions: async () => views.flatMap((v) => v.summary.accounts.filter((a) => a.userId).map((a) => ({ phoneId: v.id, phoneName: v.name || v.summary.display, account: a.index, userId: a.userId, display: a.name, server: a.server, registered: a.registered, active: a.active }))).sort((a, b) => a.userId.localeCompare(b.userId)),
    Credentials: async () => creds, SaveCredential: async (c) => ({ ...c, id: c.id || "c" + Date.now() }), DeleteCredential: async () => {},
    SavePhone: async (p) => ({ ...p, id: p.id || "pnew" }), DeletePhone: async () => {}, DeletePhones: async () => {}, SetDisabled: async () => {}, TestPhone: async () => "Logged in over HTTPS to GRP2613, firmware 1.0.13.31, c0:74:ad:11:22:33", Refresh: async () => {}, RefreshAll: async () => {}, BulkAdd: async () => ({ added: 2, errors: [] }), AddFound: async (h) => ({ added: h.length, errors: [] }),
    ScanNetwork: async () => { await new Promise((r) => setTimeout(r, 600)); return [{ host: "10.0.5.21", scheme: "https", known: true, name: "Nino K. (201)" }, { host: "10.0.5.24", scheme: "https", known: false }, { host: "10.0.5.25", scheme: "http", known: false }]; }, CancelScan: async () => {},
    Reboot: async () => {}, Dial: async () => {}, GetValues: async (id, keys) => { const v = values(phones.find((p) => p.id === id)); const out = {}; keys.split(/[\s,:]+/).forEach((k) => { if (v[k] !== undefined) out[k] = v[k]; }); return out; },
    PreviewValues: async (ids, text) => { const vals = parse(text); return ids.map((id) => { const p = phones.find((x) => x.id === id), cur = values(p); const changes = Object.keys(vals).map((k) => ({ key: k, label: labels[k] || k, from: cur[k] ?? "", to: k === "P2" || k === "P196" ? "••••••" : vals[k], secret: k === "P2" || k === "P196", changed: k === "P2" || cur[k] !== vals[k] })); return { phoneId: id, phoneName: views.find((v) => v.id === id).name || views.find((v) => v.id === id).summary.display, changes, changed: changes.filter((c) => c.changed).length, note: "" }; }); },
    ApplyValues: async (ids) => ids.map((id) => ({ phoneId: id, phoneName: views.find((v) => v.id === id).name || views.find((v) => v.id === id).summary.display, ok: id !== "p4", written: 2, error: id === "p4" ? "the phone doesn't answer on its web port" : "", transcript: ["Logged in over HTTPS.", "P30 = pool.ntp.org  (NTP server)", "Written: 2 value(s).", "Read back the phone's settings."] })),
    PhoneValuesText: async (id) => lines(Object.fromEntries(Object.entries(values(phones.find((p) => p.id === id))).filter(([k]) => /^P\d+$/.test(k)))),
    GetBackup: async (id, name) => lines(values(phones.find((p) => p.id === id))).replace("pool.ntp.org", name.startsWith("202610") ? "time.google.com" : "pool.ntp.org"),
    Templates: async () => templates, TemplateLines: async (id) => lines(templates.find((t) => t.id === id).values), SaveTemplate: async (t, text) => { const vals = parse(text); const ex = templates.find((x) => x.id === t.id); if (ex) Object.assign(ex, t, { values: vals, updatedAt: new Date().toISOString() }); else templates.push({ ...t, id: "t" + Date.now(), values: vals, createdAt: new Date().toISOString(), updatedAt: new Date().toISOString() }); return t; }, DeleteTemplate: async () => {},
    ExportProvisioning: async () => "/Users/jaba/Desktop/phonedesk-provisioning.zip", ExportText: async (n) => "/Users/jaba/Desktop/" + n, OpenURL: async () => {}, OpenPhoneWeb: async () => {},
    FirmwareImages: async () => images, UpgradeJobs: async () => [views[1].firmware.job, { id: "j0", phoneId: "p1", phoneName: "Nino K. (201)", from: "1.0.13.20", to: "1.0.13.31", image: "grp2610fw-1.0.13.31.bin", family: "grp2610", source: "10.0.5.2:6970/a1b2c", state: "done", step: "Done", progress: 1, log: ["Yesterday 02:10  Running 1.0.13.31"], error: "", startedAt: mins(1500), endedAt: mins(1490) }],
    FirmwareServer: async () => ({ running: true, port: 6970, example: "10.0.5.2:6970" }), StartFirmwareServer: async () => {}, StopFirmwareServer: async () => {}, AddFirmwareFiles: async () => ({ added: [], needVersion: ["/Users/jaba/Downloads/grp2600fw.bin"], errors: [] }), AddFirmwareFileAs: async (p, f, v) => ({ file: `grp2600fw-${v}.bin`, family: f || "grp2600", version: { text: v } }), DownloadFirmware: async () => { await new Promise((r) => setTimeout(r, 800)); return images[0]; }, CancelDownload: async () => {}, DeleteFirmware: async () => {}, OpenFirmwareFolder: async () => {},
    StartUpgrade: async (ids) => ids.map((id) => ({ id: "j" + id })), CancelUpgrade: async () => {}, ClearFinishedUpgrades: async () => {},
    DataFolder: async () => ({ path: "/Users/jaba/Library/Application Support/PhoneDesk", defaultPath: "", custom: false }), PickDataFolder: async () => null, DefaultFolderInfo: async () => ({}), ChangeDataFolder: async () => {}, OpenDataFolder: async () => {}, ExportData: async () => "/Users/jaba/Desktop/PhoneDesk-2026-10-24.pdbak", PickImportFile: async () => "", InspectImport: async () => ({ phones: 6, createdAt: new Date().toISOString(), history: true }), ImportData: async () => ({ added: 6, skipped: [] }),
  };
  window.go = { main: { App: api } };
  window.runtime = { EventsOn: (name, fn) => { if (name === "scan") { window.__scan = fn; } return () => {}; } };
})();
