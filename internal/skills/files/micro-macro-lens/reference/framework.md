# Khung phân tích Vi mô ↔ Vĩ mô

## 1. Ba tầng

| Tầng | Đơn vị phân tích | Ví dụ |
|---|---|---|
| Vi mô (micro) | Doanh nghiệp, sản phẩm, cá nhân, giao dịch, dự án, địa phương | Một startup ra mắt coding agent; một ngân hàng tăng lãi suất huy động; một bộ trưởng bị miễn nhiệm |
| Trung mô (meso) | Ngành, chuỗi cung ứng, thị trường tài sản, lĩnh vực chính sách, khu vực | Ngành phần mềm outsource; thị trường trái phiếu DN; chính sách bán dẫn; ASEAN |
| Vĩ mô (macro) | Nền kinh tế quốc gia, hệ thống tài chính toàn cầu, trật tự địa chính trị, xu hướng công nghệ nền tảng | Tăng trưởng GDP VN; chu kỳ lãi suất Fed; cạnh tranh Mỹ–Trung; làn sóng tự động hoá bằng AI |

Một sự kiện có thể *xuất phát* ở tầng này nhưng *ý nghĩa* nằm ở tầng khác. Luôn hỏi: "Nếu sự kiện này lặp lại 100 lần thì điều gì thay đổi?" (đi lên) và "Thay đổi này chạm tới ai cụ thể?" (đi xuống).

## 2. Chín kênh truyền dẫn

1. **Giá & chi phí** — giá hàng hoá, năng lượng, compute/token, chi phí đầu vào, biên lợi nhuận.
2. **Tín dụng & lãi suất** — lãi suất điều hành, chi phí vốn, tăng trưởng tín dụng, định giá tài sản.
3. **Tỷ giá & dòng vốn** — USD index, USD/VND, dòng vốn ngoại (FII, FDI), kiều hối, dự trữ ngoại hối.
4. **Thương mại & chuỗi cung ứng** — thuế quan, kiểm soát xuất khẩu, quy tắc xuất xứ, dịch chuyển nhà máy ("China+1").
5. **Chính sách & pháp lý** — luật, nghị định, quy định ngành, thuế, chính sách công nghiệp, thực thi.
6. **Công nghệ & năng suất** — thay thế/bổ trợ lao động, chi phí cận biên giảm, mô hình kinh doanh mới, hiệu ứng mạng.
7. **Lao động & kỹ năng** — việc làm, lương, cầu kỹ năng, di cư lao động, đào tạo.
8. **Kỳ vọng & niềm tin** — tâm lý thị trường, định hướng chính sách (forward guidance), uy tín thể chế, câu chuyện (narrative).
9. **Quyền lực & liên minh** — đàm phán, trừng phạt, liên minh an ninh, ảnh hưởng mềm, nhân sự cấp cao.

## 3. Checklist bối cảnh Việt Nam

**Thể chế & chính sách:** Bộ Chính trị / Ban Chấp hành TW (định hướng lớn, nghị quyết), Quốc hội (luật, ngân sách, kỳ họp tháng 5 & tháng 10–11), Chính phủ & Thủ tướng (nghị định, chỉ thị), NHNN (lãi suất điều hành, tỷ giá trung tâm, room tín dụng), Bộ Tài chính (thuế, trái phiếu, UBCKNN), Bộ KH&CN (chuyển đổi số, AI, bán dẫn).

**Đặc điểm nền kinh tế:**
- Độ mở thương mại rất cao (xuất nhập khẩu > 150% GDP) → nhạy với cầu Mỹ/EU/Trung Quốc và thuế quan.
- FDI chiếm phần lớn xuất khẩu (điện tử: Samsung, Apple supply chain, Intel; bán dẫn đang lên).
- Thị trường vốn: VN-Index, câu chuyện nâng hạng thị trường (FTSE/MSCI), khối ngoại mua/bán ròng.
- Tỷ giá USD/VND chịu áp lực khi Fed giữ lãi suất cao / DXY mạnh → NHNN cân bằng giữa tăng trưởng và ổn định tỷ giá.
- Bất động sản & trái phiếu doanh nghiệp là điểm nhạy cảm tín dụng.
- Ngành IT outsource lớn (FPT, các công ty phần mềm) → AI coding agent vừa là cơ hội (năng suất) vừa là rủi ro (giảm cầu nhân lực giá rẻ).

**Đối ngoại:** "ngoại giao cây tre", quan hệ Đối tác chiến lược toàn diện với Mỹ, Trung Quốc, Nhật, Hàn, Úc…; Biển Đông; ASEAN; vị thế hưởng lợi từ dịch chuyển chuỗi cung ứng nhưng rủi ro bị xem là kênh trung chuyển hàng Trung Quốc (trans-shipment) trong tranh chấp thuế quan.

**Nguồn dữ liệu tham chiếu:** Cục Thống kê (GSO) — GDP/CPI/IIP hằng tháng/quý; NHNN — tỷ giá trung tâm, lãi suất; Bộ Công Thương / Hải quan — xuất nhập khẩu; Bộ KH&ĐT — FDI; HOSE/HNX — thị trường.

## 4. Bản đồ hệ sinh thái AI Agent (để định vị tin AI)

- **Tầng mô hình:** frontier labs (Anthropic, OpenAI, Google DeepMind, Meta, xAI), mã nguồn mở (Llama, Qwen, DeepSeek, Mistral) — năng lực, giá token, giấy phép.
- **Tầng hạ tầng:** chip (Nvidia, AMD, TSMC, ASML; custom silicon), datacenter, điện năng, cloud (AWS, Azure, GCP), kiểm soát xuất khẩu.
- **Tầng công cụ agent:** coding agents, giao thức (MCP, A2A), SDK/framework, eval, observability, bảo mật agent.
- **Tầng ứng dụng:** doanh nghiệp áp dụng, vertical agents (luật, tài chính, y tế, CSKH), tác động năng suất & việc làm.
- **Tầng quản trị:** luật AI (EU AI Act, Mỹ, Trung Quốc, Luật/khung về AI của VN), bản quyền dữ liệu, an toàn AI.
- **Dòng tiền:** vòng gọi vốn, định giá, capex của big tech, doanh thu thực tế vs kỳ vọng (rủi ro bong bóng).

Câu hỏi then chốt cho tin AI: thay đổi này làm *chi phí* hay *năng lực* dịch chuyển? Ai nắm điểm nghẽn (bottleneck)? Ai bị commoditize?

## 5. Bẫy tư duy cần tránh

- **Nhầm tuyên bố với sự kiện** — "sẽ đầu tư 10 tỷ USD" ≠ đã giải ngân.
- **Thiên kiến tin gần (recency)** — một ngày giảm điểm không phải xu hướng.
- **Một nhân quả duy nhất** — thường có nhiều lực cùng tác động; nêu lực chính và lực đối trọng.
- **Bỏ qua độ trễ** — chính sách tiền tệ trễ 6–18 tháng; luật cần nghị định hướng dẫn.
- **Bỏ qua động cơ người phát ngôn** — doanh nghiệp, chính phủ, nhà phân tích đều có lợi ích.
- **Bản đồ không phải lãnh thổ** — nêu rõ khi chỉ dựa vào tiêu đề/tóm tắt RSS chứ chưa đọc toàn văn.

## 6. Mẫu kịch bản

| Kịch bản | Điều kiện | Xác suất chủ quan | Tín hiệu nhận biết sớm |
|---|---|---|---|
| Cơ sở | … | ~50–60% | … |
| Tích cực | … | ~20% | … |
| Tiêu cực | … | ~20% | … |
