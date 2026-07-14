package transformer

import (
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// paramPreservationLedger is a row-local must proof. A set bit means that no
// operation observed on this control-flow alternative has reassigned, aliased,
// exposed, or mutated the corresponding boundary parameter. Bits only clear,
// so replaying a cyclic transfer is idempotent and joins remain transactional.
//
// This is intentionally not an allow-by-absence mechanism. Every operation
// and extension family passes through the closed policy below; a new family
// defaults to clearing the complete proof until it gets an explicit review.
type paramPreservationLedger struct {
	tracked        bool
	boundaryParams uint32
	words          []uint64
}

func newParamPreservationLedger(params uint32) paramPreservationLedger {
	return newBoundaryPreservationLedger(params, 0)
}

func newBoundaryPreservationLedger(params, captures uint32) paramPreservationLedger {
	count := params + captures
	if count == 0 {
		return paramPreservationLedger{tracked: true, boundaryParams: params}
	}
	words := make([]uint64, (count+63)/64)
	for i := range words {
		words[i] = ^uint64(0)
	}
	if remainder := count % 64; remainder != 0 {
		words[len(words)-1] = uint64(1)<<remainder - 1
	}
	return paramPreservationLedger{tracked: true, boundaryParams: params, words: words}
}

func (l paramPreservationLedger) clone() paramPreservationLedger {
	return paramPreservationLedger{tracked: l.tracked, boundaryParams: l.boundaryParams, words: append([]uint64(nil), l.words...)}
}

func (l paramPreservationLedger) valid(params uint32) bool {
	if !l.tracked {
		return len(l.words) == 0
	}
	want := int((params + 63) / 64)
	if len(l.words) != want {
		return false
	}
	if want == 0 || params%64 == 0 {
		return true
	}
	return l.words[want-1]>>(params%64) == 0
}

func (l paramPreservationLedger) equal(other paramPreservationLedger) bool {
	if l.tracked != other.tracked || l.boundaryParams != other.boundaryParams || len(l.words) != len(other.words) {
		return false
	}
	for i := range l.words {
		if l.words[i] != other.words[i] {
			return false
		}
	}
	return true
}

func (l *paramPreservationLedger) invalidateAll() {
	if l == nil {
		return
	}
	for i := range l.words {
		l.words[i] = 0
	}
}

func (l *paramPreservationLedger) invalidate(index uint32) {
	if l == nil || int(index/64) >= len(l.words) {
		return
	}
	l.words[index/64] &^= uint64(1) << (index % 64)
}

func (l *paramPreservationLedger) retainOnly(index uint32) {
	if l == nil {
		return
	}
	retained := l.preserves(index)
	l.invalidateAll()
	if retained && int(index/64) < len(l.words) {
		l.words[index/64] |= uint64(1) << (index % 64)
	}
}

func (l paramPreservationLedger) preserves(index uint32) bool {
	return int(index/64) < len(l.words) && l.words[index/64]&(uint64(1)<<(index%64)) != 0
}

// observeFact is the exhaustiveness seam for operation-plan growth. Pure
// reads and control-flow evidence preserve the ledger. Calls, dynamic access,
// writes, and every unreviewed family reject preservation fail-closed.
func (l *paramPreservationLedger) observeFact(ctx planCompileContext, point cfg.Point, kind operationplan.Kind) {
	switch kind {
	case operationplan.ExpressionValue,
		operationplan.ExpressionOperation,
		operationplan.ExpressionPath,
		operationplan.BranchEdgeReachability,
		operationplan.BranchConditionSource,
		operationplan.BranchRefinement,
		operationplan.BranchPathEvidence:
		// Branch facts constrain feasibility and read evidence but do not assign,
		// alias, expose, or mutate the boundary root. Their value narrowing is
		// deliberately separate from this identity-preservation proof.
		return
	case operationplan.RootAssignment:
		assignment, ok := ctx.facts.RootAssignment(point)
		if !ok {
			l.invalidateAll()
			return
		}
		if index, boundary := ctx.plan.BoundaryParamIndex(assignment.TargetSymbol()); boundary {
			l.invalidate(uint32(index))
		}
		if index, boundary := ctx.plan.BoundaryCaptureIndex(assignment.TargetSymbol()); boundary {
			l.invalidate(uint32(len(ctx.plan.BoundaryParams()) + index))
		}
		// The root-assignment handler admits this ordinary write only after
		// proving one bounded numeric-for accumulator update whose operands are
		// the accumulator and certified iterator. Neither can alias a boundary
		// parameter, and the target was handled above if it is itself a param.
		if assignment.Kind() == factflow.RootAssignmentOrdinaryRootWrite && singleCertifiedAccumulatorWrite(ctx, point, assignment) {
			return
		}
		term, err := exactCompilerSourceTerm(ctx, assignment.Source())
		if err != nil {
			l.invalidateAll()
			return
		}
		l.invalidateValueDependencies(ctx.builder.Arena(), term)
		return
	case operationplan.Return:
		// Returning the root is a read, not a mutation. Return-flow and alias
		// capabilities remain independently represented by output operations;
		// preservation only certifies the formal's post-state identity.
		return
	case operationplan.CallSite:
		// A sealed lexical call is not judged here. Direct relation composition
		// transfers the callee's must-preservation proof into this ledger, or
		// invalidates every contributing caller root when that proof is absent.
		// Keeping the bit pending is what lets that later exact proof speak.
		if surface, ok := ctx.plan.CallSurface(); ok {
			if site, found := surface.Site(point); found && site.Target.Kind() == operationplan.CallSurfaceTargetLexical {
				return
			}
		}
		// Only the exact no-capture string.match slice proves a non-escaping
		// receiver: its pattern is a literal and it has no callback argument.
		// Preserve that immutable string boundary root and nothing else. Even
		// otherwise-pure methods such as gsub can invoke caller-supplied callbacks,
		// so all other calls invalidate the complete ledger.
		operation, sealed := ctx.plan.SignatureCallOperation(point)
		if sealed {
			if receiver, exact := exactPreservedBoundaryStringMatchReceiver(ctx, point, operation); exact {
				l.retainOnly(receiver)
				return
			}
		}
		l.invalidateAll()
		return
	case operationplan.DynamicIndexExpression,
		operationplan.DynamicIndexWrite,
		operationplan.PathDescendantInvalidation:
		l.invalidateAll()
		return
	default:
		l.invalidateAll()
	}
}

func exactPreservedBoundaryStringMatchReceiver(ctx planCompileContext, point cfg.Point, operation operationplan.SignatureCallOperation) (uint32, bool) {
	if ctx.plan == nil || ctx.registry == nil {
		return 0, false
	}
	site, ok := ctx.facts.CallSiteView(point)
	if !ok || site.MethodName() != "match" {
		return 0, false
	}
	if _, exact := effectlowering.StaticScalarStringMethodReturns(ctx.registry, nil, operation.Signature(), site); !exact {
		return 0, false
	}
	receiver, ok := site.ReceiverPath()
	if !ok || receiver.Symbol == 0 || receiver.Version != 0 || len(receiver.Segments) != 0 {
		return 0, false
	}
	method, hasMethod := site.MethodPath()
	if !hasMethod || !method.Equal(receiver.Field("match")) || !site.CalleePathEqual(method) {
		return 0, false
	}
	source, hasSource := site.ReceiverSource()
	if !hasSource || !source.Valid() || source.Kind != factflow.ValueSourcePath || source.PathKey != receiver.Key() ||
		source.Expanded || source.Adjusted || source.OpenTail {
		return 0, false
	}
	index, boundary := ctx.plan.BoundaryParamIndex(receiver.Symbol)
	contracts := ctx.plan.BoundaryParamContracts()
	if !boundary || index < 0 || index >= len(contracts) || !product.Equal(ctx.registry, contracts[index], typevalue.String(ctx.registry)) {
		return 0, false
	}
	return uint32(index), true
}

func (l *paramPreservationLedger) observeExtension(kind operationplan.ExtensionKind) {
	// BodyGenericFor binds iterator outputs to loop locals but never reassigns
	// the iterator source root. Any modeled mutation, escape, or call effect is
	// independently rejected by the closed row output/effect audit before a
	// preservation term can publish. Future extension families reject by
	// default.
	switch kind {
	case operationplan.BodyGenericFor:
		return
	default:
		l.invalidateAll()
	}
}

func (l *paramPreservationLedger) invalidateValueDependencies(arena *Arena, term ValueTerm) {
	if l == nil || arena == nil || term == 0 || int(term) >= len(arena.values) {
		l.invalidateAll()
		return
	}
	visited := make(map[ValueTerm]bool)
	var visit func(ValueTerm)
	visit = func(current ValueTerm) {
		if current == 0 || int(current) >= len(arena.values) || visited[current] {
			return
		}
		visited[current] = true
		node := arena.values[current]
		// A composed call result is an explicit provenance boundary. Assigning
		// that result to a local does not prove that the local directly aliases
		// any caller parameter hidden in the callee's symbolic value. Mutating or
		// exposing the local is handled by the closed operation/effect cases.
		if node.op == valueCallResult {
			return
		}
		if node.op == valueRoot {
			switch node.root.Kind {
			case RootParam:
				l.invalidate(node.root.Index)
			case RootCapture:
				l.invalidate(l.boundaryParams + node.root.Index)
			}
		}
		for _, arg := range node.args {
			visit(arg)
		}
	}
	visit(term)
}

// certifiedRefinements performs the exit audit after the operation ledger has
// survived the whole CFG alternative. It independently rejects newly added
// output/effect families, then emits only root-identity terms.
func (l paramPreservationLedger) certifiedRefinements(arena *Arena, effects *EffectArena, shape Shape, row SymbolicCFGRow, boundaryParams, boundaryCaptures []symbol.ID) []PathRefinementTerm {
	boundaryCount := shape.Params + shape.Captures
	if arena == nil || !l.tracked || !l.valid(boundaryCount) || len(boundaryParams) != int(shape.Params) || len(boundaryCaptures) != int(shape.Captures) {
		return nil
	}
	// This closed structured-output audit is intentionally shared with manual
	// PathRefinementTerm admission. Scalar post-parameter slots are harmless;
	// mutation, escape, heap, alias, and future families reject.
	if !rowPreservesRefinementRoots(effects, Row{Output: row.Output, Effects: row.Effects}) {
		return nil
	}
	// A surviving local alias would make later exposure/mutation reasoning
	// incomplete. Boundary parameter symbols themselves are exempt: branch
	// refinement may narrow their value term without changing object identity.
	paramSymbols := make(map[symbol.ID]struct{}, len(boundaryParams)+len(boundaryCaptures))
	for _, param := range boundaryParams {
		paramSymbols[param] = struct{}{}
	}
	for _, capture := range boundaryCaptures {
		paramSymbols[capture] = struct{}{}
	}
	checked := l.clone()
	for local, value := range row.Values {
		if _, boundary := paramSymbols[local]; boundary {
			continue
		}
		checked.invalidateValueDependencies(arena, value)
	}
	result := make([]PathRefinementTerm, 0, boundaryCount)
	for index := uint32(0); index < shape.Params; index++ {
		if !checked.preserves(index) {
			continue
		}
		root := Root{Kind: RootParam, Index: index}
		result = append(result, PathRefinementTerm{Path: arena.Path(root), Value: arena.Root(root)})
	}
	for index := uint32(0); index < shape.Captures; index++ {
		if !checked.preserves(shape.Params + index) {
			continue
		}
		root := Root{Kind: RootCapture, Index: index}
		result = append(result, PathRefinementTerm{Path: arena.Path(root), Value: arena.Root(root)})
	}
	return result
}
