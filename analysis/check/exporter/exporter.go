// Package exporter projects closed equation facts into a module export type.
package exporter

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/engine"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/module/exportrelation"
	"github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

// Derive returns the first static return type from evaluated facts.
func Derive(result engine.Result) typ.Type {
	order := operationOrder(result.Artifact)
	var alternatives []typ.Type
	for _, fact := range result.ReturnCandidates {
		candidate, slot, ok := returnCandidate(fact)
		if !ok || slot != "0" {
			continue
		}
		alternatives = append(alternatives, deriveValue(fact.Value, candidate, result, order))
	}
	if len(alternatives) == 0 {
		return typ.Unknown
	}
	return typ.MaterializeUnion(alternatives)
}

// DeriveSummary publishes only direct, unconditional return templates.
func DeriveSummary(result engine.Result, source string) exportrelation.Summary {
	export := Derive(result)
	return exportrelation.Summary{Type: export, Functions: deriveFunctions(source, export, nil, nil)}
}

// DeriveSummaryWithImports forwards only an already-published import relation.
func DeriveSummaryWithImports(result engine.Result, source string, imports map[string]exportrelation.Summary, aliases map[string]string) exportrelation.Summary {
	export := Derive(result)
	return exportrelation.Summary{Type: export, Functions: deriveFunctions(source, export, imports, aliases)}
}

func deriveFunctions(source string, export typ.Type, imports map[string]exportrelation.Summary, aliases map[string]string) []exportrelation.Function {
	stmts, ok := parseFunctionStatements(source)
	if !ok {
		return nil
	}
	scope := trackFunctionScope(stmts)
	return functionRelations(scope, returnedModuleRoots(stmts), export, imports, aliases)
}

func parseFunctionStatements(source string) ([]ast.Stmt, bool) {
	stmts, err := parse.ParseString(source, "<export-relation>")
	return stmts, err == nil
}

type functionScope struct {
	locals       map[string]*ast.FunctionExpr
	functions    map[string]*ast.FunctionExpr
	storeAliases map[string]bool
	stores       map[string]bool
}

func trackFunctionScope(stmts []ast.Stmt) functionScope {
	scope := functionScope{
		locals:       make(map[string]*ast.FunctionExpr),
		functions:    make(map[string]*ast.FunctionExpr),
		storeAliases: make(map[string]bool),
		stores:       make(map[string]bool),
	}
	for _, stmt := range stmts {
		switch item := stmt.(type) {
		case *ast.LocalAssignStmt:
			for i, name := range item.Names {
				if i < len(item.Exprs) {
					if fn, ok := item.Exprs[i].(*ast.FunctionExpr); ok {
						scope.locals[name] = fn
						delete(scope.storeAliases, name)
						continue
					}
					if ownershipStoreReference(item.Exprs[i]) {
						scope.storeAliases[name] = true
						delete(scope.locals, name)
						continue
					}
				}
				delete(scope.locals, name)
				delete(scope.storeAliases, name)
			}
		case *ast.FuncDefStmt:
			if item == nil || item.Func == nil {
				continue
			}
			if path, ok := functionPath(item.Name); ok {
				scope.functions[path] = item.Func
				delete(scope.stores, path)
				if !strings.Contains(path, ".") {
					delete(scope.storeAliases, path)
				}
			} else if item.Name != nil {
				if ident, ok := item.Name.Func.(*ast.IdentExpr); ok {
					scope.locals[ident.Value] = item.Func
					delete(scope.storeAliases, ident.Value)
				}
			}
		case *ast.AssignStmt:
			for i, left := range item.Lhs {
				if i >= len(item.Rhs) {
					continue
				}
				path, ok := memberPath(left)
				if !ok {
					if ident, identOK := left.(*ast.IdentExpr); identOK {
						delete(scope.locals, ident.Value)
						delete(scope.storeAliases, ident.Value)
						invalidateFunctions(scope.functions, ident.Value)
						invalidateStoreRelations(scope.stores, ident.Value)
					}
					continue
				}
				if ident, identOK := left.(*ast.IdentExpr); identOK {
					// Replacing a root invalidates every relation rooted there.
					delete(scope.locals, ident.Value)
					delete(scope.storeAliases, ident.Value)
					invalidateFunctions(scope.functions, ident.Value)
					invalidateStoreRelations(scope.stores, ident.Value)
					continue
				}
				if ownershipStoreReference(item.Rhs[i]) {
					scope.stores[path] = true
					delete(scope.functions, path)
					continue
				}
				if ident, ok := item.Rhs[i].(*ast.IdentExpr); ok && scope.storeAliases[ident.Value] {
					scope.stores[path] = true
					delete(scope.functions, path)
					continue
				}
				delete(scope.stores, path)
				if ident, ok := item.Rhs[i].(*ast.IdentExpr); ok && scope.locals[ident.Value] != nil {
					scope.functions[path] = scope.locals[ident.Value]
					continue
				}
				delete(scope.functions, path)
			}
		}
	}
	return scope
}

func functionRelations(scope functionScope, roots map[string]bool, export typ.Type, imports map[string]exportrelation.Summary, aliases map[string]string) []exportrelation.Function {
	paths := make([]string, 0, len(scope.functions)+len(scope.stores))
	for path := range scope.functions {
		paths = append(paths, path)
	}
	for path := range scope.stores {
		if scope.functions[path] == nil {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	out := make([]exportrelation.Function, 0, len(paths))
	for _, path := range paths {
		relative := path
		if cut := strings.IndexByte(path, '.'); cut > 0 {
			relative = path[cut+1:]
		}
		if scope.stores[path] && returnedModuleMember(path, roots) {
			out = append(out, exportrelation.Function{
				Path: relative, Arity: 2, Store: &exportrelation.OwnershipStore{Value: 0, Owner: 1},
			})
			continue
		}
		if relation, ok := functionRelation(relative, scope.functions[path], imports, aliases); ok && publishedFunction(export, relative, relation.Arity) {
			out = append(out, relation)
		}
	}
	return out
}

func invalidateFunctions(functions map[string]*ast.FunctionExpr, root string) {
	for path := range functions {
		if path == root || strings.HasPrefix(path, root+".") {
			delete(functions, path)
		}
	}
}

func invalidateStoreRelations(stores map[string]bool, root string) {
	for path := range stores {
		if path == root || strings.HasPrefix(path, root+".") {
			delete(stores, path)
		}
	}
}

func returnedModuleRoots(stmts []ast.Stmt) map[string]bool {
	roots := make(map[string]bool)
	for _, stmt := range stmts {
		ret, ok := stmt.(*ast.ReturnStmt)
		if !ok || len(ret.Exprs) != 1 {
			continue
		}
		if root, ok := ret.Exprs[0].(*ast.IdentExpr); ok && root.Value != "" {
			roots[root.Value] = true
		}
	}
	return roots
}

func returnedModuleMember(path string, roots map[string]bool) bool {
	root, _, found := strings.Cut(path, ".")
	return found && roots[root]
}

// ownershipStoreReference admits only ownership.store.
func ownershipStoreReference(expr ast.Expr) bool {
	member, ok := expr.(*ast.AttrGetExpr)
	if !ok || ast.KeyName(member.Key) != "store" {
		return false
	}
	global, ok := member.Object.(*ast.IdentExpr)
	return ok && global.Value == "ownership"
}

// publishedFunction requires the checked export to publish path at arity.
func publishedFunction(export typ.Type, path string, arity int) bool {
	if path == "" || arity < 0 {
		return false
	}
	current := export
	for _, name := range strings.Split(path, ".") {
		record, ok := unwrap.Alias(current).(*typ.Record)
		if !ok || record == nil {
			return false
		}
		field := record.GetField(name)
		if field == nil || field.Optional || field.Type == nil {
			return false
		}
		current = field.Type
	}
	function, ok := unwrap.Alias(current).(*typ.Function)
	return ok && function != nil && function.Variadic == nil && len(function.Params) == arity
}

func functionPath(name *ast.FuncName) (string, bool) {
	if name == nil {
		return "", false
	}
	if name.Method != "" {
		base, ok := memberPath(name.Receiver)
		if !ok {
			return "", false
		}
		return base + "." + name.Method, true
	}
	return memberPath(name.Func)
}

func memberPath(expr ast.Expr) (string, bool) {
	switch value := expr.(type) {
	case *ast.IdentExpr:
		return value.Value, value.Value != ""
	case *ast.AttrGetExpr:
		base, ok := memberPath(value.Object)
		key := ast.KeyName(value.Key)
		return base + "." + key, ok && key != ""
	default:
		return "", false
	}
}

func functionRelation(path string, fn *ast.FunctionExpr, imports map[string]exportrelation.Summary, aliases map[string]string) (exportrelation.Function, bool) {
	if fn == nil || fn.ParList == nil || fn.ParList.HasVargs {
		return exportrelation.Function{}, false
	}
	params := make(map[string]int, len(fn.ParList.Names))
	for i, name := range fn.ParList.Names {
		params[name] = i
	}
	relation := exportrelation.Function{Path: path, Arity: len(params)}
	if store, ok := ownershipStoreRelation(fn, params); ok {
		relation.Store = store
		return relation, relation.Valid()
	}
	if tuples, ok := completeLiteralReturnTuples(fn.Stmts, params); ok && multiReturnTuples(tuples) {
		relation.ReturnTuples = tuples
		return relation, relation.Valid()
	}
	if len(fn.Stmts) == 1 {
		if ret, ok := fn.Stmts[0].(*ast.ReturnStmt); ok && len(ret.Exprs) == 1 {
			if value, ok := remapValue(ret.Exprs[0], params, nil); ok {
				relation.Return = value
				return relation, relation.Valid()
			}
			if value, ok := forwardedImportedReturn(ret.Exprs[0], params, imports, aliases); ok {
				relation.Return = value
				relation.Forwarded = true
				return relation, relation.Valid()
			}
		}
	}
	if len(fn.Stmts) == 2 {
		local, localOK := fn.Stmts[0].(*ast.LocalAssignStmt)
		ret, returnOK := fn.Stmts[1].(*ast.ReturnStmt)
		if localOK && returnOK && len(local.Names) == 1 && len(local.Exprs) == 1 && len(ret.Exprs) == 1 {
			if returned, ok := ret.Exprs[0].(*ast.IdentExpr); ok && returned.Value == local.Names[0] {
				value, ok := remapValue(local.Exprs[0], params, nil)
				relation.Return = value
				return relation, ok && relation.Valid()
			}
		}
	}
	if conditional, ok := conditionalReturnRelation(fn, params); ok {
		relation.Conditional = conditional
		return relation, relation.Valid()
	}
	return exportrelation.Function{}, false
}

func multiReturnTuples(tuples []exportrelation.ReturnTuple) bool {
	if len(tuples) == 0 {
		return false
	}
	for _, tuple := range tuples {
		if len(tuple.Values) < 2 {
			return false
		}
	}
	return true
}

// completeLiteralReturnTuples accepts only a complete finite control tree made
// from return statements and if statements. It publishes each tuple exactly
// as the exporter already publishes ordinary scalar/table return templates;
// an open fallthrough, loop, call, or unsupported expression rejects the
// whole catalog rather than inventing a provider fact.
func completeLiteralReturnTuples(stmts []ast.Stmt, params map[string]int) ([]exportrelation.ReturnTuple, bool) {
	var walk func([]ast.Stmt) ([]exportrelation.ReturnTuple, bool)
	walk = func(sequence []ast.Stmt) ([]exportrelation.ReturnTuple, bool) {
		for index, stmt := range sequence {
			switch value := stmt.(type) {
			case *ast.ReturnStmt:
				if len(value.Exprs) == 0 {
					return nil, false
				}
				tuple := exportrelation.ReturnTuple{Values: make([]exportrelation.Value, 0, len(value.Exprs))}
				for _, expr := range value.Exprs {
					item, ok := remapValue(expr, params, nil)
					if !ok {
						return nil, false
					}
					tuple.Values = append(tuple.Values, item)
				}
				return []exportrelation.ReturnTuple{tuple}, true
			case *ast.IfStmt:
				thenTuples, thenComplete := walk(value.Then)
				if !thenComplete {
					return nil, false
				}
				if value.HasElse {
					elseTuples, elseComplete := walk(value.Else)
					if !elseComplete {
						return nil, false
					}
					return append(thenTuples, elseTuples...), true
				}
				tail, tailComplete := walk(sequence[index+1:])
				if !tailComplete {
					return nil, false
				}
				return append(thenTuples, tail...), true
			default:
				return nil, false
			}
		}
		return nil, false
	}
	return walk(stmts)
}

// conditionalReturnRelation accepts one complete source-level literal branch
// followed by its normal return. Both result expressions must already be
// finite templates, so importing the relation cannot reconstruct a value from
// a declaration or an unclosed body fact.
func conditionalReturnRelation(fn *ast.FunctionExpr, params map[string]int) (*exportrelation.ConditionalReturn, bool) {
	if fn == nil || len(fn.Stmts) != 2 {
		return nil, false
	}
	branch, branchOK := fn.Stmts[0].(*ast.IfStmt)
	fallback, fallbackOK := fn.Stmts[1].(*ast.ReturnStmt)
	if !branchOK || !fallbackOK || branch.HasElse || len(branch.Then) != 1 || len(fallback.Exprs) != 1 {
		return nil, false
	}
	matched, matchedOK := branch.Then[0].(*ast.ReturnStmt)
	if !matchedOK || len(matched.Exprs) != 1 {
		return nil, false
	}
	parameter, literal, predicateOK := literalParameterPredicate(branch.Condition, params)
	if !predicateOK {
		return nil, false
	}
	match, matchOK := remapValue(matched.Exprs[0], params, nil)
	otherwise, otherwiseOK := remapValue(fallback.Exprs[0], params, nil)
	if !matchOK || !otherwiseOK {
		return nil, false
	}
	return &exportrelation.ConditionalReturn{Parameter: parameter, Literal: literal, Match: match, Otherwise: otherwise}, true
}

func literalParameterPredicate(expr ast.Expr, params map[string]int) (int, string, bool) {
	relation, ok := expr.(*ast.RelationalOpExpr)
	if !ok || relation.Operator != "==" {
		return 0, "", false
	}
	for _, candidate := range []struct {
		parameter ast.Expr
		literal   ast.Expr
	}{{relation.Lhs, relation.Rhs}, {relation.Rhs, relation.Lhs}} {
		name, nameOK := candidate.parameter.(*ast.IdentExpr)
		if !nameOK {
			continue
		}
		parameter, parameterOK := params[name.Value]
		literal, literalOK := remapValue(candidate.literal, params, nil)
		if !parameterOK || !literalOK || !literal.Closed() || literal.Scalar == "" {
			continue
		}
		return parameter, literal.Scalar, true
	}
	return 0, "", false
}

// ownershipStoreRelation admits only positional ownership.store wrappers.
func ownershipStoreRelation(fn *ast.FunctionExpr, params map[string]int) (*exportrelation.OwnershipStore, bool) {
	if fn == nil || len(fn.Stmts) != 1 {
		return nil, false
	}
	statement, ok := fn.Stmts[0].(*ast.FuncCallStmt)
	if !ok {
		return nil, false
	}
	call, ok := statement.Expr.(*ast.FuncCallExpr)
	if !ok || call.Receiver != nil || call.Method != "" || len(call.Args) != 2 {
		return nil, false
	}
	callee, ok := call.Func.(*ast.AttrGetExpr)
	if !ok || ast.KeyName(callee.Key) != "store" {
		return nil, false
	}
	global, ok := callee.Object.(*ast.IdentExpr)
	if !ok || global.Value != "ownership" {
		return nil, false
	}
	value, valueOK := call.Args[0].(*ast.IdentExpr)
	owner, ownerOK := call.Args[1].(*ast.IdentExpr)
	if !valueOK || !ownerOK {
		return nil, false
	}
	valueIndex, valueOK := params[value.Value]
	ownerIndex, ownerOK := params[owner.Value]
	if !valueOK || !ownerOK || valueIndex == ownerIndex {
		return nil, false
	}
	return &exportrelation.OwnershipStore{Value: valueIndex, Owner: ownerIndex}, true
}

func forwardedImportedReturn(expr ast.Expr, params map[string]int, imports map[string]exportrelation.Summary, aliases map[string]string) (exportrelation.Value, bool) {
	call, ok := expr.(*ast.FuncCallExpr)
	if !ok || call.Receiver != nil || call.Method != "" {
		return exportrelation.Value{}, false
	}
	callee, ok := call.Func.(*ast.AttrGetExpr)
	if !ok {
		return exportrelation.Value{}, false
	}
	module, ok := callee.Object.(*ast.IdentExpr)
	if !ok || aliases == nil || imports == nil {
		return exportrelation.Value{}, false
	}
	modulePath, found := aliases[module.Value]
	if !found {
		return exportrelation.Value{}, false
	}
	name := ast.KeyName(callee.Key)
	summary, found := imports[modulePath]
	if !found || name == "" {
		return exportrelation.Value{}, false
	}
	function, found := summary.Function(name, len(call.Args))
	if !found {
		return exportrelation.Value{}, false
	}
	arguments := make([]exportrelation.Value, len(call.Args))
	for index, argument := range call.Args {
		value, ok := remapValue(argument, params, nil)
		if !ok {
			return exportrelation.Value{}, false
		}
		arguments[index] = value
	}
	return composeForwardedRelation(function.Return, arguments)
}

// composeForwardedRelation substitutes the already-published caller argument
// templates into one imported return relation. Both inputs are finite export
// facts; an unresolved parameter or malformed nested template is rejected
// instead of inventing a return member.
func composeForwardedRelation(value exportrelation.Value, arguments []exportrelation.Value) (exportrelation.Value, bool) {
	if value.Parameter != nil {
		index := *value.Parameter
		if index < 0 || index >= len(arguments) {
			return exportrelation.Value{}, false
		}
		return arguments[index], true
	}
	if value.Scalar != "" {
		return exportrelation.Value{Scalar: value.Scalar}, true
	}
	if len(value.Table) == 0 {
		return exportrelation.Value{}, false
	}
	result := exportrelation.Value{Table: make([]exportrelation.Member, 0, len(value.Table))}
	for _, member := range value.Table {
		child, ok := composeForwardedRelation(member.Value, arguments)
		if !ok || member.Suffix == "" {
			return exportrelation.Value{}, false
		}
		result.Table = append(result.Table, exportrelation.Member{Suffix: member.Suffix, Value: child})
	}
	return result, true
}

func remapValue(value any, params map[string]int, arguments []int) (exportrelation.Value, bool) {
	switch value := value.(type) {
	case *ast.IdentExpr:
		parameter, ok := params[value.Value]
		if !ok {
			return exportrelation.Value{}, false
		}
		return exportrelation.Value{Parameter: &parameter}, true
	case *ast.StringExpr:
		return exportrelation.Value{Scalar: "scalar/string/" + strconv.Quote(value.Value)}, true
	case *ast.NumberExpr:
		if _, err := strconv.ParseFloat(value.Value, 64); err != nil {
			return exportrelation.Value{}, false
		}
		return exportrelation.Value{Scalar: "scalar/number/" + value.Value}, true
	case *ast.TrueExpr:
		return exportrelation.Value{Scalar: "scalar/bool/true"}, true
	case *ast.FalseExpr:
		return exportrelation.Value{Scalar: "scalar/bool/false"}, true
	case *ast.NilExpr:
		return exportrelation.Value{Scalar: "scalar/nil"}, true
	case *ast.TableExpr:
		members := make([]exportrelation.Member, 0, len(value.Fields))
		array := 1
		for _, field := range value.Fields {
			if field == nil {
				return exportrelation.Value{}, false
			}
			suffix := ""
			if field.Key == nil {
				suffix = "[" + strconv.Itoa(array) + "]"
				array++
			} else if name := ast.KeyName(field.Key); name != "" {
				suffix = "." + name
			} else if number, ok := field.Key.(*ast.NumberExpr); ok {
				suffix = "[" + number.Value + "]"
			} else {
				return exportrelation.Value{}, false
			}
			child, ok := remapValue(field.Value, params, arguments)
			if !ok {
				return exportrelation.Value{}, false
			}
			members = append(members, exportrelation.Member{Suffix: suffix, Value: child})
		}
		return exportrelation.Value{Table: members}, len(members) != 0
	case exportrelation.Value:
		if value.Parameter != nil {
			parameter := *value.Parameter
			if parameter < 0 || parameter >= len(arguments) {
				return exportrelation.Value{}, false
			}
			parameter = arguments[parameter]
			return exportrelation.Value{Parameter: &parameter}, true
		}
		if value.Scalar != "" {
			return exportrelation.Value{Scalar: value.Scalar}, true
		}
		if len(value.Table) == 0 {
			return exportrelation.Value{}, false
		}
		out := exportrelation.Value{Table: make([]exportrelation.Member, 0, len(value.Table))}
		for _, member := range value.Table {
			child, ok := remapValue(member.Value, params, arguments)
			if !ok {
				return exportrelation.Value{}, false
			}
			out.Table = append(out.Table, exportrelation.Member{Suffix: member.Suffix, Value: child})
		}
		return out, true
	default:
		return exportrelation.Value{}, false
	}
}

func returnCandidate(fact equation.Fact) (candidate, slot string, ok bool) {
	parts := strings.Split(fact.Key, "/")
	if len(parts) != 3 || parts[0] != "return-candidate" || parts[1] == "" || parts[2] == "arity" {
		return "", "", false
	}
	return parts[1], parts[2], true
}

func operationOrder(artifact equation.Artifact) map[string]int {
	out := make(map[string]int, len(artifact.Equations))
	for index, operation := range artifact.Equations {
		out[operation.Target.Name] = index
	}
	return out
}

func deriveValue(value []byte, candidate string, result engine.Result, order map[string]int) typ.Type {
	if shape, ok := shapefact.DecodeTable(value); ok {
		fields := decodeTableFields(shape)
		if root, ok := returnRoot(result.Artifact, candidate); ok {
			overlayStaticWrites(fields, root, candidate, result.ValueFacts, order)
			if hasDynamicMutation(result.Artifact, root, candidate, order) {
				return typ.Unknown
			}
		}
		enrichInferredMemberReturns(fields, result.ValueFacts)
		return buildRecord(fields)
	}
	return scalarType(value)
}

// enrichInferredMemberReturns attaches an exported member closure's engine
// inferred return to a function field that declared none. The engine published
// exactly one summary per returned member suffix; a declared return keeps the
// closure's own canonical signature and is left untouched here.
func enrichInferredMemberReturns(fields map[fieldKey]typ.Type, valueFacts []equation.Fact) {
	var summaries map[string][]byte
	for _, fact := range valueFacts {
		if !strings.HasPrefix(fact.Key, returnMemberSummaryFieldPrefix) {
			continue
		}
		name := strings.TrimPrefix(fact.Key, returnMemberSummaryFieldPrefix)
		if name == "" || strings.ContainsAny(name, ".[]") {
			continue
		}
		if summaries == nil {
			summaries = make(map[string][]byte)
		}
		summaries[name] = fact.Value
	}
	if summaries == nil {
		return
	}
	for key, current := range fields {
		if key.kind != segment.SegmentField {
			continue
		}
		encoded, ok := summaries[key.name]
		if !ok {
			continue
		}
		function, ok := unwrap.Alias(current).(*typ.Function)
		if !ok || function == nil || len(function.Returns) != 0 || function.Variadic != nil {
			continue
		}
		returnType, err := typ.DecodeCanonicalStructural(context.Background(), encoded)
		if err != nil || returnType == nil {
			continue
		}
		fields[key] = typ.RebuildFunction(typ.FunctionParts{
			TypeParams: function.TypeParams,
			Params:     function.Params,
			Variadic:   function.Variadic,
			Returns:    []typ.Type{returnType},
		})
	}
}

// returnMemberSummaryFieldPrefix selects only named-field member summaries. The
// engine publishes each summary under the member's formatted suffix, so a
// record field name follows the leading dot.
const returnMemberSummaryFieldPrefix = "return-member-summary/."

func hasDynamicMutation(artifact equation.Artifact, root, candidate string, order map[string]int) bool {
	returnOrder, exists := order[candidate]
	if !exists {
		return true
	}
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind != "index-mutation" || order[operation.Target.Name] >= returnOrder {
			continue
		}
		for _, operand := range operation.Operands {
			if operand.Role == "container" && string(operand.Term.Encoding) == root {
				return true
			}
		}
	}
	return false
}

type fieldKey struct {
	kind  segment.SegmentKind
	name  string
	index int
}

func decodeTableFields(shape shapefact.Table) map[fieldKey]typ.Type {
	fields := make(map[fieldKey]typ.Type)
	for _, member := range shape.Members {
		if !member.Present {
			continue
		}
		segments, ok := segment.ParseFormattedSegments(member.Suffix)
		if !ok || len(segments) != 1 {
			continue
		}
		part := segments[0]
		fields[fieldKey{kind: part.Kind, name: part.Name, index: part.Index}] = decodeType([]byte(member.Value))
	}
	return fields
}

func returnRoot(artifact equation.Artifact, candidate string) (string, bool) {
	for _, operation := range artifact.Equations {
		if operation.Target.Name != candidate || operation.Occurrence.Kind != "publication" {
			continue
		}
		for _, operand := range operation.Operands {
			if operand.Role == "return-value-00000000" && strings.HasPrefix(string(operand.Term.Encoding), "path/") {
				return string(operand.Term.Encoding), true
			}
		}
	}
	return "", false
}

func overlayStaticWrites(fields map[fieldKey]typ.Type, root, candidate string, values []equation.Fact, order map[string]int) {
	returnOrder, exists := order[candidate]
	if !exists {
		return
	}
	type latest struct {
		order int
		value []byte
	}
	latestByField := make(map[fieldKey]latest)
	prefix := "value/" + root
	for _, fact := range values {
		if !strings.HasPrefix(fact.Key, prefix) {
			continue
		}
		rest := strings.TrimPrefix(fact.Key, prefix)
		cut := strings.LastIndexByte(rest, '/')
		if cut <= 0 {
			continue
		}
		segments, ok := segment.ParseFormattedSegments(rest[:cut])
		if !ok || len(segments) != 1 {
			continue
		}
		writeOrder, exists := order[rest[cut+1:]]
		if !exists || writeOrder >= returnOrder {
			continue
		}
		part := segments[0]
		key := fieldKey{kind: part.Kind, name: part.Name, index: part.Index}
		if prior, exists := latestByField[key]; !exists || writeOrder > prior.order {
			latestByField[key] = latest{order: writeOrder, value: fact.Value}
		}
	}
	for key, value := range latestByField {
		if string(value.value) == "scalar/nil" {
			delete(fields, key)
			continue
		}
		fields[key] = decodeType(value.value)
	}
}

func buildRecord(fields map[fieldKey]typ.Type) typ.Type {
	builder := table.NewRecord().SetOpen(true)
	for key, value := range fields {
		switch key.kind {
		case segment.SegmentField:
			builder.Field(key.name, value)
		case segment.SegmentIndexString:
			builder.StaticStringIndex(key.name, value)
		case segment.SegmentIndexInt:
			builder.StaticIntIndex(int64(key.index), value)
		}
	}
	return builder.Build()
}

func decodeType(value []byte) typ.Type {
	if shape, ok := shapefact.DecodeTable(value); ok {
		return buildRecord(decodeTableFields(shape))
	}
	return scalarType(value)
}

func scalarType(value []byte) typ.Type {
	encoded := string(value)
	switch {
	case strings.HasPrefix(encoded, "scalar/function/"):
		wire, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(encoded, "scalar/function/"))
		if err != nil {
			return unknownFunction()
		}
		var signature struct {
			Canonical string `json:"canonical,omitempty"`
		}
		if json.Unmarshal(wire, &signature) != nil || signature.Canonical == "" {
			return unknownFunction()
		}
		canonical, err := base64.RawURLEncoding.DecodeString(signature.Canonical)
		if err != nil {
			return unknownFunction()
		}
		// Decode closed front function publications structurally.
		function, err := typ.DecodeCanonicalStructural(context.Background(), canonical)
		if err != nil {
			return unknownFunction()
		}
		if _, ok := unwrap.Alias(function).(*typ.Function); ok {
			return function
		}
		return unknownFunction()
	case encoded == "scalar/nil":
		return typ.Nil
	case encoded == "scalar/bool/true":
		return typ.True
	case encoded == "scalar/bool/false":
		return typ.False
	case strings.HasPrefix(encoded, "scalar/number/"):
		number := strings.TrimPrefix(encoded, "scalar/number/")
		if integer, err := strconv.ParseInt(number, 10, 64); err == nil {
			return typ.LiteralInt(integer)
		}
		if floating, err := strconv.ParseFloat(number, 64); err == nil {
			return typ.LiteralNumber(floating)
		}
		return typ.Unknown
	case strings.HasPrefix(encoded, "scalar/string/"):
		text, err := strconv.Unquote(strings.TrimPrefix(encoded, "scalar/string/"))
		if err != nil {
			return typ.Unknown
		}
		return typ.LiteralString(text)
	case strings.HasPrefix(encoded, "scalar/function"):
		return unknownFunction()
	default:
		return typ.Unknown
	}
}

func unknownFunction() typ.Type {
	return typ.Func().Variadic(typ.Unknown).Returns(typ.Unknown).Build()
}
