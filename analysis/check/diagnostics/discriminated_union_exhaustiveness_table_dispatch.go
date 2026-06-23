package diagnostics

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/compiler/ast"
)

type dispatchTableEvidence struct {
	table      string
	target     string
	possible   []string
	keys       []string
	missing    []string
	missingFor []string
	tableSpan  diagnostic.Span
	lookupSpan diagnostic.Span
}

type dispatchLookup struct {
	point        cfg.Point
	expr         *ast.AttrGetExpr
	table        pathdom.Path
	discriminant pathdom.Path
}

func (p discriminatedUnionExhaustiveness) tableDispatchDiagnostics(result *body.Result, graph cfg.Graph) []diagnostic.Diagnostic {
	var out []diagnostic.Diagnostic
	for _, point := range graph.RPO() {
		for _, lookup := range p.dispatchLookupsAt(result, point) {
			if diag, ok := p.tableDispatchDiagnostic(result, lookup); ok {
				out = append(out, diag)
			}
		}
	}
	return out
}

func (p discriminatedUnionExhaustiveness) dispatchLookupsAt(result *body.Result, point cfg.Point) []dispatchLookup {
	var out []dispatchLookup
	if fact, ok := result.LocalAssignment(point); ok && fact.Expr != nil {
		out = append(out, p.dispatchLookupsInExpr(result, point, fact.Expr, false)...)
	}
	if fact, ok := result.OrdinaryAssignment(point); ok && fact.Value != nil {
		out = append(out, p.dispatchLookupsInExpr(result, point, fact.Value, false)...)
	}
	if call, ok := result.Call(point); ok && call.Call != nil {
		out = append(out, p.dispatchLookupsInExpr(result, point, call.Func, true)...)
		if call.Receiver != nil {
			out = append(out, p.dispatchLookupsInExpr(result, point, call.Receiver, true)...)
		}
	}
	return out
}

func (p discriminatedUnionExhaustiveness) dispatchLookupsInExpr(result *body.Result, point cfg.Point, expr ast.Expr, scanCallFunc bool) []dispatchLookup {
	if expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		var out []dispatchLookup
		if lookup, ok := p.dispatchLookupFromAttr(result, point, e); ok {
			out = append(out, lookup)
		}
		out = append(out, p.dispatchLookupsInExpr(result, point, e.Object, scanCallFunc)...)
		out = append(out, p.dispatchLookupsInExpr(result, point, e.Key, scanCallFunc)...)
		return out
	case *ast.FuncCallExpr:
		if !scanCallFunc {
			return nil
		}
		var out []dispatchLookup
		out = append(out, p.dispatchLookupsInExpr(result, point, e.Func, scanCallFunc)...)
		out = append(out, p.dispatchLookupsInExpr(result, point, e.Receiver, scanCallFunc)...)
		return out
	case *ast.CastExpr:
		return p.dispatchLookupsInExpr(result, point, e.Expr, scanCallFunc)
	case *ast.NonNilAssertExpr:
		return p.dispatchLookupsInExpr(result, point, e.Expr, scanCallFunc)
	case *ast.LogicalOpExpr:
		return nil
	default:
		return nil
	}
}

func (p discriminatedUnionExhaustiveness) dispatchLookupFromAttr(result *body.Result, point cfg.Point, attr *ast.AttrGetExpr) (dispatchLookup, bool) {
	if attr == nil || attr.KeySyntax != ast.AttrKeyIndex {
		return dispatchLookup{}, false
	}
	tablePath, ok := result.ExpressionPath(attr.Object)
	if !ok || tablePath.Symbol == 0 {
		return dispatchLookup{}, false
	}
	discriminantPath, ok := result.ExpressionPath(attr.Key)
	if !ok || discriminantPath.Symbol == 0 || len(discriminantPath.Segments) == 0 {
		return dispatchLookup{}, false
	}
	return dispatchLookup{
		point:        point,
		expr:         attr,
		table:        tablePath,
		discriminant: discriminantPath,
	}, true
}

func (p discriminatedUnionExhaustiveness) tableDispatchDiagnostic(result *body.Result, lookup dispatchLookup) (diagnostic.Diagnostic, bool) {
	cases, ok := p.stringDiscriminantCases(result, lookup.point, lookup.discriminant)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	keys, tableSpan, ok := p.dispatchTableKeysAt(result, lookup.point, lookup.table)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	var possible []string
	var presentKeys []string
	var missingCases []string
	var missingKeys []string
	for _, c := range cases {
		possible = append(possible, c.name)
		if keys[c.key] {
			presentKeys = append(presentKeys, dispatchKeyName(lookup.table.String(), c.key))
			continue
		}
		missingCases = append(missingCases, c.name)
		missingKeys = append(missingKeys, dispatchKeyName(lookup.table.String(), c.key))
	}
	if len(missingKeys) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	sort.Strings(presentKeys)
	lookupSpan := ast.SpanOf(lookup.expr)
	return newDispatchTableExhaustivenessDiagnostic(dispatchTableEvidence{
		table:      lookup.table.String(),
		target:     lookup.discriminant.String(),
		possible:   possible,
		keys:       presentKeys,
		missing:    missingKeys,
		missingFor: missingCases,
		tableSpan:  tableSpan,
		lookupSpan: lookupSpan,
	}), true
}
