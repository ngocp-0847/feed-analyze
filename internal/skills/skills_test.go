package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillsHaveFrontmatter(t *testing.T) {
	names := Names()
	if len(names) < 5 {
		t.Fatalf("skills=%v", names)
	}
	for _, n := range names {
		b, err := files.ReadFile(SkillPath(n))
		if err != nil {
			t.Fatalf("%s: %v", n, err)
		}
		s := string(b)
		if !strings.HasPrefix(s, "---\nname: "+n+"\n") || !strings.Contains(s, "\ndescription: ") {
			t.Errorf("%s: frontmatter thiếu name/description khớp tên thư mục", n)
		}
	}
	if !strings.Contains(Lens(), "kênh truyền dẫn") {
		t.Fatal("Lens rỗng")
	}
}

func TestInstall(t *testing.T) {
	dir := t.TempDir()
	written, err := Install(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "micro-macro-lens", "reference", "framework.md")); err != nil {
		t.Fatal(err)
	}
	if len(written) < len(Names()) {
		t.Fatalf("written=%d", len(written))
	}
}
