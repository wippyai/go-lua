package carrier

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/change"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// coverageDirectionFixture builds one slot plane with a single authored
// target and two nested guard regions, which is enough to move an authored
// surface up and down without any typed root evaluation.
func coverageDirectionFixture(t testing.TB) (*Work, *Composition, *neutralCoverageOperation, support.Mask, support.Mask) {
	t.Helper()
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	narrow, ok := regions.Literal(1, false)
	if !ok || !regions.Seal() {
		t.Fatal("narrow support")
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole support")
	}
	operation := &neutralCoverageOperation{carryOnlyOperation: &carryOnlyOperation{guards: manager}}
	composition, ok := attachTestComposition(t, []FactorOperation{operation})
	if !ok {
		t.Fatal("composition")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	t.Cleanup(func() { work.Close() })
	return work, composition, operation, whole, narrow
}

func coverageOf(composition *Composition, operation *neutralCoverageOperation, region support.Mask) contributionCoverage {
	if !region.Valid() {
		return newContributionCoverage(composition, nil)
	}
	return newContributionCoverage(composition, []slotCoverage{{targets: []TargetRegion{{target: operation.target, region: region}}}})
}

// A coverage delta carries the direction its producer already holds. The
// merge-join knows which half of the split each row landed in, so a surface
// that only gained authorship ascends, one that only lost authorship
// descends, and one that did both says so.
func TestCoverageDeltaIssuesTheDirectionItsMergeJoinAlreadyHolds(t *testing.T) {
	work, composition, operation, whole, narrow := coverageDirectionFixture(t)
	empty := support.Mask{}
	cases := []struct {
		name      string
		previous  support.Mask
		current   support.Mask
		reasons   change.Reason
		direction change.Direction
		admits    bool
		rows      int
	}{
		{
			name: "unchanged surface moves nothing", previous: whole, current: whole,
			direction: change.Known, admits: true,
		},
		{
			name: "gained authorship ascends", previous: empty, current: whole,
			reasons: change.SupportAdded | change.AuthorshipChanged, direction: change.Known | change.Ascends, admits: true, rows: 1,
		},
		{
			name: "lost authorship descends", previous: whole, current: empty,
			reasons: change.SupportRemoved | change.AuthorshipChanged, direction: change.Known | change.Descends, rows: 1,
		},
		{
			name: "shrinking region descends", previous: whole, current: narrow,
			reasons: change.SupportRemoved | change.AuthorshipChanged, direction: change.Known | change.Descends, rows: 1,
		},
		{
			name: "growing region ascends", previous: narrow, current: whole,
			reasons: change.SupportAdded | change.AuthorshipChanged, direction: change.Known | change.Ascends, admits: true, rows: 1,
		},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			delta, ok := work.coverageChangesSurface(coverageOf(composition, operation, item.previous), coverageOf(composition, operation, item.current), true)
			if !ok {
				t.Fatal("coverage delta")
			}
			evidence := delta.Evidence()
			if evidence.Reasons != item.reasons || evidence.Direction != item.direction {
				t.Fatalf("evidence=%+v want reasons=%d direction=%d", evidence, item.reasons, item.direction)
			}
			if evidence.Admits() != item.admits {
				t.Fatalf("Admits()=%v want %v for %+v", evidence.Admits(), item.admits, evidence)
			}
			if delta.Count() != item.rows {
				t.Fatalf("rows=%d want %d", delta.Count(), item.rows)
			}
			row, present := delta.At(0)
			if item.rows != 0 {
				if !present {
					t.Fatal("delta row")
				}
				if support.Empty(row.Added()) == (item.direction&change.Ascends != 0) {
					t.Fatalf("added half=%v contradicts direction %d", row.Added(), item.direction)
				}
				if support.Empty(row.Removed()) == (item.direction&change.Descends != 0) {
					t.Fatalf("removed half=%v contradicts direction %d", row.Removed(), item.direction)
				}
			}
			slots, fresh := delta.Slots()
			if !fresh {
				t.Fatal("a delta refused its own slot set")
			}
			if slots.Count() != item.rows {
				t.Fatalf("touched slots=%d want %d", slots.Count(), item.rows)
			}
		})
	}
}

// A row that both gained and lost region carries both halves, and evidence
// that mixes them is refused for reuse.
func TestCoverageDeltaCarryingBothHalvesIsRefused(t *testing.T) {
	work, composition, operation, whole, narrow := coverageDirectionFixture(t)
	manager := composition.guards
	regions := support.New(manager)
	other, ok := regions.Literal(1, true)
	if !ok || !regions.Seal() {
		t.Fatal("complement support")
	}
	_ = whole
	delta, ok := work.coverageChangesSurface(coverageOf(composition, operation, narrow), coverageOf(composition, operation, other), true)
	if !ok {
		t.Fatal("coverage delta")
	}
	evidence := delta.Evidence()
	if evidence.Direction != change.Known|change.Ascends|change.Descends {
		t.Fatalf("direction=%d want an incomparable move", evidence.Direction)
	}
	if evidence.Admits() {
		t.Fatal("an incomparable authorship move was admitted for reuse")
	}
	row, present := delta.At(0)
	if !present || support.Empty(row.Added()) || support.Empty(row.Removed()) {
		t.Fatalf("row=%+v does not carry both halves", row)
	}
}

// A borrowed slot view lives only until the issuing Work recycles the buffer
// it names. The stamp makes a stale read refuse instead of returning recycled
// words.
func TestBorrowedSlotViewsRefuseAfterTheBufferIsRecycled(t *testing.T) {
	work, composition, operation, whole, _ := coverageDirectionFixture(t)
	delta, ok := work.coverageChangesSurface(coverageOf(composition, operation, support.Mask{}), coverageOf(composition, operation, whole), false)
	if !ok {
		t.Fatal("coverage delta")
	}
	if _, fresh := delta.Slots(); !fresh {
		t.Fatal("a freshly issued view refused its own set")
	}
	// Re-running the operation recycles the same stack depth.
	if _, ok := work.coverageChangesSurface(coverageOf(composition, operation, whole), coverageOf(composition, operation, support.Mask{}), false); !ok {
		t.Fatal("second coverage delta")
	}
	if slots, fresh := delta.Slots(); fresh || slots.Count() != 0 {
		t.Fatal("a recycled buffer answered a stale borrowed view")
	}
	if _, fresh := (CoverageChangeSet{}).Slots(); fresh {
		t.Fatal("the zero delta answered a slot read")
	}
	if _, fresh := (ChangeSet{}).Slots(); fresh {
		t.Fatal("the zero ChangeSet answered a slot read")
	}
}

// The zero ChangeSet is the floor: an operation that reaches a consumer
// without classifying itself is refused, while a real empty publication is
// classified and admitted.
func TestZeroChangeSetIsUnclassifiedWhileAnEmptyPublicationIsAdmitted(t *testing.T) {
	_, composition, _, _, _ := coverageDirectionFixture(t)
	if !(ChangeSet{}).Evidence().Unknown() || (ChangeSet{}).Evidence().Admits() {
		t.Fatal("the zero ChangeSet was classified")
	}
	empty, ok := emptyChangeSet(composition)
	if !ok {
		t.Fatal("empty change set")
	}
	if empty.Evidence().Unknown() || !empty.Evidence().Admits() {
		t.Fatalf("an empty publication is not admitted: %+v", empty.Evidence())
	}
	// Accumulating an unclassified operand erases the classification, whatever
	// order the operands arrive in.
	if empty.Evidence().Union(ChangeSet{}.Evidence()).Admits() {
		t.Fatal("an accumulator admitted an unclassified operand")
	}
	if (ChangeSet{}).Evidence().Union(empty.Evidence()).Admits() {
		t.Fatal("an accumulator admitted an unclassified operand")
	}
}

// terminalCommitSites names the operations whose result can reach the
// published point plane without passing another emitting operation. They must
// issue the ChangeSet their commit produced; every other commit inside
// carrier is an internal operand whose evidence is subsumed by the operation
// that folds it, and discards it deliberately.
var terminalCommitSites = map[string]bool{
	"overlayPointSurface":               true,
	"FinishPointRHSFold":                true,
	"mergeSelectedContributionSurface":  true,
	"MergeContribution":                 true,
	"Replace":                           true,
	"merge3Under":                       true,
	"restrictState":                     true,
	"MergeTransportedPointContribution": true,
}

// Amendment 3's classification is structural: a commit either issues its
// evidence or is declared internal. A new operation is covered the moment it
// is written, because the law reads the calls rather than a list of them.
func TestEveryCarrierCommitIsClassifiedTerminalOrInternal(t *testing.T) {
	fset := token.NewFileSet()
	packages, err := parser.ParseDir(fset, ".", func(info fs.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	terminal, internal := 0, 0
	for _, parsed := range packages {
		for _, file := range parsed.Files {
			for _, declaration := range file.Decls {
				function, isFunction := declaration.(*ast.FuncDecl)
				if !isFunction || function.Body == nil {
					continue
				}
				ast.Inspect(function.Body, func(node ast.Node) bool {
					assignment, isAssignment := node.(*ast.AssignStmt)
					if !isAssignment || len(assignment.Rhs) != 1 {
						return true
					}
					call, isCall := assignment.Rhs[0].(*ast.CallExpr)
					if !isCall || !commitCall(call.Fun) || len(assignment.Lhs) != 3 {
						return true
					}
					issued := true
					if identifier, isIdentifier := assignment.Lhs[1].(*ast.Ident); isIdentifier && identifier.Name == "_" {
						issued = false
					}
					declared := terminalCommitSites[function.Name.Name]
					if issued != declared {
						t.Errorf("%s: %s %s its commit evidence but is declared terminal=%v",
							fset.Position(call.Pos()), function.Name.Name, map[bool]string{true: "issues", false: "discards"}[issued], declared)
					}
					if issued {
						terminal++
					} else {
						internal++
					}
					return true
				})
			}
		}
	}
	if terminal == 0 || internal == 0 {
		t.Fatalf("commit classification selected terminal=%d internal=%d; the law would pass vacuously", terminal, internal)
	}
	// A terminal operation must be able to hand its evidence on.
	for _, parsed := range packages {
		for _, file := range parsed.Files {
			for _, declaration := range file.Decls {
				function, isFunction := declaration.(*ast.FuncDecl)
				if !isFunction || !terminalCommitSites[function.Name.Name] || function.Type.Results == nil {
					continue
				}
				issues := false
				for _, result := range function.Type.Results.List {
					if identifier, isIdentifier := result.Type.(*ast.Ident); isIdentifier && identifier.Name == "ChangeSet" {
						issues = true
					}
				}
				if !issues {
					t.Errorf("%s: terminal operation %s does not return a ChangeSet", fset.Position(function.Pos()), function.Name.Name)
				}
			}
		}
	}
}

func commitCall(expression ast.Expr) bool {
	selector, isSelector := expression.(*ast.SelectorExpr)
	if !isSelector || selector.Sel.Name != "commit" {
		return false
	}
	receiver, isName := selector.X.(*ast.Ident)
	return isName && receiver.Name == "work"
}

// A Narrow is a descent by the definition of the operation, not by what its
// support happened to do. Its support entailment is satisfied by equality, so
// an unchanged support region must still classify the publication as a
// descent -- which is exactly the case the direction-free predicate missed.
func TestNarrowPublicationUnderEqualSupportStillDescends(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	narrow, ok := regions.Literal(1, false)
	if !ok || !regions.Seal() {
		t.Fatal("narrow support")
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole support")
	}
	composition, ok := attachTestComposition(t, []FactorOperation{&carryOnlyOperation{guards: manager}})
	if !ok {
		t.Fatal("composition")
	}
	wide, ok := NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("wide state")
	}
	small, ok := NewState(composition, composition.Scope(), narrow)
	if !ok {
		t.Fatal("small state")
	}
	narrowing, ok := composition.SealNarrowing(nil)
	if !ok {
		t.Fatal("narrowing scope")
	}
	widening, ok := composition.SealWidening(nil)
	if !ok {
		t.Fatal("widening scope")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	defer work.Close()

	equalCurrent, equalRight := selectedPointOperands(t, work, wide, wide)
	_, equalChanges, ok := work.MergeSelectedPointState(Narrow, equalCurrent, equalRight, equalRight, narrowing)
	if !ok {
		t.Fatal("equal-support narrow")
	}
	if equalChanges.Evidence().Direction&change.Descends == 0 {
		t.Fatalf("an equal-support Narrow did not descend: %+v", equalChanges.Evidence())
	}
	if equalChanges.Evidence().Admits() {
		t.Fatal("a Narrow publication was admitted for accumulator reuse")
	}

	shrinkCurrent, shrinkRight := selectedPointOperands(t, work, wide, small)
	_, shrinkChanges, ok := work.MergeSelectedPointState(Narrow, shrinkCurrent, shrinkRight, shrinkRight, narrowing)
	if !ok {
		t.Fatal("shrinking narrow")
	}
	if shrinkChanges.Evidence().Direction&change.Descends == 0 || shrinkChanges.Evidence().Admits() {
		t.Fatalf("a shrinking Narrow was not a refused descent: %+v", shrinkChanges.Evidence())
	}

	// The same operation in the ascending phase keeps its classification and
	// stays admissible, so the descent bit is a fact about the publication
	// rather than a blanket refusal.
	growCurrent, growRight := selectedPointOperands(t, work, small, wide)
	_, growChanges, ok := work.MergeSelectedPointState(Widen, growCurrent, growRight, growRight, widening)
	if !ok {
		t.Fatal("widening")
	}
	if growChanges.Evidence().Direction&change.Ascends == 0 || !growChanges.Evidence().Admits() {
		t.Fatalf("a support-growing Widen was not an admitted ascent: %+v", growChanges.Evidence())
	}
}
