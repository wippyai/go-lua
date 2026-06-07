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

// RouteResolver returns one-step provenance routes for path.
type RouteResolver func(path constraint.Path) []flow.ProvenanceRoute

// RouteClosureTarget is one path-scoped payload carried through a provenance
// route closure.
type RouteClosureTarget[T any] struct {
	Path    constraint.Path
	Payload T
}

// RouteClosureConfig supplies the domain-specific parts of a provenance route
// closure. Provenance owns path identity and graph traversal; callers own the
// payload algebra.
type RouteClosureConfig[T any] struct {
	Seed     RouteClosureTarget[T]
	Routes   RouteResolver
	Targets  func(flow.ProvenanceRoute, T) []RouteClosureTarget[T]
	IsBottom func(T) bool
	Join     func(T, T) T
	Equal    func(T, T) bool
}

type routeIdentitySet struct {
	seen map[constraint.PathKey]bool
}

func routePathIdentity(path constraint.Path) (constraint.PathKey, bool) {
	if path.Symbol == 0 {
		return "", false
	}
	key := flow.PathIdentityKey(path)
	return key, key != ""
}

func (s *routeIdentitySet) Enter(path constraint.Path) (func(), bool) {
	key, ok := routePathIdentity(path)
	if !ok {
		return nil, false
	}
	if s.seen == nil {
		s.seen = make(map[constraint.PathKey]bool)
	}
	if s.seen[key] {
		return nil, false
	}
	s.seen[key] = true
	return func() {
		delete(s.seen, key)
	}, true
}

// RouteClosure follows provenance routes backward from Seed and joins payloads
// that reach the same stable path identity. If a joined payload grows, that path
// is revisited so downstream routes see the stronger obligation.
func RouteClosure[T any](cfg RouteClosureConfig[T]) []RouteClosureTarget[T] {
	if cfg.Seed.Path.Symbol == 0 || routePayloadBottom(cfg.IsBottom, cfg.Seed.Payload) {
		return nil
	}
	var out []RouteClosureTarget[T]
	index := map[constraint.PathKey]int{}
	var queue []RouteClosureTarget[T]
	add := func(target RouteClosureTarget[T]) {
		if target.Path.Symbol == 0 || routePayloadBottom(cfg.IsBottom, target.Payload) {
			return
		}
		key, ok := routePathIdentity(target.Path)
		if !ok {
			return
		}
		if i, ok := index[key]; ok {
			joined := routePayloadJoin(cfg.Join, out[i].Payload, target.Payload)
			if routePayloadEqual(cfg.Equal, out[i].Payload, joined) {
				return
			}
			out[i].Payload = joined
			queue = append(queue, out[i])
			return
		}
		index[key] = len(out)
		out = append(out, target)
		queue = append(queue, target)
	}
	add(cfg.Seed)
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cfg.Routes == nil || cfg.Targets == nil {
			continue
		}
		for _, route := range cfg.Routes(cur.Path) {
			for _, target := range cfg.Targets(route, cur.Payload) {
				add(target)
			}
		}
	}
	return out
}

func routePayloadBottom[T any](isBottom func(T) bool, payload T) bool {
	return isBottom != nil && isBottom(payload)
}

func routePayloadJoin[T any](join func(T, T) T, a, b T) T {
	if join == nil {
		return a
	}
	return join(a, b)
}

func routePayloadEqual[T any](equal func(T, T) bool, a, b T) bool {
	if equal == nil {
		return true
	}
	return equal(a, b)
}

// SourceReadKind identifies the evidence surface used to read a route source.
type SourceReadKind uint8

const (
	SourceReadBodyContract SourceReadKind = iota + 1
	SourceReadPointPath
)

// RouteSourceTypeResolver reads one source path from the requested evidence
// surface. Callers own evidence lookup; provenance owns route shape.
type RouteSourceTypeResolver func(path constraint.Path, read SourceReadKind) typ.Type

// RouteSourceTypeGraph is the route-backed type evidence graph used by
// observation. Direct body-contract reads, point-path reads, source-read
// precedence, path identity, and cycle control are interpreted together here so
// consumers do not each rebuild route recursion.
type RouteSourceTypeGraph struct {
	Routes           RouteResolver
	BodyContractRead func(constraint.Path) typ.Type
	PointPathRead    func(constraint.Path) typ.Type
	Finalize         func(constraint.Path, typ.Type) typ.Type
	Project          SegmentTypeProjector
}

// TypeAt returns the best route-backed type evidence for path.
func (g RouteSourceTypeGraph) TypeAt(path constraint.Path) typ.Type {
	return g.typeAt(path, nil)
}

// RouteType returns the type projected by one provenance route. The route target
// itself is not finalized; source body-contract reads are finalized at their own
// paths through TypeAt.
func (g RouteSourceTypeGraph) RouteType(route flow.ProvenanceRoute) typ.Type {
	return RouteSourceType(route, nil, g.resolver(nil), g.Project)
}

func (g RouteSourceTypeGraph) typeAt(path constraint.Path, seen *routeIdentitySet) typ.Type {
	if seen == nil {
		seen = &routeIdentitySet{}
	}
	leave, ok := seen.Enter(path)
	if !ok {
		return nil
	}
	defer leave()

	if direct := g.bodyContractRead(path); direct != nil {
		return g.finalize(path, direct)
	}
	if g.Routes == nil {
		return nil
	}
	var types []typ.Type
	resolve := g.resolver(seen)
	for _, route := range g.Routes(path) {
		if sourceType := RouteSourceType(route, nil, resolve, g.Project); sourceType != nil {
			types = append(types, sourceType)
		}
	}
	switch len(types) {
	case 0:
		return nil
	case 1:
		return g.finalize(path, types[0])
	default:
		return g.finalize(path, typ.NewUnion(types...))
	}
}

func (g RouteSourceTypeGraph) resolver(seen *routeIdentitySet) RouteSourceTypeResolver {
	return func(path constraint.Path, read SourceReadKind) typ.Type {
		switch read {
		case SourceReadBodyContract:
			return g.typeAt(path, seen)
		case SourceReadPointPath:
			return g.pointPathRead(path)
		default:
			return nil
		}
	}
}

func (g RouteSourceTypeGraph) bodyContractRead(path constraint.Path) typ.Type {
	if g.BodyContractRead == nil {
		return nil
	}
	return g.BodyContractRead(path)
}

func (g RouteSourceTypeGraph) pointPathRead(path constraint.Path) typ.Type {
	if g.PointPathRead == nil {
		return nil
	}
	return g.PointPathRead(path)
}

func (g RouteSourceTypeGraph) finalize(path constraint.Path, t typ.Type) typ.Type {
	if g.Finalize == nil {
		return t
	}
	return g.Finalize(path, t)
}

// RouteProjectionKind identifies how a provenance route relates the source
// value to the local routed value.
type RouteProjectionKind uint8

const (
	RouteProjectionDirect RouteProjectionKind = iota + 1
	RouteProjectionIndexedIteratorValue
	RouteProjectionKeyedIteratorKey
	RouteProjectionKeyedIteratorValue
	RouteProjectionSequenceElement
)

// RouteProjectionAlgebra interprets a route projection in one semantic domain.
// Provenance owns which projection applies; callers own what "element", "key",
// or "value" means for their carrier.
type RouteProjectionAlgebra[T any] struct {
	IndexedIteratorValue func(T) T
	KeyedIteratorKey     func(T) T
	KeyedIteratorValue   func(T) T
	SequenceElement      func(T) T
}

// ApplyRouteProjection applies projection through algebra. Keeping this switch
// in provenance prevents observation and contract propagation from re-encoding
// route projection identity independently.
func ApplyRouteProjection[T any](projection RouteProjectionKind, value T, algebra RouteProjectionAlgebra[T]) T {
	switch projection {
	case RouteProjectionDirect:
		return value
	case RouteProjectionIndexedIteratorValue:
		if algebra.IndexedIteratorValue != nil {
			return algebra.IndexedIteratorValue(value)
		}
	case RouteProjectionKeyedIteratorKey:
		if algebra.KeyedIteratorKey != nil {
			return algebra.KeyedIteratorKey(value)
		}
	case RouteProjectionKeyedIteratorValue:
		if algebra.KeyedIteratorValue != nil {
			return algebra.KeyedIteratorValue(value)
		}
	case RouteProjectionSequenceElement:
		if algebra.SequenceElement != nil {
			return algebra.SequenceElement(value)
		}
	}
	var zero T
	return zero
}

// RouteSourceQuery is one source read and projection implied by a provenance
// route. Type observation and contract propagation consume the same query shape:
// observation projects source type forward; paramevidence inverts the projection
// into a source contract.
type RouteSourceQuery struct {
	Path       constraint.Path
	ReadOrder  []SourceReadKind
	Segments   []constraint.Segment
	Projection RouteProjectionKind
}

// RouteSourceType projects the best source type available for route. It applies
// route semantics once, while the caller supplies the evidence resolver.
func RouteSourceType(route flow.ProvenanceRoute, extra []constraint.Segment, resolve RouteSourceTypeResolver, project SegmentTypeProjector) typ.Type {
	if resolve == nil {
		return nil
	}
	var fallback typ.Type
	for _, query := range RouteSourceQueries(route, extra) {
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

// RouteSourceQueries lowers a provenance route into source reads and local
// projections. Evidence precedence is part of the query because it is a semantic
// property of the route, not of observation.
func RouteSourceQueries(route flow.ProvenanceRoute, extra []constraint.Segment) []RouteSourceQuery {
	if route.Source.Symbol == 0 {
		return nil
	}
	switch route.Kind {
	case flow.ProvenanceRouteIdentityAlias:
		return []RouteSourceQuery{{
			Path:       route.Source,
			ReadOrder:  []SourceReadKind{SourceReadBodyContract},
			Segments:   cloneSegments(extra),
			Projection: RouteProjectionDirect,
		}}
	case flow.ProvenanceRouteIndexedIterator:
		if route.VarIndex != 1 {
			return nil
		}
		return []RouteSourceQuery{{
			Path:       route.Source,
			ReadOrder:  []SourceReadKind{SourceReadBodyContract},
			Segments:   joinedSegments(route.Remainder, extra),
			Projection: RouteProjectionIndexedIteratorValue,
		}}
	case flow.ProvenanceRouteKeyedIterator:
		projection := RouteProjectionKeyedIteratorValue
		if route.VarIndex == 0 {
			projection = RouteProjectionKeyedIteratorKey
		} else if route.VarIndex != 1 {
			return nil
		}
		return []RouteSourceQuery{{
			Path:       route.Source,
			ReadOrder:  []SourceReadKind{SourceReadBodyContract},
			Segments:   joinedSegments(route.Remainder, extra),
			Projection: projection,
		}}
	case flow.ProvenanceRouteAppendElementField:
		return appendElementFieldRouteSourceQueries(route, extra)
	default:
		return nil
	}
}

// ProjectType applies q's route projection to source.
func (q RouteSourceQuery) ProjectType(source typ.Type, project SegmentTypeProjector) typ.Type {
	if source == nil {
		return nil
	}
	local := ApplyRouteProjection(q.Projection, source, routeTypeProjection)
	if local == nil || len(q.Segments) == 0 {
		return local
	}
	if project == nil {
		return nil
	}
	return project(local, q.Segments)
}

var routeTypeProjection = RouteProjectionAlgebra[typ.Type]{
	IndexedIteratorValue: querycore.ElementType,
	KeyedIteratorKey:     querycore.EntryKeyType,
	KeyedIteratorValue:   querycore.EntryValueType,
	SequenceElement:      querycore.ElementType,
}

func appendElementFieldRouteSourceQueries(route flow.ProvenanceRoute, extra []constraint.Segment) []RouteSourceQuery {
	if len(route.SourceField) > 0 {
		return []RouteSourceQuery{{
			Path:       route.Source,
			ReadOrder:  []SourceReadKind{SourceReadPointPath, SourceReadBodyContract},
			Segments:   joinedSegments(joinedSegments(route.SourceField, route.FieldRemainder), extra),
			Projection: RouteProjectionSequenceElement,
		}}
	}
	return []RouteSourceQuery{{
		Path:       appendSegments(route.Source, joinedSegments(route.FieldRemainder, extra)),
		ReadOrder:  []SourceReadKind{SourceReadBodyContract, SourceReadPointPath},
		Projection: RouteProjectionDirect,
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
