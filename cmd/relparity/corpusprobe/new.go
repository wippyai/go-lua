package main

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis"
	relruntime "github.com/wippyai/go-lua/analysis/engine/relation/runtime"
	relsnapshot "github.com/wippyai/go-lua/analysis/engine/relation/runtime/snapshot"
	canonical "github.com/wippyai/go-lua/analysis/snapshot"
	"github.com/wippyai/go-lua/internal/relparity/corpus"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// newAnswer solves the same compiled fixture on the relation engine and reads
// its answers off the canonical projection that engine publishes through.
//
// The projection is the production publication path, not a second reader
// written for this driver: whatever the relation engine tells the rest of the
// analyzer is exactly what this comparison sees.
func newAnswer(plan *analysis.Plan, project testfixture.CorpusProject) corpus.Answer {
	mount, err := MountRelationFixture(plan, project)
	if err != nil {
		return corpus.Answer{
			Side:   corpus.SideNew,
			Status: corpus.StatusUnconstructed,
			Detail: err.Error(),
		}
	}
	if !mount.Available() {
		return corpus.Answer{
			Side:   corpus.SideNew,
			Status: corpus.StatusUnconstructed,
			Detail: "relation mount is incomplete: " + ErrConstructorUnavailable.Error(),
		}
	}
	solved, ok := relruntime.Solve(mount.Mounted, mount.Base, mount.View)
	if !ok || !solved.Available() {
		return corpus.Answer{
			Side:   corpus.SideNew,
			Status: corpus.StatusRefused,
			Detail: "solve: relation runtime refused the mounted execution",
		}
	}
	projection, published := relsnapshot.Publish(solved, mount.View)
	if !published || !projection.Available() {
		return corpus.Answer{
			Side:   corpus.SideNew,
			Status: corpus.StatusError,
			Detail: "publish: the solved root produced no canonical projection",
		}
	}
	rows, err := publishedRows(mount, projection)
	if err != nil {
		return corpus.Answer{Side: corpus.SideNew, Status: corpus.StatusError, Detail: err.Error()}
	}
	rows = append(rows, corpus.Row{
		Family:  "result",
		Site:    "status",
		Value:   "AnalyzeComplete",
		Outcome: "solved",
	}, corpus.Row{
		Family:  "result",
		Site:    "family-count",
		Value:   fmt.Sprintf("%d", familyCount(rows)),
		Outcome: "solved",
	})
	return corpus.Answer{Side: corpus.SideNew, Status: corpus.StatusSolved, Rows: rows}
}

// publishedRows reads every projected cell, addressed through the
// correspondence the mount states.
//
// A cell the mount cannot address is an error, not a silently dropped row: a
// comparison missing rows on one side would read as agreement over a smaller
// corpus than was actually solved.
func publishedRows(mount RelationMount, projection relsnapshot.Projection) ([]corpus.Row, error) {
	var rows []corpus.Row
	for _, column := range projection.Columns() {
		for _, key := range projection.Keys(column.ID) {
			family, site, addressed := mount.Address(column, key)
			if !addressed {
				return nil, fmt.Errorf("column %s row %s has no published address",
					column.PublicationID, key.Row.Content())
			}
			cell, status := projection.Read(column.ID, key)
			rows = append(rows, corpus.Row{
				Family:  family,
				Site:    site,
				Value:   cellSummary(cell, status),
				Outcome: cell.Presence.Kind().String(),
				Lineage: "lineage=" + cell.Lineage.Content().String(),
			})
		}
	}
	return rows, nil
}

// cellSummary renders one projected cell's published payload.
func cellSummary(cell relsnapshot.Cell, status canonical.ReadStatus) string {
	value := "absent"
	if cell.Value.Available() {
		value = cell.Value.Opaque().String()
	}
	return fmt.Sprintf("read=%s type=%s value=%s", status, cell.Type.Content(), value)
}

// familyCount counts the distinct families a side published, so both sides
// state their family cardinality under one address.
func familyCount(rows []corpus.Row) int {
	seen := map[string]struct{}{}
	for _, row := range rows {
		seen[row.Family] = struct{}{}
	}
	return len(seen)
}
