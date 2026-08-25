package definition

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

func declaredSpecimenSymbol(name string) GoSymbol {
	return GoSymbol{PackagePath: specimenPackage, Name: name, ResultIndex: 0}
}

// declaredDerivation is the specimen's relation stated in the DECLARED form:
// one source enumeration, the one authored judgment that resolves an item to a
// row, and the lattice endpoint the set widens to a whole directory at.
func declaredDerivation() RelationDerivation {
	return RelationDerivation{
		StaticAxes: []schema.EntryReference{specimenAxis()},
		Source: []EnumerationRef{{
			Axis: specimenAxis(), Name: "Alternatives",
		}},
		Resolve:     declaredSpecimenSymbol("ResolveRow"),
		InlineWidth: 4,
		Widen: DerivationWiden{
			Predicate: declaredSpecimenSymbol("IsTop"),
			Source:    []EnumerationRef{{Axis: specimenAxis(), Name: "Directory"}},
		},
	}
}

// TestADerivationStatesOneFormOrTheOther is the exclusivity law. A relation
// carrying both the authored quartet and the declared operators is two answers
// to how its rows are built, and which one a consumer read would decide what
// the rows ARE - so the pair is refused rather than resolved by precedence.
func TestADerivationStatesOneFormOrTheOther(t *testing.T) {
	declared := declaredDerivation()
	if !declared.complete() {
		t.Fatal("a whole declared derivation was refused")
	}
	authored := RelationDerivation{
		State:      specimenType("Plan"),
		Build:      declaredSpecimenSymbol("DeriveRows"),
		Count:      declaredSpecimenSymbol("RowCount"),
		At:         declaredSpecimenSymbol("RowAt"),
		StaticAxes: []schema.EntryReference{specimenAxis()},
	}
	if !authored.complete() {
		t.Fatal("a whole authored derivation was refused")
	}
	both := authored
	both.Source = declared.Source
	both.Resolve = declared.Resolve
	if both.complete() {
		t.Fatal("a relation stating both forms was admitted; which one builds its rows would be undecided")
	}
	var neither RelationDerivation
	if neither.complete() {
		t.Fatal("a relation stating neither form was admitted as a derivation")
	}
}

// TestADeclaredDerivationIsWholeOrRefused states the row-local law of the
// declared form. Each part answers a question the generated construction has
// to ask - what does it enumerate, what turns an item into a row, and where
// does the set stop being enumerable - so a half-stated one would be generated
// against a question nobody answered.
func TestADeclaredDerivationIsWholeOrRefused(t *testing.T) {
	for _, probe := range []struct {
		name  string
		amend func(*RelationDerivation)
	}{
		{name: "no source to enumerate", amend: func(d *RelationDerivation) { d.Source = nil }},
		{name: "a source that names no enumeration", amend: func(d *RelationDerivation) { d.Source = []EnumerationRef{{Axis: specimenAxis()}} }},
		{name: "no judgment to resolve an item with", amend: func(d *RelationDerivation) { d.Resolve = GoSymbol{} }},
		{name: "no axis to resolve against", amend: func(d *RelationDerivation) { d.StaticAxes = nil }},
		{name: "one axis named twice", amend: func(d *RelationDerivation) {
			d.StaticAxes = []schema.EntryReference{specimenAxis(), specimenAxis()}
		}},
		{name: "a widen endpoint with nothing to read the whole set out of", amend: func(d *RelationDerivation) { d.Widen.Source = nil }},
		{name: "a widen directory with no endpoint", amend: func(d *RelationDerivation) { d.Widen.Predicate = GoSymbol{} }},
	} {
		t.Run(probe.name, func(t *testing.T) {
			derivation := declaredDerivation()
			probe.amend(&derivation)
			if derivation.complete() {
				t.Fatal("a half-stated declared derivation was admitted")
			}
		})
	}
}

// TestADerivationThatWidensNowhereIsStillWhole states that the endpoint is
// optional and its absence is a statement. A relation whose source is always
// enumerable - every alternative is a row, and there is no Top to answer for -
// declares no widen, and that is different from declaring half of one.
func TestADerivationThatWidensNowhereIsStillWhole(t *testing.T) {
	derivation := declaredDerivation()
	derivation.Widen = DerivationWiden{}
	if !derivation.complete() {
		t.Fatal("a declared derivation with no lattice endpoint was refused")
	}
	if derivation.Widen.Declared() {
		t.Fatal("an absent endpoint reads as declared")
	}
}

// TestAnEnumerationIsTheOwnersWholeStatement holds the axis-level row to the
// same standard: a name, an element carrier, and the owner's two symbols. A
// partial one would be a sequence nothing can walk.
//
// What it reads OUT OF is deliberately not on that list; an empty one is the
// directory case, stated by the law below.
func TestAnEnumerationIsTheOwnersWholeStatement(t *testing.T) {
	whole := Enumeration{
		Name: "Alternatives", Over: "FactCarrier", Item: "SeedCarrier",
		Count: declaredSpecimenSymbol("AlternativeCount"),
		At:    declaredSpecimenSymbol("AlternativeAt"),
	}
	if !whole.complete() {
		t.Fatal("a whole enumeration was refused")
	}
	for _, probe := range []struct {
		name  string
		amend func(*Enumeration)
	}{
		{name: "no name", amend: func(e *Enumeration) { e.Name = "" }},
		{name: "no element carrier", amend: func(e *Enumeration) { e.Item = "" }},
		{name: "no census", amend: func(e *Enumeration) { e.Count = GoSymbol{} }},
		{name: "no accessor", amend: func(e *Enumeration) { e.At = GoSymbol{} }},
	} {
		t.Run(probe.name, func(t *testing.T) {
			enumeration := whole
			probe.amend(&enumeration)
			if enumeration.complete() {
				t.Fatal("a partial enumeration was admitted")
			}
		})
	}
}

// TestOnlyAnAuthoredDerivationOwesTheLedgerARow states what the ledger is for.
// It admits the authored form one row at a time because that form is scheduled
// to be replaced. A declared derivation has no Build to schedule - its
// construction is generated - so requiring a row would be asking a migration
// set to track something already migrated.
func TestOnlyAnAuthoredDerivationOwesTheLedgerARow(t *testing.T) {
	declared := declaredDerivation()
	if declared.AuthoredDerivation() {
		t.Fatal("a declared derivation reads as authored, so the ledger would demand a row for it")
	}
	if !declared.DeclaredDerivation() {
		t.Fatal("a declared derivation does not read as declared")
	}
	if derivationOptional(declared) {
		t.Fatal("a declared derivation reads as deriving nothing at all")
	}
}

// declaredSpecimenSource is the specimen axis with two enumerations declared
// on it and its relation stated in the declared form, composing them: the
// alternatives of a fact, and what each alternative in turn yields.
// specimenWiden is the specimen's lattice endpoint. Its directory yields the
// seed carrier while the source chain yields the key one, so the endpoint owes
// its own judgment: a directory row is not the item the value decomposes to.
func specimenWiden() DerivationWiden {
	return DerivationWiden{
		Predicate: declaredSpecimenSymbol("IsTop"),
		Source:    []EnumerationRef{{Axis: specimenAxis(), Name: "Directory"}},
		Resolve:   declaredSpecimenSymbol("ResolveDirectoryRow"),
	}
}

func declaredSpecimenSource(t testing.TB) (Definition, Relation) {
	t.Helper()
	source := specimenBase()
	source.Enumerations = []Enumeration{
		{
			Name: "Alternatives", Over: "FactCarrier", Item: "SeedCarrier",
			Count: declaredSpecimenSymbol("AlternativeCount"),
			At:    declaredSpecimenSymbol("AlternativeAt"),
		},
		{
			Name: "Parts", Over: "SeedCarrier", Item: "KeyCarrier",
			Count: declaredSpecimenSymbol("PartCount"),
			At:    declaredSpecimenSymbol("PartAt"),
		},
		{
			// Read out of the axis's own schema: this is the owner's whole
			// directory, which is what a widened answer is.
			Name: "Directory", Item: "SeedCarrier",
			Count: declaredSpecimenSymbol("DirectoryCount"),
			At:    declaredSpecimenSymbol("DirectoryAt"),
		},
	}
	relation := Relation{
		Name: "Derived", Key: "specimen/derived", Subject: "FactCarrier",
		Inputs:            []RelationInput{{Carrier: "SeedCarrier"}},
		CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: specimenAxis(), Member: "specimen/candidates"}),
		Derivation: RelationDerivation{
			StaticAxes: []schema.EntryReference{specimenAxis()},
			Source: []EnumerationRef{
				{Axis: specimenAxis(), Name: "Alternatives"},
				{Axis: specimenAxis(), Name: "Parts"},
			},
			Resolve:     declaredSpecimenSymbol("ResolveRow"),
			InlineWidth: 4,
		},
	}
	source.Relations = append(source.Relations, relation)
	return source, relation
}

func declaredSpecimenRoster(t testing.TB, source Definition) Roster {
	t.Helper()
	roster, rosterOK := NewRoster(Source{Package: "specimen", Name: "specimen", Base: source})
	if !rosterOK {
		t.Fatal("declared specimen roster is not admissible")
	}
	return roster
}

// TestComposedSourcesReadWhatTheOneBeforeThemYielded is the composition law,
// and it is the reason a two-level derivation is FLAT rather than nested.
//
// The first source is enumerated over the relation's own input; each further
// one is enumerated over the item the previous yields. Nothing authored sits
// between them - the composition is two declared enumerations, so a derivation
// that walks members and then what each member projects to needs no per-pair
// step and no nested operator.
func TestComposedSourcesReadWhatTheOneBeforeThemYielded(t *testing.T) {
	source, relation := declaredSpecimenSource(t)
	roster := declaredSpecimenRoster(t, source)
	shape, ok := roster.DeclaredDerivationSignature("specimen", relation, specimenType("Key"))
	if !ok || len(shape.Sources) != 2 {
		t.Fatalf("the composed derivation derived no shape: %+v %t", shape, ok)
	}
	// The outer enumeration reads a fact and yields a seed; the inner one must
	// therefore read a seed.
	if shape.Sources[0].AtResults[0].Type != specimenType("Seed") {
		t.Fatalf("the outer source yields %+v, want the seed carrier", shape.Sources[0].AtResults[0].Type)
	}
	if shape.Sources[1].CountParams[0].Type != specimenType("Seed") {
		t.Fatalf("the inner source reads %+v, want what the outer one yielded", shape.Sources[1].CountParams[0].Type)
	}
	// Resolve is handed the static schema, the candidate, and the INNERMOST
	// item - never an outer one, which would ask it to re-walk the composition.
	if len(shape.ResolveParams) != 3 || shape.ResolveParams[2].Type != specimenType("Key") {
		t.Fatalf("resolve takes %+v, want the static schema, the candidate and the innermost item", shape.ResolveParams)
	}
	if len(shape.ResolveResults) != 3 || shape.ResolveResults[0].Type != specimenType("Fact") {
		t.Fatalf("resolve answers %+v, want the row, whether it contributes one, and validity", shape.ResolveResults)
	}
}

// TestAComposedSourceThatDoesNotReadWhatItIsGivenIsRefused is the other half.
// Two enumerations whose carriers do not chain are not a composition; the
// generated construction would hand the second one a value it does not read.
func TestAComposedSourceThatDoesNotReadWhatItIsGivenIsRefused(t *testing.T) {
	source, relation := declaredSpecimenSource(t)
	// Both sources now read a fact, so the second does not read what the first
	// yields.
	relation.Derivation.Source[1] = EnumerationRef{Axis: specimenAxis(), Name: "Alternatives"}
	roster := declaredSpecimenRoster(t, source)
	if _, ok := roster.DeclaredDerivationSignature("specimen", relation, specimenType("Key")); ok {
		t.Fatal("a source chain that does not compose derived a shape")
	}
}

// TestAWidenEndpointIsAskedOfWhatTheDerivationEnumerates states what the
// endpoint is asked.
//
// It is a judgment over exactly what Resolve is a judgment over, minus the item
// there is not one of yet: the static axis schemas, the candidate, and the
// OUTER source's own carrier. The last position is what is being enumerated,
// and the ones before it are why - whether a set has a closed list of
// alternatives can depend on what the set is OF, so an endpoint that could see
// only the value would be answering a different question.
func TestAWidenEndpointIsAskedOfWhatTheDerivationEnumerates(t *testing.T) {
	source, relation := declaredSpecimenSource(t)
	relation.Derivation.Widen = specimenWiden()
	roster := declaredSpecimenRoster(t, source)
	shape, ok := roster.DeclaredDerivationSignature("specimen", relation, specimenType("Key"))
	if !ok || len(shape.WidenParams) != len(shape.ResolveParams) {
		t.Fatalf("the endpoint derived no shape: %+v %t", shape, ok)
	}
	for index := 0; index < len(shape.WidenParams)-1; index++ {
		if shape.WidenParams[index] != shape.ResolveParams[index] {
			t.Fatalf("the endpoint's position %d is %+v, the judgment's is %+v", index, shape.WidenParams[index], shape.ResolveParams[index])
		}
	}
	if shape.WidenParams[len(shape.WidenParams)-1].Type != specimenType("Fact") {
		t.Fatalf("the endpoint is asked of %+v, want the carrier the outer source enumerates", shape.WidenParams[len(shape.WidenParams)-1].Type)
	}
	if len(shape.WidenResults) != 1 || shape.WidenResults[0].Type.Name != "bool" {
		t.Fatalf("the endpoint answers %+v, want one boolean", shape.WidenResults)
	}
}

// TestAnEnumerationOverNothingIsTheOwnersDirectory states the one enumeration
// that reads no carrier.
//
// A derivation that reaches a lattice endpoint has a fact that failed to name
// its alternatives, so there is no value left to read them out of - the answer
// is the owner's whole directory, which only the owner's schema can produce.
// An empty Over is therefore a statement rather than a missing field, and it
// is the only way a widened answer can be enumerated at all.
func TestAnEnumerationOverNothingIsTheOwnersDirectory(t *testing.T) {
	directory := Enumeration{
		Name: "Directory", Item: "SeedCarrier",
		Count: declaredSpecimenSymbol("DirectoryCount"),
		At:    declaredSpecimenSymbol("DirectoryAt"),
	}
	if !directory.complete() {
		t.Fatal("an enumeration over the owner's own schema was refused")
	}
	if !directory.OverSchema() {
		t.Fatal("an enumeration naming no carrier does not read as the owner's directory")
	}
	overCarrier := directory
	overCarrier.Over = "FactCarrier"
	if overCarrier.OverSchema() {
		t.Fatal("an enumeration naming a carrier reads as the owner's directory")
	}
}

// TestAWidenedAnswerIsReadOutOfTheOwnersSchema pins where the widened rows
// come from. The endpoint predicate is asked of the fact, but the rows it
// widens to are the owner's, so their enumeration chains from the schema and
// not from the value that reached the endpoint.
func TestAWidenedAnswerIsReadOutOfTheOwnersSchema(t *testing.T) {
	source, relation := declaredSpecimenSource(t)
	relation.Derivation.Widen = specimenWiden()
	roster := declaredSpecimenRoster(t, source)
	shape, ok := roster.DeclaredDerivationSignature("specimen", relation, specimenType("Key"))
	if !ok || len(shape.Widened) != 1 {
		t.Fatalf("the widened answer derived no shape: %+v %t", shape, ok)
	}
	schemaType, schemaOK := AxisSchemaType(source)
	if !schemaOK {
		t.Fatal("the specimen axis declares no schema type")
	}
	if shape.Widened[0].CountParams[0].Type != schemaType {
		t.Fatalf("the widened answer is read out of %+v, want the axis's own schema %+v", shape.Widened[0].CountParams[0].Type, schemaType)
	}
}

// TestADeclaredDerivationStatesTheWidthItHoldsByValue is the allocation law of
// the declared form. The generated construction holds its rows in a bounded
// inline prefix BY VALUE with an explicit spill beyond it, so an ordinary
// answer costs no allocation at all. How wide that prefix is depends on how
// many members the relation ordinarily answers, which is the owner's knowledge
// and not a number this package may pick: a derivation that states none leaves
// its generated set with no inline prefix, and then every answer reaches the
// spill and every invocation allocates.
func TestADeclaredDerivationStatesTheWidthItHoldsByValue(t *testing.T) {
	declared := declaredDerivation()
	if !declared.complete() {
		t.Fatal("a whole declared derivation was refused")
	}
	widthless := declared
	widthless.InlineWidth = 0
	if widthless.complete() {
		t.Fatal("a declared derivation stating no inline width was admitted; every answer of its generated set would allocate")
	}
	negative := declared
	negative.InlineWidth = -1
	if negative.complete() {
		t.Fatal("a declared derivation stating a negative inline width was admitted")
	}
}

// TestAnEnumerationStatesTheOrderItYieldsIn is the law the two-way widen
// choice rests on.
//
// Whether a widened answer may be read where it lies, instead of being placed
// member by member, depends on one thing: whether the directory hands its rows
// back in the order the consuming relation is ordered by. That is a promise
// about how the owner's own accessor is written, so the owner states it and
// nothing infers it - and it is stated whole or not at all, because the one
// thing a consumer may not do with a half-written promise is guess which axis
// was meant.
func TestAnEnumerationStatesTheOrderItYieldsIn(t *testing.T) {
	directory := Enumeration{
		Name: "Directory", Item: "SeedCarrier",
		Count: declaredSpecimenSymbol("DirectoryCount"),
		At:    declaredSpecimenSymbol("DirectoryAt"),
	}
	if !directory.complete() {
		t.Fatal("an enumeration stating no order was refused; saying nothing is not the same as being disordered")
	}
	if directory.YieldsInOrderOf("specimen") {
		t.Fatal("an enumeration stating no order promised one")
	}
	ordered := directory
	ordered.Order = specimenAxis()
	if !ordered.complete() {
		t.Fatal("an enumeration stating the axis it yields in order of was refused")
	}
	if !ordered.YieldsInOrderOf("specimen") {
		t.Fatal("an enumeration that yields in its own axis's order does not say so")
	}
	if ordered.YieldsInOrderOf("other") {
		t.Fatal("an enumeration promised an order it never stated")
	}
	half := directory
	half.Order = schema.EntryReference{Surface: schema.SurfaceKindAxis}
	if half.complete() {
		t.Fatal("an enumeration naming a surface but no axis was admitted; which axis it yields in order of is undecided")
	}
}

// TestAWidenEndpointYieldingAnotherItemStatesItsOwnJudgment says where a
// judgment belongs: to a source CHAIN, not to the derivation.
//
// The widened chain enumerates the owner's directory, and a directory row is
// not necessarily the item the value that reached the endpoint decomposes to -
// one symbol cannot be handed both. So the endpoint states its own judgment
// exactly where the two chains disagree, and stating a second where they agree
// would be two answers to what a member is.
func TestAWidenEndpointYieldingAnotherItemStatesItsOwnJudgment(t *testing.T) {
	source, relation := declaredSpecimenSource(t)
	relation.Derivation.Widen = specimenWiden()
	roster := declaredSpecimenRoster(t, source)
	shape, ok := roster.DeclaredDerivationSignature("specimen", relation, specimenType("Key"))
	if !ok {
		t.Fatal("an endpoint stating its own judgment derived no shape")
	}
	if len(shape.WidenResolveParams) != len(shape.ResolveParams) {
		t.Fatalf("the endpoint's judgment takes %d positions, the derivation's takes %d", len(shape.WidenResolveParams), len(shape.ResolveParams))
	}
	item := shape.WidenResolveParams[len(shape.WidenResolveParams)-1]
	if item.Type != specimenType("Seed") {
		t.Fatalf("the endpoint's judgment is handed %+v, want the item its own directory yields", item.Type)
	}
	if shape.ResolveParams[len(shape.ResolveParams)-1].Type == item.Type {
		t.Fatal("the two chains yield one item, so this specimen states nothing about two")
	}

	missing := relation
	missing.Derivation.Widen.Resolve = GoSymbol{}
	if _, admitted := roster.DeclaredDerivationSignature("specimen", missing, specimenType("Key")); admitted {
		t.Fatal("a widened chain yielding another item was admitted with no judgment for it")
	}

	agreeing := relation
	agreeing.Derivation.Widen.Source = []EnumerationRef{{Axis: specimenAxis(), Name: "Parts"}}
	if _, admitted := roster.DeclaredDerivationSignature("specimen", agreeing, specimenType("Key")); admitted {
		t.Fatal("a widened chain yielding the source's own item took a second judgment")
	}
}
