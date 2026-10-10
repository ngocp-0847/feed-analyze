// Package skills đóng gói các Claude Code skill đi kèm feed-analyze và cài chúng vào .claude/skills.
package skills

import (
	"embed"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

//go:embed files
var files embed.FS

//go:embed CLAUDE.md
var claudeMD string

// Names liệt kê skill có sẵn.
func Names() []string {
	ents, _ := fs.ReadDir(files, "files")
	var out []string
	for _, e := range ents {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

// Lens trả về khung phân tích vi mô ↔ vĩ mô — được nhúng vào prompt phân tích.
func Lens() string {
	b, _ := files.ReadFile("files/micro-macro-lens/reference/framework.md")
	return string(b)
}

// Install chép mọi skill vào <skillsDir>/<name>/..., ghi đè bản cũ. Trả về danh sách file đã ghi.
func Install(skillsDir string) ([]string, error) {
	var written []string
	err := fs.WalkDir(files, "files", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := strings.TrimPrefix(p, "files/")
		dst := filepath.Join(skillsDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		b, err := files.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return err
		}
		written = append(written, dst)
		return nil
	})
	return written, err
}

// WriteClaudeMD ghi CLAUDE.md vào home để phiên `fa chat` biết cách dùng công cụ.
func WriteClaudeMD(home string) error {
	return os.WriteFile(filepath.Join(home, "CLAUDE.md"), []byte(claudeMD), 0o644)
}

// SkillPath trả về đường dẫn nhúng của 1 file skill (dùng cho test).
func SkillPath(name string) string { return path.Join("files", name, "SKILL.md") }
