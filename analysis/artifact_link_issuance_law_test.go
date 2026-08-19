package analysis

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/domain/composite"
)

// Link construction enumerates the sealed LaneLink table. Bootstrap Spec keys
// stay on their owning registrations.

func TestCompileScalarTemplateReadsSealedSnapshot(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "compile.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	if !strings.Contains(text, "func newEngineArtifactScalarTemplate(snapshot *ingress.Snapshot)") {
		t.Fatal("scalar template builder does not take a sealed ingress snapshot")
	}
	if strings.Contains(text, "func newEngineArtifactScalarTemplate(artifact *programartifact.Artifact)") {
		t.Fatal("scalar template builder still takes ProgramArtifact")
	}
	linkFn := "func linkArtifactRows(mounts []mountedProgramArtifact)"
	start := strings.Index(text, linkFn)
	if start < 0 {
		t.Fatal("linkArtifactRows missing from compile.go")
	}
	rest := text[start:]
	end := strings.Index(rest[len(linkFn):], "\nfunc ")
	if end < 0 {
		t.Fatal("linkArtifactRows body unbound")
	}
	body := rest[:len(linkFn)+end]
	if strings.Contains(body, "ingress.Lower") {
		t.Fatal("linkArtifactRows re-Lowers instead of projecting the compile-time snapshot")
	}
}

func TestAssembleReadsSealedSnapshotIdentity(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "analyze.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	assemble := "func (state *compiledState) assembleCommittedProgram()"
	start := strings.Index(text, assemble)
	if start < 0 {
		t.Fatal("assembleCommittedProgram missing from analyze.go")
	}
	rest := text[start:]
	end := strings.Index(rest[len(assemble):], "\nfunc ")
	if end < 0 {
		t.Fatal("assembleCommittedProgram body unbound")
	}
	body := rest[:len(assemble)+end]
	if strings.Contains(body, "mount.artifact.ID()") {
		t.Fatal("assemble still reopens ProgramArtifact for identity")
	}
	if !strings.Contains(body, "mount.snapshot.ArtifactID()") {
		t.Fatal("assemble does not read the sealed snapshot identity")
	}
}

func TestObservationSitesReadSealedSnapshot(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "program", "link", "mounted", "observation_site.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	if strings.Contains(text, "mount.Artifact.DiagnosticObservation") {
		t.Fatal("observation sites still reopen ProgramArtifact diagnostic columns")
	}
	if !strings.Contains(text, "mount.Snapshot.DiagnosticObservationCount()") {
		t.Fatal("observation sites do not walk the sealed snapshot diagnostic column")
	}
}

func TestDiagnosticProjectorReadsSealedSnapshot(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "diagnostic", "observation.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	if strings.Contains(text, "diagnosticObservationByID(mount.artifact") ||
		strings.Contains(text, "func diagnosticObservationByID(artifact *programartifact.Artifact") {
		t.Fatal("diagnostic projector still reopens ProgramArtifact observation rows")
	}
	if !strings.Contains(text, "mount.Snapshot.DiagnosticObservationForID") {
		t.Fatal("diagnostic projector does not recover sealed snapshot observations")
	}
	if strings.Contains(text, "func declaredMay(artifact *programartifact.Artifact") {
		t.Fatal("declared-may still reopens ProgramArtifact static nodes")
	}
	if !strings.Contains(text, "func declaredMay(snapshot *ingress.Snapshot") {
		t.Fatal("declared-may does not read sealed static type nodes")
	}
}

func TestBindEnumeratesSealedStaticTypeArguments(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "compile.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	bind := "func (state *compiledState) newProgramBinding"
	start := strings.Index(text, bind)
	if start < 0 {
		t.Fatal("newProgramBinding missing from compile.go")
	}
	rest := text[start:]
	end := strings.Index(rest[len(bind):], "\nfunc ")
	if end < 0 {
		t.Fatal("newProgramBinding body unbound")
	}
	body := rest[:len(bind)+end]
	if !strings.Contains(body, "snapshot.StaticTypeArgumentCount()") || !strings.Contains(body, "snapshot.StaticTypeArgumentAt") {
		t.Fatal("bind does not enumerate sealed static type-argument rows")
	}
}

func TestBindEnumeratesSealedStaticTypeNodes(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "compile.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	bind := "func (state *compiledState) newProgramBinding"
	start := strings.Index(text, bind)
	if start < 0 {
		t.Fatal("newProgramBinding missing from compile.go")
	}
	rest := text[start:]
	end := strings.Index(rest[len(bind):], "\nfunc ")
	if end < 0 {
		t.Fatal("newProgramBinding body unbound")
	}
	body := rest[:len(bind)+end]
	if !strings.Contains(body, "snapshot.StaticTypeNodeCount()") || !strings.Contains(body, "snapshot.StaticTypeNodeAt") {
		t.Fatal("bind does not enumerate sealed static type-node rows")
	}
	if !strings.Contains(body, "row.Owner()") || !strings.Contains(body, "published.programID") {
		t.Fatal("bind does not require sealed static type-node owner identity")
	}
}

func TestBindEnumeratesSealedStaticExpressions(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "compile.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	bind := "func (state *compiledState) newProgramBinding"
	start := strings.Index(text, bind)
	if start < 0 {
		t.Fatal("newProgramBinding missing from compile.go")
	}
	rest := text[start:]
	end := strings.Index(rest[len(bind):], "\nfunc ")
	if end < 0 {
		t.Fatal("newProgramBinding body unbound")
	}
	body := rest[:len(bind)+end]
	if !strings.Contains(body, "snapshot.StaticExpressionCount()") || !strings.Contains(body, "snapshot.StaticExpressionAt") {
		t.Fatal("bind does not enumerate sealed static expression rows")
	}
	if !strings.Contains(body, "row.Owner()") || !strings.Contains(body, "published.programID") {
		t.Fatal("bind does not require sealed static expression owner identity")
	}
}

func TestBindEnumeratesSealedStaticInputs(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "compile.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	bind := "func (state *compiledState) newProgramBinding"
	start := strings.Index(text, bind)
	if start < 0 {
		t.Fatal("newProgramBinding missing from compile.go")
	}
	rest := text[start:]
	end := strings.Index(rest[len(bind):], "\nfunc ")
	if end < 0 {
		t.Fatal("newProgramBinding body unbound")
	}
	body := rest[:len(bind)+end]
	if !strings.Contains(body, "snapshot.StaticInputCount()") || !strings.Contains(body, "snapshot.StaticInputAt") {
		t.Fatal("bind does not enumerate sealed static input rows")
	}
	if !strings.Contains(body, "row.Owner()") || !strings.Contains(body, "published.programID") {
		t.Fatal("bind does not require sealed static input owner identity")
	}
}

func TestBindEnumeratesSealedStaticTypeValues(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "compile.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	bind := "func (state *compiledState) newProgramBinding"
	start := strings.Index(text, bind)
	if start < 0 {
		t.Fatal("newProgramBinding missing from compile.go")
	}
	rest := text[start:]
	end := strings.Index(rest[len(bind):], "\nfunc ")
	if end < 0 {
		t.Fatal("newProgramBinding body unbound")
	}
	body := rest[:len(bind)+end]
	if !strings.Contains(body, "snapshot.StaticTypeValueCount()") || !strings.Contains(body, "snapshot.StaticTypeValueAt") {
		t.Fatal("bind does not enumerate sealed static type-value rows")
	}
	if !strings.Contains(body, "row.Available()") {
		t.Fatal("bind does not require sealed static type-value availability")
	}
}

func TestBindEnumeratesSealedCallTypeArguments(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "compile.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	bind := "func (state *compiledState) newProgramBinding"
	start := strings.Index(text, bind)
	if start < 0 {
		t.Fatal("newProgramBinding missing from compile.go")
	}
	rest := text[start:]
	end := strings.Index(rest[len(bind):], "\nfunc ")
	if end < 0 {
		t.Fatal("newProgramBinding body unbound")
	}
	body := rest[:len(bind)+end]
	if !strings.Contains(body, "snapshot.CallCount()") || !strings.Contains(body, "snapshot.CallAt") {
		t.Fatal("bind does not enumerate sealed call rows")
	}
	if !strings.Contains(body, "row.TypeArgumentsID()") || !strings.Contains(body, "row.BodyID()") {
		t.Fatal("bind does not require sealed call type-argument identities")
	}
	if !strings.Contains(body, "seenCalls") || !strings.Contains(body, "operand.Callee()") {
		t.Fatal("bind does not require unique sealed call IDs and one callee operand")
	}
}

func TestBindEnumeratesSealedValues(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "compile.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	bind := "func (state *compiledState) newProgramBinding"
	start := strings.Index(text, bind)
	if start < 0 {
		t.Fatal("newProgramBinding missing from compile.go")
	}
	rest := text[start:]
	end := strings.Index(rest[len(bind):], "\nfunc ")
	if end < 0 {
		t.Fatal("newProgramBinding body unbound")
	}
	body := rest[:len(bind)+end]
	if !strings.Contains(body, "snapshot.ValuesCount()") || !strings.Contains(body, "snapshot.ValuesAt") {
		t.Fatal("bind does not enumerate sealed values rows")
	}
}

func TestBindEnumeratesSealedHeapIndexes(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "compile.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	bind := "func (state *compiledState) newProgramBinding"
	start := strings.Index(text, bind)
	if start < 0 {
		t.Fatal("newProgramBinding missing from compile.go")
	}
	rest := text[start:]
	end := strings.Index(rest[len(bind):], "\nfunc ")
	if end < 0 {
		t.Fatal("newProgramBinding body unbound")
	}
	body := rest[:len(bind)+end]
	if !strings.Contains(body, "snapshot.HeapIndexCount()") || !strings.Contains(body, "snapshot.HeapIndexAt") {
		t.Fatal("bind does not enumerate sealed heap-index rows")
	}
	if !strings.Contains(body, "snapshot.HeapAllocationCount()") || !strings.Contains(body, "snapshot.HeapAllocationAt") {
		t.Fatal("bind does not enumerate sealed heap-allocation rows")
	}
}

func TestBindEnumeratesSealedCallTargets(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "compile.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	bind := "func (state *compiledState) newProgramBinding"
	start := strings.Index(text, bind)
	if start < 0 {
		t.Fatal("newProgramBinding missing from compile.go")
	}
	rest := text[start:]
	end := strings.Index(rest[len(bind):], "\nfunc ")
	if end < 0 {
		t.Fatal("newProgramBinding body unbound")
	}
	body := rest[:len(bind)+end]
	if !strings.Contains(body, "snapshot.CallTargetCount()") || !strings.Contains(body, "snapshot.CallTargetAt") ||
		!strings.Contains(body, "row.ContextID()") || !strings.Contains(body, "body.Callable()") {
		t.Fatal("bind does not enumerate sealed call-target rows")
	}
}

func TestBindEnumeratesSealedFunctionBoundaries(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "compile.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	bind := "func (state *compiledState) newProgramBinding"
	start := strings.Index(text, bind)
	if start < 0 {
		t.Fatal("newProgramBinding missing from compile.go")
	}
	rest := text[start:]
	end := strings.Index(rest[len(bind):], "\nfunc ")
	if end < 0 {
		t.Fatal("newProgramBinding body unbound")
	}
	body := rest[:len(bind)+end]
	if !strings.Contains(body, "snapshot.FunctionBoundaryCount()") || !strings.Contains(body, "snapshot.FunctionBoundaryAt") ||
		!strings.Contains(body, "row.FormalAt") || !strings.Contains(body, "snapshot.FunctionBoundaryForBody") {
		t.Fatal("bind does not enumerate sealed function-boundary rows")
	}
	if !strings.Contains(body, "snapshot.OutcomeCount()") || !strings.Contains(body, "snapshot.OutcomeAt") ||
		!strings.Contains(body, "snapshot.OutcomeReturnValueAt") {
		t.Fatal("bind does not enumerate sealed outcome rows")
	}
	if !strings.Contains(body, "OccurrenceStorageBind") {
		t.Fatal("bind does not enumerate sealed storage-bind occurrences")
	}
	if !strings.Contains(body, "OccurrenceAllocation") || !strings.Contains(body, "OccurrenceForID") {
		t.Fatal("bind does not enumerate sealed allocation occurrences")
	}
	if !strings.Contains(body, "OccurrenceIndexRead") || !strings.Contains(body, "OccurrenceIndexWrite") {
		t.Fatal("bind does not enumerate sealed index-access occurrences")
	}
}

func TestBindEnumeratesSealedOccurrences(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "compile.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	bind := "func (state *compiledState) newProgramBinding"
	start := strings.Index(text, bind)
	if start < 0 {
		t.Fatal("newProgramBinding missing from compile.go")
	}
	rest := text[start:]
	end := strings.Index(rest[len(bind):], "\nfunc ")
	if end < 0 {
		t.Fatal("newProgramBinding body unbound")
	}
	body := rest[:len(bind)+end]
	if !strings.Contains(body, "snapshot.OccurrenceCount()") || !strings.Contains(body, "snapshot.OccurrenceAt") {
		t.Fatal("bind does not enumerate the sealed occurrence stream")
	}
}

func TestBindLooksUpOwnerHandoffFromByProgram(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "compile.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	if strings.Contains(text, "type mountedProgramArtifact struct") {
		start := strings.Index(text, "type mountedProgramArtifact struct")
		rest := text[start:]
		end := strings.Index(rest, "\n}")
		if end < 0 {
			t.Fatal("mountedProgramArtifact unbound")
		}
		if strings.Contains(rest[:end], "*programartifact.Artifact") {
			t.Fatal("mount row still carries the owner-handoff ProgramArtifact")
		}
	}
	bind := "func (state *compiledState) newProgramBinding"
	start := strings.Index(text, bind)
	if start < 0 {
		t.Fatal("newProgramBinding missing from compile.go")
	}
	rest := text[start:]
	end := strings.Index(rest[len(bind):], "\nfunc ")
	if end < 0 {
		t.Fatal("newProgramBinding body unbound")
	}
	body := rest[:len(bind)+end]
	if strings.Contains(body, "published.artifact") {
		t.Fatal("bind still reads ProgramArtifact from the mount row")
	}
	if !strings.Contains(body, "state.artifacts.byProgram[published.programID]") {
		t.Fatal("bind does not look up the owner-handoff bag by ProgramID")
	}
}

func TestArtifactPlanFileIsGone(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location")
	}
	path := filepath.Join(filepath.Dir(thisFile), "artifact_plan.go")
	if _, err := os.Stat(path); err == nil {
		t.Fatal("artifact_plan.go remains in analysis root")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestArtifactQueryPlanFileIsGone(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location")
	}
	path := filepath.Join(filepath.Dir(thisFile), "artifact_query_plan.go")
	if _, err := os.Stat(path); err == nil {
		t.Fatal("artifact_query_plan.go remains in analysis root")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestArtifactRulePlanFileIsGone(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location")
	}
	path := filepath.Join(filepath.Dir(thisFile), "artifact_rule_plan.go")
	if _, err := os.Stat(path); err == nil {
		t.Fatal("artifact_rule_plan.go remains in analysis root")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestArtifactDiagnosticPlanFileIsGone(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location")
	}
	path := filepath.Join(filepath.Dir(thisFile), "artifact_diagnostic_plan.go")
	if _, err := os.Stat(path); err == nil {
		t.Fatal("artifact_diagnostic_plan.go remains in analysis root")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestLinkConstructionUsesSealedLinkKeys(t *testing.T) {
	keys := composite.LinkKeys()
	if len(keys) == 0 {
		t.Fatal("sealed table declares no Link-lane keys")
	}
	for _, key := range keys {
		if !key.Available() || composite.MountedRuleKey(key) {
			t.Fatalf("link key %q is not a sealed LaneLink identity", key)
		}
	}
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location")
	}
	root := filepath.Dir(thisFile)
	for _, name := range []string{"compile.go", "analyze.go"} {
		src, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, literal := range []string{`"value-bootstrap"`, `"heap-bootstrap"`} {
			if strings.Contains(string(src), literal) {
				t.Fatalf("%s restates Link key %s; construction walks composite.LinkKeys", name, literal)
			}
		}
	}
}
