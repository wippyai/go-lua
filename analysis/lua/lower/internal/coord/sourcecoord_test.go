package coord

import "testing"

func TestBuildPreservesFileOnAnOpenCoordinate(t *testing.T) {
	span, ok := Build("source.lua", 3, 4, 0, 0)
	if !ok || span.File != "source.lua" || span.StartLine != 3 || span.StartCol != 4 || span.EndLine != 0 || span.EndCol != 0 {
		t.Fatalf("Build open coordinate = %#v/%v", span, ok)
	}
}
