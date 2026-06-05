package paramevidence

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	cfganalysis "github.com/wippyai/go-lua/compiler/cfg/analysis"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	flowpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// BodyPreconditions are hard parameter obligations proven by the current
// function body. Public is the subset that must be exposed to callers because
// it is not already proven by local branch/path conditions.
type BodyPreconditions struct {
	Body   []typ.Type
	Public []typ.Type
}

// IsZero reports whether no precondition evidence was found.
func (p BodyPreconditions) IsZero() bool {
	return len(p.Body) == 0 && len(p.Public) == 0
}

// BodyPreconditionContext owns the derivation from solved flow/path evidence to
// current-function BodyParams/PublicParams.
type BodyPreconditionContext struct {
	result              *api.FuncResult
	bindings            *bind.BindingTable
	currentSym          cfg.SymbolID
	paramIndexBySym     map[cfg.SymbolID]int
	paramDeclPointBySym map[cfg.SymbolID]cfg.Point
	dominates           func(cfg.Point, cfg.Point) bool
}

// NewBodyPreconditionContext constructs the solved-flow projection used to
// derive current-function parameter preconditions from hard call contexts.
func NewBodyPreconditionContext(graph *cfg.Graph, result *api.FuncResult, bindings *bind.BindingTable) BodyPreconditionContext {
	if graph == nil && result != nil {
		graph = result.Graph
	}
	paramIndexBySym, paramDeclPointBySym := functionParameterIndexes(graph)
	return BodyPreconditionContext{
		result:              result,
		bindings:            bindings,
		paramIndexBySym:     paramIndexBySym,
		paramDeclPointBySym: paramDeclPointBySym,
		dominates:           pointDominates(graph),
	}
}

// WithCurrentFunctionSymbol marks recursive self-calls so their feedback stays
// body-local instead of becoming an unconditional caller contract.
func (c BodyPreconditionContext) WithCurrentFunctionSymbol(sym cfg.SymbolID) BodyPreconditionContext {
	c.currentSym = sym
	return c
}

// PreconditionsFromCall derives current-function parameter obligations from
// one hard call context in the solved body. calleeParamInferred reports whether
// the callee parameter at a runtime argument index is an unannotated inferred
// parameter; such an expectation is an in-progress LUB of the callee's own
// callsites, not a concrete annotation, so it must not become a contravariant
// hard obligation on the caller parameter. The genuine callsite arg-check still
// enforces the callee's concrete contract after the interprocedural fixpoint.
func (c BodyPreconditionContext) PreconditionsFromCall(p cfg.Point, evidence api.CallEvidence, expectedReceiver typ.Type, calleeParamInferred func(argIdx int) bool) BodyPreconditions {
	info := evidence.Info
	if c.result == nil || info == nil || len(c.paramIndexBySym) == 0 {
		return BodyPreconditions{}
	}
	var out BodyPreconditions
	if info.Method != "" && info.Receiver != nil {
		if idx, evidence, _, ok := c.hardUseEvidence(info.Receiver, expectedReceiver, p); ok {
			out.Body, _ = MergeBodyAt(out.Body, idx, evidence, HardContractJoin)
		}
	}
	for i, arg := range info.Args {
		if calleeParamInferred != nil && calleeParamInferred(i) {
			continue
		}
		expected := evidence.ExpectedArgType(i)
		idx, evidence, public, ok := c.hardUseEvidence(arg, expected, p)
		if !ok {
			continue
		}
		out.Body, _ = MergeBodyAt(out.Body, idx, evidence, HardContractJoin)
		if public && !c.recursiveSelfCall(info) {
			out.Public, _ = MergeBodyAt(out.Public, idx, evidence, HardContractJoin)
		}
	}
	return out
}

func (c BodyPreconditionContext) recursiveSelfCall(info *cfg.CallInfo) bool {
	return c.currentSym != 0 && info != nil && info.CalleeSymbol == c.currentSym
}

func (c BodyPreconditionContext) hardUseEvidence(arg ast.Expr, expected typ.Type, p cfg.Point) (idx int, evidence typ.Type, public bool, ok bool) {
	if arg == nil || !HardPublicEvidence(expected) || c.bindings == nil {
		return 0, nil, false, false
	}
	path := flowpath.FromExprWithBindings(arg, nil, c.bindings)
	paramIdx, evidence, conditional, found := c.paramEvidenceFromPath(path, expected, p, nil)
	if !found {
		return 0, nil, false, false
	}
	if !HardPublicEvidence(evidence) {
		return 0, nil, false, false
	}
	locallyProven := false
	if proofs := c.conditionProofFacts(); proofs != nil {
		locallyProven = proofs.ProvesTypeAt(p, path, evidence)
	}
	return paramIdx, evidence, !locallyProven && !conditional, true
}

func (c BodyPreconditionContext) paramEvidenceFromPath(path constraint.Path, expected typ.Type, p cfg.Point, seen map[cfg.SymbolID]bool) (int, typ.Type, bool, bool) {
	if path.Symbol == 0 || expected == nil {
		return 0, nil, false, false
	}
	evidence := PathEvidence(path.Segments, expected)
	if evidence == nil {
		return 0, nil, false, false
	}
	var conditional bool
	if c.conditionProofFacts() != nil {
		evidence, conditional = c.conditionedPathEvidence(path, evidence, p)
	}
	if paramIdx, found := c.paramIndexBySym[path.Symbol]; found {
		if c.symbolLocallyReboundBefore(path.Symbol, p) {
			return 0, nil, false, false
		}
		return paramIdx, evidence, conditional, true
	}
	if seen == nil {
		seen = make(map[cfg.SymbolID]bool, 1)
	}
	if seen[path.Symbol] {
		return 0, nil, false, false
	}
	seen[path.Symbol] = true

	assign, ok := c.dominatingAssignmentForPath(path, p)
	if !ok {
		return 0, nil, false, false
	}
	sourceEvidence := PathEvidence(path.Segments[len(assign.TargetPath.Segments):], expected)
	if sourceEvidence == nil {
		return 0, nil, false, false
	}
	sourcePath, sourceEvidence, ok := c.sourceEvidenceForDerivedLocal(assign.Source, sourceEvidence, p)
	if !ok {
		return 0, nil, false, false
	}
	idx, sourceEvidence, sourceConditional, ok := c.paramEvidenceFromPath(sourcePath, sourceEvidence, assign.Point, seen)
	return idx, sourceEvidence, conditional || sourceConditional, ok
}

func (c BodyPreconditionContext) conditionedPathEvidence(path constraint.Path, evidence typ.Type, p cfg.Point) (typ.Type, bool) {
	proofs := c.conditionProofFacts()
	if proofs == nil {
		return evidence, false
	}
	return conditionedPathEvidenceFromCondition(path, evidence, proofs.ConditionAt(p), func(value constraint.Path) *typ.Literal {
		return c.literalValueAtPath(p, value)
	})
}

// ConditionedPathEvidence weakens hard path evidence when the current abstract
// condition proves the use is locally guarded. It is the transfer-time counterpart
// of BodyPreconditionContext.conditionedPathEvidence for demand that is already
// being computed inside the single product fixpoint.
func ConditionedPathEvidence(path constraint.Path, evidence typ.Type, cond constraint.Condition) (typ.Type, bool) {
	return conditionedPathEvidenceFromCondition(path, evidence, cond, nil)
}

// ConditionedPathContract wraps a leaf contract under path, then weakens the
// caller-visible contract when the current abstract condition proves the path is
// locally guarded. It is the native ParamContract counterpart to
// ConditionedPathEvidence, used by transfer so type, capability, and contract
// demands share one guard policy.
func ConditionedPathContract(path constraint.Path, leaf ParamContract, cond constraint.Condition) (ParamContract, bool) {
	contract := DemandFromPathContract(path.Segments, leaf)
	if path.Symbol == 0 || ParamContractDomain.Equal(contract, ParamContractDomain.Bottom()) {
		return contract, false
	}
	evidence := contract.ProjectValue()
	if evidence == nil {
		return contract, false
	}
	conditioned, ok := ConditionedPathEvidence(path, evidence, cond)
	if !ok || conditioned == nil {
		return contract, ok
	}
	return DemandFromType(conditioned), true
}

// ConditionedLeafContract weakens a demand on a derived local path before that
// demand is lifted back through ValueOrigin provenance. The returned contract is
// still leaf-relative: callers wrap it under the origin remainder after local
// guard complements have been admitted at the consumed leaf.
func ConditionedLeafContract(path constraint.Path, leaf ParamContract, cond constraint.Condition) (ParamContract, bool) {
	if path.Symbol == 0 || ParamContractDomain.Equal(leaf, ParamContractDomain.Bottom()) ||
		cond.IsFalse() || !cond.HasConstraints() {
		return leaf, false
	}
	evidence := leaf.ProjectValue()
	if evidence == nil {
		return leaf, false
	}
	complement := pathGuardComplement(path, cond)
	if complement == nil {
		return leaf, false
	}
	return DemandFromType(typ.NewUnion(evidence, complement)), true
}

func (c BodyPreconditionContext) literalValueAtPath(p cfg.Point, path constraint.Path) *typ.Literal {
	if path.IsEmpty() {
		return nil
	}
	if lit := singletonLiteralType(c.pathTypeAt(p, path)); lit != nil {
		return lit
	}
	if len(path.Segments) == 0 {
		if facts := c.constFacts(); facts != nil {
			if val := facts.ConstValueAtSym(p, path.Symbol); val != nil {
				return singletonLiteralType(val.ToLiteralType())
			}
		}
	}
	return nil
}

func conditionedPathEvidenceFromCondition(path constraint.Path, evidence typ.Type, cond constraint.Condition, resolveLiteral func(constraint.Path) *typ.Literal) (typ.Type, bool) {
	if evidence == nil || path.Symbol == 0 || cond.IsFalse() || !cond.HasConstraints() {
		return evidence, false
	}
	// A guard dominating a path use turns the exported caller obligation into an
	// implication: G => evidence, equivalently !G OR evidence. Approximate the
	// guard complement at the consumed path leaf so guarded-away runtime values
	// remain admitted by the public contract while the guarded branch still sees
	// the precise body-local evidence.
	if complement := pathGuardComplement(path, cond); complement != nil {
		if relaxed := guardedLeafEvidence(path.Segments, evidence, complement); relaxed != nil && !typ.TypeEquals(relaxed, evidence) {
			return relaxed, true
		}
		return evidence, true
	}
	conditionEvidence := typ.Type(nil)
	guarded := false
	for _, item := range cond.MustConstraints() {
		target, field, value, ok := fieldConditionConstraint(item, resolveLiteral)
		if !ok || target.Symbol != path.Symbol {
			continue
		}
		if !conditionTargetAppliesToPath(target, path) {
			continue
		}
		guarded = true
		if value == nil {
			continue
		}
		segments := append(cloneConditionSegments(target.Segments), constraint.Segment{
			Kind: constraint.SegmentField,
			Name: field,
		})
		if ev := PathEvidence(segments, value); ev != nil {
			// The accumulated discriminant constraints are precise guard values, not
			// observations: merge them structurally so a tag literal survives until it
			// joins its payload record, where it is recognized as a discriminant.
			if conditionEvidence == nil {
				conditionEvidence = ev
			} else if merged, ok := mergeConditionedRecordEvidence(conditionEvidence, ev); ok {
				conditionEvidence = merged
			} else {
				conditionEvidence = JoinBody(conditionEvidence, ev)
			}
		}
	}
	if conditionEvidence == nil {
		return evidence, guarded
	}
	conditioned := JoinBody(evidence, conditionEvidence)
	if joined, ok := mergeConditionedRecordEvidence(evidence, conditionEvidence); ok {
		conditioned = joined
	}
	if conditioned == nil || typ.TypeEquals(conditioned, evidence) {
		return evidence, guarded
	}
	return conditioned, true
}

// pathGuardComplement returns the type admitted by the negation of unary guards
// that dominate a demand on path. Versions are ignored: the guard and the use
// refer to the same runtime location by symbol and segment chain even when their
// SSA versions differ.
func pathGuardComplement(path constraint.Path, cond constraint.Condition) typ.Type {
	var complement typ.Type
	for _, item := range cond.MustConstraints() {
		var guardPath constraint.Path
		var next typ.Type
		switch v := item.(type) {
		case constraint.Truthy:
			guardPath = v.Path
			next = typ.NewUnion(typ.Nil, typ.False)
		case constraint.NotNil:
			guardPath = v.Path
			next = typ.Nil
		case constraint.FieldNotEquals:
			if v.Value == nil {
				continue
			}
			guardPath = v.Target.Field(v.Field)
			next = v.Value
		default:
			continue
		}
		if samePathIgnoringVersion(guardPath, path) {
			if complement == nil {
				complement = next
			} else {
				complement = typ.NewUnion(complement, next)
			}
		}
	}
	return complement
}

func samePathIgnoringVersion(a, b constraint.Path) bool {
	left, leftOK := flow.StableAddressOfPath(a)
	right, rightOK := flow.StableAddressOfPath(b)
	if !leftOK || !rightOK {
		return false
	}
	return left.Equal(right)
}

// guardedLeafEvidence rebuilds structural evidence for segments with the
// guard-complement admitted at the deepest consumed leaf.
func guardedLeafEvidence(segments []constraint.Segment, evidence typ.Type, complement typ.Type) typ.Type {
	if complement == nil {
		return evidence
	}
	if len(segments) == 0 {
		return typ.NewUnion(evidence, complement)
	}
	rec := unwrap.Record(evidence)
	if rec == nil || len(rec.Fields) != 1 {
		return evidence
	}
	field := rec.Fields[0]
	if len(segments) == 1 {
		builder := typ.NewRecord().SetOpen(rec.Open)
		fieldType := typ.NewUnion(field.Type, complement)
		if fieldType == nil {
			fieldType = field.Type
		}
		switch {
		case field.Optional && field.Readonly:
			builder.OptReadonlyField(field.Name, fieldType)
		case field.Optional:
			builder.OptField(field.Name, fieldType)
		case field.Readonly:
			builder.ReadonlyField(field.Name, fieldType)
		default:
			builder.Field(field.Name, fieldType)
		}
		return builder.Build()
	}
	child := guardedLeafEvidence(segments[1:], field.Type, complement)
	if child == nil || typ.TypeEquals(child, field.Type) {
		return evidence
	}
	builder := typ.NewRecord().SetOpen(rec.Open)
	switch {
	case field.Optional && field.Readonly:
		builder.OptReadonlyField(field.Name, child)
	case field.Optional:
		builder.OptField(field.Name, child)
	case field.Readonly:
		builder.ReadonlyField(field.Name, child)
	default:
		builder.Field(field.Name, child)
	}
	return builder.Build()
}

func fieldConditionConstraint(item constraint.Constraint, resolveLiteral func(constraint.Path) *typ.Literal) (constraint.Path, string, *typ.Literal, bool) {
	switch v := item.(type) {
	case constraint.FieldEquals:
		return v.Target, v.Field, v.Value, true
	case constraint.FieldEqualsPath:
		if resolveLiteral == nil {
			return v.Target, v.Field, nil, true
		}
		return v.Target, v.Field, resolveLiteral(v.Value), true
	default:
		return constraint.Path{}, "", nil, false
	}
}

func singletonLiteralType(t typ.Type) *typ.Literal {
	switch v := unwrap.Alias(t).(type) {
	case *typ.Literal:
		return v
	case *typ.Union:
		var out *typ.Literal
		for _, member := range v.Members {
			lit := singletonLiteralType(member)
			if lit == nil {
				return nil
			}
			if out == nil {
				out = lit
				continue
			}
			if !typ.LiteralEquals(out, lit) {
				return nil
			}
		}
		return out
	default:
		return nil
	}
}

func mergeConditionedRecordEvidence(evidence, condition typ.Type) (typ.Type, bool) {
	left := unwrap.Record(evidence)
	right := unwrap.Record(condition)
	if left == nil || right == nil {
		return nil, false
	}
	builder := typ.NewRecord()
	if left.Open || right.Open {
		builder.SetOpen(true)
	}
	if left.HasMapComponent() {
		builder.MapComponent(left.MapKey, left.MapValue)
	} else if right.HasMapComponent() {
		builder.MapComponent(right.MapKey, right.MapValue)
	}
	seen := make(map[fieldkey.Key]typ.Field, len(left.Fields)+len(right.Fields))
	for _, field := range left.Fields {
		key, ok := fieldkey.FromName(field.Name)
		if !ok {
			continue
		}
		seen[key] = field
	}
	for _, field := range right.Fields {
		key, ok := fieldkey.FromName(field.Name)
		if !ok {
			continue
		}
		if existing, ok := seen[key]; ok {
			field.Type = JoinBody(existing.Type, field.Type)
			field.Optional = existing.Optional && field.Optional
			field.Readonly = existing.Readonly || field.Readonly
			seen[key] = field
			continue
		}
		seen[key] = field
	}
	for _, key := range fieldkey.Sorted(seen) {
		field := seen[key]
		switch {
		case field.Optional && field.Readonly:
			builder.OptReadonlyField(field.Name, field.Type)
		case field.Optional:
			builder.OptField(field.Name, field.Type)
		case field.Readonly:
			builder.ReadonlyField(field.Name, field.Type)
		default:
			builder.Field(field.Name, field.Type)
		}
	}
	return builder.Build(), true
}

func conditionTargetAppliesToPath(target, path constraint.Path) bool {
	if target.Symbol != path.Symbol || len(target.Segments) > len(path.Segments) {
		return false
	}
	for i := range target.Segments {
		if target.Segments[i] != path.Segments[i] {
			return false
		}
	}
	return true
}

func cloneConditionSegments(in []constraint.Segment) []constraint.Segment {
	if len(in) == 0 {
		return nil
	}
	out := make([]constraint.Segment, len(in))
	copy(out, in)
	return out
}

func (c BodyPreconditionContext) symbolLocallyReboundBefore(sym cfg.SymbolID, p cfg.Point) bool {
	if c.result == nil || sym == 0 || c.dominates == nil {
		return false
	}
	for _, ev := range c.result.Evidence.Assignments {
		if ev.Info == nil || ev.Point == p || !c.dominates(ev.Point, p) {
			continue
		}
		for _, target := range ev.Info.Targets {
			if ev.Point == c.paramDeclPointBySym[sym] {
				continue
			}
			if target.Kind == cfg.TargetIdent && target.Symbol == sym {
				return true
			}
		}
	}
	return false
}

func (c BodyPreconditionContext) dominatingAssignmentForPath(path constraint.Path, p cfg.Point) (flow.UnifiedAssignment, bool) {
	if c.result == nil || c.result.FlowInputs == nil || path.Symbol == 0 || c.dominates == nil {
		return flow.UnifiedAssignment{}, false
	}
	best := -1
	ambiguous := false
	for i := range c.result.FlowInputs.Assignments {
		assign := c.result.FlowInputs.Assignments[i]
		if !assignmentTargetPrefixOfPath(assign.TargetPath, path) || !c.dominates(assign.Point, p) {
			continue
		}
		if best < 0 {
			best = i
			ambiguous = false
			continue
		}
		current := c.result.FlowInputs.Assignments[best]
		switch {
		case assign.Point == current.Point && len(assign.TargetPath.Segments) > len(current.TargetPath.Segments):
			best = i
			ambiguous = false
		case c.dominates(current.Point, assign.Point):
			best = i
			ambiguous = false
		case c.dominates(assign.Point, current.Point):
		default:
			ambiguous = true
		}
	}
	if best < 0 || ambiguous {
		return flow.UnifiedAssignment{}, false
	}
	return c.result.FlowInputs.Assignments[best], true
}

func assignmentTargetPrefixOfPath(target, path constraint.Path) bool {
	if target.Symbol == 0 || target.Symbol != path.Symbol || len(target.Segments) > len(path.Segments) {
		return false
	}
	for i := range target.Segments {
		if target.Segments[i] != path.Segments[i] {
			return false
		}
	}
	return true
}

func (c BodyPreconditionContext) sourceEvidenceForDerivedLocal(src flow.AssignmentSource, localEvidence typ.Type, p cfg.Point) (constraint.Path, typ.Type, bool) {
	if localEvidence == nil {
		return constraint.Path{}, nil, false
	}
	switch src.Kind {
	case flow.AssignmentSourcePath:
		if src.Path.Symbol == 0 {
			return constraint.Path{}, nil, false
		}
		return src.Path, localEvidence, true
	case flow.AssignmentSourceIterator:
		if src.Path.Symbol == 0 {
			return constraint.Path{}, nil, false
		}
		if containerEvidence := iteratorContainerEvidence(src, localEvidence); containerEvidence != nil {
			return src.Path, containerEvidence, true
		}
	case flow.AssignmentSourceMapElement:
		if src.MapPath.Symbol == 0 {
			return constraint.Path{}, nil, false
		}
		keyType := c.mapElementEvidenceKeyType(src, p)
		return src.MapPath, MapElementEvidence(keyType, localEvidence), true
	}
	return constraint.Path{}, nil, false
}

func (c BodyPreconditionContext) mapElementEvidenceKeyType(src flow.AssignmentSource, p cfg.Point) typ.Type {
	if src.KeySymbol == 0 {
		return typ.Any
	}
	keyPath := constraint.Path{Root: src.KeyVar, Symbol: src.KeySymbol}
	if keyType := c.pathTypeAt(p, keyPath); keyType != nil && !typ.IsAbsentOrUnknown(keyType) {
		return keyType
	}
	return typ.Any
}

func (c BodyPreconditionContext) conditionProofFacts() flow.ConditionProofFacts {
	if c.result == nil {
		return nil
	}
	return c.result.ConditionProofFacts()
}

func (c BodyPreconditionContext) constFacts() flow.ConstFacts {
	if c.result == nil {
		return nil
	}
	return c.result.ConstFacts()
}

func (c BodyPreconditionContext) pathObservationFacts() flow.PathObservationFacts {
	if c.result == nil {
		return nil
	}
	return c.result.PathObservationFacts()
}

func (c BodyPreconditionContext) pathTypeAt(p cfg.Point, path constraint.Path) typ.Type {
	if path.IsEmpty() {
		return nil
	}
	if facts := c.pathObservationFacts(); facts != nil {
		obs := facts.ObservePath(flow.PathObservationQuery{
			Point:               p,
			Path:                path,
			View:                flow.PathReadCurrent,
			AllowConditionProof: true,
			PreserveProof:       true,
		})
		if obs.Resolved() {
			return obs.Type
		}
	}
	if c.result != nil {
		if flowOps := c.result.SolvedFlow(); flowOps != nil {
			return flowOps.NarrowedTypeAt(p, path)
		}
	}
	return nil
}

func functionParameterIndexes(graph *cfg.Graph) (map[cfg.SymbolID]int, map[cfg.SymbolID]cfg.Point) {
	paramIndexBySym := make(map[cfg.SymbolID]int)
	paramDeclPointBySym := make(map[cfg.SymbolID]cfg.Point)
	if graph == nil {
		return paramIndexBySym, paramDeclPointBySym
	}
	for idx, slot := range graph.ParamSlotsReadOnly() {
		if slot.Symbol != 0 {
			paramIndexBySym[slot.Symbol] = idx
			paramDeclPointBySym[slot.Symbol] = slot.DeclPoint
		}
	}
	return paramIndexBySym, paramDeclPointBySym
}

func pointDominates(graph *cfg.Graph) func(cfg.Point, cfg.Point) bool {
	idom := cfganalysis.ComputeImmediateDominators(graph)
	return func(a, b cfg.Point) bool {
		if a == b {
			return true
		}
		for cur := b; ; {
			parentPoint, ok := idom[cur]
			if !ok {
				return false
			}
			if parentPoint == a {
				return true
			}
			if parentPoint == cur {
				return false
			}
			cur = parentPoint
		}
	}
}

func iteratorContainerEvidence(src flow.AssignmentSource, localEvidence typ.Type) typ.Type {
	switch src.IteratorKind {
	case flow.IterateIndexed:
		return IndexedIteratorEvidence(src.VarIndex, localEvidence)
	case flow.IterateKeyed:
		return KeyedIteratorEvidence(src.VarIndex, localEvidence)
	}
	return nil
}
