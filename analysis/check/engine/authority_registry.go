package engine

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/domain/value/proof"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// An authorityRule names one source that may answer a question. A view's slice
// is its precedence: the first rule that answers owns the result. The rule is a
// typed internal name, never serialized or parsed.
type authorityRule string

const (
	authoritySelectPayload           authorityRule = "select-payload"
	authorityCorrelationConeCurrent  authorityRule = "correlation-cone-current"
	authorityDeclaredOptionalMapRead authorityRule = "declared-optional-map-read"
	authorityHeapMember              authorityRule = "heap-member"
	authorityAssertionNarrowed       authorityRule = "assertion-narrowed"
	authorityProvenSequenceIndex     authorityRule = "proven-sequence-index"
	authorityTypedPath               authorityRule = "typed-path"
	authorityReconverged             authorityRule = "reconverged"
	authorityShapeMember             authorityRule = "shape-member"
	authorityDeclaredSequenceIndex   authorityRule = "declared-sequence-index"
	authorityRuntimeValue            authorityRule = "runtime-value"
	authorityCurrentSummary          authorityRule = "current-summary"
	authorityCastTarget              authorityRule = "cast-target"
	authorityDeclaredMap             authorityRule = "declared-map"
	authorityDeclaredArray           authorityRule = "declared-array"
	authorityDeclaredType            authorityRule = "declared-type"
	authorityKeyedComponent          authorityRule = "keyed-component"
	authorityExactCurrentValue       authorityRule = "exact-current-value"
	authorityCurrentTableValue       authorityRule = "current-table-value"
	authoritySealedCallableResult    authorityRule = "sealed-callable-result"
	authorityInferredCallableResult  authorityRule = "inferred-callable-result"
	authorityLocalResultSummary      authorityRule = "local-result-summary"
	authorityTypedCallableResult     authorityRule = "typed-callable-result"
	authoritySealedMethodResult      authorityRule = "sealed-method-result"
	authorityMethodReceiverResult    authorityRule = "method-receiver-result"
	authorityStaticReceiverResult    authorityRule = "static-receiver-result"
	authorityTypedMethodResult       authorityRule = "typed-method-result"
	authorityChannelMethodResult     authorityRule = "channel-method-result"
)

type authorityView struct {
	Name  string
	Rules []authorityRule
}

var valueAtReadAuthorities = authorityView{
	Name: "value-at-read",
	Rules: []authorityRule{
		authoritySelectPayload,
		authorityDeclaredOptionalMapRead,
		authorityHeapMember,
		authorityAssertionNarrowed,
		authorityProvenSequenceIndex,
		authorityTypedPath,
		authorityReconverged,
		authorityDeclaredSequenceIndex,
	},
}

var currentValueAuthorities = authorityView{
	Name: "value-current",
	Rules: []authorityRule{
		authoritySelectPayload,
		authorityCorrelationConeCurrent,
		authorityDeclaredOptionalMapRead,
		authorityHeapMember,
		authorityAssertionNarrowed,
		authorityProvenSequenceIndex,
		authorityReconverged,
		authorityTypedPath,
		authorityShapeMember,
		authorityDeclaredSequenceIndex,
	},
}

// The typed-path/reconverged swap between value-at-read and value-current is
// inherited and unexplained. It is preserved as two declared orders so any
// future unification is a reviewed data change rather than an incidental edit.

var runtimeIndexContainerTypeAuthorities = authorityView{
	Name: "runtime-index-container-type",
	Rules: []authorityRule{
		authorityRuntimeValue,
		// A Top current value does not refute the guarded, epoch-current
		// summary independently published for this RuntimeIndex projection.
		authorityCurrentSummary,
		authorityCastTarget,
		authorityTypedPath,
		// Uniform declarations answer unresolved computed keys but establish
		// no slot occupancy; a later presence proof owns nil removal.
		authorityDeclaredMap,
		authorityDeclaredArray,
		// Unresolved-key stores establish their own uniform component when no
		// value witness or declaration answered first.
		authorityKeyedComponent,
	},
}

var iteratorElementTypeAuthorities = authorityView{
	Name: "iterator-element-type",
	Rules: []authorityRule{
		authorityRuntimeValue,
		authorityTypedPath,
		authorityDeclaredType,
		authorityKeyedComponent,
	},
}

var instantiatedFormalTypeAuthorities = authorityView{
	Name: "instantiated-formal-type",
	Rules: []authorityRule{
		authorityTypedPath,
		authorityDeclaredType,
		authorityKeyedComponent,
	},
}

var integerTermTypeAuthorities = authorityView{
	Name: "integer-term-type",
	Rules: []authorityRule{
		authorityExactCurrentValue,
		authorityTypedPath,
		authorityDeclaredType,
	},
}

var placementDescentTableAuthorities = authorityView{
	Name: "placement-descent-table",
	Rules: []authorityRule{
		authorityCurrentTableValue,
		// A projected typed path is decisive: a known scalar projection must
		// refuse descent instead of falling through to an unrelated shape.
		authorityTypedPath,
	},
}

var unresolvedCallResultAuthorities = authorityView{
	Name: "unresolved-call-result",
	Rules: []authorityRule{
		authoritySealedCallableResult,
		authorityInferredCallableResult,
		authorityLocalResultSummary,
		authorityTypedCallableResult,
		authoritySealedMethodResult,
		authorityMethodReceiverResult,
		authorityStaticReceiverResult,
		authorityTypedMethodResult,
		authorityChannelMethodResult,
	},
}

type callResultAuthorityContext struct {
	Lexical           *lexicalEvaluator
	Callee            []byte
	Receiver          []byte
	Method            []byte
	Index             int
	Arguments         map[int][]byte
	Partition         equation.Partition
	LocalUnionSummary typ.Type
	LocalUnion        bool
}

type callResultAuthorityAnswer struct {
	Value          []byte
	LocalCallable  bool
	ReceiverResult bool
	ReceiverTerm   []byte
	LocalSummary   typ.Type
	MethodSummary  typ.Type
}

func resolveUnresolvedCallResult(context callResultAuthorityContext) (callResultAuthorityAnswer, error) {
	answer := callResultAuthorityAnswer{ReceiverTerm: context.Receiver}
	for _, rule := range unresolvedCallResultAuthorities.Rules {
		switch rule {
		case authoritySealedCallableResult:
			if value, ok := sealedCallableResultValue(context.Lexical, context.Callee, context.Index, context.Arguments, context.Partition); ok {
				answer.Value, answer.LocalCallable = value, true
			}
		case authorityInferredCallableResult:
			if value, ok := inferredCallableResultValue(context.Callee, context.Index, context.Partition); ok {
				answer.Value, answer.LocalCallable = value, true
			}
		case authorityLocalResultSummary:
			if summary, ok := sealedCallableResultType(context.Lexical, context.Callee, context.Index, context.Arguments, context.Partition); ok && requiresLocalUnionProof(summary) {
				answer.LocalSummary = summary
			}
			if answer.LocalSummary == nil && context.LocalUnion && requiresLocalUnionProof(context.LocalUnionSummary) {
				answer.LocalSummary = context.LocalUnionSummary
			}
			if answer.LocalSummary == nil {
				if summary, ok := typedCallableResultType(context.Callee, context.Index, context.Arguments, context.Partition); ok && requiresLocalUnionProof(summary) {
					answer.LocalSummary = summary
				}
			}
		case authorityTypedCallableResult:
			answer.Value, _ = typedCallableResultValue(context.Callee, context.Index, context.Arguments, context.Partition)
		case authoritySealedMethodResult:
			answer.Value, _ = sealedMethodResultValue(context.Lexical, context.Receiver, context.Method, context.Index, context.Partition)
		case authorityMethodReceiverResult:
			if value, ok := sealedMethodReceiverResultValue(context.Lexical, context.Receiver, context.Method, context.Index, context.Partition); ok {
				answer.Value, answer.ReceiverResult = value, true
			}
		case authorityStaticReceiverResult:
			if value, term, ok := sealedStaticMemberReceiverResultValue(context.Lexical, context.Callee, context.Index, context.Partition); ok {
				answer.Value, answer.ReceiverResult, answer.ReceiverTerm = value, true, term
			}
		case authorityTypedMethodResult:
			if summary, ok := typedMethodReturnType(context.Receiver, context.Method, context.Index, context.Partition); ok {
				answer.MethodSummary = summary
			}
			answer.Value, _ = typedMethodResultValue(context.Receiver, context.Method, context.Index, context.Partition)
		case authorityChannelMethodResult:
			answer.Value, _ = ambientChannelMethodResultValue(context.Receiver, context.Method, context.Index, context.Partition)
		default:
			return callResultAuthorityAnswer{}, unknownAuthorityError(unresolvedCallResultAuthorities, rule)
		}
		if len(answer.Value) != 0 {
			return answer, nil
		}
	}
	return answer, nil
}

type valueAuthorityContext struct {
	Term      []byte
	ReadBound string
	Partition equation.Partition
}

func resolveAuthorityValue(view authorityView, context valueAuthorityContext) ([]byte, bool, error) {
	for _, rule := range view.Rules {
		var value []byte
		var found bool
		switch rule {
		case authoritySelectPayload:
			value, found = selectPayloadValue(context.Term, context.Partition)
		case authorityCorrelationConeCurrent:
			value, found = correlationConeCurrentValue(context.Term, context.Partition)
		case authorityDeclaredOptionalMapRead:
			value, found = declaredOptionalMapReadValue(context.Term, context.Partition)
		case authorityHeapMember:
			value, found = heapMemberValue(context.Term, context.Partition)
		case authorityAssertionNarrowed:
			value, found = assertionNarrowedValue(context.Term, context.ReadBound, context.Partition)
		case authorityProvenSequenceIndex:
			value, found = provenSequenceIndexValue(context.Term, context.Partition)
		case authorityTypedPath:
			value, found = typedPathValue(context.Term, context.Partition)
		case authorityReconverged:
			value, found = reconvergedValue(context.Term, context.ReadBound, context.Partition)
		case authorityShapeMember:
			value, found = shapeMemberValue(context.Term, context.Partition)
		case authorityDeclaredSequenceIndex:
			value, found = declaredSequenceIndexValue(context.Term, context.Partition)
		default:
			return nil, false, unknownAuthorityError(view, rule)
		}
		if found {
			if len(value) == 0 {
				return nil, false, fmt.Errorf("engine: %s authority %q returned an empty value", view.Name, rule)
			}
			return value, true, nil
		}
	}
	return nil, false, nil
}

type typeAuthorityContext struct {
	Term         []byte
	CurrentValue []byte
	Partition    equation.Partition
}

func resolveAuthorityType(view authorityView, authorityContext typeAuthorityContext) (typ.Type, bool, error) {
	for _, rule := range view.Rules {
		var value typ.Type
		var found bool
		switch rule {
		case authorityRuntimeValue:
			value, found = shapefact.DecodeTarget(authorityContext.CurrentValue)
		case authorityCurrentSummary:
			if isUnknownScalar(authorityContext.CurrentValue) {
				if encoded, current := currentFamilyEpochFact(factkey.SummaryType, authorityContext.Term, authorityContext.Partition); current {
					var err error
					value, err = typ.DecodeCanonical(context.Background(), encoded)
					found = err == nil && value != nil
				}
			}
		case authorityCastTarget:
			if claim, claimed := shapefact.DecodeClaim(authorityContext.CurrentValue); claimed && claim.Kind == wir.ClaimCast {
				value, found = castTargetWitness(authorityContext.Term, authorityContext.Partition)
			}
		case authorityTypedPath:
			value, found = typedPathType(authorityContext.Term, authorityContext.Partition)
		case authorityDeclaredMap:
			value, found = declaredContainerType(authorityContext.Term, authorityContext.Partition, declaredContainerMap)
		case authorityDeclaredArray:
			value, found = declaredContainerType(authorityContext.Term, authorityContext.Partition, declaredContainerArray)
		case authorityDeclaredType:
			value, found = declaredTypeForTerm(authorityContext.Term, authorityContext.Partition)
		case authorityKeyedComponent:
			value, found = keyedComponentContainerType(authorityContext.Term, authorityContext.Partition)
		case authorityExactCurrentValue:
			current, err := resolveCurrentValue(authorityContext.Term, authorityContext.Partition)
			if err == nil {
				value, found = shapefact.DecodeExactWitnessType(current)
			}
		default:
			return nil, false, unknownAuthorityError(view, rule)
		}
		if found {
			if value == nil {
				return nil, false, fmt.Errorf("engine: %s authority %q returned a nil type", view.Name, rule)
			}
			return value, true, nil
		}
	}
	return nil, false, nil
}

type predicateAuthorityContext struct {
	Term      []byte
	Partition equation.Partition
}

func resolveAuthorityPredicate(view authorityView, context predicateAuthorityContext) (bool, bool, error) {
	for _, rule := range view.Rules {
		switch rule {
		case authorityCurrentTableValue:
			value, err := resolveCurrentValue(context.Term, context.Partition)
			if err == nil && shapefact.IsTable(value) {
				return true, true, nil
			}
		case authorityTypedPath:
			if projected, found := typedPathType(context.Term, context.Partition); found {
				return isTableTypeForPlacement(projected), true, nil
			}
		default:
			return false, false, unknownAuthorityError(view, rule)
		}
	}
	return false, false, nil
}

func unknownAuthorityError(view authorityView, rule authorityRule) error {
	return fmt.Errorf("engine: %s contains unknown authority rule %q", view.Name, rule)
}

type declaredContainerKind uint8

const (
	declaredContainerMap declaredContainerKind = iota + 1
	declaredContainerArray
)

// declaredContainerType is the single declaration authority for uniformly
// typed containers. The rule chooses which declared shape answers its question;
// a declaration never implies that any slot is occupied.
func declaredContainerType(term []byte, partition equation.Partition, containerKind declaredContainerKind) (typ.Type, bool) {
	declared, found := declaredTypeForTerm(term, partition)
	if !found || declared == nil {
		return nil, false
	}
	base := unwrap.Alias(subst.ExpandInstantiated(proof.ProjectionWithoutNil(declared)))
	if base == nil {
		return nil, false
	}
	switch containerKind {
	case declaredContainerMap:
		switch base.Kind() {
		case kind.Map, kind.ReadonlyMap:
			return declared, true
		}
	case declaredContainerArray:
		if base.Kind() == kind.Array {
			return declared, true
		}
	}
	return nil, false
}
