package engine

import (
	"context"
	"go/ast"
	"sort"
	"strings"
	"testing"
)

// The activation-revision laws. An activation revision is the one thing that
// happens to a program after it is sealed, so what a revision is allowed to
// touch is the boundary between the runtime and the compile spine that dies at
// the receipt cut.
//
// The mechanism these laws are stated against is the one in the tree: a revision
// publishes the next accepted relation and either advances the stamp alone or
// installs a prepared structural overlay over the already sealed program. There
// is no recompile seam - runtime_executor.go fails closed on a revision that no
// overlay represents - so "the revision constructs nothing" is stated here as a
// property of the whole revision path rather than of a rebuild entry point.

// revisionPathFiles is the closed file set the activation-revision path lives
// in: the executor loop that publishes the next relation, the activation
// canonicalization and overlay install it drives, the overlay preparation it
// installs, and the plane that is the only rebindable substrate underneath. The
// set is the subject of every structural law below and is shrink-only.
var revisionPathFiles = []string{
	"runtime_epoch_activation.go",
	"runtime_executor.go",
	"runtime_program_plane.go",
	"runtime_selected_overlay.go",
}

// TestActivationRevisionPathNamesNoDeletionManifestDeclaration is the
// receipt-free law: no file on the revision path names a declaration the
// deletion manifest owns, and none of them holds a pinned runtime->compile edge.
// The global manifest law allows a surviving file to keep an edge by pinning it;
// the revision path is held to the stricter bar, because a revision that reached
// the compile spine would put the receipt transaction back on the solve path.
func TestActivationRevisionPathNamesNoDeletionManifestDeclaration(t *testing.T) {
	manifest := manifestFileSet(t)
	index := manifestDeclarations(t, manifest)
	present := make(map[string]bool, len(revisionPathFiles))
	for _, name := range engineSourceFiles(t) {
		present[name] = true
	}
	pinnedFrom := make(map[string][]string, len(pinnedRuntimeCompileEdges))
	for _, edge := range pinnedRuntimeCompileEdges {
		pinnedFrom[edge.from] = append(pinnedFrom[edge.from], edge.identifier)
	}
	for _, name := range revisionPathFiles {
		if !present[name] {
			t.Fatalf("revision path names %s, which is not an engine source file", name)
		}
		if manifest[name] {
			t.Fatalf("revision path file %s is itself on the deletion manifest", name)
		}
		if identifiers := pinnedFrom[name]; len(identifiers) != 0 {
			sort.Strings(identifiers)
			t.Errorf("revision path file %s holds pinned compile edges %s; a revision may not reach the compile spine at all", name, strings.Join(identifiers, ", "))
		}
		for _, reference := range engineReferences(t, parseEngineFile(t, name), index) {
			t.Errorf("revision path file %s names %s, declared in the deletion manifest file %s", name, reference.identifier, reference.target)
		}
	}
}

// TestActivationRevisionMintsNoSolver is the sole-mint law. mintProgramSolver is
// the one place a Solver value comes into existence; the revision path installs
// an overlay into the Solver it was called on and issues no second one. The
// structural half proves the mint is unique, the behavioural half proves a real
// revision does not use it.
func TestActivationRevisionMintsNoSolver(t *testing.T) {
	manifest := manifestFileSet(t)
	mints := map[string]int{}
	for _, name := range engineSourceFiles(t) {
		if manifest[name] {
			continue
		}
		ast.Inspect(parseEngineFile(t, name), func(node ast.Node) bool {
			literal, ok := node.(*ast.CompositeLit)
			if !ok {
				return true
			}
			if identifier, named := literal.Type.(*ast.Ident); named && identifier.Name == "Solver" {
				mints[name]++
			}
			return true
		})
	}
	if len(mints) != 1 || mints["runtime_program_construct.go"] != 1 {
		t.Fatalf("Solver is constructed at %v, the sole mint is one literal in runtime_program_construct.go", mints)
	}

	fixture := newSelectedOverlayFixture(t)
	solver := fixture.solver
	runtime, store, relation := solver.runtime, solver.store, solver.relation
	state, status := solver.Solve(context.Background())
	if status != SolveComplete || state == nil {
		t.Fatalf("selected overlay revision solve = state:%t status:%v", state != nil, status)
	}
	if !relation.Precedes(solver.relation) {
		t.Fatal("the solve took no activation revision")
	}
	if solver.runtime != runtime || solver.store != store {
		t.Fatalf("the revision replaced the Solver: runtimeSame=%t storeSame=%t", solver.runtime == runtime, solver.store == store)
	}
}

// TestActivationRevisionRebindsFromSealedInputsOnly is the retained-input law:
// by the time the first revision runs, the construction ledger the program was
// sealed from is already released, so the revision has no attachment ledger, no
// bound implementation map and no committed graph handle to replay. Everything
// it consumes is a sealed value the runtime already owns.
func TestActivationRevisionRebindsFromSealedInputsOnly(t *testing.T) {
	fixture := newSelectedOverlayFixture(t)
	inner := fixture.construction.inner
	if inner == nil {
		t.Fatal("sealed construction released its handle")
	}
	inner.mu.Lock()
	closed := inner.closed
	retained := inner.programPlane != nil || inner.members != nil || inner.queries != nil || inner.observations != nil || inner.observationIDs != nil
	inner.mu.Unlock()
	if !closed || retained {
		t.Fatalf("the sealed construction still holds a ledger: closed=%t retained=%t", closed, retained)
	}
	// The ledger cannot be reopened, so no revision can route back through it.
	if fixture.construction.Close() {
		t.Fatal("a sealed construction reopened for close")
	}
	if AttachExactQuery[uint64, uint64](fixture.construction, nil, fixture.triggerID) {
		t.Fatal("a sealed construction admitted an attachment")
	}
	// With the ledger gone, the revision still completes.
	state, status := fixture.solver.Solve(context.Background())
	if status != SolveComplete || state == nil {
		t.Fatalf("revision over a released ledger = state:%t status:%v", state != nil, status)
	}
}

// TestRealActivationRevisionCompletesOverTheSupersededProgram is the
// end-to-end law, and the one whose statement the current mechanism changes. A
// real revision - one that widens demand into a body the initial closure never
// reached - drives the solve to SolveComplete. It does so without a second
// sealed program: the program, graph and carrier the construction sealed are the
// same objects afterwards, and the superseded relation stays a valid accepted
// relation of the same topology rather than being invalidated by its successor.
func TestRealActivationRevisionCompletesOverTheSupersededProgram(t *testing.T) {
	fixture := newSelectedOverlayFixture(t)
	solver := fixture.solver
	runtime := solver.runtime
	program, graph, carrier, topology := runtime.program, runtime.graph, runtime.carrier, runtime.topology
	superseded := solver.relation

	body, bodyOK := fixture.graph.lookupPoint(fixture.bodyID)
	bodyIndex, bodyIndexed := graph.PointIndex(body)
	if !bodyOK || !bodyIndexed || bodyIndex < 0 || bodyIndex >= len(runtime.activePoints) || runtime.activePoints[bodyIndex] {
		t.Fatal("the fixture does not start with an undemanded body")
	}

	state, status, report := solver.SolveWithReport(context.Background())
	if status != SolveComplete || state == nil {
		t.Fatalf("real revision solve = state:%t status:%v reason:%v", state != nil, status, report.Reason())
	}
	// The revision was real: demand widened into the body, which only an
	// installed structural overlay can do.
	if !runtime.activePoints[bodyIndex] {
		t.Fatal("the solve completed without widening demand into the body")
	}
	if !superseded.Precedes(solver.relation) || superseded.Generation() >= solver.relation.Generation() {
		t.Fatalf("the accepted relation did not advance: %d then %d", superseded.Generation(), solver.relation.Generation())
	}
	// No second program was sealed for the revision.
	if solver.runtime != runtime || runtime.program != program || runtime.graph != graph || runtime.carrier != carrier {
		t.Fatalf("the revision resealed the program: runtimeSame=%t programSame=%t graphSame=%t carrierSame=%t",
			solver.runtime == runtime, runtime.program == program, runtime.graph == graph, runtime.carrier == carrier)
	}
	// The superseded relation remains a valid accepted relation of the topology
	// it was published under, so the revision widened the frontier rather than
	// replacing an authority.
	if !topology.ValidAccepted(superseded.Rows()) || !topology.ValidAccepted(solver.relation.Rows()) {
		t.Fatal("a published relation stopped being a valid accepted relation of its topology")
	}
	// The completed state answers, and a repeated solve neither revises again nor
	// republishes a different program.
	value, readable := testSnapshotObservationValue[uint64](solver, state, fixture.observation.id)
	if !readable || value != 1 {
		t.Fatalf("observation over the revised solve = %d/%t", value, readable)
	}
	settled := solver.relation
	warm, warmStatus := solver.Solve(context.Background())
	if warmStatus != SolveComplete || warm != state || solver.relation.Generation() != settled.Generation() || solver.runtime.program != program {
		t.Fatalf("a warm solve revised again: status=%v sameState=%t generation=%d program=%t", warmStatus, warm == state, solver.relation.Generation(), solver.runtime.program == program)
	}
}
