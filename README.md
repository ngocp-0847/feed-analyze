# feed-analyze (`fa`)

Bản tin cá nhân hằng ngày: quét RSS Việt Nam + quốc tế, dùng **Claude Code CLI** để phân loại theo chủ đề bạn quan tâm
(mặc định: 🤖 AI Agent · 📈 Kinh tế · 🏛️ Chính trị · 🛡️ An ninh mạng), gom tin thành **storyline** nối mạch qua nhiều ngày, dựng **chuỗi tác động
vi mô → trung mô → vĩ mô**, rồi tổng hợp liên chủ đề + tác động tới Việt Nam. Kèm bộ **skills** để trò chuyện, zoom in/out
và đào sâu ngay trong Claude Code.

Viết bằng Go, một binary, SQLite thuần Go (không cần CGO), không cần API key — dùng phiên đăng nhập `claude` sẵn có.

## Luồng xử lý

```
 RSS (50+ nguồn VN & quốc tế)
   │  fa fetch      quét song song, khử trùng lặp, lưu SQLite + FTS5 (tìm không dấu)
   ▼
 Bài viết ──► fa classify   Claude haiku, batch 30: topic + độ liên quan, mức quan trọng 1–5,
   │                          tầng micro/meso/macro, vn/intl, thực thể, tóm tắt tiếng Việt
   ▼
 fa analyze
   ├─ mỗi chủ đề (sonnet): gom thành 3–8 storyline, dùng lại slug cũ để nối mạch qua các ngày,
   │    chuỗi nhân quả có kênh truyền dẫn, góc Việt Nam, chỉ báo theo dõi, tín hiệu yếu
   └─ tổng hợp (opus): TL;DR, bức tranh vĩ mô, liên kết chéo AI ↔ kinh tế ↔ chính trị,
        tác động tới VN, "với bạn" (theo profile), watchlist, gợi ý đào sâu, góc nhìn ngược
   ▼
 reports/YYYY-MM-DD.md  ·  fa open (web + dashboard chart)  ·  fa publish (blog công khai)  ·  fa chat (Claude Code + skills)  ·  fa deep / fa ask
```

## Cài đặt

Yêu cầu: Go ≥ 1.26, [Claude Code CLI](https://docs.claude.com/en/docs/claude-code) đã đăng nhập (`claude` chạy được).

```bash
go install github.com/ngocp-0847/feed-analyze/cmd/fa@latest
# hoặc từ mã nguồn: git clone … && go install ./cmd/fa   ($GOPATH/bin cần nằm trong PATH để skills gọi được)
fa init                    # tạo ~/.feed-analyze/{config.yaml, feed.db, .claude/skills, CLAUDE.md}
fa doctor                  # kiểm tra môi trường
```

Thư mục dữ liệu: `--home DIR` > `$FA_HOME` > `~/.feed-analyze`.

## Dùng hằng ngày

```bash
fa daily                   # fetch → classify → analyze → bản tin (lần đầu ~10–15 phút, sau đó nhanh hơn)
fa open                    # mở bản tin + dashboard trên trình duyệt (tự bật server nền)
fa brief                   # đọc bản tin markdown trong terminal (fa brief --web = mở web)
fa daily --open            # chạy xong là mở bản tin trên web luôn
fa chat                    # mở Claude Code với skills: "hôm nay có gì?", "đào sâu fed-rate-path", "zoom out"
fa schedule install --time 06:30   # Windows Task Scheduler (Linux/macOS: in dòng crontab)
```

### Zoom vi mô ↔ vĩ mô

| Muốn | Lệnh |
|---|---|
| Danh sách câu chuyện đang diễn ra | `fa stories --days 7` |
| Timeline 1 câu chuyện qua các ngày | `fa story <slug>` |
| Tìm bài (không dấu vẫn khớp) | `fa search "lai suat" --topic economy --days 14` |
| Chi tiết 1 bài | `fa show 123` |
| Hỏi nhanh dựa trên kho tin | `fa ask "Fed giữ lãi suất ảnh hưởng tỷ giá VND thế nào?"` (`--web` để tra thêm) |
| Phân tích sâu có nghiên cứu web | `fa deep <slug>` · `fa deep 123` · `fa deep "AI agent tác động ngành outsource VN"` |
| Bối cảnh gọn cho AI / pipe | `fa context --days 7` |

`fa deep` dùng WebSearch + WebFetch để đọc bài gốc, xuất báo cáo 10 phần (kèm **dashboard chart**, xem bên dưới) (tóm tắt, timeline, chuỗi tác động, các bên,
liên kết chéo, tác động VN, kịch bản có xác suất, chỉ báo, điểm mù, nguồn) vào `reports/deep/`.

## 🛡️ An ninh mạng — bản đồ mối đe doạ

Hiểu **xu hướng tấn công** (ai, nhắm vào đâu, bằng cách nào, vì sao) và biết **phải phòng thủ gì** cho các dự án phần mềm của mình.

```
 18 nguồn security (THN, SecurityWeek, Dark Reading, The Record, Krebs, Unit 42, Talos, Google TI, Microsoft, Securelist, ESET, SANS…)
   │  fa fetch + classify (topic security)
   ▼
 tải nội dung bài gốc ──► Claude (sonnet) trích xuất tình báo có cấu trúc cho từng bài:
   │   loại sự kiện, incident_key (gộp nhiều bài cùng sự cố), họ tấn công, nhóm tấn công + quốc gia + động cơ + mức quy kết,
   │   nạn nhân (nước, ngành), nước bị nhắm, cửa ngõ xâm nhập, kỹ thuật MITRE ATT&CK (id + tactic), CVE (CVSS, đã bị khai thác?),
   │   sản phẩm bị ảnh hưởng, tác động, diễn biến kỹ thuật, biện pháp phòng thủ, liên hệ Việt Nam
   ├─ CISA KEV (CVE bị khai thác thực tế) · FIRST EPSS (xác suất bị khai thác 30 ngày) · RansomLook (nạn nhân mới trên leak site)
   ▼
 fa sec brief (opus): mức đe doạ, xu hướng có dẫn chứng, lãnh thổ, ngành, Việt Nam, kỹ thuật đáng lo (phát hiện / ngăn chặn),
                      playbook phòng thủ P0–P2 theo tầng, CVE nên vá ngay, nhóm nổi bật
   ▼
 fa open security — bản đồ tương tác (tổng quan → chi tiết, lọc chéo toàn trang)
```

| Lệnh | Việc |
|---|---|
| `fa sec backfill --open` | Lần đầu: quét nguồn security tới 30 ngày, trích xuất, tổng hợp, mở bản đồ |
| `fa sec` · `fa sec --days 7` · `fa sec --json` | Tóm tắt trong terminal / dữ liệu có cấu trúc |
| `fa sec update` | KEV + leak site + trích xuất bài mới + EPSS + tổng hợp (`fa daily` tự chạy) |
| `fa sec brief` · `fa sec extract --redo --days 7` | Tổng hợp lại / trích xuất lại |
| `fa open security` · `fa open security 90` | Mở bản đồ 30 / 90 ngày |

**Trang bản đồ mối đe doạ** (`/security`, có trong site công khai): mức đe doạ + xu hướng → KPI → **bản đồ thế giới** (choropleth nạn nhân /
nguồn tấn công + đường cong luồng tấn công nguồn → nạn nhân) → **Sankey** nguồn → hình thức → ngành → xu hướng theo ngày & leak site
ransomware → ma trận **ngành × hình thức** → cửa ngõ xâm nhập → hồ sơ **nhóm tấn công** → **ma trận MITRE ATT&CK** 14 tactic →
CVE trong tin (KEV, EPSS, CVSS) · vendor & lớp lỗi **CWE** trong KEV → **playbook phòng thủ** (ma trận ưu tiên tác động × công sức,
checklist theo tầng lưu trên trình duyệt) → **chi tiết từng sự cố** (đặc tả kỹ thuật, ATT&CK, CVE, cách phòng, nguồn).
Bấm bất kỳ quốc gia / nhóm / ngành / kỹ thuật / CVE nào để **lọc toàn trang**; bộ lọc nằm trên URL (vd `/security?country=VN`,
`?origin=CN`, `?tech=T1195.002`) để chia sẻ. Leak site chỉ hiển thị số liệu tổng hợp theo nhóm/ngành, không hiển thị tên nạn nhân.

Cấu hình: mục `security:` trong `config.yaml` (model trích xuất/tổng hợp, batch, tải bài gốc, nguồn KEV/leak site, EPSS).
`security.stack` liệt kê công nghệ trong các dự án của bạn (`{name, match: [từ khoá]}`, vd PHP/Laravel, Node/npm, Python/PyPI, AWS,
GitHub/CI…): trang bản đồ có mục **"Stack của bạn bị chạm tới đâu"** (mức phơi nhiễm, sự cố & CVE liên quan, việc cần làm cho từng công nghệ,
bấm để lọc toàn trang) và playbook / danh sách CVE nên vá được ưu tiên theo stack này.
Trang `/security` chỉ có ở bản cục bộ; site công khai (`fa publish`) không đưa lên trừ khi đặt `publish.include_security: true`
(vì mục stack + playbook mô tả công nghệ trong dự án của bạn).
Chat: skill `threat-map` trong `fa chat` ("dự án của tôi cần phòng thủ gì?", "nhóm Lazarus đang làm gì?", "giải thích T1195.002").

## Web & dashboard

`fa open [đích]` bảo đảm web server đang chạy (nếu chưa: bật tiến trình nền, log ở `<home>/logs/serve.log`) rồi mở trình duyệt.
Tắt server: `fa serve --stop`. Chạy foreground: `fa serve`. Địa chỉ: `web.addr` trong config (mặc định `127.0.0.1:8787`).

| Lệnh | Trang |
|---|---|
| `fa open` · `fa open 2026-10-10` | Bản tin ngày: **dashboard ngày** + toàn văn |
| `fa open dashboard` | **Dashboard xu hướng** 7/14/30/90 ngày |
| `fa open <slug>` | Timeline 1 storyline, chuỗi tác động dạng sơ đồ |
| `fa open deep <slug>` · `fa deep … --open` | **Dashboard phân tích sâu** + báo cáo |
| `fa open security` | **Bản đồ mối đe doạ an ninh mạng** |
| `fa open "<từ khoá>"` | Tìm bài |

**Dashboard ngày** — KPI · *bản đồ storyline* (tầng vi/trung/vĩ mô × mức quan trọng, kích thước = số bài, màu = chủ đề) ·
*mạng liên kết chéo* giữa các chủ đề (rê chuột xem cơ chế) · *ma trận tác động tới VN* (kỳ hạn × chiều) · mỗi storyline có sơ đồ chuỗi tác động.

**Dashboard xu hướng** — khối lượng tin theo ngày × chủ đề · tỷ lệ tầng tác động · phạm vi địa lý (VN / quốc tế ↔ VN / quốc tế) ·
*dòng thời gian storyline* (heatmap câu chuyện × ngày) · thực thể được nhắc nhiều nhất · chi phí Claude.

**Dashboard phân tích sâu** — sau mỗi `fa deep`, Claude rút báo cáo thành dữ liệu có cấu trúc (JSON schema): chỉ số chính ·
*đồ thị nhân quả phân tầng* vi mô → trung mô → vĩ mô có kênh truyền dẫn · *kịch bản* & xác suất · *tác động tới VN* (−2…+2) ·
*bản đồ các bên liên quan* (được/mất × ảnh hưởng) · dòng thời gian (dữ kiện / tuyên bố / dự báo) · *chỉ báo cần theo dõi* có trạng thái.
Báo cáo cũ chưa có dashboard: `fa viz <slug>` hoặc `fa viz --all`.

Mọi chart có tooltip, legend và **bảng dữ liệu** đi kèm; light/dark (nút ◐ hoặc `?theme=dark`); [Apache ECharts](https://echarts.apache.org) 5.5.1 (Apache-2.0) được nhúng sẵn nên chạy offline.
Bảng màu đã kiểm bằng validator (CVD-safe, đủ tương phản): màu cố định theo chủ đề, ramp một màu cho tầng vi→vĩ mô, cặp phân kỳ xanh/đỏ cho tích cực/tiêu cực.

## Xuất bản công khai (blog) — Cloudflare, miễn phí

`fa publish` dựng **site tĩnh** từ kho dữ liệu local (bản tin, dashboard, storyline, phân tích sâu kèm chart) rồi đẩy lên Cloudflare
bằng `wrangler`. Ví dụ đang chạy: **https://blog.bomclaw.org**.

**Chọn dịch vụ — đều 0 đ:**

| Target | Chi phí | Gắn domain | Ghi chú |
|---|---|---|---|
| `workers` (mặc định) — Workers Static Assets | Free: request vào file tĩnh miễn phí, không giới hạn | **Tự động** (wrangler tạo DNS + SSL) | Cloudflare khuyến nghị cho project mới |
| `pages` — Cloudflare Pages | Free: băng thông không giới hạn, 500 deploy/tháng | Cần thêm 1 bản ghi CNAME thủ công (token wrangler không có quyền DNS) | `fa publish setup` tạo project + gắn domain |

Pipeline: `fa daily` (local, có Claude) → `fa publish build` (site tĩnh trong `<home>/site`) → `wrangler deploy` → `https://<domain>`.
Với `publish.after_daily: true`, `fa daily` (kể cả chạy theo `fa schedule`) tự làm cả chuỗi mỗi sáng.

| Lệnh | Việc |
|---|---|
| `npx wrangler login` | Đăng nhập Cloudflare (1 lần, mở trình duyệt) |
| `fa publish` | Build + deploy (lần đầu tự tạo Worker, custom domain, DNS, SSL) |
| `fa publish preview` | Build rồi xem thử đúng bản sẽ deploy tại `http://127.0.0.1:8788` |
| `fa publish build` · `fa publish --no-deploy` | Chỉ build |
| `fa publish status` | Kiểm tra site (HTTP), với `pages`: trạng thái domain/SSL/DNS |
| `fa publish setup` | Chỉ cần với `target: pages` |
| `fa daily --publish` | Xuất bản sau khi chạy bản tin (hoặc `publish.after_daily: true`) |

```yaml
publish:
  target: workers             # workers | pages
  project: fa-blog            # tên Worker / Pages project
  domain: blog.example.org    # zone phải nằm trên cùng tài khoản Cloudflare
  days: 90                    # chỉ xuất bản N ngày gần nhất (0 = tất cả)
  exclude: [q-cau-hoi-rieng]  # slug báo cáo deep / storyline không công khai
  after_daily: true
```

Bản công khai **không** chứa: profile của bạn, mục "Với bạn", chi phí Claude, trang tìm bài, gợi ý lệnh CLI.
Có thêm: trang chủ dạng thẻ, RSS/Atom `feed.xml`, `sitemap.xml`, `robots.txt`, `404.html`, `_headers` (cache + bảo mật),
canonical/OG meta, footer ghi rõ nội dung do AI tổng hợp. `wrangler.jsonc` được sinh lại mỗi lần deploy trong thư mục home.

## Skills cho Claude Code

Cài vào `<home>/.claude/skills` khi `fa init` / `fa chat`; cài toàn cục cho mọi phiên: `fa skills install --global`.

| Skill | Dùng khi |
|---|---|
| `daily-catchup` | "hôm nay có gì", bản tin 5 phút + menu đào sâu |
| `news-deep-dive` | đào sâu 1 storyline / câu hỏi, có quy trình đọc nguồn gốc & nghiên cứu |
| `micro-macro-lens` | khung 3 tầng, 9 kênh truyền dẫn, checklist bối cảnh VN, bản đồ hệ sinh thái AI agent, bẫy tư duy |
| `connect-the-dots` | xu hướng nhiều ngày, review tuần, sơ đồ liên kết (mermaid) |
| `threat-map` | an ninh mạng: xu hướng tấn công, lãnh thổ, ATT&CK, CVE cần vá, playbook phòng thủ dự án |
| `feed-curator` | thêm/sửa chủ đề, nguồn RSS, chỉnh ngưỡng lọc & model |

Khung `micro-macro-lens` cũng được nhúng thẳng vào prompt phân tích tự động, nên bản tin và phiên chat dùng chung một cách tư duy.

## Tuỳ biến (`config.yaml`)

- `profile` — mô tả bạn là ai; Claude dùng để viết mục "Với bạn" và chọn góc "vì sao quan trọng".
- `topics` — thêm chủ đề mới (vd `id: semiconductor`, `description` càng cụ thể càng phân loại tốt). Sau khi đổi: `fa classify --redo --days 2`.
- `feeds` — `{name, url, region: vn|intl, lang, topics: [gợi ý]}`; kiểm tra: `fa feeds check`, `fa feeds --url <url>`.
- `claude` — model cho từng bước (`haiku` / `sonnet` / `opus`), batch size, độ song song, timeout.
- `analyze.window_hours`, `min_relevance`, `storyline_memory_days`.
- `report.copy_to` — chép bản tin `.md` sang thư mục khác (vd vault Obsidian).

## Chi phí & hiệu năng

- Phân loại chạy với `--system-prompt` tối giản + `--setting-sources ""` (không nạp hook/MCP/CLAUDE.md) → ~1k token overhead mỗi lần gọi thay vì ~60k.
- `fa stats` hiển thị số lần gọi, lỗi, chi phí ước tính 24h/30 ngày (theo `total_cost_usd` mà CLI báo).
- Mỗi lần gọi ép JSON schema (`--json-schema`), tự thử lại 1 lần; bài bị model bỏ sót được phân loại lại tối đa 2 vòng.

## Cấu trúc mã

```
cmd/fa/                 CLI (dispatcher, các lệnh)
internal/config         config.yaml + mặc định nhúng (default.yaml)
internal/store          SQLite: articles, classifications, FTS5, storylines, reports, llm_calls
internal/feed           quét RSS/Atom song song (gofeed)
internal/llm            wrapper `claude -p` (json / stream-json / interactive)
internal/classify       phân loại batch
internal/analyze        storyline theo chủ đề + tổng hợp liên chủ đề
internal/report         render markdown, ghi file
internal/research       fa context / deep / ask
internal/security       fa sec: trích xuất tình báo, KEV/EPSS/leak site, gộp sự cố, tổng hợp + playbook (countries.json: Natural Earth)
internal/web            fa serve / fa open: trang, dashboard, static (ECharts nhúng, viz.js, sec.js, world.json, app.css)
internal/skills         skills nhúng + CLAUDE.md cho phiên chat
```

Test: `go test ./...`
