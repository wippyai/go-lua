package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	proglink "github.com/wippyai/go-lua/program/link"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	linkstatic "github.com/wippyai/go-lua/program/link/static"
	programlower "github.com/wippyai/go-lua/program/lower"
	programstatic "github.com/wippyai/go-lua/program/static"
	"github.com/wippyai/go-lua/program/target"
)

const staticLawSource = `
local subject = 1
type NumberAlias = number
type IntegerAlias = integer
type StringAlias = string
type NumberOrString = number | string
type Keys = keyof({foo: string})
type Field = {foo: string}["foo"]
type Choice = integer extends number ? string : boolean
type Snapshot = typeof(subject)
type LiteralSnapshot = typeof("literal")
type Node = { next: Node? }
`

func sealedStatic(t *testing.T, source string) (*program.Program, *proglink.Link, *Authority) {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: "static_law.lua", Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{
		InitialRoots: []target.InitialRootSpec{{
			Identity: "GlobalEnvRoot",
			Shape: target.BootShapeSpec{
				Aggregate: target.BootAggregateTable,
				Value:     target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"},
			},
		}},
		InitialEntries: []target.InitialEntrySpec{{
			Root:       "GlobalEnvRoot",
			Key:        keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"},
			Value:      target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"},
			Mutability: target.InitialMutable,
		}, {
			Root: "GlobalEnvRoot",
			Key:  keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__static_absent"},
			Value: target.InitialValueSpec{
				Kind: target.InitialValueAbsent,
			},
			Mutability: target.InitialMutable,
		}},
		InitialBindings: []target.InitialBindingSpec{{
			Name: "_G",
			Root: "GlobalEnvRoot",
			Key:  keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := proglink.Seal(&proglink.Spec{Target: contract, Modules: []linkproject.Module{{Name: "static_law", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	types, ok := typeauthority.Seal(linked)
	if !ok {
		t.Fatal("type authority")
	}
	static, _, err := Seal(linked, types)
	if err != nil {
		t.Fatal(err)
	}
	return p, linked, static
}

func TestStaticSealFencesIndependentlySealedSameContentTypeAuthority(t *testing.T) {
	p, err := programlower.Lower(programlower.Source{Name: "static_fence.lua", Text: []byte("type Value = string\n")})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	sealLink := func() *proglink.Link {
		linked, err := proglink.Seal(&proglink.Spec{
			Target:  contract,
			Modules: []linkproject.Module{{Name: "static_fence", Program: p}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return linked
	}
	left, right := sealLink(), sealLink()
	if left.ContentID() != right.ContentID() || left == right {
		t.Fatal("same-content fixture did not retain distinct Link owners")
	}
	leftTypes, ok := typeauthority.Seal(left)
	if !ok {
		t.Fatal("left type authority")
	}
	rightTypes, ok := typeauthority.Seal(right)
	if !ok {
		t.Fatal("right type authority")
	}
	if leftTypes.Link() != left || rightTypes.Link() != right {
		t.Fatal("type authority lost its exact Link owner")
	}
	if _, _, err := Seal(left, rightTypes); err == nil {
		t.Fatal("same-content foreign type authority crossed Static seal")
	}
	if _, _, err := Seal(left, leftTypes); err != nil {
		t.Fatalf("local type authority rejected: %v", err)
	}
	if _, _, err := Seal(right, rightTypes); err != nil {
		t.Fatalf("second local type authority rejected: %v", err)
	}

	encoded, err := proglink.EncodeArtifact(left)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := proglink.DecodeArtifact(encoded, contract, map[keyspace.ContentID]*program.Program{p.ContentID(): p})
	if err != nil {
		t.Fatal(err)
	}
	replayedTypes, ok := typeauthority.Seal(replayed)
	if !ok {
		t.Fatal("replayed type authority")
	}
	if _, _, err := Seal(replayed, replayedTypes); err != nil {
		t.Fatalf("replayed local type authority rejected: %v", err)
	}
}

func aliasNamed(t *testing.T, p *program.Program, name string) (keyspace.Term, keyspace.Term) {
	t.Helper()
	aliases := p.Static().Declarations().Aliases()
	for index := 0; index < aliases.Count(); index++ {
		alias, _ := aliases.At(index)
		_, target, key, _, ok := aliases.Get(alias)
		if !ok {
			continue
		}
		literal, ok := p.Source().Keys().Exact(key)
		if ok && literal.Kind == keyspace.LiteralString && literal.String == name {
			return alias, target
		}
	}
	t.Fatalf("missing alias %q", name)
	return 0, 0
}

func resultFor(t *testing.T, authority *Authority, p *program.Program, term keyspace.Term) Value {
	t.Helper()
	if _, ok := p.Static().StaticTypes().Ref(term); !ok {
		t.Fatal("invalid static reference")
	}
	for index, row := range authority.coordinates {
		if row.key.reference.Owner() != p.ContentID() || row.key.reference.Root() != term {
			continue
		}
		coordinate, ok := authority.CoordinateAt(index)
		if !ok {
			continue
		}
		value, ok := authority.Result(coordinate)
		if ok {
			return value
		}
	}
	{
		t.Fatalf("missing result for %v", term)
	}
	return Value{}
}

func TestStaticAuthorityEvaluatesDependentForestAndExactRuntimeBoundary(t *testing.T) {
	p, linked, authority := sealedStatic(t, staticLawSource)
	if !authority.ContentID().Available() || authority.LinkID() != linked.ContentID() {
		t.Fatal("authority identity")
	}

	for name, expected := range map[string]typ.Type{
		"Keys":            typ.LiteralString("foo"),
		"Field":           typ.String,
		"Choice":          typ.String,
		"LiteralSnapshot": typ.LiteralString("literal"),
	} {
		alias, _ := aliasNamed(t, p, name)
		result := resultFor(t, authority, p, alias)
		actual, ok := authority.ClosedType(result)
		if !ok || !subtype.IsSubtype(actual, expected) || !subtype.IsSubtype(expected, actual) {
			t.Fatalf("%s = %v/%v", name, actual, ok)
		}
	}

	snapshot, _ := aliasNamed(t, p, "Snapshot")
	residue, ok := resultFor(t, authority, p, snapshot).Symbolic()
	if !ok || residue.Reason() != ReasonRuntimeSubject {
		t.Fatalf("snapshot residue = %#v/%v", residue, ok)
	}
	subject, ok := residue.Subject()
	if !ok || subject.LinkID() != linked.ContentID() {
		t.Fatal("runtime subject owner")
	}
	value, ok := subject.Value()
	if !ok {
		t.Fatal("runtime subject value")
	}
	shard, term, ok := linked.Boundary().Values().Origin(value)
	if !ok {
		t.Fatal("runtime subject origin")
	}
	origin, _ := linked.Project().Mounts().Program(shard)
	if _, _, _, ok := origin.Flow().Authored().Storage().Cells().Get(term); !ok {
		t.Fatalf("typeof subject origin is not Cell: %v", term)
	}
	if body, cursor, ok := subject.SourceFrontier(); !ok || body == 0 || cursor < 0 {
		t.Fatalf("typeof subject has no exact source frontier: %v/%d/%v", body, cursor, ok)
	}
	node, _ := aliasNamed(t, p, "Node")
	nodeType, ok := authority.ClosedType(resultFor(t, authority, p, node))
	if !ok || typ.ValidateStaticGenericRecurrence(nodeType) != nil {
		t.Fatal("productive recursive alias did not remain a finite closed Mu graph")
	}
}

func TestStaticCompatibilityUsesClosedOwnerHandlesAndTwoDistinctJudgments(t *testing.T) {
	p, _, authority := sealedStatic(t, `
type Dynamic = any
type UnknownAlias = unknown
type Text = string
type IntegerAlias = integer
type NumberAlias = number
type Box<T: string> = { value: T }
type AppliedBox = Box<string>
type BoxShape = { value: string }
type BroadFunction = (value: number) -> integer
type NarrowFunction = (value: integer) -> number
type ReceiverBroad = (self: number) -> integer
type ReceiverNarrow = (self: integer) -> number
type Invalid = keyof(number)
`)
	integerAlias, _ := aliasNamed(t, p, "IntegerAlias")
	numberAlias, _ := aliasNamed(t, p, "NumberAlias")
	textAlias, _ := aliasNamed(t, p, "Text")
	dynamicAlias, _ := aliasNamed(t, p, "Dynamic")
	unknownAlias, _ := aliasNamed(t, p, "UnknownAlias")
	appliedBoxAlias, _ := aliasNamed(t, p, "AppliedBox")
	boxShapeAlias, _ := aliasNamed(t, p, "BoxShape")
	broadFunctionAlias, _ := aliasNamed(t, p, "BroadFunction")
	narrowFunctionAlias, _ := aliasNamed(t, p, "NarrowFunction")
	receiverBroadAlias, _ := aliasNamed(t, p, "ReceiverBroad")
	receiverNarrowAlias, _ := aliasNamed(t, p, "ReceiverNarrow")
	invalidAlias, _ := aliasNamed(t, p, "Invalid")
	integer := resultFor(t, authority, p, integerAlias)
	number := resultFor(t, authority, p, numberAlias)
	text := resultFor(t, authority, p, textAlias)
	dynamic := resultFor(t, authority, p, dynamicAlias)
	unknown := resultFor(t, authority, p, unknownAlias)
	appliedBox := resultFor(t, authority, p, appliedBoxAlias)
	boxShape := resultFor(t, authority, p, boxShapeAlias)
	broadFunction := resultFor(t, authority, p, broadFunctionAlias)
	narrowFunction := resultFor(t, authority, p, narrowFunctionAlias)
	receiverBroad := resultFor(t, authority, p, receiverBroadAlias)
	receiverNarrow := resultFor(t, authority, p, receiverNarrowAlias)
	invalid := resultFor(t, authority, p, invalidAlias)

	if strict, fresh := authority.Compatibility(integer, number); !strict || !fresh {
		t.Fatalf("integer -> number compatibility = %v/%v, want true/true", strict, fresh)
	}
	if strict, fresh := authority.Compatibility(number, integer); strict || fresh {
		t.Fatalf("number -> integer compatibility = %v/%v, want false/false", strict, fresh)
	}
	if strict, fresh := authority.Compatibility(dynamic, text); strict || fresh {
		t.Fatalf("any -> string compatibility = %v/%v, want false/false", strict, fresh)
	}
	if strict, fresh := authority.Compatibility(dynamic, unknown); !strict || !fresh {
		t.Fatalf("any -> unknown compatibility = %v/%v, want true/true", strict, fresh)
	}
	if strict, fresh := authority.Compatibility(unknown, dynamic); !strict || !fresh {
		t.Fatalf("unknown -> any compatibility = %v/%v, want true/true", strict, fresh)
	}
	if strict, fresh := authority.Compatibility(text, unknown); !strict || !fresh {
		t.Fatalf("string -> unknown compatibility = %v/%v, want true/true", strict, fresh)
	}
	if strict, fresh := authority.Compatibility(unknown, text); strict || fresh {
		t.Fatalf("unknown -> string compatibility = %v/%v, want false/false", strict, fresh)
	}
	if strict, fresh := authority.Compatibility(appliedBox, boxShape); !strict || !fresh {
		t.Fatalf("closed generic application -> structural target = %v/%v, want true/true", strict, fresh)
	}
	if strict, fresh := authority.Compatibility(boxShape, appliedBox); !strict || !fresh {
		t.Fatalf("structural target -> closed generic application = %v/%v, want true/true", strict, fresh)
	}
	if strict, fresh := authority.Compatibility(broadFunction, narrowFunction); !strict || !fresh {
		t.Fatalf("contravariant function compatibility = %v/%v, want true/true", strict, fresh)
	}
	if strict, fresh := authority.Compatibility(narrowFunction, broadFunction); strict || fresh {
		t.Fatalf("reverse function compatibility = %v/%v, want false/false", strict, fresh)
	}
	if strict, fresh := authority.Compatibility(receiverBroad, receiverNarrow); !strict || !fresh {
		t.Fatalf("explicit receiver positional compatibility = %v/%v, want true/true", strict, fresh)
	}
	if strict, fresh := authority.Compatibility(receiverNarrow, receiverBroad); strict || fresh {
		t.Fatalf("reverse explicit receiver compatibility = %v/%v, want false/false", strict, fresh)
	}
	if !invalid.IsInvalid() {
		kind, ok := invalid.Kind()
		t.Fatalf("invalid authored result kind = %v/%v", kind, ok)
	}
	for _, test := range []struct {
		name  string
		value Value
	}{
		{name: "bottom", value: authority.Bottom()},
		{name: "top", value: authority.Top()},
		{name: "invalid", value: invalid},
	} {
		if strict, fresh := authority.Compatibility(test.value, text); strict || fresh {
			t.Fatalf("%s -> string compatibility = %v/%v, want false/false", test.name, strict, fresh)
		}
		if strict, fresh := authority.Compatibility(text, test.value); strict || fresh {
			t.Fatalf("string -> %s compatibility = %v/%v, want false/false", test.name, strict, fresh)
		}
	}
	if strict, fresh := authority.Compatibility(number, number); !strict || !fresh {
		t.Fatalf("same exact handle compatibility = %v/%v, want true/true", strict, fresh)
	}
	if allocations := testing.AllocsPerRun(1_000, func() {
		if strict, fresh := authority.Compatibility(number, number); !strict || !fresh {
			t.Fatal("equal compatibility changed during allocation probe")
		}
	}); allocations != 0 {
		t.Fatalf("equal compatibility allocations = %.2f, want 0", allocations)
	}

	foreignProgram, _, foreignAuthority := sealedStatic(t, `
type Dynamic = any
type Text = string
type IntegerAlias = integer
type NumberAlias = number
`)
	foreignAlias, _ := aliasNamed(t, foreignProgram, "IntegerAlias")
	foreign := resultFor(t, foreignAuthority, foreignProgram, foreignAlias)
	if strict, fresh := authority.Compatibility(integer, foreign); strict || fresh {
		t.Fatalf("foreign same-content handle compatibility = %v/%v, want false/false", strict, fresh)
	}
}

func TestStaticAuthorityAcceptsEmptyAuthoredCoordinateFamily(t *testing.T) {
	p, err := programlower.Lower(programlower.Source{Name: "empty_static.lua", Text: []byte("return 1")})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := proglink.Seal(&proglink.Spec{Target: contract, Modules: []linkproject.Module{{Name: "empty_static", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	types, ok := typeauthority.Seal(linked)
	if !ok {
		t.Fatal("type authority")
	}
	if types.Count() != 0 {
		t.Fatalf("type authority coordinates = %d, want exact empty family", types.Count())
	}
	authority, _, err := Seal(linked, types)
	if err != nil {
		t.Fatal(err)
	}
	if authority.CoordinateCount() != 0 || len(authority.operands) != 0 {
		t.Fatalf("Static authored coordinates = %d coordinates/%d operands, want 0/0", authority.CoordinateCount(), len(authority.operands))
	}
	if _, ok := authority.CoordinateAt(0); ok {
		t.Fatal("empty Static family issued a sentinel coordinate")
	}
	classes := authority.Classes()
	if classes == nil || !classes.ContentID().Available() {
		t.Fatal("empty Static family did not seal its finite ClassSet")
	}
	if got := classes.Join(classes.Nil(), classes.AnyValue()); !classes.Equal(got, classes.AnyValue()) {
		t.Fatal("empty Static family ClassSet is not a valid lattice")
	}
}

func TestStaticAuthorityCoordinatesExposeExactAdmittedResults(t *testing.T) {
	_, _, authority := sealedStatic(t, staticLawSource)
	if authority.CoordinateCount() == 0 {
		t.Fatal("missing authored Static coordinates")
	}
	for index := 0; index < authority.CoordinateCount(); index++ {
		coordinate, ok := authority.CoordinateAt(index)
		if !ok {
			t.Fatalf("coordinate %d is absent", index)
		}
		value, ok := authority.Result(coordinate)
		if !ok || !authority.Owns(value) || authority.Fingerprint(value) == 0 {
			t.Fatalf("coordinate %d lacks an admitted Static result", index)
		}
	}
	_, _, foreign := sealedStatic(t, "type Text = string\nreturn 1")
	foreignCoordinate, ok := foreign.CoordinateAt(0)
	if !ok {
		t.Fatal("foreign Static coordinate")
	}
	if _, ok := authority.Result(foreignCoordinate); ok {
		t.Fatal("foreign Static coordinate entered authority")
	}
}

func TestStaticAuthorityFiniteJoinSelectsAdmittedSemanticUpperBound(t *testing.T) {
	p, _, authority := sealedStatic(t, staticLawSource)
	_, number := aliasNamed(t, p, "NumberAlias")
	_, integer := aliasNamed(t, p, "IntegerAlias")
	_, stringType := aliasNamed(t, p, "StringAlias")
	_, union := aliasNamed(t, p, "NumberOrString")
	numberValue := resultFor(t, authority, p, number)
	integerValue := resultFor(t, authority, p, integer)
	stringValue := resultFor(t, authority, p, stringType)
	unionValue := resultFor(t, authority, p, union)
	if got := authority.Join(integerValue, numberValue); !authority.Equal(got, numberValue) {
		t.Fatal("integer join number did not choose number")
	}
	if got := authority.Join(numberValue, stringValue); !authority.Equal(got, unionValue) {
		t.Fatal("admitted union was replaced by technical Top")
	}
	if authority.WidenRank(numberValue) <= authority.WidenRank(authority.Join(integerValue, numberValue)) && !authority.Equal(numberValue, integerValue) {
		// The join equals number, so a strict integer -> number transition must
		// reduce the well-founded rank.
		if authority.WidenRank(integerValue) <= authority.WidenRank(numberValue) {
			t.Fatal("rank does not descend on strict join")
		}
	}
}

func TestStaticClassSetIsSharedFiniteAuthority(t *testing.T) {
	p, _, authority := sealedStatic(t, staticLawSource)
	classes := authority.Classes()
	if classes == nil || !classes.ContentID().Available() {
		t.Fatal("ClassSet identity")
	}
	if !classes.CanBeNil(classes.AnyValue()) || !classes.CanBeNil(classes.Nil()) {
		t.Fatal("nil capability")
	}
	_, number := aliasNamed(t, p, "NumberAlias")
	numberClass, ok := classes.ClassForStatic(resultFor(t, authority, p, number))
	if !ok || classes.CanBeNil(numberClass) {
		t.Fatal("number class nil capability")
	}
	nilOrNumber, ok := authority.RuntimeTypeOf(runtimekind.Bit(runtimekind.Nil) | runtimekind.Bit(runtimekind.Number))
	if !ok {
		t.Fatal("nil|number runtime-kind result")
	}
	nilOrNumberClass, ok := classes.ClassForStatic(nilOrNumber)
	if !ok || !classes.Equal(classes.Join(classes.Nil(), numberClass), nilOrNumberClass) {
		t.Fatal("admitted nil|number class was not the precise join")
	}
	if classes.Rank(numberClass) == 0 {
		t.Fatal("non-top class rank")
	}
}

func TestStaticExactCarrierLiftsExtensionalClassCoverage(t *testing.T) {
	_, _, authority := sealedStatic(t, `
type Dynamic = any
type Text = string
`)
	var dynamic, unknown, text Value
	for index := 2; index < len(authority.results); index++ {
		candidate := Value{owner: authority, index: uint32(index)}
		decoded, ok := authority.ClosedType(candidate)
		if !ok {
			continue
		}
		switch {
		case typ.TypeEquals(decoded, typ.Any):
			dynamic = candidate
		case typ.TypeEquals(decoded, typ.Unknown):
			unknown = candidate
		case typ.TypeEquals(decoded, typ.String):
			text = candidate
		}
	}
	if !authority.Owns(dynamic) || !authority.Owns(unknown) || !authority.Owns(text) || authority.Equal(dynamic, unknown) {
		t.Fatal("exact any/unknown/string Static values were not retained")
	}
	classes := authority.Classes()
	dynamicClass, dynamicOK := classes.ClassForStatic(dynamic)
	unknownClass, unknownOK := classes.ClassForStatic(unknown)
	if !dynamicOK || !unknownOK || !classes.Equal(dynamicClass, unknownClass) {
		t.Fatal("mutual Runtime subtypes did not project to one Pack class")
	}
	if !authority.LessOrEq(dynamic, unknown) || authority.LessOrEq(unknown, dynamic) {
		t.Fatal("closed Unknown is not the canonical conservative class summary")
	}
	if joined := authority.Join(dynamic, unknown); !authority.Equal(joined, unknown) {
		t.Fatal("any/unknown join did not select canonical Unknown")
	}
	if joined := authority.Join(text, dynamic); !authority.Equal(joined, unknown) {
		t.Fatal("lower type/noncanonical top join lost the canonical Unknown summary")
	}

	values := make([]Value, len(authority.results))
	for index := range values {
		values[index] = Value{owner: authority, index: uint32(index)}
	}
	// Close the test-local finite carrier under every pair join exactly once.
	// Fingerprints only choose an interning bucket; exact equality resolves a
	// collision before two values share a table row.
	carrier := append([]Value(nil), values...)
	carrierBuckets := make(map[uint64][]int, len(carrier))
	for index, value := range carrier {
		fingerprint := authority.Fingerprint(value)
		carrierBuckets[fingerprint] = append(carrierBuckets[fingerprint], index)
	}
	intern := func(value Value) int {
		fingerprint := authority.Fingerprint(value)
		for _, index := range carrierBuckets[fingerprint] {
			if authority.Equal(carrier[index], value) {
				return index
			}
		}
		index := len(carrier)
		carrier = append(carrier, value)
		carrierBuckets[fingerprint] = append(carrierBuckets[fingerprint], index)
		return index
	}
	joins := make([][]int, len(values))
	for leftIndex, left := range values {
		joins[leftIndex] = make([]int, len(values))
		for rightIndex, right := range values {
			joins[leftIndex][rightIndex] = intern(authority.Join(left, right))
		}
	}
	originalToCarrier := make([][]bool, len(values))
	for leftIndex, left := range values {
		originalToCarrier[leftIndex] = make([]bool, len(carrier))
		for rightIndex, right := range carrier {
			originalToCarrier[leftIndex][rightIndex] = authority.LessOrEq(left, right)
		}
	}
	carrierToOriginal := make([][]bool, len(carrier))
	for leftIndex, left := range carrier {
		carrierToOriginal[leftIndex] = make([]bool, len(values))
		for rightIndex, right := range values {
			carrierToOriginal[leftIndex][rightIndex] = authority.LessOrEq(left, right)
		}
	}
	for leftIndex := range values {
		for rightIndex := range values {
			joined := joins[leftIndex][rightIndex]
			if !originalToCarrier[leftIndex][joined] || !originalToCarrier[rightIndex][joined] {
				t.Fatal("lifted Static join is not an upper bound")
			}
			for upperIndex := range values {
				if originalToCarrier[leftIndex][upperIndex] && originalToCarrier[rightIndex][upperIndex] && !carrierToOriginal[joined][upperIndex] {
					t.Fatal("lifted Static join is not least")
				}
			}
		}
	}

	reachable := make([]bool, len(classes.rows))
	reachable[0] = true
	for _, class := range classes.byStatic {
		reachable[class.index] = true
	}
	for _, class := range classes.byTarget {
		reachable[class.index] = true
	}
	for index, reached := range reachable {
		if !reached {
			t.Fatalf("dead ClassSet coverage row %d", index)
		}
	}
	_, _, replay := sealedStatic(t, `
type Dynamic = any
type Text = string
`)
	if replay.ContentID() != authority.ContentID() || replay.Classes().ContentID() != classes.ContentID() {
		t.Fatal("extensional ClassSet changed across reseal")
	}
}

func TestStaticAuthorityReplayIdentityIsDeterministic(t *testing.T) {
	_, _, left := sealedStatic(t, staticLawSource)
	_, _, right := sealedStatic(t, staticLawSource)
	if left.ContentID() != right.ContentID() || left.Classes().ContentID() != right.Classes().ContentID() {
		t.Fatal("equal sealed inputs changed Static identity")
	}
}

func TestStaticHotAlgebrasAllocateNothing(t *testing.T) {
	p, _, authority := sealedStatic(t, staticLawSource)
	_, number := aliasNamed(t, p, "NumberAlias")
	_, integer := aliasNamed(t, p, "IntegerAlias")
	left := resultFor(t, authority, p, number)
	right := resultFor(t, authority, p, integer)
	classes := authority.Classes()
	leftClass, _ := classes.ClassForStatic(left)
	rightClass, _ := classes.ClassForStatic(right)
	if allocations := testing.AllocsPerRun(1000, func() {
		_ = authority.LessOrEq(left, right)
		_ = authority.Join(left, right)
		_ = authority.Widen(left, right)
		_ = authority.WidenRank(left)
		_ = classes.LessOrEq(leftClass, rightClass)
		_ = classes.Join(leftClass, rightClass)
		_ = classes.Rank(leftClass)
		_ = classes.CanBeNil(leftClass)
	}); allocations != 0 {
		t.Fatalf("hot Static algebra allocates %v", allocations)
	}
}

func TestStaticQualifiedReferenceUsesLinkNamespaceAuthority(t *testing.T) {
	dependency, err := programlower.Lower(programlower.Source{Name: "dependency.lua", Text: []byte(`
type User = { id: string }
local M = {}
M.Schema.User = User
return M
`)})
	if err != nil {
		t.Fatal(err)
	}
	main, err := programlower.Lower(programlower.Source{Name: "main.lua", Text: []byte(`
type Namespace = typeof(require("dependency"))
local dependency = require("dependency")
type Imported = dependency.Schema.User
`)})
	if err != nil {
		t.Fatal(err)
	}
	binding := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"require"}}
	key := func(value string) keyspace.LiteralValue {
		return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}
	}
	contract, err := target.Seal(&target.Spec{
		Operations:   []target.OperationSpec{{Bindings: []target.BindingSpec{binding}, Input: target.ValuesSpec{Fixed: []typ.Type{typ.String}, Tail: target.ValuesClosed}, Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesClosed}}}, Effects: target.RowSpec{Tail: target.RowClosed}}},
		InitialRoots: []target.InitialRootSpec{{Identity: "GlobalEnvRoot", Shape: target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}}}},
		InitialEntries: []target.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: key("_G"), Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: key("__link_absent"), Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: key("require"), Value: target.InitialValueSpec{Kind: target.InitialValueOperation, Operation: binding}, Mutability: target.InitialMutable},
		},
		InitialBindings: []target.InitialBindingSpec{{Name: "_G", Root: "GlobalEnvRoot", Key: key("_G")}, {Name: "__link_absent", Root: "GlobalEnvRoot", Key: key("__link_absent")}, {Name: "require", Root: "GlobalEnvRoot", Key: key("require")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	runtimeImport, ok := main.Module().ImportAt(1)
	if !ok {
		t.Fatal("runtime source Import")
	}
	linked, err := proglink.Seal(&proglink.Spec{
		Target: contract, Modules: []linkproject.Module{{Name: "main", Program: main}, {Name: "dependency", Program: dependency}},
		Module: linkmodule.Spec{
			Actors:             []linkmodule.ActorSpec{{Name: "actor"}},
			ModuleCacheAliases: []linkmodule.ModuleCacheAliasClassSpec{{Actor: "actor", Instances: []string{"cache-main", "cache-dependency"}, Representative: "cache-main"}},
			AnalysisRoots:      []linkmodule.AnalysisRootSpec{{Name: "main-root", Module: "main", Actor: "actor", Instance: "cache-main"}, {Name: "dependency-root", Module: "dependency", Actor: "actor", Instance: "cache-dependency"}},
			ModuleCacheEntries: []linkmodule.ModuleCacheEntrySpec{
				{Module: "main", Import: runtimeImport.Term, FromRoot: "main-root", ToRoot: "dependency-root"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	types, ok := typeauthority.Seal(linked)
	if !ok {
		t.Fatal("type authority")
	}
	authority, _, err := Seal(linked, types)
	if err != nil {
		t.Fatal(err)
	}
	alias, _ := aliasNamed(t, main, "Imported")
	result := resultFor(t, authority, main, alias)
	actual, ok := authority.ClosedType(result)
	expected := typ.RebuildRecord(typ.RecordParts{Fields: []typ.Field{{Name: "id", Type: typ.String}}})
	if !ok || !subtype.IsSubtype(actual, expected) || !subtype.IsSubtype(expected, actual) {
		kind, _ := result.Kind()
		residue, _ := result.Symbolic()
		fault, _ := result.Fault()
		t.Fatalf("qualified result = %v/%v kind=%v residue=%#v fault=%v", actual, ok, kind, residue, fault)
	}
	namespaceAlias, _ := aliasNamed(t, main, "Namespace")
	namespace, ok := authority.ClosedType(resultFor(t, authority, main, namespaceAlias))
	namespaceExpected := typ.RebuildRecord(typ.RecordParts{Fields: []typ.Field{{
		Name: "Schema", Readonly: true,
		Type: typ.RebuildRecord(typ.RecordParts{Fields: []typ.Field{{Name: "User", Type: expected, Readonly: true}}}),
	}}})
	if !ok || !subtype.IsSubtype(namespace, namespaceExpected) || !subtype.IsSubtype(namespaceExpected, namespace) {
		t.Fatalf("literal-require namespace = %v/%v", namespace, ok)
	}
}

// Equal Program content may occur at multiple module names.  A qualified
// reference must retain the consumer resolver shard through Link and Static:
// ContentID alone cannot select one of those two source contexts.
func TestStaticQualifiedDuplicateContentShardsSealReplayAndPermutation(t *testing.T) {
	provider, err := programlower.Lower(programlower.Source{Name: "dependency.lua", Text: []byte(`
type User = { id: string }
local M = {}
M.Schema.User = User
return M
`)})
	if err != nil {
		t.Fatal(err)
	}
	consumerSource := programlower.Source{Name: "duplicate-consumer.lua", Text: []byte(`
local API = require("dependency")
type Subject = API.Schema.User
`)}
	first, err := programlower.Lower(consumerSource)
	if err != nil {
		t.Fatal(err)
	}
	second, err := programlower.Lower(consumerSource)
	if err != nil {
		t.Fatal(err)
	}
	if first.ContentID() != second.ContentID() {
		t.Fatal("duplicate consumers lost equal Program content")
	}
	firstImport, ok := first.Module().ImportAt(0)
	if !ok {
		t.Fatal("first consumer Import")
	}
	secondImport, ok := second.Module().ImportAt(0)
	if !ok {
		t.Fatal("second consumer Import")
	}
	contract := duplicateQualifiedStaticContract(t)
	actors := []linkmodule.ActorSpec{{Name: "actor"}}
	aliases := []linkmodule.ModuleCacheAliasClassSpec{
		{Actor: "actor", Instances: []string{"cache-dependency"}, Representative: "cache-dependency"},
		{Actor: "actor", Instances: []string{"cache-first"}, Representative: "cache-first"},
		{Actor: "actor", Instances: []string{"cache-second"}, Representative: "cache-second"},
	}
	roots := []linkmodule.AnalysisRootSpec{
		{Name: "dependency-root", Module: "dependency", Actor: "actor", Instance: "cache-dependency"},
		{Name: "first-root", Module: "first", Actor: "actor", Instance: "cache-first"},
		{Name: "second-root", Module: "second", Actor: "actor", Instance: "cache-second"},
	}
	entries := []linkmodule.ModuleCacheEntrySpec{
		{Module: "first", Import: firstImport.Term, FromRoot: "first-root", ToRoot: "dependency-root"},
		{Module: "second", Import: secondImport.Term, FromRoot: "second-root", ToRoot: "dependency-root"},
	}
	seal := func(modules []linkproject.Module) (*proglink.Link, *Authority) {
		linked, err := proglink.Seal(&proglink.Spec{
			Target: contract, Modules: modules, Module: linkmodule.Spec{
				Actors: actors, ModuleCacheAliases: aliases, AnalysisRoots: roots, ModuleCacheEntries: entries,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		types, ok := typeauthority.Seal(linked)
		if !ok {
			t.Fatal("type authority")
		}
		authority, _, err := Seal(linked, types)
		if err != nil {
			t.Fatal(err)
		}
		return linked, authority
	}
	canonicalModules := []linkproject.Module{
		{Name: "first", Program: first}, {Name: "second", Program: second}, {Name: "dependency", Program: provider},
	}
	linked, authority := seal(canonicalModules)
	assertDuplicateQualifiedStaticShards(t, linked, authority, first, provider)

	encoded, err := proglink.EncodeArtifact(linked)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := proglink.DecodeArtifact(encoded, contract, map[keyspace.ContentID]*program.Program{
		first.ContentID(): first, provider.ContentID(): provider,
	})
	if err != nil {
		t.Fatal(err)
	}
	if replayed.ContentID() != linked.ContentID() {
		t.Fatal("artifact replay changed Link identity")
	}
	replayedTypes, ok := typeauthority.Seal(replayed)
	if !ok {
		t.Fatal("replayed type authority")
	}
	replayedAuthority, _, err := Seal(replayed, replayedTypes)
	if err != nil {
		t.Fatal(err)
	}
	assertDuplicateQualifiedStaticShards(t, replayed, replayedAuthority, first, provider)

	permuted, permutedAuthority := seal([]linkproject.Module{
		{Name: "dependency", Program: provider}, {Name: "second", Program: second}, {Name: "first", Program: first},
	})
	if permuted.ContentID() != linked.ContentID() || permutedAuthority.ContentID() != authority.ContentID() {
		t.Fatal("module input permutation changed sealed static authority")
	}
	assertDuplicateQualifiedStaticShards(t, permuted, permutedAuthority, first, provider)
}

func duplicateQualifiedStaticContract(t testing.TB) *target.Contract {
	t.Helper()
	binding := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"require"}}
	key := func(value string) keyspace.LiteralValue {
		return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}
	}
	contract, err := target.Seal(&target.Spec{
		Operations: []target.OperationSpec{{
			Bindings: []target.BindingSpec{binding}, Input: target.ValuesSpec{Fixed: []typ.Type{typ.String}, Tail: target.ValuesClosed},
			Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesClosed}}},
			Effects:  target.RowSpec{Tail: target.RowClosed},
		}},
		InitialRoots: []target.InitialRootSpec{{Identity: "GlobalEnvRoot", Shape: target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}}}},
		InitialEntries: []target.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: key("_G"), Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: key("__link_absent"), Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: key("require"), Value: target.InitialValueSpec{Kind: target.InitialValueOperation, Operation: binding}, Mutability: target.InitialMutable},
		},
		InitialBindings: []target.InitialBindingSpec{
			{Name: "_G", Root: "GlobalEnvRoot", Key: key("_G")},
			{Name: "__link_absent", Root: "GlobalEnvRoot", Key: key("__link_absent")},
			{Name: "require", Root: "GlobalEnvRoot", Key: key("require")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func assertDuplicateQualifiedStaticShards(t *testing.T, linked *proglink.Link, authority *Authority, consumer, provider *program.Program) {
	t.Helper()
	_, consumerSubject := aliasNamed(t, consumer, "Subject")
	consumerRef, ok := consumer.Static().StaticTypes().Ref(consumerSubject)
	if !ok {
		t.Fatal("consumer Subject static reference")
	}
	mounts := linked.Project().Mounts()
	var providerShard linkproject.Shard
	for index := 0; index < mounts.Count(); index++ {
		shard, shardOK := mounts.At(index)
		mounted, mountedOK := mounts.Program(shard)
		if shardOK && mountedOK && mounted == provider {
			providerShard = shard
			break
		}
	}
	if providerShard == (linkproject.Shard{}) {
		t.Fatal("provider shard")
	}
	if _, ok := linked.Static().Namespaces().ResolverForShard(providerShard); !ok {
		t.Fatal("provider resolver")
	}
	seen := make(map[linkstatic.ExpressionRef]struct{}, 2)
	var providerRef programstatic.StaticTypeRef
	// StaticTypeRef is a live Program-owned capability.  Equal Program
	// content mounted in two Link shards therefore issues two distinct live
	// references even though their local terms are equal.  Select the two
	// source expressions by the stable local term, then use ExpressionRef
	// below as the shard-fenced replay identity.
	consumerTerm := consumerRef.Term()
	consumerOwner := consumer.ContentID()
	for index := 0; index < linked.Static().Expressions().Count(); index++ {
		expression, ok := linked.Static().Expressions().At(index)
		if !ok {
			t.Fatalf("StaticExpressionAt(%d)", index)
		}
		reference, ok := linked.Static().Expressions().Reference(expression)
		shard, shardOK := linked.Static().Expressions().Shard(expression)
		owner, ownerOK := linked.Project().Mounts().Program(shard)
		if !ok || !shardOK || !ownerOK || owner == nil || owner.ContentID() != consumerOwner || reference.Term() != consumerTerm {
			continue
		}
		originalRef, ok := linked.Static().Expressions().Ref(expression)
		if !ok || originalRef.Reference() != consumerRef.Term() {
			t.Fatalf("consumer expression identity %d", index)
		}
		originalShard, ok := linked.Static().Expressions().Shard(expression)
		shardIndex, shardIndexOK := mounts.Index(originalShard)
		if !ok || !shardIndexOK || uint32(shardIndex+1) != originalRef.ShardOrdinal() {
			t.Fatalf("consumer expression shard %d", index)
		}
		resolver, ok := linked.Static().Expressions().Resolver(expression)
		if !ok {
			t.Fatalf("consumer resolver %d", index)
		}
		resolverShard, ok := linked.Static().Namespaces().ResolverShard(resolver)
		if !ok || resolverShard != originalShard {
			t.Fatalf("consumer resolver shard %d", index)
		}
		rebound, ok := linked.Static().Expressions().For(resolver, consumerRef)
		if !ok {
			t.Fatalf("consumer expression rebind %d", index)
		}
		reboundRef, ok := linked.Static().Expressions().Ref(rebound)
		if !ok || reboundRef != originalRef {
			t.Fatalf("consumer expression rebound identity %d", index)
		}
		seen[originalRef] = struct{}{}
		qualified, ok := linked.Static().Expressions().Qualified(rebound)
		if !ok {
			t.Fatalf("qualified expression %d", index)
		}
		resolved, ok := linked.Static().Expressions().Reference(qualified)
		resolvedShard, shardOK := linked.Static().Expressions().Shard(qualified)
		providerReference, providerReferenceOK := provider.Static().StaticTypes().Ref(resolved.Term())
		if !ok || !shardOK || resolvedShard != providerShard || !providerReferenceOK || providerReference.Term() != resolved.Term() {
			t.Fatalf("qualified expression %d resolved %v/%v", index, resolved, ok)
		}
		if providerRef.Term() != 0 && resolved.Term() != providerRef.Term() {
			t.Fatalf("qualified expression %d changed provider reference", index)
		}
		providerRef = resolved
		qualifiedShard, ok := linked.Static().Expressions().Shard(qualified)
		if !ok || qualifiedShard != providerShard {
			t.Fatalf("qualified expression %d provider shard %v/%v", index, qualifiedShard, ok)
		}
	}
	if len(seen) != 2 {
		t.Fatalf("duplicate consumer qualified identities = %d, want two shard-fenced expressions", len(seen))
	}
	if providerRef.Term() == 0 {
		t.Fatal("duplicate consumers did not resolve a provider reference")
	}
	value := resultFor(t, authority, consumer, consumerSubject)
	actual, ok := authority.ClosedType(value)
	expected := typ.RebuildRecord(typ.RecordParts{Fields: []typ.Field{{Name: "id", Type: typ.String}}})
	if !ok || !subtype.IsSubtype(actual, expected) || !subtype.IsSubtype(expected, actual) {
		t.Fatalf("duplicate consumer qualified result = %v/%v", actual, ok)
	}
}

func TestStaticAuthorityRejectsForeignComposition(t *testing.T) {
	p, _, left := sealedStatic(t, staticLawSource)
	otherProgram, _, right := sealedStatic(t, staticLawSource)
	_, number := aliasNamed(t, p, "NumberAlias")
	_, otherNumber := aliasNamed(t, otherProgram, "NumberAlias")
	leftValue := resultFor(t, left, p, number)
	rightValue := resultFor(t, right, otherProgram, otherNumber)
	if left.Equal(leftValue, rightValue) || left.LessOrEq(leftValue, rightValue) {
		t.Fatal("foreign result compared")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("foreign join did not fail closed")
		}
	}()
	_ = left.Join(leftValue, rightValue)
}

func TestStaticAuthorityCoordinatesAreExactAndLinkFenced(t *testing.T) {
	_, _, authority := sealedStatic(t, staticLawSource)
	if authority.CoordinateCount() == 0 {
		t.Fatal("missing authored Static coordinates")
	}
	for index := 0; index < authority.CoordinateCount(); index++ {
		coordinate, ok := authority.CoordinateAt(index)
		if !ok {
			t.Fatal("malformed Static coordinate")
		}
		dense, ok := authority.CoordinateIndex(coordinate)
		if !ok || int(dense) != index {
			t.Fatalf("coordinate %d was not exact", index)
		}
		value, ok := authority.Result(coordinate)
		if !ok || !authority.Owns(value) || authority.Fingerprint(value) == 0 {
			t.Fatalf("coordinate %d result is not admitted", index)
		}
	}
	_, _, foreign := sealedStatic(t, staticLawSource)
	foreignCoordinate, ok := foreign.CoordinateAt(0)
	if !ok {
		t.Fatal("foreign Static coordinate")
	}
	if authority.coordinates[0].key.reference != foreign.coordinates[0].key.reference {
		t.Fatal("identical sealed inputs changed portable Static reference")
	}
	if _, ok := authority.CoordinateIndex(foreignCoordinate); ok {
		t.Fatal("foreign Static coordinate entered authority")
	}
	localCoordinate, ok := authority.CoordinateAt(0)
	if !ok {
		t.Fatal("local Static coordinate")
	}
	if value, ok := authority.Result(localCoordinate); !ok || !authority.Owns(value) {
		t.Fatal("local Static coordinate did not resolve local value")
	}
	if _, ok := authority.Result(foreignCoordinate); ok {
		t.Fatal("foreign Static coordinate resolved through local authority")
	}
	if value, ok := foreign.Result(foreignCoordinate); !ok || authority.Owns(value) || !foreign.Owns(value) {
		t.Fatal("foreign Static value crossed authority fence")
	}
}

// TestStaticClassSetClassifiesTargetDeclarationsDirectly proves that Static's
// declaration class denominator is Target-owned.  No Link Application is
// created or queried: a later selected endpoint Rule owns call-site evidence.
func TestStaticClassSetClassifiesTargetDeclarationsDirectly(t *testing.T) {
	formal := typ.NewTypeParam("T", typ.Any)
	contract, err := target.Seal(&target.Spec{Operations: []target.OperationSpec{
		{
			Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"concrete"}}},
			Input:    target.ValuesSpec{Fixed: []typ.Type{typ.String}, Tail: target.ValuesClosed},
			Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Fixed: []typ.Type{typ.Number}, Tail: target.ValuesClosed}}},
			Effects:  target.RowSpec{Tail: target.RowClosed},
		},
		{
			Bindings:    []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"generic"}}},
			TypeFormals: []*typ.TypeParam{formal},
			Input:       target.ValuesSpec{Fixed: []typ.Type{formal}, Tail: target.ValuesClosed},
			Outcomes:    []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Fixed: []typ.Type{formal}, Tail: target.ValuesClosed}}},
			Effects:     target.RowSpec{Tail: target.RowClosed},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := programlower.Lower(programlower.Source{Name: "static_target_law.lua", Text: []byte("local value = 1")})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := proglink.Seal(&proglink.Spec{Target: contract, Modules: []linkproject.Module{{Name: "static_target_law", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	types, ok := typeauthority.Seal(linked)
	if !ok {
		t.Fatal("type authority")
	}
	authority, _, err := Seal(linked, types)
	if err != nil {
		t.Fatal(err)
	}
	concrete, ok := contract.OperationAt(0)
	if !ok {
		t.Fatal("concrete operation")
	}
	input, ok := contract.Input(concrete)
	if !ok {
		t.Fatal("concrete input")
	}
	concreteType, ok := contract.ValuesAt(input, 0)
	if !ok {
		t.Fatal("concrete input type")
	}
	concreteClass, ok := authority.Classes().ClassForTarget(contract, concreteType)
	if !ok || authority.Classes().Equal(concreteClass, authority.Classes().AnyValue()) {
		t.Fatal("concrete Target declaration lacks finite class")
	}
	generic, ok := contract.OperationAt(1)
	if !ok {
		t.Fatal("generic operation")
	}
	genericInput, ok := contract.Input(generic)
	if !ok {
		t.Fatal("generic input")
	}
	formalType, ok := contract.ValuesAt(genericInput, 0)
	if !ok {
		t.Fatal("generic formal type")
	}
	formalClass, ok := authority.Classes().ClassForTarget(contract, formalType)
	kind, kindOK := authority.Classes().Kind(formalClass)
	if !ok || !kindOK || kind != ClassOpaque {
		t.Fatalf("generic Target formal class = %#v/%v", formalClass, ok)
	}
	if !authority.Classes().CanBeNil(formalClass) || !authority.Classes().CanBeNil(authority.Classes().Join(formalClass, concreteClass)) {
		t.Fatal("opaque Target class lost conservative nilability through a derived union")
	}
	if _, foreignOK := authority.Classes().ClassForTarget(&target.Contract{}, concreteType); foreignOK {
		t.Fatal("foreign Target ordinal crossed ClassSet owner fence")
	}
}
