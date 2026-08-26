package main

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis"
	"github.com/wippyai/go-lua/analysis/result"
	"github.com/wippyai/go-lua/internal/relparity/corpus"
)

// oldAnswer solves the compiled fixture on the engine the oracle answers with
// today and publishes one row per family per query site.
//
// The address is the one the old engine already issues: the family key names
// the family, the site identity names the query site. The relation engine is
// asked to answer under that same address, which is what the driver's seam
// carries; nothing here reshapes an answer to make the two look alike.
func oldAnswer(ctx context.Context, plan *analysis.Plan) corpus.Answer {
	solved, status := plan.Solve(ctx)
	if solved == nil {
		return corpus.Answer{
			Side:   corpus.SideOld,
			Status: corpus.StatusRefused,
			Detail: "solve: " + analyzeStatusSpelling(status),
		}
	}
	rows := []corpus.Row{{
		Family:  "result",
		Site:    "status",
		Value:   analyzeStatusSpelling(status),
		Outcome: "solved",
	}, {
		Family:  "result",
		Site:    "family-count",
		Value:   fmt.Sprintf("%d", solved.FamilyCount()),
		Outcome: "solved",
	}}
	for familyIndex := 0; familyIndex < solved.FamilyCount(); familyIndex++ {
		family, ok := solved.FamilyAt(familyIndex)
		if !ok {
			rows = append(rows, corpus.Row{
				Family:  fmt.Sprintf("family/%d", familyIndex),
				Site:    "family",
				Outcome: "unavailable",
			})
			continue
		}
		key := string(family.Key())
		rows = append(rows, corpus.Row{
			Family:  key,
			Site:    "family",
			Value:   fmt.Sprintf("queries=%d", family.QueryCount()),
			Outcome: "declared",
			Lineage: family.ContractID().String(),
		})
		for queryIndex := 0; queryIndex < family.QueryCount(); queryIndex++ {
			rows = append(rows, queryRow(family, key, queryIndex))
		}
	}
	return corpus.Answer{
		Side:   corpus.SideOld,
		Status: corpus.StatusSolved,
		Rows:   rows,
	}
}

// queryRow publishes one query site's whole answer: its publication state,
// the summary of the cell it published, and the provenance that cell was
// concluded under.
func queryRow(family result.Family, familyKey string, queryIndex int) corpus.Row {
	query, ok := family.QueryAt(queryIndex)
	if !ok {
		return corpus.Row{
			Family:  familyKey,
			Site:    fmt.Sprintf("query/%d", queryIndex),
			Outcome: "unavailable",
		}
	}
	site := fmt.Sprintf("query/%d", queryIndex)
	if id, held := query.SiteID(); held {
		site = "site/" + id.String()
	}
	row := corpus.Row{
		Family:  familyKey,
		Site:    site,
		Outcome: queryStatusSpelling(query.Status()),
		Lineage: lineageOf(query),
	}
	cell, held := query.Cell()
	if !held {
		row.Value = "cell=unavailable"
		return row
	}
	row.Value = fmt.Sprintf("present=%t rows=%d content=%s contract=%s",
		cell.Present(), cell.RowCount(), cell.ContentID(), cell.ContractID())
	return row
}

// lineageOf renders the identities a published answer was concluded under.
func lineageOf(query result.Query) string {
	lineage := ""
	appendPart := func(name string, id fmt.Stringer, held bool) {
		if !held {
			return
		}
		if lineage != "" {
			lineage += " "
		}
		lineage += name + "=" + id.String()
	}
	point, pointHeld := query.PointID()
	appendPart("point", point, pointHeld)
	mount, mountHeld := query.MountID()
	appendPart("mount", mount, mountHeld)
	context, contextHeld := query.ContextID()
	appendPart("context", context, contextHeld)
	publication, publicationHeld := query.PublicationKey()
	appendPart("publication", publication, publicationHeld)
	return lineage
}

// queryStatusSpelling renders the closed publication vocabulary of a query on
// the wire. The spelling is the protocol's, so a status the driver reads means
// the same thing whichever side wrote it.
func queryStatusSpelling(status result.QueryStatus) string {
	switch status {
	case result.QueryHit:
		return "QueryHit"
	case result.QueryProvenAbsent:
		return "QueryProvenAbsent"
	default:
		return "QueryInvalid"
	}
}

// analyzeStatusSpelling renders the closed solve vocabulary on the wire.
func analyzeStatusSpelling(status analysis.AnalyzeStatus) string {
	switch status {
	case analysis.AnalyzeComplete:
		return "AnalyzeComplete"
	case analysis.AnalyzeIncomplete:
		return "AnalyzeIncomplete"
	case analysis.AnalyzeUnsupported:
		return "AnalyzeUnsupported"
	default:
		return "AnalyzeInvalid"
	}
}

// compileStatusSpelling renders the closed compile vocabulary on the wire.
func compileStatusSpelling(status analysis.CompileStatus) string {
	switch status {
	case analysis.CompileComplete:
		return "CompileComplete"
	case analysis.CompileUnsupported:
		return "CompileUnsupported"
	default:
		return "CompileInvalid"
	}
}
