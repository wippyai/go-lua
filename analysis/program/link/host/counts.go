package host

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

var errCountRows = errors.New("link/host: invalid denominator counts")

// CountRows returns the immutable Host denominator column frozen at seal.
// Host owns the five native cardinalities; the schema owns only their stable
// entry identities.
func (c *Component) CountRows() denominator.CountRows {
	if !live(c) || !c.authority.counts.Available() {
		return denominator.CountRows{}
	}
	return c.authority.counts
}

// CountRows returns the detached Host denominator column. An unavailable Cold
// does not manufacture default facts.
func (v Cold) CountRows() denominator.CountRows {
	if v.fence == nil || !v.fence.sealed || !v.content.Available() || !v.counts.Available() {
		return denominator.CountRows{}
	}
	return v.counts
}

// buildCountRows validates the owner-local rows while deriving their native
// cardinalities. The At/Mapping checks preserve sealed-row admission without
// creating detached row identities or hashes.
func (c *Component) buildCountRows() (denominator.CountRows, error) {
	if !live(c) || c.authority.boundary == nil {
		return denominator.CountRows{}, errCountRows
	}

	endpointCount := c.authority.boundary.Endpoints().Count()
	for index := 0; index < endpointCount; index++ {
		if _, ok := c.authority.boundary.Endpoints().At(index); !ok {
			return denominator.CountRows{}, errCountRows
		}
	}

	exposureCount := c.Exposures().Count()
	for index := 0; index < exposureCount; index++ {
		if _, _, _, _, _, ok := c.Exposures().At(index); !ok {
			return denominator.CountRows{}, errCountRows
		}
	}

	bootCount := c.Globals().Count()
	for index := 0; index < bootCount; index++ {
		row, ok := c.Globals().At(index)
		if !ok {
			return denominator.CountRows{}, errCountRows
		}
		if _, _, _, _, _, _, mapped := c.Globals().Mapping(row); !mapped {
			return denominator.CountRows{}, errCountRows
		}
	}

	memberCount := c.Members().Count()
	for index := 0; index < memberCount; index++ {
		if _, _, _, _, _, _, _, ok := c.Members().At(index); !ok {
			return denominator.CountRows{}, errCountRows
		}
	}

	ids := denominator.GeneratedLinkHostIDs()
	values := []struct {
		id    schema.EntryID
		count int
	}{
		{ids.LinkHost, endpointCount},
		{ids.LinkHostExposure, exposureCount},
		{ids.LinkHostBoot, bootCount},
		{ids.LinkHostMember, memberCount},
		{ids.LinkHostEndpointTarget, endpointCount},
	}
	rows := make([]denominator.CountRow, 0, len(values))
	for _, value := range values {
		if value.count < 0 {
			return denominator.CountRows{}, errCountRows
		}
		row, ok := denominator.NewCountRow(value.id, uint64(value.count))
		if !ok {
			return denominator.CountRows{}, errCountRows
		}
		rows = append(rows, row)
	}
	sealed, ok := denominator.NewCountRows(rows)
	if !ok {
		return denominator.CountRows{}, errCountRows
	}
	return sealed, nil
}
