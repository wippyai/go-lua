package project

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

var errCountRows = errors.New("link/project: invalid denominator counts")

// buildCountRows seals the two native Project cardinalities against the
// generated schema identities. The owner computes these values from its own
// canonical rows; no local token, row digest, or cross-owner projection is
// involved.
func buildCountRows(mounts []mountRow, baseApplications []uint32) (denominator.CountRows, error) {
	ids := denominator.GeneratedLinkProjectIDs()
	mountCount, ok := denominator.NewCountRow(ids.LinkProjectShardMount, uint64(len(mounts)))
	if !ok {
		return denominator.CountRows{}, errCountRows
	}
	baseCount, ok := denominator.NewCountRow(ids.LinkProjectBaseApplication, uint64(len(baseApplications)))
	if !ok {
		return denominator.CountRows{}, errCountRows
	}
	rows, ok := denominator.NewCountRows([]denominator.CountRow{mountCount, baseCount})
	if !ok {
		return denominator.CountRows{}, errCountRows
	}
	return rows, nil
}

// CountRows returns the immutable Project denominator state frozen at Build.
func (c *Component) CountRows() denominator.CountRows {
	if c == nil || c.authority == nil || !c.authority.counts.Available() {
		return denominator.CountRows{}
	}
	return c.authority.counts
}

// CountRows returns the detached Project denominator state. An unavailable
// Cold returns an unavailable set rather than manufacturing default facts.
func (v Cold) CountRows() denominator.CountRows {
	if !v.live() || !v.counts.Available() {
		return denominator.CountRows{}
	}
	return v.counts
}
