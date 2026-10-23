/* feed-analyze — renderer chart (ECharts). Mỗi .chart[data-viz] đọc JSON từ <script id=data-src>,
   vẽ theo token CSS hiện hành (light/dark) và sinh bảng dữ liệu tương đương (tooltip không bao giờ là
   đường duy nhất để đọc giá trị). */
(function () {
  "use strict";
  var charts = [];
  var LEVELS = ["micro", "meso", "macro"];
  var LEVEL_VI = { micro: "Vi mô", meso: "Trung mô", macro: "Vĩ mô" };
  var STATUS_VI = { "new": "mới", developing: "tiếp diễn", escalating: "leo thang", cooling: "hạ nhiệt", resolved: "kết thúc" };

  function css(name) { return getComputedStyle(document.documentElement).getPropertyValue(name).trim(); }
  function T() {
    return {
      ink: css("--ink"), ink2: css("--ink-2"), muted: css("--muted"), grid: css("--grid"), axis: css("--axis"),
      surface: css("--card"), page: css("--page"), accent: css("--accent"),
      s: [1, 2, 3, 4, 5, 6, 7, 8].map(function (i) { return css("--s" + i); }),
      lv: { micro: css("--lv-micro"), meso: css("--lv-meso"), macro: css("--lv-macro") },
      seqLo: css("--seq-lo"), seqHi: css("--seq-hi"),
      pos: css("--div-pos"), neg: css("--div-neg"), mid: css("--div-mid"),
    };
  }
  function meta() {
    var el = document.getElementById("fa-meta");
    return el ? JSON.parse(el.textContent) : { topics: [] };
  }
  // màu theo thực thể (topic), không theo thứ hạng: slot cố định theo thứ tự topic trong config
  function topicColor(t, id, M) {
    for (var i = 0; i < M.topics.length; i++) if (M.topics[i].id === id) return i < 8 ? t.s[i] : t.muted;
    return t.muted;
  }
  function topicName(id, M) {
    for (var i = 0; i < M.topics.length; i++) if (M.topics[i].id === id) return M.topics[i].emoji + " " + M.topics[i].name;
    return id;
  }
  // site tĩnh công khai: trang lưu dạng thư mục → link kết thúc bằng "/"
  function pageURL(p) { return meta().public ? p + "/" : p; }
  function esc(s) { return String(s == null ? "" : s).replace(/[&<>"]/g, function (c) { return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]; }); }
  function trunc(s, n) { s = String(s || ""); return s.length > n ? s.slice(0, n - 1) + "…" : s; }

  function base(t) {
    return {
      backgroundColor: "transparent",
      textStyle: { fontFamily: 'system-ui,-apple-system,"Segoe UI",Roboto,sans-serif', color: t.ink2 },
      animationDuration: 300,
      tooltip: {
        backgroundColor: t.surface, borderColor: t.grid, borderWidth: 1, textStyle: { color: t.ink, fontSize: 13 },
        extraCssText: "box-shadow:0 4px 14px rgba(0,0,0,.12);border-radius:8px;max-width:360px;white-space:normal;",
        confine: true,
      },
    };
  }
  function axisStyle(t) {
    return {
      axisLine: { lineStyle: { color: t.axis } }, axisTick: { show: false },
      axisLabel: { color: t.muted, fontSize: 12 },
      splitLine: { lineStyle: { color: t.grid, width: 1, type: "solid" } },
    };
  }
  function legend(t, extra) {
    return Object.assign({ top: 0, left: 0, icon: "roundRect", itemWidth: 10, itemHeight: 10, itemGap: 16,
      textStyle: { color: t.ink2, fontSize: 12.5 } }, extra || {});
  }
  function merge(a, b) { for (var k in b) a[k] = b[k]; return a; }

  // ---------- renderers: trả về {cols, rows} cho bảng dữ liệu ----------
  var R = {};

  // Khối lượng bài liên quan theo ngày × topic (cột chồng)
  R.volume = function (el, d, t, M) {
    var days = d.days, series = M.topics.map(function (tp, i) {
      var byDay = {}; d.rows.forEach(function (r) { if (r.topic === tp.id) byDay[r.day] = r.n; });
      return {
        name: tp.emoji + " " + tp.name, type: "bar", stack: "v", barMaxWidth: 24,
        itemStyle: { color: topicColor(t, tp.id, M), borderColor: t.surface, borderWidth: 1 },
        emphasis: { focus: "series" },
        data: days.map(function (x) { return byDay[x] || 0; }),
      };
    });
    series[series.length - 1].itemStyle.borderRadius = [4, 4, 0, 0];
    return { opt: merge(base(t), {
      legend: legend(t), grid: { left: 36, right: 12, top: 34, bottom: 28 },
      tooltip: merge(base(t).tooltip, { trigger: "axis", axisPointer: { type: "shadow", shadowStyle: { color: t.grid, opacity: .35 } } }),
      xAxis: merge({ type: "category", data: days.map(function (x) { return x.slice(5); }) }, axisStyle(t)),
      yAxis: merge({ type: "value", minInterval: 1 }, axisStyle(t)),
      series: series,
    }), table: { cols: ["Ngày"].concat(series.map(function (s) { return s.name; })),
      rows: days.map(function (x, i) { return [x].concat(series.map(function (s) { return s.data[i]; })); }) } };
  };

  // Phân bố theo tầng / vùng cho từng topic (thanh ngang chồng 100%)
  R.facet = function (el, d, t, M) {
    var keys = d.keys, labels = d.labels, colors = d.palette === "level" ? keys.map(function (k) { return t.lv[k]; }) : [t.s[3], t.s[4], t.s[5]];
    var topics = M.topics.map(function (x) { return x.id; });
    var tot = {}; d.rows.forEach(function (r) { tot[r.topic] = (tot[r.topic] || 0) + r.n; });
    var series = keys.map(function (k, i) {
      return {
        name: labels[i], type: "bar", stack: "f", barMaxWidth: 22,
        itemStyle: { color: colors[i], borderColor: t.surface, borderWidth: 1 },
        data: topics.map(function (tp) {
          var r = d.rows.filter(function (x) { return x.topic === tp && x.key === k; })[0];
          return tot[tp] ? Math.round(100 * (r ? r.n : 0) / tot[tp]) : 0;
        }),
      };
    });
    return { opt: merge(base(t), {
      legend: legend(t), grid: { left: 8, right: 16, top: 34, bottom: 8, containLabel: true },
      tooltip: merge(base(t).tooltip, { trigger: "axis", axisPointer: { type: "none" }, valueFormatter: function (v) { return v + "%"; } }),
      xAxis: merge({ type: "value", max: 100, axisLabel: { color: t.muted, formatter: "{value}%" } }, { splitLine: axisStyle(t).splitLine, axisLine: { show: false } }),
      yAxis: merge({ type: "category", inverse: true, data: topics.map(function (id) { return topicName(id, M); }) }, axisStyle(t)),
      series: series,
    }), table: { cols: ["Chủ đề"].concat(labels).concat(["Tổng bài"]),
      rows: topics.map(function (tp, ti) { return [topicName(tp, M)].concat(series.map(function (s) { return s.data[ti] + "%"; })).concat([tot[tp] || 0]); }) } };
  };

  // Ma trận storyline × ngày (heatmap, màu = mức quan trọng, tuần tự 1 hue)
  R.storyheat = function (el, d, t, M) {
    var ys = d.stories.map(function (s) { return s.slug; });
    var data = d.cells.map(function (c) { return [d.days.indexOf(c.date), ys.indexOf(c.slug), c.importance || 1, c.status, c.articles]; })
      .filter(function (x) { return x[0] >= 0 && x[1] >= 0; });
    el.style.height = Math.max(260, 26 * ys.length + 70) + "px";
    var byslug = {}; d.stories.forEach(function (s) { byslug[s.slug] = s; });
    return { opt: merge(base(t), {
      grid: { left: 8, right: 16, top: 10, bottom: 56, containLabel: true },
      tooltip: merge(base(t).tooltip, { formatter: function (p) {
        var s = d.stories[p.value[1]];
        return "<b>" + esc(s.title) + "</b><br>" + esc(topicName(s.topic, M)) + " · " + d.days[p.value[0]] +
          "<br>Quan trọng " + p.value[2] + "/5 · " + esc(STATUS_VI[p.value[3]] || p.value[3]) + " · " + p.value[4] + " bài";
      } }),
      xAxis: merge({ type: "category", data: d.days.map(function (x) { return x.slice(5); }), splitArea: { show: false } }, axisStyle(t)),
      yAxis: merge({ type: "category", inverse: true, data: ys, axisLabel: { color: t.ink2, fontSize: 12,
        formatter: function (v) { var s = byslug[v]; return trunc(s.title, 34); } } }, { axisLine: { lineStyle: { color: t.axis } }, axisTick: { show: false } }),
      visualMap: { min: 1, max: 5, calculable: false, orient: "horizontal", left: "center", bottom: 0, itemWidth: 12, itemHeight: 120,
        text: ["quan trọng 5", "1"], textStyle: { color: t.muted, fontSize: 12 }, inRange: { color: [t.seqLo, t.seqHi] } },
      series: [{ type: "heatmap", data: data, itemStyle: { borderColor: t.surface, borderWidth: 2, borderRadius: 3 },
        emphasis: { itemStyle: { borderColor: t.ink, borderWidth: 1 } } }],
    }), click: function (p) { location.href = pageURL("/s/" + encodeURIComponent(ys[p.value[1]])); },
      table: { cols: ["Storyline", "Chủ đề", "Ngày", "Trạng thái", "Quan trọng", "Bài"],
        rows: d.cells.map(function (c) { var s = byslug[c.slug] || {}; return [s.title, topicName(s.topic, M), c.date, STATUS_VI[c.status] || c.status, c.importance, c.articles]; }) } };
  };

  // Thực thể nổi bật (thanh ngang chồng theo topic)
  R.entities = function (el, d, t, M) {
    var names = d.map(function (e) { return e.name; });
    var series = M.topics.map(function (tp) {
      return { name: tp.emoji + " " + tp.name, type: "bar", stack: "e", barMaxWidth: 16,
        itemStyle: { color: topicColor(t, tp.id, M), borderColor: t.surface, borderWidth: 1 },
        data: d.map(function (e) { return e.topics[tp.id] || 0; }) };
    });
    el.style.height = Math.max(260, 22 * names.length + 90) + "px";
    return { opt: merge(base(t), {
      legend: legend(t), grid: { left: 8, right: 24, top: 64, bottom: 8, containLabel: true },
      tooltip: merge(base(t).tooltip, { trigger: "axis", axisPointer: { type: "none" } }),
      xAxis: merge({ type: "value", minInterval: 1 }, axisStyle(t)),
      yAxis: merge({ type: "category", inverse: true, data: names, axisLabel: { color: t.ink2, fontSize: 12, formatter: function (v) { return trunc(v, 26); } } }, { axisLine: { lineStyle: { color: t.axis } }, axisTick: { show: false } }),
      series: series,
    }), click: meta().public ? null : function (p) { location.href = "/articles?days=" + (d.days || 7) + "&q=" + encodeURIComponent(names[p.dataIndex]); },
      table: { cols: ["Thực thể", "Số bài"].concat(series.map(function (s) { return s.name; })),
        rows: d.map(function (e) { return [e.name, e.n].concat(M.topics.map(function (tp) { return e.topics[tp.id] || 0; })); }) } };
  };

  // Chi phí Claude theo ngày (1 chuỗi, màu trung tính — ngữ cảnh)
  R.cost = function (el, d, t) {
    return { opt: merge(base(t), {
      grid: { left: 44, right: 12, top: 14, bottom: 28 },
      tooltip: merge(base(t).tooltip, { trigger: "axis", axisPointer: { type: "shadow", shadowStyle: { color: t.grid, opacity: .35 } },
        formatter: function (ps) { var r = d[ps[0].dataIndex]; return r.day + "<br>$" + r.cost.toFixed(3) + " · " + r.calls + " lần gọi" + (r.fails ? " · " + r.fails + " lỗi" : ""); } }),
      xAxis: merge({ type: "category", data: d.map(function (r) { return r.day.slice(5); }) }, axisStyle(t)),
      yAxis: merge({ type: "value", axisLabel: { color: t.muted, formatter: "${value}" } }, { splitLine: axisStyle(t).splitLine, axisLine: { show: false } }),
      series: [{ type: "bar", barMaxWidth: 24, itemStyle: { color: t.muted, borderRadius: [4, 4, 0, 0] }, data: d.map(function (r) { return +r.cost.toFixed(3); }) }],
    }), table: { cols: ["Ngày", "Chi phí (USD)", "Lần gọi", "Lỗi"], rows: d.map(function (r) { return [r.day, r.cost.toFixed(3), r.calls, r.fails]; }) } };
  };

  // Bản đồ storyline: tầng (x) × mức quan trọng (y), kích thước = số bài, màu = topic
  R.storymap = function (el, d, t, M) {
    var jit = {};
    var series = M.topics.slice(0, 8).map(function (tp) {
      return {
        name: tp.emoji + " " + tp.name, type: "scatter",
        itemStyle: { color: topicColor(t, tp.id, M), borderColor: t.surface, borderWidth: 2, opacity: .9 },
        label: { show: true, position: "right", color: t.ink2, fontSize: 11.5, formatter: function (p) { return p.data.imp >= 5 || p.data.n >= 10 ? trunc(p.data.title, 24) : ""; } },
        labelLayout: { hideOverlap: true },
        symbolSize: function (v, p) { return Math.max(10, Math.min(38, 8 + 3 * Math.sqrt(p.data.n || 1) * 2)); },
        data: d.filter(function (s) { return s.topic === tp.id; }).map(function (s) {
          var x = LEVELS.indexOf(s.level), key = x + ":" + s.importance; jit[key] = (jit[key] || 0) + 1;
          var k = jit[key] - 1, ang = k * 2.4, r = k === 0 ? 0 : 0.13 + 0.035 * k;
          return { value: [x + r * Math.cos(ang) * 1.6, s.importance + r * Math.sin(ang)], title: s.title, slug: s.slug, n: s.articles, imp: s.importance, status: s.status, summary: s.summary };
        }),
      };
    });
    return { opt: merge(base(t), {
      legend: legend(t), grid: { left: 40, right: el.clientWidth < 640 ? 70 : 24, top: el.clientWidth < 640 ? 80 : 56, bottom: 34 },
      tooltip: merge(base(t).tooltip, { formatter: function (p) {
        return "<b>" + esc(p.data.title) + "</b><br>" + esc(p.seriesName) + " · " + esc(STATUS_VI[p.data.status] || p.data.status) +
          " · quan trọng " + p.data.imp + "/5 · " + p.data.n + " bài<br><span style='color:" + t.ink2 + "'>" + esc(trunc(p.data.summary, 220)) + "</span>";
      } }),
      xAxis: merge({ type: "value", min: -0.6, max: 2.6, axisLabel: { color: t.ink2, fontSize: 12.5, customValues: [0, 1, 2], formatter: function (v) { return LEVEL_VI[LEVELS[Math.round(v)]] || ""; } },
        axisTick: { show: false, customValues: [0, 1, 2] } },
        { splitLine: { show: false }, axisLine: { lineStyle: { color: t.axis } } }),
      yAxis: merge({ type: "value", min: 0, max: 6, interval: 1, name: "mức quan trọng", nameGap: 12, nameTextStyle: { color: t.muted, fontSize: 12, align: "left" },
        axisLabel: { color: t.muted, formatter: function (v) { return v >= 1 && v <= 5 ? v : ""; } } }, { splitLine: { lineStyle: { color: t.grid } }, axisLine: { show: false, onZero: false }, axisTick: { show: false } }),
      series: series,
    }), click: function (p) { location.href = pageURL("/s/" + encodeURIComponent(p.data.slug)); },
      table: { cols: ["Storyline", "Chủ đề", "Tầng", "Quan trọng", "Trạng thái", "Bài"],
        rows: d.map(function (s) { return [s.title, topicName(s.topic, M), LEVEL_VI[s.level] || s.level, s.importance, STATUS_VI[s.status] || s.status, s.articles]; }) } };
  };

  // Mạng liên kết chéo giữa storyline (đồ thị lực)
  R.network = function (el, d, t, M) {
    var nodes = d.nodes.map(function (n) {
      return { id: n.id, name: n.label, topic: n.topic, slug: n.slug, symbolSize: n.slug ? 22 : 14,
        itemStyle: { color: n.slug ? topicColor(t, n.topic, M) : t.surface, borderColor: n.slug ? t.surface : topicColor(t, n.topic, M), borderWidth: 2 },
        label: { show: true, position: "right", color: t.ink2, fontSize: 11.5, formatter: function () { return trunc(n.label, 30); } } };
    });
    var cats = M.topics.map(function (tp) { return tp.emoji + " " + tp.name; });
    return { opt: merge(base(t), {
      tooltip: merge(base(t).tooltip, { formatter: function (p) {
        if (p.dataType === "edge") return "<b>" + esc(p.data.mechanism) + "</b><br>⇒ " + esc(p.data.implication);
        return "<b>" + esc(p.data.name) + "</b><br>" + esc(topicName(p.data.topic, M)) + (p.data.slug ? "<br><span style='color:" + t.muted + "'>bấm để mở storyline</span>" : "");
      } }),
      series: [{
        type: "graph", layout: "force", roam: true, draggable: true, top: 10, bottom: 10,
        force: { repulsion: 340, edgeLength: [100, 170], gravity: 0.22, friction: 0.5 },
        edgeSymbol: ["none", "arrow"], edgeSymbolSize: 8,
        lineStyle: { color: t.axis, width: 1.5, curveness: 0.15, opacity: .9 },
        emphasis: { focus: "adjacency", lineStyle: { width: 2.5, color: t.ink2 } },
        labelLayout: { hideOverlap: true },
        data: nodes,
        links: d.links.map(function (l) { return { source: l.from, target: l.to, mechanism: l.mechanism, implication: l.implication }; }),
      }],
    }), click: function (p) { if (p.dataType === "node" && p.data.slug) location.href = pageURL("/s/" + encodeURIComponent(p.data.slug)); },
      legendHTML: M.topics.map(function (tp) { return "<span><i style='background:" + topicColor(t, tp.id, M) + "'></i>" + esc(tp.emoji + " " + tp.name) + "</span>"; }).join("") +
        "<span><i style='background:transparent;border:2px solid " + t.muted + "'></i>khái niệm ngoài storyline</span>",
      table: { cols: ["Từ", "Tới", "Cơ chế", "Hệ quả"], rows: d.links.map(function (l) {
        var f = d.nodes.filter(function (n) { return n.id === l.from; })[0] || {}, to = d.nodes.filter(function (n) { return n.id === l.to; })[0] || {};
        return [f.label, to.label, l.mechanism, l.implication]; }) } };
  };

  // Đồ thị nhân quả phân tầng (vi mô → trung mô → vĩ mô)
  R.causal = function (el, d, t) {
    var cols = { micro: [], meso: [], macro: [] };
    d.nodes.forEach(function (n) { (cols[n.level] || cols.meso).push(n); });
    var maxRows = Math.max(1, cols.micro.length, cols.meso.length, cols.macro.length);
    el.style.height = Math.max(300, 92 * maxRows + 70) + "px";
    var hdr = el.previousElementSibling;
    if (!hdr || !hdr.classList.contains("colhdr")) {
      hdr = document.createElement("div"); hdr.className = "colhdr"; el.parentNode.insertBefore(hdr, el);
    }
    hdr.innerHTML = LEVELS.map(function (lv) { return "<span><i style='background:" + t.lv[lv] + "'></i>" + LEVEL_VI[lv] + "</span>"; }).join("<b>→</b>");
    var W = 1000, H = 100 * maxRows;
    var nodes = [];
    LEVELS.forEach(function (lv, ci) {
      cols[lv].forEach(function (n, ri) {
        var y = (ri + 0.5) * H / cols[lv].length;
        nodes.push({ id: n.id, name: n.label, level: lv, x: 120 + ci * (W - 240) / 2, y: y, symbol: "roundRect", symbolSize: [190, 46],
          itemStyle: { color: t.surface, borderColor: t.lv[lv], borderWidth: 2 },
          label: { show: true, color: t.ink, fontSize: 12, width: 176, overflow: "break", lineHeight: 15, formatter: function () { return trunc(n.label, 70); } } });
      });
    });
    var edges = d.edges.map(function (e) {
      return { source: e.from, target: e.to, channel: e.channel, strength: e.strength,
        lineStyle: { width: 0.8 + e.strength * 1.1, color: t.axis, curveness: 0.12 },
        label: { show: true, formatter: trunc(e.channel, 22), color: t.ink2, fontSize: 11, backgroundColor: t.surface, padding: [1, 3], borderRadius: 3 } };
    });
    return { opt: merge(base(t), {
      tooltip: merge(base(t).tooltip, { formatter: function (p) {
        if (p.dataType === "edge") return "Kênh: <b>" + esc(p.data.channel) + "</b><br>Độ mạnh " + p.data.strength + "/3";
        return "<b>" + esc(p.data.name) + "</b><br>" + LEVEL_VI[p.data.level];
      } }),
      series: [{ type: "graph", layout: "none", roam: true, top: 30, bottom: 30, left: 104, right: 104,
        edgeSymbol: ["none", "arrow"], edgeSymbolSize: 9, labelLayout: { hideOverlap: true },
        emphasis: { focus: "adjacency", lineStyle: { color: t.ink2 } },
        data: nodes, links: edges }],
    }), table: { cols: ["Từ", "Tầng", "→ Tới", "Tầng", "Kênh", "Độ mạnh"], rows: d.edges.map(function (e) {
      var f = d.nodes.filter(function (n) { return n.id === e.from; })[0] || {}, to = d.nodes.filter(function (n) { return n.id === e.to; })[0] || {};
      return [f.label, LEVEL_VI[f.level], to.label, LEVEL_VI[to.level], e.channel, e.strength]; }) } };
  };

  // Kịch bản: xác suất (1 chuỗi, thanh ngang, nhãn ở đầu thanh)
  R.scenarios = function (el, d, t) {
    el.style.height = Math.max(180, 54 * d.length + 40) + "px";
    return { opt: merge(base(t), {
      grid: { left: 8, right: 48, top: 8, bottom: 24, containLabel: true },
      tooltip: merge(base(t).tooltip, { formatter: function (p) { var s = d[p.dataIndex];
        return "<b>" + esc(s.name) + " — " + s.probability + "%</b><br>Điều kiện: " + esc(s.conditions) + "<br>Tín hiệu sớm: " + esc(s.signals); } }),
      xAxis: merge({ type: "value", max: 100, axisLabel: { color: t.muted, formatter: "{value}%" } }, { splitLine: axisStyle(t).splitLine, axisLine: { show: false } }),
      yAxis: merge({ type: "category", inverse: true, data: d.map(function (s) { return s.name; }), axisLabel: { color: t.ink, fontSize: 12.5, formatter: function (v) { return trunc(v, 30); } } }, { axisLine: { lineStyle: { color: t.axis } }, axisTick: { show: false } }),
      series: [{ type: "bar", barMaxWidth: 22, data: d.map(function (s) { return s.probability; }),
        itemStyle: { color: t.accent, borderRadius: [0, 4, 4, 0] },
        label: { show: true, position: "right", color: t.ink, fontWeight: 600, formatter: "{c}%" } }],
    }), table: { cols: ["Kịch bản", "Xác suất", "Điều kiện", "Tín hiệu sớm"], rows: d.map(function (s) { return [s.name, s.probability + "%", s.conditions, s.signals]; }) } };
  };

  // Bản đồ các bên: lập trường (x, phân kỳ) × ảnh hưởng (y)
  R.stakeholders = function (el, d, t) {
    var cnt = {};
    var data = d.map(function (a) {
      var k = a.stance + ":" + a.influence; cnt[k] = (cnt[k] || 0) + 1;
      var j = cnt[k] - 1, ang = j * 2.4, rr = j === 0 ? 0 : 0.2 + 0.05 * j;
      var col = a.stance > 0 ? t.pos : a.stance < 0 ? t.neg : t.muted;
      return { influence: a.influence, value: [a.stance + rr * Math.cos(ang) * 1.4, a.influence + rr * Math.sin(ang)], name: a.name, interest: a.interest, stance: a.stance,
        itemStyle: { color: col, borderColor: t.surface, borderWidth: 2 } };
    });
    var st = { "-2": "bị thiệt nhiều / phản đối", "-1": "bị thiệt", "0": "trung lập", "1": "hưởng lợi", "2": "hưởng lợi nhiều / ủng hộ" };
    return { opt: merge(base(t), {
      grid: { left: 44, right: 40, top: 34, bottom: 48 },
      tooltip: merge(base(t).tooltip, { formatter: function (p) { return "<b>" + esc(p.data.name) + "</b><br>" + st[p.data.stance] + " · ảnh hưởng " + p.data.influence + "/5<br>" + esc(p.data.interest); } }),
      xAxis: merge({ type: "value", min: -3, max: 3, interval: 1, name: "← bị thiệt / phản đối · hưởng lợi / ủng hộ →", nameLocation: "middle", nameGap: 28,
        nameTextStyle: { color: t.muted, fontSize: 12 }, axisLabel: { color: t.muted, formatter: function (v) { return Math.abs(v) <= 2 ? (v > 0 ? "+" + v : v) : ""; } } },
        { splitLine: { lineStyle: { color: t.grid } }, axisLine: { show: false }, axisTick: { show: false } }),
      yAxis: merge({ type: "value", min: 0, max: 6, interval: 1, name: "mức ảnh hưởng", nameGap: 12, nameTextStyle: { color: t.muted, fontSize: 12, align: "left" },
        axisLabel: { color: t.muted, formatter: function (v) { return v >= 1 && v <= 5 ? v : ""; } } }, { splitLine: { lineStyle: { color: t.grid } }, axisLine: { show: false } }),
      series: [{ type: "scatter", symbolSize: 14, data: data,
        label: { show: true, position: "right", color: t.ink, fontSize: 12, formatter: function (p) { return trunc(p.data.name, 22); } },
        labelLayout: { hideOverlap: true },
        markLine: { silent: true, symbol: "none", lineStyle: { color: t.axis, type: "solid", width: 1 }, label: { show: false }, data: [{ xAxis: 0 }] } }],
    }), legendHTML: "<span><i style='background:" + t.pos + "'></i>hưởng lợi / ủng hộ</span><span><i style='background:" + t.muted + "'></i>trung lập</span><span><i style='background:" + t.neg + "'></i>bị thiệt / phản đối</span>",
      table: { cols: ["Bên liên quan", "Lập trường", "Ảnh hưởng", "Lợi ích/động cơ"], rows: d.map(function (a) { return [a.name, st[a.stance], a.influence, a.interest]; }) } };
  };

  // Tác động tới VN: thanh phân kỳ -2..+2
  R.vnimpact = function (el, d, t) {
    el.style.height = Math.max(200, 44 * d.length + 50) + "px";
    var lab = { "-2": "rất tiêu cực", "-1": "tiêu cực", "0": "trung tính", "1": "tích cực", "2": "rất tích cực" };
    return { opt: merge(base(t), {
      grid: { left: 8, right: 30, top: 10, bottom: 28, containLabel: true },
      tooltip: merge(base(t).tooltip, { formatter: function (p) { var x = d[p.dataIndex]; return "<b>" + esc(x.sector) + "</b> — " + lab[x.score] + "<br>" + esc(x.why); } }),
      xAxis: merge({ type: "value", min: -2, max: 2, interval: 1, axisLabel: { color: t.muted, formatter: function (v) { return v > 0 ? "+" + v : v; } } }, { splitLine: axisStyle(t).splitLine, axisLine: { show: false } }),
      yAxis: merge({ type: "category", inverse: true, data: d.map(function (x) { return x.sector; }), axisLabel: { color: t.ink, fontSize: 12.5, formatter: function (v) { return trunc(v, 30); } } }, { axisLine: { show: false }, axisTick: { show: false } }),
      series: [{ type: "bar", barMaxWidth: 20,
        data: d.map(function (x) { var v = x.score === 0 ? 0.08 : x.score;
          return { value: v, itemStyle: { color: x.score > 0 ? t.pos : x.score < 0 ? t.neg : t.muted, borderRadius: x.score >= 0 ? [0, 4, 4, 0] : [4, 0, 0, 4] } }; }),
        label: { show: true, position: "outside", color: t.ink2, fontSize: 12, formatter: function (p) { return lab[d[p.dataIndex].score]; } },
        markLine: { silent: true, symbol: "none", lineStyle: { color: t.axis, type: "solid" }, label: { show: false }, data: [{ xAxis: 0 }] } }],
    }), legendHTML: "<span><i style='background:" + t.pos + "'></i>tích cực</span><span><i style='background:" + t.neg + "'></i>tiêu cực</span>",
      table: { cols: ["Lĩnh vực", "Điểm", "Đánh giá", "Lý do"], rows: d.map(function (x) { return [x.sector, x.score, lab[x.score], x.why]; }) } };
  };

  // ---------- khung ----------
  function tableHTML(tb) {
    return "<table><thead><tr>" + tb.cols.map(function (c) { return "<th>" + esc(c) + "</th>"; }).join("") + "</tr></thead><tbody>" +
      tb.rows.map(function (r) { return "<tr>" + r.map(function (v) { return typeof v === "number" ? "<td class='num'>" + v + "</td>" : "<td>" + esc(v) + "</td>"; }).join("") + "</tr>"; }).join("") +
      "</tbody></table>";
  }

  // API dùng chung cho module khác (vd sec.js — bản đồ mối đe doạ): cùng token, tooltip, bảng; vẽ lại khi đổi theme.
  window.FAViz = { T: T, meta: meta, base: base, axisStyle: axisStyle, legend: legend, merge: merge, esc: esc, trunc: trunc,
    tableHTML: tableHTML, pageURL: pageURL, hooks: [] };

  function renderAll() {
    if (!window.echarts) return; // không có ECharts → vẫn còn bảng dữ liệu
    var t = T(), M = meta();
    charts.forEach(function (c) { c.dispose(); });
    charts = [];
    document.querySelectorAll(".chart[data-viz]").forEach(function (el) {
      var fn = R[el.getAttribute("data-viz")], src = document.getElementById(el.getAttribute("data-src"));
      if (!fn || !src) return;
      var data = JSON.parse(src.textContent);
      var out = fn(el, data, t, M);
      var ch = echarts.init(el, null, { renderer: "canvas" });
      ch.setOption(out.opt);
      if (out.click) ch.on("click", out.click);
      charts.push(ch);
      var panel = el.closest(".panel");
      if (panel) {
        var lg = panel.querySelector(".legend[data-auto]");
        if (lg && out.legendHTML) lg.innerHTML = out.legendHTML;
        var tbl = panel.querySelector("details.tbl .body");
        if (tbl && out.table) tbl.innerHTML = tableHTML(out.table);
      }
    });
    window.FAViz.hooks.forEach(function (fn) { try { fn(t, M); } catch (e) { if (window.console) console.error(e); } });
  }

  function applyTheme(v) {
    if (v) document.documentElement.setAttribute("data-theme", v); else document.documentElement.removeAttribute("data-theme");
  }
  try { applyTheme(localStorage.getItem("fa-theme")); } catch (e) {}

  document.addEventListener("DOMContentLoaded", function () {
    renderAll();
    var btn = document.getElementById("theme-toggle");
    if (btn) btn.addEventListener("click", function () {
      var dark = getComputedStyle(document.documentElement).getPropertyValue("color-scheme").trim() === "dark";
      var next = dark ? "light" : "dark";
      applyTheme(next);
      try { localStorage.setItem("fa-theme", next); } catch (e) {}
      renderAll();
    });
    if (window.matchMedia) matchMedia("(prefers-color-scheme: dark)").addEventListener("change", renderAll);
    var to; window.addEventListener("resize", function () { clearTimeout(to); to = setTimeout(function () { charts.forEach(function (c) { c.resize(); }); }, 120); });
  });
})();
