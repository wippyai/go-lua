// Package provenance normalizes effect-produced relations into canonical facts
// before the abstract interpreter runs.
//
// This layer is intentionally between contracts/effect rows and transfer:
// contracts describe semantic producers, provenance turns those descriptions
// into deterministic path-keyed facts, and transfer consumes only those facts.
package provenance

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	domainpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
)

// ReturnTransformResolver resolves the return transform for the call result
// feeding one assignment target.
//
// The resolver is the only module-aware callback. Normalization itself stays
// pure over CFG, constants, callsite shape, and the resolved transform.
type ReturnTransformResolver func(call *cfg.CallInfo, retIndex int) (effect.ReturnType, bool)

// Input is the finite evidence needed to normalize provenance facts for one
// function.
type Input struct {
	Graph           *cfg.Graph
	ConstValues     map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue
	ReturnTransform ReturnTransformResolver
}

// Facts are normalized passive relations consumed by the canonical product and
// condition transfer.
type Facts struct {
	VariantFieldOrigins []flow.VariantFieldOrigin
}

// Normalize walks assignment-producing call returns and materializes all
// recognized provenance facts in deterministic order.
func Normalize(in Input) Facts {
	if in.Graph == nil || in.ReturnTransform == nil {
		return Facts{}
	}
	var out Facts
	in.Graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil {
			return
		}
		for i, target := range info.Targets {
			targetPath := targetPath(in, p, target)
			if targetPath.IsEmpty() {
				continue
			}
			call, retIndex := info.CallForTarget(i)
			if call == nil || call.Call == nil {
				continue
			}
			transform, ok := in.ReturnTransform(call, retIndex)
			if !ok || transform == nil {
				continue
			}
			effect.VisitReturnType(transform, effect.ReturnTypeVisitor[struct{}]{
				SelectResultOfCases: func(v effect.SelectResultOfCases) struct{} {
					out.VariantFieldOrigins = append(out.VariantFieldOrigins, selectResultOrigins(in, p, call, targetPath, v)...)
					return struct{}{}
				},
			})
		}
	})
	if len(out.VariantFieldOrigins) > 1 {
		tmp := flow.Inputs{VariantFieldOrigins: out.VariantFieldOrigins}
		tmp.Normalize()
		out.VariantFieldOrigins = tmp.VariantFieldOrigins
	}
	return out
}

func selectResultOrigins(in Input, p cfg.Point, call *cfg.CallInfo, target constraint.Path, transform effect.SelectResultOfCases) []flow.VariantFieldOrigin {
	casesIdx, ok := effect.ResolveParamIndex(transform.Cases, callsite.RuntimeArgCount(call))
	if !ok {
		return nil
	}
	caseExprs := selectCaseExpressions(callsite.RuntimeArgAt(call, casesIdx))
	if len(caseExprs) == 0 {
		return nil
	}
	family := flow.VariantOriginFamily(target, effect.SelectResultChannelField)
	out := make([]flow.VariantFieldOrigin, 0, len(caseExprs))
	for caseIdx, caseExpr := range caseExprs {
		sourcePath := selectCaseSourcePath(in, p, caseExpr)
		if sourcePath.IsEmpty() {
			continue
		}
		out = append(out, flow.VariantFieldOrigin{
			Target:       target,
			Field:        effect.SelectResultChannelField,
			Source:       sourcePath,
			OriginFamily: family,
			CaseIndex:    caseIdx,
		})
	}
	return out
}

func targetPath(in Input, p cfg.Point, target cfg.AssignTarget) constraint.Path {
	g := in.Graph
	if g == nil {
		return constraint.Path{}
	}
	switch target.Kind {
	case cfg.TargetIdent:
		if target.Symbol == 0 {
			return constraint.Path{}
		}
		root := target.Name
		if bindings := g.Bindings(); bindings != nil {
			if name := bindings.Name(target.Symbol); name != "" {
				root = name
			}
		}
		return domainpath.WithVersion(constraint.Path{Root: root, Symbol: target.Symbol}, g, p)
	case cfg.TargetField, cfg.TargetIndex:
		if target.Expr != nil {
			return domainpath.FromExprWithBindingsAt(target.Expr, constResolver(in, p), g.Bindings(), g, p)
		}
	}
	return constraint.Path{}
}

func selectCaseSourcePath(in Input, p cfg.Point, expr ast.Expr) constraint.Path {
	g := in.Graph
	if g == nil || expr == nil {
		return constraint.Path{}
	}
	if call, ok := expr.(*ast.FuncCallExpr); ok && callsite.IsMethodLikeExpr(call) && call.Receiver != nil {
		return domainpath.FromExprWithBindingsAt(call.Receiver, constResolver(in, p), g.Bindings(), g, p)
	}
	return domainpath.FromExprWithBindingsAt(expr, constResolver(in, p), g.Bindings(), g, p)
}

func selectCaseExpressions(expr ast.Expr) []ast.Expr {
	tbl, ok := expr.(*ast.TableExpr)
	if !ok || tbl == nil || len(tbl.Fields) == 0 {
		return nil
	}
	out := make([]ast.Expr, 0, len(tbl.Fields))
	for _, field := range tbl.Fields {
		if field == nil || field.Value == nil || selectCaseFieldIsDefault(field) {
			continue
		}
		out = append(out, field.Value)
	}
	return out
}

func selectCaseFieldIsDefault(field *ast.Field) bool {
	if field == nil || field.Key == nil {
		return false
	}
	name, ok := fieldkey.StringKeyFromTableField(field)
	return ok && name == "default"
}

func constResolver(in Input, p cfg.Point) func(string) *flow.ConstValue {
	if in.Graph == nil || len(in.ConstValues) == 0 {
		return nil
	}
	return func(name string) *flow.ConstValue {
		sym, ok := in.Graph.SymbolAt(p, name)
		if !ok || sym == 0 {
			if bindings := in.Graph.Bindings(); bindings != nil {
				symbols := bindings.SymbolsByName(name)
				if len(symbols) == 1 {
					sym = symbols[0]
				}
			}
			if sym == 0 {
				return nil
			}
		}
		at := in.ConstValues[sym]
		if at == nil {
			return nil
		}
		val := at[p]
		if val == nil || val.Kind == flow.ConstUnknown {
			return nil
		}
		return val
	}
}
