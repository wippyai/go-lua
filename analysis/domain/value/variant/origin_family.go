package variant

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/variant/internal/discriminant"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func originFamilyOf(t typ.Type) (originFamily, bool) {
	t = unwrap.Annotated(unwrap.NormalizeNil(t))
	switch v := t.(type) {
	case *typ.Alias:
		return originFamilyOf(v.UnaliasedTarget())
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return originFamily{}, false
		}
		return originFamilyOf(v.Body)
	case *typ.Optional:
		return originFamilyOf(v.Inner)
	case *typ.Instantiated:
		expanded, ok := subst.ExpandInstantiatedChanged(v)
		if !ok {
			return originFamily{}, false
		}
		return originFamilyOf(expanded)
	case *typ.Union:
		return closedRecordUnionFamily(v)
	case *typ.Record:
		return taggedRecordFamily(v)
	default:
		return originFamily{}, false
	}
}

func closedRecordUnionFamily(u *typ.Union) (originFamily, bool) {
	if u == nil {
		return originFamily{}, false
	}
	// A flattened optional union surfaces nil as a member (nil | A | B | C). The
	// discriminated family is over the record variants only; nil presence is
	// carried by the presence axis, not variant origin. Drop nil members so a
	// nil-bearing union (e.g. a guarded optional) still yields its discriminant
	// family and can be narrowed on its tag.
	members := make([]typ.Type, 0, len(u.Members))
	for _, member := range u.Members {
		if member != nil && member.Kind() == kind.Nil {
			continue
		}
		members = append(members, member)
	}
	if len(members) < 2 {
		return originFamily{}, false
	}
	records := make([]*typ.Record, 0, len(members))
	for _, member := range members {
		rec, ok := recordOf(member)
		if !ok || rec.Open || rec.HasMapComponent() {
			return originFamily{}, false
		}
		records = append(records, rec)
	}
	detector := discriminant.NewDetector()
	if !detector.ClosedRecordSetConflicts(records) && !detector.ClosedRecordSetPresenceConflicts(records) {
		return originFamily{}, false
	}
	id := internal.FnvString("discriminant.union.family")
	for _, member := range members {
		id = internal.MixHash(id, typ.EqualityHash(member))
	}
	id = nonZeroHash(id)
	cases := make([]originCase, 0, len(members))
	for i, member := range members {
		cases = append(cases, originCase{index: i, typ: member})
	}
	family := originFamily{id: id, cases: cases}
	storeOriginFamily(family)
	return family, true
}

func taggedRecordFamily(r *typ.Record) (originFamily, bool) {
	if r == nil {
		return originFamily{}, false
	}
	detector := discriminant.NewDetector()
	var tags []struct {
		path string
		hash uint64
	}
	detector.ForEachRequiredTag(r, func(path string, hash uint64) bool {
		tags = append(tags, struct {
			path string
			hash uint64
		}{path: path, hash: hash})
		return true
	})
	if len(tags) == 0 {
		return originFamily{}, false
	}
	sort.Slice(tags, func(i, j int) bool {
		return tags[i].path < tags[j].path
	})
	familyID := internal.FnvString("discriminant.tag.family")
	caseHash := internal.FnvString("discriminant.tag.case")
	for _, tag := range tags {
		familyID = internal.MixHash(familyID, internal.FnvString(tag.path))
		caseHash = internal.MixHash(caseHash, internal.FnvString(tag.path))
		caseHash = internal.MixHash(caseHash, tag.hash)
	}
	familyID = nonZeroHash(familyID)
	caseIndex := hashCaseIndex(caseHash)
	family := originFamily{
		id: familyID,
		cases: []originCase{{
			index: caseIndex,
			typ:   r,
		}},
	}
	storeOriginFamily(family)
	return family, true
}

func recordOf(t typ.Type) (*typ.Record, bool) {
	switch v := unwrap.Annotated(unwrap.NormalizeNil(t)).(type) {
	case *typ.Alias:
		return recordOf(v.UnaliasedTarget())
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return nil, false
		}
		return recordOf(v.Body)
	case *typ.Instantiated:
		expanded, ok := subst.ExpandInstantiatedChanged(v)
		if !ok {
			return nil, false
		}
		return recordOf(expanded)
	case *typ.Record:
		return v, true
	default:
		return nil, false
	}
}

func hashCaseIndex(h uint64) int {
	maxInt := uint64(^uint(0) >> 1)
	out := int(h & maxInt)
	if out == 0 {
		return 1
	}
	return out
}

func nonZeroHash(h uint64) uint64 {
	if h == 0 {
		return 1
	}
	return h
}
