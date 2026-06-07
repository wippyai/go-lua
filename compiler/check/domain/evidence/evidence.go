// Package evidence owns the normalized proof surfaces exposed by solved checker
// state. Consumers ask for semantic evidence here instead of probing Facts,
// Flow, Inputs, Graph, and Bindings independently.
package evidence

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/indexread"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
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
	if facts, ok := firstProvider[flow.PathObservationFacts](p.cfg.Facts, p.cfg.Flow); ok {
		return facts
	}
	return fallback
}

// ProductPathObservationFacts returns the product-carrier path evidence surface.
func (p Projection) ProductPathObservationFacts() flow.ProductPathObservationFacts {
	if facts, ok := firstProvider[flow.ProductPathObservationFacts](p.cfg.Facts, p.cfg.Flow); ok {
		return facts
	}
	return nil
}

// EffectiveTypeAt returns a symbol's effective type from the point facts. When
// preferPost is set, post-state facts may answer before the normal point view.
func (p Projection) EffectiveTypeAt(point cfg.Point, sym cfg.SymbolID, preferPost bool) flow.TypedValue {
	if p.cfg.Facts == nil || sym == 0 {
		return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
	}
	if preferPost {
		if post, ok := p.cfg.Facts.(postStateTypeFacts); ok {
			tv := post.PostEffectiveTypeAt(point, sym)
			if tv.Type != nil && (tv.State == flow.StateResolved || !typ.IsUnknown(tv.Type)) {
				return tv
			}
		}
	}
	return p.cfg.Facts.EffectiveTypeAt(point, sym)
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
	if facts, ok := firstProvider[flow.AssignmentSourceFacts](p.cfg.Facts, p.cfg.Flow); ok {
		return facts
	}
	return nil
}

// IndexWriteFacts returns the solved dynamic-index write proof surface.
func (p Projection) IndexWriteFacts() flow.IndexWriteFacts {
	if facts, ok := firstProvider[flow.IndexWriteFacts](p.cfg.Facts, p.cfg.Flow); ok {
		return facts
	}
	return nil
}

// ConditionProofFacts returns condition-only type proof evidence.
func (p Projection) ConditionProofFacts() flow.ConditionProofFacts {
	if facts, ok := firstProvider[flow.ConditionProofFacts](p.cfg.Facts, p.cfg.Flow); ok {
		return facts
	}
	return nil
}

// ConstFacts returns immutable constant facts; flow inputs are the canonical
// fallback because constants are captured before a solved flow may exist.
func (p Projection) ConstFacts() flow.ConstFacts {
	if facts, ok := firstProvider[flow.ConstFacts](p.cfg.Facts, p.cfg.Flow); ok {
		return facts
	}
	if p.cfg.Inputs != nil {
		return p.cfg.Inputs
	}
	return nil
}

// BodyContracts returns body-level parameter contracts published by the current
// facts carrier.
func (p Projection) BodyContracts() paramevidence.Contracts {
	if facts, ok := factsProvider[bodyContractFacts](p.cfg.Facts); ok {
		return facts.BodyContracts()
	}
	return nil
}

// ProvenanceRoutesAt returns source-route proofs for a materialized path.
func (p Projection) ProvenanceRoutesAt(point cfg.Point, path constraint.Path) []flow.ProvenanceRoute {
	if facts, ok := firstProvider[provenanceRouteFacts](p.cfg.Facts, p.cfg.Flow); ok {
		return facts.ProvenanceRoutesAt(point, path)
	}
	return nil
}

// AppendElementFieldSourceRoutesAt returns append-origin routes for array
// element fields.
func (p Projection) AppendElementFieldSourceRoutesAt(point cfg.Point, q flow.AppendElementFieldRouteQuery) []flow.ProvenanceRoute {
	if facts, ok := firstProvider[appendElementFieldRouteFacts](p.cfg.Facts, p.cfg.Flow); ok {
		return facts.AppendElementFieldSourceRoutesAt(point, q)
	}
	return nil
}

// CallReturnTypesAt returns call-result evidence published by the point facts.
func (p Projection) CallReturnTypesAt(point cfg.Point, call *ast.FuncCallExpr, expected typ.Type) ([]typ.Type, bool) {
	if facts, ok := factsProvider[callReturnFacts](p.cfg.Facts); ok {
		return facts.CallReturnTypesAt(point, call, expected)
	}
	return nil, false
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

type postStateTypeFacts interface {
	PostEffectiveTypeAt(point cfg.Point, sym cfg.SymbolID) flow.TypedValue
}

type postStatePathFacts interface {
	PostRefinedPathAt(point cfg.Point, path constraint.Path) flow.TypedValue
}

type bodyContractFacts interface {
	BodyContracts() paramevidence.Contracts
}

type provenanceRouteFacts interface {
	ProvenanceRoutesAt(p cfg.Point, path constraint.Path) []flow.ProvenanceRoute
}

type appendElementFieldRouteFacts interface {
	AppendElementFieldSourceRoutesAt(p cfg.Point, q flow.AppendElementFieldRouteQuery) []flow.ProvenanceRoute
}

type callReturnFacts interface {
	CallReturnTypesAt(point cfg.Point, call *ast.FuncCallExpr, expected typ.Type) ([]typ.Type, bool)
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

func firstProvider[T any](facts flow.TypeFacts, flowOps api.FlowOps) (T, bool) {
	if provider, ok := factsProvider[T](facts); ok {
		return provider, true
	}
	if flowOps != nil {
		if provider, ok := any(flowOps).(T); ok {
			return provider, true
		}
	}
	var zero T
	return zero, false
}

func factsProvider[T any](facts flow.TypeFacts) (T, bool) {
	if facts != nil {
		if provider, ok := any(facts).(T); ok {
			return provider, true
		}
	}
	var zero T
	return zero, false
}
