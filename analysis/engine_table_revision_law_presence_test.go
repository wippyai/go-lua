package analysis

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The construction-plane table and revision laws live in package engine. This
// pin is the presence half: those files name the totality, injectivity, and
// activation-revision statements, so a rewrite that deletes the laws without
// replacement fails here even when the engine test package is mid-edit.

func TestEngineTableAndRevisionLawsArePresent(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location")
	}
	root := filepath.Join(filepath.Dir(thisFile), "engine")
	cases := []struct {
		file  string
		names []string
	}{
		{
			file: "runtime_program_table_law_test.go",
			names: []string{
				"TestProgramQueryTableResolvesEveryDeclaredQueryToOnePublishedRow",
				"TestProgramObservationTableResolvesEveryIdentityToOneDenseOrdinal",
				"TestProgramConstructionRefusesADuplicateObservationIdentity",
			},
		},
		{
			file: "runtime_program_revision_law_test.go",
			names: []string{
				"TestActivationRevisionPathNamesNoDeletionManifestDeclaration",
				"TestActivationRevisionMintsNoSolver",
				"TestActivationRevisionRebindsFromSealedInputsOnly",
				"TestRealActivationRevisionCompletesOverTheSupersededProgram",
			},
		},
	}
	for _, law := range cases {
		src, err := os.ReadFile(filepath.Join(root, law.file))
		if err != nil {
			t.Fatalf("read %s: %v", law.file, err)
		}
		text := string(src)
		for _, name := range law.names {
			if !strings.Contains(text, "func "+name+"(") {
				t.Fatalf("%s no longer declares %s", law.file, name)
			}
		}
	}
}
