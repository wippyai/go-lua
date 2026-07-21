package body

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/check/body/internal/readexpr"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/factquery"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
)

func calleeValueProvider(
	reg *axis.Registry,
	facts factflow.Facts,
	resolver *visibility.Resolver,
	sources sourcevalue.SourceValues,
	typeValues *typevalue.Cache,
	bindings *bind.Result,
	typeResolver *typeresolve.Resolver,
	rootDeclarations func() factquery.RootDeclarationQuery,
) CalleeValueFunc {
	return func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) (product.Value, bool) {
		if site.MethodName() != "" {
			if value, ok := pathMethodCalleeValue(reg, typeValues, facts, resolver, ctx, site, in); ok {
				return value, true
			}
			if value, ok := declaredReceiverMethodCalleeValue(reg, typeValues, facts, bindings, typeResolver, rootDeclarations, ctx, site); ok {
				return value, true
			}
		}
		p := site.CalleePathRef()
		if p.IsEmpty() {
			return methodCalleeValue(reg, typeValues, sources, ctx, site, in, read)
		}
		config := readexpr.Config{Registry: reg, Facts: facts, Visibility: resolver, TypeValues: typeValues}
		if value, ok := readexpr.Project(config, ctx.Point, p, in); ok &&
			(hasCalleeEvidence(reg, value) || product.Get(reg, value, evidence.Key).IsExplicitTop()) {
			return value, true
		}
		if value, ok, authoritative := readablePrefixCalleeValue(reg, typeValues, config, ctx.Point, p, in); authoritative {
			return value, ok
		}
		if len(p.Segments) == 0 {
			return product.Value{}, false
		}
		root := p.RootOnly()
		rootValue, ok := readexpr.Project(config, ctx.Point, root, in)
		if !ok {
			return product.Value{}, false
		}
		rootType, ok := typevalue.WitnessOf(reg, rootValue)
		if !ok {
			return product.Value{}, false
		}
		projected, ok := luaPathTypeProjector(rootType, p)
		if !ok || projected == nil {
			return product.Value{}, false
		}
		return typeValues.FromTypeWithWitness(reg, projected), true
	}
}

func declaredReceiverMethodCalleeValue(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	facts factflow.Facts,
	bindings *bind.Result,
	resolver *typeresolve.Resolver,
	rootDeclarations func() factquery.RootDeclarationQuery,
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
) (product.Value, bool) {
	fn, ok := declaredReceiverMethodFunction(facts, bindings, resolver, rootDeclarations, ctx, site)
	if !ok {
		return product.Value{}, false
	}
	return typeValues.FromTypeWithWitness(reg, fn), true
}

func declaredReceiverCallableProvider(
	facts factflow.Facts,
	bindings *bind.Result,
	resolver *typeresolve.Resolver,
	rootDeclarations func() factquery.RootDeclarationQuery,
) ReceiverCallableFunc {
	return func(ctx transfer.NodeContext, site factflow.CallSiteView) (*typ.Function, bool) {
		return declaredReceiverMethodFunction(facts, bindings, resolver, rootDeclarations, ctx, site)
	}
}

func declaredReceiverMethodFunction(
	facts factflow.Facts,
	bindings *bind.Result,
	resolver *typeresolve.Resolver,
	rootDeclarations func() factquery.RootDeclarationQuery,
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
) (*typ.Function, bool) {
	if bindings == nil {
		return nil, false
	}
	method := site.MethodName()
	if method == "" {
		return nil, false
	}
	receiverPath, ok := site.ReceiverPath()
	if !ok || receiverPath.Symbol == 0 || len(receiverPath.Segments) != 0 {
		return nil, false
	}
	if rootDeclarations == nil {
		rootDeclarations = preparedRootDeclarationQuery(facts, ctx.Graph)
	}
	if _, replaced := rootDeclarations().DominatingOrdinaryRootWrite(ctx.Point, receiverPath.Symbol); replaced {
		return nil, false
	}
	typeExpr, ok := bindings.SymbolTypeAnnotation(receiverPath.Symbol)
	if !ok || typeExpr == nil {
		return nil, false
	}
	if resolver == nil {
		resolver = typeresolve.New(bindings)
	}
	receiverType, ok := resolver.Type(typeExpr)
	if !ok || receiverType == nil || typ.IsAny(receiverType) || typ.IsUnknown(receiverType) || typ.IsNever(receiverType) {
		return nil, false
	}
	fn, status, ok := typecall.MemberCallable(receiverType, method)
	if status != typecall.MemberCallOK || !ok || fn == nil {
		return nil, false
	}
	return fn, true
}

// preparedRootDeclarationQuery owns the immutable CFG dominance analysis once
// for every prepared body. Method resolution runs at transfer frequency, so
// rebuilding dominators at each call turns a structural graph query into a
// dominant allocation site on call-heavy bodies.
func preparedRootDeclarationQuery(facts factflow.Facts, graph cfg.Graph) func() factquery.RootDeclarationQuery {
	return sync.OnceValue(func() factquery.RootDeclarationQuery {
		return factquery.NewRootDeclarationQuery(facts, graph)
	})
}

func readablePrefixCalleeValue(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	config readexpr.Config,
	point cfg.Point,
	p pathdom.Path,
	in state.State,
) (product.Value, bool, bool) {
	if len(p.Segments) < 2 {
		return product.Value{}, false, false
	}
	for prefix := p.Parent(); !prefix.IsEmpty() && len(prefix.Segments) > 0; prefix = prefix.Parent() {
		value, ok := readexpr.Project(config, point, prefix, in)
		if !ok {
			continue
		}
		prefixType, ok := typevalue.WitnessOf(reg, value)
		if !ok || prefixType == nil || typ.IsAny(prefixType) || typ.IsUnknown(prefixType) || typ.IsNever(prefixType) {
			return product.Value{}, false, true
		}
		suffix := p.Segments[len(prefix.Segments):]
		projected, ok := luatypeprojection.ApplySegments(prefixType, suffix)
		if !ok || projected == nil {
			return product.Value{}, false, true
		}
		return typeValues.FromTypeWithWitness(reg, projected), true, true
	}
	return product.Value{}, false, false
}

func pathMethodCalleeValue(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	facts factflow.Facts,
	resolver *visibility.Resolver,
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	in state.State,
) (product.Value, bool) {
	method := site.MethodName()
	if method == "" {
		return product.Value{}, false
	}
	receiverPath, ok := site.ReceiverPath()
	if !ok || receiverPath.IsEmpty() {
		return product.Value{}, false
	}
	config := readexpr.Config{Registry: reg, Facts: facts, Visibility: resolver, TypeValues: typeValues}
	if receiverValue, ok := readexpr.ProjectWithoutRootStaticMemberOverlay(config, ctx.Point, receiverPath, in); ok {
		if value, ok := methodValueFromReceiverValue(reg, typeValues, receiverValue, method); ok {
			return value, true
		}
	}
	methodSegment := segment.Segment{Kind: segment.SegmentField, Name: method}
	methodValue, methodOK := readexpr.ProjectStaticMember(config, ctx.Point, receiverPath, methodSegment, in)
	if methodOK && hasCalleeEvidence(reg, methodValue) {
		return methodValue, true
	}
	methodValue, methodOK = readexpr.ProjectWithoutRootStaticMemberOverlay(config, ctx.Point, receiverPath.AppendSegments([]segment.Segment{methodSegment}), in)
	if methodOK && hasCalleeEvidence(reg, methodValue) {
		return methodValue, true
	}
	receiverValue, ok := readexpr.Project(config, ctx.Point, receiverPath, in)
	if !ok {
		return product.Value{}, false
	}
	return methodValueFromReceiverValue(reg, typeValues, receiverValue, method)
}

func methodValueFromReceiverValue(reg *axis.Registry, typeValues *typevalue.Cache, receiverValue product.Value, method string) (product.Value, bool) {
	receiverType, ok := typevalue.WitnessOf(reg, receiverValue)
	if !ok || typ.IsAny(receiverType) || typ.IsUnknown(receiverType) || typ.IsNever(receiverType) {
		return product.Value{}, false
	}
	return methodTypeValue(reg, typeValues, receiverType, method)
}

// methodCalleeValue resolves the callee of a colon-method call whose receiver
// is an anonymous prior call result (a builder chain). The receiver carries no
// symbol path, so the method is resolved from the receiver's value type with
// the same member-call mechanism the diagnostics layer uses for argument
// checks: the receiver type's member must be a concrete callable, and Self is
// bound to the receiver type. When the receiver type is any/top or the member
// is unresolved, no value is produced so the result stays top.
func methodCalleeValue(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	sources sourcevalue.SourceValues,
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	method := site.MethodName()
	if method == "" || sources == nil {
		return product.Value{}, false
	}
	source, ok := site.ReceiverSource()
	if !ok {
		return product.Value{}, false
	}
	receiverValue, ok := sources.ValueOfSource(ctx.Point, source, in, read)
	if !ok {
		return product.Value{}, false
	}
	receiverType, ok := typevalue.WitnessOf(reg, receiverValue)
	if !ok || typ.IsAny(receiverType) || typ.IsUnknown(receiverType) || typ.IsNever(receiverType) {
		return product.Value{}, false
	}
	return methodTypeValue(reg, typeValues, receiverType, method)
}

func methodTypeValue(reg *axis.Registry, typeValues *typevalue.Cache, receiverType typ.Type, method string) (product.Value, bool) {
	methodType, status, ok := typecall.MemberCallable(receiverType, method)
	if status != typecall.MemberCallOK || !ok || methodType == nil {
		return product.Value{}, false
	}
	return typeValues.FromTypeWithWitness(reg, methodType), true
}

func explicitAnyReceiverMethodOutcomeProvider(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
) callpayload.CallOutcomeProgram {
	shape := func(_ transfer.NodeContext, site factflow.CallSiteView) (callpayload.CallOutcomeSiteShape, error) {
		if site.MethodName() == "" {
			return callpayload.CallOutcomeSiteShape{}, nil
		}
		if _, present := site.ReceiverSource(); !present {
			return callpayload.CallOutcomeSiteShape{}, nil
		}
		return callpayload.CallOutcomeSiteShape{
			FieldNames: []string{"Results"},
		}, nil
	}
	evaluate := func(ctx transfer.NodeContext, site factflow.CallSiteView, input callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		if reg == nil || typeValues == nil || site.MethodName() == "" {
			return callpayload.CallOutcome{}, nil
		}
		receiverValue, ok := input.Receiver()
		if !ok || !product.Get(reg, receiverValue, evidence.Key).IsExplicitTop() {
			return callpayload.CallOutcome{}, nil
		}
		value := typeValues.FromTypeWithWitness(reg, typ.Any)
		value = sourcevalue.InheritTopOriginEvidence(reg, value, receiverValue)
		capacity := site.ResultTargetCount()
		if capacity < 1 {
			capacity = 1
		}
		results := make([]callpayload.CallResult, 0, capacity)
		seen := make(map[int]struct{}, site.ResultTargetCount())
		site.ForEachResultTarget(func(target factflow.CallResultTargetView) bool {
			index := target.ResultIndex()
			if index < 0 {
				return true
			}
			if _, ok := seen[index]; ok {
				return true
			}
			seen[index] = struct{}{}
			results = append(results, callpayload.CallResult{Index: index, Value: value})
			return true
		})
		if len(results) == 0 {
			results = append(results, callpayload.CallResult{Index: 0, Value: value})
		}
		return callpayload.CallOutcome{Results: results}, nil
	}
	return callpayload.SealObservedCallOutcomeProgram(
		"explicit any-receiver method outcome", []string{"Results"},
		state.LaneSet{}, state.LaneSet{}, callpayload.ObserveCallOutcomeOperands(false, true), shape, nil, evaluate,
	)
}

// explicitAnyCalleeOutcomeProvider propagates an explicit any boundary through
// ordinary direct and dotted calls. It deliberately relies on the callee value
// and its provenance, never a callee spelling: a manifest signature can only
// win through the separately resolved exact static path.
func explicitAnyCalleeOutcomeProvider(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
) callpayload.CallOutcomeProgram {
	shape := func(_ transfer.NodeContext, site factflow.CallSiteView) (callpayload.CallOutcomeSiteShape, error) {
		if site.MethodName() != "" {
			return callpayload.CallOutcomeSiteShape{}, nil
		}
		if _, present := site.CalleeSource(); !present {
			return callpayload.CallOutcomeSiteShape{}, nil
		}
		return callpayload.CallOutcomeSiteShape{FieldNames: []string{"Results"}}, nil
	}
	evaluate := func(ctx transfer.NodeContext, site factflow.CallSiteView, input callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		if reg == nil || typeValues == nil || site.MethodName() != "" {
			return callpayload.CallOutcome{}, nil
		}
		callee, ok := input.Callee()
		if !ok || !product.Get(reg, callee, evidence.Key).IsExplicitTop() {
			return callpayload.CallOutcome{}, nil
		}
		value := sourcevalue.InheritTopOriginEvidence(reg, typeValues.FromTypeWithWitness(reg, typ.Any), callee)
		capacity := site.ResultTargetCount()
		if capacity < 1 {
			capacity = 1
		}
		results := make([]callpayload.CallResult, 0, capacity)
		site.ForEachResultTarget(func(target factflow.CallResultTargetView) bool {
			if target.ResultIndex() >= 0 {
				results = append(results, callpayload.CallResult{Index: target.ResultIndex(), Value: value})
			}
			return true
		})
		if len(results) == 0 {
			results = append(results, callpayload.CallResult{Index: 0, Value: value})
		}
		return callpayload.CallOutcome{Results: results}, nil
	}
	return callpayload.SealObservedCallOutcomeProgram(
		"explicit any callee outcome", []string{"Results"},
		state.LaneSet{}, state.LaneSet{}, callpayload.ObserveCallOutcomeOperands(true, false), shape, nil, evaluate,
	)
}

func hasCalleeEvidence(reg *axis.Registry, value product.Value) bool {
	if typevalue.HasWitness(reg, value) {
		return true
	}
	if _, ok := product.Get(reg, value, identity.Key).ID(); ok {
		return true
	}
	return product.Get(reg, value, runtimekind.Key).Contains(runtimekind.Function)
}
