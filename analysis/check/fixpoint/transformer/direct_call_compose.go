package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// DirectCallBindings is one exact fixed-arity lexical application. Values and
// Paths use the callee Shape's packed namespace order. The first tranche admits
// only parameter roots; captures, globals, result roots, and heap templates
// remain contextual until their ownership rules are explicit.
type DirectCallBindings struct {
	Values []ValueTerm
	Paths  []PathTerm
}

// ComposeDirectCallRows performs relational rather than scalar cell
// substitution. Every feasible caller/callee alternative remains a distinct
// row, so return correlation, guards, and ordered effects cannot be torn apart.
// No row is returned on any unsupported output or malformed binding.
func ComposeDirectCallRows(builder *Builder, callerShape Shape, caller SymbolicCFGRow, callee Relation, bindings DirectCallBindings, site factflow.CallSiteView, maxRows int) ([]SymbolicCFGRow, error) {
	return composeDirectCallRows(builder, callerShape, caller, callee, bindings, site, maxRows, nil)
}

func composeDirectCallRows(builder *Builder, callerShape Shape, caller SymbolicCFGRow, callee Relation, bindings DirectCallBindings, site factflow.CallSiteView, maxRows int, annotations *relationAnnotations) ([]SymbolicCFGRow, error) {
	if builder == nil || builder.arena == nil || builder.effects == nil || callee.arena == nil {
		return nil, fmt.Errorf("transformer: direct-call composition requires caller and callee arenas")
	}
	if callerShape != builder.shape {
		return nil, fmt.Errorf("transformer: direct-call caller shape differs from builder ownership")
	}
	bottom := callee.IsBottom()
	if callee.contextual != "" || callee.widened {
		return nil, fmt.Errorf("transformer: direct-call callee is contextual")
	}
	if callee.arena.reg != builder.arena.reg {
		return nil, fmt.Errorf("transformer: direct-call registry ownership differs")
	}
	if callee.shape.Captures != 0 || callee.shape.Globals != 0 || callee.shape.Results != 0 || callee.shape.HeapTemplates != 0 {
		return nil, fmt.Errorf("transformer: direct-call non-parameter boundary requires contextual composition")
	}
	if site.OpenTail() || !site.Final() || !site.Expanded() || site.Adjusted() {
		return nil, fmt.Errorf("transformer: direct-call result list is not exact fixed arity")
	}
	if _, hasPoint := site.Point(); !hasPoint {
		return nil, fmt.Errorf("transformer: direct-call has no exact source point")
	}
	if callee.descriptors == nil {
		if !bottom {
			return nil, fmt.Errorf("transformer: direct-call callee has no descriptor identity")
		}
	} else {
		if handler, ok := callee.descriptors.handlers[DescriptorReturn].(returnHandler); ok && len(handler.declared) != 0 {
			return nil, fmt.Errorf("transformer: direct-call declared result contracts require symbolic projection")
		}
	}
	rootBindings, err := NewTermRootBindings(callee.shape, callerShape, bindings.Values, bindings.Paths)
	if err != nil {
		return nil, err
	}
	targets, err := exactDirectCallTargets(site)
	if err != nil {
		return nil, err
	}
	// Validate the complete caller binding transaction even when the callee is
	// Bottom and therefore has no row terms to import. A least element is not a
	// shortcut around arena ownership or DAG validation.
	if _, err := rebaseDirectCallTermDAGs(builder.arena, callee.arena, rootBindings, TermRebaseInput{}); err != nil {
		return nil, err
	}
	// Prefix evidence belongs to the caller lexical owner and is reached before
	// the call can return or diverge. Publish it monotonically for every direct
	// call, not only Bottom: a callee may have returning rows for one guard
	// partition and no successor for another. Returning-row duplicates are
	// canonicalized at freeze time.
	if annotations == nil && (len(caller.Observations) != 0 || len(caller.observationObligations) != 0 || len(callee.paramContracts) != 0) {
		return nil, fmt.Errorf("transformer: direct-call caller evidence requires relation annotation ownership")
	}
	if annotations != nil {
		annotations.observations = unionObservationTerms(builder.arena, annotations.observations, caller.Observations)
		annotations.obligations = unionobservationObligations(annotations.obligations, caller.observationObligations)
		if len(callee.paramContracts) != 0 {
			if err := recordCallEntryAnnotations(builder, caller, callee, rootBindings, site, annotations); err != nil {
				return nil, err
			}
		}
	}
	// A recursive relation cell starts at its owner-shaped least element. It has
	// no feasible return alternative yet, so this call contributes no successor
	// row in the current SCC round. Keep this after all identity, boundary, site,
	// and binding validation: Bottom is semantic, not an unchecked escape hatch.
	if bottom {
		return nil, nil
	}
	if maxRows <= 0 {
		maxRows = 256
	}
	if len(callee.rows) > maxRows {
		return nil, fmt.Errorf("transformer: direct-call row budget %d exceeds %d", len(callee.rows), maxRows)
	}
	out := make([]SymbolicCFGRow, 0, len(callee.rows))
	for rowIndex, calleeRow := range callee.rows {
		rebased, err := rebaseDirectCallRow(builder, callerShape, caller, callee, rootBindings, calleeRow, targets, site)
		if err != nil {
			return nil, fmt.Errorf("transformer: direct-call callee row %d: %w", rowIndex, err)
		}
		if rebased.Guard != builder.arena.False() {
			out = append(out, rebased)
		}
	}
	return dedupCFGRows(builder.arena, out), nil
}

func recordCallEntryAnnotations(builder *Builder, caller SymbolicCFGRow, callee Relation, bindings TermRootBindings, site factflow.CallSiteView, annotations *relationAnnotations) error {
	if builder.plan == nil || len(callee.paramContracts) != int(callee.shape.Params) {
		return fmt.Errorf("transformer: direct-call parameter contract shape is incomplete")
	}
	point, hasPoint := site.Point()
	if !hasPoint {
		return fmt.Errorf("transformer: direct-call has no exact source point")
	}
	for slot := uint32(0); slot < callee.shape.Params; slot++ {
		anchor, durable := builder.plan.CallArgumentObservationAnchor(point, slot)
		if !durable {
			continue
		}
		obligation := observationObligation{BodyOwner: builder.plan.ObservationBody(), Anchor: anchor, Guard: caller.Guard}
		annotations.obligations = recordobservationObligation(annotations.obligations, obligation)
		actual := bindings.value(Root{Kind: RootParam, Index: slot})
		expected := builder.arena.Constant(callee.paramContracts[slot])
		term := ObservationTerm{BodyOwner: builder.plan.ObservationBody(), Kind: ObservationCallArgument, Anchor: anchor, Guard: caller.Guard, Slot: slot, Actual: actual, Expected: expected}
		annotations.observations = recordObservationTerm(annotations.observations, term)
	}
	return nil
}

type directCallTarget struct {
	symbol symbol.ID
	slot   int
}

func exactDirectCallTargets(site factflow.CallSiteView) ([]directCallTarget, error) {
	if site.ResultTargetCount() == 0 {
		return nil, fmt.Errorf("transformer: direct-call has no explicit result targets")
	}
	out := make([]directCallTarget, 0, site.ResultTargetCount())
	seenSymbols := make(map[symbol.ID]struct{}, site.ResultTargetCount())
	seenSlots := make(map[int]struct{}, site.ResultTargetCount())
	for index := 0; index < site.ResultTargetCount(); index++ {
		target, _ := site.ResultTargetAt(index)
		if target.Kind() != factflow.CallResultTargetLocalAssignment || target.TargetSymbol() == 0 ||
			target.TargetPathEmpty() || target.TargetPathSegmentCount() != 0 || target.TargetPath().Symbol != target.TargetSymbol() || target.ResultIndex() < 0 {
			return nil, fmt.Errorf("transformer: direct-call result target %d is not an exact local root", index)
		}
		if _, duplicate := seenSymbols[target.TargetSymbol()]; duplicate {
			return nil, fmt.Errorf("transformer: direct-call duplicate result symbol %d", target.TargetSymbol())
		}
		if _, duplicate := seenSlots[target.ResultIndex()]; duplicate {
			return nil, fmt.Errorf("transformer: direct-call duplicate result slot %d", target.ResultIndex())
		}
		seenSymbols[target.TargetSymbol()] = struct{}{}
		seenSlots[target.ResultIndex()] = struct{}{}
		out = append(out, directCallTarget{symbol: target.TargetSymbol(), slot: target.ResultIndex()})
	}
	return out, nil
}

func rebaseDirectCallRow(builder *Builder, callerShape Shape, caller SymbolicCFGRow, callee Relation, bindings TermRootBindings, row Row, targets []directCallTarget, site factflow.CallSiteView) (SymbolicCFGRow, error) {
	paramPreserved, err := rebaseDirectCallParamRefinements(builder.arena, callee.arena, callerShape, caller.paramPreserved, callee.shape, bindings, row.PathRefinements)
	if err != nil {
		return SymbolicCFGRow{}, err
	}
	if err := validateDirectCallStructuredOutput(callee.arena.reg, row.Output, callee.shape); err != nil {
		return SymbolicCFGRow{}, err
	}
	values := make([]ValueTerm, 0, len(row.Ops)+len(row.Proofs))
	guards := []Guard{row.Guard}
	opValueAt := make([]int, len(row.Ops))
	for i, op := range row.Ops {
		if op.Kind != OutputReturn || op.Descriptor != DescriptorReturn {
			return SymbolicCFGRow{}, fmt.Errorf("non-return operation %q requires contextual composition", op.Descriptor)
		}
		opValueAt[i] = len(values)
		values = append(values, op.Value)
	}
	proofPathAt := make([]int, len(row.Proofs))
	proofKeyAt := make([]int, len(row.Proofs))
	paths := make([]PathTerm, 0, len(row.Proofs))
	for i, proof := range row.Proofs {
		proofPathAt[i] = len(paths)
		paths = append(paths, proof.Table)
		proofKeyAt[i] = -1
		if proof.Key != 0 {
			proofKeyAt[i] = len(values)
			values = append(values, proof.Key)
		}
	}
	rebasedTerms, err := rebaseDirectCallTermDAGs(builder.arena, callee.arena, bindings, TermRebaseInput{Values: values, Paths: paths, Guards: guards})
	if err != nil {
		return SymbolicCFGRow{}, err
	}
	next := cloneCFGRow(caller)
	next.paramPreserved = paramPreserved
	next.Guard = builder.arena.And(caller.Guard, rebasedTerms.Guards[0])
	if len(callee.paramContracts) == int(callee.shape.Params) {
		point, hasPoint := site.Point()
		if !hasPoint {
			return SymbolicCFGRow{}, fmt.Errorf("call observation has no source point")
		}
		for slot := uint32(0); slot < callee.shape.Params; slot++ {
			anchor, durable := builder.plan.CallArgumentObservationAnchor(point, slot)
			if !durable {
				continue
			}
			actual := bindings.value(Root{Kind: RootParam, Index: slot})
			expected := builder.arena.Constant(callee.paramContracts[slot])
			next.Observations = recordObservationTerm(next.Observations, ObservationTerm{BodyOwner: builder.plan.ObservationBody(), Kind: ObservationCallArgument, Anchor: anchor, Guard: caller.Guard, Slot: slot, Actual: actual, Expected: expected})
		}
	}
	returns := make(map[uint32]ValueTerm, len(row.Ops))
	for i, op := range row.Ops {
		value := rebasedTerms.Values[opValueAt[i]]
		if prior, exists := returns[op.Slot]; exists && prior != value {
			return SymbolicCFGRow{}, fmt.Errorf("return slot %d has multiple row values", op.Slot)
		}
		returns[op.Slot] = value
	}
	callPoint, hasCallPoint := site.Point()
	for _, target := range targets {
		produced, exists := returns[uint32(target.slot)]
		if !exists {
			return SymbolicCFGRow{}, fmt.Errorf("result target slot %d has no callee row value", target.slot)
		}
		value := produced
		if hasCallPoint {
			value = builder.arena.CallResultValue(callPoint, uint32(target.slot), produced)
			if value == 0 {
				return SymbolicCFGRow{}, fmt.Errorf("result target slot %d has an invalid composed value", target.slot)
			}
		}
		if prior, exists := next.Values[target.symbol]; exists {
			// Cyclic exact closure revisits the same lexical call point. An
			// identical interned result binding is the fixed point; a changed
			// value is still a contextual multi-write.
			if prior != value {
				return SymbolicCFGRow{}, fmt.Errorf("result symbol %d already has a row binding", target.symbol)
			}
		} else {
			next.Values[target.symbol] = value
		}
		if hasCallPoint {
			resultRoot := ResultRoot{Point: callPoint, Slot: uint32(target.slot)}
			if prior, exists := next.ResultRoots[resultRoot]; exists && prior != value {
				return SymbolicCFGRow{}, fmt.Errorf("call result root %d:%d already has a different row binding", callPoint, target.slot)
			}
			next.ResultRoots[resultRoot] = value
		}
	}
	for i, proof := range row.Proofs {
		rebased := proof
		rebased.Table = rebasedTerms.Paths[proofPathAt[i]]
		if proofKeyAt[i] >= 0 {
			rebased.Key = rebasedTerms.Values[proofKeyAt[i]]
		}
		next.Proofs = append(next.Proofs, rebased)
	}
	structuredProofs, err := rebaseDirectCallOutputProofs(builder.arena, bindings, row.Output.NormalReturnFacts.BranchProofs)
	if err != nil {
		return SymbolicCFGRow{}, err
	}
	next.Proofs = append(next.Proofs, structuredProofs...)
	if len(row.Effects) != 0 {
		if callee.effects == nil {
			return SymbolicCFGRow{}, fmt.Errorf("callee row effects have no arena")
		}
		rebasedEffects, err := rebaseDirectCallEffectDAGs(builder.effects, callee.effects, bindings, row.Effects)
		if err != nil {
			return SymbolicCFGRow{}, err
		}
		next.Effects = append(next.Effects, rebasedEffects.Effects...)
	}
	// Callee correlation metadata is consumed by the row cross-product above.
	// It must never become correlation among the caller function's own returns.
	if row.Output.MaySuspend {
		next.Output.MaySuspend = true
	}
	return next, nil
}

// rebaseDirectCallParamRefinements transfers the callee's root-identity proof
// into the caller's row-local preservation ledger. A proof is addressable in
// the caller only when both bindings name the same root-only caller parameter.
// Value-only terms with no boundary-root dependency cannot designate caller
// state, so their callee-local proof is deliberately consumed. Every other
// mapping is ambiguous and fails closed rather than silently dropping a fact.
func rebaseDirectCallParamRefinements(arena, calleeArena *Arena, callerShape Shape, caller paramPreservationLedger, calleeShape Shape, bindings TermRootBindings, refinements []PathRefinementTerm) (paramPreservationLedger, error) {
	if arena == nil || calleeArena == nil || bindings.callee != calleeShape || bindings.caller != callerShape {
		return paramPreservationLedger{}, fmt.Errorf("symbolic path refinements have foreign root bindings")
	}
	for index, refinement := range refinements {
		if !refinement.validPreservedBoundaryRoot(calleeArena, calleeShape) || calleeArena.paths[refinement.Path].root.Kind != RootParam {
			// The relation builder already enforces this invariant. Keep the
			// composition boundary independently fail-closed for forged relations.
			return paramPreservationLedger{}, fmt.Errorf("symbolic path refinement %d is not a preserved parameter root", index)
		}
		root := calleeArena.paths[refinement.Path].root
		for prior := 0; prior < index; prior++ {
			if calleeArena.paths[refinements[prior].Path].root == root {
				return paramPreservationLedger{}, fmt.Errorf("symbolic path refinement %d duplicates parameter %d", index, root.Index)
			}
		}
	}

	next := caller.clone()
	for index := uint32(0); index < calleeShape.Params; index++ {
		root := Root{Kind: RootParam, Index: index}
		value := bindings.value(root)
		path := bindings.path(root)
		preserved := false
		for _, refinement := range refinements {
			if calleeArena.paths[refinement.Path].root.Index == index {
				preserved = true
				break
			}
		}
		callerParam, exact := callerParamIdentityBinding(arena, callerShape, value, path)
		if exact {
			if !preserved {
				next.invalidate(callerParam)
			}
			continue
		}
		addressable, params, valid := callerBoundaryDependencies(arena, callerShape, value, path)
		if !valid {
			return paramPreservationLedger{}, fmt.Errorf("symbolic parameter %d has malformed caller bindings", index)
		}
		if preserved {
			if addressable {
				return paramPreservationLedger{}, fmt.Errorf("symbolic parameter %d does not map to one caller parameter identity", index)
			}
			// A constant or other root-free term with no PathTerm has no caller
			// boundary location to refine. Its callee-local proof is consumed.
			continue
		}
		// Without a callee preservation proof, every caller parameter that
		// contributes to the argument loses its must-preservation bit. This is
		// conservative for descendants and computed arguments while still
		// allowing exact direct composition of their return values.
		for _, param := range params {
			next.invalidate(param)
		}
	}
	return next, nil
}

func callerParamIdentityBinding(arena *Arena, shape Shape, value ValueTerm, path PathTerm) (uint32, bool) {
	if arena == nil || value == 0 || path == 0 || int(value) >= len(arena.values) || int(path) >= len(arena.paths) {
		return 0, false
	}
	v := arena.values[value]
	p := arena.paths[path]
	if v.op != valueRoot || v.root.Kind != RootParam || !shape.validate(v.root) ||
		p.root != v.root || len(p.segments) != 0 {
		return 0, false
	}
	return v.root.Index, true
}

func callerBoundaryDependencies(arena *Arena, shape Shape, value ValueTerm, path PathTerm) (bool, []uint32, bool) {
	var params []uint32
	if arena == nil || value == 0 || int(value) >= len(arena.values) {
		return false, nil, false
	}
	if path != 0 {
		if int(path) >= len(arena.paths) || !arena.validPath(path, shape) {
			return false, nil, false
		}
		if root := arena.paths[path].root; root.Kind == RootParam {
			params = append(params, root.Index)
		}
	}
	addressable := path != 0
	var visit func(ValueTerm) bool
	visit = func(term ValueTerm) bool {
		if term == 0 || int(term) >= len(arena.values) {
			return false
		}
		node := arena.values[term]
		if node.op == valueRoot {
			if !shape.validate(node.root) {
				return false
			}
			addressable = true
			if node.root.Kind == RootParam {
				seen := false
				for _, param := range params {
					if param == node.root.Index {
						seen = true
						break
					}
				}
				if !seen {
					params = append(params, node.root.Index)
				}
			}
			return true
		}
		for _, arg := range node.args {
			// Arena construction is append-only: every valid DAG edge points to
			// an older term. Checking that order makes the walk cycle-safe without
			// allocating a visited map on every direct call.
			if arg >= term || !visit(arg) {
				return false
			}
		}
		return true
	}
	if !visit(value) {
		return false, nil, false
	}
	return addressable, params, true
}

func validateDirectCallStructuredOutput(reg *axis.Registry, out summary.Summary, shape Shape) error {
	if len(out.NormalReturnParams) != 0 && len(out.NormalReturnParams) != int(shape.Params) {
		return fmt.Errorf("normal-return parameter width %d differs from callee shape %d", len(out.NormalReturnParams), shape.Params)
	}
	for index, value := range out.NormalReturnParams {
		if !product.Equal(reg, value, product.Top()) {
			return fmt.Errorf("normal-return parameter %d refinement requires symbolic state application", index)
		}
	}
	facts := out.NormalReturnFacts
	facts.BranchProofs = nil
	if !facts.Empty() {
		return fmt.Errorf("normal-return fact family outside exact branch-proof slice")
	}
	residual := out.Clone()
	residual.NormalReturnParams = nil
	residual.NormalReturnFacts = callboundary.NormalReturnFacts{}
	// These two fields are the joined encoding of callee row correlation. The
	// cross-product consumes them; copying them would retarget callee result
	// slots to the caller function's result namespace.
	residual.ReturnConditionSlotRefinements = nil
	residual.ReturnPresenceRelations = nil
	residual.MaySuspend = false
	if kinds := summary.PresentFactKinds(residual); len(kinds) != 0 {
		return fmt.Errorf("structured output facts %v require contextual composition", kinds)
	}
	if residual.HeapKeySpace != nil {
		return fmt.Errorf("structured output keyspace without admitted heap output")
	}
	return nil
}

func rebaseDirectCallOutputProofs(caller *Arena, bindings TermRootBindings, proofs []callboundary.BranchProof) ([]BranchProofTerm, error) {
	out := make([]BranchProofTerm, 0, len(proofs))
	for index, proof := range proofs {
		if proof.Kind != pathevidence.BranchProofPathPresence || !presence.Equal(proof.Presence, presence.Present()) || !proof.Other.IsEmpty() {
			return nil, fmt.Errorf("structured branch proof %d kind requires contextual composition", index)
		}
		param := proof.Path.PlaceholderIndex()
		if param < 0 || uint32(param) >= bindings.callee.Params {
			return nil, fmt.Errorf("structured branch proof %d has non-parameter path", index)
		}
		base := bindings.path(Root{Kind: RootParam, Index: uint32(param)})
		path, ok := appendCallerPath(caller, base, proof.Path.Segments)
		if !ok {
			return nil, fmt.Errorf("structured branch proof %d has no caller boundary path", index)
		}
		out = append(out, BranchProofTerm{Kind: proof.Kind, Table: path, Presence: proof.Presence})
	}
	return out, nil
}

func appendCallerPath(arena *Arena, base PathTerm, suffix []segment.Segment) (PathTerm, bool) {
	if arena == nil || base == 0 || int(base) >= len(arena.paths) {
		return 0, false
	}
	node := arena.paths[base]
	if node.root.Kind != RootParam {
		return 0, false
	}
	segments := make([]segment.Segment, 0, len(node.segments)+len(suffix))
	segments = append(segments, node.segments...)
	segments = append(segments, suffix...)
	return arena.internPath(pathNode{root: node.root, segments: segments}), true
}
