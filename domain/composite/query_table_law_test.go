package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/query"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// publishedQueryFamilies is the analyzer's published query identity space,
// spelled by the exported constants a consumer opens a result slot with. It is
// the independent statement of what the sealed surface owes: a family declared
// under no published key, or a published key no family declares, is a
// disagreement between the inventory and the identities the analyzer exports.
func publishedQueryFamilies() []schema.Key {
	return []schema.Key{QueryFamilyValueSummary, QueryFamilyEffectExact}
}

// TestQueryTableSeals states that the authored query inventory is admitted and
// sealed by the one declaration root, and that every family reads a coordinate
// space the same table declares.
func TestQueryTableSeals(t *testing.T) {
	roles, rolesOK := SemanticRoles()
	registrations, registrationsOK := queryRegistrations(roles)
	if !rolesOK || !registrationsOK {
		t.Fatal("declared query identities did not resolve")
	}
	sealed, failure := Table()
	if failure.Available() || sealed == nil {
		t.Fatalf("declaration table rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	axes, axesOK := sealed.Surface(schema.SurfaceKindAxis)
	view, viewOK := sealed.Surface(schema.SurfaceKindQuery)
	if !axesOK || !viewOK {
		t.Fatal("sealed table holds no axis or query surface")
	}
	families := publishedQueryFamilies()
	if view.Count() != len(families) {
		t.Fatalf("sealed query surface holds %d rows for %d published families", view.Count(), len(families))
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
		if registration.Population() != query.PopulationSelectedPoint {
			t.Fatalf("family %q is asked at %q", entry.Key(), registration.Population())
		}
		if registration.Projection() != query.ProjectionSummary && registration.Projection() != query.ProjectionExact {
			t.Fatalf("family %q declares projection %q", entry.Key(), registration.Projection())
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
	roles, rolesOK := SemanticRoles()
	registrations, registrationsOK := queryRegistrations(roles)
	if !rolesOK || !registrationsOK {
		t.Fatal("declared query identities did not resolve")
	}
	issued := QueryIssuance()
	if len(issued) != len(registrations) {
		t.Fatalf("issuance holds %d families for %d sealed rows", len(issued), len(registrations))
	}
	for index, registration := range registrations {
		family := issued[index]
		if family.Family != registration.Key() ||
			family.Authority != registration.Key() ||
			family.Population != registration.Population() ||
			family.Projection != registration.Projection() {
			t.Fatalf("issuance row %d is not sealed family %q", index, registration.Key())
		}
		position, resolved := queryPositionForFamily(family.Authority)
		if !resolved || registry.queries[position].Key() != family.Family {
			t.Fatalf("authority %q does not resolve to sealed family %q", family.Authority, family.Family)
		}
	}
}

// TestEveryQueryFamilyIsInventoriedOnce is the composition law of the one query
// table: a family is a member of the inventory exactly once, so a family's
// declaration, its contributor, and the slot its answers are published in are
// reached through one row and there is no second list to disagree with it.
func TestEveryQueryFamilyIsInventoriedOnce(t *testing.T) {
	roles, rolesOK := SemanticRoles()
	registrations, registrationsOK := queryRegistrations(roles)
	if !rolesOK || !registrationsOK {
		t.Fatal("declared query identities did not resolve")
	}
	counted := make(map[schema.Key]int, len(registrations))
	for _, registration := range registrations {
		counted[registration.Key()]++
	}
	for _, family := range publishedQueryFamilies() {
		if counted[family] != 1 {
			t.Fatalf("published family %q appears %d times in the query inventory", family, counted[family])
		}
	}
	if len(counted) != len(publishedQueryFamilies()) {
		t.Fatalf("query inventory holds %d families for %d published keys", len(counted), len(publishedQueryFamilies()))
	}
}

// TestQueryCodecsAreTheSchemaFreezerIdentities is the drift law of this
// inventory: a family is published under the same freezer identity the sealed
// schema opens its query slot with, so the declaration and the slot cannot
// name two contracts.
func TestQueryCodecsAreTheSchemaFreezerIdentities(t *testing.T) {
	roles, rolesOK := SemanticRoles()
	registrations, registrationsOK := queryRegistrations(roles)
	if !rolesOK || !registrationsOK {
		t.Fatal("declared query identities did not resolve")
	}
	declared := make(map[schema.Key]identity.ContentID)
	for _, registration := range registrations {
		declared[registration.Key()] = registration.Codec()
	}
	valueCodec, valueCodecOK := roles.Key("semantic/query-result/value-summary")
	effectCodec, effectCodecOK := roles.Key("semantic/query-result/effect-exact")
	if !valueCodecOK || !effectCodecOK {
		t.Fatal("declared query codec roles did not resolve")
	}
	for family, codec := range map[schema.Key]identity.ContentID{
		QueryFamilyValueSummary: identity.ContentID(valueCodec.Digest()),
		QueryFamilyEffectExact:  identity.ContentID(effectCodec.Digest()),
	} {
		if declared[family] != codec {
			t.Fatalf("family %q is declared under a codec the schema does not freeze its results with", family)
		}
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
	roles, rolesOK := SemanticRoles()
	if !rolesOK {
		t.Fatal("declared query identities did not resolve")
	}
	value := valueowner.QueryEntry()
	value.Bind = nil
	if _, admitted := query.New(value, roles); admitted {
		t.Fatal("value-summary was admitted without the contributor that folds it")
	}
	effect := effectowner.QueryEntry()
	effect.Declare = nil
	if _, admitted := query.New(effect, roles); admitted {
		t.Fatal("effect-exact was admitted without the contributor that declares its slot")
	}
}

// TestObservationProducersAreIssuedQueryFamilies states that every observation
// row names a sealed query family as its producer and carries population,
// geometry, and anchor. Observation does not invent a family construction
// does not issue.
func TestObservationProducersAreIssuedQueryFamilies(t *testing.T) {
	roles, rolesOK := SemanticRoles()
	queries, queriesOK := queryRegistrations(roles)
	specs, specsOK := observationSpecs(queries)
	if !rolesOK || !queriesOK || !specsOK {
		t.Fatal("observation inventory did not derive from the sealed query families")
	}
	issued := make(map[schema.Key]bool, len(QueryIssuance()))
	for _, family := range QueryIssuance() {
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
	roles, rolesOK := SemanticRoles()
	queries, queriesOK := queryRegistrations(roles)
	entries, entriesOK := observationEntries(queries)
	if !rolesOK || !queriesOK || !entriesOK {
		t.Fatal("observation inventory did not derive from the sealed query families")
	}
	issued := ObservationIssuance()
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
