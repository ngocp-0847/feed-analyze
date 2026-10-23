---
name: threat-map
description: Bản đồ mối đe doạ an ninh mạng từ feed-analyze — xu hướng tấn công, nhóm tấn công & lãnh thổ (ai tấn công ai), ngành bị nhắm, cửa ngõ xâm nhập, kỹ thuật MITRE ATT&CK, CVE bị khai thác (CISA KEV, EPSS), leak site ransomware, và playbook phòng thủ cho dự án phần mềm. Dùng khi người dùng hỏi về tấn công mạng, hacker, ransomware, rò rỉ dữ liệu, lỗ hổng/CVE cần vá, nhóm APT, "dự án của tôi có nguy cơ gì", "cần phòng thủ gì", "threat model", "kỹ thuật tấn công X hoạt động thế nào". Trigger - "an ninh mạng", "security", "tấn công mạng", "ransomware", "CVE", "lỗ hổng", "APT", "threat intel", "phòng thủ", "bản đồ tấn công", "fa sec".
---

# Threat Map — bản đồ mối đe doạ an ninh mạng

## Dữ liệu (đọc — an toàn)

```bash
fa sec                         # tổng quan 30 ngày: mức đe doạ, xu hướng, họ tấn công, ngành, nước, nhóm, kỹ thuật, CVE
fa sec --days 7                # cửa sổ khác
fa sec --json                  # toàn bộ dữ liệu có cấu trúc (board + brief) để lọc/đếm chính xác
fa search "<nhóm|CVE|sản phẩm>" --topic security --days 30
fa show <id>                   # bài gốc (link để WebFetch khi cần chi tiết kỹ thuật)
fa open security               # mở bản đồ tương tác cho người dùng (lọc chéo: bấm nước/nhóm/kỹ thuật…)
```

`fa sec --json` → `board.incidents[]`: mỗi sự cố/chiến dịch đã gộp nhiều bài, trường ngắn:
`k` key · `t` tiêu đề · `d`/`l` ngày đầu/cuối · `kind` · `fam` họ tấn công · `types` · `sev` 1–5 · `actor`/`origin`/`mot`/`attr` ·
`vc` nước nạn nhân · `tc` nước bị nhắm · `sec` ngành · `vec` cửa ngõ xâm nhập · `tech` ATT&CK id · `cve` · `prod` sản phẩm ·
`det` diễn biến kỹ thuật · `def` biện pháp phòng thủ · `vn` liên hệ VN · `src` nguồn.
Ngoài ra `board.kev` (CVE mới trong CISA KEV, có EPSS), `board.ransom` (leak site theo ngày/nhóm/ngành), `board.tech` (tên + tactic),
`board.stack` (mức chạm tới từng công nghệ trong `security.stack` của config: số sự cố, CVE, KEV), mỗi sự cố có `stk` (chỉ số stack),
`brief` (xu hướng, lãnh thổ, kỹ thuật đáng lo, `controls` playbook P0–P2, `patch_now`, `stack_notes` nguy cơ + việc cần làm cho từng công nghệ).

## Lệnh tốn phí (hỏi trước)

`fa sec update` (KEV/EPSS/leak site + Claude trích xuất bài mới), `fa sec brief` (tổng hợp lại xu hướng + playbook, opus),
`fa sec backfill --days 30`, `fa sec extract --redo --days N`. `fa daily` đã tự chạy các bước này.

## Cách trả lời

1. **Tổng quan → chi tiết**: bắt đầu bằng mức đe doạ + 3 xu hướng lớn (có dẫn chứng sự cố cụ thể), rồi mới đi vào nhóm, kỹ thuật, CVE.
2. **Lãnh thổ**: ai (nhóm, nước gốc, động cơ: tài chính / gián điệp / hacktivism) → nhắm ai (nước, ngành) → bằng cách nào (cửa ngõ, ATT&CK).
   Quy kết quốc gia là theo tin — nêu rõ `confirmed` hay nghi vấn; nhiều sự cố chưa được quy kết.
3. **Kỹ thuật**: giải thích chuỗi tấn công từng bước theo kill chain (Trinh sát → Xâm nhập → Thực thi → Duy trì → Leo thang → Né tránh →
   Đánh cắp credential → Di chuyển ngang → Thu thập → C2 → Tuồn dữ liệu → Tác động), kèm id ATT&CK và dấu hiệu phát hiện.
4. **Với dự án của người dùng** (kỹ sư phần mềm VN: web/API, cloud, CI/CD, mã nguồn mở, AI agent): ánh xạ mối đe doạ → biện pháp cụ thể theo tầng
   (mã nguồn, thư viện/chuỗi cung ứng, CI/CD & bí mật, cloud, danh tính, dữ liệu & backup, giám sát, con người). Ưu tiên P0 (làm tuần này)
   theo rủi ro thực tế; nêu lệnh/cấu hình/công cụ cụ thể (vd: ghim phiên bản + `npm ci`, chặn lifecycle script, OIDC thay token dài hạn cho CI,
   passkey/FIDO2 cho admin, rà OAuth app bên thứ ba, giới hạn egress của runner, log & cảnh báo đăng nhập bất thường).
5. **CVE cần vá**: ưu tiên có trong KEV (đã bị khai thác thật) → EPSS cao → CVSS; chỉ khuyên vá khi sản phẩm có trong stack người dùng —
   stack nằm ở `security.stack` trong config (và `board.stack`); hỏi thêm nếu câu hỏi về dự án dùng công nghệ chưa có trong danh sách,
   và gợi ý người dùng bổ sung vào config.
6. Khi cần chi tiết kỹ thuật vượt dữ liệu: WebFetch bài gốc (`src[].u`) hoặc advisory của vendor/CISA; ghi nguồn.

## Nguyên tắc

- Không bịa IOC, CVE, tên nhóm hay số liệu; dữ kiện dẫn sự cố (`k`) hoặc link.
- Mục đích là phòng thủ: giải thích cơ chế ở mức đủ để phát hiện & ngăn chặn, không viết mã khai thác hay hướng dẫn tấn công từng bước.
- Dữ liệu VN còn mỏng — nói rõ khi suy luận từ sản phẩm/nhóm phổ biến ở khu vực.
