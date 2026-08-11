package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
)

func TestStaticIdealComplementRanksNamedStrictEdges(t *testing.T) {
	p, _, authority := sealedStatic(t, `
type Literal = typeof("literal")
type Primitive = string
type Narrow = {left: string, right: number}
type Broad = {left: string}
type Record = {field: string}
type Dynamic = any
`)
	value := func(name string) Value {
		_, term := aliasNamed(t, p, name)
		return resultFor(t, authority, p, term)
	}
	literal, primitive := value("Literal"), value("Primitive")
	narrow, broad := value("Narrow"), value("Broad")
	record := value("Record")
	table, tableOK := authority.RuntimeTypeOf(runtimekind.Bit(runtimekind.Table))
	for _, law := range []struct {
		name        string
		left, right Value
	}{
		{name: "literal-to-primitive", left: literal, right: primitive},
		{name: "narrow-record-to-broad", left: narrow, right: broad},
	} {
		if authority.Equal(law.left, law.right) || !authority.LessOrEq(law.left, law.right) {
			t.Fatalf("%s is not a strict Static edge", law.name)
		}
		if authority.WidenRank(law.left) <= authority.WidenRank(law.right) {
			t.Fatalf("%s rank did not descend: %d -> %d", law.name, authority.WidenRank(law.left), authority.WidenRank(law.right))
		}
	}
	if !tableOK {
		t.Fatal("table RuntimeTypeOf value unavailable")
	}
	recordClass, recordClassOK := authority.Classes().ClassForStatic(record)
	tableClass, tableClassOK := authority.Classes().ClassForStatic(table)
	if !recordClassOK || !tableClassOK || authority.Classes().Equal(recordClass, tableClass) || !authority.Classes().LessOrEq(recordClass, tableClass) {
		t.Fatal("record-to-table is not a strict canonical ClassSet edge")
	}
	if authority.Classes().Rank(recordClass) <= authority.Classes().Rank(tableClass) {
		t.Fatalf("record-to-table ClassSet rank did not descend: %d -> %d", authority.Classes().Rank(recordClass), authority.Classes().Rank(tableClass))
	}

	var dynamic, unknown Value
	for index := 2; index < len(authority.results); index++ {
		candidate := Value{owner: authority, index: uint32(index)}
		closed, ok := authority.ClosedType(candidate)
		if !ok {
			continue
		}
		if typ.IsAny(closed) {
			dynamic = candidate
		}
		if typ.IsUnknown(closed) {
			unknown = candidate
		}
	}
	if !authority.Owns(dynamic) || !authority.Owns(unknown) || !authority.LessOrEq(dynamic, unknown) || authority.LessOrEq(unknown, dynamic) {
		t.Fatal("directed exact Any/Unknown edge unavailable")
	}
	if authority.WidenRank(dynamic) <= authority.WidenRank(unknown) {
		t.Fatalf("exact Any/Unknown tie digit did not descend: %d -> %d", authority.WidenRank(dynamic), authority.WidenRank(unknown))
	}
	for _, admitted := range []Value{literal, primitive, narrow, broad, record, table, dynamic, unknown} {
		if authority.WidenRank(authority.Bottom()) <= authority.WidenRank(admitted) || authority.WidenRank(admitted) <= authority.WidenRank(authority.Top()) {
			t.Fatal("Bottom/value/Top rank fence is not strict")
		}
	}
}

func TestStaticIdealComplementExhaustsSmallAntichains(t *testing.T) {
	p, _, authority := sealedStatic(t, `
type A = {a: string}
type B = {b: number}
type C = {c: boolean}
`)
	classes := authority.Classes()
	base := make([]Class, 3)
	for index, name := range []string{"A", "B", "C"} {
		_, term := aliasNamed(t, p, name)
		value := resultFor(t, authority, p, term)
		var ok bool
		base[index], ok = classes.ClassForStatic(value)
		if !ok {
			t.Fatalf("class %s unavailable", name)
		}
	}
	fromMask := func(mask int, reverse bool) Class {
		var result Class
		for step := 0; step < len(base); step++ {
			index := step
			if reverse {
				index = len(base) - 1 - step
			}
			if mask&(1<<index) == 0 {
				continue
			}
			if !classes.owns(result) {
				result = base[index]
			} else {
				result = classes.Join(result, base[index])
			}
		}
		return result
	}
	antichains := make([]Class, 0, 7)
	for mask := 1; mask < 1<<len(base); mask++ {
		forward, reverse := fromMask(mask, false), fromMask(mask, true)
		if !classes.Equal(forward, reverse) || classes.Rank(forward) != classes.Rank(reverse) {
			t.Fatalf("antichain %03b depends on permutation", mask)
		}
		atoms, ok := classes.classAtoms(forward)
		want, ranked := classes.descriptorIdealRank(atoms)
		if !ok || !ranked || classes.Rank(forward) != want {
			t.Fatalf("antichain %03b rank = %d, want ideal-complement %d/%t", mask, classes.Rank(forward), want, ranked)
		}
		antichains = append(antichains, forward)
	}
	antichains = append(antichains, classes.AnyValue())
	equal := make([][]bool, len(antichains))
	order := make([][]bool, len(antichains))
	joins := make([][]int, len(antichains))
	ranks := make([]uint64, len(antichains))
	for leftIndex, left := range antichains {
		equal[leftIndex] = make([]bool, len(antichains))
		order[leftIndex] = make([]bool, len(antichains))
		joins[leftIndex] = make([]int, len(antichains))
		ranks[leftIndex] = classes.Rank(left)
		for rightIndex, right := range antichains {
			equal[leftIndex][rightIndex] = classes.Equal(left, right)
			order[leftIndex][rightIndex] = classes.LessOrEq(left, right)
			joined := classes.Join(left, right)
			joins[leftIndex][rightIndex] = -1
			for candidateIndex, candidate := range antichains {
				if classes.Equal(joined, candidate) {
					joins[leftIndex][rightIndex] = candidateIndex
					break
				}
			}
			if joins[leftIndex][rightIndex] < 0 {
				t.Fatalf("join %d,%d is not an upper bound", leftIndex, rightIndex)
			}
		}
	}
	for leftIndex := range antichains {
		if !equal[joins[leftIndex][leftIndex]][leftIndex] {
			t.Fatalf("join idempotence failed at %d", leftIndex)
		}
		for rightIndex := range antichains {
			if !equal[leftIndex][rightIndex] && order[leftIndex][rightIndex] && ranks[leftIndex] <= ranks[rightIndex] {
				t.Fatalf("strict antichain %d <= %d did not descend", leftIndex, rightIndex)
			}
			joined := joins[leftIndex][rightIndex]
			if !equal[joined][joins[rightIndex][leftIndex]] {
				t.Fatalf("join commutativity failed at %d,%d", leftIndex, rightIndex)
			}
			if !order[leftIndex][joined] || !order[rightIndex][joined] {
				t.Fatalf("join %d,%d is not an upper bound", leftIndex, rightIndex)
			}
			for upperIndex := range antichains {
				if order[leftIndex][upperIndex] && order[rightIndex][upperIndex] && !order[joined][upperIndex] {
					t.Fatalf("join %d,%d is not least below %d", leftIndex, rightIndex, upperIndex)
				}
			}
			if !equal[joined][leftIndex] && ranks[joined] >= ranks[leftIndex] {
				t.Fatalf("antichain join %d,%d did not descend from left", leftIndex, rightIndex)
			}
			if !equal[joined][rightIndex] && ranks[joined] >= ranks[rightIndex] {
				t.Fatalf("antichain join %d,%d did not descend from right", leftIndex, rightIndex)
			}
			for thirdIndex := range antichains {
				leftAssociated := joins[joined][thirdIndex]
				rightAssociated := joins[leftIndex][joins[rightIndex][thirdIndex]]
				if !equal[leftAssociated][rightAssociated] {
					t.Fatalf("join associativity failed at %d,%d,%d", leftIndex, rightIndex, thirdIndex)
				}
			}
		}
	}
}

func TestStaticIdealComplementRanksReplayIdentically(t *testing.T) {
	const source = `
type Left = {left: string}
type Right = {right: number}
`
	leftProgram, _, leftAuthority := sealedStatic(t, source)
	rightProgram, _, rightAuthority := sealedStatic(t, source)
	read := func(p *program.Program, authority *Authority, name string) Value {
		_, term := aliasNamed(t, p, name)
		return resultFor(t, authority, p, term)
	}
	leftA, leftB := read(leftProgram, leftAuthority, "Left"), read(leftProgram, leftAuthority, "Right")
	rightA, rightB := read(rightProgram, rightAuthority, "Left"), read(rightProgram, rightAuthority, "Right")
	if leftAuthority.WidenRank(leftA) != rightAuthority.WidenRank(rightA) || leftAuthority.WidenRank(leftB) != rightAuthority.WidenRank(rightB) ||
		leftAuthority.WidenRank(leftAuthority.Join(leftA, leftB)) != rightAuthority.WidenRank(rightAuthority.Join(rightB, rightA)) {
		t.Fatal("ideal-complement ranks changed across replay or join permutation")
	}
}

func TestClassSetJoinConstructsStableDerivedUnion(t *testing.T) {
	p, _, authority := sealedStatic(t, `
type Left = {left: string}
type Right = {right: number}
`)
	_, leftTerm := aliasNamed(t, p, "Left")
	_, rightTerm := aliasNamed(t, p, "Right")
	left, leftOK := authority.Classes().ClassForStatic(resultFor(t, authority, p, leftTerm))
	right, rightOK := authority.Classes().ClassForStatic(resultFor(t, authority, p, rightTerm))
	if !leftOK || !rightOK {
		t.Fatal("record classes were not admitted")
	}
	classes := authority.Classes()
	joined := classes.Join(left, right)
	if classes.Equal(joined, classes.AnyValue()) {
		t.Fatal("missing normalized union fell back to AnyValue")
	}
	if !classes.LessOrEq(left, joined) || !classes.LessOrEq(right, joined) {
		t.Fatal("derived union is not an upper bound")
	}
	if !classes.Equal(joined, classes.Join(right, left)) {
		t.Fatal("derived union identity depends on operand order")
	}
	firstID, firstOK := classes.Identity(joined)
	secondID, secondOK := classes.Identity(classes.Join(left, right))
	if !firstOK || !secondOK || firstID != secondID {
		t.Fatal("derived union identity depends on descriptor allocation")
	}
	if kind, kindOK := classes.Kind(joined); !kindOK || kind != ClassDerived {
		t.Fatalf("derived union kind = %v/%v", kind, kindOK)
	}
}

func TestStaticJoinCarriesExactDerivedUnion(t *testing.T) {
	p, _, authority := sealedStatic(t, `
type Left = {left: string}
type Right = {right: number}
`)
	_, leftTerm := aliasNamed(t, p, "Left")
	_, rightTerm := aliasNamed(t, p, "Right")
	left := resultFor(t, authority, p, leftTerm)
	right := resultFor(t, authority, p, rightTerm)
	joined := authority.Join(left, right)
	if !authority.Owns(joined) || !joined.IsClosed() || authority.Equal(joined, authority.Top()) {
		t.Fatal("Static derived union was erased to Top")
	}
	if !authority.LessOrEq(left, joined) || !authority.LessOrEq(right, joined) {
		t.Fatal("Static derived union is not an upper bound")
	}
	if !authority.LessOrEq(authority.Bottom(), joined) || !authority.LessOrEq(joined, authority.Top()) {
		t.Fatal("derived union broke Bottom/Top lattice fences")
	}
	reversed := authority.Join(right, left)
	if !authority.Equal(joined, reversed) || authority.Fingerprint(joined) != authority.Fingerprint(reversed) {
		t.Fatal("Static derived union is not stable under permutation")
	}
	decoded, ok := authority.ClosedType(joined)
	leftType, leftOK := authority.ClosedType(left)
	rightType, rightOK := authority.ClosedType(right)
	want := typeexpr.Union(leftType, rightType)
	if !leftOK || !rightOK {
		t.Fatal("source record ClosedType unavailable")
	}
	if !ok || !typ.TypeEquals(decoded, want) {
		t.Fatalf("Static derived ClosedType = %v/%v, want exact normalized union", decoded, ok)
	}
	if authority.WidenRank(joined) >= authority.WidenRank(left) || authority.WidenRank(joined) >= authority.WidenRank(right) {
		t.Fatal("derived union rank did not descend")
	}
}

func TestClassIdentityUsesPortableSemanticDescriptor(t *testing.T) {
	classID := func(source, name string) (keyspace.ContentID, Class, *ClassSet) {
		p, _, authority := sealedStatic(t, source)
		_, term := aliasNamed(t, p, name)
		class, ok := authority.Classes().ClassForStatic(resultFor(t, authority, p, term))
		if !ok {
			t.Fatal("class")
		}
		id, ok := authority.Classes().Identity(class)
		if !ok {
			t.Fatal("class identity")
		}
		return id, class, authority.Classes()
	}
	plainID, _, _ := classID("type Kept = {value: string}", "Kept")
	insertedID, _, _ := classID("type Earlier = {other: number}\ntype Kept = {value: string}", "Kept")
	if plainID != insertedID {
		t.Fatal("unrelated earlier class insertion changed portable Class identity")
	}
	changedID, _, _ := classID("type Kept = {value: number}", "Kept")
	if plainID == changedID {
		t.Fatal("descriptor mutation retained Class identity")
	}

	p, _, authority := sealedStatic(t, "type Left = {left: string}\ntype Right = {right: number}\ntype Opaque = typeof(missing)")
	_, leftTerm := aliasNamed(t, p, "Left")
	_, rightTerm := aliasNamed(t, p, "Right")
	_, opaqueTerm := aliasNamed(t, p, "Opaque")
	classes := authority.Classes()
	left, _ := classes.ClassForStatic(resultFor(t, authority, p, leftTerm))
	right, _ := classes.ClassForStatic(resultFor(t, authority, p, rightTerm))
	opaque, opaqueOK := classes.ClassForStatic(resultFor(t, authority, p, opaqueTerm))
	if !opaqueOK {
		t.Fatal("opaque class")
	}
	derived := classes.Join(left, right)
	concreteID, _ := classes.Identity(left)
	opaqueID, _ := classes.Identity(opaque)
	derivedID, _ := classes.Identity(derived)
	if concreteID == opaqueID || concreteID == derivedID || opaqueID == derivedID {
		t.Fatal("concrete, opaque, and derived Classes aliased")
	}
}

func TestStaticRankOrientationDescendsTowardTop(t *testing.T) {
	p, _, authority := sealedStatic(t, `
type Left = {left: string}
type Right = {right: number}
`)
	_, leftTerm := aliasNamed(t, p, "Left")
	_, rightTerm := aliasNamed(t, p, "Right")
	left := resultFor(t, authority, p, leftTerm)
	right := resultFor(t, authority, p, rightTerm)
	joined := authority.Join(left, right)
	if authority.WidenRank(authority.Top()) != 0 {
		t.Fatal("Top is not the minimum widening rank")
	}
	if authority.WidenRank(authority.Bottom()) <= authority.WidenRank(left) || authority.WidenRank(authority.Bottom()) <= authority.WidenRank(right) {
		t.Fatal("Bottom is not above every admitted value")
	}
	if authority.WidenRank(joined) >= authority.WidenRank(left) || authority.WidenRank(joined) >= authority.WidenRank(right) {
		t.Fatal("strict semantic union did not descend")
	}
}

func TestClassSetJoinRankDescendsForEverySealedPair(t *testing.T) {
	_, _, authority := sealedStatic(t, `
type NilValue = nil
type IntegerValue = integer
type NumberValue = number
type StringValue = string
type Left = {left: string}
type Right = {right: number}
`)
	classes := authority.Classes()
	for leftIndex := range classes.descriptors {
		left := Class{owner: classes, index: uint32(leftIndex)}
		for rightIndex := range classes.descriptors {
			right := Class{owner: classes, index: uint32(rightIndex)}
			joined := classes.Join(left, right)
			if !classes.Equal(left, joined) && classes.Rank(joined) >= classes.Rank(left) {
				t.Fatalf("join(%d,%d) rank %d did not descend from left %d", leftIndex, rightIndex, classes.Rank(joined), classes.Rank(left))
			}
			if !classes.Equal(right, joined) && classes.Rank(joined) >= classes.Rank(right) {
				t.Fatalf("join(%d,%d) rank %d did not descend from right %d", leftIndex, rightIndex, classes.Rank(joined), classes.Rank(right))
			}
		}
	}
}

func TestClassSetRankStrictlyOrdersEverySealedSubtype(t *testing.T) {
	_, _, authority := sealedStatic(t, `
type Narrow = {left: string, right: number}
type Broad = {left: string}
type NarrowValue = {left: string, right: integer}
type FunctionNarrow = (value: any) -> string
type FunctionBroad = (value: string) -> string
`)
	classes := authority.Classes()
	for leftIndex := range classes.descriptors {
		left := Class{owner: classes, index: uint32(leftIndex)}
		for rightIndex := range classes.descriptors {
			right := Class{owner: classes, index: uint32(rightIndex)}
			if classes.Equal(left, right) || !classes.LessOrEq(left, right) {
				continue
			}
			if classes.Rank(left) <= classes.Rank(right) {
				t.Fatalf("strict ClassSet subtype rank did not descend: %d(%d) %s <= %d(%d) %s", leftIndex, classes.Rank(left), classRankName(authority, uint32(leftIndex)), rightIndex, classes.Rank(right), classRankName(authority, uint32(rightIndex)))
			}
		}
	}
}

func TestStaticClosedValueClosedTypeIsTotal(t *testing.T) {
	p, _, authority := sealedStatic(t, `
type Left = {left: string}
type Right = {right: number}
type Choice = string | number
`)
	for index := 0; index < authority.CoordinateCount(); index++ {
		coordinate, ok := authority.CoordinateAt(index)
		if !ok {
			continue
		}
		value, ok := authority.Result(coordinate)
		if !ok || !value.IsClosed() {
			continue
		}
		if _, ok := authority.ClosedType(value); !ok {
			t.Fatalf("closed Value %d has no ClosedType", index)
		}
	}
	_, leftTerm := aliasNamed(t, p, "Left")
	_, rightTerm := aliasNamed(t, p, "Right")
	joined := authority.Join(resultFor(t, authority, p, leftTerm), resultFor(t, authority, p, rightTerm))
	if !joined.IsClosed() {
		t.Fatal("derived union lost closed carrier")
	}
	if _, ok := authority.ClosedType(joined); !ok {
		t.Fatal("derived closed Value has no ClosedType")
	}
}

func classRankName(authority *Authority, index uint32) string {
	for resultIndex := 2; resultIndex < len(authority.results); resultIndex++ {
		if uint64(resultIndex) >= uint64(len(authority.valueClasses)) || authority.valueClasses[resultIndex].index != index {
			continue
		}
		if value, ok := authority.ClosedType(Value{owner: authority, index: uint32(resultIndex)}); ok {
			return value.String()
		}
	}
	return "<opaque>"
}
