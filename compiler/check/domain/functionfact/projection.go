package functionfact

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/compiler/check/domain/returnsummary"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
	typejoin "github.com/wippyai/go-lua/types/typ/join"
)

// Projection names the semantic function view requested from the product.
type Projection uint8

const (
	// ProjectionBody initializes/interprets a function body from source
	// annotations and observed entry evidence. Body contracts are obligations,
	// not assumptions for the same body.
	ProjectionBody Projection = iota
	// ProjectionFlowInput types function values while extracting abstract
	// interpreter inputs. It is a caller view with pre-flow returns; body,
	// entry, and narrowed evidence are deliberately invisible.
	ProjectionFlowInput
	// ProjectionPublic checks a caller against a public callable contract.
	ProjectionPublic
	// ProjectionExport exposes a module boundary contract.
	ProjectionExport
	// ProjectionSibling types same-scope sibling function values. It is a
	// closed-world local caller view: observed entry states are visible, while
	// body-only contracts remain caller obligations.
	ProjectionSibling
)

// ForSymbol returns the canonical stored function fact for sym.
func ForSymbol(store api.StoreReader, sym cfg.SymbolID, defaultParent *scope.State) (api.FunctionFact, bool) {
	if store == nil || sym == 0 {
		return api.FunctionFact{}, false
	}
	ref := store.FunctionRefBySym(sym)
	if ref == nil {
		return api.FunctionFact{}, false
	}
	return FactForGraph(store, graphForRef(store, ref), sym, defaultParent)
}

// BodyTypeForSymbol returns the body-view function projection for sym.
func BodyTypeForSymbol(store api.StoreReader, sym cfg.SymbolID, defaultParent *scope.State) typ.Type {
	if store == nil || sym == 0 {
		return nil
	}
	ref := store.FunctionRefBySym(sym)
	if ref == nil {
		return nil
	}
	return BodyTypeForGraph(store, graphForRef(store, ref), sym, defaultParent)
}

// SiblingTypeForSymbol returns the same-scope sibling function projection for sym.
func SiblingTypeForSymbol(store api.StoreReader, sym cfg.SymbolID, defaultParent *scope.State) typ.Type {
	if store == nil || sym == 0 {
		return nil
	}
	ref := store.FunctionRefBySym(sym)
	if ref == nil {
		return nil
	}
	return SiblingTypeForGraph(store, graphForRef(store, ref), sym, defaultParent)
}

// StoreProjectionLookup returns a store-backed function projection lookup for
// solved-state observers.
func StoreProjectionLookup(store api.StoreReader, projection Projection, mode api.SynthMode, defaultParent *scope.State) func(cfg.SymbolID) typ.Type {
	if store == nil {
		return nil
	}
	return func(sym cfg.SymbolID) typ.Type {
		if sym == 0 {
			return nil
		}
		ref := store.FunctionRefBySym(sym)
		if ref == nil {
			return nil
		}
		ff, ok := FactForGraph(store, graphForRef(store, ref), sym, defaultParent)
		if !ok {
			return nil
		}
		return ProjectType(ff, projection, mode)
	}
}

// ReturnSummaryForSymbol returns the canonical declared/pre-flow return summary
// for sym from its owning function-fact product.
func ReturnSummaryForSymbol(store api.StoreReader, sym cfg.SymbolID, defaultParent *scope.State) []typ.Type {
	ff, ok := factForSymbolInMode(store, sym, defaultParent, api.SynthModeDeclared)
	if !ok {
		return nil
	}
	return summaryTypes(ff)
}

// NarrowSummaryForSymbol returns the canonical post-flow return summary for sym
// from its owning function-fact product.
func NarrowSummaryForSymbol(store api.StoreReader, sym cfg.SymbolID, defaultParent *scope.State) []typ.Type {
	ff, ok := factForSymbolInMode(store, sym, defaultParent, api.SynthModeFlow)
	if !ok {
		return nil
	}
	return narrowTypes(ff)
}

// GraphKeyForSymbol returns the canonical parent graph key that owns sym's
// function-fact product.
func GraphKeyForSymbol(store api.StoreReader, sym cfg.SymbolID, defaultParent *scope.State) (api.GraphKey, bool) {
	if store == nil || sym == 0 {
		return api.GraphKey{}, false
	}
	ref := store.FunctionRefBySym(sym)
	if ref == nil {
		return api.GraphKey{}, false
	}
	graph := graphForRef(store, ref)
	if graph == nil {
		return api.GraphKey{}, false
	}
	parent := api.ParentScopeForGraph(store, graph.ID(), defaultParent)
	if parent == nil {
		return api.GraphKey{}, false
	}
	return store.GraphKeyFor(graph, parent)
}

// BodyTypeForGraph returns the body-view function projection for sym from
// graph's function-fact product.
func BodyTypeForGraph(store api.StoreReader, graph *cfg.Graph, sym cfg.SymbolID, defaultParent *scope.State) typ.Type {
	ff, ok := FactForGraph(store, graph, sym, defaultParent)
	if !ok {
		return nil
	}
	return ProjectType(ff, ProjectionBody, api.SynthModeDeclared)
}

// SiblingTypeForGraph returns the same-scope sibling function projection for
// sym from graph's function-fact product.
func SiblingTypeForGraph(store api.StoreReader, graph *cfg.Graph, sym cfg.SymbolID, defaultParent *scope.State) typ.Type {
	ff, ok := FactForGraph(store, graph, sym, defaultParent)
	if !ok {
		return nil
	}
	return ProjectType(ff, ProjectionSibling, api.SynthModeDeclared)
}

// ReturnSummaryForGraph returns the canonical declared/pre-flow return summary
// for sym from graph's function-fact product.
func ReturnSummaryForGraph(store api.StoreReader, graph *cfg.Graph, sym cfg.SymbolID, defaultParent *scope.State) []typ.Type {
	ff, ok := factForGraphInMode(store, graph, sym, defaultParent, api.SynthModeDeclared)
	if !ok {
		return nil
	}
	return summaryTypes(ff)
}

// NarrowSummaryForGraph returns the canonical post-flow return summary for sym
// from graph's function-fact product.
func NarrowSummaryForGraph(store api.StoreReader, graph *cfg.Graph, sym cfg.SymbolID, defaultParent *scope.State) []typ.Type {
	ff, ok := factForGraphInMode(store, graph, sym, defaultParent, api.SynthModeFlow)
	if !ok {
		return nil
	}
	return narrowTypes(ff)
}

// ReturnProjection returns the return vector visible in mode.
func ReturnProjection(facts api.FunctionFacts, sym cfg.SymbolID, mode api.SynthMode) []typ.Type {
	ff, ok := Lookup(facts, sym)
	if !ok {
		return nil
	}
	return returnsForMode(ff, mode)
}

// BodyTypeProjection returns the body-view function type projection.
func BodyTypeProjection(facts api.FunctionFacts, sym cfg.SymbolID, mode api.SynthMode) typ.Type {
	ff, ok := lookupStored(facts, sym)
	if !ok {
		return nil
	}
	return projectTypeNormalized(ff, ProjectionBody, mode)
}

// FlowInputTypeProjection returns the function view allowed while extracting
// abstract-interpreter inputs.
func FlowInputTypeProjection(facts api.FunctionFacts, sym cfg.SymbolID, mode api.SynthMode) typ.Type {
	ff, ok := lookupStored(facts, sym)
	if !ok {
		return nil
	}
	return projectTypeNormalized(ff, ProjectionFlowInput, mode)
}

// PublicTypeProjection returns the caller-facing public function type projection.
func PublicTypeProjection(facts api.FunctionFacts, sym cfg.SymbolID, mode api.SynthMode) typ.Type {
	ff, ok := lookupStored(facts, sym)
	if !ok {
		return nil
	}
	return projectTypeNormalized(ff, ProjectionPublic, mode)
}

// ExportTypeProjection returns the module-boundary function type projection.
func ExportTypeProjection(facts api.FunctionFacts, sym cfg.SymbolID, mode api.SynthMode) typ.Type {
	ff, ok := lookupStored(facts, sym)
	if !ok {
		return nil
	}
	return projectTypeNormalized(ff, ProjectionExport, mode)
}

// SiblingTypeProjection returns the same-scope sibling function type projection.
func SiblingTypeProjection(facts api.FunctionFacts, sym cfg.SymbolID, mode api.SynthMode) typ.Type {
	ff, ok := lookupStored(facts, sym)
	if !ok {
		return nil
	}
	return projectTypeNormalized(ff, ProjectionSibling, mode)
}

// SynthesisTypeProjection is the default function-fact projection for expression
// synthesis in a requested mode.
func SynthesisTypeProjection(facts api.FunctionFacts, sym cfg.SymbolID, mode api.SynthMode) typ.Type {
	if mode == api.SynthModeFlow {
		return SiblingTypeProjection(facts, sym, mode)
	}
	return FlowInputTypeProjection(facts, sym, mode)
}

// SignatureWithReturnSummary applies the canonical pre-flow return projection
// for sym to fn when that summary exists.
func SignatureWithReturnSummary(facts api.FunctionFacts, sym cfg.SymbolID, fn *typ.Function) *typ.Function {
	if fn == nil || len(facts) == 0 || sym == 0 {
		return fn
	}
	summary := ReturnSummary(facts, sym)
	if len(summary) == 0 {
		return fn
	}
	return returnsummary.ApplyToFunctionType(fn, summary)
}

// ProjectionLookup returns a named function type projection function.
func ProjectionLookup(facts api.FunctionFacts, projection Projection, mode api.SynthMode) func(cfg.SymbolID) typ.Type {
	if len(facts) == 0 {
		return nil
	}
	return func(sym cfg.SymbolID) typ.Type {
		if sym == 0 {
			return nil
		}
		ff, ok := lookupStored(facts, sym)
		if !ok {
			return nil
		}
		return projectTypeNormalized(ff, projection, mode)
	}
}

// RecursiveTypeProjection returns the read-only function projection used when
// function type synthesis re-enters the same function equation. It composes the
// source signature, contextual expected returns, and current FunctionFact return
// product without analyzing the body or writing any fact channel.
func RecursiveTypeProjection(
	signature *typ.Function,
	expected *typ.Function,
	facts api.FunctionFacts,
	sym cfg.SymbolID,
	mode api.SynthMode,
) *typ.Function {
	if signature == nil {
		return nil
	}
	fn := BodyInputProjection(signature, expected, nil)
	if expected != nil {
		if len(fn.Returns) == 0 && len(expected.Returns) > 0 {
			fn = typejoin.WithReturns(fn, expected.Returns)
		}
	}
	if sym != 0 {
		return typejoin.WithReturnsOrUnknown(fn, ReturnProjection(facts, sym, mode))
	}
	return typejoin.WithReturnsOrUnknown(fn, nil)
}

func expectedParameterEvidence(expected *typ.Function) []typ.Type {
	if expected == nil || len(expected.Params) == 0 {
		return nil
	}
	out := make([]typ.Type, len(expected.Params))
	for i, param := range expected.Params {
		out[i] = param.Type
	}
	return out
}

// BodyInputProjection derives the function signature used to initialize a
// function body's abstract state. Source annotations, contextual expected
// parameter types, and observed entry evidence are composed here so body
// interpretation has one owner for parameter precision.
func BodyInputProjection(signature *typ.Function, expected *typ.Function, entryEvidence []typ.Type) *typ.Function {
	if signature == nil {
		return nil
	}
	fn := signature
	if expected != nil {
		evidence := expectedParameterEvidence(expected)
		fn = CloseGenericBodySignature(fn, evidence)
		fn = ApplyBodySignatureEvidence(fn, evidence)
	}
	if len(entryEvidence) > 0 {
		fn = CloseGenericBodySignature(fn, entryEvidence)
		fn = ApplyBodySignatureEvidence(fn, entryEvidence)
	}
	return fn
}

// ProjectType derives a mode-specific function type from one stored product.
func ProjectType(ff api.FunctionFact, projection Projection, mode api.SynthMode) typ.Type {
	ff = Normalize(ff)
	return projectTypeNormalized(ff, projection, mode)
}

func projectTypeNormalized(ff api.FunctionFact, projection Projection, mode api.SynthMode) typ.Type {
	fn := ff.Signature
	if fn == nil {
		return nil
	}
	// A non-nilable hard public obligation proves the parameter is used as a
	// definitely non-nil value, so an unannotated parameter seeded optional is
	// not optional in any projection.
	fn = ClearOptionalForNonNilableObligation(fn, paramsTypes(ff))
	switch projection {
	case ProjectionBody:
		fn = BodyInputProjection(fn, nil, bodyEntryEvidenceNormalized(ff))
	case ProjectionSibling:
		fn = ApplyBodySignatureEvidence(fn, siblingParameterEvidenceNormalized(ff))
	case ProjectionFlowInput, ProjectionPublic, ProjectionExport:
		fn = ApplyPublicSignatureEvidence(fn, paramevidence.PublicSignatureVector(paramsTypes(ff)))
	}
	returns := returnsForMode(ff, mode)
	if len(fn.Returns) > 0 {
		returns = nil
	}
	if len(returns) == 0 {
		returns = returnsummary.Canonical(fn.Returns)
	}
	if len(returns) > 0 {
		if projectionDemotesInferredDynamicReturns(projection, fn) {
			// The module-export boundary cannot vouch for any inferred dynamic-any
			// nested in a structured return, so it demotes every any leaf. A
			// within-compilation public caller view keeps proven container/record
			// structure and only demotes a whole-slot bare any outcome, so an
			// inferred any[] or {field: any} stays precise for sibling and captured
			// local callers instead of collapsing to unknown.
			if projection == ProjectionExport {
				returns = returnsummary.DemoteInferredDynamicAny(returns)
			} else {
				returns = returnsummary.DemoteInferredDynamicAnySlot(returns)
			}
		}
		returns = preserveDeclaredDynamicReturns(fn.Returns, returns)
		if withReturns := returnsummary.ApplyToFunctionType(fn, returns); withReturns != nil {
			fn = withReturns
		}
	}
	if projection != ProjectionBody && len(ff.EnvReturns) > 0 {
		fn = withEnvReturns(fn, ff.EnvReturns)
	}
	if ff.Refinement != nil {
		fn = withRefinement(fn, ff.Refinement)
	}
	return fn
}

func projectionDemotesInferredDynamicReturns(projection Projection, fn *typ.Function) bool {
	switch projection {
	case ProjectionPublic, ProjectionExport:
	default:
		return false
	}
	return fn == nil || len(fn.Returns) == 0
}

func preserveDeclaredDynamicReturns(declared, projected []typ.Type) []typ.Type {
	if len(declared) == 0 || len(projected) == 0 {
		return projected
	}
	maxLen := len(declared)
	if len(projected) < maxLen {
		maxLen = len(projected)
	}
	var out []typ.Type
	for i := 0; i < maxLen; i++ {
		if !typ.IsUnknown(declared[i]) || projected[i] == nil || typ.IsUnknown(projected[i]) {
			continue
		}
		if out == nil {
			out = make([]typ.Type, len(projected))
			copy(out, projected)
		}
		out[i] = declared[i]
	}
	if out != nil {
		return out
	}
	return projected
}

// PublicParameterEvidence returns the public caller parameter evidence.
func PublicParameterEvidence(facts api.FunctionFacts, sym cfg.SymbolID) []typ.Type {
	ff, ok := Lookup(facts, sym)
	if !ok {
		return nil
	}
	return paramsTypes(ff)
}

// BodyEntryEvidence returns the observed entry state used to interpret a
// function body. Body contracts are obligations and are intentionally excluded
// to avoid treating consumer requirements as producer proof.
func BodyEntryEvidence(ff api.FunctionFact) []typ.Type {
	ff = Normalize(ff)
	return bodyEntryEvidenceNormalized(ff)
}

func bodyEntryEvidenceNormalized(ff api.FunctionFact) []typ.Type {
	return entryParamsTypes(ff)
}

// SiblingParameterEvidence returns the same-scope caller parameter view:
// public caller obligations applied to observed local entry states. This keeps
// public/export projections broad while letting local closed-world calls use
// the entry shapes they proved.
func SiblingParameterEvidence(ff api.FunctionFact) []typ.Type {
	ff = Normalize(ff)
	return siblingParameterEvidenceNormalized(ff)
}

func siblingParameterEvidenceNormalized(ff api.FunctionFact) []typ.Type {
	return paramevidence.ApplyBodyContractsToEntries(paramsTypes(ff), entryParamsTypes(ff))
}

// BodyEntryEvidenceForSymbol returns the observed entry evidence used to
// interpret sym's body.
func BodyEntryEvidenceForSymbol(facts api.FunctionFacts, sym cfg.SymbolID) []typ.Type {
	ff, ok := Lookup(facts, sym)
	if !ok {
		return nil
	}
	return BodyEntryEvidence(ff)
}

// BodyContractEvidence returns body contract evidence without observed
// call-entry specialization.
func BodyContractEvidence(facts api.FunctionFacts, sym cfg.SymbolID) []typ.Type {
	ff, ok := Lookup(facts, sym)
	if !ok {
		return nil
	}
	return bodyParamsTypes(ff)
}

// ReturnSummary returns the canonical declared/pre-flow return summary
// projection.
func ReturnSummary(facts api.FunctionFacts, sym cfg.SymbolID) []typ.Type {
	ff, ok := Lookup(facts, sym)
	if !ok {
		return nil
	}
	return summaryTypes(ff)
}

// NarrowSummary returns the canonical post-flow return summary projection.
func NarrowSummary(facts api.FunctionFacts, sym cfg.SymbolID) []typ.Type {
	ff, ok := Lookup(facts, sym)
	if !ok {
		return nil
	}
	return narrowTypes(ff)
}

// Refinement returns the canonical refinement projection.
func Refinement(facts api.FunctionFacts, sym cfg.SymbolID) *constraint.FunctionRefinement {
	ff, ok := Lookup(facts, sym)
	if !ok {
		return nil
	}
	return ff.Refinement
}

// Lookup returns the canonical stored function fact for sym from facts.
func Lookup(facts api.FunctionFacts, sym cfg.SymbolID) (api.FunctionFact, bool) {
	ff, ok := lookupStored(facts, sym)
	if !ok {
		return api.FunctionFact{}, false
	}
	return ff, !Empty(ff)
}

func lookupStored(facts api.FunctionFacts, sym cfg.SymbolID) (api.FunctionFact, bool) {
	if len(facts) == 0 || sym == 0 {
		return api.FunctionFact{}, false
	}
	ff, ok := facts[sym]
	if !ok {
		return api.FunctionFact{}, false
	}
	return ff, !Empty(ff)
}

// FactForGraph returns the canonical stored function fact for sym from graph's
// function-fact product.
func FactForGraph(store api.StoreReader, graph *cfg.Graph, sym cfg.SymbolID, defaultParent *scope.State) (api.FunctionFact, bool) {
	return factForGraphInMode(store, graph, sym, defaultParent, api.SynthModeDeclared)
}

func factForSymbolInMode(store api.StoreReader, sym cfg.SymbolID, defaultParent *scope.State, mode api.SynthMode) (api.FunctionFact, bool) {
	if store == nil || sym == 0 {
		return api.FunctionFact{}, false
	}
	ref := store.FunctionRefBySym(sym)
	if ref == nil {
		return api.FunctionFact{}, false
	}
	return factForGraphInMode(store, graphForRef(store, ref), sym, defaultParent, mode)
}

func factForGraphInMode(store api.StoreReader, graph *cfg.Graph, sym cfg.SymbolID, defaultParent *scope.State, mode api.SynthMode) (api.FunctionFact, bool) {
	if store == nil || graph == nil || sym == 0 {
		return api.FunctionFact{}, false
	}
	parent := api.ParentScopeForGraph(store, graph.ID(), defaultParent)
	if parent == nil {
		return api.FunctionFact{}, false
	}
	var ff api.FunctionFact
	var found bool
	load := func() {
		ff, found = store.InterprocFacts(graph, parent).FunctionFact(sym)
	}
	if switcher, ok := store.(interface{ WithSynthMode(api.SynthMode, func()) }); ok {
		switcher.WithSynthMode(mode, load)
	} else {
		load()
	}
	if !found || Empty(ff) {
		return api.FunctionFact{}, false
	}
	return ff, true
}

// RefinementsFromStore projects canonical function facts as refinement facts.
func RefinementsFromStore(store api.StoreReader, defaultParent *scope.State) api.RefinementFacts {
	if store == nil {
		return nil
	}
	return api.NewRefinementFacts(func(sym cfg.SymbolID) *constraint.FunctionRefinement {
		ff, ok := ForSymbol(store, sym, defaultParent)
		if !ok {
			return nil
		}
		return ff.Refinement
	})
}

func returnsForMode(ff api.FunctionFact, mode api.SynthMode) []typ.Type {
	if mode == api.SynthModeFlow && len(ff.Narrow) > 0 {
		return repairSummaryWithNarrow(summaryTypes(ff), narrowTypes(ff))
	}
	return summaryTypes(ff)
}

func withRefinement(fn *typ.Function, refinement *constraint.FunctionRefinement) *typ.Function {
	if fn == nil || refinement == nil {
		return fn
	}
	if fn.Refinement == refinement && functionEffectsIncludeRefinementRow(fn, refinement) {
		return fn
	}
	appliedRefinement := typ.RefinementInfo(refinement)
	if fn.Refinement != nil && fn.Refinement != refinement {
		appliedRefinement = fn.Refinement
		if functionEffectsIncludeRefinementRow(fn, refinement) {
			return fn
		}
	}
	builder := typ.Func().ReserveParams(len(fn.Params))
	for _, tp := range fn.TypeParams {
		builder = builder.TypeParamRef(tp)
	}
	for _, p := range fn.Params {
		if p.Optional {
			builder = builder.OptParam(p.Name, p.Type)
		} else {
			builder = builder.Param(p.Name, p.Type)
		}
	}
	if fn.Variadic != nil {
		builder = builder.Variadic(fn.Variadic)
	}
	if len(fn.Returns) > 0 {
		builder = builder.Returns(fn.Returns...)
	}
	if effects := mergeFunctionEffects(fn.Effects, refinement.Row); effects != nil {
		builder = builder.Effects(effects)
	}
	if fn.Spec != nil {
		builder = builder.Spec(fn.Spec)
	}
	return builder.WithRefinement(appliedRefinement).Build()
}

func functionEffectsIncludeRefinementRow(fn *typ.Function, refinement *constraint.FunctionRefinement) bool {
	if fn == nil || refinement == nil || refinement.Row == nil {
		return true
	}
	base, ok := fn.Effects.(effect.Row)
	if !ok {
		return false
	}
	row, ok := refinement.Row.(effect.Row)
	return ok && effect.Union(base, row).Equals(base)
}

func mergeFunctionEffects(left, right typ.EffectInfo) typ.EffectInfo {
	base, _ := left.(effect.Row)
	if row, ok := right.(effect.Row); ok {
		base = effect.Union(base, row)
	}
	if base.Pure() && !base.IsOpen() {
		return left
	}
	return base
}

func withEnvReturns(fn *typ.Function, envReturns []contract.EnvReturnSpec) *typ.Function {
	if fn == nil || len(envReturns) == 0 {
		return fn
	}
	spec := cloneContractSpec(fn)
	merged := JoinEnvReturns(spec.EnvReturns, envReturns)
	if EnvReturnsEqual(spec.EnvReturns, merged) {
		return fn
	}
	spec.EnvReturns = merged
	return rebuildFunctionWithSpec(fn, spec)
}

func cloneContractSpec(fn *typ.Function) *contract.Spec {
	if fn == nil || fn.Spec == nil {
		return contract.NewSpec()
	}
	spec, ok := fn.Spec.(*contract.Spec)
	if !ok || spec == nil {
		return contract.NewSpec()
	}
	clone := contract.NewSpec()
	clone.Requires = spec.Requires
	clone.Ensures = spec.Ensures
	if len(spec.ExprRequires) > 0 {
		clone.ExprRequires = append([]constraint.ExprCompare(nil), spec.ExprRequires...)
	}
	if len(spec.ExprEnsures) > 0 {
		clone.ExprEnsures = append([]constraint.ExprCompare(nil), spec.ExprEnsures...)
	}
	clone.Effects = spec.Effects
	if len(spec.Callbacks) > 0 {
		clone.Callbacks = make(map[int]*contract.CallbackSpec, len(spec.Callbacks))
		for idx, cb := range spec.Callbacks {
			clone.Callbacks[idx] = cb.Clone()
		}
	}
	clone.Return = spec.Return
	clone.EnvReturns = NormalizeEnvReturns(spec.EnvReturns)
	return clone
}

// AttachReturnLengthEnsures appends proven return-length postconditions to a
// signature's contract spec. Each ExprCompare relates a return-length term to a
// constant or parameter-length term and is consumed at call sites to seed the
// caller's numeric length lower bound. An already-present postcondition is not
// duplicated, so the attach is idempotent and the spec stays Equal to itself
// once the body's proven bound settles, letting the interprocedural fixpoint
// converge.
func AttachReturnLengthEnsures(fn *typ.Function, ensures []constraint.ExprCompare) *typ.Function {
	if fn == nil || len(ensures) == 0 {
		return fn
	}
	spec := cloneContractSpec(fn)
	added := false
	for _, e := range ensures {
		if containsExprCompare(spec.ExprEnsures, e) {
			continue
		}
		spec.ExprEnsures = append(spec.ExprEnsures, e)
		added = true
	}
	if !added {
		return fn
	}
	return rebuildFunctionWithSpec(fn, spec)
}

func containsExprCompare(list []constraint.ExprCompare, target constraint.ExprCompare) bool {
	for _, e := range list {
		if e.Equals(target) {
			return true
		}
	}
	return false
}

func rebuildFunctionWithSpec(fn *typ.Function, spec *contract.Spec) *typ.Function {
	if fn == nil {
		return nil
	}
	builder := typ.Func().ReserveParams(len(fn.Params))
	for _, tp := range fn.TypeParams {
		builder = builder.TypeParamRef(tp)
	}
	for _, p := range fn.Params {
		if p.Optional {
			builder = builder.OptParam(p.Name, p.Type)
		} else {
			builder = builder.Param(p.Name, p.Type)
		}
	}
	if fn.Variadic != nil {
		builder = builder.Variadic(fn.Variadic)
	}
	if len(fn.Returns) > 0 {
		builder = builder.Returns(fn.Returns...)
	}
	if fn.Effects != nil {
		builder = builder.Effects(fn.Effects)
	}
	if spec != nil {
		builder = builder.Spec(spec)
	}
	if fn.Refinement != nil {
		builder = builder.WithRefinement(fn.Refinement)
	}
	return builder.Build()
}

func graphForRef(store api.StoreReader, ref *api.FunctionRef) *cfg.Graph {
	if store == nil || ref == nil {
		return nil
	}
	parentGraphID := ref.ParentGraphID
	if parentGraphID == 0 {
		parentGraphID = ref.GraphID
	}
	return store.Graphs()[parentGraphID]
}
