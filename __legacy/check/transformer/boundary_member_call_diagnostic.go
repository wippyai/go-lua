package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/memberaccess"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/domain/type/subtype"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// boundaryMemberCallDiagnosticTerm is one point-local typed member call whose
// receiver is owned by the lexical boundary. Both receiver and provider are
// lifted through the same factor-native decision carrier that reaches the
// call: the receiver supplies the callable contract while the provider proves
// that the selected member still has that contract after flow-sensitive
// mutation.
type boundaryMemberCallDiagnosticTerm struct {
	site          cfg.Point
	receiver      ValueTerm
	provider      ValueTerm
	member        segment.Segment
	callable      product.Value
	hasCallable   bool
	pathArguments []boundaryMemberCallPathArgument
	obligations   []callpayload.CallParamObligation
}

// boundaryMemberCallPathArgument retains a caller-visible argument path until
// the stabilized receiver type determines the member's exact parameter type.
// Captures and globals cannot be represented as explicit parameter
// obligations; their paths cross linked frames through the ordinary diagnostic
// projection circuit.
type boundaryMemberCallPathArgument struct {
	path             PathTerm
	memberParamIndex int
	paramIndex       int
	paramOrigin      callpayload.CallParamObligationOrigin
	isParam          bool
}

func cloneBoundaryMemberCallDiagnostics(in []boundaryMemberCallDiagnosticTerm) []boundaryMemberCallDiagnosticTerm {
	if len(in) == 0 {
		return nil
	}
	out := make([]boundaryMemberCallDiagnosticTerm, len(in))
	for index, term := range in {
		term.pathArguments = append([]boundaryMemberCallPathArgument(nil), term.pathArguments...)
		term.obligations = cloneBoundaryParamObligations(term.obligations)
		out[index] = term
	}
	return out
}

func validBoundaryMemberCallDiagnostics(reg *axis.Registry, arena *Arena, shape Shape, in []boundaryMemberCallDiagnosticTerm) bool {
	if reg == nil || arena == nil {
		return false
	}
	for _, term := range in {
		if term.receiver == 0 || !arena.validValue(term.receiver, shape, make(map[ValueTerm]bool)) ||
			term.provider == 0 || !arena.validValue(term.provider, shape, make(map[ValueTerm]bool)) ||
			!memberaccess.Valid(term.member) ||
			!validBoundaryParamObligations(reg, shape.Params, term.obligations) {
			return false
		}
		if term.hasCallable && (!product.BelongsToRegistry(reg, term.callable) || product.Equal(reg, term.callable, product.Top())) {
			return false
		}
		for _, argument := range term.pathArguments {
			if argument.memberParamIndex < 0 || !arena.validPath(argument.path, shape) ||
				argument.isParam && (argument.paramIndex < 0 || uint32(argument.paramIndex) >= shape.Params || !argument.paramOrigin.HasOrigin) {
				return false
			}
		}
	}
	return true
}

func classifyBoundaryMemberCallDiagnosticValues(
	program *RelationProgram,
	body *relationProgramBody,
	receiver, provider product.Value,
	term boundaryMemberCallDiagnosticTerm,
) (callpayload.DiagnosticOutput, bool, error) {
	out := callpayload.DiagnosticOutput{SuspensionKnown: true}
	if program == nil || body == nil {
		return callpayload.DiagnosticOutput{}, false, fmt.Errorf("transformer: member-call diagnostic values are unowned")
	}
	receiverType, typed := typevalue.TypeOf(program.registry, receiver)
	var fn *typ.Function
	callable := false
	if typed && receiverType != nil {
		fn, _, callable = memberaccess.Callable(receiverType, term.member)
	}
	if (!callable || fn == nil) && term.hasCallable {
		callableType, exact := typevalue.TypeOf(program.registry, term.callable)
		if exact && callableType != nil {
			if candidate, ok := callableType.(*typ.Function); ok {
				fn, callable = candidate, true
			}
		}
	}
	if !callable || fn == nil {
		return out, false, nil
	}
	actual, actualOK := typevalue.TypeOf(program.registry, provider)
	if !actualOK || actual == nil || !subtype.IsSubtype(actual, fn) {
		return out, false, nil
	}
	out.ParamObligations = append(out.ParamObligations, term.obligations...)
	for _, argument := range term.pathArguments {
		if argument.memberParamIndex >= len(fn.Params) {
			continue
		}
		want := fn.Params[argument.memberParamIndex].Type
		if want == nil || typ.IsAny(want) || typ.IsUnknown(want) {
			continue
		}
		value := typevalue.WithWitness(program.registry, typevalue.FromType(program.registry, want), want)
		if !usefulBoundaryDiagnosticValue(program.registry, value) {
			continue
		}
		if argument.isParam {
			out.ParamObligations = append(out.ParamObligations, callpayload.CallParamObligation{ParamIndex: argument.paramIndex, Value: value, Origin: argument.paramOrigin})
			continue
		}
		path, exact := bodyRelativeBoundaryDiagnosticPath(body, argument.path)
		if !exact {
			return callpayload.DiagnosticOutput{}, false, fmt.Errorf("transformer: member-call argument is outside the lexical boundary")
		}
		out.PathObligations = append(out.PathObligations, callpayload.CallPathObligation{Path: path, Value: value})
	}
	return out.Normalize(program.registry), true, nil
}

func boundaryMemberCallFromSite(ctx planCompileContext, point cfg.Point) (boundaryMemberCallDiagnosticTerm, *typ.Function, bool) {
	if ctx.plan == nil || ctx.builder == nil || ctx.registry == nil {
		return boundaryMemberCallDiagnosticTerm{}, nil, false
	}
	site, ok := ctx.facts.CallSiteView(point)
	if !ok {
		return boundaryMemberCallDiagnosticTerm{}, nil, false
	}
	receiver, member, ok := memberCallReceiverFromFactSite(site)
	if !ok || receiver.Symbol == 0 || receiver.Version != 0 {
		return boundaryMemberCallDiagnosticTerm{}, nil, false
	}
	binding, err := exactBoundaryPathBinding(ctx, receiver)
	if err != nil {
		return boundaryMemberCallDiagnosticTerm{}, nil, false
	}
	receiverRoot := binding.Root
	if receiverRoot.Kind == 0 {
		if index, boundary := ctx.plan.BoundaryParamIndex(binding.Symbol); boundary {
			receiverRoot = Root{Kind: RootParam, Index: uint32(index)}
		} else if index, boundary := ctx.plan.BoundaryCaptureIndex(binding.Symbol); boundary {
			receiverRoot = Root{Kind: RootCapture, Index: uint32(index)}
		} else if index, boundary := ctx.plan.BoundaryGlobalIndex(binding.Symbol); boundary {
			receiverRoot = Root{Kind: RootGlobal, Index: uint32(index)}
		}
	}
	receiverTerm, _, err := ctx.builder.Arena().LowerBoundaryRequiredPathValue(receiver, binding, receiverRoot)
	if err != nil || receiverTerm == 0 {
		return boundaryMemberCallDiagnosticTerm{}, nil, false
	}
	providerPath := receiver.AppendSegments([]segment.Segment{member})
	provider, _, err := ctx.builder.Arena().LowerBoundaryRequiredPathValue(providerPath, binding, receiverRoot)
	if err != nil || provider == 0 {
		return boundaryMemberCallDiagnosticTerm{}, nil, false
	}
	term := boundaryMemberCallDiagnosticTerm{site: point, receiver: receiverTerm, provider: provider, member: member}

	// Parameter receivers have a declaration-time callable contract, so their
	// explicit-parameter provenance can be frozen immediately. Captures and
	// globals deliberately resolve the same contract from stabilized State.
	var fn *typ.Function
	receiverParam := -1
	if receiverRoot.Kind == RootParam {
		receiverParam = int(receiverRoot.Index)
		contracts := ctx.plan.BoundaryParamContracts()
		if receiverParam >= 0 && receiverParam < len(contracts) {
			receiverType, typed := typevalue.TypeOf(ctx.registry, contracts[receiverParam])
			if typed && receiverType != nil {
				if len(receiver.Segments) != 0 {
					receiverType, typed = luatypeprojection.ApplySegments(receiverType, receiver.Segments)
				}
				if typed {
					fn, _, _ = memberaccess.Callable(receiverType, member)
				}
			}
		}
	}
	if fn != nil {
		term.callable = typevalue.WithWitness(ctx.registry, typevalue.FromType(ctx.registry, fn), fn)
		term.hasCallable = true
	}
	offset := 0
	if site.MethodName() != "" {
		offset = 1
	}
	paramCount := len(ctx.plan.BoundaryParamContracts())
	site.ForEachArgumentSource(func(argumentIndex int, source factflow.ValueSource) bool {
		memberParamIndex := argumentIndex + offset
		argumentRoot, path, pathExact := exactBoundaryMemberArgumentPath(ctx, source)
		argumentLexical, _ := boundaryMemberArgumentLexicalPath(ctx, source)
		argumentLabel, _ := site.ArgumentLabelAt(argumentIndex)
		if pathExact && (argumentRoot.Kind != RootParam || fn == nil) {
			argument := boundaryMemberCallPathArgument{path: path, memberParamIndex: memberParamIndex}
			if argumentRoot.Kind == RootParam && fn == nil {
				if origin, exact := boundaryMemberOrigin(receiverParam, receiver.Segments, member, int(argumentRoot.Index), memberParamIndex, argumentIndex); exact {
					origin = labelBoundaryMemberOrigin(origin, receiver, argumentLexical, argumentLabel, member, argumentIndex)
					argument.paramIndex, argument.paramOrigin, argument.isParam = int(argumentRoot.Index), origin, true
				}
			}
			term.pathArguments = append(term.pathArguments, argument)
		}
		if fn == nil || memberParamIndex < 0 || memberParamIndex >= len(fn.Params) {
			return true
		}
		want := fn.Params[memberParamIndex].Type
		if want == nil || typ.IsAny(want) || typ.IsUnknown(want) {
			return true
		}
		appendObligation := func(paramIndex int, required typ.Type) {
			if paramIndex < 0 || paramIndex >= paramCount || required == nil {
				return
			}
			value := typevalue.WithWitness(ctx.registry, typevalue.FromType(ctx.registry, required), required)
			if !usefulBoundaryDiagnosticValue(ctx.registry, value) {
				return
			}
			origin, exact := boundaryMemberOrigin(receiverParam, receiver.Segments, member, paramIndex, memberParamIndex, argumentIndex)
			if !exact {
				return
			}
			origin = labelBoundaryMemberOrigin(origin, receiver, argumentLexical, argumentLabel, member, argumentIndex)
			term.obligations = append(term.obligations, callpayload.CallParamObligation{ParamIndex: paramIndex, Value: value, Origin: origin})
		}
		// exactBoundaryMemberArgumentPath already resolved the source through the
		// same sealed boundary binding used to build its PathTerm. Preserve that
		// single identity decision for parameter arguments; asking a second source
		// classifier can lose a structurally lowered EnvironmentPath even though
		// its boundary namespace is exact.
		if pathExact && argumentRoot.Kind == RootParam {
			appendObligation(int(argumentRoot.Index), want)
			return true
		}
		symbol, exact := compilerRootSourceSymbol(ctx, source)
		if paramIndex, boundary := ctx.plan.BoundaryParamIndex(symbol); exact && boundary {
			appendObligation(paramIndex, want)
			return true
		}
		argument, err := exactCompilerSourceTerm(ctx, source)
		if err != nil {
			return true
		}
		for _, paramIndex := range boundaryConcatOperandParamIndices(ctx, argument) {
			appendObligation(paramIndex, boundaryConcatOperandObligationType())
		}
		return true
	})
	return term, fn, true
}

// exactBoundaryMemberZeroResultReturn recognizes only a statically proved
// zero-result member call in tail position. Contextual return values are not
// diagnostic artifacts: their canonical owner is the sealed external-call
// result slot installed by bindExternalCallSlotTerms, and the ordinary return
// transaction must retain that typed term.
func exactBoundaryMemberZeroResultReturn(ctx planCompileContext, source factflow.ValueSource) bool {
	if source.Kind != factflow.ValueSourceCall || !source.HasCallPoint {
		return false
	}
	if _, ok := ctx.facts.CallSiteView(source.CallPoint); !ok {
		return false
	}
	_, fn, exact := boundaryMemberCallFromSite(ctx, source.CallPoint)
	return exact && fn != nil && len(fn.Returns) == 0
}

func boundaryMemberArgumentLexicalPath(ctx planCompileContext, source factflow.ValueSource) (pathdom.Path, bool) {
	if !source.Valid() || source.Expanded || source.Adjusted || source.OpenTail {
		return pathdom.Path{}, false
	}
	if source.PathKey != "" {
		if named, ok := compilerResolverPath(source.PathKey); ok && named.Root != "" {
			return named, true
		}
	}
	switch source.Kind {
	case factflow.ValueSourceExpression:
		if !source.HasExpr {
			return pathdom.Path{}, false
		}
		return ctx.facts.ExpressionPathRef(source.ExprRef)
	case factflow.ValueSourcePath:
		return compilerResolverPath(source.PathKey)
	default:
		return pathdom.Path{}, false
	}
}

func labelBoundaryMemberOrigin(origin callpayload.CallParamObligationOrigin, receiver, argument pathdom.Path, argumentLabel string, member segment.Segment, argumentIndex int) callpayload.CallParamObligationOrigin {
	// CallSiteView owns the frozen lexical spelling. Structural expression paths
	// deliberately retain symbol identity rather than source names, so they are
	// only a deterministic fallback when lowering did not provide a label.
	if argumentLabel == "" && argument.Root != "" {
		argumentLabel = argument.String()
	}
	if argumentLabel != "" {
		origin.SubjectLabel = fmt.Sprintf("argument %d (%s)", argumentIndex+1, argumentLabel)
	}
	provider := receiver.AppendSegments([]segment.Segment{member}).String()
	if provider != "" {
		origin.ProviderLabel = provider
	}
	return origin
}

func exactBoundaryMemberArgumentPath(ctx planCompileContext, source factflow.ValueSource) (Root, PathTerm, bool) {
	if !source.Valid() || source.Expanded || source.Adjusted || source.OpenTail {
		return Root{}, 0, false
	}
	var p pathdom.Path
	switch source.Kind {
	case factflow.ValueSourceExpression:
		if !source.HasExpr {
			return Root{}, 0, false
		}
		var ok bool
		p, ok = ctx.facts.ExpressionPathRef(source.ExprRef)
		if !ok {
			return Root{}, 0, false
		}
	case factflow.ValueSourcePath:
		var ok bool
		p, ok = compilerResolverPath(source.PathKey)
		if !ok {
			return Root{}, 0, false
		}
	default:
		return Root{}, 0, false
	}
	binding, err := exactBoundaryPathBinding(ctx, p)
	if err != nil {
		return Root{}, 0, false
	}
	root := binding.Root
	// Structural lowering spells the current post-N4 value as an
	// EnvironmentPath. For this diagnostic projection only, recover its already
	// sealed boundary namespace from the same symbol; do not mutate the generic
	// BoundaryPathBinding invariant or replace the environment storage term.
	if root.Kind == 0 {
		if index, boundary := ctx.plan.BoundaryParamIndex(binding.Symbol); boundary {
			root = Root{Kind: RootParam, Index: uint32(index)}
		} else if index, boundary := ctx.plan.BoundaryCaptureIndex(binding.Symbol); boundary {
			root = Root{Kind: RootCapture, Index: uint32(index)}
		} else if index, boundary := ctx.plan.BoundaryGlobalIndex(binding.Symbol); boundary {
			root = Root{Kind: RootGlobal, Index: uint32(index)}
		}
	}
	if len(p.Segments) == 0 {
		term := binding.Base
		return root, term, term != 0
	}
	_, term, err := ctx.builder.Arena().LowerBoundaryPathValue(p, binding)
	return root, term, err == nil && term != 0
}

func memberCallReceiverFromFactSite(site factflow.CallSiteView) (pathdom.Path, segment.Segment, bool) {
	receiver, member, ok := site.CalleeMemberAccessPath()
	if !ok || receiver.IsEmpty() || !memberaccess.Valid(member) {
		return pathdom.Path{}, segment.Segment{}, false
	}
	return receiver, member, true
}

func boundaryMemberOrigin(receiverParam int, receiverSuffix []segment.Segment, member segment.Segment, argParam, memberParamIndex, argumentIndex int) (callpayload.CallParamObligationOrigin, bool) {
	key, ok := pathaddr.RelativeStaticMemberSuffixKey(receiverSuffix)
	if len(receiverSuffix) == 0 {
		key, ok = "", true
	}
	if !ok {
		return callpayload.CallParamObligationOrigin{}, false
	}
	return callpayload.CallParamObligationOrigin{
		HasOrigin: true, ReceiverParam: receiverParam, ReceiverPath: key, Member: member,
		ArgParam: argParam, MemberParamIndex: memberParamIndex,
		SubjectLabel:  fmt.Sprintf("argument %d", argumentIndex+1),
		ProviderLabel: pathdom.NewPlaceholder(receiverParam).AppendSegments(receiverSuffix).AppendSegments([]segment.Segment{member}).String(),
	}, true
}
