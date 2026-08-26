package foldcheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const protocolShared = `
	total := 0
	staged := make([]string, 0, len(names))
	for index, name := range names {
		if previous, seen := reg[name]; seen {
			total += previous * index
			staged = append(staged, name)
			continue
		}
		reg[name] = index
		total += index
		if total > 1000 {
			total = total % 997
		}
	}
	for ordinal, name := range staged {
		if slot, seen := reg[name]; seen && slot != ordinal {
			total += slot - ordinal
		}
	}
	if len(staged) > len(names) {
		total = -total
	}
	return total
}
`

const protocolBody = `package old

func wireEverything(reg map[string]int, names []string) int {` + protocolShared

const transplantBody = `package survivor

func wireEverythingRenamed(reg map[string]int, names []string) int {` + protocolShared

const restatedBody = `package survivor

func declareEverything(rows []string) map[string]int {
	declared := make(map[string]int, len(rows))
	for ordinal, row := range rows {
		declared[row] = ordinal
	}
	if len(declared) != len(rows) {
		panic("a name declared twice is two statements of one row")
	}
	return declared
}
`

func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=law", "GIT_AUTHOR_EMAIL=law@test",
			"GIT_COMMITTER_NAME=law", "GIT_COMMITTER_EMAIL=law@test")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	return dir
}

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commit(t *testing.T, dir, message string) string {
	t.Helper()
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", message}} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=law", "GIT_AUTHOR_EMAIL=law@test",
			"GIT_COMMITTER_NAME=law", "GIT_COMMITTER_EMAIL=law@test")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out[:len(out)-1])
}

func TestATransplantedBodyIsNamed(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "old/protocol.go", protocolBody)
	rev := commit(t, dir, "pre-cut")
	if err := os.RemoveAll(filepath.Join(dir, "old")); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "survivor/renamed.go", transplantBody)
	findings, err := Check(dir, rev, []string{"old/protocol.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Survivor != "wireEverythingRenamed" {
		t.Fatalf("a transplanted body escaped the fold check: %+v", findings)
	}
}

func TestARestatementIsNotNamed(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "old/protocol.go", protocolBody)
	rev := commit(t, dir, "pre-cut")
	if err := os.RemoveAll(filepath.Join(dir, "old")); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "survivor/declared.go", restatedBody)
	findings, err := Check(dir, rev, []string{"old/protocol.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("a genuine restatement was named as a transplant: %+v", findings)
	}
}
