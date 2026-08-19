package channelselect

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/type/typ"
)

// CaseSet is the accepted channel-select case facts for one solve. Membership
// is by CaseFactID only; result type shape cannot admit a row.
type CaseSet struct {
	rows map[identity.ContentID]CaseFact
}

// Admit records one accepted receive arm. A second row under the same
// site and ordinal is refused.
func (set *CaseSet) Admit(fact CaseFact) bool {
	if set == nil || !CaseFactAvailable(fact) || fact.Channel == nil || fact.Payload == nil {
		return false
	}
	id, idOK := CaseFactID(fact)
	if !idOK {
		return false
	}
	if set.rows == nil {
		set.rows = make(map[identity.ContentID]CaseFact)
	}
	if _, duplicate := set.rows[id]; duplicate {
		return false
	}
	set.rows[id] = CaseFact{Site: fact.Site, Ordinal: fact.Ordinal, Channel: fact.Channel, Payload: fact.Payload}
	return true
}

// AdmitCases records the receive arms of one select site. Default sentinels
// are skipped. A duplicate ordinal refuses the whole batch and leaves set
// unchanged.
func AdmitCases(set *CaseSet, site identity.ContentID, cases []ResultCase) bool {
	if set == nil || !site.Available() {
		return false
	}
	var batch CaseSet
	for _, arm := range cases {
		if arm.Index == DefaultCaseIndex {
			continue
		}
		if !batch.Admit(CaseFact{Site: site, Ordinal: arm.Index, Channel: arm.Channel, Payload: arm.Payload}) {
			return false
		}
	}
	for _, fact := range batch.rows {
		id, idOK := CaseFactID(fact)
		if !idOK {
			return false
		}
		if set.rows != nil {
			if _, exists := set.rows[id]; exists {
				return false
			}
		}
	}
	for _, fact := range batch.rows {
		if !set.Admit(fact) {
			return false
		}
	}
	return true
}

// SelectFromCases builds the honest select result union and the accepted
// CaseSet for one parent-issued select site.
func SelectFromCases(site identity.ContentID, cases []ResultCase, hasDefault bool) (typ.Type, CaseSet, bool) {
	var facts CaseSet
	if !AdmitCases(&facts, site, cases) {
		return nil, CaseSet{}, false
	}
	result, ok := ResultValueTypeWithDefault(cases, hasDefault)
	if !ok {
		return nil, CaseSet{}, false
	}
	return result, facts, true
}

// Lookup returns the accepted arm at site and ordinal.
func (set CaseSet) Lookup(site identity.ContentID, ordinal int) (CaseFact, bool) {
	id, idOK := CaseFactID(CaseFact{Site: site, Ordinal: ordinal})
	if !idOK || set.rows == nil {
		return CaseFact{}, false
	}
	fact, ok := set.rows[id]
	return fact, ok
}

// All returns every accepted fact, ordered by site then ordinal.
func (set CaseSet) All() []CaseFact {
	if set.rows == nil {
		return nil
	}
	facts := make([]CaseFact, 0, len(set.rows))
	for _, fact := range set.rows {
		facts = append(facts, fact)
	}
	for i := 0; i < len(facts); i++ {
		for j := i + 1; j < len(facts); j++ {
			if factOrder(facts[j], facts[i]) {
				facts[i], facts[j] = facts[j], facts[i]
			}
		}
	}
	return facts
}

func factOrder(left, right CaseFact) bool {
	if left.Site != right.Site {
		return string(left.Site[:]) < string(right.Site[:])
	}
	return left.Ordinal < right.Ordinal
}

// Arms returns the accepted facts for one select site, in ordinal order.
func (set CaseSet) Arms(site identity.ContentID) []CaseFact {
	if !site.Available() || set.rows == nil {
		return nil
	}
	var arms []CaseFact
	for _, fact := range set.rows {
		if fact.Site == site {
			arms = append(arms, fact)
		}
	}
	for i := 0; i < len(arms); i++ {
		for j := i + 1; j < len(arms); j++ {
			if arms[j].Ordinal < arms[i].Ordinal {
				arms[i], arms[j] = arms[j], arms[i]
			}
		}
	}
	return arms
}

// MissingArms are the accepted receive ordinals of site that handled does not
// name. A default arm covers the remainder.
func (set CaseSet) MissingArms(site identity.ContentID, handled []int, hasDefault bool) []CaseFact {
	if hasDefault {
		return nil
	}
	seen := make(map[int]struct{}, len(handled))
	for _, ordinal := range handled {
		seen[ordinal] = struct{}{}
	}
	var missing []CaseFact
	for _, fact := range set.Arms(site) {
		if _, covered := seen[fact.Ordinal]; !covered {
			missing = append(missing, fact)
		}
	}
	return missing
}

// ResultWithoutFact removes the public member named by an accepted fact.
func ResultWithoutFact(resultType typ.Type, fact CaseFact) (typ.Type, bool) {
	if !CaseFactAvailable(fact) || fact.Channel == nil || fact.Payload == nil {
		return nil, false
	}
	return ResultWithoutCase(resultType, ResultCaseType(fact.Channel, fact.Payload))
}
