package definition

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

const specimenPackage = "example/axis"

func specimenType(name string) GoType { return GoType{PackagePath: specimenPackage, Name: name} }

func specimenMethod(name, receiver string) GoSymbol {
	return GoSymbol{PackagePath: specimenPackage, Name: name, Receiver: specimenType(receiver)}
}

func specimenAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "specimen"}
}

// specimenBase is one axis owner's whole authored base: carriers, one
// candidate relation with its directory, and one destination projection. It
// declares no reducer, which is the shape Compose requires.
func specimenBase() Definition {
	provider := member.AxisRelationCandidate(member.RelationRef{Axis: specimenAxis(), Member: "specimen/candidates"})
	return Definition{
		Name:       "Specimen",
		Axis:       "specimen",
		ImportPath: specimenPackage,
		Binding: Binding{Key: KeyNormalization{
			Carrier:    "KeyCarrier",
			Dense:      GoType{Name: "uint32"},
			Normalizer: specimenMethod("KeyIndex", "Schema"),
		}},
		Signature: Signature{Key: "KeyCarrier", Fact: "FactCarrier"},
		Carriers: []Carrier{
			{Name: "KeyCarrier", Key: "carrier/specimen/key", Type: specimenType("Key")},
			{Name: "FactCarrier", Key: "carrier/specimen/fact", Type: specimenType("Fact")},
			{Name: "SeedCarrier", Key: "carrier/specimen/seed", Type: specimenType("Seed")},
		},
		Relations: []Relation{{
			Name:              "Candidates",
			Key:               "specimen/candidates",
			Subject:           "SeedCarrier",
			CandidateProvider: provider,
			CandidateResolver: specimenMethod("SeedForOccurrence", "Schema"),
			CandidateOrdinal:  specimenMethod("SeedOrdinal", "Schema"),
			CandidateAt:       specimenMethod("SeedAt", "Schema"),
		}},
		Projections: []Projection{{
			Name:              "SeedCoordinate",
			Key:               "specimen/coordinate",
			Relation:          "Candidates",
			CandidateProvider: provider,
			Role:              member.Destination,
			Result:            "KeyCarrier",
			Accessor:          specimenMethod("Result", "Seed"),
		}},
	}
}

func specimenContribution(rule schema.Key, name string, key schema.Key) Contribution {
	return Contribution{
		Axis: "specimen",
		Rule: rule,
		Reducers: []Reducer{{
			Name:           name,
			Key:            key,
			Candidate:      "SeedCarrier",
			Outputs:        []ReducerOutput{{Axis: specimenAxis(), Carrier: "FactCarrier"}},
			Implementation: GoSymbol{PackagePath: specimenPackage, Name: name + "Fold"},
		}},
	}
}

func specimenSource(contributions ...Contribution) Source {
	return Source{Package: "specimen", Name: "specimen", Base: specimenBase(), Contributions: contributions}
}

// TestComposeFoldsRuleContributionsIntoTheAxisDefinition is the baseline: the
// axis vocabulary a generator renders is the base plus the reducers its rules
// declared, in roster order, each carrying the rule that declared it.
func TestComposeFoldsRuleContributionsIntoTheAxisDefinition(t *testing.T) {
	composed, ok := specimenSource(
		specimenContribution("specimen-first", "First", "specimen/reducer/first"),
		specimenContribution("specimen-second", "Second", "specimen/reducer/second"),
	).Compose()
	if !ok {
		t.Fatal("specimen source did not compose")
	}
	if !composed.Complete() {
		t.Fatal("composed definition is incomplete")
	}
	if len(composed.Reducers) != 2 {
		t.Fatalf("composed reducers = %d, want 2", len(composed.Reducers))
	}
	want := []struct {
		rule schema.Key
		key  schema.Key
	}{
		{"specimen-first", "specimen/reducer/first"},
		{"specimen-second", "specimen/reducer/second"},
	}
	for index, row := range composed.Reducers {
		if row.Key != want[index].key {
			t.Fatalf("reducer[%d] key = %q, want %q", index, row.Key, want[index].key)
		}
		if row.Rule != want[index].rule {
			t.Fatalf("reducer[%d] rule = %q, want %q", index, row.Rule, want[index].rule)
		}
	}
}

// TestComposeRefusesABaseThatDeclaresItsOwnReducer is the law that removes the
// central per-axis list. A reducer authored beside the carriers is a fold no
// rule is on record as folding with, so it cannot be merged - it is refused.
func TestComposeRefusesABaseThatDeclaresItsOwnReducer(t *testing.T) {
	source := specimenSource(specimenContribution("specimen-first", "First", "specimen/reducer/first"))
	source.Base.Reducers = []Reducer{{
		Name:           "Central",
		Key:            "specimen/reducer/central",
		Candidate:      "SeedCarrier",
		Outputs:        []ReducerOutput{{Axis: specimenAxis(), Carrier: "FactCarrier"}},
		Implementation: GoSymbol{PackagePath: specimenPackage, Name: "CentralFold"},
	}}
	if _, ok := source.Compose(); ok {
		t.Fatal("a base that hand-lists a reducer composed")
	}
}

// TestAxisWithNoContributionsComposesNoReducerRows states the third half of the
// composition law: an axis whose rules declare no fold has an empty reducer
// vocabulary rather than a missing one, and the generator has nothing to emit
// for it.
func TestAxisWithNoContributionsComposesNoReducerRows(t *testing.T) {
	composed, ok := specimenSource().Compose()
	if !ok {
		t.Fatal("an axis with no reducer contributions did not compose")
	}
	if len(composed.Reducers) != 0 {
		t.Fatalf("composed reducers = %d, want 0", len(composed.Reducers))
	}
	catalog, catalogOK := composed.Catalog()
	if !catalogOK || catalog.ReducerCount() != 0 {
		t.Fatalf("catalog reducers = %d/%t, want 0/true", catalog.ReducerCount(), catalogOK)
	}
}

// TestComposeRefusesContradictoryContributions states what one vocabulary
// means: two rules cannot claim one reducer key or one reducer name, one rule
// cannot contribute twice, and a contribution cannot place a reducer in an
// axis other than the one it names.
func TestComposeRefusesContradictoryContributions(t *testing.T) {
	cases := map[string][]Contribution{
		"duplicate-reducer-key": {
			specimenContribution("specimen-first", "First", "specimen/reducer/shared"),
			specimenContribution("specimen-second", "Second", "specimen/reducer/shared"),
		},
		"duplicate-reducer-name": {
			specimenContribution("specimen-first", "Shared", "specimen/reducer/first"),
			specimenContribution("specimen-second", "Shared", "specimen/reducer/second"),
		},
		"duplicate-rule": {
			specimenContribution("specimen-first", "First", "specimen/reducer/first"),
			specimenContribution("specimen-first", "Second", "specimen/reducer/second"),
		},
		"unnamed-rule": {
			specimenContribution("", "First", "specimen/reducer/first"),
		},
	}
	for name, contributions := range cases {
		t.Run(name, func(t *testing.T) {
			if _, ok := specimenSource(contributions...).Compose(); ok {
				t.Fatal("contradictory contributions composed")
			}
		})
	}
	foreign := specimenContribution("specimen-first", "First", "specimen/reducer/first")
	foreign.Axis = "other"
	if _, ok := specimenSource(foreign).Compose(); ok {
		t.Fatal("a contribution placed a reducer in a foreign axis")
	}
}

// TestAnAuthoredContributionStatesTheFoldItClaims keeps the law that a
// contribution declaring no reducer is a rule claiming a fold it did not state.
// It is the roster's law rather than composition's: composition receives rows
// the roster folded in from another axis's rule, which carry no fold there and
// are not empty claims, so the law belongs where a package's own declaration
// is registered.
func TestAnAuthoredContributionStatesTheFoldItClaims(t *testing.T) {
	if _, ok := NewRoster(specimenSource(Contribution{Axis: "specimen", Rule: "specimen-empty"})); ok {
		t.Fatal("a contribution claiming a fold it did not state was registered")
	}
	if _, ok := NewRoster(specimenSource(specimenContribution("specimen-first", "First", "specimen/reducer/first"))); !ok {
		t.Fatal("a contribution stating its fold was refused")
	}
}

// TestComposeRefusesAContributionCarrierThatContradictsTheBase states the
// carrier fence: a contribution may repeat a carrier the base declares, but a
// repeat that disagrees is two declarations of one name.
func TestComposeRefusesAContributionCarrierThatContradictsTheBase(t *testing.T) {
	agreeing := specimenContribution("specimen-first", "First", "specimen/reducer/first")
	agreeing.Carriers = []Carrier{{Name: "FactCarrier", Key: "carrier/specimen/fact", Type: specimenType("Fact")}}
	if _, ok := specimenSource(agreeing).Compose(); !ok {
		t.Fatal("a contribution repeating a base carrier verbatim was refused")
	}
	contradicting := specimenContribution("specimen-first", "First", "specimen/reducer/first")
	contradicting.Carriers = []Carrier{{Name: "FactCarrier", Key: "carrier/specimen/other", Type: specimenType("Fact")}}
	if _, ok := specimenSource(contradicting).Compose(); ok {
		t.Fatal("a contribution redefined a base carrier")
	}
}

// TestRosterRefusesTwoOwnersOfOneVocabulary states the registry law: a name or
// an axis appearing twice is two owners of one vocabulary.
func TestRosterRefusesTwoOwnersOfOneVocabulary(t *testing.T) {
	if _, ok := NewRoster(specimenSource(), specimenSource()); ok {
		t.Fatal("two sources of one axis were admitted")
	}
	second := specimenSource()
	second.Name = "specimen-two"
	second.Base.Axis = "specimen-two"
	roster, ok := NewRoster(specimenSource(), second)
	if !ok || roster.Count() != 2 {
		t.Fatalf("distinct sources rejected: ok=%t count=%d", ok, roster.Count())
	}
	if _, resolved := roster.Source("absent"); resolved {
		t.Fatal("an unregistered source resolved")
	}
}

// routedContribution is one rule declaring the whole shape it folds over: the
// carrier its rows are typed in, the relation those rows come from, and the
// projection that addresses them. It is the shape a routed rule needs and the
// axis base has no reason to know about.
func routedContribution(rule schema.Key, relation string, key schema.Key) Contribution {
	provider := member.AxisRelationCandidate(member.RelationRef{Axis: specimenAxis(), Member: key})
	return Contribution{
		Axis: "specimen",
		Rule: rule,
		Carriers: []Carrier{
			{Name: "RouteCarrier", Key: "carrier/specimen/route", Type: specimenType("Route")},
		},
		Relations: []Relation{{
			Name:              relation,
			Key:               key,
			Subject:           "RouteCarrier",
			CandidateProvider: provider,
			CandidateResolver: specimenMethod("RouteForOccurrence", "Schema"),
			CandidateOrdinal:  specimenMethod("RouteOrdinal", "Schema"),
			CandidateAt:       specimenMethod("RouteAt", "Schema"),
		}},
		Projections: []Projection{{
			Name:              relation + "Coordinate",
			Key:               key + "/coordinate",
			Relation:          relation,
			CandidateProvider: provider,
			Role:              member.Destination,
			Result:            "KeyCarrier",
			Accessor:          specimenMethod("Result", "Route"),
		}},
		Reducers: []Reducer{{
			Name:           "Routed",
			Key:            "specimen/reducer/routed",
			Candidate:      "RouteCarrier",
			Outputs:        []ReducerOutput{{Axis: specimenAxis(), Carrier: "FactCarrier"}},
			Implementation: GoSymbol{PackagePath: specimenPackage, Name: "RoutedFold"},
		}},
	}
}

// TestAContributionDeclaresItsOwnRelationsAndProjections is the property the
// shape exists for: a rule that folds over rows the axis base never declared
// brings those rows with it. Adding such a rule edits that rule's package and
// the roster, and nothing in the axis owner's base.
func TestAContributionDeclaresItsOwnRelationsAndProjections(t *testing.T) {
	composed, ok := specimenSource(routedContribution("specimen-routed", "Routes", "specimen/routes")).Compose()
	if !ok {
		t.Fatal("a contribution carrying its own relation and projection does not compose")
	}
	relation, relationFound := findRelation(composed, "Routes")
	if !relationFound {
		t.Fatal("the contributed relation is absent from the composed definition")
	}
	if relation.Subject != "RouteCarrier" || relation.Key != "specimen/routes" {
		t.Fatalf("contributed relation composed as %+v", relation)
	}
	if _, found := findProjection(composed, "RoutesCoordinate"); !found {
		t.Fatal("the contributed projection is absent from the composed definition")
	}
	// The base's own rows are still there and still first: a contribution
	// extends the vocabulary, it does not replace it.
	if _, found := findRelation(composed, "Candidates"); !found {
		t.Fatal("composing a contribution dropped the base's own relation")
	}
	if composed.Relations[0].Name != "Candidates" {
		t.Fatalf("base relation is not first: %s", composed.Relations[0].Name)
	}
}

// TestComposeRefusesAContributionRelationThatContradictsTheBase holds the
// contributed rows to the same law carriers are held to. One name is one
// declaration, whoever wrote it.
func TestComposeRefusesAContributionRelationThatContradictsTheBase(t *testing.T) {
	contribution := routedContribution("specimen-routed", "Candidates", "specimen/other")
	if _, ok := specimenSource(contribution).Compose(); ok {
		t.Fatal("a contribution redeclaring the base's relation with different content composes")
	}
}

// TestComposeRefusesTwoContributionsContradictingOneRelation is the same law
// between two rules, where neither is the base and neither is privileged.
func TestComposeRefusesTwoContributionsContradictingOneRelation(t *testing.T) {
	first := routedContribution("specimen-first", "Routes", "specimen/routes")
	second := routedContribution("specimen-second", "Routes", "specimen/routes")
	second.Relations[0].Subject = "SeedCarrier"
	second.Reducers[0].Name = "SecondRouted"
	second.Reducers[0].Key = "specimen/reducer/second-routed"
	second.Projections[0].Name = "SecondCoordinate"
	second.Projections[0].Key = "specimen/routes/second"
	if _, ok := specimenSource(first, second).Compose(); ok {
		t.Fatal("two contributions declaring one relation name differently compose")
	}
}

// TestARepeatedRelationDeclarationIsAdmittedVerbatim is the other half: two
// rules folding over the same rows must each be able to say so, or the second
// rule is forced to depend on the first having been registered.
func TestARepeatedRelationDeclarationIsAdmittedVerbatim(t *testing.T) {
	first := routedContribution("specimen-first", "Routes", "specimen/routes")
	second := routedContribution("specimen-second", "Routes", "specimen/routes")
	second.Reducers[0].Name = "SecondRouted"
	second.Reducers[0].Key = "specimen/reducer/second-routed"
	composed, ok := specimenSource(first, second).Compose()
	if !ok {
		t.Fatal("two rules folding over one declared relation do not compose")
	}
	count := 0
	for _, relation := range composed.Relations {
		if relation.Name == "Routes" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("one relation declared twice composed to %d rows", count)
	}
}

// TestComposeRefusesTwoRelationNamesClaimingOneKey keeps the cold key space one
// to one with the declarations, so a generated relation ordinal names exactly
// one authored row.
func TestComposeRefusesTwoRelationNamesClaimingOneKey(t *testing.T) {
	contribution := routedContribution("specimen-routed", "Routes", "specimen/candidates")
	if _, ok := specimenSource(contribution).Compose(); ok {
		t.Fatal("a contributed relation taking the base relation's key composes")
	}
}

func findRelation(definition Definition, name string) (Relation, bool) {
	for _, relation := range definition.Relations {
		if relation.Name == name {
			return relation, true
		}
	}
	return Relation{}, false
}

func findProjection(definition Definition, name string) (Projection, bool) {
	for _, projection := range definition.Projections {
		if projection.Name == name {
			return projection, true
		}
	}
	return Projection{}, false
}

// twinRelation is a second self-providing candidate directory: its own key, its
// own provider reference, and the same directory symbols the base relation
// carries. Everything Complete inspects besides the identity under test is
// satisfied, so a refusal is attributable to that identity alone.
func twinRelation(name string, key schema.Key) Relation {
	twin := specimenBase().Relations[0]
	twin.Name = name
	twin.Key = key
	twin.CandidateProvider = member.AxisRelationCandidate(member.RelationRef{Axis: specimenAxis(), Member: key})
	return twin
}

// TestCompleteRefusesTwoRelationsClaimingOneKey states who owns the sealed
// relation key space. Two rows under one key do not merge and do not race:
// every provider that names that key resolves to whichever row was composed
// last, silently, and the generated ordinal then names a declaration nobody
// wrote.
func TestCompleteRefusesTwoRelationsClaimingOneKey(t *testing.T) {
	definition := specimenBase()
	definition.Relations = append(definition.Relations, twinRelation("Twin", definition.Relations[0].Key))
	if definition.Complete() {
		t.Fatal("a definition with two relations under one key is complete")
	}
	// The same definition with the twin's identity made distinct is complete,
	// which is what makes the refusal above attributable to the shared key.
	definition.Relations[1] = twinRelation("Twin", "specimen/twin")
	if !definition.Complete() {
		t.Fatal("the same definition with distinct relation keys is not complete")
	}
}

// TestCompleteRefusesTwoRelationsClaimingOneName is the same law over the name
// space projections and reducer inputs address relations by. The base's
// projection is left out so that the name collision is the only thing wrong
// with the definition under test.
func TestCompleteRefusesTwoRelationsClaimingOneName(t *testing.T) {
	definition := specimenBase()
	definition.Projections = nil
	definition.Relations = append(definition.Relations, twinRelation(definition.Relations[0].Name, "specimen/twin"))
	if definition.Complete() {
		t.Fatal("a definition with two relations under one name is complete")
	}
	definition.Relations[1] = twinRelation("Twin", "specimen/twin")
	if !definition.Complete() {
		t.Fatal("the same definition with distinct relation names is not complete")
	}
}

// TestCompleteRefusesTwoProjectionsClaimingOneIdentity holds projections to the
// relation law, over both the name they are addressed by and the key they seal.
func TestCompleteRefusesTwoProjectionsClaimingOneIdentity(t *testing.T) {
	for _, probe := range []struct {
		name    string
		collide func(*Projection, Projection)
	}{
		{name: "key", collide: func(twin *Projection, first Projection) { twin.Key = first.Key }},
		{name: "name", collide: func(twin *Projection, first Projection) { twin.Name = first.Name }},
	} {
		t.Run(probe.name, func(t *testing.T) {
			definition := specimenBase()
			twin := definition.Projections[0]
			twin.Name = "Twin"
			twin.Key = "specimen/twin"
			probe.collide(&twin, definition.Projections[0])
			definition.Projections = append(definition.Projections, twin)
			if definition.Complete() {
				t.Fatalf("a definition with two projections sharing one %s is complete", probe.name)
			}
		})
	}
}

// TestAGlobalDirectoryOwesACensusWithoutOwingAMaterializer separates the two
// obligations a candidate census answers to. A source relation's count is the
// exact width of the dense column its materializer fills, so a count with no
// materializer there is a width nothing writes. A global directory's count is
// the bound on the occurrence inventory a Link rule enumerates from it, which
// it owes whether or not any fact is materialized from its rows. Requiring a
// materializer for the second reading would force every Link candidate
// directory to invent a zero-input fact it does not have.
func TestAGlobalDirectoryOwesACensusWithoutOwingAMaterializer(t *testing.T) {
	global := func() Definition {
		base := specimenBase()
		base.Relations[0].CandidateCount = specimenMethod("SeedCount", "Schema")
		base.Relations[0].CandidateIdentityAt = specimenMethod("SeedIDAt", "Schema")
		return base
	}

	t.Run("admitted", func(t *testing.T) {
		if !global().Complete() {
			t.Fatal("a global directory with a census and no materializer was refused")
		}
	})

	t.Run("census-without-a-reading", func(t *testing.T) {
		base := specimenBase()
		base.Relations[0].CandidateCount = specimenMethod("SeedCount", "Schema")
		if base.Complete() {
			t.Fatal("a census with neither a materializer nor an occurrence directory was admitted")
		}
	})

	t.Run("directory-without-a-census", func(t *testing.T) {
		base := global()
		base.Relations[0].CandidateCount = GoSymbol{}
		if base.Complete() {
			t.Fatal("a global directory bounded by no census was admitted")
		}
	})

	t.Run("foreign-census", func(t *testing.T) {
		base := global()
		base.Relations[0].CandidateCount = specimenMethod("SeedCount", "Elsewhere")
		if base.Complete() {
			t.Fatal("a census authored outside the axis owner was admitted")
		}
	})
}

// foreignAxisBase is a second registered axis whose rows a specimen rule wants
// to join on: its key projection's accessor is a method the FOREIGN owner has,
// which is exactly why the row cannot live on the reading rule's axis.
func foreignAxisBase() Definition {
	provider := member.AxisRelationCandidate(member.RelationRef{Axis: schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "foreign"}, Member: "foreign/candidates"})
	return Definition{
		Name:       "Foreign",
		Axis:       "foreign",
		ImportPath: specimenPackage,
		Binding: Binding{Key: KeyNormalization{
			Carrier:    "ForeignKeyCarrier",
			Dense:      GoType{Name: "uint32"},
			Normalizer: specimenMethod("ForeignKeyIndex", "ForeignSchema"),
		}},
		Signature: Signature{Key: "ForeignKeyCarrier", Fact: "ForeignFactCarrier"},
		Carriers: []Carrier{
			{Name: "ForeignKeyCarrier", Key: "carrier/foreign/key", Type: specimenType("ForeignKey")},
			{Name: "ForeignFactCarrier", Key: "carrier/foreign/fact", Type: specimenType("ForeignFact")},
			{Name: "ForeignSeedCarrier", Key: "carrier/foreign/seed", Type: specimenType("ForeignSeed")},
		},
		Relations: []Relation{{
			Name:              "ForeignCandidates",
			Key:               "foreign/candidates",
			Subject:           "ForeignSeedCarrier",
			CandidateProvider: provider,
			CandidateResolver: specimenMethod("ForeignSeedForOccurrence", "ForeignSchema"),
			CandidateOrdinal:  specimenMethod("ForeignSeedOrdinal", "ForeignSchema"),
			CandidateAt:       specimenMethod("ForeignSeedAt", "ForeignSchema"),
		}},
	}
}

func foreignSource() Source {
	return Source{Package: "foreign", Name: "foreign", Base: foreignAxisBase()}
}

// foreignJoinContribution is a specimen rule that folds on its own axis but
// reads a foreign one: the rows it joins over are the foreign axis's rows, so
// it declares them naming that axis.
func foreignJoinContribution() Contribution {
	contribution := specimenContribution("specimen-foreign", "Foreign", "specimen/reducer/foreign")
	provider := member.AxisRelationCandidate(member.RelationRef{Axis: schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "foreign"}, Member: "foreign/specimen-joins"})
	contribution.Carriers = []Carrier{
		{Name: "JoinCarrier", Key: "carrier/foreign/join", Type: specimenType("ForeignJoin")},
	}
	contribution.Relations = []Relation{{
		Name:              "ForeignJoins",
		Key:               "foreign/specimen-joins",
		Axis:              "foreign",
		Subject:           "JoinCarrier",
		CandidateProvider: provider,
		CandidateResolver: specimenMethod("JoinForOccurrence", "ForeignSchema"),
		CandidateOrdinal:  specimenMethod("JoinOrdinal", "ForeignSchema"),
		CandidateAt:       specimenMethod("JoinAt", "ForeignSchema"),
	}}
	contribution.Projections = []Projection{{
		Name:              "ForeignJoinKey",
		Key:               "foreign/specimen-joins/key",
		Axis:              "foreign",
		Relation:          "ForeignJoins",
		CandidateProvider: provider,
		Role:              member.Key,
		Result:            "ForeignKeyCarrier",
		Accessor:          specimenMethod("JoinKey", "ForeignJoin"),
	}}
	return contribution
}

// TestARowIsFoldedIntoTheSourceOfTheAxisItNames is ruling (a): a relation over
// a foreign axis's coordinates is that axis's data whichever rule declares it.
//
// The reading rule keeps its fold on the axis it writes, and the rows it joins
// over land in the source that owns them - where the key projection's accessor
// is a method the owner actually has. The alternative was for the reading
// domain's candidate to carry the foreign coordinate, which is a schema-level
// dependency between two domains that have none.
func TestARowIsFoldedIntoTheSourceOfTheAxisItNames(t *testing.T) {
	roster, rosterOK := NewRoster(specimenSource(foreignJoinContribution()), foreignSource())
	if !rosterOK {
		t.Fatal("a rule declaring rows of the axis it reads was refused")
	}
	_, specimen, specimenOK := roster.Definition("specimen")
	if !specimenOK {
		t.Fatal("the reading axis does not compose")
	}
	if _, present := findRelation(specimen, "ForeignJoins"); present {
		t.Fatal("a foreign axis's rows stayed in the reading rule's own axis")
	}
	if len(specimen.Reducers) != 1 || specimen.Reducers[0].Rule != "specimen-foreign" {
		t.Fatalf("the reading rule's fold did not stay on the axis it writes: %+v", specimen.Reducers)
	}
	_, foreign, foreignOK := roster.Definition("foreign")
	if !foreignOK {
		t.Fatal("the read axis does not compose with the folded rows")
	}
	relation, relationFound := findRelation(foreign, "ForeignJoins")
	if !relationFound || relation.Axis != "foreign" || relation.Subject != "JoinCarrier" {
		t.Fatalf("folded relation = %+v found=%t", relation, relationFound)
	}
	projection, projectionFound := findProjection(foreign, "ForeignJoinKey")
	if !projectionFound || projection.Axis != "foreign" {
		t.Fatalf("folded projection = %+v found=%t", projection, projectionFound)
	}
	if len(foreign.Reducers) != 0 {
		t.Fatalf("the reading rule's fold followed its rows into the read axis: %+v", foreign.Reducers)
	}
	carried := false
	for _, carrier := range foreign.Carriers {
		if carrier.Name == "JoinCarrier" {
			carried = true
		}
	}
	if !carried {
		t.Fatal("a folded row's subject carrier did not travel with it")
	}
}

// TestARowNamingAnUnregisteredAxisHasNoHome refuses the roster where the row is
// written. A row whose axis no source owns cannot be placed, and discovering
// that when a plan fails to resolve the relation names the wrong defect.
func TestARowNamingAnUnregisteredAxisHasNoHome(t *testing.T) {
	contribution := foreignJoinContribution()
	contribution.Relations[0].Axis = "unregistered"
	contribution.Projections[0].Axis = "unregistered"
	if _, ok := NewRoster(specimenSource(contribution), foreignSource()); ok {
		t.Fatal("a row naming an axis no source owns was registered")
	}
}

// TestADefinitionHoldsOnlyTheRowsOfItsOwnAxis is the seal-side half: whatever
// placed a row, a definition that ends up holding one belonging to another axis
// is refused where the vocabulary seals.
func TestADefinitionHoldsOnlyTheRowsOfItsOwnAxis(t *testing.T) {
	misplaced := specimenBase()
	misplaced.Relations[0].Axis = "foreign"
	if misplaced.Complete() {
		t.Fatal("a definition holding another axis's relation sealed")
	}
	misplaced = specimenBase()
	misplaced.Projections[0].Axis = "foreign"
	if misplaced.Complete() {
		t.Fatal("a definition holding another axis's projection sealed")
	}
	home := specimenBase()
	home.Relations[0].Axis = "specimen"
	home.Projections[0].Axis = "specimen"
	if !home.Complete() {
		t.Fatal("a definition holding rows that name its own axis was refused")
	}
}
