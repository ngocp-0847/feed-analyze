# feed-analyze (`fa`)

Bản tin cá nhân hằng ngày: quét RSS Việt Nam + quốc tế, dùng **Claude Code CLI** để phân loại theo chủ đề bạn quan tâm
(mặc định: 🤖 AI Agent · 📈 Kinh tế · 🏛️ Chính trị), gom tin thành **storyline** nối mạch qua nhiều ngày, dựng **chuỗi tác động
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
 reports/YYYY-MM-DD.md  ·  fa serve (web)  ·  fa chat (Claude Code + skills)  ·  fa deep / fa ask
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
fa brief                   # đọc bản tin markdown trong terminal
fa serve --open            # đọc trên trình duyệt: bản tin, storyline, tìm bài, phân tích sâu
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

`fa deep` dùng WebSearch + WebFetch để đọc bài gốc, xuất báo cáo 10 phần (tóm tắt, timeline, chuỗi tác động, các bên,
liên kết chéo, tác động VN, kịch bản có xác suất, chỉ báo, điểm mù, nguồn) vào `reports/deep/`.

## Skills cho Claude Code

Cài vào `<home>/.claude/skills` khi `fa init` / `fa chat`; cài toàn cục cho mọi phiên: `fa skills install --global`.

| Skill | Dùng khi |
|---|---|
| `daily-catchup` | "hôm nay có gì", bản tin 5 phút + menu đào sâu |
| `news-deep-dive` | đào sâu 1 storyline / câu hỏi, có quy trình đọc nguồn gốc & nghiên cứu |
| `micro-macro-lens` | khung 3 tầng, 9 kênh truyền dẫn, checklist bối cảnh VN, bản đồ hệ sinh thái AI agent, bẫy tư duy |
| `connect-the-dots` | xu hướng nhiều ngày, review tuần, sơ đồ liên kết (mermaid) |
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
internal/web            fa serve
internal/skills         skills nhúng + CLAUDE.md cho phiên chat
```

Test: `go test ./...`
