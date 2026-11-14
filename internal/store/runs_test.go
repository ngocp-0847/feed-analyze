package store

import (
	"errors"
	"testing"
	"time"
)

func TestRunLifecycle(t *testing.T) {
	s := newStore(t)

	id, err := s.StartRun("daily", "--skip-fetch")
	if err != nil {
		t.Fatal(err)
	}
	s.LogLLMCall("classify", "haiku", 0.10, time.Second, nil)
	s.LogLLMCall("analyze:economy", "sonnet", 0.25, 2*time.Second, errors.New("timeout"))
	s.AddRunStep("fetch", time.Now().Add(-time.Second), "ok", "12 bài mới", nil)
	s.AddRunStep("analyze", time.Now(), RunError, "", errors.New("boom"))

	// đang chạy
	r, err := s.Run(id)
	if err != nil || r == nil {
		t.Fatalf("Run: %v %v", r, err)
	}
	if r.Status != RunRunning || !r.Finished.IsZero() || r.Duration() != 0 {
		t.Fatalf("run đang chạy sai: %+v", r)
	}

	// ok nhưng có bước lỗi → partial
	s.FinishRun(id, RunOK, nil)
	r, _ = s.Run(id)
	if r.Status != RunPartial {
		t.Fatalf("status=%s, muốn partial", r.Status)
	}
	if r.Calls != 2 || r.Failed != 1 || r.CostUSD < 0.349 || r.CostUSD > 0.351 {
		t.Fatalf("tổng hợp gọi Claude sai: calls=%d failed=%d cost=%v", r.Calls, r.Failed, r.CostUSD)
	}
	if len(r.Steps) != 2 || r.Steps[0].Name != "fetch" || r.Steps[1].Err != "boom" {
		t.Fatalf("steps: %+v", r.Steps)
	}
	calls, _ := s.RunCalls(id)
	if len(calls) != 2 || calls[1].OK || calls[1].Err != "timeout" {
		t.Fatalf("calls: %+v", calls)
	}

	// sau khi kết thúc, lần gọi Claude ngoài run không gắn vào run cũ
	s.LogLLMCall("ask", "haiku", 1, time.Second, nil)
	r, _ = s.Run(id)
	if r.Calls != 2 {
		t.Fatalf("lần gọi sau FinishRun bị gắn nhầm vào run: calls=%d", r.Calls)
	}
	var orphan int
	_ = s.db.QueryRow(`SELECT count(*) FROM llm_calls WHERE run_id IS NULL`).Scan(&orphan)
	if orphan != 1 {
		t.Fatalf("muốn 1 lần gọi không thuộc run, có %d", orphan)
	}
}

func TestRunsListAndFilter(t *testing.T) {
	s := newStore(t)
	a, _ := s.StartRun("fetch", "")
	s.FinishRun(a, RunOK, nil)
	b, _ := s.StartRun("daily", "")
	s.FinishRun(b, RunError, errors.New("lỗi"))

	all, err := s.Runs("", 10)
	if err != nil || len(all) != 2 || all[0].ID != b {
		t.Fatalf("mới nhất phải đứng đầu: %+v err=%v", all, err)
	}
	if all[0].Err != "lỗi" || all[0].Status != RunError {
		t.Fatalf("run lỗi: %+v", all[0])
	}
	if f, _ := s.Runs("fetch", 10); len(f) != 1 || f[0].ID != a {
		t.Fatalf("lọc theo kind: %+v", f)
	}
	if one, _ := s.Runs("", 1); len(one) != 1 {
		t.Fatalf("limit: %d", len(one))
	}
	if r, err := s.Run(9999); r != nil || err != nil {
		t.Fatalf("run không tồn tại phải trả nil,nil: %v %v", r, err)
	}
	if k := s.RunKinds(); len(k) != 2 || k[0] != "daily" || k[1] != "fetch" {
		t.Fatalf("kinds: %v", k)
	}
}

func TestStaleRunMarkedInterrupted(t *testing.T) {
	s := newStore(t)
	old, _ := s.StartRun("daily", "")
	_, _ = s.db.Exec(`UPDATE runs SET started_at=? WHERE id=?`, time.Now().Add(-13*time.Hour).Unix(), old)
	recent, _ := s.StartRun("fetch", "") // StartRun kế tiếp dọn run cũ treo
	r, _ := s.Run(old)
	if r.Status != RunInterrupted {
		t.Fatalf("run treo >12h phải là interrupted, got %s", r.Status)
	}
	if r2, _ := s.Run(recent); r2.Status != RunRunning {
		t.Fatalf("run mới không được bị đánh dấu: %s", r2.Status)
	}
}

func TestDropRunAndNoRunNoStep(t *testing.T) {
	s := newStore(t)
	s.AddRunStep("x", time.Now(), "ok", "", nil) // chưa StartRun: không làm gì, không panic
	id, _ := s.StartRun("deep", "")
	s.AddRunStep("a", time.Now(), "ok", "", nil)
	s.DropRun(id)
	if r, _ := s.Run(id); r != nil {
		t.Fatal("run đã drop vẫn còn")
	}
	var n int
	_ = s.db.QueryRow(`SELECT count(*) FROM run_steps`).Scan(&n)
	if n != 0 {
		t.Fatalf("còn %d step mồ côi", n)
	}
}

// DB tạo từ phiên bản cũ (llm_calls chưa có run_id) phải được nâng cấp khi mở.
func TestMigrateOldLLMCalls(t *testing.T) {
	s := newStore(t)
	_, _ = s.db.Exec(`DROP TABLE llm_calls`)
	_, _ = s.db.Exec(`CREATE TABLE llm_calls (id INTEGER PRIMARY KEY AUTOINCREMENT, ts INTEGER, purpose TEXT, model TEXT, cost_usd REAL, duration_ms INTEGER, ok INTEGER, err TEXT)`)
	if err := migrateRuns(s.db); err != nil {
		t.Fatal(err)
	}
	if err := migrateRuns(s.db); err != nil { // chạy lại phải idempotent
		t.Fatal(err)
	}
	s.LogLLMCall("x", "m", 0.5, time.Second, nil)
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM llm_calls`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}
