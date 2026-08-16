package testfixture

import "testing"

func TestFrozenCorpusCatalogReturnsDefensiveViews(t *testing.T) {
	first, err := FrozenCorpusProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 911 || len(first[0].files) == 0 {
		t.Fatalf("unexpected frozen corpus denominator: projects=%d", len(first))
	}
	wantName, wantFile := first[0].relative, first[0].files[0]
	first[0].relative = "forged/project"
	first[0].files[0] = "forged.lua"

	second, err := FrozenCorpusProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 911 || second[0].relative != wantName || second[0].files[0] != wantFile {
		t.Fatal("caller mutation changed the process-cached corpus catalog")
	}
	project, err := FrozenCorpusProject(wantName)
	if err != nil {
		t.Fatal(err)
	}
	project.files[0] = "forged-again.lua"
	replayed, err := FrozenCorpusProject(wantName)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.files[0] != wantFile {
		t.Fatal("named lookup exposed the cached file slice")
	}
}
