package module

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

// CountRows returns the immutable LinkModule denominator rows frozen when the
// component was built. The rows are keyed by generated schema identities; no
// module-local token or auxiliary digest is part of this API.
func (c *Component) CountRows() (denominator.CountRows, bool) {
	if !live(c) || !c.authority.counts.Available() {
		return denominator.CountRows{}, false
	}
	return c.authority.counts, true
}

// CountRows returns the detached, sealed LinkModule denominator rows.
func (v Cold) CountRows() (denominator.CountRows, bool) {
	if v.fence == nil || !v.fence.sealed || !v.content.Available() || !v.counts.Available() {
		return denominator.CountRows{}, false
	}
	return v.counts, true
}

func buildCountRows(c *Component) (denominator.CountRows, bool) {
	if !live(c) || !c.authority.content.Available() {
		return denominator.CountRows{}, false
	}

	ids := denominator.GeneratedLinkModuleIDs()
	values := []struct {
		id    schema.EntryID
		value int
	}{
		{ids.LinkModule, len(c.authority.spec.ModuleCacheEntries)},
		{ids.LinkModuleCache, c.Cache().InstanceCount()},
		{ids.LinkModuleRepresentative, c.Cache().InstanceCount()},
		{ids.LinkModuleAnalysisRoot, c.Roots().Count()},
	}
	rows := make([]denominator.CountRow, 0, len(values))
	for _, value := range values {
		if value.value < 0 {
			return denominator.CountRows{}, false
		}
		row, ok := denominator.NewCountRow(value.id, uint64(value.value))
		if !ok {
			return denominator.CountRows{}, false
		}
		rows = append(rows, row)
	}
	return denominator.NewCountRows(rows)
}
