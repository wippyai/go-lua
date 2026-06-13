package projectsummary

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/effect/signature"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typeaccess"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

type callReader interface {
	Call(cfg.Point) (semantics.CallFact, bool)
	CallSite(cfg.Point) (factflow.CallSite, bool)
}

type callOutcomeAtReader interface {
	CallOutcomeAt(cfg.Point) (factapply.CallOutcome, bool)
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

type typeBindingReader interface {
	TypeRef(*ast.TypeRefExpr) (bind.TypeDecl, bool)
	PrimitiveTypeRef(*ast.PrimitiveTypeExpr) (bind.TypeDecl, bool)
	TypeDefParams(*ast.TypeDefStmt) []bind.TypeDecl
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
					ctx.addCallOutcomeObligations(out, fact)
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
	}
	var out []summary.ParamMemberCallObligation
	for _, point := range graph.RPO() {
		ctx.point = point
		fact, ok := callResult.Call(point)
		if !ok {
			continue
		}
		out = append(out, ctx.memberCallObligations(fact)...)
	}
	return out
}

type paramObligationProjector struct {
	reg      *axis.Registry
	result   ResultReader
	params   []pathdom.Path
	resolver typeannotation.Resolver
	point    cfg.Point
}

func (p paramObligationProjector) addCallOutcomeObligations(out []product.Value, fact semantics.CallFact) {
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
	if sigReader, ok := p.result.(callSignatureReader); ok {
		if sig, ok := sigReader.CallSignature(site); ok && sig.Type != nil {
			return functionParamTypes(sig.Type, false)
		}
	}
	if fn, ok := p.directCallable(site); ok {
		return functionParamTypes(fn, false)
	}
	receiver, member, ok := memberCallReceiver(fact)
	if !ok {
		return nil
	}
	receiverType, ok := p.receiverType(receiver)
	if !ok {
		return nil
	}
	memberType, status := typecall.MemberCall(receiverType, member)
	if status != typecall.MemberCallOK {
		return nil
	}
	fn, ok := typecall.Callable(memberType)
	if !ok {
		return nil
	}
	consumeReceiver := fact.Receiver != nil && fact.Method != "" && memberCallConsumesReceiver(fn, receiverType)
	return functionParamTypes(fn, consumeReceiver)
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
				if projected, ok := typeaccess.ProjectSegments(base, receiver.Segments); ok {
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

func (p paramObligationProjector) memberCallObligations(fact semantics.CallFact) []summary.ParamMemberCallObligation {
	receiver, member, ok := memberCallReceiver(fact)
	if !ok {
		return nil
	}
	receiverParam, ok := p.unconditionalPathParamIndex(receiver)
	if !ok {
		return nil
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
			Member:           member,
			ArgParam:         argParam,
			MemberParamIndex: i + memberOffset,
		})
	}
	return out
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

func (p paramObligationProjector) paramUseUnconditional(index int) bool {
	if index < 0 || index >= len(p.params) {
		return false
	}
	slot := key.SymbolValue(p.params[index].Symbol)
	if slot == "" {
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
	if p.IsEmpty() || len(p.Segments) != 0 {
		return 0, false
	}
	for i, param := range params {
		if p.Equal(param) {
			return i, true
		}
	}
	return 0, false
}

func memberCallReceiver(fact semantics.CallFact) (pathdom.Path, string, bool) {
	if fact.HasReceiverPath && fact.Method != "" {
		return fact.ReceiverPath, fact.Method, true
	}
	if !fact.HasCalleePath || len(fact.CalleePath.Segments) == 0 {
		return pathdom.Path{}, "", false
	}
	last := fact.CalleePath.Segments[len(fact.CalleePath.Segments)-1]
	switch last.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		receiver := fact.CalleePath.Parent()
		return receiver, last.Name, !receiver.IsEmpty() && last.Name != ""
	default:
		return pathdom.Path{}, "", false
	}
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

func memberCallConsumesReceiver(fn *typ.Function, receiverType typ.Type) bool {
	if fn == nil || len(fn.Params) == 0 || receiverType == nil {
		return false
	}
	if fn.Params[0].Name == "self" {
		return true
	}
	self := fn.Params[0].Type
	return self != nil && !typ.IsAny(self) && !typ.IsUnknown(self) && subtype.IsSubtype(receiverType, self)
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
				if narrowed, ok := variant.NarrowByOrigin(t, origin.Family(), origin.Cases()); ok {
					return narrowed, true
				}
			}
			return t, true
		}
	}
	if !origin.IsBottom() && !origin.IsTop() {
		return variant.TypeFromOrigin(origin.Family(), origin.Cases())
	}
	return nil, false
}
