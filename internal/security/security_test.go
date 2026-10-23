package security

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ngocp-0847/feed-analyze/internal/config"
	"github.com/ngocp-0847/feed-analyze/internal/store"
)

func TestNormalize(t *testing.T) {
	e := Event{Kind: "weird", IncidentKey: "Lazarus NPM Campaign!", Severity: 9,
		AttackTypes: []string{"supply_chain", "bogus", "supply_chain", "espionage"},
		Actor:       Actor{Name: "unknown", Origin: "kp", Motivation: "fun"},
		Victims:     []Victim{{Name: "Acme", Country: "uk", Sector: "tech"}, {Name: "X", Country: "ZZ", Sector: "nope"}},
		Targets:     []string{"vn", "VN", "EU"},
		Sectors:     []string{"finance", "made-up"},
		Vectors:     []string{"malicious_package", "teleport"},
		Techniques:  []Technique{{ID: "t1195.002", Tactic: "initial-access"}, {ID: "T9", Tactic: "impact"}, {ID: "T1566", Tactic: "nope"}},
		CVEs:        []CVERef{{ID: "cve-2026-12345", CVSS: 12}, {ID: "CVE-26-1"}, {ID: "CVE-2026-12345"}},
	}
	e.normalize()
	if e.Kind != "other" || e.IncidentKey != "lazarus-npm-campaign" || e.Severity != 5 {
		t.Fatalf("kind/key/sev: %+v", e)
	}
	if len(e.AttackTypes) != 2 || e.Actor.Name != "" || e.Actor.Origin != "KP" || e.Actor.Motivation != "unknown" {
		t.Fatalf("types/actor: %+v %+v", e.AttackTypes, e.Actor)
	}
	if e.Victims[0].Country != "GB" || e.Victims[1].Country != "" || e.Victims[1].Sector != "other" {
		t.Fatalf("victims: %+v", e.Victims)
	}
	if len(e.Targets) != 1 || e.Targets[0] != "VN" {
		t.Fatalf("targets: %v", e.Targets)
	}
	if strings.Join(e.Sectors, ",") != "finance,tech" || strings.Join(e.Vectors, ",") != "malicious_package" {
		t.Fatalf("sectors/vectors: %v %v", e.Sectors, e.Vectors)
	}
	if len(e.Techniques) != 1 || e.Techniques[0].ID != "T1195.002" {
		t.Fatalf("techniques: %+v", e.Techniques)
	}
	if len(e.CVEs) != 1 || e.CVEs[0].ID != "CVE-2026-12345" || e.CVEs[0].CVSS != 10 {
		t.Fatalf("cves: %+v", e.CVEs)
	}
	if f := e.Families(); strings.Join(f, ",") != "supply,espionage" {
		t.Fatalf("families: %v", f)
	}
}

func TestBuildBoardMergesIncidents(t *testing.T) {
	home := t.TempDir()
	cfg, err := config.Parse(config.DefaultYAML, home)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(home, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	day := cfg.Today()
	add := func(hash string, ev Event) {
		a := &store.Article{Hash: hash, Feed: "f", Source: "s.com", Region: "intl", Title: "t " + hash, Link: "https://s.com/" + hash, Published: time.Now(), Fetched: time.Now()}
		if _, err := st.InsertArticle(a); err != nil {
			t.Fatal(err)
		}
		ev.normalize()
		b, _ := json.Marshal(ev)
		if err := st.SaveSecEvent(store.SecEvent{ArticleID: a.ID, Key: ev.IncidentKey, Kind: ev.Kind, Date: day, Detail: string(b)}, "m"); err != nil {
			t.Fatal(err)
		}
	}
	add("a", Event{Kind: "incident", IncidentKey: "acme-breach", Title: "A", Severity: 3, AttackTypes: []string{"data_breach"},
		Victims: []Victim{{Name: "Acme", Country: "US", Sector: "retail"}}, CVEs: []CVERef{{ID: "CVE-2026-1111", CVSS: 7}}})
	add("b", Event{Kind: "campaign", IncidentKey: "acme-breach", Title: "A2", Severity: 4, AttackTypes: []string{"extortion"},
		Actor: Actor{Name: "ShinyHunters", Motivation: "financial", Attribution: "confirmed"}, Targets: []string{"VN", "US"},
		Techniques: []Technique{{ID: "T1078", Name: "Valid Accounts", Tactic: "initial-access"}}})
	add("c", Event{Kind: "other", Title: "podcast"})
	if _, err := st.UpsertKEV([]store.KEV{{CVE: "CVE-2026-1111", Vendor: "V", Product: "P", DateAdded: day, Ransomware: "Known", CWEs: []string{"CWE-79"}}}); err != nil {
		t.Fatal(err)
	}
	cfg.Security.Stack = []config.StackItem{{Name: "Go", Match: []string{"golang"}}, {Name: "Vendor V", Match: []string{"v p"}}, {Name: "Retail", Match: []string{"a2"}}}
	b, err := BuildBoard(cfg, st, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Stack) != 3 || b.Stack[0].Events != 0 || len(b.Stack[1].KEV) != 1 || b.Stack[2].Incidents != 1 {
		t.Fatalf("stack: %+v", b.Stack)
	}
	// sự cố chạm "Vendor V" qua sản phẩm của CVE (lấy từ KEV) và "Retail" qua tiêu đề
	if len(b.Incidents) == 1 && fmt.Sprint(b.Incidents[0].Stk) != "[1 2]" {
		t.Fatalf("incident stk: %v", b.Incidents[0].Stk)
	}
	if len(b.Incidents) != 1 || b.Articles != 2 {
		t.Fatalf("incidents=%d articles=%d", len(b.Incidents), b.Articles)
	}
	i := b.Incidents[0]
	if i.Kind != "incident" || i.Sev != 4 || i.Title != "A2" || i.Actor != "ShinyHunters" || len(i.Src) != 2 {
		t.Fatalf("merge: %+v", i)
	}
	if strings.Join(i.Fam, ",") != "ransomware,breach" || strings.Join(i.VC, ",") != "US" || strings.Join(i.TC, ",") != "VN" {
		t.Fatalf("fam/countries: %v %v %v", i.Fam, i.VC, i.TC)
	}
	if m := b.CVEs["CVE-2026-1111"]; m.KEV != day || !m.Ransom {
		t.Fatalf("cve meta: %+v", m)
	}
	if b.Counts["threats"] != 1 || b.Counts["vn"] != 1 || len(b.KEV) != 1 {
		t.Fatalf("counts: %v kev=%d", b.Counts, len(b.KEV))
	}
	if _, ok := b.Countries["VN"]; !ok {
		t.Fatal("thiếu toạ độ VN")
	}
	if !strings.Contains(briefPrompt(cfg, b), "ShinyHunters") {
		t.Fatal("briefPrompt thiếu dữ liệu sự cố")
	}
}

func TestRansomSector(t *testing.T) {
	cases := map[string]string{
		"Healthcare Services": "healthcare", "Port terminal operations – Brazil": "transport", "Law Firms & Legal Services": "legal",
		"IT services – USA": "tech", "fintech payments – US": "finance", "": "unknown", "2 posts - 1h": "unknown", "Kitesurfing": "other",
	}
	for in, want := range cases {
		if got := RansomSector(in); got != want {
			t.Errorf("RansomSector(%q)=%s, muốn %s", in, got, want)
		}
	}
}

func TestExtractText(t *testing.T) {
	page := `<html><body><nav><p>Menu menu menu menu menu menu menu menu menu</p></nav>
<article><h2>Attack chain</h2><p>The attackers exploited CVE-2026-1234 in the VPN appliance to gain initial access to the network.</p>
<div class="related-posts"><p>Read also: another totally unrelated story about something else entirely.</p></div>
<script>var x = "not text at all, should be skipped entirely";</script>
<p>They then dumped credentials with Mimikatz and moved laterally over RDP to the domain controller.</p></article>
<footer><p>Copyright notice that should not appear in the extracted text at all.</p></footer></body></html>`
	got := ExtractText(strings.NewReader(page), 2000)
	for _, w := range []string{"Attack chain", "CVE-2026-1234", "Mimikatz"} {
		if !strings.Contains(got, w) {
			t.Errorf("thiếu %q trong %q", w, got)
		}
	}
	for _, w := range []string{"Menu", "Read also", "not text", "Copyright"} {
		if strings.Contains(got, w) {
			t.Errorf("không được chứa %q: %q", w, got)
		}
	}
}

func TestCountryOf(t *testing.T) {
	if c, ct, ok := CountryOf("vn"); !ok || c != "VN" || ct.VI != "Việt Nam" {
		t.Fatalf("VN: %v %+v", ok, ct)
	}
	if c, ct, _ := CountryOf("UK"); c != "GB" || ct.VI != "Anh" {
		t.Fatalf("UK: %s %+v", c, ct)
	}
	if _, _, ok := CountryOf("XX"); ok {
		t.Fatal("XX không hợp lệ")
	}
}
