package cfg

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	basecfg "github.com/wippyai/go-lua/types/cfg"
)

// TableMutation describes how the code of a set of graphs can change the
// fields of one table.
type TableMutation struct {
	// Writes lists the definitions of each static-key field: the fields of a
	// table constructor initializing an alias, field writes through aliases,
	// and functions defined into the table.
	Writes map[string][]FieldWriteSite
	// Dynamic reports a write through an alias whose key may name any field.
	Dynamic bool
	// Reassigned reports an alias that receives another value after its
	// declaration.
	Reassigned bool
	// Escaped reports that a reference to the table leaves the tracked
	// aliases: a call argument or receiver, a return other than the export,
	// or a store into any other location.
	Escaped bool
	// Captured reports that a function nested in the analyzed graphs holds an
	// alias as an upvalue.
	Captured bool
	// Decl is the graph that declares the table's symbol as a local or a
	// parameter; nil when no analyzed graph declares it.
	Decl *Graph
}

// FieldWriteSite is one static-key field write.
type FieldWriteSite struct {
	Graph *Graph
	Point Point
}

// FieldStable reports whether field holds the value of its one definition for
// as long as the table lives: the table neither escapes, nor takes
// dynamic-key writes, nor is reassigned, and the field has at most one
// definition, made in the declaring graph.
func (m TableMutation) FieldStable(field string) bool {
	if m.Escaped || m.Dynamic || m.Reassigned {
		return false
	}
	sites := m.Writes[field]
	return len(sites) == 0 || len(sites) == 1 && m.Decl != nil && sites[0].Graph == m.Decl
}

// FieldWritten reports whether any tracked write can change field.
func (m TableMutation) FieldWritten(field string) bool {
	return m.Dynamic || len(m.Writes[field]) > 0
}

// AnalyzeTableMutation scans graphs for the writes and escapes of the table
// held by sym. The aliases of the table are sym, every stable local alias
// chain leading to it, and the self parameter of every function defined into
// it. Returns of export publish the table without letting it escape.
func AnalyzeTableMutation(graphs []*Graph, sym basecfg.SymbolID, export *Graph) TableMutation {
	out := TableMutation{Writes: make(map[string][]FieldWriteSite)}
	if sym == 0 || len(graphs) == 0 {
		return out
	}
	aliases := tableAliases(graphs, sym)
	for _, graph := range graphs {
		bindings := graph.Bindings()
		if bindings == nil {
			continue
		}
		for _, param := range graph.ParamSymbols() {
			if param == sym {
				out.Decl = graph
			}
		}
		graph.EachAssign(func(point Point, info *AssignInfo) {
			if info == nil {
				return
			}
			info.EachTargetSource(func(_ int, target AssignTarget, source ast.Expr) {
				switch target.Kind {
				case TargetIdent:
					if !aliases[target.Symbol] {
						break
					}
					if info.IsLocal && target.Symbol == sym {
						out.Decl = graph
					}
					if !info.IsLocal {
						out.Reassigned = true
					} else if tbl, ok := source.(*ast.TableExpr); ok {
						for _, field := range tbl.Fields {
							if field == nil {
								continue
							}
							if key, ok := field.Key.(*ast.StringExpr); ok {
								out.Writes[key.Value] = append(out.Writes[key.Value], FieldWriteSite{Graph: graph, Point: point})
							}
						}
					}
				case TargetField:
					if aliases[target.BaseSymbol] && len(target.FieldPath) > 0 {
						out.Writes[target.FieldPath[0]] = append(out.Writes[target.FieldPath[0]], FieldWriteSite{Graph: graph, Point: point})
					}
				case TargetIndex:
					if aliases[target.BaseSymbol] {
						if key, ok := target.Key.(*ast.StringExpr); ok {
							out.Writes[key.Value] = append(out.Writes[key.Value], FieldWriteSite{Graph: graph, Point: point})
						} else {
							out.Dynamic = true
						}
					}
				}
				if source == nil || !ExprHoldsSymbol(source, bindings, aliases) {
					return
				}
				if target.Kind != TargetIdent || !aliases[target.Symbol] || graph.DirectAliasSymbol(target.Symbol) == 0 {
					out.Escaped = true
				}
			})
		})
		graph.EachFuncDef(func(point Point, info *FuncDefInfo) {
			if info == nil || !aliases[info.TargetPath.Symbol] || len(info.TargetPath.Segments) == 0 {
				return
			}
			name := info.TargetPath.Segments[0].Name
			out.Writes[name] = append(out.Writes[name], FieldWriteSite{Graph: graph, Point: point})
		})
		graph.EachCallSite(func(_ Point, call *CallInfo) {
			if call == nil {
				return
			}
			if ExprHoldsSymbol(call.Receiver, bindings, aliases) {
				out.Escaped = true
			}
			for _, arg := range call.Args {
				if ExprHoldsSymbol(arg, bindings, aliases) {
					out.Escaped = true
				}
			}
		})
		if graph != export {
			graph.EachReturn(func(_ Point, ret *ReturnInfo) {
				if ret == nil {
					return
				}
				for _, expr := range ret.Exprs {
					if ExprHoldsSymbol(expr, bindings, aliases) {
						out.Escaped = true
					}
				}
			})
		}
		for _, nested := range graph.NestedFunctions() {
			if nested.Func == nil {
				continue
			}
			for _, captured := range bindings.CapturedSymbols(nested.Func) {
				if aliases[captured] {
					out.Captured = true
				}
			}
		}
	}
	return out
}

// tableAliases closes {sym} over stable local aliases and the self
// parameters of functions defined into an alias, across all graphs.
func tableAliases(graphs []*Graph, sym basecfg.SymbolID) map[basecfg.SymbolID]bool {
	aliases := map[basecfg.SymbolID]bool{sym: true}
	byFunc := make(map[*ast.FunctionExpr]*Graph, len(graphs))
	for _, graph := range graphs {
		if fn := graph.Func(); fn != nil {
			byFunc[fn] = graph
		}
	}
	addSelf := func(fn *ast.FunctionExpr, changed *bool) {
		graph := byFunc[fn]
		if graph == nil {
			return
		}
		names, syms := graph.ParamNames(), graph.ParamSymbols()
		if len(names) == 0 || len(syms) == 0 || names[0] != "self" || syms[0] == 0 || aliases[syms[0]] {
			return
		}
		aliases[syms[0]] = true
		*changed = true
	}
	for changed := true; changed; {
		changed = false
		for _, graph := range graphs {
			graph.EachSymbolID(func(candidate basecfg.SymbolID) bool {
				if !aliases[candidate] && aliases[graph.DirectAliasSymbol(candidate)] {
					aliases[candidate] = true
					changed = true
				}
				return false
			})
			graph.EachFuncDef(func(_ Point, info *FuncDefInfo) {
				if info != nil && aliases[info.TargetPath.Symbol] && len(info.TargetPath.Segments) > 0 {
					addSelf(info.FuncExpr, &changed)
				}
			})
			graph.EachAssign(func(_ Point, info *AssignInfo) {
				if info == nil {
					return
				}
				info.EachTargetSource(func(_ int, target AssignTarget, source ast.Expr) {
					if fn, ok := source.(*ast.FunctionExpr); ok && (target.Kind == TargetField || target.Kind == TargetIndex) && aliases[target.BaseSymbol] {
						addSelf(fn, &changed)
					}
					if tbl, ok := source.(*ast.TableExpr); ok && target.Kind == TargetIdent && aliases[target.Symbol] {
						for _, field := range tbl.Fields {
							if field == nil {
								continue
							}
							if fn, ok := field.Value.(*ast.FunctionExpr); ok {
								addSelf(fn, &changed)
							}
						}
					}
				})
			})
		}
	}
	return aliases
}

// ExprHoldsSymbol reports whether evaluating expr can yield, or build a table
// holding, the value of a symbol in syms.
func ExprHoldsSymbol(expr ast.Expr, bindings *bind.BindingTable, syms map[basecfg.SymbolID]bool) bool {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		sym, ok := bindings.SymbolOf(e)
		return ok && syms[sym]
	case *ast.TableExpr:
		for _, f := range e.Fields {
			if f != nil && (ExprHoldsSymbol(f.Key, bindings, syms) || ExprHoldsSymbol(f.Value, bindings, syms)) {
				return true
			}
		}
	case *ast.LogicalOpExpr:
		return ExprHoldsSymbol(e.Lhs, bindings, syms) || ExprHoldsSymbol(e.Rhs, bindings, syms)
	case *ast.CastExpr:
		return ExprHoldsSymbol(e.Expr, bindings, syms)
	case *ast.NonNilAssertExpr:
		return ExprHoldsSymbol(e.Expr, bindings, syms)
	}
	return false
}
