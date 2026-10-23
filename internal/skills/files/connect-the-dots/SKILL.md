---
name: connect-the-dots
description: Tổng hợp xu hướng nhiều ngày/tuần từ feed-analyze — nối các storyline rời rạc thành mô hình lớn, phát hiện tín hiệu yếu, liên kết chéo AI ↔ kinh tế ↔ chính trị ↔ an ninh mạng, và viết bản review tuần/tháng. Dùng khi người dùng hỏi "tuần này/tháng này có xu hướng gì", "các tin này liên quan nhau thế nào", "bức tranh lớn", "review tuần", "điều gì đang âm thầm thay đổi". Trigger - "review tuần", "weekly", "xu hướng", "bức tranh lớn", "connect the dots", "tín hiệu yếu".
---

# Connect the Dots — tổng hợp xu hướng

## Thu thập

```bash
fa context --days 7          # bản tin gần nhất + storyline hoạt động 7 ngày
fa stories --days 7 --json   # toàn bộ storyline: slug, topic, level, days_active, summary
fa brief --date <ngày>       # bản tin của từng ngày nếu cần chi tiết
fa story <slug>              # timeline của storyline quan trọng
```

## Phân tích

1. **Xếp hạng storyline** theo: số ngày hoạt động, tầng (vĩ mô > trung mô > vi mô), mức leo thang (status `escalating`).
2. **Gom cụm** storyline thành 3–5 *chủ đề lớn* (theme) — mỗi theme có thể vắt qua nhiều topic.
3. **Vẽ mạng liên kết** giữa theme: dùng bảng hoặc sơ đồ mermaid
   ```mermaid
   graph LR
     A[Kiểm soát chip Mỹ] -->|chi phí compute| B[Giá AI agent]
     A -->|chuyển dịch FDI| C[Bán dẫn VN]
   ```
4. **Tín hiệu yếu** — storyline mới, ít bài nhưng tầng vĩ mô hoặc có cơ chế khuếch đại.
5. **Điều đã thay đổi trong nhận định** — so với đầu kỳ, giả định nào cần cập nhật?
6. **Áp dụng `micro-macro-lens`** cho theme quan trọng nhất.

## Đầu ra: Review tuần

- 5 xu hướng lớn (mỗi cái: bằng chứng = slug storyline, cơ chế, tầng, tác động VN)
- Bản đồ liên kết chéo (mermaid)
- 3 tín hiệu yếu cần để mắt
- Lịch sự kiện sắp tới có thể làm thay đổi bức tranh (họp Fed/NHNN, kỳ họp Quốc hội, công bố số liệu GSO, sự kiện công nghệ lớn)
- Câu hỏi mở cho tuần tới

Có thể lưu bản review vào `reports/weekly/<YYYY>-W<tuần>.md` trong thư mục home của `fa`.
