package composite

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// effectAxis is the coordinate space the effect query family reads. Like
// valueAxis it is the axis surface's own authored key, resolved by the
// declaration root when the query surface is sealed.
const effectAxis = schema.Key("effect")

// The registered query families. A family's authored key is its one spelling
// in the analyzer and is the key its declaration row is identified by.
const (
	QueryFamilyValueSummary schema.Key = "value-summary"
	QueryFamilyEffectExact  schema.Key = "effect-exact"
)

// queryRegistrationSpecs is the authored analyzer query inventory: the two
// families the sealed schema opens a query slot for, declared here as data.
//
// A family's codec identity is the freezer identity its results are published
// under, taken from the closed semantic vocabulary rather than minted here, so
// the declaration and the schema slot name one contract and cannot drift. The
// fold contract is the obligation the declared fold rests on: the value
// summary composes because the summary fold is coordinatewise, and the effect
// family declares no split at all, so its obligation is the exact read it is
// answered by.
func queryRegistrationSpecs(roles vocabulary.Roles) ([]query.RegistrationSpec, bool) {
	valueCodec, valueCodecOK := roles.Key("semantic/query-result/value-summary")
	valueFold, valueFoldOK := roles.Key("semantic/factor/value/summary-coordinatewise")
	effectCodec, effectCodecOK := roles.Key("semantic/query-result/effect-exact")
	effectQuery, effectQueryOK := roles.Key("semantic/query/effect-exact")
	if !valueCodecOK || !valueFoldOK || !effectCodecOK || !effectQueryOK {
		return nil, false
	}
	return []query.RegistrationSpec{
		{
			Family:   QueryFamilyValueSummary,
			Codec:    identity.ContentID(valueCodec.Digest()),
			Fold:     query.FoldDistributive,
			Contract: identity.ContentID(valueFold.Digest()),
			Subjects: []schema.Key{valueAxis},
		},
		{
			Family:   QueryFamilyEffectExact,
			Codec:    identity.ContentID(effectCodec.Digest()),
			Fold:     query.FoldGeneral,
			Contract: identity.ContentID(effectQuery.Digest()),
			Subjects: []schema.Key{effectAxis},
		},
	}, true
}

// queryRegistrations admits the authored inventory. A rejected row leaves the
// table unavailable rather than half declared.
func queryRegistrations(roles vocabulary.Roles) ([]*query.Registration, bool) {
	specs, specsOK := queryRegistrationSpecs(roles)
	if !specsOK {
		return nil, false
	}
	registrations := make([]*query.Registration, 0, len(specs))
	for _, spec := range specs {
		registration, ok := query.NewRegistration(spec)
		if !ok {
			return nil, false
		}
		registrations = append(registrations, registration)
	}
	return registrations, true
}
