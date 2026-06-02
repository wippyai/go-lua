package functionfact

import (
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/callbackenv"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/compiler/check/domain/returnsummary"
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/join"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// Normalize canonicalizes one stored function fact. The vector carriers store
// interned product.AbstractValue; the rich per-slot engines run on the projected
// typ.Type evidence, and the canonical result is lifted back onto the carriers.
func Normalize(ff api.FunctionFact) api.FunctionFact {
	summary := returnsummary.NormalizeAndPrune(summaryTypes(ff))
	narrow := returnsummary.NormalizeAndPrune(narrowTypes(ff))
	signature := normalizeFunctionFactSignature(ff.Signature)
	summary = repairInferredSummaryWithNarrow(signature, summary, narrow)
	return api.FunctionFact{
		Params:      product.LiftVector(paramevidence.FilterEmptyVector(paramsTypes(ff))),
		BodyParams:  product.LiftVector(paramevidence.FilterEmptyBodyVector(bodyParamsTypes(ff))),
		EntryParams: product.LiftVector(paramevidence.FilterEmptyBodyVector(entryParamsTypes(ff))),
		Summary:     product.LiftVector(summary),
		Narrow:      product.LiftVector(narrow),
		Signature:   signature,
		Refinement:  NormalizeRefinement(ff.Refinement),
		EnvReturns:  NormalizeEnvReturns(ff.EnvReturns),
	}
}

func normalizeFunctionFactSignature(fn *typ.Function) *typ.Function {
	if fn == nil {
		return nil
	}
	changed := false
	builder := typ.Func().ReserveParams(len(fn.Params))
	for _, tp := range fn.TypeParams {
		builder = builder.TypeParamRef(tp)
	}
	for _, param := range fn.Params {
		paramType := normalizeFunctionParamDomainType(param.Type)
		if !typ.TypeEquals(param.Type, paramType) {
			changed = true
		}
		if param.Optional {
			builder = builder.OptParam(param.Name, paramType)
		} else {
			builder = builder.Param(param.Name, paramType)
		}
	}
	if fn.Variadic != nil {
		variadic := normalizeFunctionParamDomainType(fn.Variadic)
		if !typ.TypeEquals(fn.Variadic, variadic) {
			changed = true
		}
		builder = builder.Variadic(variadic)
	}
	if len(fn.Returns) > 0 {
		returns := make([]typ.Type, len(fn.Returns))
		for idx, ret := range fn.Returns {
			normalized := value.NormalizeFactType(ret)
			if !typ.TypeEquals(ret, normalized) {
				changed = true
			}
			returns[idx] = normalized
		}
		builder = builder.Returns(returns...)
	}
	if fn.Effects != nil {
		builder = builder.Effects(fn.Effects)
	}
	if fn.Spec != nil {
		builder = builder.Spec(fn.Spec)
	}
	if fn.Refinement != nil {
		builder = builder.WithRefinement(fn.Refinement)
	}
	if !changed {
		return fn
	}
	return builder.Build()
}

// CanonicalSourceSignature converts a synthesized function shape into the
// stored source-signature channel. Synthetic placeholder returns are omitted;
// declared return annotations remain part of the signature contract.
func CanonicalSourceSignature(fn *typ.Function, hasDeclaredReturns bool) *typ.Function {
	if fn == nil {
		return nil
	}
	if hasDeclaredReturns {
		return normalizeFunctionFactSignature(fn)
	}
	builder := typ.Func().ReserveParams(len(fn.Params))
	for _, tp := range fn.TypeParams {
		builder = builder.TypeParamRef(tp)
	}
	for _, param := range fn.Params {
		paramType := normalizeFunctionParamDomainType(param.Type)
		if param.Optional {
			builder = builder.OptParam(param.Name, paramType)
		} else {
			builder = builder.Param(param.Name, paramType)
		}
	}
	if fn.Variadic != nil {
		builder = builder.Variadic(normalizeFunctionParamDomainType(fn.Variadic))
	}
	if fn.Effects != nil {
		builder = builder.Effects(fn.Effects)
	}
	if fn.Spec != nil {
		builder = builder.Spec(fn.Spec)
	}
	if fn.Refinement != nil {
		builder = builder.WithRefinement(fn.Refinement)
	}
	return builder.Build()
}

// CanonicalPostflowSignature admits a synthesized postflow function shape into
// the canonical source-signature channel. Public seed parameters preserve the
// caller contract, inferred returns are kept only when source declarations make
// returns part of the signature, and synthetic variadics are removed for
// non-vararg source functions.
func CanonicalPostflowSignature(
	observed *typ.Function,
	publicSeed *typ.Function,
	returns []typ.Type,
	hasDeclaredReturns bool,
	hasSourceVariadic bool,
) *typ.Function {
	if observed == nil {
		return nil
	}
	candidate := observed
	if publicSeed != nil {
		candidate = withPublicSeedParams(candidate, publicSeed)
	}
	if hasDeclaredReturns && len(returns) > 0 && !returnsummary.AllNil(returns) {
		if aligned := join.WithReturns(candidate, returns); aligned != nil {
			candidate = aligned
		}
	}
	if !hasSourceVariadic {
		candidate = withoutSyntheticVariadic(candidate)
	}
	return CanonicalSourceSignature(candidate, hasDeclaredReturns)
}

func withPublicSeedParams(fn *typ.Function, publicSeed *typ.Function) *typ.Function {
	if fn == nil || publicSeed == nil || len(publicSeed.Params) == 0 {
		return fn
	}
	builder := typ.Func().ReserveParams(len(publicSeed.Params))
	for _, tp := range publicSeed.TypeParams {
		builder.TypeParamRef(tp)
	}
	for _, param := range publicSeed.Params {
		if param.Optional {
			builder.OptParam(param.Name, param.Type)
		} else {
			builder.Param(param.Name, param.Type)
		}
	}
	if publicSeed.Variadic != nil {
		builder.Variadic(publicSeed.Variadic)
	} else if fn.Variadic != nil {
		builder.Variadic(fn.Variadic)
	}
	if len(fn.Returns) > 0 {
		builder.Returns(fn.Returns...)
	}
	if fn.Effects != nil {
		builder.Effects(fn.Effects)
	}
	if fn.Spec != nil {
		builder.Spec(fn.Spec)
	}
	if fn.Refinement != nil {
		builder.WithRefinement(fn.Refinement)
	}
	return builder.Build()
}

func withoutSyntheticVariadic(fn *typ.Function) *typ.Function {
	if fn == nil || fn.Variadic == nil {
		return fn
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
	if len(fn.Returns) > 0 {
		builder = builder.Returns(fn.Returns...)
	}
	if fn.Effects != nil {
		builder = builder.Effects(fn.Effects)
	}
	if fn.Spec != nil {
		builder = builder.Spec(fn.Spec)
	}
	if fn.Refinement != nil {
		builder = builder.WithRefinement(fn.Refinement)
	}
	return builder.Build()
}

func normalizeFunctionParamDomainType(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	t = value.CollapseSequenceUnion(t, mergeParamType)
	t = value.CollapseStructuralUnionShape(t, mergeParamType)
	return value.CollapseTableTopEvidence(typ.PruneSoftUnionMembers(t))
}

// ApplyPublicSignatureEvidence makes the stored public function type agree with
// hard public parameter obligations from the canonical function-fact product.
// Explicit source annotations, including soft structural top annotations such
// as {any}, stay public-authoritative; observed call shapes refine only the
// body projection.
func ApplyPublicSignatureEvidence(fn *typ.Function, evidence []typ.Type) *typ.Function {
	if fn == nil || len(fn.Params) == 0 || len(evidence) == 0 {
		return fn
	}
	changed := false
	builder := typ.Func().ReserveParams(len(fn.Params))
	for _, tp := range fn.TypeParams {
		builder = builder.TypeParamRef(tp)
	}
	for i, param := range fn.Params {
		paramType := param.Type
		optional := param.Optional
		if i < len(evidence) {
			if candidate := evidence[i]; paramevidence.HardPublicEvidence(candidate) {
				next := applyPublicParamEvidence(param.Type, candidate)
				if !typ.TypeEquals(next, param.Type) {
					paramType = next
					changed = true
				}
				// A non-nilable hard obligation is derived only from an
				// unconditional, non-rebound use of the parameter, which proves the
				// parameter value is non-nil regardless of whether the obligation's
				// value type merges with the declared type. An unannotated parameter
				// seeded optional therefore drops its optionality.
				if optional && (!unwrap.IsOptionalLike(paramType) || nonNilableHardEvidence(candidate)) {
					optional = false
					changed = true
				}
			}
		}
		if optional {
			builder = builder.OptParam(param.Name, paramType)
		} else {
			builder = builder.Param(param.Name, paramType)
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
	if fn.Spec != nil {
		builder = builder.Spec(fn.Spec)
	}
	if fn.Refinement != nil {
		builder = builder.WithRefinement(fn.Refinement)
	}
	if !changed {
		return fn
	}
	return builder.Build()
}

// ApplyBodySignatureEvidence projects body-effective parameter evidence into a
// source signature for callee-body interpretation.
func ApplyBodySignatureEvidence(fn *typ.Function, evidence []typ.Type) *typ.Function {
	if fn == nil || len(fn.Params) == 0 || len(evidence) == 0 {
		return fn
	}
	changed := false
	builder := typ.Func().ReserveParams(len(fn.Params))
	for _, tp := range fn.TypeParams {
		builder = builder.TypeParamRef(tp)
	}
	for i, param := range fn.Params {
		paramType := param.Type
		if i < len(evidence) {
			next := applyBodyParamEvidence(param, evidence[i])
			if !typ.TypeEquals(next, param.Type) {
				paramType = next
				changed = true
			}
		}
		if param.Optional {
			builder = builder.OptParam(param.Name, paramType)
		} else {
			builder = builder.Param(param.Name, paramType)
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
	if fn.Spec != nil {
		builder = builder.Spec(fn.Spec)
	}
	if fn.Refinement != nil {
		builder = builder.WithRefinement(fn.Refinement)
	}
	if !changed {
		return fn
	}
	return builder.Build()
}

// nonNilableHardEvidence reports whether a public obligation requires a
// definitely non-nil parameter value.
func nonNilableHardEvidence(evidence typ.Type) bool {
	if evidence == nil {
		return false
	}
	_, nilable := value.SplitNilable(evidence)
	return !nilable
}

// ClearOptionalForNonNilableObligation drops the optional flag from any
// parameter whose public obligation is a non-nilable hard contract. Such an
// obligation is recorded only from an unconditional, non-rebound use, so it
// proves the parameter value is non-nil; an unannotated parameter that was
// seeded optional is therefore non-optional in every projection.
func ClearOptionalForNonNilableObligation(fn *typ.Function, obligations []typ.Type) *typ.Function {
	if fn == nil || len(fn.Params) == 0 || len(obligations) == 0 {
		return fn
	}
	changed := false
	builder := typ.Func().ReserveParams(len(fn.Params))
	for _, tp := range fn.TypeParams {
		builder = builder.TypeParamRef(tp)
	}
	for i, param := range fn.Params {
		optional := param.Optional
		if optional && i < len(obligations) &&
			paramevidence.HardPublicEvidence(obligations[i]) &&
			nonNilableHardEvidence(obligations[i]) {
			optional = false
			changed = true
		}
		if optional {
			builder = builder.OptParam(param.Name, param.Type)
		} else {
			builder = builder.Param(param.Name, param.Type)
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
	if fn.Spec != nil {
		builder = builder.Spec(fn.Spec)
	}
	if fn.Refinement != nil {
		builder = builder.WithRefinement(fn.Refinement)
	}
	if !changed {
		return fn
	}
	return builder.Build()
}

func applyPublicParamEvidence(paramType, evidence typ.Type) typ.Type {
	if evidence == nil {
		return paramType
	}
	if paramType == nil || typ.IsUnknown(paramType) || paramevidence.AnyLikeParam(paramType) {
		return mergeParamType(paramType, evidence)
	}
	if typ.IsRefinableAnnotation(paramType) {
		return paramType
	}
	if value.IsStructuredTableShape(unwrap.Optional(paramType)) &&
		value.IsStructuredTableShape(unwrap.Optional(evidence)) {
		// A declared annotation stays authoritative when the public evidence is
		// merely a widening of it (the annotation is already a subtype). Merging
		// would degrade a precise discriminated union to the evidence's
		// literal-widened shape, so refine only when the evidence adds precision.
		if subtype.IsSubtype(paramType, evidence) && !subtype.IsSubtype(evidence, paramType) {
			return paramType
		}
		return mergeParamType(paramType, evidence)
	}
	return paramType
}

func applyBodyParamEvidence(param typ.Param, evidence typ.Type) typ.Type {
	if evidence == nil {
		return param.Type
	}
	paramType := param.Type
	if unwrap.IsNilType(evidence) && (paramType == nil || typ.IsUnknown(paramType) || paramevidence.AnyLikeParam(paramType)) {
		return typ.Nil
	}
	if paramType == nil || typ.IsUnknown(paramType) || paramevidence.AnyLikeParam(paramType) {
		return mergeParamType(paramType, evidence)
	}
	if typ.IsRefinableAnnotation(paramType) {
		return paramevidence.RefineAnnotationWithEvidence(paramType, evidence)
	}
	if typ.IsSoft(paramType, typ.SoftAnnotationPolicy) {
		return mergeParamType(paramType, evidence)
	}
	if subtype.IsSubtype(evidence, paramType) {
		return evidence
	}
	if value.IsStructuredTableShape(unwrap.Optional(paramType)) &&
		value.IsStructuredTableShape(unwrap.Optional(evidence)) {
		return mergeParamType(paramType, evidence)
	}
	return paramType
}

// Empty reports whether a canonical function fact contains no information.
func Empty(ff api.FunctionFact) bool {
	return len(ff.Params) == 0 &&
		len(ff.BodyParams) == 0 &&
		len(ff.EntryParams) == 0 &&
		len(ff.Summary) == 0 &&
		len(ff.Narrow) == 0 &&
		ff.Signature == nil &&
		NormalizeRefinement(ff.Refinement) == nil &&
		len(NormalizeEnvReturns(ff.EnvReturns)) == 0
}

// Join precisely merges two observations for one local function during a single
// analysis iteration.
func Join(existing, candidate api.FunctionFact) api.FunctionFact {
	existing = Normalize(existing)
	candidate = Normalize(candidate)
	return JoinCanonical(existing, candidate)
}

// JoinCanonical precisely merges two already-canonical observations for one
// local function during a single analysis iteration.
func JoinCanonical(existing, candidate api.FunctionFact) api.FunctionFact {
	out := existing

	params := paramsTypes(existing)
	bodyParams := bodyParamsTypes(existing)
	entryParams := entryParamsTypes(existing)
	summary := summaryTypes(existing)
	narrow := narrowTypes(existing)

	if len(candidate.Params) > 0 {
		params = paramevidence.JoinCallVectors(params, paramsTypes(candidate))
	}
	if len(candidate.BodyParams) > 0 {
		bodyParams = paramevidence.JoinBodyVectors(bodyParams, bodyParamsTypes(candidate))
	}
	if len(candidate.EntryParams) > 0 {
		entryParams = paramevidence.JoinEntryVectors(entryParams, entryParamsTypes(candidate))
	}
	if len(candidate.Summary) > 0 {
		summary = returnsummary.Merge(summary, summaryTypes(candidate))
	}
	if len(candidate.Narrow) > 0 {
		narrow = returnsummary.Merge(narrow, narrowTypes(candidate))
	}
	if candidate.Signature != nil {
		out.Signature = MergeSignature(out.Signature, candidate.Signature)
	}
	out.Refinement = MergeRefinement(out.Refinement, candidate.Refinement)
	out.EnvReturns = JoinEnvReturns(out.EnvReturns, candidate.EnvReturns)
	if len(narrow) > 0 {
		if len(summary) == 0 {
			summary = returnsummary.Canonical(narrow)
		} else {
			summary = returnsummary.Merge(summary, narrow)
		}
	}
	summary = repairInferredSummaryWithNarrow(out.Signature, summary, narrow)

	out.Params = product.LiftVector(params)
	out.BodyParams = product.LiftVector(bodyParams)
	out.EntryParams = product.LiftVector(entryParams)
	out.Summary = product.LiftVector(summary)
	out.Narrow = product.LiftVector(narrow)

	return out
}

// MergeSignature merges source-level function signature facts. It preserves the
// function shape and source annotations; inferred returns remain in Summary/Narrow
// and are projected later.
func MergeSignature(existing, candidate *typ.Function) *typ.Function {
	if existing == nil {
		return candidate
	}
	if candidate == nil {
		return existing
	}
	return unwrap.Function(MergeType(existing, candidate))
}

// MergeSignatureContracts copies body-derived contract products, such as
// callback environment overlays, onto a source-shaped signature without
// admitting inferred parameter or return types through this path.
func MergeSignatureContracts(source, evidence *typ.Function) *typ.Function {
	if source == nil || evidence == nil || evidence.Spec == nil {
		return source
	}
	spec, ok := mergeFunctionSpec(source.Spec, evidence.Spec).(*contract.Spec)
	if !ok || spec == nil {
		return source
	}
	return rebuildFunctionWithSpec(source, spec)
}

// MergeExpectedSignature applies a contextual expected signature to a
// synthesized seed signature. This is a projection/admission policy for
// function shape, not a pipeline concern.
func MergeExpectedSignature(seed, expected *typ.Function) *typ.Function {
	if seed == nil {
		return expected
	}
	if expected == nil {
		return seed
	}
	if typ.TypeEquals(seed, expected) {
		return seed
	}

	builder := typ.Func().ReserveParams(maxInt(len(seed.Params), len(expected.Params)))
	if sameFunctionTypeParams(seed, expected) {
		for _, tp := range seed.TypeParams {
			builder = builder.TypeParamRef(tp)
		}
	}

	paramCount := maxInt(len(seed.Params), len(expected.Params))
	for i := 0; i < paramCount; i++ {
		name := ""
		var paramType typ.Type
		optional := true
		if i < len(seed.Params) {
			p := seed.Params[i]
			name = p.Name
			paramType = p.Type
			optional = p.Optional
		}
		if i < len(expected.Params) {
			p := expected.Params[i]
			if name == "" {
				name = p.Name
			}
			paramType = mergeExpectedParamType(paramType, p.Type, p.Optional)
			optional = p.Optional
		} else if expected.Variadic != nil {
			paramType = MergeParamType(paramType, expected.Variadic)
			optional = true
		} else if i >= len(expected.Params) {
			paramType = MergeParamType(paramType, typ.Nil)
			optional = true
		}
		if optional {
			builder = builder.OptParam(name, paramType)
		} else {
			builder = builder.Param(name, paramType)
		}
	}

	if seed.Variadic != nil || expected.Variadic != nil {
		builder = builder.Variadic(MergeParamType(seed.Variadic, expected.Variadic))
	}

	if returns := mergeSignatureReturns(seed.Returns, expected.Returns); len(returns) > 0 {
		builder = builder.Returns(returns...)
	}
	if seed.Effects != nil {
		builder = builder.Effects(seed.Effects)
	} else if expected.Effects != nil {
		builder = builder.Effects(expected.Effects)
	}
	if spec := mergeFunctionSpec(seed.Spec, expected.Spec); spec != nil {
		builder = builder.Spec(spec)
	}
	if seed.Refinement != nil {
		builder = builder.WithRefinement(seed.Refinement)
	} else if expected.Refinement != nil {
		builder = builder.WithRefinement(expected.Refinement)
	}
	return builder.Build()
}

func mergeExpectedParamType(seed, expected typ.Type, expectedOptional bool) typ.Type {
	if expected == nil {
		return seed
	}
	if seed == nil || typ.IsAny(seed) || typ.IsUnknown(seed) {
		return expected
	}
	if !expectedOptional {
		if inner, nilable := typ.SplitNilableFieldType(seed); nilable {
			if typ.TypeEquals(inner, expected) || subtype.IsSubtype(expected, inner) {
				return expected
			}
		}
		if subtype.IsSubtype(expected, seed) && !subtype.IsSubtype(seed, expected) {
			return expected
		}
	}
	return MergeParamType(seed, expected)
}

func mergeSignatureReturns(a, b []typ.Type) []typ.Type {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	out := make([]typ.Type, maxInt(len(a), len(b)))
	for i := range out {
		var left, right typ.Type
		if i < len(a) {
			left = a[i]
		}
		if i < len(b) {
			right = b[i]
		}
		out[i] = mergeExpectedReturnSlot(left, right)
	}
	return out
}

func mergeExpectedReturnSlot(seed, expected typ.Type) typ.Type {
	if seed == nil {
		return expected
	}
	if expected == nil {
		return seed
	}
	if typ.IsUnknown(seed) {
		return expected
	}
	if typ.IsUnknown(expected) {
		return seed
	}
	merged := returnsummary.Merge([]typ.Type{seed}, []typ.Type{expected})
	if len(merged) == 0 {
		return nil
	}
	return merged[0]
}

func sameFunctionTypeParams(a, b *typ.Function) bool {
	if a == nil || b == nil || len(a.TypeParams) != len(b.TypeParams) {
		return false
	}
	for i := range a.TypeParams {
		if a.TypeParams[i] == nil || b.TypeParams[i] == nil {
			if a.TypeParams[i] != b.TypeParams[i] {
				return false
			}
			continue
		}
		if a.TypeParams[i].Name != b.TypeParams[i].Name || !typ.TypeEquals(a.TypeParams[i].Constraint, b.TypeParams[i].Constraint) {
			return false
		}
	}
	return true
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func mergeFunctionSpec(existing, candidate typ.SpecInfo) typ.SpecInfo {
	if existing == nil {
		return candidate
	}
	if candidate == nil {
		return existing
	}
	if existing.Equals(candidate) {
		return existing
	}
	existingSpec, ok := existing.(*contract.Spec)
	if !ok {
		return existing
	}
	candidateSpec, ok := candidate.(*contract.Spec)
	if !ok {
		return existing
	}
	return mergeContractSpecs(existingSpec, candidateSpec)
}

func mergeContractSpecs(existing, candidate *contract.Spec) *contract.Spec {
	if existing == nil {
		return candidate
	}
	if candidate == nil {
		return existing
	}
	out := &contract.Spec{
		Requires:     existing.Requires,
		Ensures:      existing.Ensures,
		ExprRequires: mergeExprCompares(existing.ExprRequires, candidate.ExprRequires),
		ExprEnsures:  mergeExprCompares(existing.ExprEnsures, candidate.ExprEnsures),
		Effects:      existing.Effects,
		Callbacks:    make(map[int]*contract.CallbackSpec, len(existing.Callbacks)+len(candidate.Callbacks)),
		Return:       existing.Return,
		EnvReturns:   existing.GetEnvReturns(),
	}
	for idx, cb := range existing.Callbacks {
		out.Callbacks[idx] = cb.Clone()
	}
	for idx, cb := range candidate.Callbacks {
		out.Callbacks[idx] = mergeCallbackSpec(out.Callbacks[idx], cb)
	}
	return out
}

// mergeExprCompares is the lattice join of two arithmetic postcondition lists
// for one function symbol across analysis observations. Two return-length lower
// bounds on the same slot, len(ret_i) >= c1 and len(ret_i) >= c2, join to the
// weaker bound len(ret_i) >= min(c1, c2): the postcondition survives the join
// only as the floor proven by every observation. Joining to the weaker bound
// keeps the merge monotone in the descending direction even when a later
// observation proves a larger constant, so the spec reaches a fixed point and
// the interprocedural fixpoint converges. Other postconditions (e.g. stdlib
// equalities) carry no constant arm to weaken and are unioned with dedup.
func mergeExprCompares(existing, candidate []constraint.ExprCompare) []constraint.ExprCompare {
	if len(candidate) == 0 {
		return append([]constraint.ExprCompare(nil), existing...)
	}
	out := append([]constraint.ExprCompare(nil), existing...)
	for _, c := range candidate {
		if idx, ok := weakenableLowerBound(out, c); ok {
			cc := c.Right.(constraint.Const)
			ec := out[idx].Right.(constraint.Const)
			if cc.Value < ec.Value {
				out[idx].Right = constraint.C(cc.Value)
			}
			continue
		}
		duplicate := false
		for _, e := range out {
			if e.Equals(c) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			out = append(out, c)
		}
	}
	return out
}

// weakenableLowerBound finds an existing return-length lower bound on the same
// slot as c (len(ret_i) >= const, same relation), which the join weakens to the
// smaller constant. It matches only the constant-arm RetLen lower-bound shape so
// distinct postconditions on the same slot are not collapsed.
func weakenableLowerBound(list []constraint.ExprCompare, c constraint.ExprCompare) (int, bool) {
	idx, ok := retLenLowerBoundIndex(c)
	if !ok {
		return 0, false
	}
	for i, e := range list {
		if eIdx, ok := retLenLowerBoundIndex(e); ok && eIdx == idx && e.Rel == c.Rel {
			return i, true
		}
	}
	return 0, false
}

// retLenLowerBoundIndex reports the return slot of a len(ret_i) >= const
// postcondition, or ok=false for any other shape.
func retLenLowerBoundIndex(c constraint.ExprCompare) (int, bool) {
	if c.Rel != constraint.ExprGe {
		return 0, false
	}
	rl, ok := c.Left.(constraint.RetLen)
	if !ok {
		return 0, false
	}
	if _, ok := c.Right.(constraint.Const); !ok {
		return 0, false
	}
	return rl.Index, true
}

func mergeCallbackSpec(existing, candidate *contract.CallbackSpec) *contract.CallbackSpec {
	if existing == nil {
		return candidate.Clone()
	}
	if candidate == nil {
		return existing.Clone()
	}
	out := existing.Clone()
	if out.InputSource.Index == candidate.InputSource.Index {
		out.InputSource = candidate.InputSource
	}
	out.ReturnsBoolean = out.ReturnsBoolean || candidate.ReturnsBoolean
	out.Cardinality = mergeCallbackCardinality(out.Cardinality, candidate.Cardinality)
	out.Pure = out.Pure && candidate.Pure
	out.EnvOverlay = callbackenv.MergeContractOverlay(out.EnvOverlay, candidate.EnvOverlay)
	return out
}

func mergeCallbackCardinality(existing, candidate contract.Cardinality) contract.Cardinality {
	if existing == candidate {
		return existing
	}
	return contract.CardUnknown
}

// MergeType merges function-type facts through the canonical per-function fact
// policy.
func MergeType(existing, candidate typ.Type) typ.Type {
	if existing == nil {
		return candidate
	}
	if candidate == nil {
		return existing
	}

	existingFn := unwrap.Function(existing)
	candidateFn := unwrap.Function(candidate)
	if mergedFromVariants, ok := mergeVariants(existing, candidate); ok {
		return mergedFromVariants
	}
	if existingFn != nil && candidateFn != nil {
		if replacement, ok := value.FunctionEvidenceUpperBound(existingFn, candidateFn); ok {
			return replacement
		}
		if SameShape(existingFn, candidateFn) {
			return mergeByShape(existingFn, candidateFn)
		}
	}

	if subtype.IsSubtype(existing, candidate) {
		return candidate
	}
	if subtype.IsSubtype(candidate, existing) {
		return existing
	}
	return typ.JoinPreferNonSoft(existing, candidate)
}

// WidenForConvergence merges one function fact at a recursive fixpoint
// boundary.
func WidenForConvergence(prev, next api.FunctionFact) api.FunctionFact {
	out := api.FunctionFact{
		Signature:  WidenSignatureForConvergence(prev.Signature, next.Signature),
		Refinement: MergeRefinement(prev.Refinement, next.Refinement),
		EnvReturns: WidenEnvReturns(prev.EnvReturns, next.EnvReturns),
	}

	params := paramevidence.JoinConvergeCallVectors(paramsTypes(prev), paramsTypes(next))
	bodyParams := paramevidence.JoinBodyVectors(bodyParamsTypes(prev), bodyParamsTypes(next))
	entryParams := paramevidence.JoinEntryConvergeVectors(entryParamsTypes(prev), entryParamsTypes(next))
	summary := returnsummary.WidenForConvergence(summaryTypes(prev), summaryTypes(next))
	narrow := returnsummary.WidenForConvergence(narrowTypes(prev), narrowTypes(next))

	// Narrow summaries can refine optional/non-nil returns, but a nil-only
	// narrow observation must not erase an already-informative summary.
	if len(narrow) > 0 && !returnsummary.AllNil(narrow) {
		nextNarrow := narrowTypes(next)
		if len(nextNarrow) > 0 && !returnsummary.AllNil(nextNarrow) {
			narrow = repairInferredSummaryWithNarrow(out.Signature, narrow, nextNarrow)
		}
		if len(summary) == 0 {
			summary = returnsummary.Canonical(narrow)
		} else {
			summary = returnsummary.WidenForConvergence(summary, narrow)
		}
	}
	summary = repairInferredSummaryWithNarrow(out.Signature, summary, narrow)

	out.Params = product.LiftVector(params)
	out.BodyParams = product.LiftVector(bodyParams)
	out.EntryParams = product.LiftVector(entryParams)
	out.Summary = product.LiftVector(summary)
	out.Narrow = product.LiftVector(narrow)

	return out
}

// WidenSignatureForConvergence widens source-level function signatures at a
// recursive fixpoint boundary.
func WidenSignatureForConvergence(existing, candidate *typ.Function) *typ.Function {
	if existing == nil {
		return candidate
	}
	if candidate == nil {
		return existing
	}
	return unwrap.Function(WidenTypeForConvergence(existing, candidate))
}

// NormalizeRefinement canonicalizes empty function refinements away.
func NormalizeRefinement(refinement *constraint.FunctionRefinement) *constraint.FunctionRefinement {
	if refinement == nil || refinement.IsEmpty() {
		return nil
	}
	return refinement
}

// MergeRefinement returns the least imprecise sound fact that covers both
// refinement observations.
func MergeRefinement(existing, candidate *constraint.FunctionRefinement) *constraint.FunctionRefinement {
	existing = NormalizeRefinement(existing)
	candidate = NormalizeRefinement(candidate)
	switch {
	case existing == nil:
		return candidate
	case candidate == nil:
		return existing
	case existing.Equals(candidate):
		return existing
	}

	merged := &constraint.FunctionRefinement{
		Row:        mergeEffectRows(existing.Row, candidate.Row),
		OnReturn:   mergeGuaranteeCondition(existing.OnReturn, candidate.OnReturn),
		OnTrue:     mergeGuaranteeCondition(existing.OnTrue, candidate.OnTrue),
		OnFalse:    mergeGuaranteeCondition(existing.OnFalse, candidate.OnFalse),
		Terminates: existing.Terminates && candidate.Terminates,
	}
	return NormalizeRefinement(merged)
}

func mergeGuaranteeCondition(existing, candidate constraint.Condition) constraint.Condition {
	if existing.Equals(candidate) {
		return existing
	}
	if !existing.HasConstraints() || !candidate.HasConstraints() {
		return constraint.Condition{}
	}
	return constraint.Or(existing, candidate)
}

func mergeEffectRows(existing, candidate typ.EffectInfo) typ.EffectInfo {
	switch {
	case existing == nil:
		return candidate
	case candidate == nil:
		return existing
	case effectInfoEqual(existing, candidate):
		return existing
	}
	left, leftOK := existing.(effect.Row)
	right, rightOK := candidate.(effect.Row)
	if leftOK && rightOK {
		row := effect.Union(left, right)
		if row.Pure() && !row.IsOpen() {
			return nil
		}
		return row
	}
	return effect.Unknown
}

func effectInfoEqual(a, b typ.EffectInfo) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equals(b)
}

func repairSummaryWithNarrow(summary, narrow []typ.Type) []typ.Type {
	return repairSummaryWithNarrowPolicy(summary, narrow, true, true)
}

func repairInferredSummaryWithNarrow(signature *typ.Function, summary, narrow []typ.Type) []typ.Type {
	return repairSummaryWithNarrowPolicy(summary, narrow, signatureDeclaresReturnContract(signature), false)
}

func signatureDeclaresReturnContract(fn *typ.Function) bool {
	return fn != nil && len(fn.Returns) > 0
}

type repairTypeKey struct {
	summary         typ.Type
	narrow          typ.Type
	preserveAny     bool
	replaceConcrete bool
}

type repairFamilyKey struct {
	summary         uint64
	narrow          uint64
	preserveAny     bool
	replaceConcrete bool
}

type repairFamilyHashResult struct {
	hash uint64
	ok   bool
}

type repairFamilyCompareKey struct {
	a typ.Type
	b typ.Type
}

type repairTypeState struct {
	done         map[repairTypeKey]typ.Type
	active       map[repairTypeKey]bool
	activeFamily map[repairFamilyKey]bool
	familyHash   map[typ.Type]repairFamilyHashResult
	familySeen   map[repairFamilyCompareKey]bool
}

func newRepairTypeState() *repairTypeState {
	return &repairTypeState{
		done:         make(map[repairTypeKey]typ.Type),
		active:       make(map[repairTypeKey]bool),
		activeFamily: make(map[repairFamilyKey]bool),
		familyHash:   make(map[typ.Type]repairFamilyHashResult),
		familySeen:   make(map[repairFamilyCompareKey]bool),
	}
}

func repairSummaryWithNarrowPolicy(summary, narrow []typ.Type, preserveAny, replaceConcrete bool) []typ.Type {
	if len(narrow) == 0 {
		return summary
	}
	if len(summary) == 0 && returnsummary.AllNil(narrow) {
		return summary
	}
	if len(summary) == 0 {
		return narrow
	}
	state := newRepairTypeState()
	if len(summary) != len(narrow) {
		return repairPaddedSummaryWithNarrowPolicy(state, summary, narrow, preserveAny, replaceConcrete)
	}
	out := make([]typ.Type, len(summary))
	for i := range summary {
		out[i] = state.repair(summary[i], narrow[i], preserveAny, replaceConcrete)
	}
	return out
}

func repairPaddedSummaryWithNarrowPolicy(state *repairTypeState, summary, narrow []typ.Type, preserveAny, replaceConcrete bool) []typ.Type {
	maxLen := len(summary)
	if len(narrow) > maxLen {
		maxLen = len(narrow)
	}
	out := make([]typ.Type, maxLen)
	for i := 0; i < maxLen; i++ {
		var summarySlot typ.Type
		if i < len(summary) && summary[i] != nil {
			summarySlot = summary[i]
		} else {
			summarySlot = typ.Nil
		}
		var narrowSlot typ.Type
		if i < len(narrow) && narrow[i] != nil {
			narrowSlot = narrow[i]
		} else {
			narrowSlot = typ.Nil
		}
		if i < len(summary) && i < len(narrow) {
			out[i] = state.repair(summarySlot, narrowSlot, preserveAny, replaceConcrete)
			continue
		}
		out[i] = typ.JoinReturnSlot(summarySlot, narrowSlot)
	}
	return returnsummary.NormalizeAndPrune(out)
}

func (state *repairTypeState) repair(summary, narrow typ.Type, preserveAny, replaceConcrete bool) typ.Type {
	if summary == nil || narrow == nil {
		return narrow
	}
	if typ.IsUnknown(summary) {
		return narrow
	}
	if typ.IsNever(summary) {
		return narrow
	}
	if typ.IsAny(summary) {
		if typ.IsUnknown(narrow) || !preserveAny {
			return narrow
		}
		return summary
	}
	originalSummary := summary
	originalNarrow := narrow
	summary = unwrap.Alias(summary)
	narrow = unwrap.Alias(narrow)
	key := repairTypeKey{
		summary:         summary,
		narrow:          narrow,
		preserveAny:     preserveAny,
		replaceConcrete: replaceConcrete,
	}
	if repaired, ok := state.done[key]; ok {
		return repaired
	}
	if state.active[key] {
		return summary
	}
	familyKey, trackFamily := state.repairCoinductiveFamilyKey(summary, narrow, preserveAny, replaceConcrete)
	if trackFamily && state.activeFamily[familyKey] {
		return summary
	}
	state.active[key] = true
	if trackFamily {
		state.activeFamily[familyKey] = true
	}
	var repaired typ.Type
	defer func() {
		delete(state.active, key)
		if trackFamily {
			delete(state.activeFamily, familyKey)
		}
		state.done[key] = repaired
	}()
	if repairTypesEquivalent(originalSummary, originalNarrow) {
		repaired = returnsummary.SelectEquivalentReturnSlot(originalSummary, originalNarrow)
		return repaired
	}
	if upper, ok := value.DirectSelfEmbeddingUpperBound(summary, narrow); ok {
		repaired = upper
		return repaired
	}
	switch s := summary.(type) {
	case *typ.Union:
		n, ok := narrow.(*typ.Union)
		if !ok {
			members := make([]typ.Type, len(s.Members))
			changed := false
			for i, member := range s.Members {
				members[i] = state.repair(member, narrow, preserveAny, replaceConcrete)
				if repairTypeChanged(members[i], member) {
					changed = true
				}
			}
			if !changed {
				repaired = summary
				return repaired
			}
			repaired = join.Types(members...)
			return repaired
		}
		if len(s.Members) != len(n.Members) {
			repaired = summary
			return repaired
		}
		narrowIndex := state.newRepairUnionMemberIndex(n.Members)
		members := make([]typ.Type, len(s.Members))
		changed := false
		for i, member := range s.Members {
			members[i] = state.repair(member, state.bestNarrowUnionMember(member, n.Members, narrowIndex), preserveAny, replaceConcrete)
			if repairTypeChanged(members[i], member) {
				changed = true
			}
		}
		if !changed {
			repaired = summary
			return repaired
		}
		repaired = join.Types(members...)
		return repaired
	case *typ.Record:
		n, ok := narrow.(*typ.Record)
		if !ok {
			repaired = narrow
			return repaired
		}
		changed := false
		builder := typ.NewRecord().SetOpen(s.Open)
		if s.HasMapComponent() {
			mapKey := s.MapKey
			mapValue := s.MapValue
			if n.HasMapComponent() {
				mapKey = state.repair(s.MapKey, n.MapKey, preserveAny, replaceConcrete)
				mapValue = state.repair(s.MapValue, n.MapValue, preserveAny, replaceConcrete)
				if repairTypeChanged(mapKey, s.MapKey) || repairTypeChanged(mapValue, s.MapValue) {
					changed = true
				}
			}
			builder.MapComponent(mapKey, mapValue)
		}
		if s.Metatable != nil {
			metatable := s.Metatable
			if n.Metatable != nil {
				metatable = state.repair(s.Metatable, n.Metatable, preserveAny, replaceConcrete)
				if repairTypeChanged(metatable, s.Metatable) {
					changed = true
				}
			}
			builder.Metatable(metatable)
		}
		for _, field := range s.Fields {
			fieldType := field.Type
			fieldOptional := field.Optional
			if nf := n.GetField(field.Name); nf != nil {
				fieldType = state.repair(field.Type, nf.Type, preserveAny, replaceConcrete)
				if repairTypeChanged(fieldType, field.Type) {
					changed = true
				}
				if !nf.Optional {
					if fieldOptional {
						changed = true
					}
					fieldOptional = false
				}
			}
			switch {
			case fieldOptional && field.Readonly:
				builder.OptReadonlyField(field.Name, fieldType)
			case fieldOptional:
				builder.OptField(field.Name, fieldType)
			case field.Readonly:
				builder.ReadonlyField(field.Name, fieldType)
			default:
				builder.Field(field.Name, fieldType)
			}
		}
		if !changed {
			repaired = summary
			return repaired
		}
		repaired = builder.Build()
		return repaired
	case *typ.Optional:
		n, ok := narrow.(*typ.Optional)
		if !ok {
			repaired = defaultRepairResult(summary, narrow, replaceConcrete)
			return repaired
		}
		inner := state.repair(s.Inner, n.Inner, preserveAny, replaceConcrete)
		if !repairTypeChanged(inner, s.Inner) {
			repaired = summary
			return repaired
		}
		repaired = typ.NewOptional(inner)
		return repaired
	case *typ.Array:
		n, ok := narrow.(*typ.Array)
		if !ok {
			repaired = defaultRepairResult(summary, narrow, replaceConcrete)
			return repaired
		}
		element := state.repair(s.Element, n.Element, preserveAny, replaceConcrete)
		if !repairTypeChanged(element, s.Element) {
			repaired = summary
			return repaired
		}
		repaired = typ.NewArray(element)
		return repaired
	case *typ.Map:
		n, ok := narrow.(*typ.Map)
		if !ok {
			repaired = defaultRepairResult(summary, narrow, replaceConcrete)
			return repaired
		}
		key := state.repair(s.Key, n.Key, preserveAny, replaceConcrete)
		value := state.repair(s.Value, n.Value, preserveAny, replaceConcrete)
		if !repairTypeChanged(key, s.Key) && !repairTypeChanged(value, s.Value) {
			repaired = summary
			return repaired
		}
		repaired = typ.NewMap(key, value)
		return repaired
	case *typ.Tuple:
		n, ok := narrow.(*typ.Tuple)
		if !ok || len(s.Elements) != len(n.Elements) {
			repaired = defaultRepairResult(summary, narrow, replaceConcrete)
			return repaired
		}
		elements := make([]typ.Type, len(s.Elements))
		changed := false
		for i := range s.Elements {
			elements[i] = state.repair(s.Elements[i], n.Elements[i], preserveAny, replaceConcrete)
			if repairTypeChanged(elements[i], s.Elements[i]) {
				changed = true
			}
		}
		if !changed {
			repaired = summary
			return repaired
		}
		repaired = typ.NewTuple(elements...)
		return repaired
	default:
		repaired = defaultRepairResult(summary, narrow, replaceConcrete)
		return repaired
	}
}

func defaultRepairResult(summary, narrow typ.Type, replaceConcrete bool) typ.Type {
	if replaceConcrete {
		return narrow
	}
	return summary
}

func repairTypeChanged(candidate, baseline typ.Type) bool {
	if typ.SameUnionMember(candidate, baseline) {
		return false
	}
	if typ.SameProductFamily(candidate, baseline) {
		return false
	}
	if candidate == nil || baseline == nil {
		return candidate != baseline
	}
	if typ.EqualityHash(candidate) != typ.EqualityHash(baseline) {
		return true
	}
	return !typ.TypeEquals(candidate, baseline)
}

func repairTypesEquivalent(a, b typ.Type) bool {
	if typ.SameUnionMember(a, b) {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if typ.ContainsRecursive(a) || typ.ContainsRecursive(b) {
		return typ.SameProductFamily(a, b)
	}
	if typ.EqualityHash(a) == typ.EqualityHash(b) && typ.TypeEquals(a, b) {
		return true
	}
	return subtype.IsSubtype(a, b) && subtype.IsSubtype(b, a)
}

type repairUnionMemberIndex map[uint64][]typ.Type

func (state *repairTypeState) newRepairUnionMemberIndex(members []typ.Type) repairUnionMemberIndex {
	index := make(repairUnionMemberIndex, len(members))
	for _, member := range members {
		hash, ok := state.repairFamilyHash(member)
		if !ok {
			continue
		}
		index[hash] = append(index[hash], member)
	}
	return index
}

func (state *repairTypeState) bestNarrowUnionMember(summary typ.Type, members []typ.Type, index repairUnionMemberIndex) typ.Type {
	if hash, ok := state.repairFamilyHash(summary); ok && len(index) > 0 {
		for _, member := range index[hash] {
			if state.sameRepairUnionFamily(member, summary) {
				return member
			}
		}
		return summary
	}
	for _, member := range members {
		if state.sameRepairUnionFamily(member, summary) {
			return member
		}
	}
	return summary
}

func (state *repairTypeState) sameRepairUnionFamily(a, b typ.Type) bool {
	a = unwrap.Alias(a)
	b = unwrap.Alias(b)
	if a == nil || b == nil {
		return a == b
	}
	if typ.SameUnionMember(a, b) {
		return true
	}
	key := repairFamilyCompareKey{a: a, b: b}
	if state.familySeen[key] {
		return true
	}
	state.familySeen[key] = true
	defer delete(state.familySeen, key)
	if al, ok := a.(*typ.Literal); ok {
		bl, ok := b.(*typ.Literal)
		return ok && typ.LiteralEquals(al, bl)
	}
	if _, ok := b.(*typ.Literal); ok {
		return false
	}
	switch av := a.(type) {
	case *typ.Optional:
		bv, ok := b.(*typ.Optional)
		return ok && state.sameRepairUnionFamily(av.Inner, bv.Inner)
	case *typ.Record:
		bv, ok := b.(*typ.Record)
		return ok && state.sameRepairRecordFamily(av, bv)
	case *typ.Map:
		bv, ok := b.(*typ.Map)
		if !ok {
			return false
		}
		return state.repairMapKeyFamily(av.Key, bv.Key)
	case *typ.Array:
		_, ok := b.(*typ.Array)
		return ok
	case *typ.Tuple:
		bv, ok := b.(*typ.Tuple)
		return ok && len(av.Elements) == len(bv.Elements)
	case *typ.Function:
		bv, ok := b.(*typ.Function)
		return ok && len(av.Params) == len(bv.Params) && len(av.Returns) == len(bv.Returns)
	case *typ.Interface:
		bv, ok := b.(*typ.Interface)
		return ok && av.Name == bv.Name && len(av.Methods) == len(bv.Methods)
	case *typ.Recursive:
		bv, ok := b.(*typ.Recursive)
		if !ok || av.Name != bv.Name {
			return false
		}
		return state.sameRepairUnionFamily(av.Body, bv.Body)
	default:
		return a.Kind() == b.Kind()
	}
}

func (state *repairTypeState) repairCoinductiveFamilyKey(summary, narrow typ.Type, preserveAny, replaceConcrete bool) (repairFamilyKey, bool) {
	if !value.CanSelfEmbed(summary) || !value.CanSelfEmbed(narrow) {
		return repairFamilyKey{}, false
	}
	summaryHash, okSummary := state.repairFamilyHash(summary)
	narrowHash, okNarrow := state.repairFamilyHash(narrow)
	if !okSummary || !okNarrow {
		return repairFamilyKey{}, false
	}
	return repairFamilyKey{
		summary:         summaryHash,
		narrow:          narrowHash,
		preserveAny:     preserveAny,
		replaceConcrete: replaceConcrete,
	}, true
}

func (state *repairTypeState) repairFamilyHash(t typ.Type) (uint64, bool) {
	t = unwrap.Alias(t)
	if t == nil {
		return 0, false
	}
	if cached, ok := state.familyHash[t]; ok {
		return cached.hash, cached.ok
	}
	hash, ok := state.computeRepairFamilyHash(t)
	state.familyHash[t] = repairFamilyHashResult{hash: hash, ok: ok}
	return hash, ok
}

func (state *repairTypeState) computeRepairFamilyHash(t typ.Type) (uint64, bool) {
	if lit, ok := t.(*typ.Literal); ok {
		return internal.HashCombine(uint64(lit.Kind()), lit.Hash()), true
	}
	switch v := t.(type) {
	case *typ.Optional:
		inner, ok := state.repairFamilyHash(v.Inner)
		if !ok {
			return 0, false
		}
		return internal.HashCombine(uint64(v.Kind()), inner), true
	case *typ.Union:
		h := internal.HashCombine(uint64(v.Kind()), uint64(len(v.Members)))
		for _, member := range v.Members {
			memberHash, ok := state.repairFamilyHash(member)
			if !ok {
				return 0, false
			}
			h = internal.HashCombine(h, memberHash)
		}
		return h, true
	case *typ.Intersection:
		h := internal.HashCombine(uint64(v.Kind()), uint64(len(v.Members)))
		for _, member := range v.Members {
			memberHash, ok := state.repairFamilyHash(member)
			if !ok {
				return 0, false
			}
			h = internal.HashCombine(h, memberHash)
		}
		return h, true
	case *typ.Record:
		h := internal.HashCombine(uint64(v.Kind()), boolHash(v.Open))
		h = internal.HashCombine(h, boolHash(v.HasMapComponent()))
		if v.HasMapComponent() {
			keyHash, ok := state.repairMapKeyFamilyHash(v.MapKey)
			if !ok {
				return 0, false
			}
			h = internal.HashCombine(h, keyHash)
		}
		h = internal.HashCombine(h, uint64(len(v.Fields)))
		for _, field := range v.Fields {
			h = internal.HashCombine(h, internal.FnvString(field.Name))
			h = internal.HashCombine(h, boolHash(field.Optional))
			h = internal.HashCombine(h, boolHash(field.Readonly))
			h = internal.HashCombine(h, repairFieldFamilyHash(field.Type))
		}
		return h, true
	case *typ.Map:
		keyHash, ok := state.repairMapKeyFamilyHash(v.Key)
		if !ok {
			return 0, false
		}
		return internal.HashCombine(uint64(v.Kind()), keyHash), true
	case *typ.Array:
		return uint64(v.Kind()), true
	case *typ.Tuple:
		return internal.HashCombine(uint64(v.Kind()), uint64(len(v.Elements))), true
	case *typ.Function:
		h := internal.HashCombine(uint64(v.Kind()), uint64(len(v.Params)))
		h = internal.HashCombine(h, uint64(len(v.Returns)))
		if v.Variadic != nil {
			h = internal.HashCombine(h, 1)
		}
		return h, true
	case *typ.Interface:
		h := internal.HashCombine(uint64(v.Kind()), internal.FnvString(v.Name))
		h = internal.HashCombine(h, uint64(len(v.Methods)))
		return h, true
	case *typ.Recursive:
		return internal.HashCombine(uint64(v.Kind()), internal.FnvString(v.Name)), true
	default:
		return uint64(t.Kind()), true
	}
}

func (state *repairTypeState) repairMapKeyFamilyHash(t typ.Type) (uint64, bool) {
	t = unwrap.Alias(t)
	if t == nil {
		return 0, false
	}
	if typ.IsAny(t) || typ.IsUnknown(t) {
		return uint64(t.Kind()), true
	}
	return state.repairFamilyHash(t)
}

func repairFieldFamilyHash(t typ.Type) uint64 {
	t = unwrap.Alias(t)
	if lit, ok := t.(*typ.Literal); ok {
		return internal.HashCombine(uint64(lit.Kind()), lit.Hash())
	}
	if t == nil {
		return 0
	}
	return uint64(t.Kind())
}

func boolHash(v bool) uint64 {
	if v {
		return 1
	}
	return 0
}

func (state *repairTypeState) repairMapKeyFamily(a, b typ.Type) bool {
	if typ.IsAny(a) || typ.IsAny(b) || typ.IsUnknown(a) || typ.IsUnknown(b) {
		return true
	}
	return state.sameRepairUnionFamily(a, b)
}

func (state *repairTypeState) sameRepairRecordFamily(a, b *typ.Record) bool {
	if !value.ShallowStructuralShapeEquals(a, b) {
		return false
	}
	for _, af := range a.Fields {
		bf := b.GetField(af.Name)
		if bf == nil || !sameRepairFieldFamily(af.Type, bf.Type) {
			return false
		}
	}
	return true
}

func sameRepairFieldFamily(a, b typ.Type) bool {
	a = unwrap.Alias(a)
	b = unwrap.Alias(b)
	al, aLit := a.(*typ.Literal)
	bl, bLit := b.(*typ.Literal)
	if aLit || bLit {
		return aLit && bLit && typ.LiteralEquals(al, bl)
	}
	return true
}

// ClassFamilyJoin joins two class-family record bodies at the recursive fixpoint
// boundary. The recursive-family interner passes the full class records as
// existing and candidate.
//
// A class body's fields are method functions plus the __index back-edge. Method
// fields merge through WidenTypeForConvergence, which collapses two same-method
// function snapshots to one function; the value-domain merge would instead union
// them into fn|fn and re-degrade the method surface. So when both bodies are
// records the join rebuilds the record field-by-field, widening function-typed
// fields with the function-aware merge and other fields with the value-domain
// convergence merge. Non-record bodies fall back to the value-domain merge.
func ClassFamilyJoin(existing, candidate typ.Type) typ.Type {
	exRec := unwrap.Record(existing)
	caRec := unwrap.Record(candidate)
	if exRec == nil || caRec == nil {
		return value.MergeForConvergence(existing, candidate)
	}

	builder := typ.NewRecord()
	keys := make(map[fieldkey.Key]struct{}, len(exRec.Fields)+len(caRec.Fields))
	for _, f := range exRec.Fields {
		addClassFamilyFieldKey(keys, f.Name)
	}
	for _, f := range caRec.Fields {
		addClassFamilyFieldKey(keys, f.Name)
	}

	for _, key := range fieldkey.Sorted(keys) {
		if key.Kind != constraint.SegmentField || key.Name == "" {
			continue
		}
		name := key.Name
		merged, opt, ro := mergeClassFamilyField(name, exRec.GetField(name), caRec.GetField(name))
		switch {
		case opt && ro:
			builder.OptReadonlyField(name, merged)
		case opt:
			builder.OptField(name, merged)
		case ro:
			builder.ReadonlyField(name, merged)
		default:
			builder.Field(name, merged)
		}
	}
	if exRec.Metatable != nil || caRec.Metatable != nil {
		builder.Metatable(value.MergeForConvergence(metatableOf(exRec), metatableOf(caRec)))
	}
	switch {
	case exRec.HasMapComponent():
		builder.MapComponent(exRec.MapKey, exRec.MapValue)
	case caRec.HasMapComponent():
		builder.MapComponent(caRec.MapKey, caRec.MapValue)
	}
	return builder.SetOpen(exRec.Open || caRec.Open).Build()
}

func addClassFamilyFieldKey(keys map[fieldkey.Key]struct{}, name string) {
	key, ok := fieldkey.FromName(name)
	if !ok {
		return
	}
	keys[key] = struct{}{}
}

// mergeClassFamilyField merges one class-family field from the two record bodies.
// The __index back-edge denotes the class family itself, so when one observation
// has folded it to the recursion variable while the other still carries the
// structural unfolding, the merge collapses to the recursion variable rather than
// unioning the two views; otherwise unioning would leave __index as
// {unfolded proto} | mu, which method resolution cannot fold. Function fields use
// the function-aware widen; remaining fields use the value-domain merge. It
// returns the merged type and the optional/readonly attributes (true when either
// side carries them).
func mergeClassFamilyField(name string, ef, cf *typ.Field) (typ.Type, bool, bool) {
	switch {
	case ef == nil && cf == nil:
		return typ.Unknown, false, false
	case ef == nil:
		return cf.Type, cf.Optional, cf.Readonly
	case cf == nil:
		return ef.Type, ef.Optional, ef.Readonly
	}
	var merged typ.Type
	switch {
	case name == classIndexField:
		merged = mergeClassBackEdge(ef.Type, cf.Type)
	case unwrap.Function(ef.Type) != nil || unwrap.Function(cf.Type) != nil:
		merged = mergeClassMethodField(ef.Type, cf.Type)
	default:
		merged = value.MergeForConvergence(ef.Type, cf.Type)
	}
	return merged, ef.Optional || cf.Optional, ef.Readonly || cf.Readonly
}

// classIndexField names the Lua metatable __index slot carrying the class
// prototype back-edge.
const classIndexField = "__index"

// mergeClassMethodField merges two observations of a class method field across
// fixpoint iterations. An uninformative seed (a parameterless, return-less
// placeholder emitted before the method literal has a solved projection) is
// dropped in favor of the informative side, collapsing the seed-vs-solved fn|fn
// the task warns about.
//
// For two same-shape function observations the merge is monotone-precise on the
// return vector: a class method's return refines from unknown toward its solved
// type across iterations, so each return slot takes the informative side when the
// other is still unknown, and otherwise the value-domain convergence merge. This
// keeps a partially-solved snapshot such as (unknown, string?) from pinning the
// family body below a later (Context[]?, string?) observation. Differently-shaped
// or non-function observations fall back to the value-domain convergence merge.
func mergeClassMethodField(a, b typ.Type) typ.Type {
	af := unwrap.Function(a)
	bf := unwrap.Function(b)
	if af == nil || bf == nil {
		return value.MergeForConvergence(a, b)
	}
	switch {
	case value.UninformativeFunctionSeed(af) && !value.UninformativeFunctionSeed(bf):
		return b
	case value.UninformativeFunctionSeed(bf) && !value.UninformativeFunctionSeed(af):
		return a
	}
	if !SameShape(af, bf) || len(af.Returns) != len(bf.Returns) {
		return value.MergeForConvergence(a, b)
	}
	rets := make([]typ.Type, len(af.Returns))
	for i := range af.Returns {
		rets[i] = mergeClassMethodReturnSlot(af.Returns[i], bf.Returns[i])
	}
	if merged := join.WithReturns(af, rets); merged != nil {
		return merged
	}
	return value.MergeForConvergence(a, b)
}

// mergeClassMethodReturnSlot merges one return slot of a class method, taking the
// informative side when the other is still the unknown inference seed and the
// value-domain convergence merge otherwise.
func mergeClassMethodReturnSlot(a, b typ.Type) typ.Type {
	switch {
	case a == nil || typ.IsUnknown(a):
		return b
	case b == nil || typ.IsUnknown(b):
		return a
	default:
		return value.MergeForConvergence(a, b)
	}
}

// mergeClassBackEdge merges two observations of a class __index back-edge,
// collapsing to a recursion variable when one side carries it so the back-edge
// stays a single recursion edge instead of a {unfolding | mu} union.
func mergeClassBackEdge(a, b typ.Type) typ.Type {
	if _, ok := a.(*typ.Recursive); ok {
		return a
	}
	if _, ok := b.(*typ.Recursive); ok {
		return b
	}
	return value.MergeForConvergence(a, b)
}

func metatableOf(rec *typ.Record) typ.Type {
	if rec == nil {
		return nil
	}
	return rec.Metatable
}

// WidenTypeForConvergence merges function-type facts at a recursive fixpoint
// boundary.
func WidenTypeForConvergence(existing, candidate typ.Type) typ.Type {
	existing = value.NormalizeFactType(existing)
	candidate = value.NormalizeFactType(candidate)
	if existing == nil {
		return candidate
	}
	if candidate == nil {
		return existing
	}
	existingFn := unwrap.Function(existing)
	candidateFn := unwrap.Function(candidate)
	if existingFn != nil && candidateFn != nil {
		if replacement, ok := value.FunctionEvidenceUpperBound(existingFn, candidateFn); ok {
			return value.WidenForConvergence(replacement)
		}
	}
	if existingFn != nil && candidateFn != nil && SameShape(existingFn, candidateFn) {
		return value.WidenForConvergence(widenByShapeForConvergence(existingFn, candidateFn))
	}
	return value.MergeForConvergence(existing, candidate)
}

type variants struct {
	funcs     []*typ.Function
	residuals []typ.Type
}

func mergeVariants(existing, candidate typ.Type) (typ.Type, bool) {
	existingVariants := splitVariants(existing)
	candidateVariants := splitVariants(candidate)
	if len(existingVariants.funcs) == 0 || len(candidateVariants.funcs) == 0 {
		return nil, false
	}

	all := make([]*typ.Function, 0, len(existingVariants.funcs)+len(candidateVariants.funcs))
	all = append(all, existingVariants.funcs...)
	all = append(all, candidateVariants.funcs...)
	for i := 1; i < len(all); i++ {
		if !SameShape(all[0], all[i]) {
			return nil, false
		}
	}

	merged := all[0]
	for i := 1; i < len(all); i++ {
		next, _ := mergeByShape(merged, all[i]).(*typ.Function)
		if next == nil {
			return nil, false
		}
		merged = next
	}

	residuals := make([]typ.Type, 0, len(existingVariants.residuals)+len(candidateVariants.residuals)+1)
	residuals = append(residuals, existingVariants.residuals...)
	residuals = append(residuals, candidateVariants.residuals...)
	if len(residuals) == 0 {
		return merged, true
	}
	residuals = append(residuals, merged)
	return join.Types(residuals...), true
}

func splitVariants(t typ.Type) variants {
	var out variants
	collectVariants(t, &out)
	return out
}

func collectVariants(t typ.Type, out *variants) {
	if t == nil || out == nil {
		return
	}
	switch v := unwrap.Alias(t).(type) {
	case *typ.Union:
		for _, member := range v.Members {
			collectVariants(member, out)
		}
		return
	}
	if fn := unwrap.Function(t); fn != nil {
		out.funcs = append(out.funcs, fn)
		return
	}
	out.residuals = append(out.residuals, t)
}

// SameShape reports whether two function fact types can be merged slot-wise.
func SameShape(a, b *typ.Function) bool {
	if a == nil || b == nil {
		return false
	}
	if len(a.TypeParams) != len(b.TypeParams) {
		return false
	}
	if !typeParamsEqual(a.TypeParams, b.TypeParams) {
		return false
	}
	return len(a.Params) == len(b.Params)
}

func mergeByShape(existing, candidate *typ.Function) typ.Type {
	if existing == nil {
		return candidate
	}
	if candidate == nil {
		return existing
	}

	builder := typ.Func()
	for _, tp := range existing.TypeParams {
		builder = builder.TypeParamRef(tp)
	}

	for i, p := range existing.Params {
		paramType := mergeParamType(p.Type, candidate.Params[i].Type)
		name := p.Name
		if name == "" {
			name = candidate.Params[i].Name
		}
		optional := p.Optional || candidate.Params[i].Optional
		if optional {
			builder = builder.OptParam(name, paramType)
		} else {
			builder = builder.Param(name, paramType)
		}
	}

	if existing.Variadic != nil || candidate.Variadic != nil {
		builder = builder.Variadic(mergeParamType(existing.Variadic, candidate.Variadic))
	}

	if mergedReturns := returnsummary.Merge(existing.Returns, candidate.Returns); len(mergedReturns) > 0 {
		builder = builder.Returns(mergedReturns...)
	}

	effects := existing.Effects
	if effects == nil {
		effects = candidate.Effects
	}
	if effects != nil {
		builder = builder.Effects(effects)
	}
	if spec := mergeFunctionSpec(existing.Spec, candidate.Spec); spec != nil {
		builder = builder.Spec(spec)
	}
	refinement := existing.Refinement
	if refinement == nil {
		refinement = candidate.Refinement
	}
	if refinement != nil {
		builder = builder.WithRefinement(refinement)
	}

	return builder.Build()
}

func widenByShapeForConvergence(existing, candidate *typ.Function) typ.Type {
	if existing == nil {
		return candidate
	}
	if candidate == nil {
		return existing
	}

	builder := typ.Func()
	for _, tp := range existing.TypeParams {
		builder = builder.TypeParamRef(tp)
	}
	for i, p := range existing.Params {
		paramType := widenParamTypeForConvergence(p.Type, candidate.Params[i].Type)
		name := p.Name
		if name == "" {
			name = candidate.Params[i].Name
		}
		if p.Optional || candidate.Params[i].Optional {
			builder = builder.OptParam(name, paramType)
		} else {
			builder = builder.Param(name, paramType)
		}
	}
	if existing.Variadic != nil || candidate.Variadic != nil {
		builder = builder.Variadic(widenParamTypeForConvergence(existing.Variadic, candidate.Variadic))
	}
	if returns := returnsummary.WidenForConvergence(existing.Returns, candidate.Returns); len(returns) > 0 {
		builder = builder.Returns(returns...)
	}

	effects := existing.Effects
	if effects == nil {
		effects = candidate.Effects
	}
	if effects != nil {
		builder = builder.Effects(effects)
	}
	if spec := mergeFunctionSpec(existing.Spec, candidate.Spec); spec != nil {
		builder = builder.Spec(spec)
	}
	refinement := existing.Refinement
	if refinement == nil {
		refinement = candidate.Refinement
	}
	if refinement != nil {
		builder = builder.WithRefinement(refinement)
	}
	return builder.Build()
}

// MergeParamType merges one function parameter type using the same evidence
// policy as canonical function-fact merging.
func MergeParamType(existing, candidate typ.Type) typ.Type {
	return mergeParamType(existing, candidate)
}

func mergeParamType(existing, candidate typ.Type) typ.Type {
	if existing == nil {
		return candidate
	}
	if candidate == nil {
		return existing
	}

	existing = typ.PruneSoftUnionMembers(existing)
	candidate = typ.PruneSoftUnionMembers(candidate)
	if unwrap.IsNilType(existing) && !unwrap.IsNilType(candidate) {
		return candidate
	}
	if unwrap.IsNilType(candidate) && !unwrap.IsNilType(existing) {
		return existing
	}
	if preferred, ok := preferStructuredRecord(existing, candidate); ok {
		return preferred
	}
	if preservesBodyStructuralPrecision(candidate) &&
		subtype.IsSubtype(candidate, existing) &&
		!subtype.IsSubtype(existing, candidate) {
		return candidate
	}
	if seq, ok := value.JoinSequenceShape(existing, candidate, mergeParamType); ok {
		return seq
	}
	if joined, ok := value.JoinRecordShape(existing, candidate, mergeParamType); ok {
		return joined
	}
	if joined, ok := value.JoinMapRecordShape(existing, candidate, mergeParamType); ok {
		return joined
	}
	if joined, ok := value.JoinStructuralUnionShape(existing, candidate, mergeParamType); ok {
		return joined
	}
	if preferred, ok := value.PreferConcreteOverSoft(existing, candidate); ok {
		return preferred
	}
	if typ.IsUnknown(existing) {
		return candidate
	}
	if typ.IsUnknown(candidate) {
		return existing
	}
	if paramevidence.AnyLikeParam(existing) && paramevidence.PassiveOptionalRecordEvidence(candidate) {
		return existing
	}
	if paramevidence.AnyLikeParam(existing) && !paramevidence.AnyLikeParam(candidate) {
		return candidate
	}
	if paramevidence.AnyLikeParam(candidate) && !paramevidence.AnyLikeParam(existing) {
		return existing
	}
	if typ.IsAny(existing) && typ.IsAny(candidate) {
		return typ.Any
	}
	if typ.IsAny(existing) {
		return candidate
	}
	if typ.IsAny(candidate) {
		return existing
	}
	if value.FactTypeEqual(existing, candidate) {
		return existing
	}
	if unwrap.Function(existing) != nil || unwrap.Function(candidate) != nil {
		return MergeType(existing, candidate)
	}
	if typ.TypeEquals(existing, candidate) {
		return existing
	}
	if paramevidence.RefinesFunctionParam(candidate, existing) {
		return candidate
	}
	if paramevidence.RefinesFunctionParam(existing, candidate) {
		return existing
	}
	if subtype.IsSubtype(existing, candidate) && !subtype.IsSubtype(candidate, existing) {
		return candidate
	}
	if subtype.IsSubtype(candidate, existing) && !subtype.IsSubtype(existing, candidate) {
		if preservesBodyStructuralPrecision(candidate) {
			return candidate
		}
		return existing
	}
	return typ.JoinPreferNonSoft(existing, candidate)
}

func widenParamTypeForConvergence(existing, candidate typ.Type) typ.Type {
	existing = value.NormalizeFactType(existing)
	candidate = value.NormalizeFactType(candidate)
	if existing == nil {
		return candidate
	}
	if candidate == nil {
		return existing
	}
	if value.FactTypeEqual(existing, candidate) {
		return existing
	}
	if unwrap.Function(existing) != nil || unwrap.Function(candidate) != nil {
		return WidenTypeForConvergence(existing, candidate)
	}
	if typ.TypeEquals(existing, candidate) {
		return existing
	}
	if typ.IsAny(existing) || typ.IsUnknown(existing) {
		return existing
	}
	if typ.IsAny(candidate) || typ.IsUnknown(candidate) {
		return candidate
	}
	if preferred, ok := value.PreferConcreteOverSoft(existing, candidate); ok {
		return preferred
	}
	if upper, ok := value.SelfEmbeddingUpperBound(existing, candidate); ok {
		return upper
	}
	if preservesBodyStructuralPrecision(candidate) &&
		subtype.IsSubtype(candidate, existing) &&
		!subtype.IsSubtype(existing, candidate) {
		return candidate
	}
	if seq, ok := value.JoinSequenceShape(existing, candidate, widenParamTypeForConvergence); ok {
		return seq
	}
	if joined, ok := value.JoinRecordShape(existing, candidate, widenParamTypeForConvergence); ok {
		return joined
	}
	if joined, ok := value.JoinMapRecordShape(existing, candidate, widenParamTypeForConvergence); ok {
		return joined
	}
	if joined, ok := value.JoinStructuralUnionShape(existing, candidate, widenParamTypeForConvergence); ok {
		return joined
	}
	if paramevidence.RefinesFunctionParam(candidate, existing) {
		return candidate
	}
	if paramevidence.RefinesFunctionParam(existing, candidate) {
		return existing
	}
	if typ.ContainsRecursive(existing) || typ.ContainsRecursive(candidate) ||
		value.HasHigherOrderGrowthRisk(existing) || value.HasHigherOrderGrowthRisk(candidate) {
		return value.MergeForConvergence(existing, candidate)
	}
	if subtype.IsSubtype(candidate, existing) && !subtype.IsSubtype(existing, candidate) {
		if preservesBodyStructuralPrecision(candidate) {
			return candidate
		}
		return existing
	}
	if subtype.IsSubtype(existing, candidate) && !subtype.IsSubtype(candidate, existing) {
		return candidate
	}
	return typ.JoinPreferNonSoft(existing, candidate)
}

func preferStructuredRecord(existing, candidate typ.Type) (typ.Type, bool) {
	existingRec, okExisting := unwrap.Alias(existing).(*typ.Record)
	candidateRec, okCandidate := unwrap.Alias(candidate).(*typ.Record)
	if !okExisting || !okCandidate {
		return nil, false
	}

	existingOpenTop := existingRec.Open && len(existingRec.Fields) == 0 && !existingRec.HasMapComponent()
	candidateOpenTop := candidateRec.Open && len(candidateRec.Fields) == 0 && !candidateRec.HasMapComponent()
	if existingOpenTop == candidateOpenTop {
		return nil, false
	}
	if existingOpenTop {
		if candidateRec.HasMapComponent() || len(candidateRec.Fields) > 0 {
			return candidate, true
		}
	}
	if candidateOpenTop {
		if existingRec.HasMapComponent() || len(existingRec.Fields) > 0 {
			return existing, true
		}
	}
	return nil, false
}

func preservesBodyStructuralPrecision(t typ.Type) bool {
	switch v := unwrap.Optional(t).(type) {
	case *typ.Array, *typ.Map, *typ.Tuple:
		return true
	case *typ.Record:
		return v.HasMapComponent() || len(v.Fields) > 0
	case *typ.Union:
		for _, member := range v.Members {
			if preservesBodyStructuralPrecision(member) {
				return true
			}
		}
	}
	return false
}

// MergeReturnsForSameSignature merges return slots for function signatures that
// already have identical call shapes.
func MergeReturnsForSameSignature(prevFn, nextFn *typ.Function) (typ.Type, bool) {
	if prevFn == nil || nextFn == nil {
		return nil, false
	}
	if len(prevFn.TypeParams) != len(nextFn.TypeParams) {
		return nil, false
	}
	if !typeParamsEqual(prevFn.TypeParams, nextFn.TypeParams) {
		return nil, false
	}
	if len(prevFn.Params) != len(nextFn.Params) {
		return nil, false
	}
	if (prevFn.Variadic == nil) != (nextFn.Variadic == nil) {
		return nil, false
	}
	if prevFn.Variadic != nil && !typ.TypeEquals(prevFn.Variadic, nextFn.Variadic) {
		return nil, false
	}
	for i := range prevFn.Params {
		if prevFn.Params[i].Optional != nextFn.Params[i].Optional {
			return nil, false
		}
		if !typ.TypeEquals(prevFn.Params[i].Type, nextFn.Params[i].Type) {
			return nil, false
		}
	}
	if len(prevFn.Returns) == 0 && len(nextFn.Returns) == 0 {
		return prevFn, true
	}
	if len(prevFn.Returns) != len(nextFn.Returns) || len(prevFn.Returns) == 0 {
		return nil, false
	}

	allowedTypeParams := make(map[string]bool, len(prevFn.TypeParams))
	for _, tp := range prevFn.TypeParams {
		if tp != nil && tp.Name != "" {
			allowedTypeParams[tp.Name] = true
		}
	}
	normalizeReturn := func(t typ.Type) (typ.Type, bool) {
		if t == nil {
			return nil, false
		}
		if !typ.ContainsTypeParam(t) {
			return t, false
		}
		leaked := false
		return typ.Rewrite(t, func(node typ.Type) (typ.Type, bool) {
			tp, ok := node.(*typ.TypeParam)
			if !ok {
				return node, false
			}
			if allowedTypeParams[tp.Name] {
				return node, false
			}
			// Free type params in non-generic function returns are unstable placeholders.
			leaked = true
			return typ.Unknown, true
		}), leaked
	}
	normalizedPrev := make([]typ.Type, len(prevFn.Returns))
	normalizedNext := make([]typ.Type, len(nextFn.Returns))
	leakedPrev := make([]bool, len(prevFn.Returns))
	leakedNext := make([]bool, len(nextFn.Returns))
	for i := range prevFn.Returns {
		normalizedPrev[i], leakedPrev[i] = normalizeReturn(prevFn.Returns[i])
		normalizedNext[i], leakedNext[i] = normalizeReturn(nextFn.Returns[i])
	}

	mergedReturns := make([]typ.Type, len(normalizedPrev))
	for i := range mergedReturns {
		switch {
		case leakedPrev[i] && !leakedNext[i]:
			mergedReturns[i] = normalizedNext[i]
		case leakedNext[i] && !leakedPrev[i]:
			mergedReturns[i] = normalizedPrev[i]
		default:
			mergedReturns[i] = typ.JoinReturnSlot(normalizedPrev[i], normalizedNext[i])
		}
	}
	if returnsummary.Equal(prevFn.Returns, mergedReturns) {
		return prevFn, true
	}
	if returnsummary.Equal(nextFn.Returns, mergedReturns) {
		return nextFn, true
	}

	effects := prevFn.Effects
	if effects == nil {
		effects = nextFn.Effects
	}
	spec := mergeFunctionSpec(prevFn.Spec, nextFn.Spec)
	refinement := prevFn.Refinement
	if refinement == nil {
		refinement = nextFn.Refinement
	}

	builder := typ.Func().
		Effects(effects).
		Spec(spec).
		WithRefinement(refinement)
	for _, tp := range prevFn.TypeParams {
		builder = builder.TypeParamRef(tp)
	}
	for _, p := range prevFn.Params {
		if p.Optional {
			builder = builder.OptParam(p.Name, p.Type)
		} else {
			builder = builder.Param(p.Name, p.Type)
		}
	}
	if prevFn.Variadic != nil {
		builder = builder.Variadic(prevFn.Variadic)
	}
	builder = builder.Returns(mergedReturns...)
	return builder.Build(), true
}

func typeParamsEqual(a, b []*typ.TypeParam) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] == nil || b[i] == nil {
			if a[i] != b[i] {
				return false
			}
			continue
		}
		if !a[i].Equals(b[i]) {
			return false
		}
	}
	return true
}
