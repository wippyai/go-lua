package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/program/keyspace"
)

// AddMountedSummaryQuery issues one summary query row anchored at a mounted
// artifact point.  The point is addressed by the mount and reusable artifact
// point ID; callers never receive equation references.  It must be called
// after ReceiptAssembly.SealSources and before Commit.
func AddMountedSummaryQuery[V, R any](assembly *ReceiptAssembly, implementation *SummaryQueryImplementation[V, R], id, mount, reusable keyspace.ContentID) bool {
	return addMountedQuery[V, R](assembly, implementation, id, mount, reusable, composition.QueryFactorSummary)
}

// AddMountedExactQuery is the exact-query counterpart of
// AddMountedSummaryQuery.
func AddMountedExactQuery[V, R any](assembly *ReceiptAssembly, implementation *ExactQueryImplementation[V, R], id, mount, reusable keyspace.ContentID) bool {
	return addMountedQuery[V, R](assembly, implementation, id, mount, reusable, composition.QueryFactorExact)
}

func addMountedQuery[V, R any](assembly *ReceiptAssembly, implementation bindingQueryReceipt, id, mount, reusable keyspace.ContentID, kind composition.QueryProjectionKind) bool {
	if assembly == nil || assembly.builder == nil || implementation == nil || !id.Available() || !mount.Available() || !reusable.Available() {
		return false
	}
	state, authority, family, ordinal, ok := implementation.boundTopologyQueryReceipt()
	if !ok || state != assembly.builder.inner.state || authority != assembly.builder.inner.authority || !family.Available() {
		return false
	}
	projection, ok := state.schema.queryProjectionShapeAt(ordinal, 0)
	if !ok || projection.Kind != kind || !projection.Factor.Available() {
		return false
	}
	inner, locked := assembly.builder.lockTopologyOpen()
	if !locked {
		return false
	}
	rows := inner.artifact
	site, siteOK := rows.mountedSite(mount, reusable)
	pointID, pointOK := inner.semantic.pointAt[site]
	point, pointRefOK := inner.semantic.points[pointID]
	if !siteOK || !pointOK || !pointRefOK || point == 0 {
		inner.mu.Unlock()
		return false
	}
	inner.mu.Unlock()
	surface := equation.Surface{Factor: projection.Factor, Form: equation.SurfaceReadExact, Local: 1}
	if kind == composition.QueryFactorSummary {
		surface.Form = equation.SurfaceReadSummary
		surface.Semantic = projection.Normalizer
		surface.Normalizer = projection.Normalizer
		summaryImplementation, summaryOK := implementation.(*SummaryQueryImplementation[V, R])
		if !summaryOK {
			return false
		}
		mapping, mappingOK := summaryImplementation.topologySummaryMapping(surface)
		if !mappingOK || !assembly.builder.addSummary(summaryImplementation, mapping) {
			return false
		}
	}
	row, issued := assembly.builder.issueQueryRow(implementation, equation.QueryInstance{Family: family, Point: point, Surfaces: []equation.Surface{surface}})
	if !issued {
		return false
	}
	_, added := assembly.builder.addSemanticQuery(id, row)
	return added
}

// Query returns the graph-owned query receipt issued under id.
func (receipt *ReceiptGraph) Query(id keyspace.ContentID) (ReceiptQuery, bool) {
	return receipt.lookupQuery(id)
}
