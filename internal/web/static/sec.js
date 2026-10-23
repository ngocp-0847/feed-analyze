/* feed-analyze — bản đồ mối đe doạ an ninh mạng. Một bộ lọc chung cho cả trang: bấm quốc gia / nhóm / ngành /
   kỹ thuật… ở bất kỳ biểu đồ nào → mọi biểu đồ + danh sách sự cố vẽ lại theo cùng lát dữ liệu (tổng quan → chi tiết).
   Dùng chung token màu, tooltip, bảng dữ liệu với viz.js (window.FAViz). */
(function () {
  "use strict";
  document.addEventListener("DOMContentLoaded", function () {
    var V = window.FAViz, src = document.getElementById(window.FA_SEC_SRC);
    if (!V || !src || !window.echarts) return;
    var D = JSON.parse(src.textContent), B = D.board, BR = D.brief || {}, L = B.labels;
    var esc = V.esc, trunc = V.trunc, merge = V.merge;

    // ---------- danh mục ----------
    function dict(list) { var m = {}; (list || []).forEach(function (x) { m[x.id] = x.vi; }); return m; }
    var FAM = L.families.map(function (x) { return x.id; }), FAMVI = dict(L.families), SECVI = dict(L.sectors),
      VECVI = dict(L.vectors), MOTVI = dict(L.motivations), KINDVI = dict(L.kinds), TACVI = dict(L.tactics), LAYVI = dict(L.layers);
    var TYPEVI = L.types || {}, CWEVI = L.cwe || {};
    function cname(c) { var x = B.countries[c]; return x ? x[0] : c; }
    function flag(c) {
      if (!c || c.length !== 2) return "";
      return String.fromCodePoint(0x1F1E6 + c.charCodeAt(0) - 65, 0x1F1E6 + c.charCodeAt(1) - 65);
    }
    function has(a, v) { return !!a && a.indexOf(v) >= 0; }
    function uniq(a) { return a.filter(function (x, i) { return x && a.indexOf(x) === i; }); }
    function num(n) { return (n || 0).toLocaleString("vi-VN"); }
    function bigParts(n) {
      var f = function (x) { return x.toLocaleString("vi-VN", { maximumFractionDigits: 1 }); };
      if (n >= 1e9) return [f(n / 1e9), "tỷ"];
      if (n >= 1e6) return [f(n / 1e6), "triệu"];
      if (n >= 1e3) return [f(n / 1e3), "nghìn"];
      return [num(n), ""];
    }
    function big(n) { var p = bigParts(n); return p[0] + (p[1] ? " " + p[1] : ""); }
    function $(id) { return document.getElementById(id); }
    function css(n) { return getComputedStyle(document.documentElement).getPropertyValue(n).trim(); }
    function tok() {
      var t = V.T();
      t.heat = [1, 2, 3, 4, 5].map(function (i) { return css("--heat-" + i); });
      t.land = css("--land"); t.flow = css("--flow");
      return t;
    }
    function famColor(t, f) { var i = FAM.indexOf(f); return i >= 0 ? t.s[i] : t.muted; }
    function tally(rs, fn) {
      var m = {};
      rs.forEach(function (i) { uniq(fn(i) || []).forEach(function (k) { m[k] = (m[k] || 0) + 1; }); });
      return Object.keys(m).map(function (k) { return [k, m[k]]; }).sort(function (a, b) { return b[1] - a[1] || (a[0] < b[0] ? -1 : 1); });
    }
    function places(i) { return uniq((i.vc || []).concat(i.tc || [])); }
    function hexRGB(h) { h = h.replace("#", ""); if (h.length === 3) h = h.replace(/./g, "$&$&"); var n = parseInt(h, 16); return [n >> 16 & 255, n >> 8 & 255, n & 255]; }
    function mix(a, b, k) {
      var x = hexRGB(a), y = hexRGB(b);
      return "rgb(" + x.map(function (v, i) { return Math.round(v + (y[i] - v) * k); }).join(",") + ")";
    }
    function lum(rgb) { var m = rgb.match(/\d+/g).map(Number); return (0.299 * m[0] + 0.587 * m[1] + 0.114 * m[2]) / 255; }

    // ---------- bộ lọc chung ----------
    var F = { scope: "threats", vn: false, q: "", country: null, origin: null, sector: null, fam: null, actor: null, tech: null, vec: null, cve: null, mot: null, stack: null };
    var FLAB = { country: "Nước nạn nhân", origin: "Nguồn tấn công", sector: "Ngành", fam: "Hình thức", actor: "Nhóm", tech: "Kỹ thuật", vec: "Cửa ngõ", cve: "CVE", mot: "Động cơ", stack: "Stack của bạn" };
    function fval(k, v) {
      if (k === "country" || k === "origin") return flag(v) + " " + cname(v);
      if (k === "sector") return SECVI[v] || v;
      if (k === "fam") return FAMVI[v] || "Khác";
      if (k === "vec") return VECVI[v] || v;
      if (k === "mot") return MOTVI[v] || v;
      if (k === "tech") return v + (B.tech[v] ? " " + B.tech[v].n : "");
      if (k === "stack") return ((B.stack || [])[+v] || {}).name || v;
      return v;
    }
    function match(i) {
      if (F.scope === "threats" && i.kind !== "incident" && i.kind !== "campaign") return false;
      if (F.vn && !(i.vn || has(i.vc, "VN") || has(i.tc, "VN"))) return false;
      if (F.country && !has(places(i), F.country)) return false;
      if (F.origin && i.origin !== F.origin) return false;
      if (F.sector && !has(i.sec, F.sector)) return false;
      if (F.fam && !(F.fam === "_" ? !(i.fam || []).length : has(i.fam, F.fam))) return false;
      if (F.actor && i.actor !== F.actor) return false;
      if (F.tech && !(i.tech || []).some(function (x) { return x === F.tech || x.indexOf(F.tech + ".") === 0; })) return false;
      if (F.vec && !has(i.vec, F.vec)) return false;
      if (F.cve && !has(i.cve, F.cve)) return false;
      if (F.mot && i.mot !== F.mot) return false;
      if (F.stack != null && !has(i.stk, +F.stack)) return false;
      if (F.q) {
        var h = [i.t, i.sum, i.actor, (i.alias || []).join(" "), (i.prod || []).join(" "), (i.cve || []).join(" "), (i.vic || []).join(" "), i.det, (i.tech || []).join(" ")].join(" ").toLowerCase();
        if (h.indexOf(F.q) < 0) return false;
      }
      return true;
    }
    function setF(k, v, keep) {
      F[k] = F[k] === v && !keep ? null : v;
      shown = 25;
      update();
    }
    // bộ lọc ↔ URL (?country=VN&actor=...): chia sẻ / mở lại đúng lát dữ liệu
    var URLKEYS = Object.keys(FLAB).concat(["q", "scope", "vn"]);
    (function () {
      try {
        var u = new URLSearchParams(location.search);
        URLKEYS.forEach(function (k) {
          var v = u.get(k);
          if (v == null || v === "") return;
          if (k === "vn") F.vn = v === "1"; else if (k === "q") F.q = v.toLowerCase(); else F[k] = k === "country" || k === "origin" ? v.toUpperCase() : v;
        });
        var bar = document.getElementById("sec-filterbar");
        if (bar) {
          var sc = bar.querySelector("input[name=scope][value=" + (F.scope === "all" ? "all" : "threats") + "]"); if (sc) sc.checked = true;
          var vn = bar.querySelector("input[name=vn]"); if (vn) vn.checked = F.vn;
          var q = bar.querySelector("input[name=q]"); if (q && F.q) q.value = F.q;
        }
      } catch (e) {}
    })();
    function syncURL() {
      try {
        var u = new URLSearchParams(location.search);
        URLKEYS.forEach(function (k) { u.delete(k); });
        Object.keys(FLAB).forEach(function (k) { if (F[k]) u.set(k, F[k]); });
        if (F.q) u.set("q", F.q);
        if (F.scope !== "threats") u.set("scope", F.scope);
        if (F.vn) u.set("vn", "1");
        var qs = u.toString();
        history.replaceState(null, "", location.pathname + (qs ? "?" + qs : "") + location.hash);
      } catch (e) {}
    }

    var mapMode = "victim", sortBy = "new", shown = 25, geoState = 0, actorsAll = false, attackAll = false; // 0 chưa tải, 1 đang tải, 2 sẵn sàng, -1 lỗi

    // ---------- biểu đồ ----------
    var inst = {};
    function chart(id) {
      var el = $(id);
      if (!el) return null;
      if (!inst[id]) {
        inst[id] = echarts.init(el, null, { renderer: "canvas" });
        inst[id].__handlers = {};
      }
      return inst[id];
    }
    function on(c, ev, fn) { c.off(ev); c.on(ev, fn); }
    function table(id, cols, rows) { var el = $(id); if (el) el.innerHTML = V.tableHTML({ cols: cols, rows: rows }); }
    function tip(t, extra) { return merge(V.base(t).tooltip, extra || {}); }

    function kpis(t, rs) {
      var el = $("sec-kpis");
      if (!el) return;
      var ransom = rs.filter(function (i) { return has(i.fam, "ransomware"); }).length;
      var recs = 0, withRec = 0;
      rs.forEach(function (i) { if (i.rec) { recs += i.rec; withRec++; } });
      var actors = uniq(rs.map(function (i) { return i.actor; })).length;
      var ctry = uniq([].concat.apply([], rs.map(places))).length;
      var vn = rs.filter(function (i) { return i.vn || has(i.vc, "VN") || has(i.tc, "VN"); }).length;
      var sev = rs.filter(function (i) { return i.sev >= 4; }).length;
      var kevRw = B.kev.filter(function (k) { return k.rw; }).length;
      function k(lab, val, unit, delta, note) {
        return "<div class='kpi'><div class='lab'>" + esc(lab) + "</div><div class='val'>" + esc(val) + (unit ? "<span class='unit'>" + esc(unit) + "</span>" : "") + "</div>" +
          (delta ? "<div class='delta'>" + esc(delta) + "</div>" : "") + (note ? "<div class='note'>" + esc(note) + "</div>" : "") + "</div>";
      }
      el.innerHTML =
        k(F.scope === "threats" ? "Sự cố & chiến dịch" : "Sự kiện an ninh", num(rs.length), "", sev + " nghiêm trọng (mức ≥ 4)", B.articles + " bài nguồn") +
        k("Ransomware", num(ransom), "vụ", B.ransom.total ? num(B.ransom.total) + " nạn nhân mới trên leak site" : "", "") +
        k("Dữ liệu bị lộ", recs ? bigParts(recs)[0] : "—", recs ? bigParts(recs)[1] : "", recs ? "bản ghi · " + withRec + " vụ có số liệu" : "", "") +
        k("Nhóm tấn công", num(actors), "nhóm", "", "được nêu tên") +
        k("Quốc gia bị nhắm", num(ctry), "nước", "", "") +
        k("CVE bị khai thác mới", num(B.kev.length), "", kevRw + " dùng trong ransomware", "CISA KEV trong kỳ") +
        k("Liên quan Việt Nam", num(vn), "", "", "");
    }

    function chips() {
      var el = $("sec-chips");
      if (!el) return;
      var html = "";
      Object.keys(FLAB).forEach(function (k) {
        if (F[k]) html += "<button type='button' class='fchip' data-clear='" + k + "'>" + esc(FLAB[k]) + ": <b>" + esc(fval(k, F[k])) + "</b> ✕</button>";
      });
      if (html) html += "<button type='button' class='fchip clear' data-clear='*'>Xoá lọc</button>";
      el.innerHTML = html;
    }

    // ---- bản đồ
    function loadGeo() {
      if (geoState) return;
      geoState = 1;
      fetch("/static/world.json").then(function (r) { return r.json(); }).then(function (g) {
        echarts.registerMap("world", g);
        B.__poly = {};
        g.features.forEach(function (f) { B.__poly[f.properties.name] = true; });
        geoState = 2;
        renderMap(tok(), B.incidents.filter(match));
      }).catch(function () { geoState = -1; var el = $("sec-map"); if (el) el.innerHTML = "<div class='empty'>Không tải được dữ liệu bản đồ — xem bảng dữ liệu bên dưới.</div>"; });
    }
    function renderMap(t, rs) {
      var vic = {}, ori = {}, flows = {};
      rs.forEach(function (i) {
        var ps = places(i);
        ps.forEach(function (c) { vic[c] = (vic[c] || 0) + 1; });
        if (i.origin) {
          ori[i.origin] = (ori[i.origin] || 0) + 1;
          ps.forEach(function (c) { if (c !== i.origin) { var k = i.origin + ">" + c; flows[k] = (flows[k] || 0) + 1; } });
        }
      });
      var all = uniq(Object.keys(vic).concat(Object.keys(ori))).sort(function (a, b) { return (vic[b] || 0) + (ori[b] || 0) - (vic[a] || 0) - (ori[a] || 0); });
      table("sec-map-tbl", ["Quốc gia", "Là nạn nhân (sự cố)", "Là nguồn tấn công (sự cố)"], all.map(function (c) { return [flag(c) + " " + cname(c), vic[c] || 0, ori[c] || 0]; }));
      bars("sec-victims", tally(rs, places).slice(0, 12), function (c) { return flag(c) + " " + cname(c); }, "country", t.heat[3]);
      bars("sec-origins", tally(rs, function (i) { return [i.origin]; }).slice(0, 12), function (c) { return flag(c) + " " + cname(c); }, "origin", t.heat[4]);
      if (geoState !== 2) { loadGeo(); return; }
      var mel = $("sec-map");
      if (mel) el_h("sec-map", Math.round(Math.max(260, Math.min(560, mel.clientWidth * 0.52))));
      var c = chart("sec-map");
      if (!c) return;
      var val = mapMode === "victim" ? vic : ori;
      var vmax = 1;
      Object.keys(val).forEach(function (k) { vmax = Math.max(vmax, val[k]); });
      var data = Object.keys(val).map(function (k) { return { name: k, value: val[k] }; });
      var small = Object.keys(val).filter(function (k) { return !B.__poly[k] && B.countries[k]; }).map(function (k) {
        var x = B.countries[k]; return { name: k, value: [x[1], x[2], val[k]] };
      });
      var lines = Object.keys(flows).map(function (k) {
        var p = k.split(">"), a = B.countries[p[0]], b = B.countries[p[1]];
        if (!a || !b) return null;
        return { from: p[0], to: p[1], n: flows[k], coords: [[a[1], a[2]], [b[1], b[2]]], lineStyle: { width: Math.min(5, 0.8 + Math.sqrt(flows[k]) * 0.9) } };
      }).filter(Boolean);
      function tipFor(code) {
        var fams = tally(rs.filter(function (i) { return has(places(i), code); }), function (i) { return i.fam; }).slice(0, 3);
        return "<b>" + flag(code) + " " + esc(cname(code)) + "</b><br>Nạn nhân: " + (vic[code] || 0) + " sự cố · Nguồn tấn công: " + (ori[code] || 0) +
          (fams.length ? "<br><span style='color:" + t.ink2 + "'>" + fams.map(function (f) { return esc(FAMVI[f[0]]) + " " + f[1]; }).join(" · ") + "</span>" : "") +
          "<br><span style='color:" + t.muted + "'>bấm để lọc</span>";
      }
      var sel = mapMode === "victim" ? F.country : F.origin;
      c.setOption(merge(V.base(t), {
        tooltip: tip(t, { trigger: "item", formatter: function (p) {
          if (p.seriesType === "lines") return "<b>" + flag(p.data.from) + " " + esc(cname(p.data.from)) + " → " + flag(p.data.to) + " " + esc(cname(p.data.to)) + "</b><br>" + p.data.n + " sự cố được quy kết";
          var code = p.name; return code && B.countries[code] ? tipFor(code) : esc(code || "");
        } }),
        visualMap: { type: "continuous", min: 0, max: vmax, seriesIndex: 0, calculable: false, orient: "horizontal", left: 8, bottom: 6, itemWidth: 10, itemHeight: 140,
          text: [String(vmax), "0"], textStyle: { color: t.muted, fontSize: 12 }, inRange: { color: [t.heat[0], t.heat[4]] } },
        geo: { map: "world", roam: true, scaleLimit: { min: 1, max: 10 }, zoom: 1.12, center: [20, 18], silent: false, selectedMode: false,
          itemStyle: { areaColor: t.land, borderColor: t.surface, borderWidth: 0.6 },
          emphasis: { label: { show: false }, itemStyle: { areaColor: t.accent } },
          regions: [{ name: "VN", itemStyle: { borderColor: t.ink, borderWidth: 1.4 } }].concat(sel ? [{ name: sel, itemStyle: { borderColor: t.ink, borderWidth: 2 } }] : []) },
        series: [
          { type: "map", geoIndex: 0, data: data },
          { type: "lines", coordinateSystem: "geo", zlevel: 2, data: lines, lineStyle: { color: t.flow, opacity: 0.6, curveness: 0.28 },
            effect: { show: true, period: 5, trailLength: 0.2, symbol: "arrow", symbolSize: 6, color: t.flow }, emphasis: { lineStyle: { opacity: 1 } } },
          { type: "scatter", coordinateSystem: "geo", zlevel: 3, data: small, symbolSize: function (v) { return 6 + Math.min(14, Math.sqrt(v[2]) * 3); },
            itemStyle: { color: t.heat[4], borderColor: t.surface, borderWidth: 2 } },
        ],
      }), true);
      on(c, "click", function (p) {
        if (p.seriesType === "lines") { F.origin = p.data.from; setF("country", p.data.to, true); return; }
        var code = p.name;
        if (code && B.countries[code]) setF(mapMode === "victim" ? "country" : "origin", code);
      });
      var lg = $("map-legend");
      if (lg) lg.innerHTML = "<span><i style='background:" + t.heat[4] + "'></i>" + (mapMode === "victim" ? "nhiều sự cố nhắm vào" : "nhiều sự cố xuất phát từ") + "</span>" +
        "<span><i style='background:" + t.land + "'></i>chưa ghi nhận</span><span><i style='background:transparent;border-top:2px solid " + t.flow + ";border-radius:0;height:0;vertical-align:3px'></i>luồng tấn công (nguồn → nạn nhân)</span>" +
        "<span><i style='background:transparent;border:1.5px solid " + t.ink + "'></i>Việt Nam</span>";
    }

    // thanh ngang HTML (danh sách ngắn, bấm để lọc)
    function bars(id, kvs, label, fkey, color) {
      var el = $(id);
      if (!el) return;
      if (!kvs.length) { el.innerHTML = "<div class='sub'>Chưa có dữ liệu.</div>"; return; }
      var mx = kvs[0][1];
      el.innerHTML = kvs.map(function (kv) {
        var on = F[fkey] === kv[0] ? " on" : "";
        return "<button type='button' class='hbar" + on + "' data-f='" + fkey + "' data-v='" + esc(kv[0]) + "'><span class='l'>" + esc(label(kv[0])) +
          "</span><span class='b'><i style='width:" + Math.max(3, 100 * kv[1] / mx) + "%;background:" + color + "'></i></span><span class='n'>" + kv[1] + "</span></button>";
      }).join("");
    }

    // ---- sankey
    function renderSankey(t, rs) {
      var c = chart("sec-sankey");
      if (!c) return;
      var oTop = tally(rs, function (i) { return [i.origin]; }).slice(0, 6).map(function (x) { return x[0]; });
      var sTop = tally(rs, function (i) { return (i.sec || []).slice(0, 1); }).slice(0, 10).map(function (x) { return x[0]; });
      var links = {}, nodes = {};
      function node(id, name, color) { if (!nodes[id]) nodes[id] = { name: id, label: name, itemStyle: { color: color, borderColor: t.surface } }; }
      rs.forEach(function (i) {
        var o = i.origin ? (has(oTop, i.origin) ? "o:" + i.origin : "o:+") : "o:?";
        var f = (i.fam || [])[0] || "_";
        var s0 = (i.sec || [])[0], s = s0 ? (has(sTop, s0) ? "s:" + s0 : "s:+") : "s:?";
        node(o, o === "o:?" ? "Chưa quy kết" : o === "o:+" ? "Nước khác" : flag(i.origin) + " " + cname(i.origin), t.ink2);
        node("f:" + f, f === "_" ? "Khác" : FAMVI[f], famColor(t, f));
        node(s, s === "s:?" ? "Chưa rõ ngành" : s === "s:+" ? "Ngành khác" : SECVI[s0], t.muted);
        [[o, "f:" + f], ["f:" + f, s]].forEach(function (p) { var k = p[0] + "|" + p[1]; links[k] = (links[k] || 0) + 1; });
      });
      var ls = Object.keys(links).map(function (k) {
        var p = k.split("|"), fam = p[0].indexOf("f:") === 0 ? p[0].slice(2) : p[1].slice(2);
        return { source: p[0], target: p[1], value: links[k], lineStyle: { color: famColor(t, fam), opacity: 0.35 } };
      });
      var ns = Object.keys(nodes).map(function (k) { return nodes[k]; });
      el_h("sec-sankey", Math.max(420, Math.min(820, 34 * Math.max(oTop.length + 2, sTop.length + 2, 9) + 60)));
      c.setOption(merge(V.base(t), {
        tooltip: tip(t, { trigger: "item", formatter: function (p) {
          if (p.dataType === "edge") return esc(nodes[p.data.source].label) + " → " + esc(nodes[p.data.target].label) + ": <b>" + p.data.value + "</b> sự cố";
          return "<b>" + esc(p.data.label) + "</b>: " + p.value + " sự cố<br><span style='color:" + t.muted + "'>bấm để lọc</span>";
        } }),
        series: [{ type: "sankey", left: 8, right: 150, top: 12, bottom: 12, nodeWidth: 12, nodeGap: 10, draggable: false, layoutIterations: 48,
          emphasis: { focus: "adjacency" }, data: ns, links: ls,
          label: { color: t.ink, fontSize: 12.5, formatter: function (p) { return trunc(p.data.label, 30) + "  " + p.value; } },
          lineStyle: { curveness: 0.5 } }],
      }), true);
      on(c, "click", function (p) {
        if (p.dataType !== "node") return;
        var id = p.data.name, v = id.slice(2);
        if (v === "?" || v === "+") return;
        if (id[0] === "o") setF("origin", v); else if (id[0] === "f") setF("fam", v); else setF("sector", v);
      });
      table("sec-sankey-tbl", ["Từ", "Tới", "Số sự cố"], ls.map(function (l) { return [nodes[l.source].label, nodes[l.target].label, l.value]; }));
    }
    function el_h(id, h) { var el = $(id); if (el && parseInt(el.style.height, 10) !== h) { el.style.height = h + "px"; if (inst[id]) inst[id].resize(); } }

    // ---- xu hướng
    function renderTrend(t, rs) {
      var c = chart("sec-trend");
      if (!c) return;
      var keys = FAM.concat(["_"]);
      var byDay = {};
      rs.forEach(function (i) { var f = (i.fam || [])[0] || "_"; byDay[i.d] = byDay[i.d] || {}; byDay[i.d][f] = (byDay[i.d][f] || 0) + 1; });
      var series = keys.map(function (f) {
        return { name: f === "_" ? "Khác" : FAMVI[f], type: "bar", stack: "x", barMaxWidth: 22, emphasis: { focus: "series" },
          itemStyle: { color: famColor(t, f), borderColor: t.surface, borderWidth: 1 }, data: B.days.map(function (d) { return (byDay[d] || {})[f] || 0; }), _f: f };
      }).filter(function (s) { return s.data.some(function (x) { return x > 0; }); });
      if (series.length) series[series.length - 1].itemStyle.borderRadius = [4, 4, 0, 0];
      c.setOption(merge(V.base(t), {
        legend: V.legend(t), grid: { left: 36, right: 12, top: c.getWidth() < 700 ? 96 : 66, bottom: 28 },
        tooltip: tip(t, { trigger: "axis", axisPointer: { type: "shadow", shadowStyle: { color: t.grid, opacity: 0.35 } } }),
        xAxis: merge({ type: "category", data: B.days.map(function (d) { return d.slice(5); }) }, V.axisStyle(t)),
        yAxis: merge({ type: "value", minInterval: 1 }, V.axisStyle(t)),
        series: series,
      }), true);
      on(c, "click", function (p) { var s = series[p.seriesIndex]; if (s) setF("fam", s._f); });
      table("sec-trend-tbl", ["Ngày"].concat(series.map(function (s) { return s.name; })), B.days.map(function (d, i) { return [d].concat(series.map(function (s) { return s.data[i]; })); }));
    }

    function hbarChart(id, t, kvs, label, color, click, tblCols) {
      var c = chart(id);
      if (!c) return;
      el_h(id, Math.max(200, 26 * kvs.length + 40));
      c.setOption(merge(V.base(t), {
        grid: { left: 8, right: 40, top: 6, bottom: 22, containLabel: true },
        tooltip: tip(t, { trigger: "axis", axisPointer: { type: "none" } }),
        xAxis: merge({ type: "value", minInterval: 1 }, V.axisStyle(t)),
        yAxis: merge({ type: "category", inverse: true, data: kvs.map(function (x) { return label(x[0]); }),
          axisLabel: { color: t.ink2, fontSize: 12, formatter: function (v) { return trunc(v, 32); } } }, { axisLine: { lineStyle: { color: t.axis } }, axisTick: { show: false } }),
        series: [{ type: "bar", barMaxWidth: 16, data: kvs.map(function (x) { return x[1]; }), itemStyle: { color: color, borderRadius: [0, 4, 4, 0] },
          label: { show: true, position: "right", color: t.ink2, fontSize: 12 } }],
      }), true);
      on(c, "click", function (p) { if (click) click(kvs[p.dataIndex][0]); });
      table(id + "-tbl", tblCols, kvs.map(function (x) { return [label(x[0]), x[1]]; }));
    }

    function renderLeak(t) {
      var c = chart("sec-leak");
      if (c) {
        var ds = B.days.map(function (d) { return B.ransom.days[d] || 0; });
        c.setOption(merge(V.base(t), {
          grid: { left: 36, right: 12, top: 12, bottom: 28 },
          tooltip: tip(t, { trigger: "axis", axisPointer: { type: "shadow", shadowStyle: { color: t.grid, opacity: 0.35 } } }),
          xAxis: merge({ type: "category", data: B.days.map(function (d) { return d.slice(5); }) }, V.axisStyle(t)),
          yAxis: merge({ type: "value", minInterval: 1 }, V.axisStyle(t)),
          series: [{ name: "Nạn nhân mới", type: "bar", barMaxWidth: 18, data: ds, itemStyle: { color: t.s[0], borderRadius: [4, 4, 0, 0] } }],
        }), true);
        table("sec-leak-tbl", ["Ngày", "Nạn nhân mới"], B.days.map(function (d, i) { return [d, ds[i]]; }));
        if (!B.ransom.total) $("sec-leak").innerHTML = "<div class='empty'>Chưa có dữ liệu leak site (bật <code>security.ransom_url</code>).</div>";
      }
      hbarChart("sec-groups", t, (B.ransom.groups || []).filter(function (x) { return x.k !== "(khác)"; }).map(function (x) { return [x.k, x.n]; }), function (g) { return g; }, t.s[0], null, ["Nhóm", "Nạn nhân"]);
      var rest = (B.ransom.groups || []).filter(function (x) { return x.k === "(khác)"; })[0], gn = $("sec-groups-note");
      if (gn) gn.textContent = rest ? "Các nhóm khác: " + num(rest.n) + " nạn nhân." : "";
      var unk = 0, ls = [];
      (B.ransom.sectors || []).forEach(function (x) { if (x.k === "unknown") unk = x.n; else if (x.k !== "other") ls.push([x.k, x.n]); });
      (B.ransom.sectors || []).forEach(function (x) { if (x.k === "other") ls.push([x.k, x.n]); });
      hbarChart("sec-leaksec", t, ls, function (s) { return SECVI[s] || s; }, t.s[0], function (s) { if (s !== "other") setF("sector", s); }, ["Ngành", "Nạn nhân"]);
      var nt = $("sec-leaksec-note");
      if (nt) nt.textContent = unk ? "Ngoài ra " + num(unk) + " nạn nhân không ghi ngành." : "";
    }

    // ---- ngành × hình thức
    function renderHeat(t, rs) {
      var c = chart("sec-heat");
      if (!c) return;
      var secs = tally(rs, function (i) { return i.sec; }).map(function (x) { return x[0]; });
      var fams = FAM.filter(function (f) { return rs.some(function (i) { return has(i.fam, f); }); });
      var data = [], mx = 1;
      secs.forEach(function (s, y) {
        fams.forEach(function (f, x) {
          var n = rs.filter(function (i) { return has(i.sec, s) && has(i.fam, f); }).length;
          if (n) { data.push([x, y, n]); mx = Math.max(mx, n); }
        });
      });
      // chữ trong ô đổi trắng/đen theo độ đậm của ô (cùng nội suy với visualMap)
      var cells = data.map(function (d) {
        var bg = mix(t.seqLo, t.seqHi, mx > 1 ? (d[2] - 1) / (mx - 1) : 1);
        return { value: d, label: { color: lum(bg) > 0.55 ? "#0b0b0b" : "#ffffff" } };
      });
      el_h("sec-heat", Math.max(260, 30 * secs.length + 110));
      c.setOption(merge(V.base(t), {
        grid: { left: 8, right: 16, top: 10, bottom: 70, containLabel: true },
        tooltip: tip(t, { formatter: function (p) { return "<b>" + esc(SECVI[secs[p.value[1]]]) + "</b> × " + esc(FAMVI[fams[p.value[0]]]) + "<br>" + p.value[2] + " sự cố<br><span style='color:" + t.muted + "'>bấm để lọc</span>"; } }),
        xAxis: merge({ type: "category", data: fams.map(function (f) { return FAMVI[f]; }), axisLabel: { color: t.ink2, fontSize: 11.5, interval: 0,
          width: Math.max(56, (c.getWidth() - 190) / Math.max(1, fams.length) - 8), overflow: "break", lineHeight: 14 } }, { axisLine: { lineStyle: { color: t.axis } }, axisTick: { show: false } }),
        yAxis: merge({ type: "category", inverse: true, data: secs.map(function (s) { return SECVI[s] || s; }), axisLabel: { color: t.ink2, fontSize: 12 } }, { axisLine: { lineStyle: { color: t.axis } }, axisTick: { show: false } }),
        visualMap: { min: 1, max: mx, calculable: false, orient: "horizontal", left: "center", bottom: 0, itemWidth: 10, itemHeight: 120,
          text: [String(mx), "1"], textStyle: { color: t.muted, fontSize: 12 }, inRange: { color: [t.seqLo, t.seqHi] } },
        series: [{ type: "heatmap", data: cells, label: { show: true, fontSize: 11.5, formatter: function (p) { return p.value[2]; } },
          itemStyle: { borderColor: t.surface, borderWidth: 2, borderRadius: 3 }, emphasis: { itemStyle: { borderColor: t.ink, borderWidth: 1 } } }],
      }), true);
      on(c, "click", function (p) {
        var s = secs[p.value[1]], f = fams[p.value[0]];
        if (F.sector === s && F.fam === f) { F.sector = null; F.fam = null; update(); return; }
        F.sector = s; setF("fam", f, true);
      });
      var rows = [];
      secs.forEach(function (s) { rows.push([SECVI[s] || s].concat(fams.map(function (f) { var d = data.filter(function (x) { return secs[x[1]] === s && fams[x[0]] === f; })[0]; return d ? d[2] : 0; }))); });
      table("sec-heat-tbl", ["Ngành"].concat(fams.map(function (f) { return FAMVI[f]; })), rows);
    }

    // ---- nhóm tấn công
    function renderActors(rs) {
      var el = $("sec-actors");
      if (!el) return;
      var g = {};
      rs.forEach(function (i) {
        if (!i.actor) return;
        var a = g[i.actor] = g[i.actor] || { n: 0, origin: i.origin, mot: i.mot, fam: [], sec: [], tech: {}, last: "", sev: 0, vc: [] };
        a.n++; a.origin = a.origin || i.origin; if (i.mot !== "unknown") a.mot = i.mot;
        a.fam = uniq(a.fam.concat(i.fam || [])); a.sec = uniq(a.sec.concat(i.sec || [])); a.vc = uniq(a.vc.concat(places(i)));
        (i.tech || []).forEach(function (x) { a.tech[x] = (a.tech[x] || 0) + 1; });
        if (i.l > a.last) a.last = i.l; a.sev = Math.max(a.sev, i.sev);
      });
      var all = Object.keys(g).sort(function (a, b) { return g[b].n - g[a].n || g[b].sev - g[a].sev; });
      var names = all.slice(0, actorsAll ? 60 : 10);
      if (!names.length) { el.innerHTML = "<div class='sub'>Chưa có nhóm nào được nêu tên trong lát dữ liệu này.</div>"; return; }
      el.innerHTML = "<table class='actors'><thead><tr><th>Nhóm · nguồn · động cơ</th><th class='num'>Sự cố</th><th>Hình thức</th><th>Nhắm vào</th><th>Kỹ thuật chính</th><th>Gần nhất</th></tr></thead><tbody>" +
        names.map(function (n) {
          var a = g[n], tech = Object.keys(a.tech).sort(function (x, y) { return a.tech[y] - a.tech[x]; }).slice(0, 4);
          return "<tr><td><button type='button' class='lnk' data-f='actor' data-v='" + esc(n) + "'><b>" + esc(n) + "</b></button><br><small>" +
            (a.origin ? "<button type='button' class='lnk' data-f='origin' data-v='" + a.origin + "'>" + flag(a.origin) + " " + esc(cname(a.origin)) + "</button>" : "nguồn chưa rõ") +
            " · " + esc(MOTVI[a.mot] || "") + "</small></td><td class='num'>" + a.n + "</td><td class='fams'>" +
            a.fam.map(function (f) { return "<span class='dot f-" + FAM.indexOf(f) + "'></span> " + esc(FAMVI[f]); }).join("<br>") +
            "</td><td>" + esc(a.sec.slice(0, 3).map(function (s) { return SECVI[s]; }).join(" · ")) + (a.vc.length ? "<br><small>" + esc(a.vc.slice(0, 5).map(cname).join(", ")) + "</small>" : "") +
            "</td><td>" + tech.map(function (x) { return "<button type='button' class='code' data-f='tech' data-v='" + x + "' title='" + esc((B.tech[x] || {}).n || "") + "'>" + x + "</button>"; }).join(" ") +
            "</td><td class='nw'>" + esc(a.last) + "</td></tr>";
        }).join("") + "</tbody></table>" +
        (all.length > 10 ? "<button type='button' class='more' id='sec-actors-more'>" + (actorsAll ? "Thu gọn" : "Xem tất cả " + all.length + " nhóm") + "</button>" : "");
    }

    // ---- ma trận ATT&CK
    function renderAttack(t, rs) {
      var el = $("sec-attack");
      if (!el) return;
      var cnt = {};
      rs.forEach(function (i) { uniq(i.tech || []).forEach(function (x) { cnt[x] = (cnt[x] || 0) + 1; }); });
      var mx = 1;
      Object.keys(cnt).forEach(function (k) { mx = Math.max(mx, cnt[k]); });
      var cols = L.tactics.map(function (tc) {
        var ts = Object.keys(cnt).filter(function (x) { return B.tech[x] && B.tech[x].tac === tc.id; }).sort(function (a, b) { return cnt[b] - cnt[a] || (a < b ? -1 : 1); });
        var tot = ts.reduce(function (s, x) { return s + cnt[x]; }, 0);
        var hidden = attackAll ? 0 : Math.max(0, ts.length - 8);
        var vis = hidden ? ts.slice(0, 8) : ts;
        return "<div class='tac'><div class='th'><b>" + esc(tc.vi) + "</b><small>" + (tot ? ts.length + " kỹ thuật · " + tot + " lượt" : "chưa ghi nhận") + "</small></div>" +
          (vis.length ? vis.map(function (x) {
            var k = cnt[x] / mx, bg = mix(t.seqLo, t.seqHi, 0.15 + 0.85 * k), fg = lum(bg) > 0.55 ? "#0b0b0b" : "#ffffff", on = F.tech === x ? " on" : "";
            return "<button type='button' class='tc" + on + "' data-f='tech' data-v='" + x + "' style='background:" + bg + ";color:" + fg + "' title='" + esc(x + " " + B.tech[x].n + " — " + cnt[x] + " sự cố") + "'>" +
              "<span class='id'>" + x + "</span><span class='nm'>" + esc(B.tech[x].n) + "</span><span class='ct'>" + cnt[x] + "</span></button>";
          }).join("") + (hidden ? "<button type='button' class='tcmore' data-attack-all>+" + hidden + " kỹ thuật</button>" : "") : "<div class='none'>—</div>") + "</div>";
      }).join("");
      el.innerHTML = "<div class='grid'>" + cols + "</div><div class='legend'><span><i style='background:" + t.seqLo + "'></i>ít sự cố</span><span><i style='background:" + t.seqHi + "'></i>nhiều sự cố (tối đa " + mx + ")</span></div>";
    }

    // ---- CVE & KEV
    function renderVulns(t, rs) {
      var el = $("sec-cves");
      if (el) {
        var cv = tally(rs, function (i) { return i.cve; });
        cv.sort(function (a, b) {
          var x = B.cves[a[0]] || {}, y = B.cves[b[0]] || {};
          return (!!y.kev - !!x.kev) || ((y.epss || 0) - (x.epss || 0)) || ((y.cvss || 0) - (x.cvss || 0)) || (b[1] - a[1]);
        });
        if (!cv.length) el.innerHTML = "<div class='sub'>Không có CVE trong lát dữ liệu này.</div>";
        else el.innerHTML = "<table class='cves'><thead><tr><th>CVE</th><th>Sản phẩm</th><th class='num'>CVSS</th><th>EPSS (30 ngày)</th><th>Khai thác</th><th class='num'>Sự cố</th></tr></thead><tbody>" +
          cv.slice(0, 40).map(function (x) {
            var m = B.cves[x[0]] || {}, sev = m.cvss >= 9 ? ["critical", "Critical"] : m.cvss >= 7 ? ["serious", "High"] : m.cvss >= 4 ? ["warning", "Medium"] : m.cvss > 0 ? ["neutral", "Low"] : null;
            var ep = m.epss ? "<span class='meter'><i style='width:" + Math.max(2, m.epss * 100) + "%'></i></span> " + (m.epss * 100).toFixed(m.epss < 0.1 ? 1 : 0) + "%" : "<span class='sub'>—</span>";
            var ex = (m.kev ? "<span class='st critical'><i>■</i> KEV " + esc(m.kev) + "</span>" : m.x ? "<span class='st serious'><i>▼</i> đã bị khai thác</span>" : "<span class='sub'>chưa ghi nhận</span>") + (m.rw ? " <span class='badge'>ransomware</span>" : "");
            return "<tr><td><button type='button' class='code' data-f='cve' data-v='" + x[0] + "'>" + x[0] + "</button> <a class='ext' href='https://nvd.nist.gov/vuln/detail/" + x[0] + "' target='_blank' rel='noopener' title='NVD'>↗</a></td><td>" + esc(m.p || "") +
              "</td><td class='num'>" + (sev ? "<span class='st " + sev[0] + "'>" + m.cvss.toFixed(1) + " " + sev[1] + "</span>" : "—") + "</td><td>" + ep + "</td><td>" + ex + "</td><td class='num'>" + x[1] + "</td></tr>";
          }).join("") + "</tbody></table>";
      }
      var vend = {}, vrw = {};
      B.kev.forEach(function (k) { vend[k.v] = (vend[k.v] || 0) + 1; if (k.rw) vrw[k.v] = (vrw[k.v] || 0) + 1; });
      var vs = Object.keys(vend).sort(function (a, b) { return vend[b] - vend[a] || (a < b ? -1 : 1); }).slice(0, 14);
      var c = chart("sec-kev");
      if (c) {
        el_h("sec-kev", Math.max(220, 26 * vs.length + 40));
        c.setOption(merge(V.base(t), {
          grid: { left: 8, right: 24, top: 6, bottom: 22, containLabel: true },
          tooltip: tip(t, { trigger: "axis", axisPointer: { type: "none" } }),
          xAxis: merge({ type: "value", minInterval: 1 }, V.axisStyle(t)),
          yAxis: merge({ type: "category", inverse: true, data: vs, axisLabel: { color: t.ink2, fontSize: 12, formatter: function (v) { return trunc(v, 24); } } }, { axisLine: { lineStyle: { color: t.axis } }, axisTick: { show: false } }),
          series: [
            { name: "Dùng trong ransomware", type: "bar", stack: "k", barMaxWidth: 16, data: vs.map(function (v) { return vrw[v] || 0; }), itemStyle: { color: t.s[1], borderColor: t.surface, borderWidth: 1 } },
            { name: "Khác", type: "bar", stack: "k", barMaxWidth: 16, data: vs.map(function (v) { return vend[v] - (vrw[v] || 0); }), itemStyle: { color: t.s[0], borderColor: t.surface, borderWidth: 1, borderRadius: [0, 4, 4, 0] } },
          ],
        }), true);
        var lg = $("sec-kev-legend");
        if (lg) lg.innerHTML = "<span><i style='background:" + t.s[1] + "'></i>đã dùng trong ransomware</span><span><i style='background:" + t.s[0] + "'></i>khác</span>";
        table("sec-kev-tbl", ["Vendor", "CVE trong KEV", "Dùng trong ransomware"], vs.map(function (v) { return [v, vend[v], vrw[v] || 0]; }));
        if (!B.kev.length) $("sec-kev").innerHTML = "<div class='empty'>Không có CVE mới trong KEV kỳ này.</div>";
      }
      var cw = {};
      B.kev.forEach(function (k) { (k.cwe || []).forEach(function (x) { cw[x] = (cw[x] || 0) + 1; }); });
      var cws = Object.keys(cw).map(function (k) { return [k, cw[k]]; }).sort(function (a, b) { return b[1] - a[1]; }).slice(0, 14);
      hbarChart("sec-cwe", t, cws, function (x) { return x + (CWEVI[x] ? " · " + CWEVI[x] : ""); }, t.s[0], null, ["CWE", "Số CVE"]);
      var kl = $("sec-kevlist");
      if (kl && !kl.__done) {
        kl.__done = true;
        kl.innerHTML = "<table><thead><tr><th>CVE</th><th>Vendor / sản phẩm</th><th>Lỗ hổng</th><th>Thêm vào</th><th>Hạn vá</th><th class='num'>EPSS</th><th>Ransomware</th></tr></thead><tbody>" +
          B.kev.map(function (k) {
            return "<tr><td><a href='https://nvd.nist.gov/vuln/detail/" + k.cve + "' target='_blank' rel='noopener'><code>" + k.cve + "</code></a></td><td>" + esc(k.v + " " + k.p) + "</td><td title='" + esc(k.desc || "") + "'>" + esc(k.n) +
              (k.cwe && k.cwe.length ? "<br><small>" + esc(k.cwe.map(function (x) { return x + (CWEVI[x] ? " " + CWEVI[x] : ""); }).join(", ")) + "</small>" : "") + "</td><td>" + k.d + "</td><td>" + esc(k.due || "") +
              "</td><td class='num'>" + (k.epss ? (k.epss * 100).toFixed(1) + "%" : "—") + "</td><td>" + (k.rw ? "<span class='st critical'><i>■</i> có</span>" : "") + "</td></tr>";
          }).join("") + "</tbody></table>";
      }
    }

    // ---- stack người dùng: mức phơi nhiễm từng công nghệ
    var EXPO = { high: ["critical", "■", "Phơi nhiễm cao"], medium: ["serious", "▼", "Trung bình"], low: ["warning", "◆", "Thấp"], none: ["neutral", "●", "Chưa thấy"] };
    function renderStack(rs) {
      var el = $("sec-stack");
      if (!el || !(B.stack || []).length) return;
      var notes = {};
      (BR.stack_notes || []).forEach(function (n) { notes[(n.stack || "").toLowerCase()] = n; });
      var order = { high: 0, medium: 1, low: 2, none: 3 };
      var cards = B.stack.map(function (h, idx) {
        var n = notes[h.name.toLowerCase()] || null;
        var lv = n && EXPO[n.exposure] ? n.exposure : h.n > 0 && h.sev >= 4 ? "high" : h.events > 0 ? "medium" : "none";
        var live = rs.filter(function (i) { return has(i.stk, idx); }).length;
        return { h: h, idx: idx, n: n, lv: lv, live: live };
      }).sort(function (a, b) { return order[a.lv] - order[b.lv] || b.h.events - a.h.events; });
      el.innerHTML = cards.map(function (c) {
        var e = EXPO[c.lv], h = c.h, on = String(F.stack) === String(c.idx) ? " on" : "";
        var stats = h.n + " sự cố/chiến dịch · " + h.events + " sự kiện" + ((h.cves || []).length ? " · " + h.cves.length + " CVE" : "") + ((h.kev || []).length ? " · " + h.kev.length + " trong KEV" : "");
        var cv = (h.cves || []).concat((h.kev || []).filter(function (x) { return (h.cves || []).indexOf(x) < 0; })).slice(0, 6)
          .map(function (x) { return "<button type='button' class='code' data-f='cve' data-v='" + x + "'>" + x + "</button>"; }).join(" ");
        return "<div class='stk " + e[0] + on + "' data-stack='" + c.idx + "' role='button' tabindex='0'><div class='h'><b>" + esc(h.name) + "</b><span class='st " + e[0] + "'><i>" + e[1] + "</i> " + e[2] + "</span></div>" +
          "<div class='sub'>" + esc(stats) + (F.stack != null || c.live !== h.events ? "" : "") + "</div>" +
          (c.n && c.n.risk ? "<p>" + esc(c.n.risk) + "</p>" : (h.top || []).length ? "<p>" + esc(h.top.slice(0, 2).join(" · ")) + "</p>" : "<p class='sub'>Chưa thấy sự kiện nào chạm tới công nghệ này trong kỳ.</p>") +
          (c.n && (c.n.actions || []).length ? "<ul>" + c.n.actions.map(function (a) { return "<li>" + esc(a) + "</li>"; }).join("") + "</ul>" : "") +
          (cv ? "<div class='cv'>" + cv + "</div>" : "") + "</div>";
      }).join("");
    }

    // ---- playbook phòng thủ
    var CK = "fa-sec-ck";
    function ckState() { try { return JSON.parse(localStorage.getItem(CK) || "{}"); } catch (e) { return {}; } }
    function ckSave(s) { try { localStorage.setItem(CK, JSON.stringify(s)); } catch (e) {} }
    function renderDefense(t) {
      var ctr = BR.controls || [];
      if (!ctr.length) return;
      var PR = { P0: t.lv.macro, P1: t.lv.meso, P2: t.lv.micro }, EF = { S: 0, M: 1, L: 2 };
      var c = chart("sec-prio");
      if (c) {
        var cnt = {};
        var series = ["P0", "P1", "P2"].map(function (p) {
          return { name: p, type: "scatter", symbolSize: 16, itemStyle: { color: PR[p], borderColor: t.surface, borderWidth: 2 },
            label: { show: true, position: "right", color: t.ink2, fontSize: 11.5, formatter: function (x) { return trunc(x.data.name, 26); } }, labelLayout: { hideOverlap: true },
            data: ctr.filter(function (x) { return x.priority === p; }).map(function (x) {
              var k = x.effort + x.impact; cnt[k] = (cnt[k] || 0) + 1; var j = cnt[k] - 1, ang = j * 2.4, r = j ? 0.12 + 0.03 * j : 0;
              return { name: x.control, why: x.why, layer: x.layer, value: [(EF[x.effort] || 0) + r * Math.cos(ang), x.impact + r * Math.sin(ang) * 1.4] };
            }) };
        });
        c.setOption(merge(V.base(t), {
          grid: { left: 40, right: 24, top: 16, bottom: 44 },
          tooltip: tip(t, { formatter: function (p) { return "<b>" + esc(p.data.name) + "</b> — " + p.seriesName + "<br>" + esc(LAYVI[p.data.layer] || "") + "<br><span style='color:" + t.ink2 + "'>" + esc(trunc(p.data.why, 220)) + "</span>"; } }),
          xAxis: merge({ type: "value", min: -0.5, max: 2.5, interval: 1, name: "công sức →", nameLocation: "middle", nameGap: 28, nameTextStyle: { color: t.muted },
            axisLabel: { color: t.ink2, formatter: function (v) { return ["Nhỏ (S)", "Vừa (M)", "Lớn (L)"][Math.round(v)] || ""; } } }, { splitLine: { lineStyle: { color: t.grid } }, axisLine: { lineStyle: { color: t.axis } }, axisTick: { show: false } }),
          yAxis: merge({ type: "value", min: 0, max: 6, interval: 1, name: "tác động", nameTextStyle: { color: t.muted, align: "left" },
            axisLabel: { color: t.muted, formatter: function (v) { return v >= 1 && v <= 5 ? v : ""; } } }, { splitLine: { lineStyle: { color: t.grid } }, axisLine: { show: false } }),
          series: series,
        }), true);
        var lg = $("sec-prio-legend");
        if (lg) lg.innerHTML = "<span><i style='background:" + PR.P0 + "'></i>P0 — làm ngay</span><span><i style='background:" + PR.P1 + "'></i>P1</span><span><i style='background:" + PR.P2 + "'></i>P2</span>";
        table("sec-prio-tbl", ["Biện pháp", "Ưu tiên", "Tầng", "Công sức", "Tác động"], ctr.map(function (x) { return [x.control, x.priority, LAYVI[x.layer] || x.layer, x.effort, x.impact]; }));
      }
      var ly = $("sec-layers");
      if (ly) {
        var st = ckState(), boxes = document.querySelectorAll(".controls input[data-ck]");
        var done = {}, tot = {};
        boxes.forEach(function (b) { var l = b.closest(".ctrl").getAttribute("data-layer"); tot[l] = (tot[l] || 0) + 1; if (st[b.getAttribute("data-ck")]) done[l] = (done[l] || 0) + 1; });
        ly.innerHTML = L.layers.map(function (l) {
          var xs = ctr.filter(function (x) { return x.layer === l.id; });
          if (!xs.length) return "<div class='layer empty-l'><span class='l'>" + esc(l.vi) + "</span><span class='sub'>chưa có biện pháp — điểm mù?</span></div>";
          var seg = ["P0", "P1", "P2"].map(function (p) { var n = xs.filter(function (x) { return x.priority === p; }).length; return n ? "<i style='flex:" + n + ";background:" + PR[p] + "' title='" + p + ": " + n + "'></i>" : ""; }).join("");
          return "<div class='layer'><span class='l'>" + esc(l.vi) + "</span><span class='seg'>" + seg + "</span><span class='n'>" + (done[l.id] || 0) + "/" + (tot[l.id] || xs.length) + " ✓</span></div>";
        }).join("");
      }
    }

    // ---- danh sách sự cố
    function dots(n) { return "<span class='sevd s" + n + "' title='Mức nghiêm trọng " + n + "/5'>" + "●●●●●".slice(0, n) + "<span>" + "●●●●●".slice(0, 5 - n) + "</span></span>"; }
    function tag(f, v, text, cls) { return "<button type='button' class='tag " + (cls || "") + "' data-f='" + f + "' data-v='" + esc(v) + "'>" + text + "</button>"; }
    function renderList(rs) {
      var el = $("sec-list");
      if (!el) return;
      var xs = rs.slice();
      if (sortBy === "sev") xs.sort(function (a, b) { return b.sev - a.sev || (b.l > a.l ? 1 : -1); });
      else if (sortBy === "src") xs.sort(function (a, b) { return b.src.length - a.src.length || b.sev - a.sev; });
      $("sec-count").textContent = xs.length + " sự kiện" + (F.scope === "threats" ? " (sự cố & chiến dịch)" : "");
      var more = $("sec-more");
      if (more) more.style.display = xs.length > shown ? "" : "none";
      if (!xs.length) { el.innerHTML = "<div class='empty'>Không có sự kiện khớp bộ lọc.</div>"; return; }
      el.innerHTML = xs.slice(0, shown).map(function (i) {
        var tags = (i.fam || []).map(function (f) { return tag("fam", f, "<i class='dot f-" + FAM.indexOf(f) + "'></i>" + esc(FAMVI[f])); }).join("");
        if (i.actor) tags += tag("actor", i.actor, "🕵️ " + esc(i.actor) + (i.origin ? " " + flag(i.origin) : "") + (i.attr === "suspected" ? " <small>(nghi vấn)</small>" : ""));
        else if (i.origin) tags += tag("origin", i.origin, "Nguồn: " + flag(i.origin) + " " + esc(cname(i.origin)));
        places(i).slice(0, 6).forEach(function (c) { tags += tag("country", c, flag(c) + " " + esc(cname(c))); });
        (i.sec || []).slice(0, 4).forEach(function (s) { tags += tag("sector", s, esc(SECVI[s] || s), "lt"); });
        var tech = (i.tech || []).map(function (x) { var m = B.tech[x] || {}; return "<button type='button' class='code' data-f='tech' data-v='" + x + "' title='" + esc(TACVI[m.tac] || "") + "'>" + x + "</button> <span class='sub'>" + esc(m.n || "") + "</span>"; }).join("<br>");
        var cves = (i.cve || []).map(function (x) { var m = B.cves[x] || {}; return "<button type='button' class='code' data-f='cve' data-v='" + x + "'>" + x + "</button>" + (m.kev ? " <span class='badge'>KEV</span>" : "") + (m.cvss ? " <span class='sub'>CVSS " + m.cvss + "</span>" : ""); }).join("<br>");
        var imp = [i.imp, i.rec ? big(i.rec) + " bản ghi bị lộ" : "", i.usd ? "$" + big(i.usd) : ""].filter(Boolean).join(" · ");
        return "<article class='inc s" + i.sev + "'><header>" + dots(i.sev) + "<b>" + esc(i.t) + "</b><span class='kind'>" + esc(KINDVI[i.kind] || i.kind) + "</span><span class='date'>" + esc(i.d === i.l ? i.d : i.d + " → " + i.l) + " · " + i.src.length + " nguồn</span></header>" +
          "<div class='tags'>" + tags + "</div><p>" + esc(i.sum) + "</p>" + (i.vn ? "<p class='vn'>🇻🇳 " + esc(i.vn) + "</p>" : "") +
          "<details><summary>Đặc tả kỹ thuật, kỹ thuật ATT&CK, CVE & cách phòng thủ</summary><div class='det'>" +
          "<div><h4>Diễn biến kỹ thuật</h4><p>" + esc(i.det || "—") + "</p>" + (imp ? "<h4>Tác động</h4><p>" + esc(imp) + "</p>" : "") +
          ((i.vec || []).length ? "<h4>Cửa ngõ xâm nhập</h4><p>" + (i.vec || []).map(function (v) { return tag("vec", v, esc(VECVI[v] || v), "lt"); }).join("") + "</p>" : "") +
          ((i.prod || []).length ? "<h4>Sản phẩm / hệ thống</h4><p>" + esc(i.prod.join(", ")) + "</p>" : "") +
          ((i.vic || []).length ? "<h4>Nạn nhân được nêu</h4><p>" + esc(i.vic.join(", ")) + "</p>" : "") +
          ((i.types || []).length ? "<h4>Loại tấn công</h4><p>" + esc(i.types.map(function (x) { return TYPEVI[x] || x; }).join(", ")) + "</p>" : "") + "</div>" +
          "<div>" + (tech ? "<h4>Kỹ thuật ATT&CK</h4><p>" + tech + "</p>" : "") + (cves ? "<h4>CVE</h4><p>" + cves + "</p>" : "") + "</div></div>" +
          ((i.def || []).length ? "<h4>🧱 Phòng thủ cho dự án</h4><ul class='def'>" + i.def.map(function (d) { return "<li>" + esc(d) + "</li>"; }).join("") + "</ul>" : "") +
          "<h4>Nguồn</h4><ul class='src'>" + i.src.map(function (s) { return "<li><a href='" + esc(s.u) + "' target='_blank' rel='noopener'>" + esc(s.t) + "</a> <span class='sub'>" + esc(s.s) + " · " + esc(s.d) + "</span></li>"; }).join("") + "</ul>" +
          "</details></article>";
      }).join("");
    }

    // ---------- vẽ ----------
    var T0 = null;
    function update() {
      var t = T0 || (T0 = tok()), rs = B.incidents.filter(match);
      chips(); syncURL(); kpis(t, rs); renderMap(t, rs); renderSankey(t, rs); renderTrend(t, rs); renderHeat(t, rs);
      hbarChart("sec-vectors", t, tally(rs, function (i) { return i.vec; }), function (v) { return VECVI[v] || v; }, t.s[0], function (v) { setF("vec", v); }, ["Cửa ngõ", "Sự cố"]);
      renderActors(rs); renderAttack(t, rs); renderVulns(t, rs); renderStack(rs); renderList(rs);
      document.querySelectorAll(".card.actor").forEach(function (cd) { cd.classList.toggle("on", F.actor === cd.getAttribute("data-actor")); });
    }
    function full() {
      T0 = tok();
      Object.keys(inst).forEach(function (k) { inst[k].dispose(); });
      inst = {};
      renderLeak(T0); renderDefense(T0); update();
    }
    V.hooks.push(full);

    // ---------- tương tác ----------
    document.addEventListener("click", function (e) {
      var x = e.target.closest("[data-f]");
      if (x && x.closest(".sec")) { e.preventDefault(); setF(x.getAttribute("data-f"), x.getAttribute("data-v")); return; }
      var cl = e.target.closest("[data-clear]");
      if (cl) {
        var k = cl.getAttribute("data-clear");
        if (k === "*") Object.keys(FLAB).forEach(function (q) { F[q] = null; }); else F[k] = null;
        update(); return;
      }
      var sk = e.target.closest("[data-stack]");
      if (sk) { setF("stack", sk.getAttribute("data-stack")); return; }
      var ac = e.target.closest(".card.actor");
      if (ac) { setF("actor", ac.getAttribute("data-actor")); document.getElementById("actors").scrollIntoView({ behavior: "smooth" }); return; }
      var ct = e.target.closest("[data-counter]");
      if (ct) {
        var v = ct.getAttribute("data-counter").trim(), m = v.match(/T\d{4}(\.\d{3})?/);
        if (m) setF("tech", m[0], true); else if (FAM.indexOf(v) >= 0) setF("fam", v, true); else { F.q = v.toLowerCase(); var qi = document.querySelector("#sec-filterbar input[name=q]"); if (qi) qi.value = v; update(); }
        document.getElementById("explorer").scrollIntoView({ behavior: "smooth" });
        return;
      }
      if (e.target.closest("#sec-actors-more")) { actorsAll = !actorsAll; renderActors(B.incidents.filter(match)); return; }
      if (e.target.closest("[data-attack-all]")) { attackAll = true; renderAttack(T0 || tok(), B.incidents.filter(match)); return; }
      var mm = e.target.closest("#map-mode button");
      if (mm) {
        mapMode = mm.getAttribute("data-m");
        document.querySelectorAll("#map-mode button").forEach(function (b) { b.classList.toggle("on", b === mm); });
        renderMap(T0 || tok(), B.incidents.filter(match));
      }
    });
    var bar = $("sec-filterbar");
    if (bar) {
      bar.addEventListener("change", function (e) {
        if (e.target.name === "scope") F.scope = e.target.value;
        if (e.target.name === "vn") F.vn = e.target.checked;
        shown = 25; update();
      });
      var qt;
      bar.addEventListener("input", function (e) {
        if (e.target.name !== "q") return;
        clearTimeout(qt); qt = setTimeout(function () { F.q = e.target.value.trim().toLowerCase(); shown = 25; update(); }, 200);
      });
    }
    var so = $("sec-sort");
    if (so) so.addEventListener("change", function () { sortBy = so.value; renderList(B.incidents.filter(match)); });
    var mo = $("sec-more");
    if (mo) mo.addEventListener("click", function () { shown += 25; renderList(B.incidents.filter(match)); });
    // checklist phòng thủ (lưu trên trình duyệt)
    var st = ckState();
    document.querySelectorAll(".controls input[data-ck]").forEach(function (b) {
      b.checked = !!st[b.getAttribute("data-ck")];
      b.closest(".ctrl").classList.toggle("done", b.checked);
      b.addEventListener("click", function (e) { e.stopPropagation(); });
      b.addEventListener("change", function () {
        var s = ckState(); s[b.getAttribute("data-ck")] = b.checked; ckSave(s);
        b.closest(".ctrl").classList.toggle("done", b.checked);
        renderDefense(T0 || tok());
      });
    });
    var to;
    window.addEventListener("resize", function () { clearTimeout(to); to = setTimeout(function () { Object.keys(inst).forEach(function (k) { inst[k].resize(); }); }, 150); });
  });
})();
