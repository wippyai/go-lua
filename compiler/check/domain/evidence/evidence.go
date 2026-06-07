// Package evidence owns the normalized proof surfaces exposed by solved checker
// state. Consumers ask for semantic evidence here instead of probing Facts,
// Flow, Inputs, Graph, and Bindings independently.
package evidence

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/indexread"
	flowpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// Config supplies the solved carriers needed to build semantic evidence views.
type Config struct {
	Graph  *cfg.Graph
	Facts  flow.TypeFacts
	Inputs *flow.Inputs
	Flow   api.FlowOps
}

// Projection is the canonical lens from raw solved carriers to semantic proof
// surfaces. It centralizes provider selection so domain consumers do not rebuild
// parallel Facts-vs-Flow adapters.
type Projection struct {
	cfg Config
}

// New returns a normalized evidence projection over solved checker state.
func New(cfg Config) Projection {
	return Projection{cfg: cfg}
}

// SolvedFlow returns the full solved-flow surface, when one is available.
func (p Projection) SolvedFlow() api.FlowOps {
	return p.cfg.Flow
}

// PathReadFlow returns the path-read flow used when no per-point Facts carrier is
// installed. A Facts carrier is already a point-projection, so mixing it with
// solved Flow would read from two different state views.
func (p Projection) PathReadFlow() api.FlowOps {
	if p.cfg.Facts == nil {
		return p.cfg.Flow
	}
	return nil
}

// PathObservationFacts returns the high-level path observation surface, falling
// back to the caller-supplied observer only after canonical carriers decline it.
func (p Projection) PathObservationFacts(fallback flow.PathObservationFacts) flow.PathObservationFacts {
	if p.cfg.Facts != nil {
		if facts, ok := p.cfg.Facts.(flow.PathObservationFacts); ok {
			return facts
		}
	}
	if p.cfg.Flow != nil {
		if facts, ok := p.cfg.Flow.(flow.PathObservationFacts); ok {
			return facts
		}
	}
	return fallback
}

// ProductPathObservationFacts returns the product-carrier path evidence surface.
func (p Projection) ProductPathObservationFacts() flow.ProductPathObservationFacts {
	if p.cfg.Facts != nil {
		if facts, ok := p.cfg.Facts.(flow.ProductPathObservationFacts); ok {
			return facts
		}
	}
	if p.cfg.Flow != nil {
		if facts, ok := p.cfg.Flow.(flow.ProductPathObservationFacts); ok {
			return facts
		}
	}
	return nil
}

// DirectPathCandidate projects a direct materialized path fact for a source path.
// It is a producer-side candidate only; selection against declared/proof facts is
// still owned by flow.SelectPathObservationResult.
func (p Projection) DirectPathCandidate(point cfg.Point, path constraint.Path, view flow.PathReadView) flow.PathObservationCandidate {
	if p.cfg.Facts == nil || path.Symbol == 0 || len(path.Segments) == 0 {
		return flow.PathObservationCandidate{}
	}
	var refined flow.TypedValue
	if view == flow.PathReadPost {
		if post, ok := p.cfg.Facts.(postStatePathFacts); ok {
			refined = post.PostRefinedPathAt(point, path)
		}
	}
	if refined.State != flow.StateResolved || typ.IsAbsentOrUnknown(refined.Type) {
		if pathFacts, ok := p.cfg.Facts.(flow.PathFacts); ok {
			refined = pathFacts.RefinedPathAt(point, path)
		}
	}
	if refined.State != flow.StateResolved || typ.IsAbsentOrUnknown(refined.Type) {
		return flow.PathObservationCandidate{}
	}
	return flow.PathObservationCandidate{
		Type:   refined.Type,
		Source: flow.PathObservationDirectPath,
		OK:     true,
	}
}

// AssignmentSourceFacts returns the transfer-owned RHS source evidence surface.
func (p Projection) AssignmentSourceFacts() flow.AssignmentSourceFacts {
	if p.cfg.Facts != nil {
		if facts, ok := p.cfg.Facts.(flow.AssignmentSourceFacts); ok {
			return facts
		}
	}
	if p.cfg.Flow != nil {
		if facts, ok := p.cfg.Flow.(flow.AssignmentSourceFacts); ok {
			return facts
		}
	}
	return nil
}

// IndexWriteFacts returns the solved dynamic-index write proof surface.
func (p Projection) IndexWriteFacts() flow.IndexWriteFacts {
	if p.cfg.Facts != nil {
		if facts, ok := p.cfg.Facts.(flow.IndexWriteFacts); ok {
			return facts
		}
	}
	if p.cfg.Flow != nil {
		if facts, ok := p.cfg.Flow.(flow.IndexWriteFacts); ok {
			return facts
		}
	}
	return nil
}

// ConditionProofFacts returns condition-only type proof evidence.
func (p Projection) ConditionProofFacts() flow.ConditionProofFacts {
	if p.cfg.Facts != nil {
		if facts, ok := p.cfg.Facts.(flow.ConditionProofFacts); ok {
			return facts
		}
	}
	if p.cfg.Flow != nil {
		if facts, ok := p.cfg.Flow.(flow.ConditionProofFacts); ok {
			return facts
		}
	}
	return nil
}

// ConstFacts returns immutable constant facts; flow inputs are the canonical
// fallback because constants are captured before a solved flow may exist.
func (p Projection) ConstFacts() flow.ConstFacts {
	if p.cfg.Facts != nil {
		if facts, ok := p.cfg.Facts.(flow.ConstFacts); ok {
			return facts
		}
	}
	if p.cfg.Flow != nil {
		if facts, ok := p.cfg.Flow.(flow.ConstFacts); ok {
			return facts
		}
	}
	if p.cfg.Inputs != nil {
		return p.cfg.Inputs
	}
	return nil
}

// IndexReadFlow returns the solved proof surface needed by indexread refiners.
func (p Projection) IndexReadFlow() indexread.Flow {
	if flowOps := p.PathReadFlow(); flowOps != nil {
		return flowOps
	}
	kf, hasKeyOf := p.cfg.Facts.(keyOfFacts)
	nf, hasNum := p.cfg.Facts.(flow.NumericFacts)
	lf, hasLen := p.cfg.Facts.(flow.LengthFacts)
	iw, hasIndexWrites := p.cfg.Facts.(flow.IndexWriteFacts)
	mr, _ := p.cfg.Facts.(indexread.MapReadbackFlow)
	aliases, _ := p.cfg.Facts.(indexWriteKeyAliasFacts)
	if !hasKeyOf && !hasNum && !hasLen && !hasIndexWrites && mr == nil {
		return nil
	}
	return factsIndexReadFlow{
		keyOf:       kf,
		numeric:     nf,
		length:      lf,
		indexWrites: iw,
		mapReadback: mr,
		keyAliases:  aliases,
		graph:       p.cfg.Graph,
	}
}

type postStatePathFacts interface {
	PostRefinedPathAt(point cfg.Point, path constraint.Path) flow.TypedValue
}

type keyOfFacts interface {
	HasKeyOf(p cfg.Point, tablePath, keyPath constraint.Path) bool
}

type indexWriteKeyAliasFacts interface {
	IndexWriteKeyAliasesAt(p cfg.Point, key flow.StableAddress) []flow.StableAddress
}

type factsIndexReadFlow struct {
	keyOf       keyOfFacts
	numeric     flow.NumericFacts
	length      flow.LengthFacts
	indexWrites flow.IndexWriteFacts
	mapReadback indexread.MapReadbackFlow
	keyAliases  indexWriteKeyAliasFacts
	graph       *cfg.Graph
}

func (f factsIndexReadFlow) HasKeyOf(p cfg.Point, tablePath, keyPath constraint.Path) bool {
	if f.keyOf == nil {
		return false
	}
	return f.keyOf.HasKeyOf(p, tablePath, keyPath)
}

func (f factsIndexReadFlow) MapReadback(q flow.IndexWriteReadQuery) (typ.Type, bool) {
	if f.mapReadback != nil {
		if got, ok := f.mapReadback.MapReadback(q); ok {
			return got, true
		}
	}
	return f.IndexWriteAdmission(q)
}

func (f factsIndexReadFlow) IndexWriteAdmission(q flow.IndexWriteReadQuery) (typ.Type, bool) {
	if f.indexWrites == nil {
		return nil, false
	}
	var aliases flow.IndexWriteKeyAliases
	if f.keyAliases != nil {
		aliases = f.keyAliases.IndexWriteKeyAliasesAt
	}
	return flow.IndexWriteAdmissionWithKeyAliases(
		q,
		f.indexWrites.IndexWriteAdmission,
		aliases,
	)
}

func (f factsIndexReadFlow) BoundsAt(p cfg.Point, name string) (int64, int64, bool) {
	if f.numeric == nil {
		return 0, 0, false
	}
	sym, ok := f.symbolAt(p, name)
	if !ok {
		return 0, 0, false
	}
	return f.numeric.NumericBoundsAt(p, sym)
}

func (f factsIndexReadFlow) ArrayLenBoundWithOffsetAt(p cfg.Point, varName string) (string, int64, bool) {
	if f.numeric == nil {
		return "", 0, false
	}
	sym, ok := f.symbolAt(p, varName)
	if !ok {
		return "", 0, false
	}
	arrSym, offset, ok := f.numeric.ArrayLenRefAt(p, sym)
	if !ok {
		return "", 0, false
	}
	arrPath := flowpath.WithVersion(constraint.Path{Symbol: arrSym}, f.graph, p)
	return string(arrPath.Key()), offset, true
}

func (f factsIndexReadFlow) LengthBoundsAt(p cfg.Point, path constraint.Path) (int64, int64, bool) {
	if f.length == nil || path.Symbol == 0 {
		return 0, 0, false
	}
	if pathFacts, ok := f.length.(flow.PathLengthFacts); ok {
		if lower, ok := pathFacts.LengthLowerBoundForPathAt(p, path); ok {
			return lower, 0, true
		}
	}
	if len(path.Segments) != 0 {
		return 0, 0, false
	}
	lower, ok := f.length.LengthLowerBoundAt(p, path.Symbol)
	if !ok {
		return 0, 0, false
	}
	return lower, 0, true
}

func (f factsIndexReadFlow) symbolAt(p cfg.Point, name string) (cfg.SymbolID, bool) {
	if f.graph == nil || name == "" {
		return 0, false
	}
	sym, ok := f.graph.SymbolAt(p, name)
	if !ok || sym == 0 {
		return 0, false
	}
	return sym, true
}
