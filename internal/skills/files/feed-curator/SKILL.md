---
name: feed-curator
description: Quản lý chủ đề và nguồn RSS của feed-analyze — thêm/bớt/sửa topic quan tâm, thêm nguồn tin VN hoặc quốc tế, kiểm tra nguồn chết, tinh chỉnh profile & ngưỡng lọc để bản tin sát nhu cầu hơn. Dùng khi người dùng nói "theo dõi thêm chủ đề X", "thêm nguồn Y", "bản tin nhiều rác quá", "thiếu tin về Z", "nguồn nào đang lỗi", "đổi model". Trigger - "thêm topic", "thêm chủ đề", "thêm nguồn", "thêm RSS", "feed lỗi", "tinh chỉnh bản tin".
---

# Feed Curator

Cấu hình nằm ở `config.yaml` trong thư mục home của `fa` (mặc định `~/.feed-analyze/config.yaml`, hoặc `$FA_HOME`). Xem đường dẫn bằng `fa stats`.

## Thêm / sửa chủ đề

Mỗi topic gồm `id` (kebab-case, không dấu, không đổi sau khi đã có dữ liệu), `name`, `emoji`, `description` (quan trọng nhất — Claude phân loại dựa vào đây), `keywords` (gợi ý).

- Viết `description` theo phạm vi cụ thể: liệt kê tiểu chủ đề, tổ chức, khu vực. Ghi rõ cái *không* thuộc topic nếu dễ nhầm.
- Sau khi sửa: `fa topics` để xác nhận; bài mới sẽ được phân loại theo topic mới. Muốn phân loại lại bài cũ: `fa classify --redo --days 2`.

## Thêm nguồn RSS

```yaml
- {name: Tên duy nhất, url: "https://...", region: vn|intl, lang: vi|en, topics: [economy]}
```

1. Tìm URL RSS (thường `/rss`, `/feed`, `/rss.xml`; báo VN: `https://<báo>/rss/<chuyên-mục>.rss`).
2. Thử nhanh: `fa feeds check --url "<url>"` — báo số item và item mới nhất.
3. Thêm vào `config.yaml`, rồi `fa feeds check` để kiểm tra toàn bộ.
4. Nguồn lỗi kéo dài: đặt `disabled: true` thay vì xoá (giữ lịch sử).

## Chẩn đoán chất lượng

- `fa stats` — số bài theo topic 24h, chi phí Claude, lỗi.
- `fa feeds` — trạng thái từng nguồn (lần quét cuối, số bài mới).
- Nhiều rác: tăng `analyze.min_relevance` (0.5 → 0.6) hoặc thu hẹp `description`.
- Thiếu tin: thêm nguồn chuyên ngành, mở rộng `description`, giảm `min_relevance`.
- Đổi model: `claude.classify_model` (haiku), `analyze_model` (sonnet), `synthesis_model`/`deep_model` (opus).

## Gợi ý nguồn theo mảng

- **VN chính sách:** VnExpress Thời sự, Tuổi Trẻ, Thanh Niên, VietnamNet Chính trị, Báo Chính phủ.
- **VN kinh tế/thị trường:** CafeF, VnEconomy, VnExpress Kinh doanh, Dân trí Kinh doanh.
- **Quốc tế:** BBC, NYT, Guardian, CNBC, FT, WSJ, The Economist, Nikkei Asia, SCMP, The Diplomat.
- **AI:** TechCrunch AI, The Verge AI, MIT Tech Review, Simon Willison, Latent Space, Hacker News (hnrss.org query), blog OpenAI/DeepMind/Hugging Face.
