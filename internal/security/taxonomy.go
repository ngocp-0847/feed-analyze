// Package security dựng bản đồ mối đe doạ an ninh mạng: trích xuất sự cố/nhóm tấn công/lãnh thổ/kỹ thuật ATT&CK/CVE
// từ tin topic security (kèm nội dung bài gốc), kết hợp CISA KEV, EPSS và hoạt động leak site ransomware,
// rồi tổng hợp xu hướng + playbook phòng thủ cho dự án phần mềm.
package security

import (
	_ "embed"
	"encoding/json"
	"strings"
)

// Label: mã ổn định + nhãn tiếng Việt (thứ tự trong slice là thứ tự hiển thị / slot màu).
type Label struct {
	ID string `json:"id"`
	VI string `json:"vi"`
}

// Families: nhóm hình thức tấn công. Thứ tự cố định = slot màu categorical s1..s8 (màu theo thực thể, không theo hạng).
var Families = []Label{
	{"ransomware", "Ransomware & tống tiền"},
	{"breach", "Rò rỉ / đánh cắp dữ liệu"},
	{"exploit", "Khai thác lỗ hổng"},
	{"phishing", "Lừa đảo & phishing"},
	{"supply", "Chuỗi cung ứng"},
	{"espionage", "Gián điệp / APT"},
	{"disruption", "Phá hoại: DDoS, wiper"},
	{"malware", "Malware & chiếm tài khoản"},
}

// AttackTypes: loại tấn công chi tiết Claude được phép dùng → họ (family).
var AttackTypes = map[string]string{
	"ransomware": "ransomware", "extortion": "ransomware",
	"data_breach": "breach", "data_leak": "breach", "misconfiguration": "breach", "insider": "breach",
	"vuln_exploit": "exploit", "zero_day": "exploit", "ai_abuse": "exploit",
	"phishing": "phishing", "bec": "phishing", "scam_fraud": "phishing", "social_engineering": "phishing",
	"supply_chain": "supply", "malicious_package": "supply",
	"espionage": "espionage",
	"ddos":      "disruption", "wiper": "disruption", "hacktivism": "disruption", "defacement": "disruption",
	"malware": "malware", "infostealer": "malware", "botnet": "malware", "account_takeover": "malware",
	"credential_theft": "malware", "cryptojacking": "malware", "crypto_theft": "malware",
}

var AttackTypeVI = map[string]string{
	"ransomware": "ransomware", "extortion": "tống tiền dữ liệu", "data_breach": "rò rỉ dữ liệu", "data_leak": "lộ dữ liệu",
	"misconfiguration": "cấu hình sai", "insider": "nội gián", "vuln_exploit": "khai thác lỗ hổng", "zero_day": "zero-day",
	"ai_abuse": "lạm dụng AI / prompt injection", "phishing": "phishing", "bec": "lừa đảo email doanh nghiệp (BEC)",
	"scam_fraud": "lừa đảo trực tuyến", "social_engineering": "tấn công phi kỹ thuật", "supply_chain": "chuỗi cung ứng",
	"malicious_package": "gói thư viện độc", "espionage": "gián điệp", "ddos": "DDoS", "wiper": "wiper (xoá dữ liệu)",
	"hacktivism": "hacktivism", "defacement": "thay đổi giao diện web", "malware": "malware", "infostealer": "infostealer",
	"botnet": "botnet", "account_takeover": "chiếm tài khoản", "credential_theft": "đánh cắp thông tin đăng nhập",
	"cryptojacking": "đào coin trái phép", "crypto_theft": "trộm tài sản số",
}

var Sectors = []Label{
	{"government", "Chính phủ & công quyền"}, {"defense", "Quốc phòng"}, {"finance", "Tài chính – ngân hàng"},
	{"crypto", "Crypto / Web3"}, {"healthcare", "Y tế"}, {"education", "Giáo dục"}, {"telecom", "Viễn thông"},
	{"energy", "Năng lượng & tiện ích"}, {"manufacturing", "Sản xuất"}, {"retail", "Bán lẻ & TMĐT"},
	{"tech", "Công nghệ & SaaS"}, {"cloud", "Cloud & hosting"}, {"media", "Truyền thông"},
	{"transport", "Giao thông & logistics"}, {"legal", "Pháp lý & dịch vụ chuyên môn"}, {"hospitality", "Du lịch & khách sạn"},
	{"nonprofit", "Phi lợi nhuận"}, {"consumers", "Người dùng cá nhân"}, {"developers", "Lập trình viên & mã nguồn mở"},
	{"other", "Khác"},
}

// Vectors: cửa ngõ xâm nhập ban đầu — ánh xạ thẳng sang biện pháp phòng thủ.
var Vectors = []Label{
	{"phishing", "Email / tin nhắn lừa đảo"}, {"credential", "Mật khẩu bị lộ / dò mật khẩu"},
	{"exploit_public", "Lỗ hổng ứng dụng public (web/API)"}, {"edge_device", "Thiết bị biên: VPN, firewall, router"},
	{"supply_chain", "Nhà cung cấp / bên thứ ba"}, {"malicious_package", "Thư viện / extension độc"},
	{"oauth_token", "Token, session, OAuth bị đánh cắp"}, {"social_helpdesk", "Lừa helpdesk / gọi điện (vishing)"},
	{"misconfig", "Cấu hình cloud/DB lộ ra Internet"}, {"insider", "Người trong nội bộ"},
	{"drive_by", "Quảng cáo độc / SEO độc / watering hole"}, {"ai_prompt", "Prompt injection / AI agent"},
	{"physical", "Thiết bị vật lý / USB"},
}

var Motivations = []Label{
	{"financial", "Tài chính"}, {"espionage", "Gián điệp"}, {"hacktivism", "Hacktivism / chính trị"},
	{"destructive", "Phá hoại"}, {"unknown", "Chưa rõ"},
}

// Tactics: 14 tactic MITRE ATT&CK Enterprise theo thứ tự kill chain.
var Tactics = []Label{
	{"reconnaissance", "Trinh sát"}, {"resource-development", "Chuẩn bị hạ tầng"}, {"initial-access", "Xâm nhập ban đầu"},
	{"execution", "Thực thi"}, {"persistence", "Duy trì"}, {"privilege-escalation", "Leo thang đặc quyền"},
	{"defense-evasion", "Né tránh phòng thủ"}, {"credential-access", "Đánh cắp thông tin xác thực"},
	{"discovery", "Khám phá hệ thống"}, {"lateral-movement", "Di chuyển ngang"}, {"collection", "Thu thập"},
	{"command-and-control", "Điều khiển (C2)"}, {"exfiltration", "Tuồn dữ liệu ra"}, {"impact", "Gây tác động"},
}

var Kinds = []Label{
	{"incident", "Sự cố / vụ tấn công"}, {"campaign", "Chiến dịch / nhóm tấn công"}, {"vulnerability", "Lỗ hổng & bản vá"},
	{"advisory", "Cảnh báo / khuyến nghị"}, {"takedown", "Triệt phá / bắt giữ"}, {"research", "Nghiên cứu / kỹ thuật mới"},
	{"policy", "Chính sách & ngành"}, {"other", "Khác"},
}

// Layers: tầng phòng thủ cho playbook dự án.
var Layers = []Label{
	{"code", "Mã nguồn & ứng dụng"}, {"dependencies", "Thư viện & chuỗi cung ứng"}, {"cicd", "CI/CD & bí mật"},
	{"cloud", "Cloud & hạ tầng"}, {"identity", "Danh tính & truy cập"}, {"data", "Dữ liệu & sao lưu"},
	{"monitoring", "Giám sát & ứng cứu"}, {"people", "Con người & quy trình"},
}

// CWE phổ biến trong KEV → tên ngắn tiếng Việt (lớp lỗi mà lập trình viên cần phòng).
var CWEVI = map[string]string{
	"CWE-20": "Kiểm tra đầu vào không đúng", "CWE-22": "Path traversal", "CWE-23": "Path traversal tương đối",
	"CWE-73": "Kiểm soát tên/đường dẫn file từ ngoài", "CWE-74": "Injection (chung)", "CWE-84": "XSS qua URI mã hoá",
	"CWE-116": "Mã hoá/escape đầu ra sai", "CWE-276": "Quyền mặc định sai", "CWE-312": "Lưu dữ liệu nhạy cảm dạng rõ",
	"CWE-319": "Truyền dữ liệu nhạy cảm dạng rõ", "CWE-327": "Thuật toán mã hoá yếu", "CWE-330": "Ngẫu nhiên yếu",
	"CWE-427": "Đường dẫn tìm kiếm không kiểm soát (DLL hijack)", "CWE-470": "Reflection không an toàn",
	"CWE-521": "Mật khẩu yếu", "CWE-522": "Bảo vệ credential yếu", "CWE-532": "Lộ thông tin qua log",
	"CWE-613": "Hết hạn session sai", "CWE-770": "Cấp tài nguyên không giới hạn", "CWE-829": "Nạp chức năng từ nguồn không tin cậy",
	"CWE-940": "Không kiểm nguồn kênh giao tiếp", "CWE-1188": "Mặc định không an toàn", "CWE-77": "Command injection",
	"CWE-78": "OS command injection", "CWE-79": "XSS", "CWE-89": "SQL injection", "CWE-94": "Code injection",
	"CWE-95": "Eval injection", "CWE-119": "Tràn bộ nhớ đệm", "CWE-120": "Buffer overflow (copy)",
	"CWE-121": "Stack overflow", "CWE-122": "Heap overflow", "CWE-125": "Đọc ngoài vùng nhớ", "CWE-134": "Format string",
	"CWE-190": "Tràn số nguyên", "CWE-200": "Lộ thông tin nhạy cảm", "CWE-269": "Quản lý đặc quyền sai",
	"CWE-284": "Kiểm soát truy cập sai", "CWE-285": "Phân quyền sai", "CWE-287": "Xác thực sai",
	"CWE-288": "Vượt xác thực qua kênh khác", "CWE-290": "Vượt xác thực bằng giả mạo", "CWE-306": "Thiếu xác thực chức năng quan trọng",
	"CWE-307": "Không giới hạn đăng nhập sai", "CWE-345": "Không kiểm chứng tính xác thực dữ liệu", "CWE-347": "Kiểm chữ ký sai",
	"CWE-352": "CSRF", "CWE-362": "Race condition", "CWE-400": "Cạn kiệt tài nguyên", "CWE-416": "Use-after-free",
	"CWE-434": "Upload file nguy hiểm", "CWE-444": "HTTP request smuggling", "CWE-476": "Con trỏ NULL",
	"CWE-502": "Deserialization không an toàn", "CWE-552": "File truy cập được từ ngoài", "CWE-601": "Open redirect",
	"CWE-611": "XXE", "CWE-639": "IDOR (vượt phân quyền qua khoá)", "CWE-668": "Lộ tài nguyên sai phạm vi",
	"CWE-693": "Cơ chế bảo vệ thất bại", "CWE-732": "Phân quyền file/tài nguyên sai", "CWE-749": "Lộ phương thức nguy hiểm",
	"CWE-787": "Ghi ngoài vùng nhớ", "CWE-798": "Mật khẩu/khoá cứng trong mã", "CWE-822": "Con trỏ không tin cậy",
	"CWE-843": "Nhầm kiểu (type confusion)", "CWE-862": "Thiếu phân quyền", "CWE-863": "Phân quyền không đúng",
	"CWE-912": "Chức năng ẩn (backdoor)", "CWE-918": "SSRF", "CWE-1321": "Prototype pollution",
	"CWE-1336": "Template injection (SSTI)",
}

func ids(ls []Label) []string {
	out := make([]string, len(ls))
	for i, l := range ls {
		out[i] = l.ID
	}
	return out
}

// LabelVI: nhãn tiếng Việt của mã trong danh mục.
func LabelVI(ls []Label, id string) string { return labelOf(ls, id) }

func labelOf(ls []Label, id string) string {
	for _, l := range ls {
		if l.ID == id {
			return l.VI
		}
	}
	return id
}

// ---------- quốc gia ----------

//go:embed countries.json
var countriesJSON []byte

// Country: tên tiếng Việt, tên tiếng Anh, toạ độ nhãn (lon, lat) — Natural Earth 1:50m.
type Country struct {
	VI, EN   string
	Lon, Lat float64
}

var countries = func() map[string]Country {
	raw := map[string][4]any{}
	if err := json.Unmarshal(countriesJSON, &raw); err != nil {
		panic(err)
	}
	out := make(map[string]Country, len(raw))
	for k, v := range raw {
		c := Country{VI: v[0].(string), EN: v[1].(string)}
		c.Lon, _ = v[2].(float64)
		c.Lat, _ = v[3].(float64)
		out[k] = c
	}
	// tên thường dùng trên báo chí Việt Nam
	for k, vi := range map[string]string{"US": "Mỹ", "GB": "Anh", "KP": "Triều Tiên", "BR": "Brazil", "CZ": "Séc",
		"AE": "UAE", "CD": "CHDC Congo", "MK": "Bắc Macedonia", "VA": "Vatican"} {
		c := out[k]
		c.VI = vi
		out[k] = c
	}
	return out
}()

// CountryOf: chuẩn hoá mã ISO 3166-1 alpha-2 (cả "UK" → GB); ok=false nếu không nhận ra.
func CountryOf(code string) (string, Country, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "UK" {
		code = "GB"
	}
	c, ok := countries[code]
	return code, c, ok
}
