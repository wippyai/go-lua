package cutoververify

import "testing"

func writeBuildFixtureModule(t *testing.T, dir string) {
	t.Helper()
	writeFixtureFile(t, dir, "go.mod", "module fixture\n\ngo 1.23\n")
}

func TestRunBuildPassesOnACleanModule(t *testing.T) {
	dir := t.TempDir()
	writeBuildFixtureModule(t, dir)
	writeFixtureFile(t, dir, "clean.go", "package fixture\n\nfunc Clean() int { return 1 }\n")

	result := RunBuild(dir)
	if result.Status != StatusPass {
		t.Fatalf("status = %s, want PASS: %s\n%s", result.Status, result.Note, result.Detail)
	}
}

func TestRunBuildFailsOnAProductionCompileError(t *testing.T) {
	dir := t.TempDir()
	writeBuildFixtureModule(t, dir)
	writeFixtureFile(t, dir, "broken.go", "package fixture\n\nfunc Broken() int { return \"not an int\" }\n")

	result := RunBuild(dir)
	if result.Status != StatusFail {
		t.Fatalf("status = %s, want FAIL", result.Status)
	}
	if result.Note != "go build ./... failed" {
		t.Fatalf("note = %q, want the go build failure note", result.Note)
	}
}

// TestRunBuildCatchesATestOnlyCompileError is the law the hostile audit at
// 6c3bf305 forced: go build ./... never compiles _test.go files, so a
// production package that builds cleanly can still carry a test file that
// does not compile - exactly what c2eb571bc4 and 6b457d1d01 each left
// unfixed through several more commits. RunBuild must fail this shape, not
// just a production compile error.
func TestRunBuildCatchesATestOnlyCompileError(t *testing.T) {
	dir := t.TempDir()
	writeBuildFixtureModule(t, dir)
	writeFixtureFile(t, dir, "clean.go", "package fixture\n\nfunc Clean() int { return 1 }\n")
	writeFixtureFile(t, dir, "clean_test.go", "package fixture\n\nimport \"testing\"\n\nfunc TestClean(t *testing.T) {\n\tvar x int = \"not an int\"\n\t_ = x\n}\n")

	result := RunBuild(dir)
	if result.Status != StatusFail {
		t.Fatalf("status = %s, want FAIL - a test-only compile error must fail the BUILD step", result.Status)
	}
	if result.Note != "go vet ./... failed (a package's tests do not compile)" {
		t.Fatalf("note = %q, want the vet-catches-tests note", result.Note)
	}
	if result.Detail == "" {
		t.Fatal("no diagnostic detail reported for the failing test-only compile error")
	}
}

func TestPackagePatternHandlesBareAndRootedImportPaths(t *testing.T) {
	if got := packagePattern("domain/x/y"); got != "./domain/x/y/..." {
		t.Fatalf("packagePattern(bare) = %q", got)
	}
	if got := packagePattern("./domain/x/y"); got != "./domain/x/y/..." {
		t.Fatalf("packagePattern(rooted) = %q", got)
	}
}

func TestFirstLinesTruncates(t *testing.T) {
	if got := firstLines("a\nb\nc\nd\n", 2); got != "a\nb" {
		t.Fatalf("firstLines = %q, want %q", got, "a\nb")
	}
	if got := firstLines("a\nb\n", 10); got != "a\nb" {
		t.Fatalf("firstLines short input = %q", got)
	}
}
