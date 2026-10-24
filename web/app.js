(() => {
  const REPO = "jaba0x/PhoneDesk";
  const reduce = matchMedia("(prefers-reduced-motion: reduce)").matches;

  // ---------- nav border once scrolled ----------
  const nav = document.querySelector(".nav");
  const onScroll = () => nav.classList.toggle("scrolled", scrollY > 8);
  addEventListener("scroll", onScroll, { passive: true });
  onScroll();

  // ---------- line-key field behind the hero ----------
  // Columns of line keys, like BLF sidecars next to a desk phone: each key
  // has an LED and a label. Green is registered, red isn't, grey is a key
  // with nothing on it, and blue blinks while a line is in a call.
  const cv = document.querySelector(".ports");
  if (cv) {
    const ctx = cv.getContext("2d");
    let keys = [], w = 0, h = 0, dpr = 1;
    const LED = { off: [42, 49, 61], up: [76, 195, 138], down: [240, 97, 109], call: [110, 168, 254] };
    const KW = 74, KH = 16, GX = 22, GY = 8, ROWS = 6, MODGAP = 18;
    const layout = () => {
      dpr = Math.min(devicePixelRatio || 1, 2);
      w = cv.clientWidth; h = cv.clientHeight;
      cv.width = w * dpr; cv.height = h * dpr;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      keys = [];
      for (let x = 14, col = 0; x < w; x += KW + GX, col++) {
        // every other column starts half a key lower, so the grid doesn't read as a table
        for (let y = 16 + (col % 2) * 12, row = 0; y < h; y += KH + GY, row++) {
          if (row && row % ROWS === 0) y += MODGAP;
          const r = Math.random();
          const state = r < 0.3 ? "up" : r < 0.33 ? "down" : "off";
          keys.push({ x, y, state, label: 20 + Math.random() * 26, phase: Math.random() * Math.PI * 2, speed: 0.5 + Math.random() * 1.4, until: 0 });
        }
      }
    };
    const rr = (x, y, bw, bh, r) => { ctx.beginPath(); ctx.roundRect ? ctx.roundRect(x, y, bw, bh, r) : ctx.rect(x, y, bw, bh); ctx.fill(); };
    const draw = (t) => {
      ctx.clearRect(0, 0, w, h);
      for (const k of keys) {
        // key cap and label
        ctx.fillStyle = "rgba(42,49,61,.34)";
        rr(k.x, k.y, KW, KH, 4);
        ctx.fillStyle = k.state === "off" ? "rgba(123,132,148,.14)" : "rgba(123,132,148,.3)";
        rr(k.x + 20, k.y + 6.5, k.label, 3, 1.5);
        // LED
        const [r, g, b] = LED[k.state];
        let a = 0.5;
        if (k.state === "up") a = 0.3 + 0.5 * (0.5 + 0.5 * Math.sin(k.phase + t * 0.001 * k.speed));
        if (k.state === "down") a = 0.55 + 0.35 * (0.5 + 0.5 * Math.sin(k.phase + t * 0.0025));
        if (k.state === "call") a = 0.3 + 0.7 * (Math.sin(t * 0.006) > 0 ? 1 : 0.2);
        ctx.fillStyle = `rgba(${r},${g},${b},${a})`;
        rr(k.x + 6, k.y + 4, 8, 8, 2);
      }
    };
    // phones register and drop, calls start and end
    const flip = (t) => {
      for (const k of keys) if (k.state === "call" && t > k.until) k.state = "up";
      for (let i = 0; i < 2; i++) {
        const k = keys[(Math.random() * keys.length) | 0];
        if (!k || k.state === "call") continue;
        const r = Math.random();
        if (k.state === "up" && r < 0.12) { k.state = "call"; k.until = t + 4000 + Math.random() * 6000; }
        else if (k.state === "down" && r < 0.5) k.state = "up";
        else if (k.state !== "down") k.state = k.state === "up" ? "off" : "up";
      }
    };
    layout();
    let visible = true;
    new IntersectionObserver(([e]) => { visible = e.isIntersecting; }).observe(cv);
    if (reduce) {
      draw(0);
    } else {
      let lastFlip = 0;
      const loop = (t) => {
        if (visible) {
          if (t - lastFlip > 1100) { flip(t); lastFlip = t; }
          draw(t);
        }
        requestAnimationFrame(loop);
      };
      requestAnimationFrame(loop);
    }
    let rt;
    addEventListener("resize", () => { clearTimeout(rt); rt = setTimeout(() => { layout(); draw(performance.now()); }, 150); });
  }

  // ---------- reveal on scroll ----------
  const els = document.querySelectorAll(".feature > *, .divider");
  if (!reduce && "IntersectionObserver" in window) {
    const io = new IntersectionObserver((entries) => {
      for (const e of entries) if (e.isIntersecting) { e.target.classList.add("in"); io.unobserve(e.target); }
    }, { rootMargin: "0px 0px -8% 0px" });
    els.forEach((el) => { el.classList.add("reveal"); io.observe(el); });
  }

  // ---------- highlight the visitor's platform ----------
  const ua = navigator.userAgent;
  const mine = /Windows/i.test(ua) ? "win" : /Mac OS X|Macintosh/i.test(ua) ? "mac" : "";
  if (mine) {
    document.querySelector(`.plat[data-plat="${mine}"]`)?.classList.add("mine");
  }

  // ---------- latest release: version, sizes, direct links ----------
  const mb = (n) => (n / 1048576).toFixed(1) + " MB";
  fetch(`https://api.github.com/repos/${REPO}/releases/latest`, { headers: { Accept: "application/vnd.github+json" } })
    .then((r) => (r.ok ? r.json() : Promise.reject(r.status)))
    .then((rel) => {
      const ver = (rel.tag_name || "").replace(/^v/, "");
      const date = rel.published_at ? new Date(rel.published_at).toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" }) : "";
      document.querySelectorAll("[data-ver]").forEach((el) => { el.textContent = ver ? `${ver}${date ? " · " + date : ""}` : "Latest"; });
      const pick = { mac: /macos/i, win: /windows/i };
      for (const [k, re] of Object.entries(pick)) {
        const a = (rel.assets || []).find((x) => re.test(x.name));
        if (!a) continue;
        document.querySelectorAll(`[data-dl="${k}"]`).forEach((el) => { el.href = a.browser_download_url; });
        document.querySelectorAll(`[data-size="${k}"]`).forEach((el) => { el.textContent = mb(a.size); });
      }
    })
    .catch(() => {});

  fetch(`https://api.github.com/repos/${REPO}`)
    .then((r) => (r.ok ? r.json() : Promise.reject()))
    .then((repo) => {
      const n = repo.stargazers_count;
      const el = document.querySelector("[data-stars]");
      if (el && typeof n === "number" && n > 0) el.textContent = "★ " + (n >= 1000 ? (n / 1000).toFixed(1) + "k" : String(n));
    })
    .catch(() => {});
})();
