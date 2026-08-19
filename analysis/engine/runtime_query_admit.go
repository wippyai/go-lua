package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

// MountedQueryBatch is the query admission scope. It is handed to a batch
// queued through the topology builder and lives only while SealSources drains
// that batch, so a query row is always issued after source admission ends
// and before the graph commits.
type MountedQueryBatch struct {
	builder  *BindingTopologyBuilder
	draining bool
}

func (batch *MountedQueryBatch) openBuilder() (*BindingTopologyBuilder, bool) {
	if batch == nil || !batch.draining || batch.builder == nil {
		return nil, false
	}
	return batch.builder, true
}

// AddMountedSummaryQuery issues one summary query row anchored at a mounted
// artifact point.  The point is addressed by the mount and reusable artifact
// point ID; callers never receive equation references.
func AddMountedSummaryQuery[V, R any](batch *MountedQueryBatch, implementation *SummaryQueryImplementation[V, R], id, mount, reusable identity.ContentID) bool {
	return addMountedQuery[V, R](batch, implementation, id, mount, reusable, composition.QueryFactorSummary)
}

// AddMountedExactQuery is the exact-query counterpart of
// AddMountedSummaryQuery.
func AddMountedExactQuery[V, R any](batch *MountedQueryBatch, implementation *ExactQueryImplementation[V, R], id, mount, reusable identity.ContentID) bool {
	return addMountedQuery[V, R](batch, implementation, id, mount, reusable, composition.QueryFactorExact)
}

func (implementation *SummaryQueryImplementation[V, R]) admitMountedQuery(batch *MountedQueryBatch, id, mount, point identity.ContentID) bool {
	return AddMountedSummaryQuery(batch, implementation, id, mount, point)
}

func (implementation *SummaryQueryImplementation[V, R]) bindConstruction(construction *ProgramConstruction, id identity.ContentID) bool {
	return AttachSummaryQuery(construction, implementation, id)
}

func (implementation *ExactQueryImplementation[V, R]) admitMountedQuery(batch *MountedQueryBatch, id, mount, point identity.ContentID) bool {
	return AddMountedExactQuery(batch, implementation, id, mount, point)
}

func (implementation *ExactQueryImplementation[V, R]) bindConstruction(construction *ProgramConstruction, id identity.ContentID) bool {
	return AttachExactQuery(construction, implementation, id)
}

func addMountedQuery[V, R any](batch *MountedQueryBatch, implementation bindingQueryReceipt, id, mount, reusable identity.ContentID, kind composition.QueryProjectionKind) bool {
	builder, scoped := batch.openBuilder()
	if !scoped || builder.inner == nil || implementation == nil || !id.Available() || !mount.Available() || !reusable.Available() {
		return false
	}
	state, authority, family, ordinal, ok := implementation.boundTopologyQueryReceipt()
	if !ok || state != builder.inner.state || authority != builder.inner.authority || !family.Available() {
		return false
	}
	projection, ok := state.schema.queryProjectionShapeAt(ordinal, 0)
	if !ok || projection.Kind != kind || !projection.Factor.Available() {
		return false
	}
	inner, locked := builder.lockTopologyOpen()
	if !locked {
		return false
	}
	rows := batch.builder.mountedRows
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
		if !mappingOK || !builder.addSummary(summaryImplementation, mapping) {
			return false
		}
	}
	row, issued := builder.issueQueryRow(implementation, equation.QueryInstance{Family: family, Point: point, Surfaces: []equation.Surface{surface}})
	if !issued {
		return false
	}
	_, added := builder.addSemanticQuery(id, row)
	return added
}
