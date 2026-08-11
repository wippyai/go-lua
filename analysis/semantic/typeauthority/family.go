package typeauthority

import (
	"github.com/wippyai/go-lua/analysis/type/discriminant"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type familyResult struct {
	arms []Selector
	ok   bool
}

// familyArms accepts only a direct sealed union whose non-nil arms are closed
// record values. Nil is presence, not a variant case. The family identity is
// still the Program union root; no structural discovery catalog is created.
// A later Rule may derive discriminant evidence from these exact arm selectors.
func (a *Authority) familyArms(root Selector) ([]Selector, bool) {
	if a == nil {
		return nil, false
	}
	a.familyMu.Lock()
	defer a.familyMu.Unlock()
	if cached, ok := a.families[root]; ok {
		return cached.arms, cached.ok
	}
	arms, ok := a.familyArmsUncached(root)
	if a.families == nil {
		a.families = make(map[Selector]familyResult)
	}
	a.families[root] = familyResult{arms: arms, ok: ok}
	return arms, ok
}

func (a *Authority) familyArmsUncached(root Selector) ([]Selector, bool) {
	entry, ok := a.entry(root)
	if !ok {
		return nil, false
	}
	rootTerm := entry.ref.Root()
	count, ok := entry.program.Static().Types().Unions().MemberCount(rootTerm)
	if !ok || count < 2 {
		return nil, false
	}
	arms := make([]Selector, 0, count)
	records := make([]*typ.Record, 0, count)
	for index := 0; index < count; index++ {
		term, ok := entry.program.Static().Types().Unions().MemberAt(rootTerm, index)
		if !ok {
			return nil, false
		}
		selector, ok := a.lookupProgramTerm(entry.program, term)
		if !ok {
			return nil, false
		}
		value, ok := a.Materialize(selector)
		if !ok {
			return nil, false
		}
		if isNilArm(value) {
			continue
		}
		record, ok := closedRecordArm(value)
		if !ok {
			return nil, false
		}
		for _, prior := range records {
			// A direct Program union preserves authored arm occurrence, but
			// semantically equal arms cannot become distinct runtime cases.
			// Reject rather than silently picking an ordinal or creating a
			// second case vocabulary.
			if typ.TypeEquals(record, prior) {
				return nil, false
			}
		}
		arms = append(arms, selector)
		records = append(records, record)
	}
	if len(arms) < 2 {
		return nil, false
	}
	proof := discriminant.NewDetector()
	for left := 0; left < len(records); left++ {
		for right := left + 1; right < len(records); right++ {
			// Ordinal cases form a partition only when every pair has a
			// canonical separating proof. A single separated pair is not
			// enough: an unseparated third arm would make narrowing ambiguous.
			if !proof.RecordsConflict(records[left], records[right]) &&
				!proof.RecordsPresenceConflict(records[left], records[right]) {
				return nil, false
			}
		}
	}
	return arms, true
}

func isNilArm(value typ.Type) bool {
	for {
		switch typed := value.(type) {
		case *typ.Alias:
			value = typed.UnaliasedTarget()
			continue
		case *typ.Instantiated:
			expanded, ok := subst.ExpandInstantiatedChanged(typed)
			if !ok {
				return false
			}
			value = expanded
			continue
		}
		return value != nil && value.Kind() == kind.Nil
	}
}

func closedRecordArm(value typ.Type) (*typ.Record, bool) {
	for {
		switch typed := value.(type) {
		case *typ.Alias:
			value = typed.UnaliasedTarget()
			continue
		case *typ.Instantiated:
			expanded, ok := subst.ExpandInstantiatedChanged(typed)
			if !ok {
				return nil, false
			}
			value = expanded
			continue
		case *typ.Record:
			return typed, !typed.Open && !typed.HasMapComponent()
		default:
			return nil, false
		}
	}
}
