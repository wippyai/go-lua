package link

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

var errCountRows = errors.New("link: invalid denominator counts")

// CountRows returns the Link denominator state as one immutable snapshot of
// its sealed cold owners. Program rows are read once for every authored mount
// (including repeated mounts of the same Program), Target is read once, and
// each Link child contributes its own already-sealed column. The aggregation
// is deliberately computed on demand; Link does not retain a second mutable
// denominator authority.
func (l *Link) CountRows() (denominator.CountRows, error) {
	if l == nil || !l.id.Available() || l.project == nil || l.boundary == nil || l.module == nil || l.static == nil || l.host == nil {
		return denominator.CountRows{}, errCountRows
	}

	parts := make([]denominator.CountRows, 0, 7+l.project.Mounts().Count())
	mounts := l.project.Mounts()
	for index := 0; index < mounts.Count(); index++ {
		shard, ok := mounts.At(index)
		if !ok {
			return denominator.CountRows{}, errCountRows
		}
		mounted, ok := mounts.Program(shard)
		if !ok || mounted == nil {
			return denominator.CountRows{}, errCountRows
		}
		// Program.CountRows is the sealed Program/Artifact denominator column
		// for this exact mount. SumCountRows intentionally retains duplicate
		// identities when the same reusable Program is mounted more than once.
		rows := mounted.CountRows()
		if !rows.Available() {
			return denominator.CountRows{}, errCountRows
		}
		parts = append(parts, rows)
	}

	contract, ok := l.boundary.Target()
	if !ok || contract == nil {
		return denominator.CountRows{}, errCountRows
	}
	targetRows := contract.CountRows()
	if !targetRows.Available() {
		return denominator.CountRows{}, errCountRows
	}
	parts = append(parts, targetRows)

	projectRows := l.project.CountRows()
	if !projectRows.Available() {
		return denominator.CountRows{}, errCountRows
	}
	parts = append(parts, projectRows)

	boundaryRows, ok := l.boundary.CountRows()
	if !ok || !boundaryRows.Available() {
		return denominator.CountRows{}, errCountRows
	}
	parts = append(parts, boundaryRows)

	moduleRows, ok := l.module.CountRows()
	if !ok || !moduleRows.Available() {
		return denominator.CountRows{}, errCountRows
	}
	parts = append(parts, moduleRows)

	staticRows, ok := l.static.CountRows()
	if !ok || !staticRows.Available() {
		return denominator.CountRows{}, errCountRows
	}
	parts = append(parts, staticRows)

	hostRows := l.host.CountRows()
	if !hostRows.Available() {
		return denominator.CountRows{}, errCountRows
	}
	parts = append(parts, hostRows)

	rows, ok := denominator.SumCountRows(parts...)
	if !ok || !denominator.GeneratedCountRowsComplete(rows) {
		return denominator.CountRows{}, errCountRows
	}
	return rows, nil
}
