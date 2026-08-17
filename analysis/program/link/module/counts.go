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

	outcomes := 0
	for index := 0; index < c.Generations().Count(); index++ {
		generation, ok := c.Generations().At(index)
		if !ok || !addCount(&outcomes, c.Outcomes().Count(generation)) {
			return denominator.CountRows{}, false
		}
	}

	ids := denominator.GeneratedLinkModuleIDs()
	values := []struct {
		id    schema.EntryID
		value int
	}{
		{ids.LinkModule, c.Cache().EntryCount()},
		{ids.LinkModuleCache, c.Cache().InstanceCount()},
		{ids.LinkModuleRepresentative, c.Cache().InstanceCount()},
		{ids.LinkModuleTransport, c.Coordinates().Count()},
		{ids.LinkModuleAnalysisRoot, c.Roots().Count()},
		{ids.LinkModuleInitGeneration, c.Generations().Count()},
		{ids.LinkModuleInitOutcome, outcomes},
		{ids.LinkModuleInitTerminal, c.Terminals().Count()},
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

func addCount(total *int, value int) bool {
	if total == nil || value < 0 {
		return false
	}
	sum, ok := denominator.SumInts(*total, value)
	if !ok {
		return false
	}
	*total = sum
	return true
}
