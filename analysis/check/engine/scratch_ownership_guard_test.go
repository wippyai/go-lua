package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Scratch ownership is a type property, not a filename or declaration-name
// convention. The module has exactly two worker scratch owners, and both share
// evalscratch.Depth. Any alias, embedding, or third struct carrying that depth
// recreates the displaced scratch owner even when it is moved or renamed.
func TestEvaluatorScratchHasOnlyDeclaredModuleOwners(t *testing.T) {
	loader := newFencePackageLoader(t, "./...")
	for _, meta := range loader.modulePackages("/") {
		candidate := false
		for _, name := range meta.GoFiles {
			source, err := os.ReadFile(filepath.Join(meta.Dir, name))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(source), "EvaluatorScratch") ||
				strings.Contains(string(source), "ProjectionScratch") ||
				strings.Contains(string(source), "evalscratch") {
				candidate = true
				break
			}
		}
		if !candidate {
			continue
		}
		for _, owner := range fenceScratchOwnerTypes(loader.load(meta)) {
			t.Errorf("undeclared scratch owner type %s", owner)
		}
	}
}

func TestEvaluatorScratchFenceRejectsTypeEvasions(t *testing.T) {
	loader := newFencePackageLoader(t, "./...")
	tests := map[string]string{
		"rescan4 third declaration": `package evalscratch
type EvaluatorScratch struct{}
`,
		"renamed depth owner": `package engine
import "` + modulePath + `/analysis/check/fixpoint/evalscratch"
type localWorkerArena struct {
	depth evalscratch.Depth
}
`,
		"alias of equation owner": `package engine
import "` + modulePath + `/analysis/check/fixpoint/equation"
type localWorkerArena = equation.EvaluatorScratch
`,
		"embedded interproc owner": `package engine
import "` + modulePath + `/analysis/check/fixpoint/interproc"
type localWorkerArena struct {
	*interproc.ProjectionScratch
}
`,
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			importPath := modulePath + "/analysis/check/engine"
			if name == "rescan4 third declaration" {
				importPath = modulePath + "/analysis/check/fixpoint/evalscratch"
			}
			typed := loader.source(importPath, source)
			if owners := fenceScratchOwnerTypes(typed); len(owners) == 0 {
				t.Fatal("type-based module scratch fence accepted a third owner")
			}
		})
	}
}
