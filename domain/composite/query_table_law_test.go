package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	"github.com/wippyai/go-lua/analysis/schema/plane"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	"github.com/wippyai/go-lua/domain/effect/factor"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
	placementquery "github.com/wippyai/go-lua/domain/placement/query"
	"github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// publishedQueryFamilies is the analyzer's published query identity space,
// spelled by the exported constants a consumer opens a result slot with. It is
// the independent statement of what the sealed surface owes: a family declared
// under no published key, or a published key no family declares, is a
// disagreement between the inventory and the identities the analyzer exports.
func publishedQueryFamilies() []schema.Key {
	return []schema.Key{QueryFamilyValueSummary, QueryFamilyEffectExact, QueryFamilyPlacementSummary}
}

// queryPositionPins preserves the existing result-column ordinals while
// appending Placement's family at the end of the sealed query inventory.
func queryPositionPins() []schema.Key {
	return []schema.Key{QueryFamilyValueSummary, QueryFamilyEffectExact, QueryFamilyPlacementSummary}
}

// queryIssuancePins includes the producer-only observation family after the
// three selected-point Result families. The first three positions are the
// existing Result ordinals and must not move.
func queryIssuancePins() []schema.Key {
	return []schema.Key{QueryFamilyValueSummary, QueryFamilyEffectExact, QueryFamilyPlacementSummary, QueryFamilyCallCalleeSet}
}

// TestQueryTableSeals states that the authored query inventory is admitted and
// sealed by the one declaration root, and that every family reads a coordinate
// space the same table declares.
func TestQueryTableSeals(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	roles, rolesOK := SemanticRoles(compilation)
	registrations, _, registrationsOK := queryRegistrations(roles, queryGeometryTypes(t), queryPlacementSummaryTypes(t))
	if !rolesOK || !registrationsOK {
		t.Fatal("declared query identities did not resolve")
	}
	sealed, failure := Table(compilation)
	if failure.Available() || sealed == nil {
		t.Fatalf("declaration table rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	axes, axesOK := sealed.Surface(schema.SurfaceKindAxis)
	view, viewOK := sealed.Surface(schema.SurfaceKindQuery)
	if !axesOK || !viewOK {
		t.Fatal("sealed table holds no axis or query surface")
	}
	families := queryIssuancePins()
	if view.Count() != len(families) {
		t.Fatalf("sealed query surface holds %d rows for %d issued families", view.Count(), len(families))
	}
	for _, family := range families {
		if _, declared := view.ByID(schema.NewEntryID(schema.SurfaceKindQuery, family)); !declared {
			t.Fatalf("published family %q is declared by no sealed row", family)
		}
	}
	for position, entry := range registrations {
		row, rowOK := view.At(position)
		registration, registrationOK := row.(*query.Registration)
		if !rowOK || !registrationOK || registration.Key() != entry.Key() {
			t.Fatalf("query row %d is not the authored family %q", position, entry.Key())
		}
		if position < len(queryPositionPins()) {
			if registration.Population() != query.PopulationSelectedPoint {
				t.Fatalf("family %q is asked at %q", entry.Key(), registration.Population())
			}
			if registration.Projection() != query.ProjectionSummary && registration.Projection() != query.ProjectionExact {
				t.Fatalf("family %q declares projection %q", entry.Key(), registration.Projection())
			}
		} else if registration.Key() != QueryFamilyCallCalleeSet || registration.Population() != query.PopulationObservation || registration.Projection() != query.ProjectionExact {
			t.Fatalf("producer-only family %q has population/projection %q/%q", registration.Key(), registration.Population(), registration.Projection())
		}
		for index := 0; index < registration.SubjectCount(); index++ {
			subject, subjectOK := registration.SubjectAt(index)
			if !subjectOK {
				t.Fatalf("family %q holds no subject at %d", entry.Key(), index)
			}
			if _, declared := axes.ByID(schema.NewEntryID(schema.SurfaceKindAxis, subject)); !declared {
				t.Fatalf("family %q reads axis %q, which is not declared", entry.Key(), subject)
			}
		}
	}
}

// TestQueryIssuanceIsTheSealedInventory states that construction walks the
// same families the table sealed, under the population and projection those
// families declared.
func TestQueryIssuanceIsTheSealedInventory(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	roles, rolesOK := SemanticRoles(compilation)
	registrations, _, registrationsOK := queryRegistrations(roles, queryGeometryTypes(t), queryPlacementSummaryTypes(t))
	if !rolesOK || !registrationsOK {
		t.Fatal("declared query identities did not resolve")
	}
	issued := QueryIssuance(compilation)
	if len(issued) != len(registrations) {
		t.Fatalf("issuance holds %d families for %d sealed rows", len(issued), len(registrations))
	}
	pins := queryIssuancePins()
	if len(issued) != len(pins) {
		t.Fatalf("query issuance holds %d families, but %d issuance ordinals are pinned", len(issued), len(pins))
	}
	for position, key := range pins {
		if issued[position].Family != key {
			t.Fatalf("query family at position %d is %q, want %q", position, issued[position].Family, key)
		}
	}
	for index, registration := range registrations {
		family := issued[index]
		if family.Family != registration.Key() ||
			family.Authority != registration.Key() ||
			family.Population != registration.Population() ||
			family.Projection != registration.Projection() {
			t.Fatalf("issuance row %d is not sealed family %q", index, registration.Key())
		}
		position, resolved := queryPositionForFamily(compilation.catalog, family.Authority)
		if !resolved || compilation.catalog.queries[position].Key() != family.Family {
			t.Fatalf("authority %q does not resolve to sealed family %q", family.Authority, family.Family)
		}
	}
}

// TestEveryQueryFamilyIsInventoriedOnce is the composition law of the one query
// table: a family is a member of the inventory exactly once, so a family's
// declaration, its contributor, and the slot its answers are published in are
// reached through one row and there is no second list to disagree with it.
func TestEveryQueryFamilyIsInventoriedOnce(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	roles, rolesOK := SemanticRoles(compilation)
	registrations, _, registrationsOK := queryRegistrations(roles, queryGeometryTypes(t), queryPlacementSummaryTypes(t))
	if !rolesOK || !registrationsOK {
		t.Fatal("declared query identities did not resolve")
	}
	counted := make(map[schema.Key]int, len(registrations))
	for _, registration := range registrations {
		counted[registration.Key()]++
	}
	for _, family := range queryIssuancePins() {
		if counted[family] != 1 {
			t.Fatalf("issued family %q appears %d times in the query inventory", family, counted[family])
		}
	}
	if len(counted) != len(queryIssuancePins()) {
		t.Fatalf("query inventory holds %d families for %d issued keys", len(counted), len(queryIssuancePins()))
	}
	for _, family := range publishedQueryFamilies() {
		if counted[family] != 1 {
			t.Fatalf("selected-point family %q appears %d times in the query inventory", family, counted[family])
		}
	}
}

// TestQueryCodecsAreTheSchemaFreezerIdentities is the drift law of this
// inventory: a family is published under the same freezer identity the sealed
// schema opens its query slot with, so the declaration and the slot cannot
// name two contracts.
func TestQueryCodecsAreTheSchemaFreezerIdentities(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	roles, rolesOK := SemanticRoles(compilation)
	registrations, _, registrationsOK := queryRegistrations(roles, queryGeometryTypes(t), queryPlacementSummaryTypes(t))
	if !rolesOK || !registrationsOK {
		t.Fatal("declared query identities did not resolve")
	}
	declared := make(map[schema.Key]identity.ContentID)
	for _, registration := range registrations {
		declared[registration.Key()] = registration.Codec()
	}
	valueCodec, valueCodecOK := roles.Key("semantic/query-result/value-summary")
	effectCodec, effectCodecOK := roles.Key("semantic/query-result/effect-exact")
	placementCodec, placementCodecOK := roles.Key("semantic/query-result/placement-summary")
	callCodec, callCodecOK := roles.Key("semantic/query-result/call-callee-set")
	if !valueCodecOK || !effectCodecOK || !placementCodecOK || !callCodecOK {
		t.Fatal("declared query codec roles did not resolve")
	}
	for family, codec := range map[schema.Key]identity.ContentID{
		QueryFamilyValueSummary:     identity.ContentID(valueCodec.Digest()),
		QueryFamilyEffectExact:      identity.ContentID(effectCodec.Digest()),
		QueryFamilyPlacementSummary: identity.ContentID(placementCodec.Digest()),
		QueryFamilyCallCalleeSet:    identity.ContentID(callCodec.Digest()),
	} {
		if declared[family] != codec {
			t.Fatalf("family %q is declared under a codec the schema does not freeze its results with", family)
		}
	}
}

// TestCallCalleeSetIsAProducerOnlyIssuedFamily states that Call's query is
// sealed and bound as a typed observation producer, while the Result lane
// remains absent and therefore cannot acquire a selected-point publication.
func TestCallCalleeSetIsAProducerOnlyIssuedFamily(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	roles, rolesOK := SemanticRoles(compilation)
	registrations, contributors, registrationsOK := queryRegistrations(roles, queryGeometryTypes(t), queryPlacementSummaryTypes(t))
	if !rolesOK || !registrationsOK {
		t.Fatal("declared query identities did not resolve")
	}
	position := -1
	for index, registration := range registrations {
		if registration != nil && registration.Key() == QueryFamilyCallCalleeSet {
			position = index
			break
		}
	}
	if position != len(queryPositionPins()) || position >= len(contributors) {
		t.Fatalf("Call producer-only family position = %d, want %d", position, len(queryPositionPins()))
	}
	registration := registrations[position]
	contributor := contributors[position]
	if registration.Population() != query.PopulationObservation || registration.Projection() != query.ProjectionExact {
		t.Fatalf("Call query population/projection = %q/%q", registration.Population(), registration.Projection())
	}
	if !contributor.producerComplete() || contributor.resultComplete() || contributor.complete() {
		t.Fatal("Call query did not retain producer-only capability split")
	}
}

// TestWithdrawingAContributorRefusesTheFamily states that a family is answered
// by its contributor and by nothing else. Withdrawing one hook of an owning
// domain's declaration is refused at admission, so the family never reaches the
// table to be answered from a fallback.
//
// The withdrawal is performed on a copy of the authored declaration inside this
// test. Production holds no hook that can remove a contributor.
func TestWithdrawingAContributorRefusesTheFamily(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	roles, rolesOK := SemanticRoles(compilation)
	if !rolesOK {
		t.Fatal("declared query identities did not resolve")
	}
	geometry := queryGeometryTypes(t)
	if _, _, admitted := wireQuery(valueowner.QuerySpec(geometry), roles, valueowner.DeclareQuery, nil, valueowner.RecoverQuery, engine.NewSummaryQueryAdmission, value.SummaryPublication()); admitted {
		t.Fatal("value-summary was admitted without the contributor that folds it")
	}
	if _, _, admitted := wireQuery(effectowner.QuerySpec(geometry), roles, nil, effectowner.BindQuery, effectowner.RecoverQuery, engine.NewExactQueryAdmission, factor.ExactPublication()); admitted {
		t.Fatal("effect-exact was admitted without the contributor that declares its slot")
	}
	if _, _, admitted := wireUnplanedQuery(placementquery.QuerySpec(geometry, queryPlacementSummaryTypes(t)), roles, placementquery.DeclareQuery, nil, placementquery.RecoverQuery, engine.NewHeterogeneousQueryAdmission, placementquery.EncodeQueryAnswer); admitted {
		t.Fatal("placement-summary was admitted without the contributor that binds it")
	}
	if _, _, admitted := wireQuery(valueowner.QuerySpec(geometry), roles, valueowner.DeclareQuery, valueowner.BindQuery, valueowner.RecoverQuery, nil, value.SummaryPublication()); admitted {
		t.Fatal("value-summary was admitted without its owner admission callback")
	}
	if _, _, admitted := wireQuery(valueowner.QuerySpec(geometry), roles, valueowner.DeclareQuery, valueowner.BindQuery, valueowner.RecoverQuery, engine.NewSummaryQueryAdmission, plane.Publication[value.ValueSummaryObservation]{}); admitted {
		t.Fatal("value-summary was admitted without its publication declaration")
	}
}

// TestObservationProducerDoesNotNeedResultPublication states the population
// split at the composition boundary. An observation family can seal its typed
// declaration, binding, and recovery path while leaving Result admission and
// encoding absent; construction can register that producer without inventing
// a byte codec or a selected-point row.
func TestObservationProducerDoesNotNeedResultPublication(t *testing.T) {
	roles := queryCapabilityLawRoles(t)
	spec := valueowner.QuerySpec(queryGeometryTypes(t))
	spec.Family = "value-summary-observation-producer-law"
	spec.Population = query.PopulationObservation
	registration, contributor, admitted := wireObservation(spec, roles, valueowner.DeclareQuery, valueowner.BindQuery, valueowner.RecoverQuery)
	if !admitted || registration == nil {
		t.Fatal("observation producer was rejected without Result callbacks")
	}
	if !contributor.producerComplete() {
		t.Fatal("observation producer lost its typed producer capability")
	}
	if contributor.resultComplete() || contributor.complete() {
		t.Fatal("observation producer acquired a fabricated Result capability")
	}
	if _, admitted := contributor.admit(nil, query.Cell{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, executioncontext.Context{}); admitted {
		t.Fatal("producer-only observation exposed a selected-point admission")
	}
	if !contributor.registrable(registration) {
		t.Fatal("observation producer is not registrable through the population law")
	}
}

// TestSelectedPointProducerRequiresCompleteResultPublication is the converse
// law: a selected-point row is a Result lane, so a typed producer with no
// admission/encoder/contract cannot reach the query table.
func TestSelectedPointProducerRequiresCompleteResultPublication(t *testing.T) {
	roles := queryCapabilityLawRoles(t)
	spec := valueowner.QuerySpec(queryGeometryTypes(t))
	spec.Family = "value-summary-selected-result-law"
	registration, contributor, admitted := wireQuery(spec, roles, valueowner.DeclareQuery, valueowner.BindQuery, valueowner.RecoverQuery, nil, value.SummaryPublication())
	if admitted {
		t.Fatal("selected-point producer without Result capability was admitted")
	}
	if registration != nil || contributor.producerComplete() {
		t.Fatal("selected-point producer without Result capability escaped the fail-closed boundary")
	}
	if contributor.resultComplete() || contributor.complete() {
		t.Fatal("incomplete selected-point Result capability reported complete")
	}
}

// TestSelectedPointProducerWithResultPublicationRemainsAdmitted pins the
// existing selected-point path: adding the capability split must not alter a
// complete family's contract or its admission eligibility.
func TestSelectedPointProducerWithResultPublicationRemainsAdmitted(t *testing.T) {
	roles := queryCapabilityLawRoles(t)
	spec := valueowner.QuerySpec(queryGeometryTypes(t))
	spec.Family = "value-summary-selected-complete-law"
	registration, contributor, admitted := wireQuery(spec, roles, valueowner.DeclareQuery, valueowner.BindQuery, valueowner.RecoverQuery, engine.NewSummaryQueryAdmission, value.SummaryPublication())
	if !admitted || registration == nil || !contributor.producerComplete() || !contributor.resultComplete() || !contributor.complete() {
		t.Fatal("complete selected-point producer did not retain Result publication capability")
	}
	if contributor.queryResultPublication.contract.FamilyID() != identity.ContentID(registration.EntryID()) || contributor.queryResultPublication.contract.Codec() != registration.Freezer() {
		t.Fatal("selected-point Result contract drifted from registration identity")
	}
}

// TestObservationPartialResultCapabilityDoesNotSilentlyPass exercises the
// all-or-nothing Result unit. An observation may omit Result entirely, but a
// partially supplied capability is malformed rather than a reason to retain a
// callback that cannot publish a canonical cell.
func TestObservationPartialResultCapabilityDoesNotSilentlyPass(t *testing.T) {
	roles := queryCapabilityLawRoles(t)
	spec := valueowner.QuerySpec(queryGeometryTypes(t))
	spec.Family = "value-summary-observation-partial-result-law"
	spec.Population = query.PopulationObservation
	if _, _, admitted := wireQuery(spec, roles, valueowner.DeclareQuery, valueowner.BindQuery, valueowner.RecoverQuery, nil, value.SummaryPublication()); admitted {
		t.Fatal("observation accepted a partial Result capability")
	}
}

func TestUnknownQueryPopulationDoesNotAcquireProducerOnlyAdmission(t *testing.T) {
	roles := queryCapabilityLawRoles(t)
	spec := valueowner.QuerySpec(queryGeometryTypes(t))
	spec.Family = "value-summary-unknown-population-law"
	spec.Population = "semantic/query/population/unknown-law"
	if _, _, admitted := wireObservation(spec, roles, valueowner.DeclareQuery, valueowner.BindQuery, valueowner.RecoverQuery); admitted {
		t.Fatal("unknown query population acquired observation-producer admission")
	}
}

func queryCapabilityLawRoles(t *testing.T) vocabulary.Roles {
	t.Helper()
	specs := queryRoleVocabulary()
	specs = append(specs, vocabulary.RoleSpecs("query/population/unknown-law")...)
	specs = append(specs, vocabulary.RoleSpecs("query/value-summary", "query-result/value-summary", "factor/value/summary-coordinatewise")...)
	entries, collected := structure.Collect(specs)
	if !collected {
		t.Fatal("query capability role inventory did not collect")
	}
	roles, resolved := vocabulary.NewRoles(entries)
	if !resolved {
		t.Fatal("query capability role inventory did not resolve")
	}
	return roles
}

// TestObservationProducersAreIssuedQueryFamilies states that every observation
// row names a sealed query family as its producer and carries population,
// geometry, and anchor. Observation does not invent a family construction
// does not issue.
func TestObservationProducersAreIssuedQueryFamilies(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	roles, rolesOK := SemanticRoles(compilation)
	queries, _, queriesOK := queryRegistrations(roles, queryGeometryTypes(t), queryPlacementSummaryTypes(t))
	specs, specsOK := observationSpecs(queries)
	if !rolesOK || !queriesOK || !specsOK {
		t.Fatal("observation inventory did not derive from the sealed query families")
	}
	issued := make(map[schema.Key]bool, len(QueryIssuance(compilation)))
	for _, family := range QueryIssuance(compilation) {
		issued[family.Family] = true
	}
	if len(specs) == 0 {
		t.Fatal("no observation rows were derived")
	}
	for _, spec := range specs {
		if spec.Producer.Surface != schema.SurfaceKindQuery || !issued[spec.Producer.Key] {
			t.Fatalf("observation %q names producer %q, which QueryIssuance does not issue", spec.Key, spec.Producer.Key)
		}
		if !spec.Population.Available() || !spec.Geometry.Available() || !spec.Anchor.Available() {
			t.Fatalf("observation %q is missing population, geometry, or anchor", spec.Key)
		}
	}
}

// TestObservationIssuanceIsTheSealedInventory states that construction walks
// the same observation rows the table sealed, under the producer those rows
// declared.
func TestObservationIssuanceIsTheSealedInventory(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	roles, rolesOK := SemanticRoles(compilation)
	queries, _, queriesOK := queryRegistrations(roles, queryGeometryTypes(t), queryPlacementSummaryTypes(t))
	entries, entriesOK := observationEntries(queries)
	if !rolesOK || !queriesOK || !entriesOK {
		t.Fatal("observation inventory did not derive from the sealed query families")
	}
	issued := ObservationIssuance(compilation)
	if len(issued) != len(entries) {
		t.Fatalf("issuance holds %d observations for %d sealed rows", len(issued), len(entries))
	}
	for index, entry := range entries {
		row := issued[index]
		if row.Key != entry.Key() || row.Producer != entry.Producer().Key {
			t.Fatalf("issuance row %d is not sealed observation %q", index, entry.Key())
		}
		if !row.Population.Available() || !row.Geometry.Available() || !row.Anchor.Available() {
			t.Fatalf("issuance row %d is missing population, geometry, or anchor", index)
		}
	}
}
