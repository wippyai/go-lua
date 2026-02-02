package decl

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	basecfg "github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// ExtractTypeKeys collects all type keys from scope into inputs.TypeKeys.
func ExtractTypeKeys(fc *core.FlowContext, inputs *flow.Inputs) {
	if fc.Base == nil {
		return
	}
	for s := fc.Base; s != nil; s = s.Parent() {
		s.RangeTypes(func(_ string, t typ.Type) bool {
			AddTypeKey(inputs, t)
			return true
		})
	}
}

// AddTypeKey registers a type in the TypeKeys map.
func AddTypeKey(inputs *flow.Inputs, t typ.Type) {
	if inputs == nil || t == nil {
		return
	}
	switch v := t.(type) {
	case *typ.Alias:
		if v.Target == nil {
			return
		}
		h := v.Hash()
		if h != 0 {
			inputs.TypeKeys[h] = v.Target
		}
		AddTypeKey(inputs, v.Target)
		return
	case *typ.Meta:
		if v.Of != nil {
			h := v.Hash()
			if h != 0 {
				inputs.TypeKeys[h] = v.Of
			}
		}
		AddTypeKey(inputs, v.Of)
		return
	}
	h := t.Hash()
	if h != 0 {
		if _, ok := inputs.TypeKeys[h]; !ok {
			inputs.TypeKeys[h] = t
		}
	}
}

// ExtractDeclaredTypes collects declared types from CFG annotations.
// Collects:
// 1. Inherited symbols from globals map (globals, stdlib)
// 2. Explicitly annotated local variables (tracked in AnnotatedVars)
// 3. Local function definitions with explicit return type annotations only
// Non-annotated variables are NOT stored here; their types come from Assignments + flow solver.
func ExtractDeclaredTypes(fc *core.FlowContext, inputs *flow.Inputs) {
	if fc.Graph == nil {
		return
	}

	entry := fc.Graph.Entry()

	bindings := fc.Graph.Bindings()
	if fc.CheckCtx != nil && fc.CheckCtx.Bindings() != nil {
		bindings = fc.CheckCtx.Bindings()
	}
	globalNames := make([]string, 0, len(fc.Globals))
	for name := range fc.Globals {
		globalNames = append(globalNames, name)
	}
	sort.Strings(globalNames)
	for _, name := range globalNames {
		t := fc.Globals[name]
		if t == nil {
			continue
		}
		sym, ok := fc.Graph.SymbolAt(entry, name)
		if !ok {
			continue
		}
		if bindings != nil {
			kind, ok := bindings.Kind(sym)
			if !ok || kind != basecfg.SymbolGlobal {
				continue
			}
		}
		inputs.DeclaredTypes[sym] = resolve.Ref(t, fc.Base)
	}

	if fc.Services != nil {
		fc.Graph.EachFuncDef(func(p cfg.Point, info *cfg.FuncDefInfo) {
			if info == nil || info.Name == "" || info.FuncExpr == nil {
				return
			}
			if info.TargetKind != cfg.FuncDefGlobal {
				return
			}
			sym := info.Symbol
			if sym == 0 {
				return
			}
			sc := fc.Scopes[p]
			fnType := fc.Services.ResolveFunctionSignature(info.FuncExpr, sc)
			if fnType != nil && len(fnType.Returns) > 0 {
				inputs.DeclaredTypes[sym] = fnType
			}
		})
	}

	fc.Graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil || !info.IsLocal {
			return
		}
		sc := fc.Scopes[p]

		for i, target := range info.Targets {
			if target.Kind != cfg.TargetIdent || target.Name == "" {
				continue
			}

			sym := target.Symbol
			hasAnnotation := i < len(info.TypeAnnotations) && info.TypeAnnotations[i] != nil

			if hasAnnotation {
				if inputs.AnnotatedVars == nil {
					inputs.AnnotatedVars = make(map[cfg.SymbolID]bool)
				}
				annotate := false
				var annType typ.Type
				if fc.Services != nil {
					annType = fc.Services.ResolveTypeExpr(info.TypeAnnotations[i], sc)
					if annType != nil {
						resolved := resolve.Ref(annType, sc)
						if typ.IsSoft(annType, typ.SoftAnnotationPolicy) {
							if existing := inputs.DeclaredTypes[sym]; existing == nil || typ.IsSoft(existing, typ.SoftAnnotationPolicy) {
								inputs.DeclaredTypes[sym] = resolved
							}
						} else {
							inputs.DeclaredTypes[sym] = resolved
							annotate = true
						}
					}
				} else if fc.CheckCtx != nil && fc.CheckCtx.Types() != nil {
					tv := fc.CheckCtx.Types().DeclaredAt(p, sym)
					if tv.State == flow.StateResolved && tv.Type != nil {
						resolved := resolve.Ref(tv.Type, sc)
						if typ.IsSoft(tv.Type, typ.SoftAnnotationPolicy) {
							if existing := inputs.DeclaredTypes[sym]; existing == nil || typ.IsSoft(existing, typ.SoftAnnotationPolicy) {
								inputs.DeclaredTypes[sym] = resolved
							}
						} else {
							inputs.DeclaredTypes[sym] = resolved
							annotate = true
						}
					}
				}
				if annotate {
					inputs.AnnotatedVars[sym] = true
				}
			} else if fc.Services != nil && i < len(info.Sources) {
				if fnExpr, ok := info.Sources[i].(*ast.FunctionExpr); ok {
					fnType := fc.Services.ResolveFunctionSignature(fnExpr, sc)
					if fnType != nil && len(fnType.Returns) > 0 {
						if inputs.AnnotatedVars == nil {
							inputs.AnnotatedVars = make(map[cfg.SymbolID]bool)
						}
						inputs.AnnotatedVars[sym] = true
						inputs.DeclaredTypes[sym] = fnType
					}
				}
			}
		}
	})
}

// ExtractModuleAliases collects symbol -> module path mappings from require() assignments.
// Detects patterns like: local x = require("mod") or x = require("mod")
func ExtractModuleAliases(fc *core.FlowContext, inputs *flow.Inputs) {
	if fc.Graph == nil || inputs == nil {
		return
	}

	fc.Graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil {
			return
		}

		for i, target := range info.Targets {
			if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
				continue
			}
			if i >= len(info.Sources) {
				continue
			}
			source := info.Sources[i]
			if source == nil {
				continue
			}

			call, ok := source.(*ast.FuncCallExpr)
			if !ok || call.Method != "" || call.Receiver != nil {
				continue
			}
			ident, ok := call.Func.(*ast.IdentExpr)
			if !ok || ident.Value != "require" {
				continue
			}
			if len(call.Args) != 1 {
				continue
			}
			strArg, ok := call.Args[0].(*ast.StringExpr)
			if !ok {
				continue
			}

			inputs.ModuleAliases[target.Symbol] = strArg.Value
		}
	})
}
