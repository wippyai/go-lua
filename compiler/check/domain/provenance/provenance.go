package provenance

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// IdentBindingLookup resolves identifier expressions to graph symbols.
type IdentBindingLookup interface {
	SymbolOf(ident *ast.IdentExpr) (cfg.SymbolID, bool)
}

// FreshTableLiteral is a table constructor that still owns the value currently
// read through a local symbol. Point is where the literal assignment occurred.
type FreshTableLiteral struct {
	Table *ast.TableExpr
	Point cfg.Point
}

// SegmentTypeProjector projects a structural type through a field/index suffix.
type SegmentTypeProjector func(base typ.Type, segments []constraint.Segment) typ.Type

// SourceReadKind identifies the evidence surface used to read a route source.
type SourceReadKind uint8

const (
	SourceReadBodyContract SourceReadKind = iota + 1
	SourceReadPointPath
)

// RouteSourceTypeResolver reads one source path from the requested evidence
// surface. Callers own evidence lookup; provenance owns route shape.
type RouteSourceTypeResolver func(path constraint.Path, read SourceReadKind) typ.Type

type sourceProjectionKind uint8

const (
	sourceProjectionDirect sourceProjectionKind = iota + 1
	sourceProjectionIndexedIteratorValue
	sourceProjectionKeyedIteratorKey
	sourceProjectionKeyedIteratorValue
	sourceProjectionSequenceElement
)

// RouteSourceTypeQuery is one source read and projection implied by a
// provenance route.
type RouteSourceTypeQuery struct {
	Path      constraint.Path
	ReadOrder []SourceReadKind
	Segments  []constraint.Segment
	project   sourceProjectionKind
}

// RouteSourceType projects the best source type available for route. It applies
// route semantics once, while the caller supplies the evidence resolver.
func RouteSourceType(route flow.ProvenanceRoute, extra []constraint.Segment, resolve RouteSourceTypeResolver, project SegmentTypeProjector) typ.Type {
	if resolve == nil {
		return nil
	}
	var fallback typ.Type
	for _, query := range RouteSourceTypeQueries(route, extra) {
		for _, read := range query.ReadOrder {
			source := resolve(query.Path, read)
			projected := query.ProjectType(source, project)
			if typ.IsAbsentOrUnknown(projected) {
				continue
			}
			if !typ.IsAny(projected) {
				return projected
			}
			fallback = projected
		}
	}
	return fallback
}

// RouteSourceTypeQueries lowers a provenance route into source reads and local
// type projections. Evidence precedence is part of the query because it is a
// semantic property of the route, not of observation.
func RouteSourceTypeQueries(route flow.ProvenanceRoute, extra []constraint.Segment) []RouteSourceTypeQuery {
	if route.Source.Symbol == 0 {
		return nil
	}
	switch route.Kind {
	case flow.ProvenanceRouteIdentityAlias:
		return []RouteSourceTypeQuery{{
			Path:      route.Source,
			ReadOrder: []SourceReadKind{SourceReadBodyContract},
			Segments:  cloneSegments(extra),
			project:   sourceProjectionDirect,
		}}
	case flow.ProvenanceRouteIndexedIterator:
		if route.VarIndex != 1 {
			return nil
		}
		return []RouteSourceTypeQuery{{
			Path:      route.Source,
			ReadOrder: []SourceReadKind{SourceReadBodyContract},
			Segments:  joinedSegments(route.Remainder, extra),
			project:   sourceProjectionIndexedIteratorValue,
		}}
	case flow.ProvenanceRouteKeyedIterator:
		project := sourceProjectionKeyedIteratorValue
		if route.VarIndex == 0 {
			project = sourceProjectionKeyedIteratorKey
		} else if route.VarIndex != 1 {
			return nil
		}
		return []RouteSourceTypeQuery{{
			Path:      route.Source,
			ReadOrder: []SourceReadKind{SourceReadBodyContract},
			Segments:  joinedSegments(route.Remainder, extra),
			project:   project,
		}}
	case flow.ProvenanceRouteAppendElementField:
		return appendElementFieldRouteSourceTypeQueries(route, extra)
	default:
		return nil
	}
}

// ProjectType applies q's route projection to source.
func (q RouteSourceTypeQuery) ProjectType(source typ.Type, project SegmentTypeProjector) typ.Type {
	if source == nil {
		return nil
	}
	var local typ.Type
	switch q.project {
	case sourceProjectionDirect:
		local = source
	case sourceProjectionIndexedIteratorValue, sourceProjectionSequenceElement:
		local = querycore.ElementType(source)
	case sourceProjectionKeyedIteratorKey:
		local = querycore.EntryKeyType(source)
	case sourceProjectionKeyedIteratorValue:
		local = querycore.EntryValueType(source)
	default:
		return nil
	}
	if local == nil || len(q.Segments) == 0 {
		return local
	}
	if project == nil {
		return nil
	}
	return project(local, q.Segments)
}

func appendElementFieldRouteSourceTypeQueries(route flow.ProvenanceRoute, extra []constraint.Segment) []RouteSourceTypeQuery {
	if len(route.SourceField) > 0 {
		return []RouteSourceTypeQuery{{
			Path:      route.Source,
			ReadOrder: []SourceReadKind{SourceReadPointPath, SourceReadBodyContract},
			Segments:  joinedSegments(joinedSegments(route.SourceField, route.FieldRemainder), extra),
			project:   sourceProjectionSequenceElement,
		}}
	}
	return []RouteSourceTypeQuery{{
		Path:      appendSegments(route.Source, joinedSegments(route.FieldRemainder, extra)),
		ReadOrder: []SourceReadKind{SourceReadBodyContract, SourceReadPointPath},
		project:   sourceProjectionDirect,
	}}
}

func appendSegments(path constraint.Path, segments []constraint.Segment) constraint.Path {
	for _, seg := range segments {
		path = path.Append(seg)
	}
	return path
}

func joinedSegments(first, second []constraint.Segment) []constraint.Segment {
	if len(first) == 0 {
		return cloneSegments(second)
	}
	out := cloneSegments(first)
	out = append(out, second...)
	return out
}

func cloneSegments(segments []constraint.Segment) []constraint.Segment {
	if len(segments) == 0 {
		return nil
	}
	return append([]constraint.Segment(nil), segments...)
}

// CurrentFreshTableLiteral returns the transfer-proven fresh table literal for
// source at a point. The freshness proof is produced by trace.GraphEvidence;
// this reducer only matches the identifier use to canonical evidence.
func CurrentFreshTableLiteral(
	source ast.Expr,
	at cfg.Point,
	bindings IdentBindingLookup,
	freshTables []api.FreshTableLiteralEvidence,
) (FreshTableLiteral, bool) {
	if source == nil || bindings == nil || len(freshTables) == 0 {
		return FreshTableLiteral{}, false
	}
	ident, ok := source.(*ast.IdentExpr)
	if !ok {
		return FreshTableLiteral{}, false
	}
	sym, ok := bindings.SymbolOf(ident)
	if !ok || sym == 0 {
		return FreshTableLiteral{}, false
	}
	for _, ev := range freshTables {
		if ev.Point != at || ev.Symbol != sym || ev.Table == nil {
			continue
		}
		return FreshTableLiteral{Table: ev.Table, Point: ev.AssignmentPoint}, true
	}
	return FreshTableLiteral{}, false
}

// ExprMayExposeSymbolValue reports whether evaluating expr may publish the
// symbol's current value as an alias or to a call. Field reads do not expose the
// base object; calls and table constructors do.
func ExprMayExposeSymbolValue(expr ast.Expr, sym cfg.SymbolID, bindings IdentBindingLookup) bool {
	if expr == nil || sym == 0 || bindings == nil {
		return false
	}
	switch e := expr.(type) {
	case *ast.IdentExpr:
		bound, ok := bindings.SymbolOf(e)
		return ok && bound == sym
	case *ast.CastExpr:
		return ExprMayExposeSymbolValue(e.Expr, sym, bindings)
	case *ast.NonNilAssertExpr:
		return ExprMayExposeSymbolValue(e.Expr, sym, bindings)
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			if ExprMayExposeSymbolValue(field.Key, sym, bindings) || ExprMayExposeSymbolValue(field.Value, sym, bindings) {
				return true
			}
		}
		return false
	case *ast.FuncCallExpr:
		return ExprReferencesSymbol(e.Func, sym, bindings) ||
			ExprReferencesSymbol(e.Receiver, sym, bindings) ||
			exprsReferenceSymbol(e.Args, sym, bindings)
	case *ast.LogicalOpExpr:
		return ExprMayExposeSymbolValue(e.Lhs, sym, bindings) || ExprMayExposeSymbolValue(e.Rhs, sym, bindings)
	default:
		return false
	}
}

// ExprReferencesSymbol reports whether expr reads the given symbol anywhere.
func ExprReferencesSymbol(expr ast.Expr, sym cfg.SymbolID, bindings IdentBindingLookup) bool {
	if expr == nil || sym == 0 || bindings == nil {
		return false
	}

	switch e := expr.(type) {
	case *ast.IdentExpr:
		if bound, ok := bindings.SymbolOf(e); ok && bound == sym {
			return true
		}
		return false
	case *ast.AttrGetExpr:
		return ExprReferencesSymbol(e.Object, sym, bindings) || ExprReferencesSymbol(e.Key, sym, bindings)
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			if ExprReferencesSymbol(field.Key, sym, bindings) || ExprReferencesSymbol(field.Value, sym, bindings) {
				return true
			}
		}
		return false
	case *ast.FuncCallExpr:
		return ExprReferencesSymbol(e.Func, sym, bindings) ||
			ExprReferencesSymbol(e.Receiver, sym, bindings) ||
			exprsReferenceSymbol(e.Args, sym, bindings)
	case *ast.LogicalOpExpr:
		return ExprReferencesSymbol(e.Lhs, sym, bindings) || ExprReferencesSymbol(e.Rhs, sym, bindings)
	case *ast.RelationalOpExpr:
		return ExprReferencesSymbol(e.Lhs, sym, bindings) || ExprReferencesSymbol(e.Rhs, sym, bindings)
	case *ast.StringConcatOpExpr:
		return ExprReferencesSymbol(e.Lhs, sym, bindings) || ExprReferencesSymbol(e.Rhs, sym, bindings)
	case *ast.ArithmeticOpExpr:
		return ExprReferencesSymbol(e.Lhs, sym, bindings) || ExprReferencesSymbol(e.Rhs, sym, bindings)
	case *ast.UnaryMinusOpExpr:
		return ExprReferencesSymbol(e.Expr, sym, bindings)
	case *ast.UnaryNotOpExpr:
		return ExprReferencesSymbol(e.Expr, sym, bindings)
	case *ast.UnaryLenOpExpr:
		return ExprReferencesSymbol(e.Expr, sym, bindings)
	case *ast.UnaryBNotOpExpr:
		return ExprReferencesSymbol(e.Expr, sym, bindings)
	case *ast.CastExpr:
		return ExprReferencesSymbol(e.Expr, sym, bindings)
	case *ast.NonNilAssertExpr:
		return ExprReferencesSymbol(e.Expr, sym, bindings)
	default:
		return false
	}
}

func exprsReferenceSymbol(exprs []ast.Expr, sym cfg.SymbolID, bindings IdentBindingLookup) bool {
	for _, expr := range exprs {
		if ExprReferencesSymbol(expr, sym, bindings) {
			return true
		}
	}
	return false
}
