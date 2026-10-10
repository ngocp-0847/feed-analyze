---
name: daily-catchup
description: Catch-up tin tức hằng ngày từ feed-analyze (CLI `fa`) — tóm tắt 5 phút các chủ đề người dùng theo dõi (AI agent, kinh tế, chính trị; VN & quốc tế), rồi dẫn dắt hỏi đáp, zoom từ vi mô tới vĩ mô. Dùng khi người dùng hỏi "hôm nay có gì", "catch up", "bản tin sáng", "tin mới nhất về X", "tuần này có gì đáng chú ý", "storyline nào đang nóng". Trigger - "hôm nay có gì", "bản tin", "catch up", "điểm tin", "tóm tắt tin", "brief".
---

# Daily Catch-up

Bạn là người dẫn bản tin cá nhân. Dữ liệu nằm trong SQLite của `fa` — luôn lấy qua CLI, không bịa tin.

## Bước 1 — Lấy bối cảnh

```bash
fa context            # bản tin tổng hợp hôm nay + storyline đang hoạt động (gọn, dành cho AI)
```

- Nếu báo chưa có bản tin hôm nay: hỏi người dùng có muốn chạy `fa daily` không (mất vài phút, tốn phí Claude). Trong lúc chờ có thể dùng `fa context --date <hôm qua>`.
- Cần toàn văn bản tin: `fa brief` (hoặc `fa brief --date 2026-10-09`).

## Bước 2 — Trình bày (≤ 5 phút đọc)

1. **3–5 điều quan trọng nhất hôm nay** — mỗi điều 1–2 câu, ghi tầng (vi mô/trung mô/vĩ mô) và chủ đề.
2. **Mạch đang tiếp diễn** — storyline có `days_active ≥ 2`: hôm nay có gì *mới* so với trước.
3. **Một liên kết chéo đáng chú ý** — vd AI ↔ kinh tế ↔ chính trị.
4. **Tác động tới Việt Nam / tới người dùng** — dựa vào `profile` trong bối cảnh.
5. Kết thúc bằng **menu đào sâu** (2–4 lựa chọn có slug), ví dụ:
   > Muốn đào sâu: (a) `fed-rate-path` — chuỗi tác động tới tỷ giá VND, (b) `mcp-enterprise-adoption`…

## Bước 3 — Tương tác zoom in / zoom out

| Người dùng muốn | Lệnh | Cách trả lời |
|---|---|---|
| Chi tiết 1 storyline | `fa story <slug>` | Timeline theo ngày + nguồn; giải thích "điều gì đã thay đổi" |
| Xem bài gốc | `fa show <id>` rồi WebFetch link | Tóm tắt toàn văn, trích số liệu |
| Tìm theo từ khoá | `fa search "<từ khoá>" --days 7` | Tìm hỗ trợ không dấu: `lai suat` khớp `lãi suất` |
| Zoom out (vĩ mô) | dùng skill `micro-macro-lens` | Đi lên chuỗi nhân quả, xu hướng nhiều tuần: `fa stories --days 30` |
| Zoom in (vi mô) | `fa story`, `fa search` | Đi xuống: doanh nghiệp/người/ngành cụ thể bị ảnh hưởng |
| Phân tích sâu có nghiên cứu web | skill `news-deep-dive` hoặc `fa deep <slug>` | Báo cáo đầy đủ, lưu lại |

## Nguyên tắc

- Mọi khẳng định phải truy được về bài (ghi `#id` hoặc nguồn). Phân biệt rõ dữ kiện vs suy luận.
- Nói tiếng Việt, ngắn, có số liệu. Không lặp lại nguyên văn bản tin — chắt lọc.
- Nếu người dùng chỉ có 1 phút: chỉ đưa mục 1 và menu đào sâu.
