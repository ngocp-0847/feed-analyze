package security

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ngocp-0847/feed-analyze/internal/config"
	"github.com/ngocp-0847/feed-analyze/internal/store"
)

func getJSON(ctx context.Context, url string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("%s: HTTP %d %s", url, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// UpdateKEV tải danh mục CISA Known Exploited Vulnerabilities. Trả về (tổng, số CVE mới).
func UpdateKEV(ctx context.Context, cfg *config.Config, st *store.Store) (int, int, error) {
	var doc struct {
		Vulnerabilities []struct {
			CVE         string   `json:"cveID"`
			Vendor      string   `json:"vendorProject"`
			Product     string   `json:"product"`
			Name        string   `json:"vulnerabilityName"`
			DateAdded   string   `json:"dateAdded"`
			Description string   `json:"shortDescription"`
			Action      string   `json:"requiredAction"`
			DueDate     string   `json:"dueDate"`
			Ransomware  string   `json:"knownRansomwareCampaignUse"`
			CWEs        []string `json:"cwes"`
		} `json:"vulnerabilities"`
	}
	if err := getJSON(ctx, cfg.Security.KEVURL, &doc); err != nil {
		return 0, 0, err
	}
	items := make([]store.KEV, 0, len(doc.Vulnerabilities))
	for _, v := range doc.Vulnerabilities {
		items = append(items, store.KEV{CVE: v.CVE, Vendor: v.Vendor, Product: v.Product, Name: v.Name, DateAdded: v.DateAdded,
			Description: v.Description, Action: v.Action, DueDate: v.DueDate, Ransomware: v.Ransomware, CWEs: v.CWEs})
	}
	added, err := st.UpsertKEV(items)
	return len(items), added, err
}

// UpdateEPSS lấy điểm EPSS (FIRST.org) cho CVE trong danh sách, bỏ qua CVE vừa cập nhật < 3 ngày.
func UpdateEPSS(ctx context.Context, st *store.Store, cves []string) (int, error) {
	need := st.EPSSStale(cves, time.Now().Add(-72*time.Hour))
	n := 0
	for i := 0; i < len(need); i += 80 {
		chunk := need[i:min(i+80, len(need))]
		var resp struct {
			Data []struct {
				CVE        string `json:"cve"`
				EPSS       string `json:"epss"`
				Percentile string `json:"percentile"`
			} `json:"data"`
		}
		if err := getJSON(ctx, "https://api.first.org/data/v1/epss?limit=100&cve="+strings.Join(chunk, ","), &resp); err != nil {
			return n, err
		}
		scores := map[string][2]float64{}
		for _, c := range chunk {
			scores[c] = [2]float64{0, 0} // không có điểm → đánh dấu đã tra
		}
		for _, d := range resp.Data {
			e, _ := strconv.ParseFloat(d.EPSS, 64)
			p, _ := strconv.ParseFloat(d.Percentile, 64)
			scores[d.CVE] = [2]float64{e, p}
		}
		if err := st.SetEPSS(scores); err != nil {
			return n, err
		}
		n += len(resp.Data)
	}
	return n, nil
}

// UpdateRansom lấy bài đăng nạn nhân mới trên leak site ransomware (RansomLook) trong `days` ngày.
// Chỉ lưu nhóm, thời điểm, ngành — không lưu tên nạn nhân.
func UpdateRansom(ctx context.Context, cfg *config.Config, st *store.Store, days int) (int, int, error) {
	if cfg.Security.RansomURL == "" {
		return 0, 0, nil
	}
	var posts []struct {
		Title       string `json:"post_title"`
		Discovered  string `json:"discovered"`
		Description string `json:"description"`
		Group       string `json:"group_name"`
	}
	url := strings.ReplaceAll(cfg.Security.RansomURL, "{days}", strconv.Itoa(days))
	if err := getJSON(ctx, url, &posts); err != nil {
		return 0, 0, err
	}
	var out []store.RansomPost
	for _, p := range posts {
		t, err := time.Parse("2006-01-02 15:04:05.999999", p.Discovered)
		if err != nil {
			if t, err = time.Parse(time.RFC3339, p.Discovered); err != nil {
				continue
			}
		}
		h := sha1.Sum([]byte(strings.ToLower(p.Group + "|" + p.Title)))
		out = append(out, store.RansomPost{ID: hex.EncodeToString(h[:10]), Group: strings.ToLower(strings.TrimSpace(p.Group)),
			Discovered: t, Industry: RansomSector(p.Description)})
	}
	n, err := st.InsertRansomPosts(out)
	return len(out), n, err
}

// RansomSector ánh xạ mô tả ngành tự do (tiếng Anh) của leak site sang mã ngành; "unknown" nếu không có mô tả dùng được.
func RansomSector(desc string) string {
	d := strings.ToLower(desc)
	rules := []struct {
		sector string
		words  []string
	}{
		{"healthcare", []string{"health", "hospital", "medical", "clinic", "pharma", "dental", "surgery", "surgical", "patholog", "neurolog", "home care", "telehealth", "physician", "senior living"}},
		{"education", []string{"school", "education", "university", "college", "academ", "training"}},
		{"government", []string{"government", "municipal", "city of", "county", "public admin", "public sector", "ministry"}},
		{"crypto", []string{"crypto", "blockchain", "web3"}},
		{"finance", []string{"bank", "financ", "fintech", "payment", "insurance", "accounting", "credit", "investment", "tax", "liquidity", "trading", "wealth", "mortgage"}},
		{"legal", []string{"law firm", "legal", "consult", "professional service", "business service", "architect", "engineering", "audit", "investigation", "staffing", "recruit", "real estate"}},
		{"telecom", []string{"telecom", "broadband", "internet service provider", "isp)", "wireless", "communication"}},
		{"cloud", []string{"hosting", "data center", "datacenter", "cloud"}},
		{"tech", []string{"software", "technology", "it service", "it solutions", "information technology", "computer", "saas", "erp", "internet", "cyber", "electronics", "semiconductor"}},
		{"energy", []string{"energy", "oil", "gas", "utilit", "electric", "water", "power", "solar", "mining"}},
		{"transport", []string{"transport", "logistic", "shipping", "freight", "trucking", "aviation", "port ", "port terminal", "maritime", "airline", "rail"}},
		{"retail", []string{"retail", "wholesale", "store", "consumer", "e-commerce", "ecommerce", "restaurant", "apparel", "fashion", "dealership", "distribut"}},
		{"hospitality", []string{"hotel", "hospitality", "travel", "leisure", "tourism", "casino", "betting"}},
		{"media", []string{"media", "publish", "entertain", "marketing", "advertis", "broadcast", "printing"}},
		{"nonprofit", []string{"non-profit", "nonprofit", "charity", "church", "religio", "ministries", "foundation"}},
		{"manufacturing", []string{"manufactur", "industrial", "machinery", "chemical", "construction", "metal", "plastic", "agricult",
			"food", "beverage", "automotive", "packaging", "textile", "building", "producer", "factory", "aerospace"}},
	}
	for _, r := range rules {
		for _, w := range r.words {
			if strings.Contains(d, w) {
				return r.sector
			}
		}
	}
	if len(strings.Fields(d)) == 0 || strings.Contains(d, "posts") || strings.Contains(d, "announced") || strings.Contains(d, "no description") {
		return "unknown"
	}
	return "other"
}
