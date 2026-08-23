package composite

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/plane"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	callquery "github.com/wippyai/go-lua/domain/call/query"
	"github.com/wippyai/go-lua/domain/effect/factor"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
	"github.com/wippyai/go-lua/domain/placement"
	placementquery "github.com/wippyai/go-lua/domain/placement/query"
	"github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// The registered query families. A family's authored key is its one spelling
// in the analyzer and is the key its declaration row is identified by. The
// spelling is the owning domain's; these are the constants a consumer opens a
// result slot with, and the table's own law holds the two to one name.
const (
	QueryFamilyValueSummary     = value.SummaryResultFamily
	QueryFamilyEffectExact      = factor.ExactResultFamily
	QueryFamilyPlacementSummary = placement.SummaryResultFamily
	QueryFamilyCallCalleeSet    = callquery.CalleeSetResultFamily
)

// queryRegistrations is the authored analyzer query inventory: the three
// selected-point families and the producer-only Call observation family the
// sealed schema opens a query slot for. Each row instantiates one owning
// domain's own query declaration, which carries that family's
// identities, subject axes, and fold contract. Selected-point rows also carry
// the Result publication capability; observation rows may carry only their
// typed producer.
//
// The declaration lives with the domain that owns the facts the family is
// answered from, exactly as an axis declaration does, so the inventory here
// states membership and order alone and no fold reaches this package. A family
// appears once: this list is the whole query surface, and a family withheld
// from it is a family nothing answers rather than one answered by a default.
func queryRegistrations(roles vocabulary.Roles) ([]*query.Registration, []queryContributor, bool) {
	var admitted []*query.Registration
	var contributors []queryContributor
	rejected := false
	add := func(entry *query.Registration, contributor queryContributor, ok bool) {
		if !ok || !contributor.registrable(entry) {
			rejected = true
			return
		}
		admitted = append(admitted, entry)
		contributors = append(contributors, contributor)
	}

	add(wireQuery(valueowner.QuerySpec(), roles, valueowner.DeclareQuery, valueowner.BindQuery, valueowner.RecoverQuery,
		engine.NewSummaryQueryAdmission, value.SummaryPublication()))
	add(wireQuery(effectowner.QuerySpec(), roles, effectowner.DeclareQuery, effectowner.BindQuery, effectowner.RecoverQuery,
		engine.NewExactQueryAdmission, factor.ExactPublication()))
	// Placement still detaches its answers with a codec of its own, so it
	// declares no publication and the composition seals it no layout. CX-10
	// cuts that codec onto the plane and this arm goes with it.
	add(wireUnplanedQuery(placementquery.QuerySpec(), roles, placementquery.DeclareQuery, placementquery.BindQuery, placementquery.RecoverQuery,
		engine.NewHeterogeneousQueryAdmission, placementquery.EncodeQueryAnswer))
	// Call's callee set is an observation population. It carries its typed
	// producer alone: no selected-point admission and no published answer.
	add(wireObservation(callquery.QuerySpec(), roles, callquery.DeclareQuery, callquery.BindQuery, callquery.RecoverQuery))

	if rejected {
		return nil, nil, false
	}
	return admitted, contributors, true
}

// queryRoleVocabulary is the Artifact population and projection catalog
// query families resolve against. Construction reads these from the sealed
// family rather than restating a family name.
func queryRoleVocabulary() []structure.Spec {
	return vocabulary.RoleSpecs(
		"query/population/selected-point",
		"query/population/observation",
		"query/projection/summary",
		"query/projection/exact",
	)
}

// IssuedQuery is one sealed family's construction handle: the authored
// identity the row address is derived from, the Artifact population and
// projection the family is attached through, and the authority
// ProgramBinding.Query recovers the sealed implementation by.
type IssuedQuery struct {
	Family schema.Key
	// Authority is retained as the authored family lookup key for compatibility
	// with diagnostic collection. RegistrationID is the canonical owner-issued
	// identity; site and query consumers must carry it rather than rebuilding an
	// authority from Family, Population, or Projection.
	Authority       schema.Key
	RegistrationID  schema.EntryID
	Ordinal         uint32
	SelectedOrdinal uint32
	Population      schema.Key
	Projection      schema.Key
}

// QueryIssuance returns this compilation's sealed query inventory in catalog
// order.
func QueryIssuance(compilation Compilation) []IssuedQuery {
	return queryIssuance(compilation.catalog)
}

func queryIssuance(state *catalog) []IssuedQuery {
	if state == nil {
		return nil
	}
	issued := make([]IssuedQuery, 0, len(state.queries))
	selectedOrdinal := uint32(0)
	for position, registration := range state.queries {
		// Preserve the sealed inventory's cardinality even when a malformed
		// construction-only state contains a nil row. A missing row is an
		// unavailable issuance record, not permission to compact the ordinal
		// stream or let a later family occupy its slot.
		if registration == nil {
			issued = append(issued, IssuedQuery{Ordinal: uint32(position + 1)})
			continue
		}
		family := registration.Key()
		if registration.Population() == query.PopulationSelectedPoint {
			selectedOrdinal++
		}
		rowSelectedOrdinal := uint32(0)
		if registration.Population() == query.PopulationSelectedPoint {
			rowSelectedOrdinal = selectedOrdinal
		}
		issued = append(issued, IssuedQuery{
			Family:          family,
			Authority:       family,
			RegistrationID:  registration.EntryID(),
			Ordinal:         uint32(position + 1),
			SelectedOrdinal: rowSelectedOrdinal,
			Population:      registration.Population(),
			Projection:      registration.Projection(),
		})
	}
	return issued
}

// QueryResultLayout is the sealed layout one family's answers are detached
// under in this compilation. It is the one place a reader opens a published
// payload from: the layout is the seal's, so a consumer never holds a
// declaration of the wire beside the one that wrote it.
func QueryResultLayout(compilation Compilation, family schema.Key) (*plane.Sealed, bool) {
	return queryResultLayout(compilation.catalog, family)
}

func queryResultLayout(state *catalog, family schema.Key) (*plane.Sealed, bool) {
	if state == nil || !family.Available() {
		return nil, false
	}
	position, positioned := queryPositionForFamily(state, family)
	if !positioned || position < 0 || position >= len(state.queryContributors) {
		return nil, false
	}
	layout := state.queryContributors[position].queryResultPublication.layout
	return layout, layout.Available()
}
