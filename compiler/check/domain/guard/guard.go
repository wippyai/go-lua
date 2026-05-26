package guard

import (
	"strings"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/pathkey"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// TruthyPathKey uniquely identifies a field path for truthy guard tracking.
type TruthyPathKey struct {
	Symbol cfg.SymbolID
	Field  string
}

// TypeProbe describes a builtin type(expr) equality check.
type TypeProbe struct {
	Expr ast.Expr
	Key  narrow.TypeKey
}

// TypeProbeComparison describes a builtin type(expr) comparison. Equal is
// true for `==` and false for `~=`.
type TypeProbeComparison struct {
	Probe TypeProbe
	Equal bool
}

// ExtractTypeProbeComparison extracts the runtime type predicate from a
// `type(expr) == "kind"` or `type(expr) ~= "kind"` comparison.
func ExtractTypeProbeComparison(expr ast.Expr) (TypeProbeComparison, bool) {
	rel, ok := expr.(*ast.RelationalOpExpr)
	if !ok || rel == nil {
		return TypeProbeComparison{}, false
	}
	equal := false
	switch rel.Operator {
	case "==":
		equal = true
	case "~=":
		equal = false
	default:
		return TypeProbeComparison{}, false
	}
	if probe, ok := typeProbeSide(rel.Lhs, rel.Rhs); ok {
		return TypeProbeComparison{Probe: probe, Equal: equal}, true
	}
	if probe, ok := typeProbeSide(rel.Rhs, rel.Lhs); ok {
		return TypeProbeComparison{Probe: probe, Equal: equal}, true
	}
	return TypeProbeComparison{}, false
}

// ExtractTypeEqualityProbe extracts the runtime type predicate from a
// `type(expr) == "kind"` comparison. It is intentionally expression-only so
// synthesis, field validation, and flow guard collection share one parser.
func ExtractTypeEqualityProbe(expr ast.Expr) (TypeProbe, bool) {
	cmp, ok := ExtractTypeProbeComparison(expr)
	if !ok || !cmp.Equal {
		return TypeProbe{}, false
	}
	return cmp.Probe, true
}

// IsTypeCall reports whether call has builtin type(expr) shape.
func IsTypeCall(call *ast.FuncCallExpr) bool {
	if call == nil || callsite.IsMethodLikeExpr(call) || len(call.Args) != 1 {
		return false
	}
	ident, ok := call.Func.(*ast.IdentExpr)
	return ok && ident != nil && ident.Value == "type"
}

// TypeForTypeKey returns the broad runtime type represented by a builtin
// type() result key.
func TypeForTypeKey(key narrow.TypeKey) typ.Type {
	if kind, ok := key.BuiltinKind(); ok {
		return narrow.TypeForKind(kind)
	}
	return typ.Unknown
}

// EvaluateTypeProbeComparison returns the boolean singleton for a builtin
// type() comparison when the current abstract value proves it. It returns
// typ.Boolean when gradual or union uncertainty remains, and typ.Never when
// the observed value is unreachable.
func EvaluateTypeProbeComparison(observed typ.Type, cmp TypeProbeComparison) typ.Type {
	truth, known := TypeProbeEqualityTruth(observed, cmp.Probe.Key)
	if !known {
		return typ.Boolean
	}
	if typ.IsNever(truth) {
		return typ.Never
	}
	if !cmp.Equal {
		if truth == typ.True {
			return typ.False
		}
		return typ.True
	}
	return truth
}

// TypeProbeEqualityTruth evaluates `type(observed) == key` as a singleton
// boolean when the abstract type is precise enough to prove or refute it.
func TypeProbeEqualityTruth(observed typ.Type, key narrow.TypeKey) (typ.Type, bool) {
	if observed == nil {
		return typ.Boolean, false
	}
	observed = typ.UnwrapAnnotated(unwrap.Alias(observed))
	if typ.IsNever(observed) {
		return typ.Never, true
	}
	if observed.Kind().IsPlaceholder() {
		return typ.Boolean, false
	}
	target := TypeForTypeKey(key)
	target = typ.UnwrapAnnotated(unwrap.Alias(target))
	if target == nil || typ.IsAny(target) || typ.IsUnknown(target) {
		return typ.Boolean, false
	}
	if subtype.IsSubtype(observed, target) {
		return typ.True, true
	}
	if !narrow.TypesOverlap(observed, target) {
		return typ.False, true
	}
	return typ.Boolean, false
}

// CollectTruthyGuards scans the CFG for conditions that establish truthy guards
// and propagates them to dominated points. Used to narrow optional types.
func CollectTruthyGuards(graph *cfg.Graph, branches []api.BranchEvidence, bindings *bind.BindingTable) map[cfg.Point]map[TruthyPathKey]bool {
	if graph == nil || bindings == nil {
		return nil
	}

	result := make(map[cfg.Point]map[TruthyPathKey]bool)

	for _, branch := range branches {
		branchPoint := branch.Point
		info := branch.Info
		if info == nil || info.Condition == nil {
			continue
		}

		succs := graph.Successors(branchPoint)
		var trueEdge cfg.Point
		for _, s := range succs {
			if cond, ok := graph.EdgeCond(branchPoint, s); ok && cond {
				trueEdge = s
				break
			}
		}
		if trueEdge == 0 {
			continue
		}

		keys := ExtractTruthyPathKeys(info.Condition, bindings)
		if len(keys) == 0 {
			continue
		}

		propagateTruthyGuards(graph, trueEdge, keys, result)
	}

	return result
}

// CollectTypeGuards scans branch conditions for builtin type() checks and
// propagates positive type facts to dominated points.
//
// Example:
//
//	if type(x.y) ~= "string" then return end
//	-- dominated fallthrough points get x.y : string
func CollectTypeGuards(graph *cfg.Graph, branches []api.BranchEvidence, bindings *bind.BindingTable) map[cfg.Point]map[TruthyPathKey]narrow.TypeKey {
	if graph == nil || bindings == nil {
		return nil
	}

	result := make(map[cfg.Point]map[TruthyPathKey]narrow.TypeKey)

	for _, branch := range branches {
		branchPoint := branch.Point
		info := branch.Info
		if info == nil || info.Condition == nil {
			continue
		}
		key, typeKey, hasTypeOnTrue, ok := extractTypeGuard(info.Condition, bindings)
		if !ok || key.Field == "" || typeKey.IsZero() {
			continue
		}

		succs := graph.Successors(branchPoint)
		var trueEdge cfg.Point
		var falseEdge cfg.Point
		for _, s := range succs {
			if cond, ok := graph.EdgeCond(branchPoint, s); ok {
				if cond {
					trueEdge = s
				} else {
					falseEdge = s
				}
			}
		}

		if hasTypeOnTrue {
			if trueEdge != 0 {
				propagateTypeGuards(graph, trueEdge, key, typeKey, result)
			}
		} else if falseEdge != 0 {
			propagateTypeGuards(graph, falseEdge, key, typeKey, result)
		}
	}

	return result
}

// ExtractTruthyPathKeys extracts path keys from expressions in truthy position.
func ExtractTruthyPathKeys(expr ast.Expr, bindings *bind.BindingTable) []TruthyPathKey {
	if expr == nil || bindings == nil {
		return nil
	}

	switch e := expr.(type) {
	case *ast.IdentExpr:
		if key, ok := TruthyKeyFromExpr(e, bindings); ok {
			return []TruthyPathKey{key}
		}
	case *ast.AttrGetExpr:
		if key, ok := TruthyKeyFromExpr(e, bindings); ok && key.Field != "" {
			return []TruthyPathKey{key}
		}
	case *ast.LogicalOpExpr:
		if e.Operator == "and" {
			var keys []TruthyPathKey
			keys = append(keys, ExtractTruthyPathKeys(e.Lhs, bindings)...)
			keys = append(keys, ExtractTruthyPathKeys(e.Rhs, bindings)...)
			return keys
		}
	}
	return nil
}

// TruthyKeyFromExpr builds a canonical guard key for static expression paths.
func TruthyKeyFromExpr(expr ast.Expr, bindings *bind.BindingTable) (TruthyPathKey, bool) {
	sym, segs, ok := callsite.StaticPathWithBaseSymbol(bindings, expr)
	if !ok || sym == 0 {
		return TruthyPathKey{}, false
	}
	return TruthyPathKey{
		Symbol: sym,
		Field:  truthyPathSuffix(segs),
	}, true
}

func truthyPathSuffix(segs []constraint.Segment) string {
	if len(segs) == 0 {
		return ""
	}
	suffix := pathkey.SegmentsSuffix(segs)
	return strings.TrimPrefix(suffix, ".")
}

// propagateTruthyGuards propagates truthy guards from a starting point to all reachable points
// until a join point (multiple predecessors from outside the region) is reached.
func propagateTruthyGuards(graph *cfg.Graph, start cfg.Point, keys []TruthyPathKey, result map[cfg.Point]map[TruthyPathKey]bool) {
	visited := make(map[cfg.Point]bool)
	worklist := []cfg.Point{start}

	for len(worklist) > 0 {
		p := worklist[0]
		worklist = worklist[1:]

		if visited[p] {
			continue
		}
		visited[p] = true

		if result[p] == nil {
			result[p] = make(map[TruthyPathKey]bool)
		}
		for _, key := range keys {
			result[p][key] = true
		}

		succs := graph.Successors(p)
		for _, succ := range succs {
			if !visited[succ] {
				preds := graph.Predecessors(succ)
				hasUnvisitedPred := false
				for _, pred := range preds {
					if !visited[pred] && pred != p {
						hasUnvisitedPred = true
						break
					}
				}
				if !hasUnvisitedPred {
					worklist = append(worklist, succ)
				}
			}
		}
	}
}

func propagateTypeGuards(
	graph *cfg.Graph,
	start cfg.Point,
	key TruthyPathKey,
	typeKey narrow.TypeKey,
	result map[cfg.Point]map[TruthyPathKey]narrow.TypeKey,
) {
	visited := make(map[cfg.Point]bool)
	worklist := []cfg.Point{start}

	for len(worklist) > 0 {
		p := worklist[0]
		worklist = worklist[1:]

		if visited[p] {
			continue
		}
		visited[p] = true

		if result[p] == nil {
			result[p] = make(map[TruthyPathKey]narrow.TypeKey)
		}
		result[p][key] = typeKey

		succs := graph.Successors(p)
		for _, succ := range succs {
			if visited[succ] {
				continue
			}
			preds := graph.Predecessors(succ)
			hasUnvisitedPred := false
			for _, pred := range preds {
				if !visited[pred] && pred != p {
					hasUnvisitedPred = true
					break
				}
			}
			if !hasUnvisitedPred {
				worklist = append(worklist, succ)
			}
		}
	}
}

// NarrowTableFieldsByGuard narrows optional record fields using truthy guards.
// When a table literal like {from = event.from} is inside a truthy guard for
// event.from, the field type should be narrowed from string? to string.
func NarrowTableFieldsByGuard(
	recType typ.Type,
	tbl *ast.TableExpr,
	p cfg.Point,
	bindings *bind.BindingTable,
	truthyGuards map[cfg.Point]map[TruthyPathKey]bool,
	typeGuards map[cfg.Point]map[TruthyPathKey]narrow.TypeKey,
) typ.Type {
	rec, ok := recType.(*typ.Record)
	if !ok || rec == nil || len(rec.Fields) == 0 {
		return recType
	}
	guards := truthyGuards[p]
	typeAtPoint := typeGuards[p]
	if guards == nil && typeAtPoint == nil {
		return recType
	}

	fieldSources := make(map[string]ast.Expr)
	for _, field := range tbl.Fields {
		if field == nil || field.Key == nil {
			continue
		}
		var name string
		switch k := field.Key.(type) {
		case *ast.StringExpr:
			name = k.Value
		case *ast.IdentExpr:
			name = k.Value
		}
		if name != "" {
			fieldSources[name] = field.Value
		}
	}

	changed := false
	newFields := make([]typ.Field, len(rec.Fields))
	copy(newFields, rec.Fields)

	for i, f := range newFields {
		originalOptional := f.Optional
		fieldType := f.Type
		srcExpr := fieldSources[f.Name]
		if srcExpr == nil {
			continue
		}
		attr, isAttr := srcExpr.(*ast.AttrGetExpr)
		if !isAttr {
			continue
		}
		key, ok := TruthyKeyFromExpr(attr, bindings)
		if !ok || key.Field == "" {
			continue
		}
		if guards != nil && guards[key] {
			if opt, isOpt := fieldType.(*typ.Optional); isOpt {
				fieldType = opt.Inner
			}
			if f.Optional || unwrap.IsOptionalLike(fieldType) {
				if nonNil := narrow.RemoveNil(fieldType); nonNil != nil && !typ.IsNever(nonNil) {
					fieldType = nonNil
				}
				newFields[i].Optional = false
			}
		}
		if typeAtPoint != nil {
			if tk, ok := typeAtPoint[key]; ok && !tk.IsZero() {
				if narrowed := narrow.ByTypeKey(fieldType, tk, nil); narrowed != nil && !typ.IsNever(narrowed) {
					fieldType = narrowed
				}
			}
		}
		if !typ.TypeEquals(fieldType, f.Type) {
			newFields[i].Type = fieldType
			changed = true
		}
		if newFields[i].Optional != originalOptional {
			changed = true
		}
	}

	if !changed {
		return recType
	}

	builder := typ.NewRecord()
	if rec.Open {
		builder.SetOpen(true)
	}
	for _, f := range newFields {
		switch {
		case f.Optional && f.Readonly:
			builder.OptReadonlyField(f.Name, f.Type)
		case f.Optional:
			builder.OptField(f.Name, f.Type)
		case f.Readonly:
			builder.ReadonlyField(f.Name, f.Type)
		default:
			builder.Field(f.Name, f.Type)
		}
	}
	if rec.Metatable != nil {
		builder.Metatable(rec.Metatable)
	}
	return builder.Build()
}

func extractTypeGuard(expr ast.Expr, bindings *bind.BindingTable) (TruthyPathKey, narrow.TypeKey, bool, bool) {
	rel, ok := expr.(*ast.RelationalOpExpr)
	if !ok || rel == nil {
		return TruthyPathKey{}, narrow.TypeKey{}, false, false
	}

	hasTypeOnTrue := false
	switch rel.Operator {
	case "==":
		hasTypeOnTrue = true
	case "~=":
		hasTypeOnTrue = false
	default:
		return TruthyPathKey{}, narrow.TypeKey{}, false, false
	}

	key, typeKey, ok := typeGuardPathAndKey(rel.Lhs, rel.Rhs, bindings)
	if ok {
		return key, typeKey, hasTypeOnTrue, true
	}
	key, typeKey, ok = typeGuardPathAndKey(rel.Rhs, rel.Lhs, bindings)
	if ok {
		return key, typeKey, hasTypeOnTrue, true
	}
	return TruthyPathKey{}, narrow.TypeKey{}, false, false
}

func typeGuardPathAndKey(typeExpr, keyExpr ast.Expr, bindings *bind.BindingTable) (TruthyPathKey, narrow.TypeKey, bool) {
	probe, ok := typeProbeSide(typeExpr, keyExpr)
	if !ok {
		return TruthyPathKey{}, narrow.TypeKey{}, false
	}

	key, ok := TruthyKeyFromExpr(probe.Expr, bindings)
	if !ok || key.Field == "" {
		return TruthyPathKey{}, narrow.TypeKey{}, false
	}
	return key, probe.Key, true
}

func typeProbeSide(typeExpr, keyExpr ast.Expr) (TypeProbe, bool) {
	call, ok := typeExpr.(*ast.FuncCallExpr)
	if !ok || !IsTypeCall(call) {
		return TypeProbe{}, false
	}
	typeName, ok := typeStringLiteral(keyExpr)
	if !ok {
		return TypeProbe{}, false
	}
	typeKey, ok := narrow.KnownBuiltinTypeKey(typeName)
	if !ok {
		return TypeProbe{}, false
	}
	return TypeProbe{Expr: call.Args[0], Key: typeKey}, true
}

func typeStringLiteral(expr ast.Expr) (string, bool) {
	if v, ok := expr.(*ast.StringExpr); ok {
		return v.Value, true
	}
	return "", false
}
