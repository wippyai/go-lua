package measure

import "testing"

// TestLocInDirSplitsGeneratedFromAuthored pins locInDir's four-way split
// directly, independent of the rest of Measure: a name-matched generated
// non-test file (generated_helper.go), a name-matched generated test file
// (generated_extra_test.go), a content-only-matched generated non-test
// file (family.go carries no marker, rule_members.go is name-matched, so
// this exercises rule_members.go for name-matching) and, in areab, a
// content-only-matched generated non-test file (marked.go) plus a
// content-only-matched generated test file (stamped_test.go) that carries
// the "Code generated" header under a name isGeneratedByName does not
// match.
func TestLocInDirSplitsGeneratedFromAuthored(t *testing.T) {
	areaa, err := locInDir("testdata/fixture/domain/areaa")
	if err != nil {
		t.Fatalf("locInDir(areaa): %v", err)
	}
	want := LOC{NonTest: 33, Test: 14, GeneratedNonTest: 10, GeneratedTest: 7}
	if areaa != want {
		t.Errorf("locInDir(areaa) = %+v, want %+v", areaa, want)
	}

	areab, err := locInDir("testdata/fixture/domain/areab")
	if err != nil {
		t.Fatalf("locInDir(areab): %v", err)
	}
	want = LOC{NonTest: 11, Test: 5, GeneratedNonTest: 5, GeneratedTest: 7}
	if areab != want {
		t.Errorf("locInDir(areab) = %+v, want %+v", areab, want)
	}
}

// TestLocInDirContentOnlyGeneratedTestFile confirms a test file classified
// as generated purely by its "Code generated" header - not by name - is
// counted as GeneratedTest, not Test, matching the detection
// generatedStats uses for isGeneratedByContent.
func TestLocInDirContentOnlyGeneratedTestFile(t *testing.T) {
	name := "stamped_test.go"
	if isGeneratedByName(name) {
		t.Fatalf("%s unexpectedly matches isGeneratedByName; fixture no longer exercises the content-only path", name)
	}
	hit, err := isGeneratedByContent("testdata/fixture/domain/areab/stamped_test.go")
	if err != nil {
		t.Fatalf("isGeneratedByContent: %v", err)
	}
	if !hit {
		t.Fatal("stamped_test.go expected to carry the Code generated marker")
	}

	areab, err := locInDir("testdata/fixture/domain/areab")
	if err != nil {
		t.Fatalf("locInDir(areab): %v", err)
	}
	if areab.GeneratedTest == 0 {
		t.Errorf("locInDir(areab).GeneratedTest = 0, want stamped_test.go's lines counted as generated test LOC")
	}
}

// TestLocInDirNameOnlyGeneratedTestFile confirms a test file matched only
// by the generated_*.go naming convention is counted as GeneratedTest
// even without re-checking file content.
func TestLocInDirNameOnlyGeneratedTestFile(t *testing.T) {
	if !isGeneratedByName("generated_extra_test.go") {
		t.Fatal("generated_extra_test.go expected to match isGeneratedByName")
	}
	areaa, err := locInDir("testdata/fixture/domain/areaa")
	if err != nil {
		t.Fatalf("locInDir(areaa): %v", err)
	}
	if areaa.GeneratedTest == 0 {
		t.Errorf("locInDir(areaa).GeneratedTest = 0, want generated_extra_test.go's lines counted as generated test LOC")
	}
}
