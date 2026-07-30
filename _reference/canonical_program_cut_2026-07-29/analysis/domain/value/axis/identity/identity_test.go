package identity

import (
	"strconv"
	"testing"
	"unsafe"

	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
)

func TestIDHotMapKeySizeRemainsCompact(t *testing.T) {
	if got := unsafe.Sizeof(ID{}); got != 40 {
		t.Fatalf("identity.ID size = %d, want 40 bytes", got)
	}
}

func testTableIdentity(scope, site uint64) ID {
	if scope == 0 || site == 0 {
		return ID{}
	}
	return ID{Kind: "lua.table", Site: "test-table:" + strconv.FormatUint(scope, 10), Index: site}
}

func testBodyIdentity(scope uint64) lexicalidentity.StableLexicalBodyID {
	return lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte(strconv.FormatUint(scope, 10))))
}

var (
	allocA  = ID{Kind: "alloc", Site: "chunk.lua:12:4", Index: 1}
	allocB  = ID{Kind: "alloc", Site: "chunk.lua:12:4", Index: 2}
	closure = ID{Kind: "closure", Site: "chunk.lua:20:9", Index: 1}
)

func TestIdentityLatticeLaws(t *testing.T) {
	suite := latticelaws.LawSuite[Value]{
		Name:   "axis.identity",
		Domain: Spec().Lattice(),
		Sample: []Value{
			Bottom(),
			Singleton(allocA),
			Singleton(allocB),
			Singleton(closure),
			Top(),
		},
		Format: Value.String,
	}
	suite.Run(t)
}

func TestSingletonReadback(t *testing.T) {
	v := Singleton(allocA)
	got, ok := v.ID()
	if !ok || got != allocA {
		t.Fatalf("ID() = (%#v, %v), want (%#v, true)", got, ok, allocA)
	}
	if got, ok := Bottom().ID(); ok || got != (ID{}) {
		t.Fatalf("Bottom().ID() = (%#v, %v), want zero/false", got, ok)
	}
	if got, ok := Top().ID(); ok || got != (ID{}) {
		t.Fatalf("Top().ID() = (%#v, %v), want zero/false", got, ok)
	}
}

func TestReturnedAllocationIsDeterministicPerStaticCallerSite(t *testing.T) {
	template := testTableIdentity(31, 7)
	first := ReturnedAllocationInBody(template, testBodyIdentity(41), 0, 0, 0, 9)
	if first == (ID{}) || !IsReturnedAllocation(first) {
		t.Fatalf("ReturnedAllocation = %#v, want tagged non-zero identity", first)
	}
	if again := ReturnedAllocationInBody(template, testBodyIdentity(41), 0, 0, 0, 9); again != first {
		t.Fatalf("same static site changed identity: first=%#v again=%#v", first, again)
	}
	if other := ReturnedAllocationInBody(template, testBodyIdentity(41), 0, 0, 0, 10); other == first {
		t.Fatal("distinct call points aliased")
	}
	if other := ReturnedAllocationInBody(template, testBodyIdentity(42), 0, 0, 0, 9); other == first {
		t.Fatal("equal call points in distinct caller graphs aliased")
	}
	if other := ReturnedAllocationInBody(testTableIdentity(31, 8), testBodyIdentity(41), 0, 0, 0, 9); other == first {
		t.Fatal("distinct callee templates at one call point aliased")
	}
	if got := ReturnedAllocationInBody(ID{}, testBodyIdentity(41), 0, 0, 0, 9); got != (ID{}) {
		t.Fatalf("zero template produced %#v", got)
	}
	if IsReturnedAllocation(ID{Kind: "lua.table", Site: "returned-allocation-v2:forged", Index: 9}) {
		t.Fatal("site spelling forged returned-allocation phase")
	}
}

// TestReturnedAllocationRecognitionIsIndependentOfCalleeTemplateKind guards the
// #1682 register defect: ReturnedAllocationInBody once tagged the constructed
// ID with the callee template's own Kind, while IsReturnedAllocation checked
// the ID against the fixed returned-allocation Kind. Those two Kinds can never
// be equal, so the predicate was permanently false and callers always took the
// overwrite path for a returned heap allocation instead of joining it with
// whatever the same static call-outcome slot already held. Every callee
// template, regardless of its own Kind, must still be recognized as a returned
// allocation once instantiated at a caller site.
func TestReturnedAllocationRecognitionIsIndependentOfCalleeTemplateKind(t *testing.T) {
	body := testBodyIdentity(53)
	templates := map[string]ID{
		"lua.table":           testTableIdentity(31, 7),
		"manifest.allocation": ManifestAllocation("callee-template-site", 5),
	}
	for name, template := range templates {
		t.Run(name, func(t *testing.T) {
			if template.Kind != name {
				t.Fatalf("test setup: template kind = %q, want %q", template.Kind, name)
			}
			got := ReturnedAllocationInBody(template, body, 0, 0, 0, 12)
			if got == (ID{}) {
				t.Fatal("empty returned-allocation identity")
			}
			if got.Kind == template.Kind {
				t.Fatalf("returned-allocation Kind leaked the callee template Kind %q", template.Kind)
			}
			if !IsReturnedAllocation(got) {
				t.Fatalf("IsReturnedAllocation(%#v) = false for callee template kind %q, want true", got, template.Kind)
			}
		})
	}
}

// TestReturnedAllocationJoinPromotesDistinctEvidenceInsteadOfLastWriteWins ties
// the #1682 defect to the identity axis's own lattice law: two distinct
// returned allocations reaching the same axis value must JOIN to Top, the
// axis's documented signal that evidence diverged, and must never resolve to
// whichever singleton was written last.
func TestReturnedAllocationJoinPromotesDistinctEvidenceInsteadOfLastWriteWins(t *testing.T) {
	body := testBodyIdentity(53)
	first := ReturnedAllocationInBody(testTableIdentity(31, 7), body, 0, 0, 0, 12)
	second := ReturnedAllocationInBody(testTableIdentity(31, 8), body, 0, 0, 0, 12)
	if first == second {
		t.Fatal("test setup: distinct callee templates aliased at one call point")
	}
	joined := Join(Singleton(first), Singleton(second))
	if !joined.IsTop() {
		t.Fatalf("Join(first, second) = %s, want top", joined)
	}
	if lastWriteWins := Join(Singleton(first), Singleton(second)); Equal(lastWriteWins, Singleton(second)) {
		t.Fatal("join resolved to the second write instead of promoting to top")
	}
}

func TestBoundaryAllocationIsFiniteLexicalCallSiteIdentity(t *testing.T) {
	caller := testBodyIdentity(44)
	template := ManifestAllocationTemplate(testBodyIdentity(91), 7, 1)
	first := BoundaryAllocation(template, caller, 19, 3)
	if first == (ID{}) || !IsBoundaryAllocation(first) {
		t.Fatalf("boundary allocation = %#v", first)
	}
	if again := BoundaryAllocation(template, caller, 19, 3); again != first {
		t.Fatalf("same lexical mu application changed identity: %#v != %#v", again, first)
	}
	for name, other := range map[string]ID{
		"allocation": BoundaryAllocation(ManifestAllocationTemplate(testBodyIdentity(91), 8, 1), caller, 19, 3),
		"object":     BoundaryAllocation(ManifestAllocationTemplate(testBodyIdentity(91), 7, 2), caller, 19, 3),
		"owner":      BoundaryAllocation(ManifestAllocationTemplate(testBodyIdentity(92), 7, 1), caller, 19, 3),
		"caller":     BoundaryAllocation(template, testBodyIdentity(45), 19, 3),
		"point":      BoundaryAllocation(template, caller, 20, 3),
		"occurrence": BoundaryAllocation(template, caller, 19, 4),
	} {
		if other == first {
			t.Fatalf("%s did not distinguish boundary allocation", name)
		}
	}
	if BoundaryAllocation(AllocationTemplate{}, caller, 19, 3) != (ID{}) ||
		BoundaryAllocation(template, lexicalidentity.StableLexicalBodyID{}, 19, 3) != (ID{}) ||
		BoundaryAllocation(template, caller, 0, 3) != (ID{}) {
		t.Fatal("invalid boundary allocation authority did not fail closed")
	}
	if _, concrete := AllocationTerm(template).Concrete(); concrete {
		t.Fatal("template and instantiated allocation alternatives were not separated")
	}
}

func TestBoundaryAllocationCannotConsumeAnInstantiatedIdentity(t *testing.T) {
	template := ManifestAllocationTemplate(testBodyIdentity(91), 7, 1)
	actual := BoundaryAllocation(template, testBodyIdentity(44), 19, 3)
	if actual == (ID{}) {
		t.Fatal("expected instantiated allocation")
	}
	// The next phase accepts only the structural coordinate. There is no
	// conversion from actual (ID) back to AllocationTemplate; recreating the
	// same coordinate is idempotent and never incorporates actual's spelling.
	again := BoundaryAllocation(ManifestAllocationTemplate(template.Owner(), template.AllocationOrdinal(), template.ObjectOrdinal()), testBodyIdentity(44), 19, 3)
	if again != actual {
		t.Fatalf("structural coordinate changed identity: %#v != %#v", again, actual)
	}
}

func TestRootBoundaryAllocationIsFiniteStructuralIdentity(t *testing.T) {
	template := ManifestAllocationTemplate(testBodyIdentity(93), 7, 2)
	got := RootBoundaryAllocation(template)
	if got == (ID{}) || !IsRootBoundaryAllocation(got) {
		t.Fatalf("root allocation = %#v", got)
	}
	if again := RootBoundaryAllocation(template); again != got {
		t.Fatalf("root allocation is nondeterministic: %#v != %#v", again, got)
	}
	if other := RootBoundaryAllocation(ManifestAllocationTemplate(testBodyIdentity(93), 7, 3)); other == got {
		t.Fatal("distinct root allocation objects collided")
	}
}

func TestLuaTableLiteralIdentityDoesNotAliasEqualGraphAndExprPairs(t *testing.T) {
	first := testTableIdentity(31, 31)
	second := testTableIdentity(42, 42)
	if first == (ID{}) || second == (ID{}) {
		t.Fatalf("LuaTableLiteral returned empty identities: %#v %#v", first, second)
	}
	if first == second {
		t.Fatalf("LuaTableLiteral equal graph/expr pairs aliased: %#v", first)
	}
	if first.Index == 0 || second.Index == 0 {
		t.Fatalf("LuaTableLiteral produced zero index: %#v %#v", first, second)
	}

	ordered := testTableIdentity(31, 42)
	swapped := testTableIdentity(42, 31)
	if ordered == swapped {
		t.Fatalf("LuaTableLiteral lost field order: %#v", ordered)
	}
	if got := testTableIdentity(31, 31); got != first {
		t.Fatalf("LuaTableLiteral is not stable: %#v then %#v", first, got)
	}
}

func TestLuaTableLiteralRejectsEmptyInputs(t *testing.T) {
	for _, tc := range []struct {
		name    string
		graphID uint64
		exprRef uint64
	}{
		{name: "zero graph", graphID: 0, exprRef: 1},
		{name: "zero expression", graphID: 1, exprRef: 0},
		{name: "zero both", graphID: 0, exprRef: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := testTableIdentity(tc.graphID, tc.exprRef); got != (ID{}) {
				t.Fatalf("testTableIdentity(%d, %d) = %#v, want empty ID", tc.graphID, tc.exprRef, got)
			}
		})
	}
}

func TestTableLiteralAtPrecomputedSiteAllocatesNothing(t *testing.T) {
	site := TableLiteralSiteForBody(testBodyIdentity(91))
	if allocations := testing.AllocsPerRun(1000, func() {
		if LuaTableLiteralAtSite(site, 7) == (ID{}) {
			t.Fatal("empty identity")
		}
	}); allocations != 0 {
		t.Fatalf("allocations = %v, want 0", allocations)
	}
}

func TestReturnedAllocationUsesOneSiteAllocation(t *testing.T) {
	template := testTableIdentity(31, 7)
	body := testBodyIdentity(92)
	if allocations := testing.AllocsPerRun(1000, func() {
		if ReturnedAllocationInBody(template, body, 1, 2, 3, 4) == (ID{}) {
			t.Fatal("empty identity")
		}
	}); allocations > 1 {
		t.Fatalf("allocations = %v, want at most 1", allocations)
	}
}

func TestIdentityJoin(t *testing.T) {
	a := Singleton(allocA)
	same := Singleton(allocA)
	b := Singleton(allocB)

	if got := Join(Bottom(), a); !Equal(got, a) {
		t.Fatalf("Bottom join singleton = %s, want %s", got, a)
	}
	if got := Join(a, same); !Equal(got, a) {
		t.Fatalf("same singleton join = %s, want %s", got, a)
	}
	if got := Join(a, b); !Equal(got, Top()) {
		t.Fatalf("different singleton join = %s, want top", got)
	}
	if got := Widen(a, b); !Equal(got, Join(a, b)) {
		t.Fatalf("Widen = %s, want Join result", got)
	}
}

func TestIdentityOrderAndCovers(t *testing.T) {
	a := Singleton(allocA)
	b := Singleton(allocB)

	if !LessOrEq(Bottom(), a) || !LessOrEq(a, Top()) {
		t.Fatalf("expected bottom < singleton < top")
	}
	if LessOrEq(a, b) || LessOrEq(b, a) {
		t.Fatalf("distinct singleton identities must be incomparable")
	}
	if !Top().Covers(a) || !a.Covers(Bottom()) {
		t.Fatalf("Covers should be the inverse order")
	}
	if a.Covers(b) {
		t.Fatalf("singleton should not cover a distinct singleton")
	}
}

func TestIdentityHashAndString(t *testing.T) {
	a := Singleton(allocA)
	same := Singleton(allocA)
	b := Singleton(allocB)

	if a.Hash() != same.Hash() {
		t.Fatalf("equal singleton values should have equal hashes")
	}
	if a.Hash() == b.Hash() {
		t.Fatalf("distinct singleton identities should not collide in this regression case")
	}
	if Bottom().Hash() == Top().Hash() {
		t.Fatalf("bottom and top should not hash identically")
	}
	if got := allocA.String(); got != "alloc:chunk.lua:12:4#1" {
		t.Fatalf("ID.String() = %q, want alloc:chunk.lua:12:4#1", got)
	}
	if got := a.String(); got != "singleton(alloc:chunk.lua:12:4#1)" {
		t.Fatalf("Value.String() = %q, want singleton(alloc:chunk.lua:12:4#1)", got)
	}
}
