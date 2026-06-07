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
	"github.com/wippyai/go-lua/compiler/check/domain/typepath"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// Config supplies the solved carriers needed to build semantic evidence views.
type Config struct {
	Graph   *cfg.Graph
	Facts   flow.TypeFacts
	Inputs  *flow.Inputs
	Flow    api.FlowOps
	Ctx     *db.QueryContext
	TypeOps querycore.TypeOps
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

// ProjectedPathType derives a path type from per-point facts for producers that
// have not published the full PathObservationFacts surface. It keeps root
// refinement admission, member projection, length-index proofs, and condition
// projection together so consumers do not rebuild a partial path-read algebra.
func (p Projection) ProjectedPathType(point cfg.Point, path constraint.Path) (typ.Type, bool) {
	if p.cfg.Facts == nil || path.Symbol == 0 {
		return nil, false
	}
	if len(path.Segments) > 0 {
		if pathFacts, ok := p.cfg.Facts.(flow.PathFacts); ok {
			refined := pathFacts.RefinedPathAt(point, path)
			if refined.State == flow.StateResolved && !typ.IsAbsentOrUnknown(refined.Type) {
				return refined.Type, true
			}
		}
	}
	refined := p.cfg.Facts.RefinedAt(point, path.Symbol)
	if refined.State != flow.StateResolved || refined.Type == nil || typ.IsUnknown(refined.Type) {
		return nil, false
	}
	root := p.soundRootRefinement(point, path.Symbol, refined.Type)
	if root == nil {
		return nil, false
	}
	if len(path.Segments) == 0 {
		return root, true
	}
	derived := p.typeAtSegments(root, path.Segments)
	if derived == nil {
		return nil, false
	}
	derived = p.refineLengthIndex(point, path, root, derived)
	if cf, ok := p.cfg.Facts.(conditionFacts); ok {
		derived = p.refineCondition(path, root, derived, cf.ConditionAt(point))
	}
	return derived, true
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
	ir, _ := p.cfg.Facts.(flow.IndexReadbackFacts)
	aliases, _ := p.cfg.Facts.(indexWriteKeyAliasFacts)
	if !hasKeyOf && !hasNum && !hasLen && !hasIndexWrites && ir == nil {
		return nil
	}
	return factsIndexReadFlow{
		keyOf:       kf,
		numeric:     nf,
		length:      lf,
		indexWrites: iw,
		readback:    ir,
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

type conditionFacts interface {
	ConditionAt(point cfg.Point) constraint.Condition
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
	readback    flow.IndexReadbackFacts
	keyAliases  indexWriteKeyAliasFacts
	graph       *cfg.Graph
}

func (f factsIndexReadFlow) HasKeyOf(p cfg.Point, tablePath, keyPath constraint.Path) bool {
	if f.keyOf == nil {
		return false
	}
	return f.keyOf.HasKeyOf(p, tablePath, keyPath)
}

func (f factsIndexReadFlow) IndexReadback(q flow.IndexWriteReadQuery) (typ.Type, bool) {
	if f.readback != nil {
		if got, ok := f.readback.IndexReadback(q); ok {
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
	return flowpath.ArrayLenBoundKeyWithOffset(p, varName, f.graph, f.numeric, f.symbolAt)
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

func (p Projection) soundRootRefinement(point cfg.Point, sym cfg.SymbolID, refined typ.Type) typ.Type {
	declared := p.declaredSymbolType(point, sym)
	if declared == nil {
		return refined
	}
	if declared.Kind().IsPlaceholder() {
		// A gradual declaration remains the assignment contract unless flow has
		// proven a concrete guard narrowing for the current read.
		if refined.Kind().IsPlaceholder() {
			return nil
		}
		return refined
	}
	if !subtype.IsSubtype(refined, declared) {
		return nil
	}
	return refined
}

func (p Projection) declaredSymbolType(point cfg.Point, sym cfg.SymbolID) typ.Type {
	if p.cfg.Facts == nil || sym == 0 || !p.cfg.Facts.IsAnnotated(sym) {
		return nil
	}
	tv := p.cfg.Facts.DeclaredAt(point, sym)
	if tv.State != flow.StateResolved || tv.Type == nil || typ.IsUnknown(tv.Type) {
		return nil
	}
	return tv.Type
}

func (p Projection) typeAtSegments(base typ.Type, segments []constraint.Segment) typ.Type {
	return typepath.TypeAtSegments(base, segments, typepath.Options{
		Ctx:               p.cfg.Ctx,
		Ops:               p.cfg.TypeOps,
		MissingFieldAsNil: true,
	})
}

func (p Projection) refineLengthIndex(point cfg.Point, path constraint.Path, root, derived typ.Type) typ.Type {
	if len(path.Segments) == 0 {
		return derived
	}
	idx := -1
	for i := len(path.Segments) - 1; i >= 0; i-- {
		if path.Segments[i].Kind == constraint.SegmentIndexInt {
			idx = i
			break
		}
	}
	if idx < 0 {
		return derived
	}
	seg := path.Segments[idx]
	if seg.Kind != constraint.SegmentIndexInt || seg.Index < 1 {
		return derived
	}
	lengthFacts, ok := p.cfg.Facts.(flow.LengthFacts)
	if !ok {
		return derived
	}
	containerPath := constraint.Path{
		Root:     path.Root,
		Symbol:   path.Symbol,
		Version:  path.Version,
		Segments: append([]constraint.Segment(nil), path.Segments[:idx]...),
	}
	lower, ok := lengthFacts.LengthLowerBoundAt(point, path.Symbol)
	if pathFacts, hasPathFacts := p.cfg.Facts.(flow.PathLengthFacts); hasPathFacts {
		if pathLower, pathOK := pathFacts.LengthLowerBoundForPathAt(point, containerPath); pathOK {
			lower, ok = pathLower, true
		}
	}
	if !ok || lower < int64(seg.Index) {
		return derived
	}
	container := p.typeAtSegments(root, path.Segments[:idx])
	if container == nil {
		return derived
	}
	if refined := narrow.RefineSequenceIndex(container, derived, int64(seg.Index)); refined != nil {
		return refined
	}
	return derived
}

func (p Projection) refineCondition(query constraint.Path, root, derived typ.Type, cond constraint.Condition) typ.Type {
	if root == nil || derived == nil || !cond.HasConstraints() || cond.IsFalse() || cond.IsTrue() {
		return derived
	}
	var out typ.Type
	applied := false
	for i := 0; i < cond.NumDisjuncts(); i++ {
		next, ok := p.refineConditionDisjunct(query, root, cond.DisjunctConstraints(i))
		if !ok || next == nil {
			next = derived
		} else {
			applied = true
		}
		out = typ.JoinPreferNonSoft(out, next)
	}
	if !applied || out == nil {
		return derived
	}
	return typ.PruneSoftUnionMembers(out)
}

func (p Projection) refineConditionDisjunct(query constraint.Path, root typ.Type, constraints []constraint.Constraint) (typ.Type, bool) {
	if root == nil || len(constraints) == 0 {
		return nil, false
	}
	for _, c := range constraints {
		switch v := c.(type) {
		case constraint.FieldEquals:
			if next, ok := p.refineQueryByFieldEquals(query, root, v.Target, v.Field, v.Value); ok {
				return next, true
			}
		case constraint.IndexEquals:
			if lit, ok := v.Key.(*typ.Literal); ok && lit.Base == kind.Integer {
				if n, ok := lit.Value.(int64); ok {
					target := v.Target.Append(constraint.Segment{Kind: constraint.SegmentIndexInt, Index: int(n)})
					if next, ok := p.refineQueryByLiteralPath(query, root, target, v.Value); ok {
						return next, true
					}
				}
			}
		}
	}
	return nil, false
}

func (p Projection) refineQueryByFieldEquals(query constraint.Path, root typ.Type, target constraint.Path, field string, value typ.Type) (typ.Type, bool) {
	if field == "" {
		return nil, false
	}
	return p.refineQueryByFieldLiteral(query, root, target, field, value)
}

func (p Projection) refineQueryByLiteralPath(query constraint.Path, root typ.Type, literalPath constraint.Path, value typ.Type) (typ.Type, bool) {
	if len(literalPath.Segments) == 0 {
		return nil, false
	}
	fieldSeg := literalPath.Segments[len(literalPath.Segments)-1]
	if fieldSeg.Kind != constraint.SegmentField && fieldSeg.Kind != constraint.SegmentIndexString {
		return nil, false
	}
	target := constraint.Path{
		Root:     literalPath.Root,
		Symbol:   literalPath.Symbol,
		Version:  literalPath.Version,
		Segments: append([]constraint.Segment(nil), literalPath.Segments[:len(literalPath.Segments)-1]...),
	}
	return p.refineQueryByFieldLiteral(query, root, target, fieldSeg.Name, value)
}

func (p Projection) refineQueryByFieldLiteral(query constraint.Path, root typ.Type, target constraint.Path, field string, value typ.Type) (typ.Type, bool) {
	lit, ok := unwrap.Alias(value).(*typ.Literal)
	if !ok || lit == nil {
		return nil, false
	}
	if !pathHasPrefix(query, target) {
		return nil, false
	}
	targetType := p.typeAtSegments(root, target.Segments)
	if targetType == nil {
		return nil, false
	}
	refinedTarget := narrow.ByFieldLiteral(targetType, field, lit, queryResolver{})
	if refinedTarget == nil || typ.TypeEquals(refinedTarget, targetType) {
		return nil, false
	}
	suffix := query.Segments[len(target.Segments):]
	out := p.typeAtSegments(refinedTarget, suffix)
	if out == nil {
		return nil, false
	}
	return out, true
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

type queryResolver struct{}

func (queryResolver) Field(t typ.Type, name string) (typ.Type, bool) {
	return querycore.Field(t, name)
}

func (queryResolver) Index(t typ.Type, key typ.Type) (typ.Type, bool) {
	return querycore.Index(t, key)
}

func pathHasPrefix(path, prefix constraint.Path) bool {
	if path.Symbol != 0 && prefix.Symbol != 0 {
		if path.Symbol != prefix.Symbol {
			return false
		}
	} else if path.Root != prefix.Root {
		return false
	}
	if len(prefix.Segments) > len(path.Segments) {
		return false
	}
	for i := range prefix.Segments {
		if !segmentEqual(path.Segments[i], prefix.Segments[i]) {
			return false
		}
	}
	return true
}

func segmentEqual(a, b constraint.Segment) bool {
	return a.Kind == b.Kind && a.Name == b.Name && a.Index == b.Index
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
