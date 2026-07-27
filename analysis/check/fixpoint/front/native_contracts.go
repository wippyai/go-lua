package front

// This file turns immutable, binder-owned lexical topology into descriptive
// native contracts.  It deliberately does not infer a value: every row below
// is about syntax/binding topology (entry layout, a closed local call edge, or
// a closed recursive component).  The engine publishes these descriptors with
// its ordinary value closure after it has completed evaluation.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
)

type NativeContract struct {
	Family string
	// Key is an exact lowering-owned publication key. Ordinary descriptors
	// leave it empty and receive the stable family/contract ordinal spelling.
	// Coordinate-keyed native families use the canonical factkey record and
	// BuildKey path before entering the publication transport.
	Key factkey.Key
	// Source asks the publication kernel to read this term from the closed
	// partition and render its exact constant word. It is empty for static
	// descriptor values.
	Source string
	Value  string
	// Subject is the closed term encoding of the binding the contract concerns,
	// in the same spelling the equations publish an operand term in. Publication
	// anchors the row on it, so a source display name reaches the consumer
	// without a second name resolution. It is empty when no single binding owns
	// the row.
	Subject     string
	Revocations []string
}

// NativeProjection is the typed publication form for a native descriptor whose
// public key does not carry all of its source display and validity metadata.
// Producers fill this record before evaluation; Result.Native only decodes the
// guarded fact and never re-walks WIR or CFG to recover those fields.
type NativeProjection struct {
	Version     uint8                        `json:"version"`
	Key         string                       `json:"key"`
	Value       string                       `json:"value"`
	Term        string                       `json:"term,omitempty"`
	Subject     string                       `json:"subject,omitempty"`
	Occurrence  string                       `json:"occurrence,omitempty"`
	Established string                       `json:"established,omitempty"`
	Revoked     string                       `json:"revoked,omitempty"`
	Event       string                       `json:"event,omitempty"`
	Revocations []NativeProjectionRevocation `json:"revocations,omitempty"`
	// HostGlobal is the project-boundary capability the semantic-tail
	// publication kernel must validate before exposing this row. Lowering owns
	// the call coordinate and path; the engine-owned global type map supplies
	// the stable host binding and resolved result contract.
	HostGlobal *NativeHostGlobalRequirement `json:"host_global,omitempty"`
}

type NativeHostGlobalRequirement struct {
	Root   string   `json:"root"`
	Fields []string `json:"fields,omitempty"`
}

type NativeProjectionRevocation struct {
	Established string `json:"established,omitempty"`
	Revoked     string `json:"revoked,omitempty"`
	Event       string `json:"event,omitempty"`
}

func EncodeNativeProjection(row NativeProjection) ([]byte, error) {
	row.Version = 1
	if row.Key == "" || row.Value == "" {
		return nil, fmt.Errorf("front: incomplete native projection")
	}
	return json.Marshal(row)
}

func DecodeNativeProjection(encoded []byte) (NativeProjection, bool) {
	var row NativeProjection
	if json.Unmarshal(encoded, &row) != nil || row.Version != 1 || row.Key == "" || row.Value == "" {
		return NativeProjection{}, false
	}
	return row, true
}

func nativeContracts(stmts []ast.Stmt, bindings *bind.Result, captureTransports map[wir.FunctionSymbolID]int) []NativeContract {
	if bindings == nil {
		return nil
	}
	byName := map[string]*ast.FunctionExpr{}
	functionNames := map[*ast.FunctionExpr]string{}
	var labelFunctions func([]ast.Stmt)
	labelFunctions = func(body []ast.Stmt) {
		for _, stmt := range body {
			switch item := stmt.(type) {
			case *ast.FuncDefStmt:
				if item.Func != nil && item.Name != nil {
					name := item.Name.Method
					if name == "" {
						if id, ok := item.Name.Func.(*ast.IdentExpr); ok {
							name = id.Value
						}
					}
					if name != "" {
						functionNames[item.Func] = name
					}
					labelFunctions(item.Func.Stmts)
				}
			case *ast.LocalAssignStmt:
				for i, expr := range item.Exprs {
					if i < len(item.Names) {
						if fn, ok := expr.(*ast.FunctionExpr); ok {
							functionNames[fn] = item.Names[i]
							labelFunctions(fn.Stmts)
						}
					}
				}
			case *ast.AssignStmt:
				for i, expr := range item.Rhs {
					if i < len(item.Lhs) {
						if fn, ok := expr.(*ast.FunctionExpr); ok {
							if id, ok := item.Lhs[i].(*ast.IdentExpr); ok {
								functionNames[fn] = id.Value
								labelFunctions(fn.Stmts)
							}
						}
					}
				}
			case *ast.IfStmt:
				labelFunctions(item.Then)
				labelFunctions(item.Else)
			case *ast.DoBlockStmt:
				labelFunctions(item.Stmts)
			}
		}
	}
	labelFunctions(stmts)
	literalMembers := map[string]bool{}
	for _, stmt := range stmts {
		local, ok := stmt.(*ast.LocalAssignStmt)
		if !ok {
			continue
		}
		for index, name := range local.Names {
			if index >= len(local.Exprs) {
				continue
			}
			table, ok := local.Exprs[index].(*ast.TableExpr)
			if !ok {
				continue
			}
			for _, field := range table.Fields {
				if field == nil || ast.KeyName(field.Key) == "" {
					continue
				}
				if _, callable := field.Value.(*ast.FunctionExpr); callable {
					literalMembers[name+"."+ast.KeyName(field.Key)] = true
				}
			}
		}
	}
	for _, origin := range bindings.FunctionOrigins() {
		name := ""
		if origin.HasTargetSymbol {
			name = bindings.Name(origin.TargetSymbol)
		}
		if def, ok := origin.Stmt.(*ast.FuncDefStmt); ok && def != nil && def.Name != nil {
			if id, ok := def.Name.Func.(*ast.IdentExpr); ok {
				name = id.Value
			}
			if def.Name.Method != "" {
				name = def.Name.Method
			}
		}
		if name == "" {
			name = functionNames[origin.Func]
		}
		if name == "" {
			continue
		}
		byName[name] = origin.Func
	}

	var out []NativeContract
	// An entry layout is an immutable property of the lowered lexical body. It
	// is reusable because it has no caller values in it.
	for _, fn := range bindings.Functions() {
		if fn == nil {
			continue
		}
		out = append(out, NativeContract{Family: "function_entry", Value: functionEntryValue(fn)})
	}

	var calls []callEdge
	var walkExpr func(*ast.FunctionExpr, ast.Expr)
	var walkStmts func(*ast.FunctionExpr, []ast.Stmt)
	walkExpr = func(owner *ast.FunctionExpr, expr ast.Expr) {
		switch x := expr.(type) {
		case *ast.FuncCallExpr:
			calls = append(calls, callEdge{owner: owner, call: x, target: callTargetName(x)})
			walkExpr(owner, x.Func)
			walkExpr(owner, x.Receiver)
			for _, a := range x.Args {
				walkExpr(owner, a)
			}
		case *ast.AttrGetExpr:
			walkExpr(owner, x.Object)
			walkExpr(owner, x.Key)
		case *ast.TableExpr:
			for _, f := range x.Fields {
				if f != nil {
					walkExpr(owner, f.Key)
					walkExpr(owner, f.Value)
				}
			}
		case *ast.LogicalOpExpr:
			walkExpr(owner, x.Lhs)
			walkExpr(owner, x.Rhs)
		case *ast.RelationalOpExpr:
			walkExpr(owner, x.Lhs)
			walkExpr(owner, x.Rhs)
		case *ast.StringConcatOpExpr:
			walkExpr(owner, x.Lhs)
			walkExpr(owner, x.Rhs)
		case *ast.ArithmeticOpExpr:
			walkExpr(owner, x.Lhs)
			walkExpr(owner, x.Rhs)
		case *ast.UnaryMinusOpExpr:
			walkExpr(owner, x.Expr)
		case *ast.UnaryNotOpExpr:
			walkExpr(owner, x.Expr)
		case *ast.UnaryLenOpExpr:
			walkExpr(owner, x.Expr)
		case *ast.UnaryBNotOpExpr:
			walkExpr(owner, x.Expr)
		case *ast.FunctionExpr:
			walkStmts(x, x.Stmts)
		}
	}
	walkStmts = func(owner *ast.FunctionExpr, body []ast.Stmt) {
		for _, stmt := range body {
			switch s := stmt.(type) {
			case *ast.AssignStmt:
				for _, x := range s.Lhs {
					walkExpr(owner, x)
				}
				for _, x := range s.Rhs {
					walkExpr(owner, x)
				}
			case *ast.LocalAssignStmt:
				for _, x := range s.Exprs {
					walkExpr(owner, x)
				}
			case *ast.FuncDefStmt:
				if s.Func != nil {
					walkStmts(s.Func, s.Func.Stmts)
				}
			case *ast.FuncCallStmt:
				walkExpr(owner, s.Expr)
			case *ast.ReturnStmt:
				for _, x := range s.Exprs {
					walkExpr(owner, x)
				}
			case *ast.IfStmt:
				walkExpr(owner, s.Condition)
				walkStmts(owner, s.Then)
				walkStmts(owner, s.Else)
			case *ast.DoBlockStmt:
				walkStmts(owner, s.Stmts)
			case *ast.WhileStmt:
				walkExpr(owner, s.Condition)
				walkStmts(owner, s.Stmts)
			case *ast.RepeatStmt:
				walkStmts(owner, s.Stmts)
				walkExpr(owner, s.Condition)
			case *ast.NumberForStmt:
				walkExpr(owner, s.Init)
				walkExpr(owner, s.Limit)
				walkExpr(owner, s.Step)
				walkStmts(owner, s.Stmts)
			case *ast.GenericForStmt:
				for _, x := range s.Exprs {
					walkExpr(owner, x)
				}
				walkStmts(owner, s.Stmts)
			}
		}
	}
	walkStmts(nil, stmts)

	stable := make(map[*ast.FuncCallExpr]bool)
	for _, closure := range bindings.LocalFunctionUseClosures() {
		if !closure.CallSetComplete {
			continue
		}
		for _, call := range closure.DirectCalls {
			stable[call] = true
		}
	}
	hasRequire := false
	for _, edge := range calls {
		hasRequire = hasRequire || edge.target == "require"
	}
	for _, edge := range calls {
		if edge.call == nil {
			continue
		}
		if edge.target == "require" {
			continue
		}
		if _, member := edge.call.Func.(*ast.AttrGetExpr); member && !isLiteralMemberCall(edge.call, literalMembers) {
			continue
		}
		value, revocations := "completeness=unknown", []string(nil)
		if stable[edge.call] && !functionHasParameterCall(byName[edge.target]) {
			value = "cardinality=1 completeness=complete"
		} else if edge.owner != nil && isTwoTargetLocal(edge.owner, edge.call) {
			value, revocations = "cardinality=2 completeness=incomplete", []string{"write.local"}
		} else if edge.owner != nil && isParameterCall(edge.owner, edge.call) {
			revocations = []string{"escape"}
		} else if hasRequire {
			revocations = []string{"write.field", "load.dynamic"}
		}
		// A literal table member is sealed by the same constructor fact that
		// made the member closure available.  Other table paths stay unknown.
		if isLiteralMemberCall(edge.call, literalMembers) {
			value, revocations = "cardinality=1 completeness=complete", []string{"write.field", "meta.set"}
		}
		out = append(out, NativeContract{Family: "callee_set", Value: value, Revocations: revocations})
	}

	// Static member writes, entry ownership and repeated layouts are binder-owned
	// lexical topology.  The record_construction row itself is WIR-owned: only
	// the resolved lowering carries the constructor's destination, which its
	// escape boundary is published from.
	out = append(out, recordNativeContracts(stmts)...)
	// Operation rows are derived from the same admitted, binder-owned syntax
	// tree as topology contracts and remain absent when a boundary is unknown.
	out = append(out, nativeOperationContracts(stmts, bindings, captureTransports)...)
	return out
}

// recordNativeContracts describes only closed literal layouts and writes whose
// receiver is a local literal binding in the same lexical body.  It deliberately
// does not model aliases, dynamic keys, or an optional field's absent shape.
// Those cases have no unique physical layout and must remain un-published.
// Entry ownership belongs to the WIR publisher next to record_construction:
// only the resolved lowering names the producer each entry value came from.
func recordNativeContracts(stmts []ast.Stmt) []NativeContract {
	var out []NativeContract
	shapes := make(map[string]int)

	var walkExpr func(ast.Expr, map[string]string)
	var walkStmts func([]ast.Stmt, map[string]string)
	// A closed constructor is named by its canonical field set, which is the
	// layout identity the rows below key on.
	constructor := func(table *ast.TableExpr) (string, bool) {
		if table == nil {
			return "", false
		}
		fields := make([]string, 0, len(table.Fields))
		for _, field := range table.Fields {
			if field == nil {
				return "", false
			}
			name := ast.KeyName(field.Key)
			if name == "" {
				return "", false
			}
			fields = append(fields, name)
		}
		sort.Strings(fields)
		return strings.Join(fields, ","), true
	}
	written := func(stmt *ast.AssignStmt) (string, bool) {
		if stmt == nil || len(stmt.Lhs) != 1 || len(stmt.Rhs) != 1 {
			return "", false
		}
		field, ok := stmt.Lhs[0].(*ast.AttrGetExpr)
		if !ok || field.KeySyntax != ast.AttrKeyDot {
			return "", false
		}
		owner, ok := field.Object.(*ast.IdentExpr)
		if !ok || owner.Value == "" {
			return "", false
		}
		if ast.KeyName(field.Key) == "" {
			return "", false
		}
		return owner.Value, true
	}
	walkExpr = func(expr ast.Expr, locals map[string]string) {
		switch item := expr.(type) {
		case *ast.TableExpr:
			if shape, ok := constructor(item); ok {
				shapes[shape]++
			}
			for _, field := range item.Fields {
				if field != nil {
					walkExpr(field.Value, locals)
				}
			}
		case *ast.FuncCallExpr:
			walkExpr(item.Func, locals)
			walkExpr(item.Receiver, locals)
			for _, arg := range item.Args {
				walkExpr(arg, locals)
			}
		case *ast.AttrGetExpr:
			walkExpr(item.Object, locals)
			walkExpr(item.Key, locals)
		case *ast.LogicalOpExpr:
			walkExpr(item.Lhs, locals)
			walkExpr(item.Rhs, locals)
		case *ast.RelationalOpExpr:
			walkExpr(item.Lhs, locals)
			walkExpr(item.Rhs, locals)
		case *ast.StringConcatOpExpr:
			walkExpr(item.Lhs, locals)
			walkExpr(item.Rhs, locals)
		case *ast.ArithmeticOpExpr:
			walkExpr(item.Lhs, locals)
			walkExpr(item.Rhs, locals)
		case *ast.UnaryMinusOpExpr:
			walkExpr(item.Expr, locals)
		case *ast.UnaryNotOpExpr:
			walkExpr(item.Expr, locals)
		case *ast.UnaryLenOpExpr:
			walkExpr(item.Expr, locals)
		case *ast.UnaryBNotOpExpr:
			walkExpr(item.Expr, locals)
		case *ast.FunctionExpr:
			walkStmts(item.Stmts, make(map[string]string))
		}
	}
	walkStmts = func(body []ast.Stmt, locals map[string]string) {
		for _, stmt := range body {
			switch item := stmt.(type) {
			case *ast.LocalAssignStmt:
				for index, expr := range item.Exprs {
					if index < len(item.Names) {
						if table, ok := expr.(*ast.TableExpr); ok {
							if shape, closed := constructor(table); closed {
								locals[item.Names[index]] = shape
								shapes[shape]++
								for _, field := range table.Fields {
									if field != nil {
										walkExpr(field.Value, locals)
									}
								}
								continue
							}
						}
					}
					walkExpr(expr, locals)
				}
			case *ast.AssignStmt:
				if owner, ok := written(item); ok {
					if _, closed := locals[owner]; closed {
						out = append(out, NativeContract{Family: "shape_transition", Value: "new_shape=published old_shape=published same_object_policy=published storage_offset=published transition_edge=published new_identity=minted old_identity_reused=false", Revocations: []string{"shape.transition"}})
						// The write establishes a distinct replacement layout.  Both
						// identities are published with the transition's deopt class.
						out = append(out,
							NativeContract{Family: "shape_identity", Value: "field_offsets=identical stable_across_sites=true", Revocations: []string{"shape.transition"}},
							NativeContract{Family: "shape_identity", Value: "field_offsets=identical stable_across_sites=true", Revocations: []string{"shape.transition"}},
						)
					}
				}
				for _, expr := range item.Lhs {
					walkExpr(expr, locals)
				}
				for _, expr := range item.Rhs {
					walkExpr(expr, locals)
				}
			case *ast.FuncDefStmt:
				if item.Func != nil {
					walkStmts(item.Func.Stmts, make(map[string]string))
				}
			case *ast.FuncCallStmt:
				walkExpr(item.Expr, locals)
			case *ast.ReturnStmt:
				for _, expr := range item.Exprs {
					walkExpr(expr, locals)
				}
			case *ast.IfStmt:
				walkExpr(item.Condition, locals)
				walkStmts(item.Then, make(map[string]string))
				walkStmts(item.Else, make(map[string]string))
			case *ast.DoBlockStmt:
				walkStmts(item.Stmts, make(map[string]string))
			case *ast.WhileStmt:
				walkExpr(item.Condition, locals)
				walkStmts(item.Stmts, make(map[string]string))
			case *ast.RepeatStmt:
				walkStmts(item.Stmts, make(map[string]string))
				walkExpr(item.Condition, locals)
			case *ast.NumberForStmt:
				walkExpr(item.Init, locals)
				walkExpr(item.Limit, locals)
				walkExpr(item.Step, locals)
				walkStmts(item.Stmts, make(map[string]string))
			case *ast.GenericForStmt:
				for _, expr := range item.Exprs {
					walkExpr(expr, locals)
				}
				walkStmts(item.Stmts, make(map[string]string))
			}
		}
	}
	walkStmts(stmts, make(map[string]string))
	for shape, count := range shapes {
		if shape == "" || count < 2 {
			continue
		}
		// A repeated complete layout is a structural identity.  The extra row is
		// the shared reader identity: it is the same canonical layout, not a new
		// allocation or a source-order-dependent shape.
		for index := 0; index < count+1; index++ {
			out = append(out, NativeContract{Family: "shape_identity", Value: "distinct_identities=1 field_offsets=identical field_order=canonical interned=true stable_across_sites=true", Revocations: []string{"shape.transition"}})
		}
	}
	return out
}

type callEdge struct {
	owner  *ast.FunctionExpr
	call   *ast.FuncCallExpr
	target string
}

func callTargetName(c *ast.FuncCallExpr) string {
	if c == nil {
		return ""
	}
	if x, ok := c.Func.(*ast.IdentExpr); ok {
		return x.Value
	}
	if x, ok := c.Func.(*ast.AttrGetExpr); ok {
		return ast.KeyName(x.Key)
	}
	return ""
}
func isLiteralMemberCall(c *ast.FuncCallExpr, members map[string]bool) bool {
	x, ok := c.Func.(*ast.AttrGetExpr)
	if !ok {
		return false
	}
	id, ok := x.Object.(*ast.IdentExpr)
	return ok && members[id.Value+"."+ast.KeyName(x.Key)]
}
func isParameterCall(fn *ast.FunctionExpr, c *ast.FuncCallExpr) bool {
	id, ok := c.Func.(*ast.IdentExpr)
	if !ok || fn == nil || fn.ParList == nil {
		return false
	}
	for _, name := range fn.ParList.Names {
		if name == id.Value {
			return true
		}
	}
	return false
}
func functionHasParameterCall(fn *ast.FunctionExpr) bool {
	if fn == nil {
		return false
	}
	var found bool
	var walk func([]ast.Stmt)
	walk = func(body []ast.Stmt) {
		for _, stmt := range body {
			switch item := stmt.(type) {
			case *ast.ReturnStmt:
				for _, expr := range item.Exprs {
					if call, ok := expr.(*ast.FuncCallExpr); ok && isParameterCall(fn, call) {
						found = true
					}
				}
			case *ast.FuncCallStmt:
				if call, ok := item.Expr.(*ast.FuncCallExpr); ok && isParameterCall(fn, call) {
					found = true
				}
			case *ast.IfStmt:
				walk(item.Then)
				walk(item.Else)
			}
		}
	}
	walk(fn.Stmts)
	return found
}
func isTwoTargetLocal(fn *ast.FunctionExpr, c *ast.FuncCallExpr) bool {
	id, ok := c.Func.(*ast.IdentExpr)
	if !ok || fn == nil {
		return false
	}
	seen := map[string]bool{}
	var walk func([]ast.Stmt)
	walk = func(body []ast.Stmt) {
		for _, s := range body {
			switch x := s.(type) {
			case *ast.LocalAssignStmt:
				for i, n := range x.Names {
					if n == id.Value && i < len(x.Exprs) {
						if v, ok := x.Exprs[i].(*ast.IdentExpr); ok {
							seen[v.Value] = true
						}
					}
				}
			case *ast.AssignStmt:
				for i, left := range x.Lhs {
					if v, ok := left.(*ast.IdentExpr); ok && v.Value == id.Value && i < len(x.Rhs) {
						if r, ok := x.Rhs[i].(*ast.IdentExpr); ok {
							seen[r.Value] = true
						}
					}
				}
			case *ast.IfStmt:
				walk(x.Then)
				walk(x.Else)
			}
		}
	}
	walk(fn.Stmts)
	return len(seen) == 2
}
func functionEntryValue(fn *ast.FunctionExpr) string {
	params := "{'exact': True, 'count': 0}"
	if fn.ParList != nil {
		if fn.ParList.HasVargs {
			params = fmt.Sprintf("{'exact': False, 'prefix': %d, 'open_tail': True}", len(fn.ParList.Names))
		} else {
			params = fmt.Sprintf("{'exact': True, 'count': %d}", len(fn.ParList.Names))
		}
	}
	present := "['normal']"
	if functionCallsNamed(fn.Stmts, "error") {
		present = "['normal', 'throw']"
	}
	results := functionResults(fn)
	return fmt.Sprintf("params=%s completions={'known': ['normal', 'throw', 'user_suspend', 'system_suspend'], 'present': %s} results=%s", params, present, results)
}
func functionResults(fn *ast.FunctionExpr) string {
	var returns []*ast.ReturnStmt
	var walk func([]ast.Stmt)
	walk = func(body []ast.Stmt) {
		for _, s := range body {
			switch x := s.(type) {
			case *ast.ReturnStmt:
				returns = append(returns, x)
			case *ast.IfStmt:
				walk(x.Then)
				walk(x.Else)
			case *ast.DoBlockStmt:
				walk(x.Stmts)
			}
		}
	}
	walk(fn.Stmts)
	if len(returns) == 0 {
		return "{'exact': True, 'count': 0}"
	}
	for _, r := range returns {
		if len(r.Exprs) == 1 {
			switch r.Exprs[0].(type) {
			case *ast.Comma3Expr, *ast.FuncCallExpr:
				return "{'exact': False, 'prefix': 0, 'open_tail': True}"
			}
		}
	}
	return fmt.Sprintf("{'exact': True, 'count': %d}", len(returns[0].Exprs))
}
func functionCallsNamed(body []ast.Stmt, name string) bool {
	found := false
	var expr func(ast.Expr)
	var stmts func([]ast.Stmt)
	expr = func(e ast.Expr) {
		if c, ok := e.(*ast.FuncCallExpr); ok {
			if callTargetName(c) == name {
				found = true
			}
			expr(c.Func)
			for _, a := range c.Args {
				expr(a)
			}
		}
	}
	stmts = func(xs []ast.Stmt) {
		for _, s := range xs {
			switch x := s.(type) {
			case *ast.FuncCallStmt:
				expr(x.Expr)
			case *ast.ReturnStmt:
				for _, e := range x.Exprs {
					expr(e)
				}
			case *ast.IfStmt:
				stmts(x.Then)
				stmts(x.Else)
			}
		}
	}
	stmts(body)
	return found
}
func typeName(t ast.TypeExpr) string {
	if p, ok := t.(*ast.PrimitiveTypeExpr); ok {
		return p.Name
	}
	return "unknown"
}
func containsName(xs []string, x string) bool {
	for _, s := range xs {
		if s == x {
			return true
		}
	}
	return false
}
func nativeSCCs(adj map[string]map[string]bool) [][]string {
	var out [][]string
	index := 0
	indices, low := map[string]int{}, map[string]int{}
	stack := []string{}
	on := map[string]bool{}
	var visit func(string)
	visit = func(v string) {
		index++
		indices[v], low[v] = index, index
		stack = append(stack, v)
		on[v] = true
		for w := range adj[v] {
			if indices[w] == 0 {
				visit(w)
				if low[w] < low[v] {
					low[v] = low[w]
				}
			} else if on[w] && indices[w] < low[v] {
				low[v] = indices[w]
			}
		}
		if low[v] == indices[v] {
			var c []string
			for {
				n := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				on[n] = false
				c = append(c, n)
				if n == v {
					break
				}
			}
			sort.Strings(c)
			out = append(out, c)
		}
	}
	keys := make([]string, 0, len(adj))
	for k := range adj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if indices[k] == 0 {
			visit(k)
		}
	}
	return out
}
