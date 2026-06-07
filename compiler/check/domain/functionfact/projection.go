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

// StoreView is the canonical store-backed read surface for function facts. It
// keeps symbol ownership, parent graph-key resolution, synth mode, and type
// projection in one place so callers do not rebuild partial views.
type StoreView struct {
	store         api.StoreReader
	defaultParent *scope.State
}

// StoreProjection returns a normalized view over store-backed function facts.
func StoreProjection(store api.StoreReader, defaultParent *scope.State) StoreView {
	return StoreView{store: store, defaultParent: defaultParent}
}

// StoreSymbolView is one resolved function-fact product plus its owning key.
type StoreSymbolView struct {
	Fact api.FunctionFact
	Key  api.GraphKey
}

// StoreSymbolOwner is the canonical placement for a symbol's function-fact
// product. It can exist before the product has any facts.
type StoreSymbolOwner struct {
	Graph  *cfg.Graph
	Parent *scope.State
	Key    api.GraphKey
}

// Owner resolves sym to the parent graph-key product that owns its facts.
func (v StoreView) Owner(sym cfg.SymbolID) (StoreSymbolOwner, bool) {
	if v.store == nil || sym == 0 {
		return StoreSymbolOwner{}, false
	}
	ref := v.store.FunctionRefBySym(sym)
	if ref == nil {
		return StoreSymbolOwner{}, false
	}
	return v.GraphOwner(graphForRef(v.store, ref))
}

// GraphOwner resolves graph's parent-key function-fact product.
func (v StoreView) GraphOwner(graph *cfg.Graph) (StoreSymbolOwner, bool) {
	return storeOwnerForGraph(v.store, graph, v.defaultParent)
}

// Symbol resolves sym to the function-fact product owned by its parent graph.
func (v StoreView) Symbol(sym cfg.SymbolID, mode api.SynthMode) (StoreSymbolView, bool) {
	if v.store == nil || sym == 0 {
		return StoreSymbolView{}, false
	}
	ref := v.store.FunctionRefBySym(sym)
	if ref == nil {
		return StoreSymbolView{}, false
	}
	return v.GraphSymbol(graphForRef(v.store, ref), sym, mode)
}

// GraphSymbol resolves sym in graph's parent-key function-fact product.
func (v StoreView) GraphSymbol(graph *cfg.Graph, sym cfg.SymbolID, mode api.SynthMode) (StoreSymbolView, bool) {
	ff, owner, ok := storeFactForGraphInMode(v.store, graph, sym, v.defaultParent, mode)
	if !ok {
		return StoreSymbolView{}, false
	}
	return StoreSymbolView{Fact: ff, Key: owner.Key}, true
}

// Type projects a resolved function fact into one semantic view.
func (v StoreSymbolView) Type(projection Projection, mode api.SynthMode) typ.Type {
	return ProjectType(v.Fact, projection, mode)
}

// Returns projects a resolved function fact's visible return vector.
func (v StoreSymbolView) Returns(mode api.SynthMode) []typ.Type {
	return returnsForMode(v.Fact, mode)
}

// TypeLookup returns a store-backed function projection lookup for solved-state
// observers.
func (v StoreView) TypeLookup(projection Projection, mode api.SynthMode) func(cfg.SymbolID) typ.Type {
	if v.store == nil {
		return nil
	}
	return func(sym cfg.SymbolID) typ.Type {
		sv, ok := v.Symbol(sym, mode)
		if !ok {
			return nil
		}
		return sv.Type(projection, mode)
	}
}

// FactsView is the canonical read surface for one in-memory FunctionFacts
// product. It gives product-local consumers the same vocabulary as StoreView
// without repeating lookup and projection policy at call sites.
type FactsView struct {
	facts api.FunctionFacts
}

// FactsProjection returns a normalized view over an in-memory FunctionFacts product.
func FactsProjection(facts api.FunctionFacts) FactsView {
	return FactsView{facts: facts}
}

// FactSymbolView is one resolved function-fact product.
type FactSymbolView struct {
	Fact api.FunctionFact
}

// Symbol resolves sym inside this function-fact product.
func (v FactsView) Symbol(sym cfg.SymbolID) (FactSymbolView, bool) {
	ff, ok := lookupStored(v.facts, sym)
	if !ok {
		return FactSymbolView{}, false
	}
	return FactSymbolView{Fact: ff}, true
}

// Type projects sym into one semantic function view.
func (v FactsView) Type(sym cfg.SymbolID, projection Projection, mode api.SynthMode) typ.Type {
	sv, ok := v.Symbol(sym)
	if !ok {
		return nil
	}
	return sv.Type(projection, mode)
}

// Returns projects sym's visible return vector in mode.
func (v FactsView) Returns(sym cfg.SymbolID, mode api.SynthMode) []typ.Type {
	sv, ok := v.Symbol(sym)
	if !ok {
		return nil
	}
	return sv.Returns(mode)
}

// BodyEntryEvidence returns sym's observed entry evidence.
func (v FactsView) BodyEntryEvidence(sym cfg.SymbolID) []typ.Type {
	sv, ok := v.Symbol(sym)
	if !ok {
		return nil
	}
	return sv.BodyEntryEvidence()
}

// BodyContractEvidence returns sym's body contract evidence.
func (v FactsView) BodyContractEvidence(sym cfg.SymbolID) []typ.Type {
	sv, ok := v.Symbol(sym)
	if !ok {
		return nil
	}
	return sv.BodyContractEvidence()
}

// PublicParameterEvidence returns sym's public caller parameter evidence.
func (v FactsView) PublicParameterEvidence(sym cfg.SymbolID) []typ.Type {
	sv, ok := v.Symbol(sym)
	if !ok {
		return nil
	}
	return sv.PublicParameterEvidence()
}

// ReturnSummary returns sym's declared/pre-flow return summary projection.
func (v FactsView) ReturnSummary(sym cfg.SymbolID) []typ.Type {
	sv, ok := v.Symbol(sym)
	if !ok {
		return nil
	}
	return sv.ReturnSummary()
}

// NarrowSummary returns sym's post-flow return summary projection.
func (v FactsView) NarrowSummary(sym cfg.SymbolID) []typ.Type {
	sv, ok := v.Symbol(sym)
	if !ok {
		return nil
	}
	return sv.NarrowSummary()
}

// Refinement returns sym's function refinement.
func (v FactsView) Refinement(sym cfg.SymbolID) *constraint.FunctionRefinement {
	sv, ok := v.Symbol(sym)
	if !ok {
		return nil
	}
	return sv.Refinement()
}

// SynthesisType is the default function-fact projection for expression
// synthesis in a requested mode.
func (v FactsView) SynthesisType(sym cfg.SymbolID, mode api.SynthMode) typ.Type {
	if mode == api.SynthModeFlow {
		return v.Type(sym, ProjectionSibling, mode)
	}
	return v.Type(sym, ProjectionFlowInput, mode)
}

// TypeLookup returns a named function type projection function.
func (v FactsView) TypeLookup(projection Projection, mode api.SynthMode) func(cfg.SymbolID) typ.Type {
	if len(v.facts) == 0 {
		return nil
	}
	return func(sym cfg.SymbolID) typ.Type {
		return v.Type(sym, projection, mode)
	}
}

// Type projects a resolved fact into one semantic function view.
func (v FactSymbolView) Type(projection Projection, mode api.SynthMode) typ.Type {
	return projectTypeNormalized(v.Fact, projection, mode)
}

// Returns projects a resolved fact's visible return vector in mode.
func (v FactSymbolView) Returns(mode api.SynthMode) []typ.Type {
	return returnsForMode(v.Fact, mode)
}

// BodyEntryEvidence returns observed entry evidence used to interpret a body.
func (v FactSymbolView) BodyEntryEvidence() []typ.Type {
	return BodyEntryEvidence(v.Fact)
}

// BodyContractEvidence returns body contract evidence without entry specialization.
func (v FactSymbolView) BodyContractEvidence() []typ.Type {
	return bodyParamsTypes(v.Fact)
}

// PublicParameterEvidence returns public caller parameter evidence.
func (v FactSymbolView) PublicParameterEvidence() []typ.Type {
	return paramsTypes(v.Fact)
}

// ReturnSummary returns the declared/pre-flow return summary projection.
func (v FactSymbolView) ReturnSummary() []typ.Type {
	return summaryTypes(v.Fact)
}

// NarrowSummary returns the post-flow return summary projection.
func (v FactSymbolView) NarrowSummary() []typ.Type {
	return narrowTypes(v.Fact)
}

// Refinement returns the canonical refinement projection.
func (v FactSymbolView) Refinement() *constraint.FunctionRefinement {
	return v.Fact.Refinement
}

// SignatureWithReturnSummary applies the canonical pre-flow return projection
// for sym to fn when that summary exists.
func SignatureWithReturnSummary(facts api.FunctionFacts, sym cfg.SymbolID, fn *typ.Function) *typ.Function {
	if fn == nil || len(facts) == 0 || sym == 0 {
		return fn
	}
	summary := FactsProjection(facts).Returns(sym, api.SynthModeDeclared)
	if len(summary) == 0 {
		return fn
	}
	return returnsummary.ApplyToFunctionType(fn, summary)
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
		return typejoin.WithReturnsOrUnknown(fn, FactsProjection(facts).Returns(sym, mode))
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
		fn = BodyInputProjection(fn, nil, entryParamsTypes(ff))
	case ProjectionSibling:
		publicParams := paramsTypes(ff)
		entryParams := entryParamsTypes(ff)
		fn = ApplyBodySignatureEvidence(fn, paramevidence.ApplyBodyContractsToEntries(publicParams, entryParams))
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

// BodyEntryEvidence returns the observed entry state used to interpret a
// function body. Body contracts are obligations and are intentionally excluded
// to avoid treating consumer requirements as producer proof.
func BodyEntryEvidence(ff api.FunctionFact) []typ.Type {
	ff = Normalize(ff)
	return entryParamsTypes(ff)
}

// SiblingParameterEvidence returns the same-scope caller parameter view:
// public caller obligations applied to observed local entry states. This keeps
// public/export projections broad while letting local closed-world calls use
// the entry shapes they proved.
func SiblingParameterEvidence(ff api.FunctionFact) []typ.Type {
	ff = Normalize(ff)
	return paramevidence.ApplyBodyContractsToEntries(paramsTypes(ff), entryParamsTypes(ff))
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

func storeFactForGraphInMode(store api.StoreReader, graph *cfg.Graph, sym cfg.SymbolID, defaultParent *scope.State, mode api.SynthMode) (api.FunctionFact, StoreSymbolOwner, bool) {
	if store == nil || graph == nil || sym == 0 {
		return api.FunctionFact{}, StoreSymbolOwner{}, false
	}
	owner, ok := storeOwnerForGraph(store, graph, defaultParent)
	if !ok {
		return api.FunctionFact{}, StoreSymbolOwner{}, false
	}
	var ff api.FunctionFact
	var found bool
	load := func() {
		ff, found = store.InterprocFacts(owner.Graph, owner.Parent).FunctionFact(sym)
	}
	if switcher, ok := store.(interface{ WithSynthMode(api.SynthMode, func()) }); ok {
		switcher.WithSynthMode(mode, load)
	} else {
		load()
	}
	if !found || Empty(ff) {
		return api.FunctionFact{}, StoreSymbolOwner{}, false
	}
	return ff, owner, true
}

func storeOwnerForGraph(store api.StoreReader, graph *cfg.Graph, defaultParent *scope.State) (StoreSymbolOwner, bool) {
	if store == nil || graph == nil {
		return StoreSymbolOwner{}, false
	}
	parent := api.ParentScopeForGraph(store, graph.ID(), defaultParent)
	if parent == nil {
		return StoreSymbolOwner{}, false
	}
	key, ok := store.GraphKeyFor(graph, parent)
	if !ok {
		return StoreSymbolOwner{}, false
	}
	return StoreSymbolOwner{Graph: graph, Parent: parent, Key: key}, true
}

// RefinementsFromStore projects canonical function facts as refinement facts.
func RefinementsFromStore(store api.StoreReader, defaultParent *scope.State) api.RefinementFacts {
	if store == nil {
		return nil
	}
	view := StoreProjection(store, defaultParent)
	return api.NewRefinementFacts(func(sym cfg.SymbolID) *constraint.FunctionRefinement {
		sv, ok := view.Symbol(sym, api.SynthModeDeclared)
		if !ok {
			return nil
		}
		return sv.Fact.Refinement
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
