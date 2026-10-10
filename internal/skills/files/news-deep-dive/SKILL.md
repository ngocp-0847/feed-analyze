---
name: news-deep-dive
description: Phân tích sâu một storyline, bài báo hoặc câu hỏi thời sự từ feed-analyze — đọc bài gốc, nghiên cứu bổ sung trên web, dựng chuỗi nhân quả vi mô → vĩ mô, các bên liên quan, tác động tới Việt Nam, kịch bản và chỉ báo theo dõi. Dùng khi người dùng muốn "đào sâu", "phân tích kỹ", "giải thích vì sao", "điều này dẫn tới đâu", hoặc chọn một mục trong menu đào sâu của bản tin. Trigger - "đào sâu", "deep dive", "phân tích kỹ", "giải thích storyline", "kịch bản", "fa deep".
---

# News Deep Dive

## Hai cách chạy

- **Nhanh, không tương tác:** `fa deep <slug | id | "câu hỏi"> --open` — Claude tự nghiên cứu (WebSearch/WebFetch), lưu báo cáo vào `reports/deep/`, dựng dashboard chart (chỉ số, đồ thị nhân quả phân tầng, kịch bản, các bên liên quan, tác động VN, chỉ báo) và mở trên trình duyệt. Báo cáo cũ chưa có chart: `fa viz <slug> --open`.
- **Tương tác (mặc định trong phiên chat):** làm theo quy trình dưới, dừng hỏi người dùng ở các điểm rẽ.

## Quy trình tương tác

1. **Thu thập nội bộ**
   ```bash
   fa story <slug>                  # timeline + danh sách bài (có id, link)
   fa search "<thực thể/từ khoá>" --days 30
   fa show <id>                     # chi tiết 1 bài
   ```
2. **Đọc nguồn gốc** — WebFetch 2–4 bài quan trọng nhất (ưu tiên nguồn gốc: thông cáo, văn bản pháp luật, báo cáo số liệu). Ghi lại số liệu then chốt.
3. **Nghiên cứu bổ sung** — WebSearch cho: bối cảnh lịch sử, số liệu vĩ mô mới nhất, phản ứng của các bên, góc nhìn trái chiều.
4. **Phân tích** — áp dụng skill `micro-macro-lens` (đọc `reference/framework.md` của skill đó).
5. **Báo cáo** theo cấu trúc:
   1. Tóm tắt 30 giây
   2. Diễn biến & bối cảnh (timeline có ngày)
   3. Chuỗi tác động vi mô → trung mô → vĩ mô (ghi kênh truyền dẫn)
   4. Các bên liên quan & động cơ
   5. Liên kết chéo sang chủ đề khác
   6. Tác động tới Việt Nam & tới người dùng (theo profile)
   7. Kịch bản (cơ sở / tích cực / tiêu cực, xác suất chủ quan, tín hiệu sớm)
   8. Chỉ báo cần theo dõi (cụ thể, có ngưỡng/mốc thời gian)
   9. Điểm mù & phản biện
   10. Nguồn (link)
6. **Đề xuất bước tiếp** — 2–3 câu hỏi đào sâu hơn hoặc storyline liên quan.

## Lưu kết quả

Nếu người dùng muốn giữ lại: ghi file markdown vào `reports/deep/<YYYY-MM-DD>-<slug>.md` trong thư mục `fa` home (xem `fa stats` để biết đường dẫn), hoặc chạy lại bằng `fa deep` để lưu chính thức.

## Nguyên tắc

- Dữ kiện có nguồn; suy luận ghi rõ là suy luận; mức tin cậy cho mỗi kết luận lớn.
- Không dừng ở "có thể ảnh hưởng" — nói rõ ai, qua kênh nào, chiều nào, bao lâu.
