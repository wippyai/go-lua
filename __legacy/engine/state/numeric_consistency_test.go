package state

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
)

func TestNumericConsistencyContradictoryRelationCycleIsBottom(t *testing.T) {
	domain, _ := numericConsistencyTestDomain(t)
	x := RelValueOperand(pathaddr.StateKey("local:x"))
	y := RelValueOperand(pathaddr.StateKey("local:y"))
	raw := domain.Lattice().Bottom().
		WriteDiffConstraint(x, y, -1).
		WriteDiffConstraint(y, x, -1)
	if got := domain.Normalize(raw); !domain.Lattice().Equal(got, domain.Lattice().Bottom()) {
		t.Fatalf("contradictory relation cycle remained reachable: %#v", got.RelConstraints())
	}
}

func TestNumericConsistencyFloorAboveCeilingIsBottom(t *testing.T) {
	domain, keys := numericConsistencyTestDomain(t)
	x := pathaddr.StateKey("local:x")
	raw := domain.Lattice().Bottom().WriteNumFloor(keys, x, 2).WriteNumCeil(keys, x, 1)
	if got := domain.Normalize(raw); !domain.Lattice().Equal(got, domain.Lattice().Bottom()) {
		t.Fatalf("floor above ceiling remained reachable: floors=%#v ceilings=%#v", got.NumFloorsSnapshot(keys), got.NumCeilsSnapshot(keys))
	}
}

func TestNumericConsistencyCrossRelationAndBoundsIsBottom(t *testing.T) {
	domain, keys := numericConsistencyTestDomain(t)
	xKey, yKey := pathaddr.StateKey("local:x"), pathaddr.StateKey("local:y")
	raw := domain.Lattice().Bottom().
		WriteNumFloor(keys, xKey, 1).
		WriteNumCeil(keys, yKey, 1).
		WriteDiffConstraint(RelValueOperand(xKey), RelValueOperand(yKey), -1)
	if got := domain.Normalize(raw); !domain.Lattice().Equal(got, domain.Lattice().Bottom()) {
		t.Fatalf("cross-lane contradiction remained reachable")
	}
}

func TestNumericConsistencySatisfiableStateRetainedAndNormalizationIdempotent(t *testing.T) {
	domain, keys := numericConsistencyTestDomain(t)
	xKey, yKey := pathaddr.StateKey("local:x"), pathaddr.StateKey("local:y")
	raw := domain.Lattice().Bottom().
		WriteNumFloor(keys, xKey, 1).
		WriteNumCeil(keys, yKey, 9).
		WriteDiffConstraint(RelValueOperand(xKey), RelValueOperand(yKey), -1)
	first := domain.Normalize(raw)
	if domain.Lattice().Equal(first, domain.Lattice().Bottom()) || first.numericConsistency != numericConsistencyCertified {
		t.Fatalf("satisfiable state was not certified")
	}
	second := domain.Normalize(first)
	if !domain.Lattice().Equal(first, second) || second.numericConsistency != numericConsistencyCertified {
		t.Fatalf("normalization is not an idempotent certified fixed point")
	}
}

func TestNumericConsistencyDirtyComponentContradictionIsWholeStateBottom(t *testing.T) {
	domain, keys := numericConsistencyTestDomain(t)
	xKey, yKey := pathaddr.StateKey("local:x"), pathaddr.StateKey("local:y")
	base := domain.Normalize(domain.Lattice().Bottom().
		WriteNumCeil(keys, yKey, 1).
		WriteDiffConstraint(RelValueOperand(xKey), RelValueOperand(yKey), -1))
	if domain.Lattice().Equal(base, domain.Lattice().Bottom()) || base.numericConsistency != numericConsistencyCertified {
		t.Fatal("satisfiable base was not certified")
	}
	candidate := base.WriteNumFloor(keys, xKey, 1)
	if candidate.numericConsistency != numericConsistencyDirty {
		t.Fatal("single-component mutation did not take localized certification path")
	}
	got := domain.Normalize(candidate)
	bottom := domain.Lattice().Bottom()
	if !domain.Lattice().Equal(got, bottom) || !got.canonical || got.laneMask != bottom.laneMask || got.numericConsistency != numericConsistencyCertified {
		t.Fatal("dirty-component contradiction did not publish canonical whole-State Bottom")
	}
}

func TestNumericConsistencyFactorComposePublication(t *testing.T) {
	domain, keys := numericConsistencyTestDomain(t)
	xKey, yKey := pathaddr.StateKey("local:x"), pathaddr.StateKey("local:y")
	satisfiable := domain.Normalize(domain.Lattice().Bottom().
		WriteNumFloor(keys, xKey, 1).
		WriteNumCeil(keys, yKey, 9).
		WriteDiffConstraint(RelValueOperand(xKey), RelValueOperand(yKey), -1))
	factors, err := domain.Decompose(satisfiable)
	if err != nil {
		t.Fatal(err)
	}
	recomposed, err := domain.Compose(factors)
	if err != nil || !domain.Lattice().Equal(recomposed, satisfiable) || recomposed.numericConsistency != numericConsistencyCertified {
		t.Fatalf("factor round trip = equal:%t certified:%t err:%v", domain.Lattice().Equal(recomposed, satisfiable), recomposed.numericConsistency == numericConsistencyCertified, err)
	}

	states := map[LaneID]State{
		LaneNumFloors:     domain.Lattice().Bottom().WriteNumFloor(keys, xKey, 1),
		LaneNumCeils:      domain.Lattice().Bottom().WriteNumCeil(keys, yKey, 1),
		LaneDiffRelations: domain.Lattice().Bottom().WriteDiffConstraint(RelValueOperand(xKey), RelValueOperand(yKey), -1),
	}
	mixed, err := domain.Decompose(domain.Lattice().Bottom())
	if err != nil {
		t.Fatal(err)
	}
	for laneID, source := range states {
		lane, _ := domain.ProductLane(laneID)
		selected, selectErr := domain.DecomposeLanes(source, []ProductLane{lane})
		if selectErr != nil {
			t.Fatal(selectErr)
		}
		mixed[int(lane.Ordinal())] = selected[0]
	}
	contradictory, err := domain.Compose(mixed)
	if err != nil || !domain.Lattice().Equal(contradictory, domain.Lattice().Bottom()) {
		t.Fatalf("mixed contradictory factors = bottom:%t err:%v", domain.Lattice().Equal(contradictory, domain.Lattice().Bottom()), err)
	}
}

func TestNumericConsistencyMutationSurfaceUsesCanonicalSetters(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	directory := filepath.Dir(thisFile)
	files, err := filepath.Glob(filepath.Join(directory, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	tracked := map[string]struct{}{"numFloors": {}, "numCeils": {}, "lenFloors": {}, "diffRelations": {}}
	for _, file := range files {
		if filepath.Base(file) == "numeric_consistency.go" || filepath.Ext(file) != ".go" || strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			assignment, ok := node.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for _, target := range assignment.Lhs {
				selector, selectorOK := target.(*ast.SelectorExpr)
				if !selectorOK {
					continue
				}
				if _, watched := tracked[selector.Sel.Name]; watched {
					t.Errorf("direct numeric lane assignment outside canonical setter: %s", file)
				}
			}
			return true
		})
	}
}

func BenchmarkNumericConsistencyTwoThousandComponents(b *testing.B) {
	domain, _ := numericConsistencyTestDomain(b)
	raw := domain.Lattice().Bottom()
	for index := 0; index < 2000; index++ {
		left := RelValueOperand(pathaddr.StateKey(fmt.Sprintf("local:left-%d", index)))
		right := RelValueOperand(pathaddr.StateKey(fmt.Sprintf("local:right-%d", index)))
		raw = raw.WriteDiffConstraint(left, right, int64(index))
	}
	b.ResetTimer()
	for b.Loop() {
		candidate := raw
		candidate.numericConsistency = numericConsistencyUnknown
		if got := domain.Normalize(candidate); domain.Lattice().Equal(got, domain.Lattice().Bottom()) {
			b.Fatal("satisfiable components became Bottom")
		}
	}
}

func BenchmarkNumericConsistencyOneComponentUpdateAmongTwoThousand(b *testing.B) {
	domain, keys := numericConsistencyTestDomain(b)
	raw := domain.Lattice().Bottom()
	for index := 0; index < 2000; index++ {
		left := RelValueOperand(pathaddr.StateKey(fmt.Sprintf("local:left-%d", index)))
		right := RelValueOperand(pathaddr.StateKey(fmt.Sprintf("local:right-%d", index)))
		raw = raw.WriteDiffConstraint(left, right, int64(index))
	}
	base := domain.Normalize(raw)
	target := pathaddr.StateKey("local:left-0")
	b.ResetTimer()
	for b.Loop() {
		candidate := base.WriteNumFloor(keys, target, 1)
		if got := domain.Normalize(candidate); domain.Lattice().Equal(got, domain.Lattice().Bottom()) {
			b.Fatal("satisfiable local update became Bottom")
		}
	}
}

type numericConsistencyTesting interface {
	Helper()
	Fatal(...any)
}

func numericConsistencyTestDomain(t numericConsistencyTesting) (ProductDomain, *keyspace.KeySpace) {
	t.Helper()
	domain := RegisteredProductDomain(standard.Registry())
	if !domain.Valid() {
		t.Fatal("invalid product domain")
	}
	return domain, keyspace.New()
}
