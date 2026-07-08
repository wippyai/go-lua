package projectsummary

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/memberaccess"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
	"github.com/wippyai/go-lua/compiler/ast"
)

type callSiteReader interface {
	CallSite(cfg.Point) (factflow.CallSite, bool)
}

type callSiteViewReader interface {
	CallSiteView(cfg.Point) (factflow.CallSiteView, bool)
}

type callOutcomeAtReader interface {
	CallOutcomeAt(cfg.Point) (callpayload.CallOutcome, bool)
}

type callSignatureViewReader interface {
	CallSiteViewSignatureType(factflow.CallSiteView) (*typ.Function, bool)
}

type expressionOperationRefReader interface {
	ExpressionOperationRef(factflow.ExprRef) (factflow.ExpressionOperation, bool)
}

type sourceValueAtBoundaryReader interface {
	SourceValueAtBoundary(cfg.Point, factflow.ValueSource) (product.Value, bool)
}

type sourceValueBeforeBoundaryReader interface {
	SourceValueBeforeBoundary(cfg.Point, factflow.ValueSource) (product.Value, bool)
}

type symbolTypeAnnotationReader interface {
	SymbolTypeAnnotation(symbol.ID) (ast.TypeExpr, bool)
}

type symbolNameReader interface {
	SymbolName(symbol.ID) string
}

type pathValueAtBoundaryReader interface {
	PathValueAtBoundary(cfg.Point, pathdom.Path) (product.Value, bool)
}

type pathEquivalentAtBoundaryReader interface {
	PathsEquivalentAtBoundary(cfg.Point, pathdom.Path, pathdom.Path) bool
}

type pathKeyAtBoundaryReader interface {
	PathKeyAtBoundary(cfg.Point, pathdom.Path) (pathdom.PathKey, bool)
}

type stateAtBoundaryReader interface {
	StateAtBoundary(cfg.Point) (state.State, bool)
}

type typeBindingReader interface {
	TypeRef(*ast.TypeRefExpr) (bind.TypeDecl, bool)
	PrimitiveTypeRef(*ast.PrimitiveTypeExpr) (bind.TypeDecl, bool)
	TypeDefParams(*ast.TypeDefStmt) []bind.TypeDecl
}

type functionIdentityReader interface {
	Function() *ast.FunctionExpr
	FunctionSymbol(*ast.FunctionExpr) (symbol.ID, bool)
	FunctionBySymbol(symbol.ID) (*ast.FunctionExpr, bool)
	FunctionOrigin(*ast.FunctionExpr) (bind.FunctionOrigin, bool)
}

func projectParamObligations(reg *axis.Registry, result ResultReader, cache *paramObligationProjectorCache) []product.Value {
	params := parameterValuePaths(result)
	if reg == nil || len(params) == 0 {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	ctx := newParamObligationProjector(reg, result, params, graph, cache)
	out := make([]product.Value, len(params))
	for i := range out {
		out[i] = product.Top()
	}
	for _, point := range graph.RPO() {
		ctx.point = point
		if site, ok := callSiteViewAt(result, point); ok {
			site.ForEachArgumentSource(func(_ int, source factflow.ValueSource) bool {
				ctx.addArithmeticObligationsFromSource(out, source)
				return true
			})
			ctx.addTypedCallObligations(out, site)
			ctx.addCallOutcomeObligations(out, site)
		}
		if sourceReader, ok := result.(returnValueSourceReader); ok {
			if sources, sourcesOK := sourceReader.ReturnValueSources(point); sourcesOK {
				for _, source := range sources {
					ctx.addArithmeticObligationsFromSource(out, source)
				}
			}
		}
		if assignment, ok := rootAssignmentAt(result, point); ok {
			ctx.addArithmeticObligationsFromSource(out, assignment.Source())
		}
		if assignment, ok := pathAssignmentAt(result, point); ok {
			ctx.addArithmeticObligationsFromSource(out, assignment.Source())
		}
		if write, ok := dynamicIndexWriteAt(result, point); ok {
			ctx.addArithmeticObligationsFromSource(out, write.KeySource())
			ctx.addArithmeticObligationsFromSource(out, write.Source())
		}
	}
	return out
}

func projectParamMemberCallObligations(reg *axis.Registry, result ResultReader, cache *paramObligationProjectorCache) []summary.ParamMemberCallObligation {
	params := parameterValuePaths(result)
	if reg == nil || len(params) == 0 {
		return nil
	}
	graph := result.Graph()
	if graph == nil || !hasCallSiteView(result) {
		return nil
	}
	ctx := newParamObligationProjector(reg, result, params, graph, cache)
	var out []summary.ParamMemberCallObligation
	for _, point := range graph.RPO() {
		ctx.point = point
		site, ok := callSiteViewAt(result, point)
		if !ok {
			continue
		}
		out = append(out, ctx.memberCallObligations(site)...)
	}
	return out
}

func callSiteViewAt(result ResultReader, point cfg.Point) (factflow.CallSiteView, bool) {
	if reader, ok := result.(callSiteViewReader); ok {
		return reader.CallSiteView(point)
	}
	if reader, ok := result.(callSiteReader); ok {
		site, ok := reader.CallSite(point)
		if !ok {
			return factflow.CallSiteView{}, false
		}
		return site.View(), true
	}
	return factflow.CallSiteView{}, false
}

func hasCallSiteView(result ResultReader) bool {
	if _, ok := result.(callSiteViewReader); ok {
		return true
	}
	_, ok := result.(callSiteReader)
	return ok
}

func rootAssignmentAt(result ResultReader, point cfg.Point) (factflow.RootAssignment, bool) {
	reader, ok := result.(rootAssignmentReader)
	if !ok {
		return factflow.RootAssignment{}, false
	}
	return reader.RootAssignment(point)
}

func localRootAssignmentAt(result ResultReader, point cfg.Point) (factflow.RootAssignment, bool) {
	assignment, ok := rootAssignmentAt(result, point)
	if !ok || assignment.Kind() != factflow.RootAssignmentLocalDeclaration {
		return factflow.RootAssignment{}, false
	}
	return assignment, true
}

func ordinaryRootAssignmentAt(result ResultReader, point cfg.Point) (factflow.RootAssignment, bool) {
	assignment, ok := rootAssignmentAt(result, point)
	if !ok || assignment.Kind() != factflow.RootAssignmentOrdinaryRootWrite {
		return factflow.RootAssignment{}, false
	}
	return assignment, true
}

func pathAssignmentAt(result ResultReader, point cfg.Point) (factflow.PathAssignment, bool) {
	reader, ok := result.(pathAssignmentReader)
	if !ok {
		return factflow.PathAssignment{}, false
	}
	return reader.PathAssignment(point)
}

func pathDescendantInvalidationAt(result ResultReader, point cfg.Point) (factflow.PathDescendantInvalidation, bool) {
	reader, ok := result.(pathDescendantInvalidationReader)
	if !ok {
		return factflow.PathDescendantInvalidation{}, false
	}
	return reader.PathDescendantInvalidation(point)
}

func dynamicIndexWriteAt(result ResultReader, point cfg.Point) (factflow.DynamicIndexWrite, bool) {
	reader, ok := result.(dynamicIndexWriteReader)
	if !ok {
		return factflow.DynamicIndexWrite{}, false
	}
	return reader.DynamicIndexWrite(point)
}

func projectParamMemberReturnSlots(reg *axis.Registry, result ResultReader, cache *paramObligationProjectorCache) []summary.ParamMemberReturnSlot {
	params := parameterValuePaths(result)
	if reg == nil || len(params) == 0 {
		return nil
	}
	graph := result.Graph()
	sourceReader, hasSources := result.(returnValueSourceReader)
	if graph == nil || !hasSources || !hasCallSiteView(result) {
		return nil
	}
	ctx := newParamObligationProjector(reg, result, params, graph, cache)
	var out []summary.ParamMemberReturnSlot
	for _, returnPoint := range result.ReturnPoints() {
		sources, ok := sourceReader.ReturnValueSources(returnPoint)
		if !ok {
			continue
		}
		callPoint, slots, ok := delegatedReturnCallSlots(result, sources)
		if !ok || len(slots) == 0 {
			continue
		}
		site, ok := callSiteViewAt(result, callPoint)
		if !ok {
			continue
		}
		receiver, member, ok := memberCallReceiverFromSite(site)
		if !ok {
			continue
		}
		ctx.point = callPoint
		receiverParam, ok := ctx.unconditionalPathParamIndex(receiver)
		if !ok {
			continue
		}
		for returnIndex, memberResultIndex := range slots {
			out = append(out, summary.ParamMemberReturnSlot{
				ReceiverParam:     receiverParam,
				Member:            member,
				ReturnIndex:       returnIndex,
				MemberResultIndex: memberResultIndex,
			})
		}
	}
	return out
}

func projectCapturedPathObligations(reg *axis.Registry, result ResultReader, cache *paramObligationProjectorCache) []summary.CapturedPathObligation {
	if reg == nil || !hasCallSiteView(result) {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	captureReader, ok := result.(functionCaptureReader)
	if !ok {
		return nil
	}
	captured := capturedSinkSymbols(captureReader)
	if len(captured) == 0 {
		return nil
	}
	ctx := newParamObligationProjector(reg, result, nil, graph, cache)
	var out []summary.CapturedPathObligation
	for _, point := range graph.RPO() {
		ctx.point = point
		site, ok := callSiteViewAt(result, point)
		if !ok {
			continue
		}
		ctx.addCapturedCallOutcomeObligations(&out, site, captured)
		ctx.addCapturedTypedCallObligations(&out, site, captured)
	}
	return out
}

func delegatedReturnCallSlots(result ResultReader, sources []factflow.ValueSource) (cfg.Point, map[int]int, bool) {
	if len(sources) == 0 {
		return 0, nil, false
	}
	first := sources[0]
	if first.Kind != factflow.ValueSourceCall || !first.HasCallPoint {
		return 0, nil, false
	}
	slots := make(map[int]int, len(sources))
	if len(sources) == 1 {
		if first.OpenTail {
			if reader, ok := result.(callOutcomeAtReader); ok {
				if outcome, ok := reader.CallOutcomeAt(first.CallPoint); ok {
					for _, ret := range outcome.Results {
						if ret.Index >= 0 {
							slots[ret.Index] = ret.Index
						}
					}
				}
			}
		}
		if len(slots) == 0 && first.ResultIndex >= 0 {
			slots[0] = first.ResultIndex
		}
		return first.CallPoint, slots, len(slots) != 0
	}
	for returnIndex, source := range sources {
		if source.Kind != factflow.ValueSourceCall ||
			!source.HasCallPoint ||
			source.CallPoint != first.CallPoint ||
			!source.Expanded ||
			source.ResultIndex < 0 {
			return 0, nil, false
		}
		if _, exists := slots[returnIndex]; exists {
			return 0, nil, false
		}
		slots[returnIndex] = source.ResultIndex
	}
	return first.CallPoint, slots, len(slots) != 0
}

type paramObligationProjector struct {
	reg       *axis.Registry
	result    ResultReader
	params    []pathdom.Path
	resolver  typeannotation.Resolver
	reach     *cfg.Reachability
	dom       *dominance.ImmediateDominators
	stability map[memberReceiverStabilityKey]bool
	point     cfg.Point
}

type paramObligationProjectorCache struct {
	reach     *cfg.Reachability
	dom       *dominance.ImmediateDominators
	stability map[memberReceiverStabilityKey]bool
}

func newParamObligationProjectorCache(graph cfg.Graph) *paramObligationProjectorCache {
	return &paramObligationProjectorCache{
		reach:     cfg.NewReachability(graph),
		dom:       dominance.ComputeImmediateDominatorInfo(graph),
		stability: make(map[memberReceiverStabilityKey]bool),
	}
}

func newParamObligationProjector(
	reg *axis.Registry,
	result ResultReader,
	params []pathdom.Path,
	graph cfg.Graph,
	cache *paramObligationProjectorCache,
) paramObligationProjector {
	if cache == nil {
		cache = newParamObligationProjectorCache(graph)
	}
	return paramObligationProjector{
		reg:       reg,
		result:    result,
		params:    params,
		resolver:  paramObligationTypeResolver(result),
		reach:     cache.reach,
		dom:       cache.dom,
		stability: cache.stability,
	}
}

type memberReceiverStabilityKey struct {
	point    cfg.Point
	receiver pathdom.PathKey
	member   segment.Segment
}

func (p paramObligationProjector) addCallOutcomeObligations(out []product.Value, site factflow.CallSiteView) {
	if p.selfCallSymbol(site.CalleeSymbol()) {
		return
	}
	if receiver, member, ok := memberCallReceiverFromSite(site); ok && !p.memberCallReceiverStable(receiver, member) {
		return
	}
	reader, ok := p.result.(callOutcomeAtReader)
	if !ok {
		return
	}
	outcome, ok := reader.CallOutcomeAt(p.point)
	if !ok {
		return
	}
	for _, obligation := range outcome.ParamObligations {
		source, sourceOK := site.ArgumentSourceAt(obligation.ParamIndex)
		if obligation.ParamIndex < 0 || !sourceOK {
			continue
		}
		if want, ok := typevalue.TypeOf(p.reg, obligation.Value); ok && p.sourceValueSatisfiesType(source, want) {
			continue
		}
		param, ok := p.unconditionalSourceParamIndex(source)
		if !ok {
			continue
		}
		p.add(out, param, obligation.Value)
	}
	for _, obligation := range outcome.PathObligations {
		p.addPathValueObligation(out, obligation.Path, obligation.Value, 0)
	}
}

func (p paramObligationProjector) addCapturedCallOutcomeObligations(out *[]summary.CapturedPathObligation, site factflow.CallSiteView, captured map[symbol.ID]struct{}) {
	if p.selfCallSymbol(site.CalleeSymbol()) {
		return
	}
	if receiver, member, ok := memberCallReceiverFromSite(site); ok && !p.memberCallReceiverStable(receiver, member) {
		return
	}
	reader, ok := p.result.(callOutcomeAtReader)
	if !ok {
		return
	}
	outcome, ok := reader.CallOutcomeAt(p.point)
	if !ok {
		return
	}
	for _, obligation := range outcome.ParamObligations {
		source, sourceOK := site.ArgumentSourceAt(obligation.ParamIndex)
		if obligation.ParamIndex < 0 || !sourceOK {
			continue
		}
		p.addCapturedSourceValueObligation(out, source, obligation.Value, captured, 0)
	}
	for _, obligation := range outcome.PathObligations {
		p.addCapturedPathValueObligation(out, obligation.Path, obligation.Value, captured, 0)
	}
}

func (p paramObligationProjector) selfCallSymbol(callee symbol.ID) bool {
	if callee == 0 {
		return false
	}
	reader, ok := p.result.(functionIdentityReader)
	if !ok {
		return false
	}
	fn := reader.Function()
	if fn == nil {
		return false
	}
	if origin, ok := reader.FunctionOrigin(fn); ok && origin.HasTargetSymbol && origin.TargetSymbol == callee {
		return true
	}
	if calleeFn, ok := reader.FunctionBySymbol(callee); ok {
		if calleeFn == fn {
			return true
		}
	}
	current, ok := reader.FunctionSymbol(fn)
	return ok && current != 0 && current == callee
}

func (p paramObligationProjector) addTypedCallObligations(out []product.Value, site factflow.CallSiteView) {
	params := p.callParamTypesForSite(site)
	if len(params) == 0 {
		return
	}
	site.ForEachArgumentSource(func(i int, source factflow.ValueSource) bool {
		if i < len(params) {
			p.addTypedValueSourceObligation(out, source, params[i], 0)
		}
		return true
	})
}

func (p paramObligationProjector) addCapturedTypedCallObligations(out *[]summary.CapturedPathObligation, site factflow.CallSiteView, captured map[symbol.ID]struct{}) {
	params := p.callParamTypesForSite(site)
	if len(params) == 0 {
		return
	}
	site.ForEachArgumentSource(func(i int, source factflow.ValueSource) bool {
		if i < len(params) {
			p.addCapturedTypedValueSourceObligation(out, source, params[i], captured, 0, nil)
		}
		return true
	})
}

func (p paramObligationProjector) valueSatisfiesType(value product.Value, want typ.Type) bool {
	got, ok := typevalue.TypeOf(p.reg, value)
	if !ok || got == nil || typ.IsAny(got) || typ.IsUnknown(got) || typ.IsNever(got) {
		return false
	}
	return subtype.IsSubtype(got, want)
}

func (p paramObligationProjector) addPathValueObligation(out []product.Value, path pathdom.Path, value product.Value, depth int) {
	if depth > typ.DefaultRecursionDepth || !summary.UsefulParamObligation(p.reg, value) {
		return
	}
	if param, ok := p.unconditionalPathParamIndex(path); ok {
		p.add(out, param, value)
		return
	}
	source, ok := p.stableLocalPathSource(path)
	if !ok {
		return
	}
	want, ok := typevalue.TypeOf(p.reg, value)
	if !ok {
		return
	}
	p.addTypedValueSourceObligation(out, source, want, depth+1)
}

func (p paramObligationProjector) addCapturedTypedValueSourceObligation(
	out *[]summary.CapturedPathObligation,
	source factflow.ValueSource,
	want typ.Type,
	captured map[symbol.ID]struct{},
	depth int,
	seen map[factflow.ExprRef]struct{},
) {
	if depth > typ.DefaultRecursionDepth || want == nil {
		return
	}
	if value, ok := obligationValueFromType(p.reg, want); ok {
		p.addCapturedSourceValueObligation(out, source, value, captured, depth+1)
	}
	if sourcePath, ok := p.valueSourcePath(source); ok {
		if localSource, sourceOK := p.stableLocalPathSource(sourcePath); sourceOK {
			p.addCapturedTypedValueSourceObligation(out, localSource, want, captured, depth+1, seen)
			return
		}
	}
	if !source.HasExpr {
		return
	}
	if _, ok := seen[source.ExprRef]; ok {
		return
	}
	if seen == nil {
		seen = make(map[factflow.ExprRef]struct{}, 1)
	}
	seen[source.ExprRef] = struct{}{}
	reader, ok := p.result.(expressionOperationRefReader)
	if !ok {
		return
	}
	op, ok := reader.ExpressionOperationRef(source.ExprRef)
	if !ok || op.Kind() != factflow.ExpressionOperationBinary || op.Op() != ".." || !concatResultSatisfies(want) {
		return
	}
	p.addCapturedConcatOperandSourceObligation(out, op.Left(), captured, depth+1, seen)
	p.addCapturedConcatOperandSourceObligation(out, op.Right(), captured, depth+1, seen)
}

func (p paramObligationProjector) addCapturedSourceValueObligation(
	out *[]summary.CapturedPathObligation,
	source factflow.ValueSource,
	value product.Value,
	captured map[symbol.ID]struct{},
	depth int,
) {
	if depth > typ.DefaultRecursionDepth {
		return
	}
	sourcePath, ok := p.valueSourcePath(source)
	if !ok {
		return
	}
	p.addCapturedPathValueObligation(out, sourcePath, value, captured, depth+1)
}

func (p paramObligationProjector) addCapturedPathValueObligation(out *[]summary.CapturedPathObligation, path pathdom.Path, value product.Value, captured map[symbol.ID]struct{}, depth int) {
	if out == nil || depth > typ.DefaultRecursionDepth || !summary.UsefulParamObligation(p.reg, value) {
		return
	}
	if stable, ok := p.stableCapturedPathKey(path, captured); ok {
		*out = append(*out, summary.CapturedPathObligation{Path: stable, Value: value})
		return
	}
	if source, ok := p.stableLocalPathSource(path); ok {
		if want, typeOK := typevalue.TypeOf(p.reg, value); typeOK {
			p.addCapturedTypedValueSourceObligation(out, source, want, captured, depth+1, nil)
		}
	}
}

func (p paramObligationProjector) addCapturedConcatOperandSourceObligation(
	out *[]summary.CapturedPathObligation,
	source factflow.ValueSource,
	captured map[symbol.ID]struct{},
	depth int,
	seen map[factflow.ExprRef]struct{},
) {
	if source.HasExpr {
		if reader, ok := p.result.(expressionOperationRefReader); ok {
			if op, opOK := reader.ExpressionOperationRef(source.ExprRef); opOK &&
				op.Kind() == factflow.ExpressionOperationBinary &&
				op.Op() == ".." {
				p.addCapturedTypedValueSourceObligation(out, source, typ.String, captured, depth+1, seen)
				return
			}
		}
	}
	p.addCapturedTypedValueSourceObligation(out, source, concatOperandObligationType(), captured, depth+1, seen)
}

func (p paramObligationProjector) stableCapturedPathKey(path pathdom.Path, captured map[symbol.ID]struct{}) (pathaddr.StableKey, bool) {
	if path.IsEmpty() || path.Symbol == 0 {
		return "", false
	}
	if _, ok := captured[path.Symbol]; !ok {
		return "", false
	}
	if !p.capturedPathStableAtUse(path) {
		return "", false
	}
	key := pathaddr.SymbolStableKey(path.Symbol, path.Segments)
	return key, key != ""
}

func (p paramObligationProjector) capturedPathStableAtUse(path pathdom.Path) bool {
	if path.Symbol == 0 {
		return false
	}
	graph := p.result.Graph()
	if graph == nil {
		return false
	}
	for _, point := range graph.RPO() {
		if point == p.point {
			continue
		}
		if !p.canReach(graph, graph.Entry(), point) || !p.canReach(graph, point, p.point) {
			continue
		}
		if p.assignmentInvalidatesSymbolRoot(point, path.Symbol) {
			return false
		}
	}
	return true
}

func concatResultSatisfies(want typ.Type) bool {
	return want != nil && subtype.IsSubtype(typ.String, want)
}

func concatOperandObligationType() typ.Type {
	return normalize.UnionForEvidence(typ.String, typ.Number)
}

func (p paramObligationProjector) callParamTypesForSite(site factflow.CallSiteView) []typ.Type {
	return p.callParamTypesForSiteWithReceiver(site, site.MethodName() != "")
}

func (p paramObligationProjector) callParamTypesForSiteWithReceiver(site factflow.CallSiteView, receiverSyntax bool) []typ.Type {
	receiver, member, hasMemberCall := memberCallReceiverFromSite(site)
	if sigReader, ok := p.result.(callSignatureViewReader); ok {
		if fn, ok := sigReader.CallSiteViewSignatureType(site); ok {
			return functionParamTypes(fn, false)
		}
	}
	if hasMemberCall {
		if _, _, receiverFromParam := p.unconditionalReceiverParamPath(receiver); receiverFromParam && !p.memberCallReceiverStable(receiver, member) {
			return nil
		}
		receiverType, ok := p.receiverType(receiver)
		if ok {
			fn, status, ok := memberaccess.Callable(receiverType, member)
			if status == typecall.MemberCallOK && ok {
				consumeReceiver := receiverSyntax && typecall.CallableConsumesReceiver(fn, receiverType)
				return functionParamTypes(fn, consumeReceiver)
			}
		}
	}
	if fn, ok := p.directCallable(site); ok {
		return functionParamTypes(fn, false)
	}
	return nil
}

func (p paramObligationProjector) directCallable(site factflow.CallSiteView) (*typ.Function, bool) {
	sym := site.CalleeSymbol()
	if sym == 0 {
		return nil, false
	}
	annotationReader, ok := p.result.(symbolTypeAnnotationReader)
	if !ok {
		return nil, false
	}
	expr, ok := annotationReader.SymbolTypeAnnotation(sym)
	if !ok {
		return nil, false
	}
	base, ok := lowerParamObligationType(expr, p.resolver)
	if !ok || typ.IsAny(base) || typ.IsUnknown(base) {
		return nil, false
	}
	return typecall.Callable(base)
}

func (p paramObligationProjector) receiverType(receiver pathdom.Path) (typ.Type, bool) {
	if annotationReader, ok := p.result.(symbolTypeAnnotationReader); ok && receiver.Symbol != 0 {
		if expr, ok := annotationReader.SymbolTypeAnnotation(receiver.Symbol); ok {
			base, ok := lowerParamObligationType(expr, p.resolver)
			if ok && base != nil && !typ.IsAny(base) && !typ.IsUnknown(base) {
				if len(receiver.Segments) == 0 {
					return base, true
				}
				if projected, ok := luatypeprojection.ApplySegments(base, receiver.Segments); ok {
					return projected, true
				}
			}
		}
	}
	valueReader, ok := p.result.(pathValueAtBoundaryReader)
	if !ok {
		return nil, false
	}
	value, ok := valueReader.PathValueAtBoundary(p.point, receiver)
	if !ok {
		return nil, false
	}
	return paramObligationTypeFromValue(p.reg, value)
}

func (p paramObligationProjector) memberCallObligations(site factflow.CallSiteView) []summary.ParamMemberCallObligation {
	receiver, member, ok := memberCallReceiverFromSite(site)
	if !ok {
		return nil
	}
	if !p.memberCallReceiverStable(receiver, member) {
		return nil
	}
	receiverParam, receiverSuffix, ok := p.unconditionalReceiverParamPath(receiver)
	if !ok {
		return nil
	}
	var receiverPath pathaddr.SuffixKey
	if len(receiverSuffix) != 0 {
		receiverPath, ok = pathaddr.RelativeStaticMemberSuffixKey(receiverSuffix)
		if !ok {
			return nil
		}
	}
	memberOffset := 0
	if site.MethodName() != "" {
		memberOffset = 1
	}
	params := p.callParamTypesForSiteWithReceiver(site, memberOffset != 0)
	var out []summary.ParamMemberCallObligation
	site.ForEachArgumentSource(func(i int, source factflow.ValueSource) bool {
		argParam, ok := p.unconditionalSourceParamIndex(source)
		if !ok {
			return true
		}
		memberParamIndex := i + memberOffset
		if memberParamIndex < len(params) &&
			p.sourceValueSatisfiesType(source, params[memberParamIndex]) &&
			p.paramHasDeclaredType(argParam) {
			return true
		}
		out = append(out, summary.ParamMemberCallObligation{
			ReceiverParam:    receiverParam,
			ReceiverPath:     receiverPath,
			Member:           member,
			ArgParam:         argParam,
			MemberParamIndex: memberParamIndex,
			SubjectLabel:     p.memberCallArgumentSubjectLabel(i, p.params[argParam]),
			ProviderLabel:    p.memberCallProviderLabel(receiver, member),
		})
		return true
	})
	return out
}

func (p paramObligationProjector) paramHasDeclaredType(param int) bool {
	if param < 0 || param >= len(p.params) {
		return false
	}
	sym := p.params[param].Symbol
	if sym == 0 {
		return false
	}
	annotationReader, ok := p.result.(symbolTypeAnnotationReader)
	if !ok {
		return false
	}
	_, ok = annotationReader.SymbolTypeAnnotation(sym)
	return ok
}

func (p paramObligationProjector) memberCallArgumentSubjectLabel(index int, param pathdom.Path) string {
	if param.IsEmpty() {
		return ""
	}
	return "argument " + strconv.Itoa(index+1) + " (" + p.pathLabel(param) + ")"
}

func (p paramObligationProjector) memberCallProviderLabel(receiver pathdom.Path, member segment.Segment) string {
	if receiver.IsEmpty() {
		return ""
	}
	return p.pathLabel(receiver) + segment.FormatSegments([]segment.Segment{member})
}

func (p paramObligationProjector) pathLabel(path pathdom.Path) string {
	if path.Symbol != 0 {
		if names, ok := p.result.(symbolNameReader); ok {
			if name := names.SymbolName(path.Symbol); name != "" {
				return name + segment.FormatSegments(path.Segments)
			}
		}
	}
	return path.String()
}

func (p paramObligationProjector) memberCallReceiverStable(receiver pathdom.Path, member segment.Segment) bool {
	if receiver.IsEmpty() || !memberaccess.Valid(member) {
		return false
	}
	key := memberReceiverStabilityKey{
		point:    p.point,
		receiver: receiver.Key(),
		member:   member,
	}
	if p.stability != nil {
		if stable, ok := p.stability[key]; ok {
			return stable
		}
	}
	stable := p.computeMemberCallReceiverStable(receiver, member)
	if p.stability != nil {
		p.stability[key] = stable
	}
	return stable
}

func (p paramObligationProjector) computeMemberCallReceiverStable(receiver pathdom.Path, member segment.Segment) bool {
	graph := p.result.Graph()
	hasCallSites := hasCallSiteView(p.result)
	callOutcomeResult, hasCallOutcomes := p.result.(callOutcomeAtReader)
	if graph == nil {
		return true
	}
	for _, point := range graph.RPO() {
		if point == p.point {
			return true
		}
		if hasCallSites && hasCallOutcomes && p.canReach(graph, point, p.point) {
			if site, siteOK := callSiteViewAt(p.result, point); siteOK {
				if outcome, outcomeOK := callOutcomeResult.CallOutcomeAt(point); outcomeOK &&
					p.callOutcomeInvalidatesMemberReceiver(site, outcome, receiver, member) {
					return false
				}
			}
		}
		if p.assignmentInvalidatesMemberCallReceiverAt(point, receiver, member) {
			return false
		}
	}
	return true
}

func (p paramObligationProjector) callOutcomeInvalidatesMemberReceiver(site factflow.CallSiteView, outcome callpayload.CallOutcome, receiver pathdom.Path, member segment.Segment) bool {
	appendSubstituted := func(targets *[]pathdom.Path, bindings []pathdom.Path, target pathdom.Path) {
		substituted, ok := target.Substitute(bindings)
		if !ok || substituted.IsEmpty() {
			return
		}
		*targets = append(*targets, substituted)
	}
	var targets []pathdom.Path
	argBindings := p.callArgumentBindings(site)
	callBindings := p.callBindings(site)
	for _, invalidation := range outcome.ParamPathInvalidations {
		appendSubstituted(&targets, argBindings, invalidation.Path)
	}
	for _, invalidation := range outcome.NormalReturnFacts.PathInvalidations {
		appendSubstituted(&targets, callBindings, invalidation.Path)
	}
	for _, memberWrite := range outcome.NormalReturnFacts.PathStaticMembers {
		if !pathStaticMemberHasClearingInvalidation(outcome.NormalReturnFacts.PathInvalidations, memberWrite.Path) {
			continue
		}
		appendSubstituted(&targets, callBindings, memberWrite.Path)
	}
	for _, target := range targets {
		for _, memberPath := range memberaccess.Paths(receiver, member) {
			if p.invalidationPathReachesMemberPath(target, memberPath) {
				return true
			}
		}
	}
	return false
}

func pathStaticMemberHasClearingInvalidation(invalidations []callboundary.PathInvalidationFact, memberPath pathdom.Path) bool {
	if memberPath.IsEmpty() {
		return false
	}
	for _, invalidation := range invalidations {
		if !invalidation.ClearTarget {
			continue
		}
		if pathsMatchIgnoringRootVersion(invalidation.Path, memberPath) {
			return true
		}
	}
	return false
}

func pathsMatchIgnoringRootVersion(left, right pathdom.Path) bool {
	if left.Symbol == 0 || right.Symbol == 0 || left.Symbol != right.Symbol {
		return false
	}
	if len(left.Segments) != len(right.Segments) {
		return false
	}
	for i := range left.Segments {
		if left.Segments[i] != right.Segments[i] {
			return false
		}
	}
	return true
}

func (p paramObligationProjector) callArgumentBindings(site factflow.CallSiteView) []pathdom.Path {
	pathReader, ok := p.result.(expressionPathRefReader)
	if !ok {
		return nil
	}
	var bindings []pathdom.Path
	site.ForEachArgumentSource(func(i int, source factflow.ValueSource) bool {
		sourcePath, ok := valueSourcePath(p.result, pathReader, source)
		if !ok || sourcePath.IsEmpty() {
			return true
		}
		bindings = appendPathBinding(bindings, i, sourcePath)
		return true
	})
	return bindings
}

func (p paramObligationProjector) callBindings(site factflow.CallSiteView) []pathdom.Path {
	return callSiteBindings(p.result, site)
}

func callSiteBindings(result ResultReader, site factflow.CallSiteView) []pathdom.Path {
	pathReader, ok := result.(expressionPathRefReader)
	if !ok {
		return nil
	}
	var bindings []pathdom.Path
	offset := 0
	if receiverPath, ok := site.ReceiverPath(); ok {
		bindings = appendPathBinding(bindings, 0, receiverPath)
		offset = 1
	}
	site.ForEachArgumentSource(func(i int, source factflow.ValueSource) bool {
		sourcePath, ok := valueSourcePath(result, pathReader, source)
		if !ok || sourcePath.IsEmpty() {
			return true
		}
		bindings = appendPathBinding(bindings, i+offset, sourcePath)
		return true
	})
	return bindings
}

func appendPathBinding(bindings []pathdom.Path, index int, value pathdom.Path) []pathdom.Path {
	if index < 0 || value.IsEmpty() {
		return bindings
	}
	for len(bindings) <= index {
		bindings = append(bindings, pathdom.Path{})
	}
	bindings[index] = value
	return bindings
}

func (p paramObligationProjector) canReach(graph cfg.Graph, from, to cfg.Point) bool {
	if p.reach != nil {
		return p.reach.CanReach(from, to)
	}
	return cfg.PointCanReach(graph, from, to)
}

func (p paramObligationProjector) dominates(point, dominated cfg.Point) bool {
	if p.dom != nil {
		return p.dom.Dominates(point, dominated)
	}
	graph := p.result.Graph()
	if graph == nil {
		return false
	}
	return dominance.Dominates(dominance.ComputeImmediateDominators(graph), point, dominated)
}

func projectPathHasPrefix(candidate, prefix pathdom.Path) bool {
	if candidate.Symbol != 0 || prefix.Symbol != 0 {
		if candidate.Symbol != prefix.Symbol || candidate.Version != prefix.Version {
			return false
		}
	} else if candidate.Root != prefix.Root || candidate.Version != prefix.Version {
		return false
	}
	if len(prefix.Segments) > len(candidate.Segments) {
		return false
	}
	for i := range prefix.Segments {
		if candidate.Segments[i] != prefix.Segments[i] {
			return false
		}
	}
	return true
}

func (p paramObligationProjector) assignmentInvalidatesMemberCallReceiverAt(point cfg.Point, receiver pathdom.Path, member segment.Segment) bool {
	if receiver.IsEmpty() || !memberaccess.Valid(member) {
		return false
	}
	if assignment, ok := ordinaryRootAssignmentAt(p.result, point); ok &&
		p.assignmentPathInvalidatesMemberCallReceiver(assignment.TargetPathRef(), receiver, member) {
		return true
	}
	if assignment, ok := pathAssignmentAt(p.result, point); ok &&
		p.assignmentPathInvalidatesMemberCallReceiver(assignment.TargetPathRef(), receiver, member) {
		return true
	}
	if invalidation, ok := pathDescendantInvalidationAt(p.result, point); ok &&
		p.assignmentPathInvalidatesMemberCallReceiver(invalidation.ContainerPathRef(), receiver, member) {
		return true
	}
	return false
}

func (p paramObligationProjector) assignmentPathInvalidatesMemberCallReceiver(target pathdom.Path, receiver pathdom.Path, member segment.Segment) bool {
	if target.IsEmpty() {
		return false
	}
	memberPaths := memberaccess.Paths(receiver, member)
	if target.Equal(receiver) {
		return true
	}
	for _, memberPath := range memberPaths {
		if p.invalidationPathReachesMemberPath(target, memberPath) {
			return true
		}
	}
	return false
}

func (p paramObligationProjector) invalidationPathReachesMemberPath(invalidated, memberPath pathdom.Path) bool {
	if invalidated.IsEmpty() || memberPath.IsEmpty() {
		return false
	}
	if projectPathHasPrefix(memberPath, invalidated) {
		return true
	}
	if len(invalidated.Segments) > len(memberPath.Segments) {
		return false
	}
	invalidatedRoot := invalidated.RootOnly()
	if !p.pathRootReadable(invalidatedRoot) && segmentsHavePrefix(memberPath.Segments, invalidated.Segments) {
		return true
	}
	for prefixLen := 0; prefixLen <= len(memberPath.Segments); prefixLen++ {
		if len(invalidated.Segments) > len(memberPath.Segments)-prefixLen {
			break
		}
		if !segmentsHavePrefix(memberPath.Segments[prefixLen:], invalidated.Segments) {
			continue
		}
		memberPrefix := memberPath
		memberPrefix.Segments = append(memberPath.Segments[:0:0], memberPath.Segments[:prefixLen]...)
		if p.pathsShareExactIdentity(invalidatedRoot, memberPrefix) {
			return true
		}
	}
	return false
}

func (p paramObligationProjector) pathRootReadable(root pathdom.Path) bool {
	valueReader, ok := p.result.(pathValueAtBoundaryReader)
	if !ok || root.IsEmpty() {
		return false
	}
	_, ok = valueReader.PathValueAtBoundary(p.point, root)
	return ok
}

func segmentsHavePrefix(candidate, prefix []segment.Segment) bool {
	if len(prefix) > len(candidate) {
		return false
	}
	for i := range prefix {
		if candidate[i] != prefix[i] {
			return false
		}
	}
	return true
}

func (p paramObligationProjector) pathsShareExactIdentity(left, right pathdom.Path) bool {
	if left.IsEmpty() || right.IsEmpty() || left.Equal(right) {
		return false
	}
	if equivalent, ok := p.result.(pathEquivalentAtBoundaryReader); ok && equivalent.PathsEquivalentAtBoundary(p.point, left, right) {
		return true
	}
	if p.pathsShareBoundaryEquivalence(left, right) {
		return true
	}
	valueReader, ok := p.result.(pathValueAtBoundaryReader)
	if !ok || p.reg == nil {
		return false
	}
	leftValue, leftOK := valueReader.PathValueAtBoundary(p.point, left)
	rightValue, rightOK := valueReader.PathValueAtBoundary(p.point, right)
	if !leftOK || !rightOK {
		return false
	}
	leftID, leftOK := product.Get(p.reg, leftValue, identity.Key).ID()
	rightID, rightOK := product.Get(p.reg, rightValue, identity.Key).ID()
	return leftOK && rightOK && leftID != (identity.ID{}) && leftID == rightID
}

func (p paramObligationProjector) pathsShareBoundaryEquivalence(left, right pathdom.Path) bool {
	stateReader, stateOK := p.result.(stateAtBoundaryReader)
	keyReader, keyOK := p.result.(pathKeyAtBoundaryReader)
	if !stateOK || !keyOK {
		return false
	}
	boundary, ok := stateReader.StateAtBoundary(p.point)
	if !ok {
		return false
	}
	ks := p.result.KeySpace()
	if ks == nil {
		return false
	}
	leftCarried := rootlessCarriedPathKey(left)
	rightCarried := rootlessCarriedPathKey(right)
	if leftCarried == "" && rightCarried == "" {
		return false
	}
	if !p.pathRootIsParameter(left) && !p.pathRootIsParameter(right) {
		return false
	}
	leftKeys := p.pathBoundaryEquivalenceKeys(keyReader, left, leftCarried)
	rightKeys := p.pathBoundaryEquivalenceKeys(keyReader, right, rightCarried)
	for _, leftKey := range leftKeys {
		if leftKey == "" {
			continue
		}
		for _, rightKey := range rightKeys {
			if rightKey == "" {
				continue
			}
			if leftKey == rightKey || pathKeyListContains(boundary.EquivalentPathKeys(ks, leftKey), rightKey) {
				return true
			}
			if pathKeyListContains(boundary.EquivalentPathKeys(ks, rightKey), leftKey) {
				return true
			}
		}
	}
	return false
}

func (p paramObligationProjector) pathBoundaryEquivalenceKeys(reader pathKeyAtBoundaryReader, target pathdom.Path, carried pathdom.PathKey) []pathdom.PathKey {
	var out []pathdom.PathKey
	if key, ok := reader.PathKeyAtBoundary(p.point, target); ok && key != "" {
		out = append(out, key)
	}
	if carried != "" && !pathKeyListContains(out, carried) {
		out = append(out, carried)
	}
	return out
}

func rootlessCarriedPathKey(target pathdom.Path) pathdom.PathKey {
	if target.Root != "" || target.Symbol == 0 || target.Version == 0 {
		return ""
	}
	return target.Key()
}

func (p paramObligationProjector) pathRootIsParameter(target pathdom.Path) bool {
	if target.IsEmpty() {
		return false
	}
	root := target.RootOnly()
	for _, param := range p.params {
		if param.Symbol != 0 && root.Symbol == param.Symbol {
			return true
		}
		if param.Symbol == 0 && root.Symbol == 0 && param.Root == root.Root {
			return true
		}
	}
	return false
}

func pathKeyListContains(keys []pathdom.PathKey, want pathdom.PathKey) bool {
	for _, key := range keys {
		if key == want {
			return true
		}
	}
	return false
}

func (p paramObligationProjector) addArithmeticObligationsFromSource(out []product.Value, source factflow.ValueSource) {
	p.addArithmeticObligationsFromSourceSeen(out, source, nil)
}

func (p paramObligationProjector) addArithmeticObligationsFromSourceSeen(
	out []product.Value,
	source factflow.ValueSource,
	seen map[factflow.ExprRef]struct{},
) {
	if !source.HasExpr {
		return
	}
	if _, ok := seen[source.ExprRef]; ok {
		return
	}
	if seen == nil {
		seen = make(map[factflow.ExprRef]struct{}, 1)
	}
	seen[source.ExprRef] = struct{}{}
	reader, ok := p.result.(expressionOperationRefReader)
	if !ok {
		return
	}
	op, ok := reader.ExpressionOperationRef(source.ExprRef)
	if ok {
		p.addArithmeticOperationObligationsSeen(out, op, seen)
	}
	if objectReader, ok := p.result.(objectLiteralExprReader); ok {
		if literal, literalOK := objectReader.ObjectLiteralView(source.ExprRef); literalOK {
			p.addArithmeticObligationsFromObjectLiteral(out, literal, seen)
		}
	}
}

func (p paramObligationProjector) addArithmeticObligationsFromObjectLiteral(
	out []product.Value,
	literal factflow.ObjectLiteralView,
	seen map[factflow.ExprRef]struct{},
) {
	literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		p.addArithmeticObligationsFromSourceSeen(out, entry.Source(), seen)
		return true
	})
	if source, ok := literal.ListElementSource(); ok {
		p.addArithmeticObligationsFromSourceSeen(out, source, seen)
	}
}

func (p paramObligationProjector) addArithmeticOperationObligationsSeen(
	out []product.Value,
	op factflow.ExpressionOperation,
	seen map[factflow.ExprRef]struct{},
) {
	switch op.Kind() {
	case factflow.ExpressionOperationBinary:
		left, right := op.Left(), op.Right()
		switch op.Op() {
		case "+", "-", "*", "/", "//", "%", "^", "&", "|", "~", "<<", ">>":
			p.addArithmeticOperandSourceSeen(out, left, seen)
			p.addArithmeticOperandSourceSeen(out, right, seen)
		}
		p.addArithmeticObligationsFromSourceSeen(out, left, seen)
		p.addArithmeticObligationsFromSourceSeen(out, right, seen)
	case factflow.ExpressionOperationUnary:
		operand := op.Left()
		switch op.Op() {
		case "-", "~":
			p.addArithmeticOperandSourceSeen(out, operand, seen)
		case "#":
			p.addLengthOperandSourceObligation(out, operand)
		}
		p.addArithmeticObligationsFromSourceSeen(out, operand, seen)
	}
}

func (p paramObligationProjector) addArithmeticOperandSourceSeen(
	out []product.Value,
	source factflow.ValueSource,
	seen map[factflow.ExprRef]struct{},
) {
	p.addTypedValueSourceObligationSeen(out, source, typ.Number, 0, seen)
}

func (p paramObligationProjector) addLengthOperandSourceObligation(out []product.Value, source factflow.ValueSource) {
	pathReader, ok := p.result.(expressionPathRefReader)
	if !ok {
		return
	}
	sourcePath, ok := valueSourcePath(p.result, pathReader, source)
	if !ok {
		return
	}
	param, suffix, ok := p.unconditionalReceiverParamPath(sourcePath)
	if !ok {
		return
	}
	want := lengthOperandObligationTypeAtSuffix(suffix)
	value, ok := obligationValueFromType(p.reg, want)
	if !ok {
		return
	}
	p.add(out, param, value)
}

func (p paramObligationProjector) addTypedValueSourceObligation(out []product.Value, source factflow.ValueSource, want typ.Type, depth int) {
	p.addTypedValueSourceObligationSeen(out, source, want, depth, nil)
}

func (p paramObligationProjector) addTypedValueSourceObligationSeen(
	out []product.Value,
	source factflow.ValueSource,
	want typ.Type,
	depth int,
	seen map[factflow.ExprRef]struct{},
) {
	if depth > typ.DefaultRecursionDepth || want == nil {
		return
	}
	if p.sourceValueSatisfiesType(source, want) {
		return
	}
	if value, ok := obligationValueFromType(p.reg, want); ok {
		if param, ok := p.unconditionalSourceParamIndex(source); ok {
			p.add(out, param, value)
			return
		}
		if param, suffix, ok := p.sourceParamSuffix(source); ok {
			if suffixedValue, valueOK := obligationValueFromType(p.reg, obligationTypeAtSuffix(want, suffix)); valueOK {
				p.add(out, param, suffixedValue)
				return
			}
		}
	}
	if sourcePath, ok := p.valueSourcePath(source); ok {
		if localSource, sourceOK := p.stableLocalPathSource(sourcePath); sourceOK {
			p.addTypedValueSourceObligationSeen(out, localSource, want, depth+1, seen)
			return
		}
	}
	if source.HasExpr {
		if reader, ok := p.result.(expressionOperationRefReader); ok {
			if op, opOK := reader.ExpressionOperationRef(source.ExprRef); opOK &&
				op.Kind() == factflow.ExpressionOperationBinary &&
				op.Op() == ".." &&
				concatResultSatisfies(want) {
				p.addConcatOperandSourceObligationSeen(out, op.Left(), seen)
				p.addConcatOperandSourceObligationSeen(out, op.Right(), seen)
				return
			}
		}
	}
	p.addArithmeticObligationsFromSourceSeen(out, source, seen)
}

func (p paramObligationProjector) addConcatOperandSourceObligationSeen(
	out []product.Value,
	source factflow.ValueSource,
	seen map[factflow.ExprRef]struct{},
) {
	if source.HasExpr {
		if reader, ok := p.result.(expressionOperationRefReader); ok {
			if op, opOK := reader.ExpressionOperationRef(source.ExprRef); opOK &&
				op.Kind() == factflow.ExpressionOperationBinary &&
				op.Op() == ".." {
				p.addTypedValueSourceObligationSeen(out, source, typ.String, 0, seen)
				return
			}
		}
	}
	p.addTypedValueSourceObligationSeen(out, source, concatOperandObligationType(), 0, seen)
}

func (p paramObligationProjector) sourceValueSatisfiesType(source factflow.ValueSource, want typ.Type) bool {
	if want == nil || p.reg == nil {
		return false
	}
	if valueReader, ok := p.result.(sourceValueBeforeBoundaryReader); ok {
		if value, valueOK := valueReader.SourceValueBeforeBoundary(p.point, source); valueOK {
			return p.valueSatisfiesType(value, want)
		}
	}
	if valueReader, ok := p.result.(sourceValueAtBoundaryReader); ok {
		if value, valueOK := valueReader.SourceValueAtBoundary(p.point, source); valueOK {
			return p.valueSatisfiesType(value, want)
		}
	}
	pathReader, ok := p.result.(expressionPathRefReader)
	if !ok {
		return false
	}
	valueReader, ok := p.result.(pathValueAtBoundaryReader)
	if !ok {
		return false
	}
	sourcePath, ok := valueSourcePath(p.result, pathReader, source)
	if !ok || sourcePath.IsEmpty() {
		return false
	}
	value, ok := valueReader.PathValueAtBoundary(p.point, sourcePath)
	if !ok {
		return false
	}
	return p.valueSatisfiesType(value, want)
}

func (p paramObligationProjector) valueSourcePath(source factflow.ValueSource) (pathdom.Path, bool) {
	pathReader, ok := p.result.(expressionPathRefReader)
	if !ok {
		return pathdom.Path{}, false
	}
	return valueSourcePath(p.result, pathReader, source)
}

func (p paramObligationProjector) unconditionalSourceParamIndex(source factflow.ValueSource) (int, bool) {
	sourcePath, ok := p.valueSourcePath(source)
	if !ok {
		return 0, false
	}
	return p.unconditionalPathParamIndex(sourcePath)
}

func (p paramObligationProjector) sourceParamSuffix(source factflow.ValueSource) (int, []segment.Segment, bool) {
	sourcePath, ok := p.valueSourcePath(source)
	if !ok {
		return 0, nil, false
	}
	return p.unconditionalReceiverParamPath(sourcePath)
}

func lengthOperandObligationTypeAtSuffix(suffix []segment.Segment) typ.Type {
	return obligationTypeAtSuffix(lengthOperandType(), suffix)
}

func lengthOperandType() typ.Type {
	return normalize.UnionForEvidence(typ.String, typetable.BuiltinTopMarker())
}

func obligationTypeAtSuffix(leaf typ.Type, suffix []segment.Segment) typ.Type {
	if leaf == nil || len(suffix) == 0 {
		return leaf
	}
	seg := suffix[len(suffix)-1]
	switch seg.Kind {
	case segment.SegmentField:
		return obligationTypeAtSuffix(
			typetable.NewRecord().Field(seg.Name, leaf).Build(),
			suffix[:len(suffix)-1],
		)
	default:
		return leaf
	}
}

func (p paramObligationProjector) add(out []product.Value, param int, value product.Value) {
	if param < 0 || param >= len(out) || !summary.UsefulParamObligation(p.reg, value) {
		return
	}
	if product.Equal(p.reg, out[param], product.Top()) {
		out[param] = value
		return
	}
	out[param] = product.Meet(p.reg, out[param], value)
}

func (p paramObligationProjector) unconditionalPathParamIndex(exprPath pathdom.Path) (int, bool) {
	index, ok := paramIndexForPath(exprPath, p.params)
	if !ok {
		return 0, false
	}
	if !p.paramUseUnconditional(index) {
		return 0, false
	}
	return index, true
}

func (p paramObligationProjector) stableLocalPathSource(local pathdom.Path) (factflow.ValueSource, bool) {
	if local.IsEmpty() || local.Symbol == 0 || len(local.Segments) != 0 {
		return factflow.ValueSource{}, false
	}
	graph := p.result.Graph()
	if graph == nil {
		return factflow.ValueSource{}, false
	}
	var sourcePoint cfg.Point
	var source factflow.ValueSource
	for _, point := range graph.RPO() {
		assignment, ok := localRootAssignmentAt(p.result, point)
		if !ok || assignment.TargetSymbol() != local.Symbol {
			continue
		}
		if !p.dominates(point, p.point) {
			continue
		}
		sourcePoint = point
		source = assignment.Source()
	}
	if source.Kind == factflow.ValueSourceUnknown {
		return factflow.ValueSource{}, false
	}
	for _, point := range graph.RPO() {
		if point == sourcePoint || point == p.point {
			continue
		}
		if !p.canReach(graph, sourcePoint, point) || !p.canReach(graph, point, p.point) {
			continue
		}
		if p.assignmentInvalidatesSymbolRoot(point, local.Symbol) {
			return factflow.ValueSource{}, false
		}
	}
	return source, true
}

func (p paramObligationProjector) assignmentInvalidatesSymbolRoot(point cfg.Point, sym symbol.ID) bool {
	if sym == 0 {
		return false
	}
	if assignment, ok := ordinaryRootAssignmentAt(p.result, point); ok {
		target := assignment.TargetPathRef()
		if assignment.TargetSymbol() == sym || target.Symbol == sym {
			return true
		}
	}
	if assignment, ok := pathAssignmentAt(p.result, point); ok && assignment.TargetPathRef().Symbol == sym {
		return true
	}
	if invalidation, ok := pathDescendantInvalidationAt(p.result, point); ok && invalidation.ContainerPathRef().Symbol == sym {
		return true
	}
	return false
}

func (p paramObligationProjector) unconditionalReceiverParamPath(receiver pathdom.Path) (int, []segment.Segment, bool) {
	index, suffix, ok := paramIndexAndSuffixForPath(receiver, p.params)
	if ok && p.paramUseUnconditional(index) {
		return index, suffix, true
	}
	source, ok := p.localAliasSourcePath(receiver)
	if !ok {
		return 0, nil, false
	}
	index, suffix, ok = paramIndexAndSuffixForPath(source, p.params)
	if !ok || !p.paramUseUnconditional(index) {
		return 0, nil, false
	}
	if len(receiver.Segments) != 0 {
		suffix = append(suffix[:len(suffix):len(suffix)], receiver.Segments...)
	}
	return index, suffix, true
}

func (p paramObligationProjector) localAliasSourcePath(receiver pathdom.Path) (pathdom.Path, bool) {
	if receiver.IsEmpty() || receiver.Symbol == 0 {
		return pathdom.Path{}, false
	}
	pathReader, ok := p.result.(expressionPathRefReader)
	if !ok {
		return pathdom.Path{}, false
	}
	graph := p.result.Graph()
	if graph == nil {
		return pathdom.Path{}, false
	}
	for _, point := range graph.RPO() {
		if point == p.point {
			return pathdom.Path{}, false
		}
		assignment, ok := localRootAssignmentAt(p.result, point)
		if !ok || assignment.TargetSymbol() != receiver.Symbol {
			continue
		}
		source, ok := valueSourcePath(p.result, pathReader, assignment.Source())
		if !ok || source.IsEmpty() {
			continue
		}
		return source, true
	}
	return pathdom.Path{}, false
}

func (p paramObligationProjector) paramUseUnconditional(index int) bool {
	if index < 0 || index >= len(p.params) {
		return false
	}
	slot := key.SymbolValue(p.params[index].Symbol)
	if slot == 0 {
		return false
	}
	if reassignedReader, ok := p.result.(reassignedParameterValueSlotReader); ok {
		if _, reassigned := reassignedReader.ReassignedParameterValueSlots()[slot]; reassigned {
			return false
		}
	}
	entryReader, ok := p.result.(entryStateReader)
	if !ok {
		return false
	}
	stateReader, ok := p.result.(stateAtReader)
	if !ok {
		return false
	}
	entry, ok := entryReader.EntryState()
	if !ok {
		return false
	}
	atPoint, ok := stateReader.StateAt(p.point)
	if !ok {
		return false
	}
	return product.Equal(p.reg, entry.ReadValue(p.reg, slot), atPoint.ReadValue(p.reg, slot))
}

func paramIndexForPath(p pathdom.Path, params []pathdom.Path) (int, bool) {
	index, suffix, ok := paramIndexAndSuffixForPath(p, params)
	return index, ok && len(suffix) == 0
}

func paramIndexAndSuffixForPath(p pathdom.Path, params []pathdom.Path) (int, []segment.Segment, bool) {
	if p.IsEmpty() {
		return 0, nil, false
	}
	for i, param := range params {
		if !pathRootEqual(p, param) || len(p.Segments) < len(param.Segments) {
			continue
		}
		matched := true
		for j := range param.Segments {
			if p.Segments[j] != param.Segments[j] {
				matched = false
				break
			}
		}
		if matched {
			return i, append([]segment.Segment(nil), p.Segments[len(param.Segments):]...), true
		}
	}
	return 0, nil, false
}

func pathRootEqual(a, b pathdom.Path) bool {
	if a.Symbol != 0 || b.Symbol != 0 {
		return a.Symbol == b.Symbol && a.Version == b.Version
	}
	return a.Root == b.Root
}

func memberCallReceiverFromSite(site factflow.CallSiteView) (pathdom.Path, segment.Segment, bool) {
	if receiver, ok := site.ReceiverPath(); ok && site.MethodName() != "" {
		member := segment.Segment{Kind: segment.SegmentField, Name: site.MethodName()}
		return receiver, member, !receiver.IsEmpty() && memberaccess.Valid(member)
	}
	callee := site.CalleePathRef()
	if callee.IsEmpty() || len(callee.Segments) == 0 {
		return pathdom.Path{}, segment.Segment{}, false
	}
	last := callee.Segments[len(callee.Segments)-1]
	if !memberaccess.Valid(last) {
		return pathdom.Path{}, segment.Segment{}, false
	}
	receiver := callee.Parent()
	return receiver, last, !receiver.IsEmpty()
}

func functionParamTypes(fn *typ.Function, skipFirst bool) []typ.Type {
	if fn == nil {
		return nil
	}
	start := 0
	if skipFirst && len(fn.Params) > 0 {
		start = 1
	}
	params := make([]typ.Type, 0, len(fn.Params)-start)
	for i := start; i < len(fn.Params); i++ {
		params = append(params, fn.Params[i].Type)
	}
	return params
}

func obligationValueFromType(reg *axis.Registry, t typ.Type) (product.Value, bool) {
	if reg == nil || t == nil || typ.IsAny(t) || typ.IsUnknown(t) || refinement.ContainsFreeTypeParam(t) {
		return product.Value{}, false
	}
	return typevalue.WithWitness(reg, typevalue.FromType(reg, t), t), true
}

func paramObligationTypeResolver(result ResultReader) typeannotation.Resolver {
	if bindings, ok := result.(typeBindingReader); ok {
		return typeresolve.New(bindings)
	}
	return nil
}

func lowerParamObligationType(expr ast.TypeExpr, resolver typeannotation.Resolver) (typ.Type, bool) {
	if r, ok := resolver.(*typeresolve.Resolver); ok {
		return r.Type(expr)
	}
	return typeannotation.Type(expr, resolver)
}

func paramObligationTypeFromValue(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	if reg == nil {
		return nil, false
	}
	origin := product.Get(reg, value, variantorigin.Key)
	if witness := product.Get(reg, value, typewitness.Key); !witness.IsTop() {
		if t, ok := witness.Type(); ok {
			if !origin.IsBottom() && !origin.IsTop() {
				if narrowed, ok := variant.NarrowByOrigin(t, origin.Family(), origin.CasesRef()); ok {
					return narrowed, true
				}
			}
			return t, true
		}
	}
	if !origin.IsBottom() && !origin.IsTop() {
		return variant.TypeFromOrigin(origin.Family(), origin.CasesRef())
	}
	return nil, false
}
