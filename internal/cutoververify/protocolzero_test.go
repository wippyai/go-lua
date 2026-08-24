package cutoververify

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFixtureFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestClassifyProtocolZeroFindsLegacyTokens(t *testing.T) {
	dir := t.TempDir()
	writeFixtureFile(t, dir, "rule.go", "package pkg\n\nfunc init() {\n\tHotRule(\"x\")\n\tBindRule(y)\n}\n")
	writeFixtureFile(t, dir, "clean.go", "package pkg\n\nfunc Clean() int { return 1 }\n")

	report, err := ClassifyProtocolZero(dir)
	if err != nil {
		t.Fatalf("ClassifyProtocolZero: %v", err)
	}
	if len(report.Hits) != 2 {
		t.Fatalf("got %d hits, want 2: %+v", len(report.Hits), report.Hits)
	}
	if report.Hits[0].Line != 4 || report.Hits[1].Line != 5 {
		t.Fatalf("unexpected hit lines: %+v", report.Hits)
	}
}

func TestClassifyProtocolZeroSkipsTestFiles(t *testing.T) {
	dir := t.TempDir()
	writeFixtureFile(t, dir, "rule_test.go", "package pkg\n\nfunc TestX() {\n\tHotRule(\"x\")\n}\n")

	report, err := ClassifyProtocolZero(dir)
	if err != nil {
		t.Fatalf("ClassifyProtocolZero: %v", err)
	}
	if len(report.Hits) != 0 {
		t.Fatalf("got %d hits in a _test.go file, want 0: %+v", len(report.Hits), report.Hits)
	}
}

func TestClassifyProtocolZeroWordBoundary(t *testing.T) {
	dir := t.TempDir()
	writeFixtureFile(t, dir, "similar.go", "package pkg\n\nfunc HotRuleHandler() {}\nfunc callBindRuleSet() {}\n")

	report, err := ClassifyProtocolZero(dir)
	if err != nil {
		t.Fatalf("ClassifyProtocolZero: %v", err)
	}
	if len(report.Hits) != 0 {
		t.Fatalf("got %d hits for names that only contain the token as a substring, want 0: %+v", len(report.Hits), report.Hits)
	}
}

func TestClassifyProtocolZeroCleanTreeIsZero(t *testing.T) {
	dir := t.TempDir()
	writeFixtureFile(t, dir, "clean.go", "package pkg\n\nfunc Clean() int { return 1 }\n")

	report, err := ClassifyProtocolZero(dir)
	if err != nil {
		t.Fatalf("ClassifyProtocolZero: %v", err)
	}
	if len(report.Hits) != 0 {
		t.Fatalf("got %d hits on a clean tree, want 0", len(report.Hits))
	}
}

func TestProtocolZeroCheckWarnsByDefaultAndFailsWhenRequired(t *testing.T) {
	clonePath := t.TempDir()
	pkg := "domain/x"
	writeFixtureFile(t, clonePath, filepath.Join(pkg, "rule.go"), "package x\n\nfunc init() { DeclareRule(nil) }\n")

	warn, err := ProtocolZeroCheck(clonePath, pkg, false)
	if err != nil {
		t.Fatalf("ProtocolZeroCheck: %v", err)
	}
	if warn.Status != StatusWarn {
		t.Fatalf("got status %s, want WARN when requireZero=false", warn.Status)
	}

	fail, err := ProtocolZeroCheck(clonePath, pkg, true)
	if err != nil {
		t.Fatalf("ProtocolZeroCheck: %v", err)
	}
	if fail.Status != StatusFail {
		t.Fatalf("got status %s, want FAIL when requireZero=true", fail.Status)
	}
}
