package projectsummary

import (
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
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

type callReader interface {
	Call(cfg.Point) (semantics.CallFact, bool)
	CallSite(cfg.Point) (factflow.CallSite, bool)
}

type callOutcomeAtReader interface {
	CallOutcomeAt(cfg.Point) (callpayload.CallOutcome, bool)
}

type returnFactReader interface {
	ReturnFact(cfg.Point) (semantics.ReturnFact, bool)
}

type localAssignmentReader interface {
	LocalAssignment(cfg.Point) (semantics.LocalAssignmentFact, bool)
}

type ordinaryAssignmentReader interface {
	OrdinaryAssignment(cfg.Point) (semantics.OrdinaryAssignmentFact, bool)
}

type callSignatureReader interface {
	CallSignature(factflow.CallSite) (signature.Function, bool)
}

type expressionPathReader interface {
	ExpressionPath(ast.Expr) (pathdom.Path, bool)
}

type symbolTypeAnnotationReader interface {
	SymbolTypeAnnotation(symbol.ID) (ast.TypeExpr, bool)
}

type pathValueAtBoundaryReader interface {
	PathValueAtBoundary(cfg.Point, pathdom.Path) (product.Value, bool)
}

type pathEquivalentAtBoundaryReader interface {
	PathsEquivalentAtBoundary(cfg.Point, pathdom.Path, pathdom.Path) bool
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

func projectParamObligations(reg *axis.Registry, result ResultReader) []product.Value {
	params := parameterValuePaths(result)
	if reg == nil || len(params) == 0 {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	ctx := paramObligationProjector{
		reg:      reg,
		result:   result,
		params:   params,
		resolver: paramObligationTypeResolver(result),
		reach:    newCFGReachabilityCache(graph),
	}
	out := make([]product.Value, len(params))
	for i := range out {
		out[i] = product.Top()
	}
	for _, point := range graph.RPO() {
		ctx.point = point
		if callResult, ok := result.(callReader); ok {
			if fact, ok := callResult.Call(point); ok {
				if site, siteOK := callResult.CallSite(point); siteOK {
					ctx.addCallOutcomeObligations(out, fact, site)
					ctx.addTypedCallObligations(out, fact, site)
				}
				for _, arg := range fact.Args {
					ctx.addArithmeticObligations(out, arg)
				}
			}
		}
		if returnResult, ok := result.(returnFactReader); ok {
			if fact, ok := returnResult.ReturnFact(point); ok {
				for _, expr := range fact.Exprs {
					ctx.addArithmeticObligations(out, expr)
				}
			}
		}
		if localResult, ok := result.(localAssignmentReader); ok {
			if fact, ok := localResult.LocalAssignment(point); ok {
				ctx.addArithmeticObligations(out, fact.Expr)
			}
		}
		if ordinaryResult, ok := result.(ordinaryAssignmentReader); ok {
			if fact, ok := ordinaryResult.OrdinaryAssignment(point); ok {
				ctx.addArithmeticObligations(out, fact.Value)
			}
		}
	}
	return out
}

func projectParamMemberCallObligations(reg *axis.Registry, result ResultReader) []summary.ParamMemberCallObligation {
	params := parameterValuePaths(result)
	if reg == nil || len(params) == 0 {
		return nil
	}
	graph := result.Graph()
	callResult, ok := result.(callReader)
	if graph == nil || !ok {
		return nil
	}
	ctx := paramObligationProjector{
		reg:      reg,
		result:   result,
		params:   params,
		resolver: paramObligationTypeResolver(result),
		reach:    newCFGReachabilityCache(graph),
	}
	var out []summary.ParamMemberCallObligation
	for _, point := range graph.RPO() {
		ctx.point = point
		fact, ok := callResult.Call(point)
		if !ok {
			continue
		}
		site, _ := callResult.CallSite(point)
		out = append(out, ctx.memberCallObligations(fact, site)...)
	}
	return out
}

func projectParamMemberReturnSlots(reg *axis.Registry, result ResultReader) []summary.ParamMemberReturnSlot {
	params := parameterValuePaths(result)
	if reg == nil || len(params) == 0 {
		return nil
	}
	graph := result.Graph()
	sourceReader, hasSources := result.(returnValueSourceReader)
	callResult, hasCalls := result.(callReader)
	if graph == nil || !hasSources || !hasCalls {
		return nil
	}
	ctx := paramObligationProjector{
		reg:      reg,
		result:   result,
		params:   params,
		resolver: paramObligationTypeResolver(result),
		reach:    newCFGReachabilityCache(graph),
	}
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
		fact, ok := callResult.Call(callPoint)
		if !ok {
			continue
		}
		receiver, member, ok := memberCallReceiver(fact)
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
	reg      *axis.Registry
	result   ResultReader
	params   []pathdom.Path
	resolver typeannotation.Resolver
	reach    *cfgReachabilityCache
	point    cfg.Point
}

func (p paramObligationProjector) addCallOutcomeObligations(out []product.Value, fact semantics.CallFact, site factflow.CallSite) {
	if p.selfCall(fact) {
		return
	}
	if receiver, member, ok := memberCallReceiverForSite(fact, site); ok && !p.memberCallReceiverStable(receiver, member) {
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
		if obligation.ParamIndex < 0 || obligation.ParamIndex >= len(fact.Args) {
			continue
		}
		param, ok := p.unconditionalParamIndex(fact.Args[obligation.ParamIndex])
		if !ok {
			continue
		}
		p.add(out, param, obligation.Value)
	}
}

func (p paramObligationProjector) selfCall(fact semantics.CallFact) bool {
	if !fact.HasCalleeSymbol || fact.CalleeSymbol == 0 {
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
	if origin, ok := reader.FunctionOrigin(fn); ok && origin.HasTargetSymbol && origin.TargetSymbol == fact.CalleeSymbol {
		return true
	}
	if calleeFn, ok := reader.FunctionBySymbol(fact.CalleeSymbol); ok {
		if calleeFn == fn {
			return true
		}
	}
	current, ok := reader.FunctionSymbol(fn)
	return ok && current != 0 && current == fact.CalleeSymbol
}

func (p paramObligationProjector) addTypedCallObligations(out []product.Value, fact semantics.CallFact, site factflow.CallSite) {
	params := p.callParamTypes(fact, site)
	if len(params) == 0 {
		return
	}
	for i, want := range params {
		if i >= len(fact.Args) {
			break
		}
		value, ok := obligationValueFromType(p.reg, want)
		if !ok {
			continue
		}
		param, ok := p.unconditionalParamIndex(fact.Args[i])
		if !ok {
			continue
		}
		p.add(out, param, value)
	}
}

func (p paramObligationProjector) callParamTypes(fact semantics.CallFact, site factflow.CallSite) []typ.Type {
	receiver, member, hasMemberCall := memberCallReceiverForSite(fact, site)
	if hasMemberCall && !p.memberCallReceiverStable(receiver, member) {
		return nil
	}
	if sigReader, ok := p.result.(callSignatureReader); ok {
		if sig, ok := sigReader.CallSignature(site); ok && sig.Type != nil {
			return functionParamTypes(sig.Type, false)
		}
	}
	if hasMemberCall {
		receiverType, ok := p.receiverType(receiver)
		if ok {
			fn, status, ok := memberaccess.Callable(receiverType, member)
			if status == typecall.MemberCallOK && ok {
				consumeReceiver := fact.Receiver != nil && fact.Method != "" && typecall.CallableConsumesReceiver(fn, receiverType)
				return functionParamTypes(fn, consumeReceiver)
			}
		}
	}
	if fn, ok := p.directCallable(site); ok {
		return functionParamTypes(fn, false)
	}
	return nil
}

func (p paramObligationProjector) directCallable(site factflow.CallSite) (*typ.Function, bool) {
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

func (p paramObligationProjector) memberCallObligations(fact semantics.CallFact, site factflow.CallSite) []summary.ParamMemberCallObligation {
	receiver, member, ok := memberCallReceiverForSite(fact, site)
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
	if fact.Receiver != nil && fact.Method != "" {
		memberOffset = 1
	}
	var out []summary.ParamMemberCallObligation
	for i, arg := range fact.Args {
		argParam, ok := p.unconditionalParamIndex(arg)
		if !ok {
			continue
		}
		out = append(out, summary.ParamMemberCallObligation{
			ReceiverParam:    receiverParam,
			ReceiverPath:     receiverPath,
			Member:           member,
			ArgParam:         argParam,
			MemberParamIndex: i + memberOffset,
		})
	}
	return out
}

func (p paramObligationProjector) memberCallReceiverStable(receiver pathdom.Path, member segment.Segment) bool {
	if receiver.IsEmpty() || !memberaccess.Valid(member) {
		return false
	}
	graph := p.result.Graph()
	ordinaryResult, ok := p.result.(ordinaryAssignmentReader)
	callResult, hasCalls := p.result.(callReader)
	callOutcomeResult, hasCallOutcomes := p.result.(callOutcomeAtReader)
	if graph == nil {
		return true
	}
	for _, point := range graph.RPO() {
		if point == p.point {
			return true
		}
		if hasCalls && hasCallOutcomes && p.canReach(graph, point, p.point) {
			if site, siteOK := callResult.CallSite(point); siteOK {
				if outcome, outcomeOK := callOutcomeResult.CallOutcomeAt(point); outcomeOK &&
					p.callOutcomeInvalidatesMemberReceiver(site, outcome, receiver, member) {
					return false
				}
			}
		}
		if ok {
			fact, ok := ordinaryResult.OrdinaryAssignment(point)
			if ok && p.assignmentInvalidatesMemberCallReceiver(fact, receiver, member) {
				return false
			}
		}
	}
	return true
}

func (p paramObligationProjector) callOutcomeInvalidatesMemberReceiver(site factflow.CallSite, outcome callpayload.CallOutcome, receiver pathdom.Path, member segment.Segment) bool {
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
	for _, target := range targets {
		for _, memberPath := range memberaccess.Paths(receiver, member) {
			if p.invalidationPathReachesMemberPath(target, memberPath) {
				return true
			}
		}
	}
	return false
}

func (p paramObligationProjector) callArgumentBindings(site factflow.CallSite) []pathdom.Path {
	pathReader, ok := p.result.(expressionPathRefReader)
	if !ok {
		return nil
	}
	var bindings []pathdom.Path
	site.ForEachArgumentSource(func(i int, source factflow.ValueSource) bool {
		if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
			return true
		}
		sourcePath, ok := pathReader.ExpressionPathRef(source.ExprRef)
		if !ok || sourcePath.IsEmpty() {
			return true
		}
		bindings = appendPathBinding(bindings, i, sourcePath)
		return true
	})
	return bindings
}

func (p paramObligationProjector) callBindings(site factflow.CallSite) []pathdom.Path {
	return callSiteBindings(p.result, site)
}

func callSiteBindings(result ResultReader, site factflow.CallSite) []pathdom.Path {
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
		if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
			return true
		}
		sourcePath, ok := pathReader.ExpressionPathRef(source.ExprRef)
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
		return p.reach.canReach(from, to)
	}
	return cfg.PointCanReach(graph, from, to)
}

type cfgReachabilityCache struct {
	graph         cfg.Graph
	reachableFrom map[cfg.Point]map[cfg.Point]struct{}
}

func newCFGReachabilityCache(graph cfg.Graph) *cfgReachabilityCache {
	if graph == nil {
		return nil
	}
	return &cfgReachabilityCache{
		graph:         graph,
		reachableFrom: make(map[cfg.Point]map[cfg.Point]struct{}),
	}
}

func (c *cfgReachabilityCache) canReach(from, to cfg.Point) bool {
	if c == nil || c.graph == nil {
		return false
	}
	if from == to {
		return true
	}
	reachable := c.reachableSet(from)
	_, ok := reachable[to]
	return ok
}

func (c *cfgReachabilityCache) reachableSet(from cfg.Point) map[cfg.Point]struct{} {
	if reachable, ok := c.reachableFrom[from]; ok {
		return reachable
	}
	reachable := map[cfg.Point]struct{}{from: {}}
	stack := append([]cfg.Point(nil), c.graph.Successors(from)...)
	for len(stack) != 0 {
		point := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, ok := reachable[point]; ok {
			continue
		}
		reachable[point] = struct{}{}
		stack = append(stack, c.graph.Successors(point)...)
	}
	c.reachableFrom[from] = reachable
	return reachable
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

func (p paramObligationProjector) assignmentInvalidatesMemberCallReceiver(fact semantics.OrdinaryAssignmentFact, receiver pathdom.Path, member segment.Segment) bool {
	if receiver.IsEmpty() || !memberaccess.Valid(member) {
		return false
	}
	memberPaths := memberaccess.Paths(receiver, member)
	if fact.HasPath {
		if fact.Path.Equal(receiver) {
			return true
		}
		for _, memberPath := range memberPaths {
			if p.invalidationPathReachesMemberPath(fact.Path, memberPath) {
				return true
			}
		}
		return false
	}
	if fact.HasContainerPath {
		return fact.ContainerPath.Equal(receiver)
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

func (p paramObligationProjector) addArithmeticObligations(out []product.Value, expr ast.Expr) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.ArithmeticOpExpr:
		p.addArithmeticOperand(out, e.Lhs)
		p.addArithmeticOperand(out, e.Rhs)
		p.addArithmeticObligations(out, e.Lhs)
		p.addArithmeticObligations(out, e.Rhs)
	case *ast.UnaryMinusOpExpr:
		p.addArithmeticOperand(out, e.Expr)
		p.addArithmeticObligations(out, e.Expr)
	case *ast.UnaryBNotOpExpr:
		p.addArithmeticOperand(out, e.Expr)
		p.addArithmeticObligations(out, e.Expr)
	case *ast.LogicalOpExpr:
		p.addArithmeticObligations(out, e.Lhs)
		p.addArithmeticObligations(out, e.Rhs)
	case *ast.RelationalOpExpr:
		p.addArithmeticObligations(out, e.Lhs)
		p.addArithmeticObligations(out, e.Rhs)
	case *ast.StringConcatOpExpr:
		p.addArithmeticObligations(out, e.Lhs)
		p.addArithmeticObligations(out, e.Rhs)
	case *ast.UnaryLenOpExpr:
		p.addArithmeticObligations(out, e.Expr)
	case *ast.UnaryNotOpExpr:
		p.addArithmeticObligations(out, e.Expr)
	case *ast.CastExpr:
		p.addArithmeticObligations(out, e.Expr)
	case *ast.NonNilAssertExpr:
		p.addArithmeticObligations(out, e.Expr)
	case *ast.AttrGetExpr:
		p.addArithmeticObligations(out, e.Object)
		if e.KeySyntax == ast.AttrKeyIndex {
			p.addArithmeticObligations(out, e.Key)
		}
	case *ast.FuncCallExpr:
		p.addArithmeticObligations(out, e.Func)
		p.addArithmeticObligations(out, e.Receiver)
		for _, arg := range e.Args {
			p.addArithmeticObligations(out, arg)
		}
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			if field.KeySyntax == ast.AttrKeyIndex {
				p.addArithmeticObligations(out, field.Key)
			}
			p.addArithmeticObligations(out, field.Value)
		}
	}
}

func (p paramObligationProjector) addArithmeticOperand(out []product.Value, expr ast.Expr) {
	param, ok := p.unconditionalParamIndex(expr)
	if !ok {
		return
	}
	value, ok := obligationValueFromType(p.reg, typ.Number)
	if !ok {
		return
	}
	p.add(out, param, value)
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

func (p paramObligationProjector) unconditionalParamIndex(expr ast.Expr) (int, bool) {
	pathReader, ok := p.result.(expressionPathReader)
	if !ok || expr == nil {
		return 0, false
	}
	exprPath, ok := pathReader.ExpressionPath(expr)
	if !ok {
		return 0, false
	}
	return p.unconditionalPathParamIndex(exprPath)
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
	pathReader, ok := p.result.(expressionPathReader)
	if !ok {
		return pathdom.Path{}, false
	}
	graph := p.result.Graph()
	localResult, ok := p.result.(localAssignmentReader)
	if graph == nil || !ok {
		return pathdom.Path{}, false
	}
	for _, point := range graph.RPO() {
		if point == p.point {
			return pathdom.Path{}, false
		}
		fact, ok := localResult.LocalAssignment(point)
		if !ok || !fact.HasSymbol || fact.Symbol != receiver.Symbol || fact.Expr == nil {
			continue
		}
		source, ok := pathReader.ExpressionPath(fact.Expr)
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

func memberCallReceiver(fact semantics.CallFact) (pathdom.Path, segment.Segment, bool) {
	if fact.HasReceiverPath && fact.Method != "" {
		return fact.ReceiverPath, segment.Segment{Kind: segment.SegmentField, Name: fact.Method}, true
	}
	if !fact.HasCalleePath || len(fact.CalleePath.Segments) == 0 {
		return pathdom.Path{}, segment.Segment{}, false
	}
	last := fact.CalleePath.Segments[len(fact.CalleePath.Segments)-1]
	switch last.Kind {
	case segment.SegmentField, segment.SegmentIndexString, segment.SegmentIndexInt:
		receiver := fact.CalleePath.Parent()
		return receiver, last, !receiver.IsEmpty() && memberaccess.Valid(last)
	default:
		return pathdom.Path{}, segment.Segment{}, false
	}
}

func memberCallReceiverForSite(fact semantics.CallFact, site factflow.CallSite) (pathdom.Path, segment.Segment, bool) {
	if receiver, member, ok := memberCallReceiver(fact); ok {
		return receiver, member, true
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
