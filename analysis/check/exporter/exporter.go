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

// Derive returns the sound static type of a module's first return value. It
// consumes only evaluated equation facts. Opaque results become Unknown rather
// than a guessed structure.
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

// DeriveSummary adds finite return templates to the existing type export.  It
// accepts only direct, unconditional return expressions from a parsed producer
// body; unsupported control flow and expressions simply publish no relation.
func DeriveSummary(result engine.Result, source string) exportrelation.Summary {
	export := Derive(result)
	return exportrelation.Summary{Type: export, Functions: deriveFunctions(source, export)}
}

func deriveFunctions(source string, export typ.Type) []exportrelation.Function {
	stmts, err := parse.ParseString(source, "<export-relation>")
	if err != nil {
		return nil
	}
	locals := make(map[string]*ast.FunctionExpr)
	functions := make(map[string]*ast.FunctionExpr)
	for _, stmt := range stmts {
		switch item := stmt.(type) {
		case *ast.LocalAssignStmt:
			for i, name := range item.Names {
				if i < len(item.Exprs) {
					if fn, ok := item.Exprs[i].(*ast.FunctionExpr); ok {
						locals[name] = fn
						continue
					}
				}
				delete(locals, name)
			}
		case *ast.FuncDefStmt:
			if item == nil || item.Func == nil {
				continue
			}
			if path, ok := functionPath(item.Name); ok {
				functions[path] = item.Func
			} else if item.Name != nil {
				if ident, ok := item.Name.Func.(*ast.IdentExpr); ok {
					locals[ident.Value] = item.Func
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
						delete(locals, ident.Value)
						invalidateFunctions(functions, ident.Value)
					}
					continue
				}
				if ident, ok := item.Rhs[i].(*ast.IdentExpr); ok && locals[ident.Value] != nil {
					functions[path] = locals[ident.Value]
					continue
				}
				delete(functions, path)
			}
		}
	}
	paths := make([]string, 0, len(functions))
	for path := range functions {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	out := make([]exportrelation.Function, 0, len(paths))
	for _, path := range paths {
		relative := path
		if cut := strings.IndexByte(path, '.'); cut > 0 {
			relative = path[cut+1:]
		}
		if relation, ok := functionRelation(relative, functions[path]); ok && publishedFunction(export, relative, relation.Arity) {
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

// publishedFunction ties a source-level candidate to the checked module
// export. A parsed member name alone is not authority to create an import fact.
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

func functionRelation(path string, fn *ast.FunctionExpr) (exportrelation.Function, bool) {
	if fn == nil || fn.ParList == nil || fn.ParList.HasVargs {
		return exportrelation.Function{}, false
	}
	params := make(map[string]int, len(fn.ParList.Names))
	for i, name := range fn.ParList.Names {
		params[name] = i
	}
	relation := exportrelation.Function{Path: path, Arity: len(params)}
	if len(fn.Stmts) == 1 {
		if ret, ok := fn.Stmts[0].(*ast.ReturnStmt); ok && len(ret.Exprs) == 1 {
			value, ok := templateValue(ret.Exprs[0], params)
			relation.Return = value
			return relation, ok && relation.Valid()
		}
	}
	return exportrelation.Function{}, false
}

func templateValue(expr ast.Expr, params map[string]int) (exportrelation.Value, bool) {
	switch value := expr.(type) {
	case *ast.IdentExpr:
		i, ok := params[value.Value]
		if !ok {
			return exportrelation.Value{}, false
		}
		return exportrelation.Value{Parameter: &i}, true
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
			child, ok := templateValue(field.Value, params)
			if !ok {
				return exportrelation.Value{}, false
			}
			members = append(members, exportrelation.Member{Suffix: suffix, Value: child})
		}
		return exportrelation.Value{Table: members}, len(members) != 0
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
		fields := tableFields(shape)
		if root, ok := returnRoot(result.Artifact, candidate); ok {
			overlayStaticWrites(fields, root, candidate, result.ValueFacts, order)
			if hasDynamicMutation(result.Artifact, root, candidate, order) {
				return typ.Unknown
			}
		}
		return buildRecord(fields)
	}
	return scalarType(value)
}

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

func tableFields(shape shapefact.Table) map[fieldKey]typ.Type {
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
		fields[fieldKey{kind: part.Kind, name: part.Name, index: part.Index}] = scalarOrTableType([]byte(member.Value))
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
		fields[key] = scalarOrTableType(value.value)
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

func scalarOrTableType(value []byte) typ.Type {
	if shape, ok := shapefact.DecodeTable(value); ok {
		return buildRecord(tableFields(shape))
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
		// This payload is the front's closed function publication. Recursive
		// aliases in a signature are structural here: the exported manifest
		// needs their graph, not the producer's declaration identity.
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
