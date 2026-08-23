package engine

import "testing"

// The read-boundary contract is the engine's single declaration of how a read
// is delivered. These laws hold it to the four things a Fold must never have to
// restate: the declared member order, the Factor default at an unwritten
// coordinate, the widened reading of an opaque alternative, and the named
// refusal of a foreign owner.

// TestFactorDefaultReadDeliversTheDefaultAndNeverAnAbsentCell proves the sparse
// clause: under ReadSparseFactorDefault an unwritten coordinate reaches the Fold
// as the Factor's declared default, present, so there is no absent branch left
// for a rule to get wrong. The explicit declaration keeps the distinction.
func TestFactorDefaultReadDeliversTheDefaultAndNeverAnAbsentCell(t *testing.T) {
	const declaredDefault = uint64(9)
	owned := &schemaFactorBindingCell[uint64, uint64]{}
	target := routeSetLawFactor{unit: routeSetLawUnit(t), row: owned, zero: declaredDefault, top: 1 << 40}

	if _, admitted := stagedReadPolicy[uint64](target, nil, ReadContract{Sparse: ReadSparseFactorDefault}); admitted {
		t.Fatal("read policy admitted a nil owner row")
	}
	foreign := &schemaFactorBindingCell[uint64, uint64]{}
	if _, admitted := stagedReadPolicy[uint64](target, foreign, ReadContract{Sparse: ReadSparseFactorDefault}); admitted {
		t.Fatal("read policy admitted a Factor its sealed row does not own")
	}
	defaulted, admitted := stagedReadPolicy[uint64](target, target.row, ReadContract{Sparse: ReadSparseFactorDefault})
	if !admitted {
		t.Fatalf("factor-default policy = %+v admitted=%t", defaulted, admitted)
	}

	written := summaryCellFrom(defaulted, 4, true)
	if written.value != 4 || !written.present {
		t.Fatalf("a written coordinate was rewritten: %+v", written)
	}
	unwritten := summaryCellFrom(defaulted, 0, false)
	if !unwritten.present {
		t.Fatal("a FactorDefault read delivered an absent cell to its fold")
	}
	if unwritten.value != declaredDefault {
		t.Fatalf("unwritten coordinate = %d, want the declared default %d", unwritten.value, declaredDefault)
	}

	cells := OrderedCells[uint64]{record: newOrderedCellsRecord([]summaryCell[uint64]{unwritten})}
	value, available := cells.Value(0)
	if !available || value != declaredDefault {
		t.Fatalf("Value(0) = %d %t, want the declared default present", value, available)
	}

	explicit, explicitOK := stagedReadPolicy[uint64](target, target.row, ReadContract{})
	if !explicitOK {
		t.Fatalf("explicit policy was refused: %+v", explicit)
	}
	if cell := summaryCellFrom(explicit, 0, false); cell.present {
		t.Fatal("explicit sparsity lost the unwritten distinction")
	}
}

// TestOpaqueAlternativeWidensToFactorTopOrRefusesByDeclaration proves the
// widening clause: opacity is evidence a locator reports, and the read's
// declaration alone decides whether the engine substitutes the Factor's Top or
// refuses. A rule never branches on it.
func TestOpaqueAlternativeWidensToFactorTopOrRefusesByDeclaration(t *testing.T) {
	const declaredTop = uint64(1 << 40)
	target := routeSetLawFactor{unit: routeSetLawUnit(t), row: &schemaFactorBindingCell[uint64, uint64]{}, zero: 9, top: declaredTop}

	widening, admitted := stagedReadPolicy[uint64](target, target.row, ReadContract{OnOpaque: ReadOpaqueWiden})
	if !admitted {
		t.Fatalf("widening policy = %+v admitted=%t", widening, admitted)
	}
	if unopaque := summaryCellFrom(widening, 3, true); unopaque.value != 3 || !unopaque.present {
		t.Fatalf("a widening declaration substituted Top before an opaque alternative was reported: %+v", unopaque)
	}
	opaque := widening.Widen()
	for _, sample := range []summaryCell[uint64]{summaryCellFrom(opaque, 3, true), summaryCellFrom(opaque, 0, false)} {
		if !sample.present || sample.value != declaredTop {
			t.Fatalf("widened read delivered %+v, want the Factor Top present", sample)
		}
	}

	sink := stagedRouteSink[uint64, uint64]{target: target}
	if !sink.accept(emittedOpaque{}) || !sink.opaque {
		t.Fatal("the route sink did not record an opaque alternative")
	}
	if len(sink.routes) != 0 {
		t.Fatal("an opaque alternative fabricated a route")
	}
	if !sink.accept(emittedRoute[uint64]{ref: routeSetLawRef(1), tag: 1}) || len(sink.routes) != 1 {
		t.Fatal("an opaque alternative blocked the addressable routes beside it")
	}

	if (ReadContract{OnOpaque: ReadOpaqueWiden}).exactValid() {
		t.Fatal("an exact read with no alternative set admitted a widening declaration")
	}
	if (ReadContract{OnOpaque: ReadOpaque(7)}).valid() {
		t.Fatal("an unnamed opaque disposition was admitted")
	}
}

// TestByTagOrderRanksMembersByTagAndRefusesDuplicates proves the order clause:
// under ReadOrderByTag the presentation ordinal is the rank of a member's own
// tag whatever the canonical Unit order was, and two members sharing a tag admit
// no order at all.
func TestByTagOrderRanksMembersByTagAndRefusesDuplicates(t *testing.T) {
	session := &typedStagedSelectionSession[uint64, uint64, uint64]{}
	routes := []stagedRoute[uint64]{{tag: 30}, {tag: 10}, {tag: 20}}

	canonical, ordered := session.orderMembers(routes, ReadOrderCanonical)
	if !ordered || len(canonical) != 3 {
		t.Fatalf("canonical order = %v ok=%t", canonical, ordered)
	}
	for position, routeIndex := range canonical {
		if routeIndex != position {
			t.Fatalf("canonical order permuted the engine's own route order: %v", canonical)
		}
	}

	byTag, ordered := session.orderMembers(routes, ReadOrderByTag)
	if !ordered || len(byTag) != 3 {
		t.Fatalf("by-tag order = %v ok=%t", byTag, ordered)
	}
	previous := uint64(0)
	for position, routeIndex := range byTag {
		tag := routes[routeIndex].tag
		if tag <= previous {
			t.Fatalf("by-tag member %d carried tag %d after %d", position, tag, previous)
		}
		previous = tag
	}

	duplicate := []stagedRoute[uint64]{{tag: 10}, {tag: 10}}
	if _, ordered := session.orderMembers(duplicate, ReadOrderByTag); ordered {
		t.Fatal("a by-tag read admitted two members with one tag")
	}
	if _, ordered := session.orderMembers(routes, ReadOrder(7)); ordered {
		t.Fatal("an unnamed member order was admitted")
	}
}

// TestSelectionMemberByTagResolvesUnderBothOrders proves the tag-ordinal lookup
// itself: a rule names the member it means, and a canonical selection that
// repeats a tag names no single member rather than silently picking one.
func TestSelectionMemberByTagResolvesUnderBothOrders(t *testing.T) {
	ranked := &typedStagedSelectionSession[uint64, uint64, uint64]{
		tagOrdered: true,
		values: [][]stagedSelectionValue[uint64, uint64]{{
			{tag: 10, value: 100}, {tag: 20, value: 200}, {tag: 30, value: 300},
		}},
	}
	for tag, want := range map[uint64]uint64{10: 100, 20: 200, 30: 300} {
		value, found := ranked.memberByTag(0, tag)
		if !found || value != want {
			t.Fatalf("ranked member %d = %d %t, want %d", tag, value, found, want)
		}
	}
	if _, found := ranked.memberByTag(0, 25); found {
		t.Fatal("a ranked selection resolved a tag no member carries")
	}
	if _, found := ranked.memberByTag(1, 10); found {
		t.Fatal("a ranked selection resolved a row it does not own")
	}

	canonical := &typedStagedSelectionSession[uint64, uint64, uint64]{
		values: [][]stagedSelectionValue[uint64, uint64]{{
			{tag: 30, value: 300}, {tag: 10, value: 100}, {tag: 10, value: 111},
		}},
	}
	if value, found := canonical.memberByTag(0, 30); !found || value != 300 {
		t.Fatalf("canonical member 30 = %d %t", value, found)
	}
	if _, found := canonical.memberByTag(0, 10); found {
		t.Fatal("a canonical selection resolved an ambiguous repeated tag")
	}
}

// TestForeignOwnerReadRefusesUnderItsOwnName proves the owner fence: the read
// boundary authenticates the Factor once, and a Factor that is not the one the
// sealed row owns refuses under the name read/foreign-owner. Every rule can then
// drop its own value self-equality check.
func TestForeignOwnerReadRefusesUnderItsOwnName(t *testing.T) {
	binding, _, foreign, rule, leftSlot, _ := bindDirectSelectedRuleLaw(t)
	if _, ok := BindSelectedRuleDirectExactRead[uint64, uint64, struct{}, uint64](binding, rule, leftSlot, foreign.Ref(), func(struct{}) (uint64, bool) { return 0, true }); ok {
		t.Fatal("a read bound against a Factor its sealed row does not own")
	}
	if !binding.Poisoned() {
		t.Fatal("a foreign-owner read left the binding usable")
	}
	refusal, named := binding.Refusal()
	if !named || refusal != readForeignOwnerRefusal {
		t.Fatalf("foreign-owner refusal = %q %t, want %q", refusal, named, readForeignOwnerRefusal)
	}
}

// TestMalformedReadContractRefusesUnderItsOwnName keeps an undeclarable clause
// out of the sealed binding rather than letting a read carry a delivery the
// boundary cannot honour.
func TestMalformedReadContractRefusesUnderItsOwnName(t *testing.T) {
	binding, factor, _, rule, leftSlot, _ := bindDirectSelectedRuleLaw(t)
	if _, ok := BindSelectedRuleDirectExactReadUnderContract[uint64, uint64, struct{}, uint64](binding, rule, leftSlot, factor.Ref(), func(struct{}) (uint64, bool) { return 0, true }, ReadContract{Order: ReadOrderByTag}); ok {
		t.Fatal("an exact read accepted a member-order declaration it cannot honour")
	}
	refusal, named := binding.Refusal()
	if !binding.Poisoned() || !named || refusal != readContractRefusal {
		t.Fatalf("contract refusal = %q %t poisoned=%t", refusal, named, binding.Poisoned())
	}
}

// TestReadContractIsTotalOverItsDeclaredClauses keeps the contract closed: every
// clause value is either one the engine names or one it refuses, so no read can
// carry a declaration the boundary silently ignores.
func TestReadContractIsTotalOverItsDeclaredClauses(t *testing.T) {
	if !(ReadContract{}).valid() || !(ReadContract{}).exactValid() {
		t.Fatal("the zero contract is not the engine's original delivery")
	}
	for _, contract := range []ReadContract{
		{Order: ReadOrder(9)},
		{Sparse: ReadSparse(9)},
		{OnOpaque: ReadOpaque(9)},
	} {
		if contract.valid() {
			t.Fatalf("unnamed clause admitted: %+v", contract)
		}
	}
	if !(ReadContract{Order: ReadOrderByTag, Sparse: ReadSparseFactorDefault, OnOpaque: ReadOpaqueWiden}).valid() {
		t.Fatal("the fully declared contract was refused")
	}
	if (ReadContract{Order: ReadOrderByTag}).exactValid() {
		t.Fatal("an exact read admitted a member-order declaration it cannot honour")
	}
	if !(ReadContract{Sparse: ReadSparseFactorDefault}).exactValid() {
		t.Fatal("an exact read refused the sparse clause it can honour")
	}
}
