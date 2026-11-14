package store

import (
	"database/sql"
	"strings"
	"time"
)

// Lịch sử run: mỗi lần chạy lệnh có ghi nhận (fa daily, fetch, classify, analyze, deep, sec …) là 1 dòng trong `runs`,
// các bước của nó trong `run_steps`; mọi lần gọi Claude trong run được gắn run_id (llm_calls.run_id) để tính chi phí theo run.

const runSchema = `
CREATE TABLE IF NOT EXISTS runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  kind TEXT NOT NULL, args TEXT,
  started_at INTEGER NOT NULL, finished_at INTEGER,
  status TEXT NOT NULL DEFAULT 'running', err TEXT
);
CREATE INDEX IF NOT EXISTS idx_runs_started ON runs(started_at);
CREATE TABLE IF NOT EXISTS run_steps (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id INTEGER NOT NULL, name TEXT NOT NULL,
  started_at INTEGER NOT NULL, duration_ms INTEGER,
  status TEXT, detail TEXT, err TEXT
);
CREATE INDEX IF NOT EXISTS idx_run_steps_run ON run_steps(run_id);
`

// Trạng thái của một run.
const (
	RunRunning     = "running"
	RunOK          = "ok"
	RunPartial     = "partial" // xong nhưng có bước lỗi không chặn (vd an ninh mạng, publish)
	RunError       = "error"
	RunCancelled   = "cancelled"   // Ctrl-C
	RunInterrupted = "interrupted" // tiến trình bị giết giữa chừng — không kịp ghi kết thúc
)

// staleRunAfter: run còn "running" quá lâu được coi là đã bị ngắt (lần StartRun sau sẽ đánh dấu).
const staleRunAfter = 12 * time.Hour

// migrateRuns thêm cột llm_calls.run_id cho DB tạo từ phiên bản cũ (CREATE TABLE IF NOT EXISTS không thêm cột).
func migrateRuns(db *sql.DB) error {
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM pragma_table_info('llm_calls') WHERE name='run_id'`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		// 2 tiến trình mở DB cùng lúc có thể cùng thêm cột — bỏ qua lỗi "duplicate column".
		if _, err := db.Exec(`ALTER TABLE llm_calls ADD COLUMN run_id INTEGER`); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return err
		}
	}
	_, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_llm_calls_run ON llm_calls(run_id)`)
	return err
}

type Run struct {
	ID                      int64
	Kind, Args, Status, Err string
	Started, Finished       time.Time // Finished = zero khi chưa kết thúc
	Calls, Failed           int       // số lần gọi Claude, số lần lỗi
	CostUSD                 float64
	Steps                   []RunStep
}

// Duration: thời gian chạy; 0 nếu chưa kết thúc.
func (r Run) Duration() time.Duration {
	if r.Finished.IsZero() {
		return 0
	}
	return r.Finished.Sub(r.Started)
}

type RunStep struct {
	Name, Status, Detail, Err string
	Started                   time.Time
	Duration                  time.Duration
}

type LLMCall struct {
	Time     time.Time
	Purpose  string
	Model    string
	CostUSD  float64
	Duration time.Duration
	OK       bool
	Err      string
}

// StartRun mở 1 run và gắn mọi lệnh gọi Claude / bước tiếp theo của tiến trình này vào run đó.
// Mỗi tiến trình `fa` chỉ có một run hiện hành nên trạng thái này nằm trong Store.
func (s *Store) StartRun(kind, args string) (int64, error) {
	now := time.Now().Unix()
	_, _ = s.db.Exec(`UPDATE runs SET status=? WHERE status=? AND started_at<?`,
		RunInterrupted, RunRunning, now-int64(staleRunAfter.Seconds()))
	res, err := s.db.Exec(`INSERT INTO runs(kind,args,started_at,status) VALUES(?,?,?,?)`, kind, args, now, RunRunning)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	s.runID.Store(id)
	return id, nil
}

// FinishRun ghi kết thúc. status "ok" mà có bước lỗi → "partial".
func (s *Store) FinishRun(id int64, status string, runErr error) {
	if id == 0 {
		return
	}
	if status == RunOK {
		var n int
		_ = s.db.QueryRow(`SELECT count(*) FROM run_steps WHERE run_id=? AND status=?`, id, RunError).Scan(&n)
		if n > 0 {
			status = RunPartial
		}
	}
	errStr := ""
	if runErr != nil {
		errStr = trimTo(runErr.Error(), 1000)
	}
	_, _ = s.db.Exec(`UPDATE runs SET finished_at=?, status=?, err=? WHERE id=?`, time.Now().Unix(), status, errStr, id)
	s.runID.CompareAndSwap(id, 0)
}

// DropRun xoá hẳn 1 run (vd lệnh chỉ in --help, không có gì để lưu).
func (s *Store) DropRun(id int64) {
	if id == 0 {
		return
	}
	_, _ = s.db.Exec(`DELETE FROM run_steps WHERE run_id=?`, id)
	_, _ = s.db.Exec(`DELETE FROM runs WHERE id=?`, id)
	s.runID.CompareAndSwap(id, 0)
}

// AddRunStep ghi 1 bước của run hiện hành (không làm gì nếu chưa StartRun).
func (s *Store) AddRunStep(name string, started time.Time, status, detail string, stepErr error) {
	id := s.runID.Load()
	if id == 0 {
		return
	}
	errStr := ""
	if stepErr != nil {
		errStr = trimTo(stepErr.Error(), 500)
	}
	_, _ = s.db.Exec(`INSERT INTO run_steps(run_id,name,started_at,duration_ms,status,detail,err) VALUES(?,?,?,?,?,?,?)`,
		id, name, started.Unix(), time.Since(started).Milliseconds(), status, detail, errStr)
}

const runCols = `r.id,r.kind,r.args,r.started_at,r.finished_at,r.status,r.err,COALESCE(c.n,0),COALESCE(c.failed,0),COALESCE(c.cost,0)`

const runFrom = ` FROM runs r LEFT JOIN (
  SELECT run_id, count(*) n, sum(CASE WHEN ok=0 THEN 1 ELSE 0 END) failed, sum(cost_usd) cost
  FROM llm_calls WHERE run_id IS NOT NULL GROUP BY run_id) c ON c.run_id=r.id`

func scanRun(sc interface{ Scan(...any) error }) (Run, error) {
	var r Run
	var args, errStr sql.NullString
	var started int64
	var fin sql.NullInt64
	if err := sc.Scan(&r.ID, &r.Kind, &args, &started, &fin, &r.Status, &errStr, &r.Calls, &r.Failed, &r.CostUSD); err != nil {
		return r, err
	}
	r.Args, r.Err = args.String, errStr.String
	r.Started = time.Unix(started, 0)
	if fin.Valid {
		r.Finished = time.Unix(fin.Int64, 0)
	}
	return r, nil
}

// Runs: các run mới nhất trước; kind rỗng = mọi loại.
func (s *Store) Runs(kind string, limit int) ([]Run, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `SELECT ` + runCols + runFrom
	var args []any
	if kind != "" {
		q += ` WHERE r.kind=?`
		args = append(args, kind)
	}
	q += ` ORDER BY r.started_at DESC, r.id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Run
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Run: 1 run kèm các bước; nil nếu không có.
func (s *Store) Run(id int64) (*Run, error) {
	r, err := scanRun(s.db.QueryRow(`SELECT `+runCols+runFrom+` WHERE r.id=?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT name,started_at,COALESCE(duration_ms,0),COALESCE(status,''),COALESCE(detail,''),COALESCE(err,'')
		FROM run_steps WHERE run_id=? ORDER BY started_at, id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var st RunStep
		var ts, ms int64
		if err := rows.Scan(&st.Name, &ts, &ms, &st.Status, &st.Detail, &st.Err); err != nil {
			return nil, err
		}
		st.Started, st.Duration = time.Unix(ts, 0), time.Duration(ms)*time.Millisecond
		r.Steps = append(r.Steps, st)
	}
	return &r, rows.Err()
}

// RunCalls: các lần gọi Claude thuộc 1 run, theo thứ tự thời gian.
func (s *Store) RunCalls(id int64) ([]LLMCall, error) {
	rows, err := s.db.Query(`SELECT ts,purpose,model,cost_usd,duration_ms,ok,COALESCE(err,'') FROM llm_calls WHERE run_id=? ORDER BY ts, id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LLMCall
	for rows.Next() {
		var c LLMCall
		var ts, ms int64
		var ok int
		if err := rows.Scan(&ts, &c.Purpose, &c.Model, &c.CostUSD, &ms, &ok, &c.Err); err != nil {
			return nil, err
		}
		c.Time, c.Duration, c.OK = time.Unix(ts, 0), time.Duration(ms)*time.Millisecond, ok == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

// RunKinds: các loại run đã có (cho bộ lọc).
func (s *Store) RunKinds() []string {
	rows, err := s.db.Query(`SELECT DISTINCT kind FROM runs ORDER BY kind`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var k string
		if rows.Scan(&k) == nil {
			out = append(out, k)
		}
	}
	return out
}

func trimTo(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
