package composite

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
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
)

// queryRegistrations is the authored analyzer query inventory: the three families
// the sealed schema opens a query slot for. Each row instantiates one owning
// domain's own query declaration, which carries that family's identities, its
// subject axes, its fold contract, and the contributor that folds and freezes
// its answers.
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
		if !ok || !contributor.complete() {
			rejected = true
			return
		}
		admitted = append(admitted, entry)
		contributors = append(contributors, contributor)
	}

	add(wireQuery(valueowner.QuerySpec(), roles, valueowner.DeclareQuery, valueowner.BindQuery, valueowner.RecoverQuery, engine.NewSummaryQueryAdmission, valueowner.EncodeQueryAnswer))
	add(wireQuery(effectowner.QuerySpec(), roles, effectowner.DeclareQuery, effectowner.BindQuery, effectowner.RecoverQuery, engine.NewExactQueryAdmission, effectowner.EncodeQueryAnswer))
	add(wireQuery(placementquery.QuerySpec(), roles, placementquery.DeclareQuery, placementquery.BindQuery, placementquery.RecoverQuery, engine.NewHeterogeneousQueryAdmission, placementquery.EncodeQueryAnswer))

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
		"query/projection/summary",
		"query/projection/exact",
	)
}

// IssuedQuery is one sealed family's construction handle: the authored
// identity the row address is derived from, the Artifact population and
// projection the family is attached through, and the authority
// ProgramBinding.Query recovers the sealed implementation by.
type IssuedQuery struct {
	Family     schema.Key
	Authority  schema.Key
	Population schema.Key
	Projection schema.Key
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
	for _, registration := range state.queries {
		if registration == nil {
			continue
		}
		family := registration.Key()
		issued = append(issued, IssuedQuery{
			Family:     family,
			Authority:  family,
			Population: registration.Population(),
			Projection: registration.Projection(),
		})
	}
	return issued
}
