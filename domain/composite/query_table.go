package composite

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// The registered query families. A family's authored key is its one spelling
// in the analyzer and is the key its declaration row is identified by. The
// spelling is the owning domain's; these are the constants a consumer opens a
// result slot with, and the table's own law holds the two to one name.
const (
	QueryFamilyValueSummary schema.Key = "value-summary"
	QueryFamilyEffectExact  schema.Key = "effect-exact"
)

// queryRegistrations is the authored analyzer query inventory: the two families
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
func queryRegistrations(roles vocabulary.Roles) ([]*query.Registration, bool) {
	var admitted []*query.Registration
	rejected := false
	add := func(entry *query.Registration, ok bool) {
		if !ok {
			rejected = true
			return
		}
		admitted = append(admitted, entry)
	}

	add(query.New(valueowner.QueryEntry(), roles))
	add(query.New(effectowner.QueryEntry(), roles))

	if rejected {
		return nil, false
	}
	return admitted, true
}
