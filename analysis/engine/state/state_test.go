package state

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/symbol"
	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestBottomReadsProductBottom(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	valueDomain := product.Domain(reg)
	var s State

	if got := s.ReadValue(reg, key.SymbolValue(1)); !valueDomain.Equal(got, valueDomain.Bottom()) {
		t.Fatalf("absent value slot = %s, want product bottom", formatValue(reg, got))
	}
	if got := s.ReadPathKey(reg, ks, pathdom.PathKey("sym1@1.field")); !valueDomain.Equal(got, valueDomain.Bottom()) {
		t.Fatalf("absent path key = %s, want product bottom", formatValue(reg, got))
	}
}

func TestDomainStableAcrossRepeatedConstruction(t *testing.T) {
	reg := standard.Registry()
	top := Domain(reg).Top()
	bottom := Domain(reg).Bottom()
	domain := Domain(reg)

	if !domain.Equal(top, domain.Top()) {
		t.Fatalf("reconstructed state domain did not recognize prior top")
	}
	if !domain.Equal(bottom, domain.Bottom()) {
		t.Fatalf("reconstructed state domain did not recognize prior bottom")
	}
	if !domain.Equal(domain.Join(bottom, top), top) {
		t.Fatalf("reconstructed state domain join(bottom, top) did not produce top")
	}
}

func TestWriteReadValueSlots(t *testing.T) {
	reg := standard.Registry()
	valueDomain := product.Domain(reg)
	symSlot := key.SymbolValue(symbol.ID(10))
	retSlot := key.ReturnSlot(1)
	symValue := presentValue(reg)
	retValue := absentValue(reg)

	s := State{}.
		WriteValue(reg, symSlot, symValue).
		WriteValue(reg, retSlot, retValue)

	if got := s.ReadValue(reg, symSlot); !valueDomain.Equal(got, symValue) {
		t.Fatalf("symbol slot = %s, want %s", formatValue(reg, got), formatValue(reg, symValue))
	}
	if got := s.ReadSymbolValue(reg, symbol.ID(10)); !valueDomain.Equal(got, symValue) {
		t.Fatalf("symbol value = %s, want %s", formatValue(reg, got), formatValue(reg, symValue))
	}
	if got := s.ReadValue(reg, retSlot); !valueDomain.Equal(got, retValue) {
		t.Fatalf("return slot = %s, want %s", formatValue(reg, got), formatValue(reg, retValue))
	}
}

func TestLenFloorStateSemantics(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	stateDomain := Domain(reg)
	pathKey := pathdom.PathKey("sym12@1.items")

	if floor, ok := (State{}).ReadLenFloor(ks, testStateKey(t, pathKey)); ok || floor != 0 {
		t.Fatalf("missing len floor = %d/%v, want absent", floor, ok)
	}
	if got := (State{}).WriteLenFloor(ks, pathaddr.StateKey(""), 2); !stateDomain.Equal(got, State{}) {
		t.Fatalf("empty len-floor path changed state: %s", formatState(reg, ks, got))
	}
	if got := (State{}).WriteLenFloor(ks, testStateKey(t, pathKey), 0); !stateDomain.Equal(got, State{}) {
		t.Fatalf("non-positive len floor changed state: %s", formatState(reg, ks, got))
	}

	withFloor := State{}.WriteLenFloor(ks, testStateKey(t, pathKey), 2)
	if floor, ok := withFloor.ReadLenFloor(ks, testStateKey(t, pathKey)); !ok || floor != 2 {
		t.Fatalf("len floor = %d/%v, want 2/present", floor, ok)
	}
	weaker := withFloor.WriteLenFloor(ks, testStateKey(t, pathKey), 1)
	if !stateDomain.Equal(weaker, withFloor) {
		t.Fatalf("weaker len floor changed state: %s", formatState(reg, ks, weaker))
	}
	stronger := withFloor.WriteLenFloor(ks, testStateKey(t, pathKey), 4)
	if floor, ok := stronger.ReadLenFloor(ks, testStateKey(t, pathKey)); !ok || floor != 4 {
		t.Fatalf("stronger len floor = %d/%v, want 4/present", floor, ok)
	}

	fromBottom := stateDomain.Bottom().WriteLenFloor(ks, testStateKey(t, pathKey), 3)
	if floor, ok := fromBottom.ReadLenFloor(ks, testStateKey(t, pathKey)); !ok || floor != 3 {
		t.Fatalf("bottom write len floor = %d/%v, want 3/present", floor, ok)
	}
	if stateDomain.Equal(fromBottom, stateDomain.Bottom()) {
		t.Fatalf("writing len floor from bottom kept state at lattice bottom")
	}
}

func TestLenFloorInvalidationFollowsPathMutationPrefixes(t *testing.T) {
	ks := keyspace.New()
	root := pathdom.PathKey("sym12@1.items")
	child := pathdom.PathKey("sym12@1.items.child")
	sibling := pathdom.PathKey("sym12@1.itemized")
	aliasRoot := pathdom.PathKey("sym13@1.alias")
	aliasChild := pathdom.PathKey("sym13@1.alias.child")
	proof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: mustStateKey(t, ks, root), Other: mustStateKey(t, ks, aliasRoot)}

	s := State{}.
		WriteLenFloor(ks, testStateKey(t, root), 2).
		WriteLenFloor(ks, testStateKey(t, child), 5).
		WriteLenFloor(ks, testStateKey(t, sibling), 7).
		WriteLenFloor(ks, testStateKey(t, aliasRoot), 3).
		WriteLenFloor(ks, testStateKey(t, aliasChild), 4).
		AddBranchProof(proof)

	out, ok := s.InvalidatePathKeyDescendants(ks, root)
	if !ok {
		t.Fatal("InvalidatePathKeyDescendants rejected root")
	}
	for _, removed := range []pathdom.PathKey{root, child, aliasRoot, aliasChild} {
		if floor, ok := out.ReadLenFloor(ks, testStateKey(t, removed)); ok || floor != 0 {
			t.Fatalf("%s length floor = %d/%v, want cleared", removed, floor, ok)
		}
	}
	if floor, ok := out.ReadLenFloor(ks, testStateKey(t, sibling)); !ok || floor != 7 {
		t.Fatalf("%s length floor = %d/%v, want 7/present", sibling, floor, ok)
	}
	if floor, ok := s.ReadLenFloor(ks, testStateKey(t, root)); !ok || floor != 2 {
		t.Fatalf("original root length floor = %d/%v, want unchanged 2/present", floor, ok)
	}

	out, ok = s.InvalidatePathKeySubtree(ks, root)
	if !ok {
		t.Fatal("InvalidatePathKeySubtree rejected root")
	}
	for _, removed := range []pathdom.PathKey{root, child, aliasRoot, aliasChild} {
		if floor, ok := out.ReadLenFloor(ks, testStateKey(t, removed)); ok || floor != 0 {
			t.Fatalf("%s subtree length floor = %d/%v, want cleared", removed, floor, ok)
		}
	}
	if floor, ok := out.ReadLenFloor(ks, testStateKey(t, sibling)); !ok || floor != 7 {
		t.Fatalf("%s subtree length floor = %d/%v, want 7/present", sibling, floor, ok)
	}
}

func TestTypestateStateLaneTracksOpenClosedAndEscapedResources(t *testing.T) {
	reg := standard.Registry()
	domain := Domain(reg)
	resource := TypestateResource(testStateKey(t, pathdom.PathKey("tx@1")), typestate.Protocol("transaction"))
	obligation := typestate.Obligation{Final: typestate.State("closed")}

	open := State{}.AcquireTypestate(resource, typestate.State("open"), obligation)
	if obligations := open.OpenTypestateObligations(); len(obligations) != 1 ||
		obligations[0].Resource != resource ||
		obligations[0].Current != typestate.State("open") ||
		obligations[0].Obligation != obligation {
		t.Fatalf("open obligations = %#v, want one open transaction", obligations)
	}

	closed := open.TransitionTypestate(resource, typestate.State("open"), typestate.State("closed"))
	if obligations := closed.OpenTypestateObligations(); len(obligations) != 0 {
		t.Fatalf("closed obligations = %#v, want none", obligations)
	}

	escaped := open.EscapeTypestate(resource)
	if obligations := escaped.OpenTypestateObligations(); len(obligations) != 0 {
		t.Fatalf("escaped obligations = %#v, want none", obligations)
	}

	joined := domain.Join(open, closed)
	if obligations := joined.OpenTypestateObligations(); len(obligations) != 1 ||
		obligations[0].Resource != resource ||
		obligations[0].Obligation != obligation {
		t.Fatalf("joined obligations = %#v, want maybe-open transaction obligation", obligations)
	}
}

func TestTypestateResourceUsesValidatedStateKeyIdentity(t *testing.T) {
	target := testStateKey(t, pathdom.PathKey("sym12@1.tx"))
	resource := TypestateResource(target, typestate.Protocol("transaction"))
	if resource.ID != target.String() || resource.Protocol != typestate.Protocol("transaction") {
		t.Fatalf("resource = %#v, want state key %q and protocol transaction", resource, target)
	}
}

func TestTypestateResourceKeyFollowsProvenPathEquality(t *testing.T) {
	ks := keyspace.New()
	tx := pathdom.PathKey("sym10@1.tx")
	alias := pathdom.PathKey("sym11@1.alias")
	proof := pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofPathEqual,
		Path:  mustStateKey(t, ks, tx),
		Other: mustStateKey(t, ks, alias),
	}
	state := State{}.AddBranchProof(proof)

	gotFromTX := state.CanonicalTypestateResourceKey(ks, testStateKey(t, tx))
	gotFromAlias := state.CanonicalTypestateResourceKey(ks, testStateKey(t, alias))
	if gotFromTX == "" || gotFromTX != gotFromAlias {
		t.Fatalf("canonical typestate keys = %q/%q, want same non-empty key", gotFromTX, gotFromAlias)
	}
	if gotFromTX.PathKey() != tx {
		t.Fatalf("canonical typestate key = %q, want stable lowest equivalent key %q", gotFromTX, tx)
	}
}

func TestNumFloorStateSemantics(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	stateDomain := Domain(reg)
	pathKey := pathdom.PathKey("sym13@1.index")

	if floor, ok := (State{}).ReadNumFloor(ks, testStateKey(t, pathKey)); ok || floor != 0 {
		t.Fatalf("missing num floor = %d/%v, want absent", floor, ok)
	}
	if got := (State{}).WriteNumFloor(ks, pathaddr.StateKey(""), 2); !stateDomain.Equal(got, State{}) {
		t.Fatalf("empty num-floor path changed state: %s", formatState(reg, ks, got))
	}
	if snapshot := stateDomain.Bottom().NumFloorsSnapshot(ks); !snapshot.Bottom || len(snapshot.Floors) != 0 {
		t.Fatalf("bottom num-floor snapshot = %#v, want bottom without floors", snapshot)
	}

	withFloor := State{}.WriteNumFloor(ks, testStateKey(t, pathKey), -5)
	if floor, ok := withFloor.ReadNumFloor(ks, testStateKey(t, pathKey)); !ok || floor != -5 {
		t.Fatalf("num floor = %d/%v, want -5/present", floor, ok)
	}
	weaker := withFloor.WriteNumFloor(ks, testStateKey(t, pathKey), -10)
	if !stateDomain.Equal(weaker, withFloor) {
		t.Fatalf("weaker num floor changed state: %s", formatState(reg, ks, weaker))
	}
	stronger := withFloor.WriteNumFloor(ks, testStateKey(t, pathKey), 1)
	if floor, ok := stronger.ReadNumFloor(ks, testStateKey(t, pathKey)); !ok || floor != 1 {
		t.Fatalf("stronger num floor = %d/%v, want 1/present", floor, ok)
	}

	snapshot := stronger.NumFloorsSnapshot(ks)
	if snapshot.Bottom || snapshot.Floors[pathKey] != 1 {
		t.Fatalf("num-floor snapshot = %#v, want path floor 1", snapshot)
	}
	snapshot.Floors[pathKey] = 99
	if floor, _ := stronger.ReadNumFloor(ks, testStateKey(t, pathKey)); floor != 1 {
		t.Fatalf("mutating num-floor snapshot changed state floor to %d", floor)
	}

	cleared := stronger.ClearNumFloor(ks, testStateKey(t, pathKey))
	if floor, ok := cleared.ReadNumFloor(ks, testStateKey(t, pathKey)); ok || floor != 0 {
		t.Fatalf("cleared num floor = %d/%v, want absent", floor, ok)
	}
	if again := cleared.ClearNumFloor(ks, testStateKey(t, pathKey)); !stateDomain.Equal(again, cleared) {
		t.Fatalf("clearing absent num floor changed state: %s", formatState(reg, ks, again))
	}
}

func TestStoreRelationStateMustSemantics(t *testing.T) {
	reg := standard.Registry()
	stateDomain := Domain(reg)
	common := StoreRelation{
		Source: testStateKey(t, pathdom.PathKey("sym20@1.value")),
		Into:   testStateKey(t, pathdom.PathKey("sym21@1.container")),
	}
	leftOnly := StoreRelation{
		Source: testStateKey(t, pathdom.PathKey("sym22@1.value")),
		Into:   testStateKey(t, pathdom.PathKey("sym21@1.container")),
	}
	rightOnly := StoreRelation{
		Source: testStateKey(t, pathdom.PathKey("sym23@1.value")),
		Into:   testStateKey(t, pathdom.PathKey("sym21@1.container")),
	}

	left := State{}.
		AddStoreRelation(common).
		AddStoreRelation(leftOnly)
	right := State{}.
		AddStoreRelation(common).
		AddStoreRelation(rightOnly)

	joined := stateDomain.Join(left, right)
	if !joined.HasStoreRelation(common) {
		t.Fatalf("joined store relations = %#v, want common relation", joined.StoreRelationsSnapshot())
	}
	if joined.HasStoreRelation(leftOnly) || joined.HasStoreRelation(rightOnly) {
		t.Fatalf("joined store relations = %#v, want branch-local relations removed", joined.StoreRelationsSnapshot())
	}
	if snapshot := joined.StoreRelationsSnapshot(); snapshot.Bottom || snapshot.Top || len(snapshot.Relations) != 1 {
		t.Fatalf("joined snapshot = %#v, want exactly one common relation", snapshot)
	}

	fromBottom := stateDomain.Bottom().AddStoreRelation(common)
	if !fromBottom.HasStoreRelation(common) || stateDomain.Equal(fromBottom, stateDomain.Bottom()) {
		t.Fatalf("bottom store relation write did not make reachable state: %#v", fromBottom.StoreRelationsSnapshot())
	}
}

func TestWritesAreImmutable(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	valueDomain := product.Domain(reg)
	slot := key.SymbolValue(symbol.ID(11))
	pathKey := pathdom.PathKey("sym11@1.field")
	present := presentValue(reg)
	absent := absentValue(reg)

	s1 := State{}.
		WriteValue(reg, slot, present).
		WritePathKey(reg, ks, pathKey, present)
	s2 := s1.
		WriteValue(reg, slot, absent).
		WritePathKey(reg, ks, pathKey, absent)

	if got := s1.ReadValue(reg, slot); !valueDomain.Equal(got, present) {
		t.Fatalf("original value slot changed to %s", formatValue(reg, got))
	}
	if got := s1.ReadPathKey(reg, ks, pathKey); !valueDomain.Equal(got, present) {
		t.Fatalf("original path key changed to %s", formatValue(reg, got))
	}
	if got := s2.ReadValue(reg, slot); !valueDomain.Equal(got, absent) {
		t.Fatalf("updated value slot = %s, want absent value", formatValue(reg, got))
	}
	if got := s2.ReadPathKey(reg, ks, pathKey); !valueDomain.Equal(got, absent) {
		t.Fatalf("updated path key = %s, want absent value", formatValue(reg, got))
	}
}

func TestUpdateHelpersReadCurrentAndCanonicalizeBottom(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	valueDomain := product.Domain(reg)
	stateDomain := Domain(reg)
	slot := key.SymbolValue(symbol.ID(12))
	retSlot := 0
	pathKey := pathdom.PathKey("sym12@1.field")
	present := presentValue(reg)
	absent := absentValue(reg)
	bottom := valueDomain.Bottom()

	s1 := State{}.
		WriteValue(reg, slot, present).
		WriteReturnSlot(reg, retSlot, present).
		WritePathKey(reg, ks, pathKey, present)
	s2 := s1.
		UpdateValue(reg, slot, func(got product.Value) product.Value {
			if !valueDomain.Equal(got, present) {
				t.Fatalf("UpdateValue read %s, want present", formatValue(reg, got))
			}
			return bottom
		}).
		UpdateReturnSlot(reg, retSlot, func(got product.Value) product.Value {
			if !valueDomain.Equal(got, present) {
				t.Fatalf("UpdateReturnSlot read %s, want present", formatValue(reg, got))
			}
			return absent
		}).
		UpdatePathKey(reg, ks, pathKey, func(got product.Value) product.Value {
			if !valueDomain.Equal(got, present) {
				t.Fatalf("UpdatePathKey read %s, want present", formatValue(reg, got))
			}
			return bottom
		})

	if got := s1.ReadValue(reg, slot); !valueDomain.Equal(got, present) {
		t.Fatalf("original value slot changed to %s", formatValue(reg, got))
	}
	if got := s1.ReadReturnSlot(reg, retSlot); !valueDomain.Equal(got, present) {
		t.Fatalf("original return slot changed to %s", formatValue(reg, got))
	}
	if got := s1.ReadPathKey(reg, ks, pathKey); !valueDomain.Equal(got, present) {
		t.Fatalf("original path key changed to %s", formatValue(reg, got))
	}
	if got := s2.ReadValue(reg, slot); !valueDomain.Equal(got, bottom) {
		t.Fatalf("updated value slot = %s, want bottom", formatValue(reg, got))
	}
	if got := s2.ReadReturnSlot(reg, retSlot); !valueDomain.Equal(got, absent) {
		t.Fatalf("updated return slot = %s, want absent", formatValue(reg, got))
	}
	if got := s2.ReadPathKey(reg, ks, pathKey); !valueDomain.Equal(got, bottom) {
		t.Fatalf("updated path key = %s, want bottom", formatValue(reg, got))
	}
	if s2.values.hasFinite(slot) {
		t.Fatalf("UpdateValue to bottom kept finite value entry")
	}
	if _, ok := s2.PathRefinementsSnapshot(ks).Refinements[pathKey]; ok {
		t.Fatalf("UpdatePathKey to bottom kept finite path entry")
	}
	if !stateDomain.Equal(State{}.WriteReturnSlot(reg, retSlot, absent), State{}.WriteValue(reg, key.ReturnSlot(retSlot), absent)) {
		t.Fatalf("return-slot helper does not use key.ReturnSlot spelling")
	}
}

func TestDomainPointwiseOperations(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	valueDomain := product.Domain(reg)
	stateDomain := Domain(reg)
	present := presentValue(reg)
	absent := absentValue(reg)
	valueSlot := key.SymbolValue(symbol.ID(21))
	retSlot := key.ReturnSlot(0)
	pathKey := pathdom.PathKey("sym21@2.field")
	otherPathKey := pathdom.PathKey("$0.item")

	a := State{}.
		WriteValue(reg, valueSlot, present).
		WritePathKey(reg, ks, pathKey, present)
	b := State{}.
		WriteValue(reg, valueSlot, absent).
		WriteValue(reg, retSlot, present).
		WritePathKey(reg, ks, pathKey, absent).
		WritePathKey(reg, ks, otherPathKey, present)

	joined := stateDomain.Join(a, b)
	if got := joined.ReadValue(reg, valueSlot); !valueDomain.Equal(got, product.Top()) {
		t.Fatalf("joined shared value slot = %s, want top", formatValue(reg, got))
	}
	if got := joined.ReadValue(reg, retSlot); !valueDomain.Equal(got, present) {
		t.Fatalf("joined disjoint value slot = %s, want present", formatValue(reg, got))
	}
	if got := joined.ReadPathKey(reg, ks, pathKey); !valueDomain.Equal(got, product.Top()) {
		t.Fatalf("joined shared path key = %s, want top", formatValue(reg, got))
	}
	if got := joined.ReadPathKey(reg, ks, otherPathKey); !valueDomain.Equal(got, product.Bottom(reg)) {
		t.Fatalf("joined disjoint path key = %s, want bottom (dropped by must join)", formatValue(reg, got))
	}

	if widened := stateDomain.Widen(a, b); !stateDomain.Equal(widened, joined) {
		t.Fatalf("Widen differs from Join: got %s, want %s", formatState(reg, ks, widened), formatState(reg, ks, joined))
	}
	if !stateDomain.LessOrEq(a, joined) || !stateDomain.LessOrEq(b, joined) {
		t.Fatalf("Join is not an upper bound: a=%s b=%s joined=%s",
			formatState(reg, ks, a), formatState(reg, ks, b), formatState(reg, ks, joined))
	}
	if stateDomain.LessOrEq(joined, a) {
		t.Fatalf("joined state unexpectedly <= left operand")
	}
	if stateDomain.Equal(a, b) {
		t.Fatalf("states with different pointwise lanes compare equal")
	}
	if !stateDomain.Equal(a, a.Snapshot()) {
		t.Fatalf("Snapshot should preserve state equality")
	}
}

func TestStateLatticeLaws(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	d := Domain(reg)
	suite := latticelaws.LawSuite[State]{
		Name:   "state.State",
		Domain: d,
		Sample: stateLawSample(reg, ks),
		Format: stateLawFormat(reg, ks),
	}
	suite.Run(t)
}

func TestStateOrderConsistencyAndJoinMonotonicity(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	d := Domain(reg)
	sample := stateLawSample(reg, ks)

	for _, a := range sample {
		for _, b := range sample {
			eq := d.Equal(a, b)
			le := d.LessOrEq(a, b)
			ge := d.LessOrEq(b, a)
			if eq != (le && ge) {
				t.Fatalf("equality/order mismatch: a=%s b=%s equal=%v less-or-eq=%v reverse=%v",
					stateLawFormat(reg, ks)(a), stateLawFormat(reg, ks)(b), eq, le, ge)
			}
		}
	}

	for _, a := range sample {
		for _, b := range sample {
			if !d.LessOrEq(a, b) {
				continue
			}
			for _, c := range sample {
				left := d.Join(a, c)
				right := d.Join(b, c)
				if !d.LessOrEq(left, right) {
					t.Fatalf("join monotonicity failed: %s ⊑ %s but Join(%s,%s)=%s ⊑ Join(%s,%s)=%s does not hold",
						stateLawFormat(reg, ks)(a), stateLawFormat(reg, ks)(b),
						stateLawFormat(reg, ks)(a), stateLawFormat(reg, ks)(c), stateLawFormat(reg, ks)(left),
						stateLawFormat(reg, ks)(b), stateLawFormat(reg, ks)(c), stateLawFormat(reg, ks)(right))
				}
				left = d.Join(c, a)
				right = d.Join(c, b)
				if !d.LessOrEq(left, right) {
					t.Fatalf("join monotonicity failed on left argument: %s ⊑ %s but Join(%s,%s)=%s ⊑ Join(%s,%s)=%s does not hold",
						stateLawFormat(reg, ks)(a), stateLawFormat(reg, ks)(b),
						stateLawFormat(reg, ks)(c), stateLawFormat(reg, ks)(a), stateLawFormat(reg, ks)(left),
						stateLawFormat(reg, ks)(c), stateLawFormat(reg, ks)(b), stateLawFormat(reg, ks)(right))
				}
			}
		}
	}
}

func TestStateCloneIndependenceAcrossLanes(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	fx := stateLawFixtureFor(reg, ks)

	original := State{}.
		WriteValue(reg, fx.valueSlot, fx.present).
		WritePathKey(reg, ks, fx.pathKey, fx.present).
		WritePathStaticMember(ks, fx.staticKey, fx.present).
		WriteDynamicIndexFact(reg, fx.dynamicKey, fx.dynamicFact).
		WriteHeapTableObject(reg, fx.heapID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: fx.present,
			StaticMembers: map[keyspace.Key]product.Value{
				fx.staticHeapKey: fx.present,
			},
		})).
		WriteEffectDelta(fx.effectKey, fx.effectDelta).
		WritePlacement(fx.escapeID, placement.Stack).
		WriteLenFloor(ks, testStateKey(t, fx.pathKey), 2).
		WriteNumFloor(ks, testStateKey(t, fx.pathKey), 3).
		WriteDiffConstraint(pathdom.PathKey("clone-i"), pathdom.PathKey("clone-j"), -1).
		AcquireTypestate(
			TypestateResource(testStateKey(t, pathdom.PathKey("clone-tx")), typestate.Protocol("transaction")),
			typestate.State("open"),
			typestate.Obligation{Final: typestate.State("closed")},
		).
		AddStoreRelation(StoreRelation{Source: testStateKey(t, pathdom.PathKey("clone-src")), Into: testStateKey(t, pathdom.PathKey("clone-dst"))}).
		FreezeTable(fx.freezeID).
		AddChannelSelectFact(fx.channelFact).
		AddBranchProof(fx.proof)

	cloneOnlyProof := pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofPathNotEqual,
		Path:  mustStateKey(t, ks, fx.pathKey),
		Other: mustStateKey(t, ks, pathdom.PathKey("sym201@1.other")),
	}
	cloneOnlyFrozenID := identity.ID{Kind: "table", Site: "clone-only-freeze", Index: 1}
	cloneOnlyStoreRelation := StoreRelation{Source: testStateKey(t, pathdom.PathKey("clone-only-src")), Into: testStateKey(t, pathdom.PathKey("clone-dst"))}
	typestateResource := TypestateResource(testStateKey(t, pathdom.PathKey("clone-tx")), typestate.Protocol("transaction"))
	clone := original.Snapshot()
	clone = clone.WriteValue(reg, fx.valueSlot, fx.absent)
	clone = clone.WritePathKey(reg, ks, fx.pathKey, fx.absent)
	clone = clone.WritePathStaticMember(ks, fx.staticKey, fx.absent)
	clone = clone.AddBranchProof(cloneOnlyProof)
	clone = clone.WriteDynamicIndexFact(reg, fx.dynamicKey, dynamicindex.Bottom(reg))
	clone = clone.WriteHeapTableObject(reg, fx.heapID, heapidentity.BottomObject(reg))
	clone = clone.WriteEffectDelta(fx.effectKey, effectdelta.Bottom(reg))
	clone = clone.WritePlacement(fx.escapeID, placement.Unknown)
	clone = clone.WriteLenFloor(ks, testStateKey(t, fx.pathKey), 5)
	clone = clone.WriteNumFloor(ks, testStateKey(t, fx.pathKey), 7)
	clone = clone.WriteDiffConstraint(pathdom.PathKey("clone-extra"), pathdom.PathKey("clone-j"), 0)
	clone = clone.TransitionTypestate(typestateResource, typestate.State("open"), typestate.State("closed"))
	clone = clone.AddStoreRelation(cloneOnlyStoreRelation)
	clone = clone.FreezeTable(cloneOnlyFrozenID)
	clone = clone.AddChannelSelectFact(channelselectfact.Fact{
		Select: "clone-only",
		Kind:   channelselectfact.FactCase,
		Case:   fx.pathKey,
		Index:  7,
	})

	if got := original.ReadValue(reg, fx.valueSlot); !product.Equal(reg, got, fx.present) {
		t.Fatalf("original value slot mutated through clone: %s", formatValue(reg, got))
	}
	if got := original.ReadPathKey(reg, ks, fx.pathKey); !product.Equal(reg, got, fx.present) {
		t.Fatalf("original path key mutated through clone: %s", formatValue(reg, got))
	}
	if got, ok := original.ReadPathStaticMember(ks, fx.staticKey); !ok || !product.Equal(reg, got, fx.present) {
		t.Fatalf("original static member mutated through clone: %s ok=%v", formatValue(reg, got), ok)
	}
	if got := original.ReadDynamicIndexFact(reg, fx.dynamicKey); !dynamicindex.Domain(reg).Equal(got, fx.dynamicFact) {
		t.Fatalf("original dynamic index mutated through clone: %#v", got)
	}
	if got := original.ReadHeapTableObject(reg, fx.heapID); !product.Equal(reg, got.Root(), fx.present) ||
		!staticMemberEqual(reg, got, fx.staticHeapKey, fx.present) {
		t.Fatalf("original heap object mutated through clone: %#v", got)
	}
	if got := original.ReadEffectDelta(fx.effectKey); !effectdelta.Domain(reg).Equal(got, fx.effectDelta) {
		t.Fatalf("original effect delta mutated through clone: %#v", got)
	}
	if got := original.ReadPlacement(fx.escapeID); got != placement.Stack {
		t.Fatalf("original placement mutated through clone: %v", got)
	}
	if got, ok := original.ReadLenFloor(ks, testStateKey(t, fx.pathKey)); !ok || got != 2 {
		t.Fatalf("original len floor mutated through clone: %d/%v", got, ok)
	}
	if got, ok := original.ReadNumFloor(ks, testStateKey(t, fx.pathKey)); !ok || got != 3 {
		t.Fatalf("original num floor mutated through clone: %d/%v", got, ok)
	}
	if constraints := original.RelConstraints().Constraints; len(constraints) != 1 ||
		constraints[0].A != pathdom.PathKey("clone-i") ||
		constraints[0].C != pathdom.PathKey("clone-j") ||
		constraints[0].K != -1 {
		t.Fatalf("original relational constraints mutated through clone: %#v", constraints)
	}
	if obligations := original.OpenTypestateObligations(); len(obligations) != 1 ||
		obligations[0].Resource != typestateResource ||
		obligations[0].Current != typestate.State("open") {
		t.Fatalf("original typestate mutated through clone: %#v", obligations)
	}
	if !original.HasStoreRelation(StoreRelation{Source: testStateKey(t, pathdom.PathKey("clone-src")), Into: testStateKey(t, pathdom.PathKey("clone-dst"))}) ||
		original.HasStoreRelation(cloneOnlyStoreRelation) {
		t.Fatalf("original store-relation lane mutated through clone")
	}
	if !original.IsTableFrozen(fx.freezeID) || original.IsTableFrozen(cloneOnlyFrozenID) {
		t.Fatalf("original frozen-table lane mutated through clone")
	}
	if !original.HasChannelSelectFact(fx.channelFact) {
		t.Fatalf("original channel-select fact mutated through clone")
	}
	if !original.HasBranchProof(fx.proof) {
		t.Fatalf("original branch proof mutated through clone")
	}

	if got := clone.ReadValue(reg, fx.valueSlot); !product.Equal(reg, got, fx.absent) {
		t.Fatalf("clone value slot = %s, want absent", formatValue(reg, got))
	}
	if got := clone.ReadPathKey(reg, ks, fx.pathKey); !product.Equal(reg, got, fx.absent) {
		t.Fatalf("clone path key = %s, want absent", formatValue(reg, got))
	}
	if got, ok := clone.ReadPathStaticMember(ks, fx.staticKey); !ok || !product.Equal(reg, got, fx.absent) {
		t.Fatalf("clone static member = %s ok=%v, want absent", formatValue(reg, got), ok)
	}
	if got := clone.ReadDynamicIndexFact(reg, fx.dynamicKey); dynamicindex.Domain(reg).Equal(got, fx.dynamicFact) {
		t.Fatalf("clone dynamic index did not change")
	}
	if got := clone.ReadHeapTableObject(reg, fx.heapID); product.Equal(reg, got.Root(), fx.present) {
		t.Fatalf("clone heap object root did not change")
	}
	if got := clone.ReadEffectDelta(fx.effectKey); effectdelta.Domain(reg).Equal(got, fx.effectDelta) {
		t.Fatalf("clone effect delta did not change")
	}
	if got := clone.ReadPlacement(fx.escapeID); got != placement.Unknown {
		t.Fatalf("clone placement = %v, want unknown", got)
	}
	if got, ok := clone.ReadLenFloor(ks, testStateKey(t, fx.pathKey)); !ok || got != 5 {
		t.Fatalf("clone len floor = %d/%v, want 5", got, ok)
	}
	if got, ok := clone.ReadNumFloor(ks, testStateKey(t, fx.pathKey)); !ok || got != 7 {
		t.Fatalf("clone num floor = %d/%v, want 7", got, ok)
	}
	if constraints := clone.RelConstraints().Constraints; len(constraints) != 2 {
		t.Fatalf("clone relational constraints = %#v, want original plus clone-only", constraints)
	}
	if obligations := clone.OpenTypestateObligations(); len(obligations) != 0 {
		t.Fatalf("clone typestate obligations = %#v, want closed", obligations)
	}
	if !clone.HasStoreRelation(StoreRelation{Source: testStateKey(t, pathdom.PathKey("clone-src")), Into: testStateKey(t, pathdom.PathKey("clone-dst"))}) ||
		!clone.HasStoreRelation(cloneOnlyStoreRelation) {
		t.Fatalf("clone store-relation updates did not stick")
	}
	if !clone.IsTableFrozen(fx.freezeID) || !clone.IsTableFrozen(cloneOnlyFrozenID) {
		t.Fatalf("clone frozen-table update did not stick")
	}
	if !clone.HasChannelSelectFact(fx.channelFact) || !clone.HasChannelSelectFact(channelselectfact.Fact{
		Select: "clone-only",
		Kind:   channelselectfact.FactCase,
		Case:   fx.pathKey,
		Index:  7,
	}) {
		t.Fatalf("clone channel-select update did not stick")
	}
	if !clone.HasBranchProof(fx.proof) || !clone.HasBranchProof(cloneOnlyProof) {
		t.Fatalf("clone branch proof updates did not stick")
	}
}

func TestStateBottomWritesRemoveExplicitBottomEntries(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	valueDomain := product.Domain(reg)
	fx := stateLawFixtureFor(reg, ks)

	state := State{}.
		WriteValue(reg, fx.valueSlot, fx.present).
		WritePathKey(reg, ks, fx.pathKey, fx.present).
		WriteDynamicIndexFact(reg, fx.dynamicKey, fx.dynamicFact).
		WriteHeapTableObject(reg, fx.heapID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: fx.present,
			StaticMembers: map[keyspace.Key]product.Value{
				fx.staticHeapKey: fx.present,
			},
		})).
		WriteEffectDelta(fx.effectKey, fx.effectDelta).
		WritePlacement(fx.escapeID, placement.Stack)

	state = state.WriteValue(reg, fx.valueSlot, valueDomain.Bottom())
	state = state.WritePathKey(reg, ks, fx.pathKey, valueDomain.Bottom())
	state = state.WriteDynamicIndexFact(reg, fx.dynamicKey, dynamicindex.Bottom(reg))
	state = state.WriteHeapTableObject(reg, fx.heapID, heapidentity.BottomObject(reg))
	state = state.WriteEffectDelta(fx.effectKey, effectdelta.Bottom(reg))
	state = state.WritePlacement(fx.escapeID, placement.Bottom)

	if state.values.hasFinite(fx.valueSlot) {
		t.Fatalf("value slot kept explicit bottom entry")
	}
	if _, ok := state.PathRefinementsSnapshot(ks).Refinements[fx.pathKey]; ok {
		t.Fatalf("path refinement kept explicit bottom entry")
	}
	if state.dynamicIndex.hasFinite(fx.dynamicKey) {
		t.Fatalf("dynamic index kept explicit bottom entry")
	}
	if state.heapTableIdentity.hasFinite(fx.heapID) {
		t.Fatalf("heap identity kept explicit bottom entry")
	}
	if state.effectDeltas.hasFinite(fx.effectKey) {
		t.Fatalf("effect delta kept explicit bottom entry")
	}
	if state.placement.hasFinite(fx.escapeID) {
		t.Fatalf("placement kept explicit bottom entry")
	}
}

func TestExplicitBottomEntriesCanonicalizeToAbsence(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	valueDomain := product.Domain(reg)
	stateDomain := Domain(reg)
	bottom := valueDomain.Bottom()
	explicit := State{
		values: valueLane{mapLane[key.Value, product.Value]{values: map[key.Value]product.Value{
			key.ReturnSlot(0): bottom,
		}}},
	}

	if !stateDomain.Equal(explicit, State{}) {
		t.Fatalf("explicit bottom entries should equal absence")
	}
	joined := stateDomain.Join(explicit, State{})
	if !stateDomain.Equal(joined, State{}) {
		t.Fatalf("Join should canonicalize bottom entries away, got %s", formatState(reg, ks, joined))
	}
	if joined.values.hasFinite(key.ReturnSlot(0)) {
		t.Fatalf("Join kept bottom entry")
	}

	withValue := State{}.WriteValue(reg, key.ReturnSlot(0), presentValue(reg))
	withoutValue := withValue.WriteValue(reg, key.ReturnSlot(0), bottom)
	if !stateDomain.Equal(withoutValue, State{}) {
		t.Fatalf("writing bottom should delete the value entry, got %s", formatState(reg, ks, withoutValue))
	}
}

func TestWritesFromStateBottomProduceReachableState(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	stateDomain := Domain(reg)
	bottom := stateDomain.Bottom()
	empty := State{}
	if stateDomain.Equal(bottom, empty) {
		t.Fatalf("lattice bottom must stay distinct from reachable empty for must-fact lanes")
	}

	slot := key.SymbolValue(symbol.ID(65))
	pathKey := pathdom.PathKey("sym65@1.member")
	dynamicKey := dynamicindex.Key{Table: mustStateKey(t, ks, pathdom.PathKey("sym65@1.table")), Site: "dyn"}
	heapID := identity.ID{Kind: "table", Site: "bottom-write", Index: 1}
	proof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathPresence, Path: mustStateKey(t, ks, pathKey), Presence: presence.Present()}
	implication := pathevidence.PathPresenceImplication{
		Trigger:         mustStateKey(t, ks, pathKey),
		TriggerPresence: presence.Absent(),
		Target:          mustStateKey(t, ks, pathdom.PathKey("sym65@1.value")),
		TargetPresence:  presence.Present(),
	}
	effectKey := effectdelta.Key{Target: mustStateKey(t, ks, pathdom.PathKey("sym65@1.table")), Site: "effect", Kind: effectdelta.Mutation}
	channel := channelselectfact.Fact{Select: "select-bottom", Kind: channelselectfact.FactSelect, Result: pathKey}
	escapeID := identity.ID{Kind: "table", Site: "escape-bottom", Index: 1}
	freezeID := identity.ID{Kind: "table", Site: "freeze-bottom", Index: 1}
	present := presentValue(reg)

	dynamicFact := dynamicindex.Fact{
		KeyPresence: presence.Present(),
		KeyValue:    present,
		Value:       present,
		Admission:   dynamicindex.AdmissionAdmitted,
	}
	effectDelta := effectdelta.Value{Before: present, After: present, Change: effectdelta.ChangeChanged}

	cases := []struct {
		name  string
		state State
	}{
		{"value", bottom.WriteValue(reg, slot, present)},
		{"path", bottom.WritePathKey(reg, ks, pathKey, present)},
		{"static-member", bottom.WritePathStaticMember(ks, pathKey, present)},
		{"dynamic-index", bottom.WriteDynamicIndexFact(reg, dynamicKey, dynamicFact)},
		{"heap-table", bottom.WriteHeapTableObject(reg, heapID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: present}))},
		{"branch-proof", bottom.AddBranchProof(proof)},
		{"path-presence-implication", bottom.AddPathPresenceImplication(implication)},
		{"effect-delta", bottom.WriteEffectDelta(effectKey, effectDelta)},
		{"channel-select", bottom.AddChannelSelectFact(channel)},
		{"placement", bottom.WritePlacement(escapeID, placement.Stack)},
		{"frozen-table", bottom.FreezeTable(freezeID)},
	}
	for _, tc := range cases {
		if tc.state.pathEvidence.StaticMembersBottom() ||
			tc.state.pathEvidence.ProofsBottom() ||
			tc.state.pathEvidence.PathPresenceImplicationsBottom() ||
			tc.state.ChannelSelectFactsSnapshot().Bottom ||
			tc.state.frozenTables.bottom {
			t.Fatalf("%s write left partial must-lane bottom: %#v", tc.name, tc.state)
		}
		if !stateDomain.LessOrEq(bottom, tc.state) {
			t.Fatalf("%s write did not move upward from lattice bottom", tc.name)
		}
	}
}

func TestPathPresenceImplicationsUseMustJoinAndInvalidate(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	stateDomain := Domain(reg)
	errKey := mustStateKey(t, ks, pathdom.PathKey("sym101@1.err"))
	valueKey := mustStateKey(t, ks, pathdom.PathKey("sym101@1.value"))
	common := pathevidence.PathPresenceImplication{
		Trigger:         errKey,
		TriggerPresence: presence.Absent(),
		Target:          valueKey,
		TargetPresence:  presence.Present(),
	}
	leftOnly := pathevidence.PathPresenceImplication{
		Trigger:         errKey,
		TriggerPresence: presence.Present(),
		Target:          valueKey,
		TargetPresence:  presence.Absent(),
	}
	rightOnly := pathevidence.PathPresenceImplication{
		Trigger:         valueKey,
		TriggerPresence: presence.Absent(),
		Target:          errKey,
		TargetPresence:  presence.Present(),
	}
	left := State{}.AddPathPresenceImplication(common).AddPathPresenceImplication(leftOnly)
	right := State{}.AddPathPresenceImplication(common).AddPathPresenceImplication(rightOnly)

	if !left.HasPathPresenceImplication(common) {
		t.Fatalf("missing inserted path-presence implication")
	}
	if !stateDomain.Equal(stateDomain.Join(stateDomain.Bottom(), left), left) {
		t.Fatalf("state bottom should be join identity for path-presence implications")
	}
	joined := stateDomain.Join(left, right)
	if !joined.HasPathPresenceImplication(common) {
		t.Fatalf("common path-presence implication was dropped")
	}
	if joined.HasPathPresenceImplication(leftOnly) || joined.HasPathPresenceImplication(rightOnly) {
		t.Fatalf("disjoint path-presence implication survived must join")
	}
	if widened := stateDomain.Widen(left, right); !stateDomain.Equal(widened, joined) {
		t.Fatalf("path-presence implication widen differs from join")
	}
	clone := left.Snapshot().AddPathPresenceImplication(rightOnly)
	if left.HasPathPresenceImplication(rightOnly) || !clone.HasPathPresenceImplication(rightOnly) {
		t.Fatalf("path-presence implication clone write mutated original or missed clone")
	}

	out, ok := left.InvalidatePathKeySubtree(ks, pathdom.PathKey("sym101@1.err"))
	if !ok {
		t.Fatal("InvalidatePathKeySubtree rejected trigger path")
	}
	if out.HasPathPresenceImplication(common) || out.HasPathPresenceImplication(leftOnly) {
		t.Fatalf("trigger path-presence implication survived trigger invalidation")
	}

	out, ok = left.InvalidatePathKeySubtree(ks, pathdom.PathKey("sym101@1.value"))
	if !ok {
		t.Fatal("InvalidatePathKeySubtree rejected target path")
	}
	if out.HasPathPresenceImplication(common) || out.HasPathPresenceImplication(leftOnly) {
		t.Fatalf("target path-presence implication survived target invalidation")
	}
}

func TestPathStaticMembersAndRefinementsUseMustJoin(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	valueDomain := product.Domain(reg)
	stateDomain := Domain(reg)
	common := pathdom.PathKey("sym70@1.shared")
	leftOnly := pathdom.PathKey("sym70@1.left")
	rightOnly := pathdom.PathKey("sym70@1.right")
	present := presentValue(reg)
	absent := absentValue(reg)

	left := State{}.
		WritePathKey(reg, ks, leftOnly, present).
		WritePathStaticMember(ks, common, present).
		WritePathStaticMember(ks, leftOnly, present)
	right := State{}.
		WritePathKey(reg, ks, rightOnly, present).
		WritePathStaticMember(ks, common, absent).
		WritePathStaticMember(ks, rightOnly, present)

	joined := stateDomain.Join(left, right)
	if got := joined.ReadPathKey(reg, ks, leftOnly); !valueDomain.Equal(got, product.Bottom(reg)) {
		t.Fatalf("left-only refinement survived must join: %s", formatValue(reg, got))
	}
	if got := joined.ReadPathKey(reg, ks, rightOnly); !valueDomain.Equal(got, product.Bottom(reg)) {
		t.Fatalf("right-only refinement survived must join: %s", formatValue(reg, got))
	}
	if got, ok := joined.ReadPathStaticMember(ks, common); !ok || !valueDomain.Equal(got, product.Top()) {
		t.Fatalf("joined static member = %s ok=%v, want top common fact", formatValue(reg, got), ok)
	}
	if _, ok := joined.ReadPathStaticMember(ks, leftOnly); ok {
		t.Fatalf("left-only static member survived must join")
	}
	if _, ok := joined.ReadPathStaticMember(ks, rightOnly); ok {
		t.Fatalf("right-only static member survived must join")
	}
	if widened := stateDomain.Widen(left, right); !stateDomain.Equal(widened, joined) {
		t.Fatalf("static member widen differs from join")
	}
	if !stateDomain.LessOrEq(left, joined) || stateDomain.LessOrEq(joined, left) {
		t.Fatalf("static member order should move toward fewer/weaker must facts")
	}
	if !stateDomain.Equal(stateDomain.Join(stateDomain.Bottom(), left), left) {
		t.Fatalf("state bottom should be join identity for static members")
	}

	clone := left.Snapshot().WritePathStaticMember(ks, common, absent)
	if got, _ := left.ReadPathStaticMember(ks, common); !valueDomain.Equal(got, present) {
		t.Fatalf("static member clone write mutated original: %s", formatValue(reg, got))
	}
	if got, _ := clone.ReadPathStaticMember(ks, common); !valueDomain.Equal(got, absent) {
		t.Fatalf("static member clone write = %s, want absent", formatValue(reg, got))
	}
}

func TestDynamicIndexKeysPointwiseFacts(t *testing.T) {
	reg := standard.Registry()
	valueDomain := product.Domain(reg)
	stateDomain := Domain(reg)
	ks := keyspace.New()
	tableKey := mustStateKey(t, ks, pathdom.PathKey("sym80@1.table"))
	common := dynamicindex.Key{Table: tableKey, Site: "common"}
	leftOnly := dynamicindex.Key{Table: tableKey, Site: "left"}
	presentFact := dynamicindex.Fact{
		KeyPresence: presence.Present(),
		KeyValue:    presentValue(reg),
		Value:       presentValue(reg),
		Admission:   dynamicindex.AdmissionAdmitted,
	}
	absentFact := dynamicindex.Fact{
		KeyPresence: presence.Absent(),
		KeyValue:    absentValue(reg),
		Value:       absentValue(reg),
		Admission:   dynamicindex.AdmissionRejected,
	}

	if got := (State{}).ReadDynamicIndexFact(reg, common); !dynamicindex.Domain(reg).Equal(got, dynamicindex.Bottom(reg)) {
		t.Fatalf("empty dynamic index fact = %#v, want bottom", got)
	}

	left := State{}.
		WriteDynamicIndexFact(reg, common, presentFact).
		WriteDynamicIndexFact(reg, leftOnly, presentFact)
	right := State{}.WriteDynamicIndexFact(reg, common, absentFact)
	same := State{}.WriteDynamicIndexFact(reg, common, presentFact)
	if !stateDomain.Equal(State{}.WriteDynamicIndexFact(reg, common, presentFact), same) {
		t.Fatalf("equal dynamic index facts compare different")
	}
	if !stateDomain.Equal(stateDomain.Join(stateDomain.Bottom(), left), left) {
		t.Fatalf("state bottom should be join identity for dynamic index facts")
	}

	joined := stateDomain.Join(left, right)
	got := joined.ReadDynamicIndexFact(reg, common)
	if !presence.Equal(got.KeyPresence, presence.Maybe()) ||
		!valueDomain.Equal(got.KeyValue, product.Top()) ||
		!valueDomain.Equal(got.Value, product.Top()) ||
		got.Admission != dynamicindex.AdmissionUnknown {
		t.Fatalf("joined dynamic fact = %#v, want joined key/value/admission atoms", got)
	}
	if got := joined.ReadDynamicIndexFact(reg, leftOnly); !dynamicindex.Domain(reg).Equal(got, presentFact) {
		t.Fatalf("disjoint dynamic index fact did not survive pointwise join: %#v", got)
	}
	if widened := stateDomain.Widen(left, right); !stateDomain.Equal(widened, joined) {
		t.Fatalf("dynamic index widen differs from join")
	}
	if !stateDomain.LessOrEq(left, joined) || stateDomain.LessOrEq(joined, left) {
		t.Fatalf("dynamic index order should be pointwise")
	}

	clone := left.Snapshot().WriteDynamicIndexFact(reg, common, absentFact)
	if got := left.ReadDynamicIndexFact(reg, common); !dynamicindex.Domain(reg).Equal(got, presentFact) {
		t.Fatalf("dynamic index clone write mutated original: %#v", got)
	}
	if got := clone.ReadDynamicIndexFact(reg, common); !dynamicindex.Domain(reg).Equal(got, absentFact) {
		t.Fatalf("dynamic index clone write = %#v, want absent fact", got)
	}
}

func TestHeapTableIdentityFacadeReadWriteAndCopy(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	valueDomain := product.Domain(reg)
	stateDomain := Domain(reg)
	id := identity.ID{Kind: "table", Site: "alloc", Index: 1}
	staticCommon := heapStaticKey(t, ks, "sym90@1.table.name")
	dynCommon := dynamicindex.Key{Table: mustStateKey(t, ks, pathdom.PathKey("sym90@1.table")), Site: "dyn"}
	present := presentValue(reg)
	absent := absentValue(reg)

	object := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: present,
		StaticMembers: map[keyspace.Key]product.Value{
			staticCommon: present,
		},
		DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
			dynCommon: {
				KeyPresence: presence.Present(),
				KeyValue:    present,
				Value:       present,
				Admission:   dynamicindex.AdmissionAdmitted,
			},
		},
	})

	if got := (State{}).ReadHeapTableObject(reg, id); !heapidentity.ObjectDomain(reg).Equal(got, heapidentity.BottomObject(reg)) {
		t.Fatalf("empty heap object = %#v, want bottom", got)
	}

	written := State{}.WriteHeapTableObject(reg, id, object)
	if !stateDomain.Equal(stateDomain.Join(stateDomain.Bottom(), written), written) {
		t.Fatalf("state bottom should be join identity for heap table identity")
	}
	if got := written.ReadHeapTableObject(reg, id); !valueDomain.Equal(got.Root(), present) {
		t.Fatalf("heap object root = %s, want present", formatValue(reg, got.Root()))
	}
	read := written.ReadHeapTableObject(reg, id)
	readStatic := read.StaticMembers()
	readDynamic := read.DynamicIndexFacts()
	readStatic[staticCommon] = absent
	readDynamic[dynCommon] = dynamicindex.Fact{Admission: dynamicindex.AdmissionRejected}
	again := written.ReadHeapTableObject(reg, id)
	if got, ok := again.StaticMember(staticCommon); !ok || !valueDomain.Equal(got, present) {
		t.Fatalf("heap object read exposed mutable static members")
	}
	if got, ok := again.DynamicIndexFact(dynCommon); !ok || got.Admission != dynamicindex.AdmissionAdmitted {
		t.Fatalf("heap object read exposed mutable dynamic facts")
	}
}

func TestBranchProofsUseMustJoin(t *testing.T) {
	ks := keyspace.New()
	stateDomain := Domain(standard.Registry())
	common := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathPresence, Path: mustStateKey(t, ks, pathdom.PathKey("sym100@1.err")), Presence: presence.Present()}
	leftOnly := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: mustStateKey(t, ks, pathdom.PathKey("sym100@1.a")), Other: mustStateKey(t, ks, pathdom.PathKey("sym100@1.b"))}
	rightOnly := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathNotEqual, Path: mustStateKey(t, ks, pathdom.PathKey("sym100@1.a")), Other: mustStateKey(t, ks, pathdom.PathKey("sym100@1.c"))}
	left := State{}.AddBranchProof(common).AddBranchProof(leftOnly)
	right := State{}.AddBranchProof(common).AddBranchProof(rightOnly)

	if !left.HasBranchProof(common) || !stateDomain.Equal(State{}.AddBranchProof(common), State{}.AddBranchProof(common)) {
		t.Fatalf("branch proof empty/equality behavior failed")
	}
	if !stateDomain.Equal(stateDomain.Join(stateDomain.Bottom(), left), left) {
		t.Fatalf("state bottom should be join identity for branch proofs")
	}
	joined := stateDomain.Join(left, right)
	if !joined.HasBranchProof(common) {
		t.Fatalf("common branch proof was dropped")
	}
	if joined.HasBranchProof(leftOnly) || joined.HasBranchProof(rightOnly) {
		t.Fatalf("disjoint branch proof survived must join")
	}
	if widened := stateDomain.Widen(left, right); !stateDomain.Equal(widened, joined) {
		t.Fatalf("branch proof widen differs from join")
	}
	if !stateDomain.LessOrEq(left, joined) || stateDomain.LessOrEq(joined, left) {
		t.Fatalf("branch proof order should move toward fewer must proofs")
	}
	clone := left.Snapshot().AddBranchProof(rightOnly)
	if left.HasBranchProof(rightOnly) || !clone.HasBranchProof(rightOnly) {
		t.Fatalf("branch proof clone write mutated original or missed clone")
	}
}

func TestEquivalentPathKeysFollowEqualityProofs(t *testing.T) {
	ks := keyspace.New()
	s := State{}.
		AddBranchProof(pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathEqual,
			Path:  mustStateKey(t, ks, pathdom.PathKey("sym10@1")),
			Other: mustStateKey(t, ks, pathdom.PathKey("sym20@1")),
		}).
		AddBranchProof(pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathEqual,
			Path:  mustStateKey(t, ks, pathdom.PathKey("sym20@1.child")),
			Other: mustStateKey(t, ks, pathdom.PathKey("sym30@1.leaf")),
		}).
		AddBranchProof(pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathNotEqual,
			Path:  mustStateKey(t, ks, pathdom.PathKey("sym10@1")),
			Other: mustStateKey(t, ks, pathdom.PathKey("sym40@1")),
		})

	got := s.EquivalentPathKeys(ks, pathdom.PathKey("sym10@1.child.name"))
	want := []pathdom.PathKey{
		pathdom.PathKey("sym20@1.child.name"),
		pathdom.PathKey("sym30@1.leaf.name"),
	}
	if len(got) != len(want) {
		t.Fatalf("EquivalentPathKeys len = %d (%#v), want %d (%#v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("EquivalentPathKeys[%d] = %s, want %s (all %#v)", i, got[i], want[i], got)
		}
	}

	if got := s.EquivalentPathKeys(ks, pathdom.PathKey("sym99@1.child")); len(got) != 0 {
		t.Fatalf("unrelated EquivalentPathKeys = %#v, want empty", got)
	}
}

func TestEffectDeltasPointwiseJoin(t *testing.T) {
	reg := standard.Registry()
	stateDomain := Domain(reg)
	ks := keyspace.New()
	common := effectdelta.Key{Target: mustStateKey(t, ks, pathdom.PathKey("sym110@1.table")), Site: "call", Kind: effectdelta.Mutation}
	leftOnly := effectdelta.Key{Target: mustStateKey(t, ks, pathdom.PathKey("sym110@1.left")), Site: "call", Kind: effectdelta.Mutation}
	presentDelta := effectdelta.Value{Before: presentValue(reg), After: presentValue(reg), Change: effectdelta.ChangeChanged}
	absentDelta := effectdelta.Value{Before: absentValue(reg), After: absentValue(reg), Change: effectdelta.ChangeNone}

	if got := (State{}).ReadEffectDelta(common); !effectdelta.Domain(reg).Equal(got, effectdelta.Bottom(reg)) {
		t.Fatalf("empty effect delta = %#v, want bottom", got)
	}
	if !stateDomain.Equal(State{}.WriteEffectDelta(common, presentDelta), State{}.WriteEffectDelta(common, presentDelta)) {
		t.Fatalf("equal effect deltas compare different")
	}

	left := State{}.WriteEffectDelta(common, presentDelta).WriteEffectDelta(leftOnly, presentDelta)
	right := State{}.WriteEffectDelta(common, absentDelta)
	if !stateDomain.Equal(stateDomain.Join(stateDomain.Bottom(), left), left) {
		t.Fatalf("state bottom should be join identity for effect deltas")
	}
	joined := stateDomain.Join(left, right)
	got := joined.ReadEffectDelta(common)
	if !effectdelta.Domain(reg).Equal(got, effectdelta.Top()) {
		t.Fatalf("joined effect delta = %#v, want joined values and unknown change", got)
	}
	if got := joined.ReadEffectDelta(leftOnly); !effectdelta.Domain(reg).Equal(got, presentDelta) {
		t.Fatalf("disjoint effect delta did not survive pointwise join: %#v", got)
	}
	if widened := stateDomain.Widen(left, right); !stateDomain.Equal(widened, joined) {
		t.Fatalf("effect delta widen differs from join")
	}
	if !stateDomain.LessOrEq(left, joined) || stateDomain.LessOrEq(joined, left) {
		t.Fatalf("effect delta order should be pointwise")
	}
	clone := left.Snapshot().WriteEffectDelta(common, absentDelta)
	if got := left.ReadEffectDelta(common); !effectdelta.Domain(reg).Equal(got, presentDelta) {
		t.Fatalf("effect delta clone write mutated original: %#v", got)
	}
	if got := clone.ReadEffectDelta(common); !effectdelta.Domain(reg).Equal(got, absentDelta) {
		t.Fatalf("effect delta clone write = %#v, want absent delta", got)
	}
}

func TestChannelSelectFactsUseMustJoin(t *testing.T) {
	stateDomain := Domain(standard.Registry())
	common := channelselectfact.Fact{Select: "select-1", Kind: channelselectfact.FactSelect, Result: pathdom.PathKey("sym120@1.result")}
	leftOnly := channelselectfact.Fact{Select: "select-1", Kind: channelselectfact.FactReceive, Case: pathdom.PathKey("sym120@1.left"), Index: 0}
	rightOnly := channelselectfact.Fact{Select: "select-1", Kind: channelselectfact.FactCase, Case: pathdom.PathKey("sym120@1.right"), Index: 1}
	left := State{}.AddChannelSelectFact(common).AddChannelSelectFact(leftOnly)
	right := State{}.AddChannelSelectFact(common).AddChannelSelectFact(rightOnly)

	if !left.HasChannelSelectFact(common) || !stateDomain.Equal(State{}.AddChannelSelectFact(common), State{}.AddChannelSelectFact(common)) {
		t.Fatalf("channel select empty/equality behavior failed")
	}
	if !stateDomain.Equal(stateDomain.Join(stateDomain.Bottom(), left), left) {
		t.Fatalf("state bottom should be join identity for channel select facts")
	}
	joined := stateDomain.Join(left, right)
	if !joined.HasChannelSelectFact(common) {
		t.Fatalf("common channel select fact was dropped")
	}
	if joined.HasChannelSelectFact(leftOnly) || joined.HasChannelSelectFact(rightOnly) {
		t.Fatalf("disjoint channel select fact survived must join")
	}
	if widened := stateDomain.Widen(left, right); !stateDomain.Equal(widened, joined) {
		t.Fatalf("channel select widen differs from join")
	}
	if !stateDomain.LessOrEq(left, joined) || stateDomain.LessOrEq(joined, left) {
		t.Fatalf("channel select order should move toward fewer must facts")
	}
	clone := left.Snapshot().AddChannelSelectFact(rightOnly)
	if left.HasChannelSelectFact(rightOnly) || !clone.HasChannelSelectFact(rightOnly) {
		t.Fatalf("channel select clone write mutated original or missed clone")
	}
}

func TestFrozenTablesUseMustJoin(t *testing.T) {
	stateDomain := Domain(standard.Registry())
	common := identity.ID{Kind: "table", Site: "freeze", Index: 1}
	leftOnly := identity.ID{Kind: "table", Site: "freeze", Index: 2}
	rightOnly := identity.ID{Kind: "table", Site: "freeze", Index: 3}

	if (State{}).IsTableFrozen(common) {
		t.Fatalf("empty state reported table frozen")
	}
	if stateDomain.Bottom().IsTableFrozen(common) {
		t.Fatalf("bottom state reported table frozen")
	}
	if stateDomain.Top().IsTableFrozen(common) {
		t.Fatalf("top state reported table frozen")
	}
	if got := (State{}).FreezeTable(identity.ID{}); !stateDomain.Equal(got, State{}) {
		t.Fatalf("freezing zero identity changed state")
	}

	written := State{}.FreezeTable(common)
	if !written.IsTableFrozen(common) || written.IsTableFrozen(leftOnly) {
		t.Fatalf("frozen table readback failed")
	}
	if !stateDomain.Equal(written.FreezeTable(common), written) {
		t.Fatalf("freezing the same table twice changed state")
	}
	if !stateDomain.Equal(stateDomain.Join(stateDomain.Bottom(), written), written) {
		t.Fatalf("state bottom should be join identity for frozen tables")
	}

	fromBottom := stateDomain.Bottom().FreezeTable(common)
	if !fromBottom.IsTableFrozen(common) || fromBottom.frozenTables.bottom {
		t.Fatalf("freeze from bottom did not produce reachable frozen-table lane")
	}

	left := State{}.FreezeTable(common).FreezeTable(leftOnly)
	right := State{}.FreezeTable(common).FreezeTable(rightOnly)
	joined := stateDomain.Join(left, right)
	if !joined.IsTableFrozen(common) {
		t.Fatalf("common frozen-table identity was dropped")
	}
	if joined.IsTableFrozen(leftOnly) || joined.IsTableFrozen(rightOnly) {
		t.Fatalf("one-sided frozen-table identity survived must join")
	}
	if widened := stateDomain.Widen(left, right); !stateDomain.Equal(widened, joined) {
		t.Fatalf("frozen-table widen differs from join")
	}
	if !stateDomain.LessOrEq(left, joined) || stateDomain.LessOrEq(joined, left) {
		t.Fatalf("frozen-table order should move toward fewer must proofs")
	}

	clone := left.Snapshot().FreezeTable(rightOnly)
	if left.IsTableFrozen(rightOnly) || !clone.IsTableFrozen(rightOnly) {
		t.Fatalf("frozen-table snapshot write mutated original or missed clone")
	}
}

func TestPlacementOrderAndCopy(t *testing.T) {
	stateDomain := Domain(standard.Registry())
	id := identity.ID{Kind: "table", Site: "escape", Index: 1}
	otherID := identity.ID{Kind: "table", Site: "escape", Index: 2}

	if got := (State{}).ReadPlacement(id); got != placement.Bottom {
		t.Fatalf("empty placement = %v, want bottom", got)
	}
	if !stateDomain.Equal(State{}.WritePlacement(id, placement.Stack), State{}.WritePlacement(id, placement.Stack)) {
		t.Fatalf("equal placements compare different")
	}

	left := State{}.
		WritePlacement(id, placement.Stack).
		WritePlacement(otherID, placement.OwnedHeap)
	right := State{}.WritePlacement(id, placement.SharedHeap)
	if !stateDomain.Equal(stateDomain.Join(stateDomain.Bottom(), left), left) {
		t.Fatalf("state bottom should be join identity for placement")
	}
	joined := stateDomain.Join(left, right)
	if got := joined.ReadPlacement(id); got != placement.SharedHeap {
		t.Fatalf("joined placement = %v, want shared heap", got)
	}
	if got := joined.ReadPlacement(otherID); got != placement.OwnedHeap {
		t.Fatalf("disjoint placement did not survive pointwise join: %v", got)
	}
	if widened := stateDomain.Widen(left, right); !stateDomain.Equal(widened, joined) {
		t.Fatalf("placement widen differs from join")
	}
	if !stateDomain.LessOrEq(left, joined) || stateDomain.LessOrEq(joined, left) {
		t.Fatalf("placement order should move toward shared/unknown")
	}
	clone := left.Snapshot().WritePlacement(id, placement.Unknown)
	if got := left.ReadPlacement(id); got != placement.Stack {
		t.Fatalf("placement clone write mutated original: %v", got)
	}
	if got := clone.ReadPlacement(id); got != placement.Unknown {
		t.Fatalf("placement clone write = %v, want unknown", got)
	}
}

func TestPlacementCanBeReadThroughValueIdentity(t *testing.T) {
	reg := standard.Registry()
	id := identity.ID{Kind: "table", Site: "literal", Index: 7}
	value := product.Set(reg, product.Top(), identity.Key, identity.Singleton(id))
	state := State{}.WritePlacement(id, placement.OwnedHeap)

	if got := placementOfValue(reg, state, value); got != placement.OwnedHeap {
		t.Fatalf("placement through identity = %v, want owned heap", got)
	}
	if got := placementOfValue(reg, state, product.Top()); got != placement.Bottom {
		t.Fatalf("placement without identity = %v, want bottom", got)
	}
}

func TestInvalidatePathKeySubtreeRemovesStructuredDescendants(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	valueDomain := product.Domain(reg)
	present := presentValue(reg)
	bottom := valueDomain.Bottom()
	root := pathdom.PathKey("sym40@3")
	prefix := pathdom.PathKey("sym40@3.field")
	child := pathdom.PathKey("sym40@3.field.deep")
	siblingPrefixCollision := pathdom.PathKey("sym40@3.fieldish")
	localVersionless := pathdom.PathKey("sym40.field.deep")
	otherVersion := pathdom.PathKey("sym40@4.field.deep")
	otherSymbol := pathdom.PathKey("sym41@3.field.deep")
	placeholderPrefix := pathdom.PathKey("$0.field")
	placeholderChild := pathdom.PathKey("$0.field.deep")
	placeholderSibling := pathdom.PathKey("$0.fieldish")
	prefixProof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathPresence, Path: mustStateKey(t, ks, prefix), Presence: presence.Present()}
	childProof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: mustStateKey(t, ks, child), Other: mustStateKey(t, ks, otherSymbol)}
	otherProof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathNotEqual, Path: mustStateKey(t, ks, otherSymbol), Other: mustStateKey(t, ks, otherVersion)}

	s := State{}.
		WritePathKey(reg, ks, root, present).
		WritePathKey(reg, ks, prefix, present).
		WritePathKey(reg, ks, child, present).
		WritePathKey(reg, ks, siblingPrefixCollision, present).
		WritePathKey(reg, ks, localVersionless, present).
		WritePathKey(reg, ks, otherVersion, present).
		WritePathKey(reg, ks, otherSymbol, present).
		WritePathKey(reg, ks, placeholderPrefix, present).
		WritePathKey(reg, ks, placeholderChild, present).
		WritePathKey(reg, ks, placeholderSibling, present).
		WritePathStaticMember(ks, prefix, present).
		WritePathStaticMember(ks, child, present).
		WritePathStaticMember(ks, siblingPrefixCollision, present).
		WritePathStaticMember(ks, otherVersion, present).
		AddBranchProof(prefixProof).
		AddBranchProof(childProof).
		AddBranchProof(otherProof)

	invalidPrefix, ok := s.InvalidatePathKeySubtree(ks, pathdom.PathKey(".field"))
	if ok {
		t.Fatal("InvalidatePathKeySubtree accepted invalid path key")
	}
	if !Domain(reg).Equal(invalidPrefix, s) {
		t.Fatal("invalid path-key prefix changed state")
	}

	out, ok := s.InvalidatePathKeySubtree(ks, prefix)
	if !ok {
		t.Fatal("InvalidatePathKeySubtree rejected versioned prefix")
	}
	for _, removed := range []pathdom.PathKey{prefix, child} {
		if got := out.ReadPathKey(reg, ks, removed); !valueDomain.Equal(got, bottom) {
			t.Fatalf("%s = %s, want bottom", removed, formatValue(reg, got))
		}
	}
	for _, kept := range []pathdom.PathKey{root, siblingPrefixCollision, otherVersion} {
		if got := out.ReadPathKey(reg, ks, kept); !valueDomain.Equal(got, present) {
			t.Fatalf("%s = %s, want present", kept, formatValue(reg, got))
		}
	}
	if got := out.ReadPathKey(reg, ks, otherSymbol); !valueDomain.Equal(got, bottom) {
		t.Fatalf("%s = %s, want bottom through alias proof", otherSymbol, formatValue(reg, got))
	}
	for _, notStored := range []pathdom.PathKey{localVersionless, placeholderPrefix, placeholderChild, placeholderSibling} {
		if got := out.ReadPathKey(reg, ks, notStored); !valueDomain.Equal(got, bottom) {
			t.Fatalf("%s = %s, want bottom because path evidence stores only point-local keys", notStored, formatValue(reg, got))
		}
	}
	for _, removed := range []pathdom.PathKey{prefix, child} {
		if got, ok := out.ReadPathStaticMember(ks, removed); ok {
			t.Fatalf("static member %s = %s, want removed", removed, formatValue(reg, got))
		}
	}
	for _, kept := range []pathdom.PathKey{siblingPrefixCollision, otherVersion} {
		if got, ok := out.ReadPathStaticMember(ks, kept); !ok || !valueDomain.Equal(got, present) {
			t.Fatalf("static member %s = %s ok=%v, want present", kept, formatValue(reg, got), ok)
		}
	}
	if out.HasBranchProof(prefixProof) || out.HasBranchProof(childProof) {
		t.Fatalf("subtree branch proof survived path invalidation")
	}
	if out.HasBranchProof(otherProof) {
		t.Fatalf("branch proof attached to invalidated alias survived")
	}
	if got := s.ReadPathKey(reg, ks, child); !valueDomain.Equal(got, present) {
		t.Fatalf("original child changed to %s", formatValue(reg, got))
	}
	if got, ok := s.ReadPathStaticMember(ks, child); !ok || !valueDomain.Equal(got, present) {
		t.Fatalf("original static member changed to %s ok=%v", formatValue(reg, got), ok)
	}
	if !s.HasBranchProof(childProof) {
		t.Fatalf("original branch proof changed")
	}

	beforePlaceholderInvalidation := out
	out, ok = out.InvalidatePathKeySubtree(ks, placeholderPrefix)
	if !ok {
		t.Fatal("InvalidatePathKeySubtree rejected placeholder prefix")
	}
	if !Domain(reg).Equal(out, beforePlaceholderInvalidation) {
		t.Fatalf("placeholder invalidation changed point-local path evidence")
	}
}

func placementOfValue(reg *axis.Registry, state State, value product.Value) placement.Value {
	idValue := product.Get(reg, value, identity.Key)
	id, ok := idValue.ID()
	if !ok {
		return placement.Bottom
	}
	return state.ReadPlacement(id)
}

func TestInvalidatePathKeySubtreeRemovesBranchProofsWithOtherUnderSubtree(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	valueDomain := product.Domain(reg)
	present := presentValue(reg)
	bottom := valueDomain.Bottom()
	prefix := pathdom.PathKey("sym70@2.field")
	otherInside := pathdom.PathKey("sym70@2.field.deep")
	outside := pathdom.PathKey("sym72@2.field")
	proof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: mustStateKey(t, ks, outside), Other: mustStateKey(t, ks, otherInside)}

	s := State{}.
		WritePathKey(reg, ks, prefix, present).
		WritePathKey(reg, ks, otherInside, present).
		WritePathKey(reg, ks, outside, present).
		WritePathStaticMember(ks, otherInside, present).
		AddBranchProof(proof)

	out, ok := s.InvalidatePathKeySubtree(ks, prefix)
	if !ok {
		t.Fatal("InvalidatePathKeySubtree rejected versioned prefix")
	}
	if got := out.ReadPathKey(reg, ks, prefix); !valueDomain.Equal(got, bottom) {
		t.Fatalf("%s = %s, want bottom", prefix, formatValue(reg, got))
	}
	if got := out.ReadPathKey(reg, ks, otherInside); !valueDomain.Equal(got, bottom) {
		t.Fatalf("%s = %s, want bottom", otherInside, formatValue(reg, got))
	}
	if got := out.ReadPathKey(reg, ks, outside); !valueDomain.Equal(got, bottom) {
		t.Fatalf("%s = %s, want bottom through alias proof", outside, formatValue(reg, got))
	}
	if got, ok := out.ReadPathStaticMember(ks, otherInside); ok {
		t.Fatalf("static member %s = %s, want removed", otherInside, formatValue(reg, got))
	}
	if out.HasBranchProof(proof) {
		t.Fatalf("branch proof with Other under invalidated subtree survived")
	}
}

func TestInvalidatePathKeyDescendantsKeepsContainerAndUnrelatedPaths(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	valueDomain := product.Domain(reg)
	present := presentValue(reg)
	bottom := valueDomain.Bottom()
	container := pathdom.PathKey("sym60@2.item")
	child := pathdom.PathKey("sym60@2.item.count")
	deepChild := pathdom.PathKey("sym60@2.item.name.first")
	siblingPrefixCollision := pathdom.PathKey("sym60@2.itemized.count")
	root := pathdom.PathKey("sym60@2")
	otherVersion := pathdom.PathKey("sym60@3.item.count")
	otherSymbol := pathdom.PathKey("sym61@2.item.count")
	placeholderContainer := pathdom.PathKey("$0.item")
	placeholderChild := pathdom.PathKey("$0.item.count")
	containerProof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathPresence, Path: mustStateKey(t, ks, container), Presence: presence.Present()}
	childProof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: mustStateKey(t, ks, child), Other: mustStateKey(t, ks, otherSymbol)}

	s := State{}.
		WritePathKey(reg, ks, container, present).
		WritePathKey(reg, ks, child, present).
		WritePathKey(reg, ks, deepChild, present).
		WritePathKey(reg, ks, siblingPrefixCollision, present).
		WritePathKey(reg, ks, root, present).
		WritePathKey(reg, ks, otherVersion, present).
		WritePathKey(reg, ks, otherSymbol, present).
		WritePathKey(reg, ks, placeholderContainer, present).
		WritePathKey(reg, ks, placeholderChild, present).
		WritePathStaticMember(ks, container, present).
		WritePathStaticMember(ks, child, present).
		WritePathStaticMember(ks, deepChild, present).
		WritePathStaticMember(ks, otherVersion, present).
		AddBranchProof(containerProof).
		AddBranchProof(childProof)

	invalidPrefix, ok := s.InvalidatePathKeyDescendants(ks, pathdom.PathKey(".item"))
	if ok {
		t.Fatal("InvalidatePathKeyDescendants accepted invalid path key")
	}
	if !Domain(reg).Equal(invalidPrefix, s) {
		t.Fatal("invalid path-key prefix changed state")
	}

	out, ok := s.InvalidatePathKeyDescendants(ks, container)
	if !ok {
		t.Fatal("InvalidatePathKeyDescendants rejected versioned prefix")
	}
	for _, removed := range []pathdom.PathKey{child, deepChild} {
		if got := out.ReadPathKey(reg, ks, removed); !valueDomain.Equal(got, bottom) {
			t.Fatalf("%s = %s, want bottom", removed, formatValue(reg, got))
		}
	}
	for _, kept := range []pathdom.PathKey{container, siblingPrefixCollision, root, otherVersion} {
		if got := out.ReadPathKey(reg, ks, kept); !valueDomain.Equal(got, present) {
			t.Fatalf("%s = %s, want present", kept, formatValue(reg, got))
		}
	}
	if got := out.ReadPathKey(reg, ks, otherSymbol); !valueDomain.Equal(got, bottom) {
		t.Fatalf("%s = %s, want bottom through descendant alias proof", otherSymbol, formatValue(reg, got))
	}
	for _, notStored := range []pathdom.PathKey{placeholderContainer, placeholderChild} {
		if got := out.ReadPathKey(reg, ks, notStored); !valueDomain.Equal(got, bottom) {
			t.Fatalf("%s = %s, want bottom because path evidence stores only point-local keys", notStored, formatValue(reg, got))
		}
	}
	for _, removed := range []pathdom.PathKey{child, deepChild} {
		if got, ok := out.ReadPathStaticMember(ks, removed); ok {
			t.Fatalf("static member %s = %s, want removed", removed, formatValue(reg, got))
		}
	}
	for _, kept := range []pathdom.PathKey{container, otherVersion} {
		if got, ok := out.ReadPathStaticMember(ks, kept); !ok || !valueDomain.Equal(got, present) {
			t.Fatalf("static member %s = %s ok=%v, want present", kept, formatValue(reg, got), ok)
		}
	}
	if !out.HasBranchProof(containerProof) {
		t.Fatalf("container branch proof was removed by descendant invalidation")
	}
	if out.HasBranchProof(childProof) {
		t.Fatalf("descendant branch proof survived descendant invalidation")
	}
	if got := s.ReadPathKey(reg, ks, child); !valueDomain.Equal(got, present) {
		t.Fatalf("original child changed to %s", formatValue(reg, got))
	}

	beforePlaceholderInvalidation := out
	out, ok = out.InvalidatePathKeyDescendants(ks, placeholderContainer)
	if !ok {
		t.Fatal("InvalidatePathKeyDescendants rejected placeholder prefix")
	}
	if !Domain(reg).Equal(out, beforePlaceholderInvalidation) {
		t.Fatalf("placeholder invalidation changed point-local path evidence")
	}
}

func TestInvalidatePathKeyDescendantsFromRootRemovesStaticMembersAndBranchProofs(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	valueDomain := product.Domain(reg)
	present := presentValue(reg)
	bottom := valueDomain.Bottom()
	root := pathdom.PathKey("sym80@1")
	child := pathdom.PathKey("sym80@1.field")
	descendant := pathdom.PathKey("sym80@1.field.deep")
	outside := pathdom.PathKey("sym81@1.field")
	proof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathNotEqual, Path: mustStateKey(t, ks, outside), Other: mustStateKey(t, ks, descendant)}

	s := State{}.
		WritePathKey(reg, ks, root, present).
		WritePathKey(reg, ks, child, present).
		WritePathKey(reg, ks, descendant, present).
		WritePathKey(reg, ks, outside, present).
		WritePathStaticMember(ks, child, present).
		WritePathStaticMember(ks, descendant, present).
		AddBranchProof(proof)

	out, ok := s.InvalidatePathKeyDescendants(ks, root)
	if !ok {
		t.Fatal("InvalidatePathKeyDescendants rejected versioned root")
	}
	if got := out.ReadPathKey(reg, ks, root); !valueDomain.Equal(got, present) {
		t.Fatalf("%s = %s, want present", root, formatValue(reg, got))
	}
	for _, removed := range []pathdom.PathKey{child, descendant} {
		if got := out.ReadPathKey(reg, ks, removed); !valueDomain.Equal(got, bottom) {
			t.Fatalf("%s = %s, want bottom", removed, formatValue(reg, got))
		}
		if got, ok := out.ReadPathStaticMember(ks, removed); ok {
			t.Fatalf("static member %s = %s, want removed", removed, formatValue(reg, got))
		}
	}
	if got := out.ReadPathKey(reg, ks, outside); !valueDomain.Equal(got, present) {
		t.Fatalf("%s = %s, want present", outside, formatValue(reg, got))
	}
	if out.HasBranchProof(proof) {
		t.Fatalf("branch proof with Other under root descendant survived descendant invalidation")
	}
}

func TestTopLanesReadTopAndRejectFiniteUpdates(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	valueDomain := product.Domain(reg)
	top := Domain(reg).Top()
	slot := key.SymbolValue(symbol.ID(50))
	pathKey := pathdom.PathKey("sym50@1.field")
	dynamicKey := dynamicindex.Key{Table: mustStateKey(t, ks, pathdom.PathKey("sym50@1.table")), Site: "dyn"}
	heapID := identity.ID{Kind: "table", Site: "top", Index: 1}
	effectKey := effectdelta.Key{Target: mustStateKey(t, ks, pathdom.PathKey("sym50@1.table")), Site: "effect", Kind: effectdelta.Mutation}
	escapeID := identity.ID{Kind: "table", Site: "escape-top", Index: 1}
	present := presentValue(reg)
	dynamicFact := dynamicindex.Fact{
		KeyPresence: presence.Present(),
		KeyValue:    present,
		Value:       present,
		Admission:   dynamicindex.AdmissionAdmitted,
	}
	effectDelta := effectdelta.Value{Before: present, After: present, Change: effectdelta.ChangeChanged}

	if got := top.ReadValue(reg, slot); !valueDomain.Equal(got, product.Top()) {
		t.Fatalf("top value read = %s, want top", formatValue(reg, got))
	}
	if got := top.ReadReturnSlot(reg, 0); !valueDomain.Equal(got, product.Top()) {
		t.Fatalf("top return read = %s, want top", formatValue(reg, got))
	}
	if got := top.ReadPathKey(reg, ks, pathKey); !valueDomain.Equal(got, product.Bottom(reg)) {
		t.Fatalf("top path read = %s, want bottom absence", formatValue(reg, got))
	}
	if got := top.ReadDynamicIndexFact(reg, dynamicKey); !dynamicindex.Domain(reg).Equal(got, dynamicindex.Top()) {
		t.Fatalf("top dynamic-index read = %#v, want top", got)
	}
	if got := top.ReadHeapTableObject(reg, heapID); !heapidentity.ObjectDomain(reg).Equal(got, heapidentity.TopObject()) {
		t.Fatalf("top heap-object read = %#v, want top", got)
	}
	if got := top.ReadEffectDelta(effectKey); !effectdelta.Domain(reg).Equal(got, effectdelta.Top()) {
		t.Fatalf("top effect-delta read = %#v, want top", got)
	}
	if got := top.ReadPlacement(escapeID); got != placement.Unknown {
		t.Fatalf("top placement read = %v, want unknown", got)
	}
	if top.IsTableFrozen(heapID) {
		t.Fatalf("top frozen-table read = true, want conservative false")
	}
	if _, ok := top.ReadPathStaticMember(ks, pathKey); ok {
		t.Fatalf("top static-member lane should read as unknown absence")
	}

	requirePanic(t, func() {
		top.WriteValue(reg, slot, present)
	})
	requirePanic(t, func() {
		top.UpdateValue(reg, slot, func(v product.Value) product.Value {
			if !valueDomain.Equal(v, product.Top()) {
				t.Fatalf("UpdateValue on top read %s, want top", formatValue(reg, v))
			}
			return present
		})
	})
	requirePanic(t, func() {
		top.WriteReturnSlot(reg, 0, present)
	})
	requirePanic(t, func() {
		top.UpdateReturnSlot(reg, 0, func(v product.Value) product.Value {
			if !valueDomain.Equal(v, product.Top()) {
				t.Fatalf("UpdateReturnSlot on top read %s, want top", formatValue(reg, v))
			}
			return present
		})
	})
	requirePanic(t, func() {
		top.WriteDynamicIndexFact(reg, dynamicKey, dynamicFact)
	})
	requirePanic(t, func() {
		top.WriteHeapTableObject(reg, heapID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: present}))
	})
	requirePanic(t, func() {
		top.WriteEffectDelta(effectKey, effectDelta)
	})
	requirePanic(t, func() {
		top.WritePlacement(escapeID, placement.Stack)
	})
}

func TestStatePackageDoesNotImportLuaPackages(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", ".")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps . failed: %v", err)
	}
	banned := []string{
		"github.com/wippyai/go-lua/__old",
		"github.com/wippyai/go-lua/analysis/engine/visibility",
		"github.com/wippyai/go-lua/analysis/ir/cfg",
		"github.com/wippyai/go-lua/analysis/lua",
		"github.com/wippyai/go-lua/compiler",
		"github.com/wippyai/go-lua/compiler/ast",
		"go/ast",
	}
	for _, dep := range strings.Fields(string(out)) {
		for _, prefix := range banned {
			if dep == prefix || strings.HasPrefix(dep, prefix+"/") {
				t.Fatalf("state package imports forbidden dependency %q", dep)
			}
		}
	}
}

type stateLawFixture struct {
	valueSlot     key.Value
	returnSlot    int
	pathKey       pathdom.PathKey
	staticKey     pathdom.PathKey
	staticHeapKey keyspace.Key
	dynamicKey    dynamicindex.Key
	heapID        identity.ID
	effectKey     effectdelta.Key
	escapeID      identity.ID
	freezeID      identity.ID
	channelFact   channelselectfact.Fact
	proof         pathevidence.BranchProof
	present       product.Value
	absent        product.Value
	dynamicFact   dynamicindex.Fact
	effectDelta   effectdelta.Value
}

func stateLawFixtureFor(reg *axis.Registry, ks *keyspace.KeySpace) stateLawFixture {
	present := presentValue(reg)
	absent := absentValue(reg)
	pathKey := pathdom.PathKey("sym201@1.field")
	staticKey := pathdom.PathKey("sym201@1.shared")
	staticHeapKey, ok := ks.FromStateKey(staticKey)
	if !ok {
		panic("stateLawFixtureFor: FromStateKey failed for static heap key")
	}
	tableKey := pathdom.PathKey("sym201@1.table")
	tableHeapKey, ok := ks.FromStateKey(tableKey)
	if !ok {
		panic("stateLawFixtureFor: FromStateKey failed for table heap key")
	}
	valueSlot := key.SymbolValue(symbol.ID(201))
	returnSlot := 3
	dynamicKey := dynamicindex.Key{Table: tableHeapKey, Site: "dyn"}
	heapID := identity.ID{Kind: "table", Site: "state-law", Index: 1}
	effectKey := effectdelta.Key{Target: tableHeapKey, Site: "effect", Kind: effectdelta.Mutation}
	escapeID := identity.ID{Kind: "table", Site: "escape-law", Index: 1}
	freezeID := identity.ID{Kind: "table", Site: "freeze-law", Index: 1}
	channelFact := channelselectfact.Fact{Select: "select-law", Kind: channelselectfact.FactSelect, Result: pathKey}
	proofPathKey, ok := ks.FromStateKey(pathKey)
	if !ok {
		panic("stateLawFixtureFor: FromStateKey failed for proof path key")
	}
	proof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathPresence, Path: proofPathKey, Presence: presence.Present()}
	dynamicFact := dynamicindex.Fact{
		KeyPresence: presence.Present(),
		KeyValue:    present,
		Value:       present,
		Admission:   dynamicindex.AdmissionAdmitted,
	}
	effectDelta := effectdelta.Value{Before: present, After: present, Change: effectdelta.ChangeChanged}

	return stateLawFixture{
		valueSlot:     valueSlot,
		returnSlot:    returnSlot,
		pathKey:       pathKey,
		staticKey:     staticKey,
		staticHeapKey: staticHeapKey,
		dynamicKey:    dynamicKey,
		heapID:        heapID,
		effectKey:     effectKey,
		escapeID:      escapeID,
		freezeID:      freezeID,
		channelFact:   channelFact,
		proof:         proof,
		present:       present,
		absent:        absent,
		dynamicFact:   dynamicFact,
		effectDelta:   effectDelta,
	}
}

func stateLawSample(reg *axis.Registry, ks *keyspace.KeySpace) []State {
	fx := stateLawFixtureFor(reg, ks)
	bottom := Domain(reg).Bottom()
	top := Domain(reg).Top()

	valueState := State{}.
		WriteValue(reg, fx.valueSlot, fx.present).
		WriteReturnSlot(reg, fx.returnSlot, fx.absent)
	pathState := State{}.
		WritePathKey(reg, ks, fx.pathKey, fx.present).
		WritePathStaticMember(ks, fx.staticKey, fx.present).
		AddBranchProof(fx.proof)
	dynamicState := State{}.WriteDynamicIndexFact(reg, fx.dynamicKey, fx.dynamicFact)
	heapState := State{}.WriteHeapTableObject(reg, fx.heapID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: fx.present,
		StaticMembers: map[keyspace.Key]product.Value{
			fx.staticHeapKey: fx.present,
		},
	}))
	effectState := State{}.
		WriteEffectDelta(fx.effectKey, fx.effectDelta).
		WritePlacement(fx.escapeID, placement.Stack)
	channelState := State{}.AddChannelSelectFact(fx.channelFact)
	frozenState := State{}.FreezeTable(fx.freezeID)
	fullState := valueState.
		WritePathKey(reg, ks, fx.pathKey, fx.present).
		WritePathStaticMember(ks, fx.staticKey, fx.present).
		WriteDynamicIndexFact(reg, fx.dynamicKey, fx.dynamicFact).
		WriteHeapTableObject(reg, fx.heapID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: fx.present,
			StaticMembers: map[keyspace.Key]product.Value{
				fx.staticHeapKey: fx.present,
			},
		})).
		WriteEffectDelta(fx.effectKey, fx.effectDelta).
		WritePlacement(fx.escapeID, placement.Stack).
		FreezeTable(fx.freezeID).
		AddChannelSelectFact(fx.channelFact).
		AddBranchProof(fx.proof)

	return []State{bottom, top, valueState, pathState, dynamicState, heapState, effectState, channelState, frozenState, fullState}
}

func stateLawFormat(reg *axis.Registry, ks *keyspace.KeySpace) func(State) string {
	fx := stateLawFixtureFor(reg, ks)
	return func(s State) string {
		static := "absent"
		if got, ok := s.ReadPathStaticMember(ks, fx.staticKey); ok {
			static = formatValue(reg, got)
		}
		return fmt.Sprintf(
			"v=%s ret=%s path=%s static=%s dyn=%#v heap-root=%s effect=%#v placement=%v frozen=%v chan=%v proof=%v",
			formatValue(reg, s.ReadValue(reg, fx.valueSlot)),
			formatValue(reg, s.ReadReturnSlot(reg, fx.returnSlot)),
			formatValue(reg, s.ReadPathKey(reg, ks, fx.pathKey)),
			static,
			s.ReadDynamicIndexFact(reg, fx.dynamicKey),
			formatValue(reg, s.ReadHeapTableObject(reg, fx.heapID).Root()),
			s.ReadEffectDelta(fx.effectKey),
			s.ReadPlacement(fx.escapeID),
			s.IsTableFrozen(fx.freezeID),
			s.HasChannelSelectFact(fx.channelFact),
			s.HasBranchProof(fx.proof),
		)
	}
}

func TestFiniteLaneSettersRejectBottomValues(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	tableKey := mustStateKey(t, ks, pathdom.PathKey("sym1@1.table"))
	dynamicKey := dynamicindex.Key{Table: tableKey, Site: "site"}
	effectKey := effectdelta.Key{Target: tableKey, Site: "site", Kind: effectdelta.Mutation}
	id := identity.ID{Kind: "table", Site: "lane-bottom", Index: 1}

	requirePanic(t, func() {
		dynamicIndexLane{}.with(dynamicKey, dynamicindex.Bottom(reg))
	})
	requirePanic(t, func() {
		effectDeltaLane{}.with(effectKey, effectdelta.Bottom(reg))
	})
	requirePanic(t, func() {
		placementLane{}.with(id, placement.Bottom)
	})
}

func requirePanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}

func presentValue(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Present())
}

func absentValue(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Absent())
}

func heapStaticKey(t *testing.T, ks *keyspace.KeySpace, name string) keyspace.Key {
	t.Helper()
	k, ok := ks.FromStateKey(pathdom.PathKey(name))
	if !ok {
		t.Fatalf("FromStateKey(%q) failed", name)
	}
	return k
}

func mustStateKey(t *testing.T, ks *keyspace.KeySpace, key pathdom.PathKey) keyspace.Key {
	t.Helper()
	k, ok := ks.FromStateKey(key)
	if !ok {
		t.Fatalf("FromStateKey(%q) failed", key)
	}
	return k
}

func testStateKey(t *testing.T, key pathdom.PathKey) pathaddr.StateKey {
	t.Helper()
	got, ok := pathaddr.StateKeyFromPathKey(key)
	if !ok {
		t.Fatalf("StateKeyFromPathKey(%q) failed", key)
	}
	return got
}

func staticMemberEqual(reg *axis.Registry, object heapidentity.TableObject, key keyspace.Key, want product.Value) bool {
	got, ok := object.StaticMember(key)
	return ok && product.Equal(reg, got, want)
}

func formatValue(reg *axis.Registry, v product.Value) string {
	switch {
	case product.Equal(reg, v, product.Bottom(reg)):
		return "bottom"
	case product.Equal(reg, v, product.Top()):
		return "top"
	case presence.Equal(product.PresenceOf(v), presence.Present()):
		return "present"
	case presence.Equal(product.PresenceOf(v), presence.Absent()):
		return "absent"
	default:
		return product.PresenceOf(v).String()
	}
}

func formatState(reg *axis.Registry, ks *keyspace.KeySpace, s State) string {
	return "value-slot=" + formatValue(reg, s.ReadValue(reg, key.SymbolValue(21))) +
		" return-slot=" + formatValue(reg, s.ReadValue(reg, key.ReturnSlot(0))) +
		" path=" + formatValue(reg, s.ReadPathKey(reg, ks, pathdom.PathKey("sym21@2.field"))) +
		" other-path=" + formatValue(reg, s.ReadPathKey(reg, ks, pathdom.PathKey("$0.item")))
}
