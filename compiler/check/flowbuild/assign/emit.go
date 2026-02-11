// Package assign implements assignment constraint extraction for the flow system.
// It processes assignment statements in the CFG and emits type constraints that
// the flow solver uses to track variable types through the program.
//
// # EXTRACTION PIPELINE
//
// The package extracts three types of assignment information:
//
//  1. Variable Assignments: Local declarations and reassignments that establish
//     or update the type of a symbol at a CFG point.
//
//  2. Field Assignments: Table field writes (t.foo = v) that contribute to
//     record type inference for table values.
//
//  3. Function Definitions: Named function definitions (function M.foo()) that
//     add fields to module tables.
//
// # SPEC NARROWING
//
// The package implements spec-based type narrowing where contract specifications
// on function parameters constrain the types of expressions passed to methods.
// For example, if a function is annotated with @spec, the spec types are used
// to narrow parameter types at call sites.
//
// # INFERRED TYPES
//
// For unannotated variables, types are inferred from their initialization
// expressions using the synthesis engine. These inferred types are tracked
// separately from annotated types to support different narrowing behaviors.
package assign

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/cond"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/constprop"
	fbcore "github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/guard"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/keyscoll"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/mutator"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/path"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/predicate"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/tblutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// ExtractAssignments extracts assignment info from graph.
func ExtractAssignments(fc *fbcore.FlowContext, inputs *flow.Inputs, keysCollector KeysCollectorFunc) {
	if fc == nil || fc.Graph == nil {
		return
	}
	if inputs == nil {
		return
	}
	derived := fc.Derived
	if derived == nil {
		derived = &fbcore.Derived{}
	}
	synth := derived.Synth
	if synth == nil {
		synth = func(ast.Expr, cfg.Point) typ.Type {
			return typ.Unknown
		}
	}
	symResolver := derived.SymResolver
	if symResolver == nil {
		symResolver = func(cfg.Point, cfg.SymbolID) (typ.Type, bool) {
			return nil, false
		}
	}
	// Worklist fixpoint for spec narrowing:
	// Collects spec-narrowed types from contract specs and propagates through method calls.
	// Uses expandValues with SpecTypes overlay for method call synthesis.
	specNarrowed := CollectSpecNarrowedTypes(fc.Graph, fc.Scopes, synth, symResolver, fc.API)
	inferredTypes := collectInferredTypes(fc.Graph, fc.Scopes, synth, fc.API, symResolver, specNarrowed, inputs.AnnotatedVars, inputs, fc.CallCtx, fc.TypeOps, fc.Services)
	// Promote inferred parameter types into DeclaredTypes for unannotated params.
	// This enables bidirectional inference at call sites (e.g., custom assert helpers).
	if inputs.DeclaredTypes != nil {
		for _, sym := range fc.Graph.ParamSymbols() {
			if sym == 0 {
				continue
			}
			if inputs.AnnotatedVars != nil && inputs.AnnotatedVars[sym] {
				continue
			}
			inferred := inferredTypes[sym]
			if typ.IsAbsentOrUnknown(inferred) {
				continue
			}
			current := inputs.DeclaredTypes[sym]
			if current == nil || current.Kind().IsPlaceholder() {
				inputs.DeclaredTypes[sym] = inferred
			}
		}
	}

	bindings := fc.Graph.Bindings()

	// Precompute loop variable types for all for loops to improve RHS synthesis.
	// This includes both numeric for loops (integer index) and generic for loops (iterator variables).
	loopVarTypes := make(map[cfg.SymbolID]typ.Type)
	fc.Graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil || len(info.Targets) == 0 {
			return
		}
		// Handle numeric for loops
		if info.NumericFor != nil {
			target, ok := info.FirstTarget()
			if !ok {
				return
			}
			if target.Kind != cfg.TargetIdent || target.Name == "" || target.Symbol == 0 {
				return
			}
			loopVarTypes[target.Symbol] = typ.Integer
			return
		}
		// Handle generic for loops
		if len(info.IterExprs) > 0 {
			var varTypes []typ.Type
			if fc.API != nil {
				varTypes = fc.API.InferIterVarsWithSpecTypes(info.IterExprs, len(info.Targets), p, nil)
			}
			info.EachTargetSource(func(i int, target cfg.AssignTarget, _ ast.Expr) {
				if target.Kind != cfg.TargetIdent || target.Name == "" || target.Symbol == 0 {
					return
				}
				if i < len(varTypes) && varTypes[i] != nil {
					loopVarTypes[target.Symbol] = varTypes[i]
				}
			})
		}
	})
	overlayTypes := MergeSpecTypes(inputs.DeclaredTypes, inferredTypes)
	overlayTypes = MergeSpecTypes(overlayTypes, specNarrowed)
	overlayTypes = MergeSpecTypes(overlayTypes, loopVarTypes)

	// Precompute truthy guards: map from CFG point to paths that are narrowed (non-nil) at that point.
	// Used during table literal synthesis to unwrap optional types.
	truthyGuards := guard.CollectTruthyGuards(fc.Graph, bindings)

	baseSynth := resolve.SynthWithOverlay(overlayTypes, bindings, synth)
	var wrappedSynth func(ast.Expr, cfg.Point) typ.Type
	wrappedSynth = func(expr ast.Expr, p cfg.Point) typ.Type {
		if table, ok := expr.(*ast.TableExpr); ok && !tblutil.TableHasFunctionField(table) {
			if t := tblutil.SynthTableLiteralWithWrapper(table, p, wrappedSynth); t != nil {
				return t
			}
		}
		// Check if this is an attribute access where the full path has a truthy guard.
		if attr, ok := expr.(*ast.AttrGetExpr); ok {
			t := synth(expr, p)
			if t != nil && bindings != nil {
				if sym, fieldPath, ok := callsite.FieldPathWithBaseSymbol(bindings, attr); ok && sym != 0 && fieldPath != "" {
					pathKey := guard.TruthyPathKey{Symbol: sym, Field: fieldPath}
					if guards, ok := truthyGuards[p]; ok {
						if guards[pathKey] {
							if opt, ok := t.(*typ.Optional); ok {
								return opt.Inner
							}
						}
					}
				}
			}
			return t
		}
		return baseSynth(expr, p)
	}

	// Build a resolver that includes spec-narrowed types
	resolverWithSpec := func(p cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		if t, ok := loopVarTypes[sym]; ok {
			return t, true
		}
		if t, ok := specNarrowed[sym]; ok {
			return t, true
		}
		return symResolver(p, sym)
	}

	fc.Graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		sc := fc.Scopes[p]

		// Handle numeric for loops
		if info.NumericFor != nil {
			target, ok := info.FirstTarget()
			if !ok {
				return
			}
			if target.Kind != cfg.TargetIdent || target.Name == "" {
				return
			}
			inputs.Assignments = append(inputs.Assignments, flow.UnifiedAssignment{
				Point:      p,
				TargetPath: constraint.Path{Root: resolve.RootName(fc.Graph, target.Symbol, target.Name), Symbol: target.Symbol},
				Type:       typ.Integer,
			})
			return
		}

		// Handle generic for loops
		if len(info.IterExprs) > 0 && len(info.Targets) > 0 {
			var varTypes []typ.Type
			if fc.API != nil {
				varTypes = fc.API.InferIterVarsWithSpecTypes(info.IterExprs, len(info.Targets), p, overlayTypes)
			}
			// Build const resolver for iterator source extraction
			constResolver := predicate.BuildConstResolver(inputs, p)
			iterSource := resolve.ExtractIteratorSource(info.IterExprs, p, wrappedSynth, resolverWithSpec, constResolver, bindings)
			info.EachTargetSource(func(i int, target cfg.AssignTarget, _ ast.Expr) {
				if target.Kind != cfg.TargetIdent || target.Name == "" {
					return
				}
				sym := target.Symbol
				varType := typ.Unknown
				if i < len(varTypes) && varTypes[i] != nil {
					varType = varTypes[i]
				}
				assignment := flow.UnifiedAssignment{
					Point:      p,
					TargetPath: constraint.Path{Root: resolve.RootName(fc.Graph, sym, target.Name), Symbol: sym},
					Type:       resolve.Ref(varType, sc),
				}
				if iterSource != nil {
					assignment.IterSource = &flow.IteratorSource{
						Path:     iterSource.Path,
						Kind:     iterSource.Kind,
						VarIndex: i,
					}
				}
				inputs.Assignments = append(inputs.Assignments, assignment)
			})
			return
		}

		// Build const resolver for this point
		constResolver := predicate.BuildConstResolver(inputs, p)

		// Get expanded values if we have multiple targets
		// Spec-narrowed types are passed as overlay facts instead of mutating scope.
		values := expandedAssignValues(fc.API, info, p, overlayTypes)

		for i, target := range info.Targets {
			source := info.SourceAt(i)

			// Handle identifier targets
			if target.Kind == cfg.TargetIdent {
				name := target.Name
				if name == "" {
					continue
				}

				sym := target.Symbol

				// Determine assigned type using identity-based resolver.
				// For annotated locals, keep the declared type (RHS should not override).
				assignedType := typ.Unknown
				if info.IsLocal {
					if inputs != nil && inputs.AnnotatedVars != nil && inputs.AnnotatedVars[sym] {
						if dt, ok := inputs.DeclaredTypes[sym]; ok && dt != nil {
							assignedType = dt
						}
					} else {
						if t, ok := resolverWithSpec(p, sym); ok && t != nil {
							assignedType = t
						}
					}
				}
				// Fall back to expression synthesis if no declared/known type
				if typ.IsAbsentOrUnknown(assignedType) {
					if value := assignValueAt(values, i); value != nil {
						assignedType = value
					} else if wrappedSynth != nil && source != nil {
						assignedType = wrappedSynth(source, p)
					}
				}
				// Override with expanded values if source call has a spec-narrowed receiver.
				// This handles cases like ch:receive() where ch was narrowed by spec.
				// Propagate the result to specNarrowed for subsequent method calls (e.g., msg:from()).
				if call, _ := info.CallForTarget(i); call != nil {
					if call.Receiver != nil {
						if recvSym := call.ReceiverSymbol; recvSym != 0 {
							if value := assignValueAt(values, i); value != nil {
								if _, narrowed := specNarrowed[recvSym]; narrowed {
									assignedType = value
									// Propagate to specNarrowed for method calls on this variable
									if !typ.IsAbsentOrUnknown(assignedType) {
										specNarrowed[sym] = assignedType
									}
								}
							}
						}
					}
				}
				if assignedType == nil {
					assignedType = typ.Unknown
				}

				// Use pre-collected spec-narrowed type if available (via SymbolID)
				if narrowed, ok := specNarrowed[sym]; ok {
					assignedType = narrowed
				}

				// Build source path with const resolution and bindings
				var sourcePath constraint.Path
				if source != nil {
					if sp := path.FromExprWithBindings(source, constResolver, bindings); !sp.IsEmpty() {
						sourcePath = constraint.Path{
							Root:     resolve.RootNameFromBindings(bindings, sp.Symbol, sp.Root),
							Symbol:   sp.Symbol,
							Segments: sp.Segments,
						}
					}
				}

				// Extract predicate link if RHS is a predicate call.
				if callInfo, retIndex := info.CallForTarget(i); callInfo != nil {
					if link := cond.ExtractPredicateLinkFromCallInfo(callInfo, retIndex, p, sc, inputs, derived.TypeKeyRes, wrappedSynth, derived.EffectBySym, symResolver, fc.Graph); link != nil {
						if retIndex == 1 && callInfo.IsTypeCheck && callInfo.Method == "is" && callInfo.Receiver != nil && derived.TypeKeyRes != nil {
							if typeKey, ok := derived.TypeKeyRes(callInfo.TypeCheckName, sc); ok && !typeKey.IsZero() {
								valuePath := constraint.Path{}
								valIndex := i - retIndex
								if valIndex >= 0 && valIndex < len(info.Targets) {
									valTarget := info.Targets[valIndex]
									if valTarget.Kind == cfg.TargetIdent && valTarget.Name != "" && valTarget.Symbol != 0 {
										valuePath = constraint.Path{
											Root:   resolve.RootName(fc.Graph, valTarget.Symbol, valTarget.Name),
											Symbol: valTarget.Symbol,
										}
									}
								}
								if !valuePath.IsEmpty() {
									link.OnFalsy = constraint.And(link.OnFalsy, constraint.FromConstraints(constraint.HasType{Path: valuePath, Type: typeKey}))
									link.OnTruthy = constraint.And(link.OnTruthy, constraint.FromConstraints(constraint.NotHasType{Path: valuePath, Type: typeKey}))
									link.OnTruthy = constraint.And(link.OnTruthy, constraint.FromConstraints(constraint.IsNil{Path: valuePath}))
									var checkType typ.Type
									if sc != nil {
										if resolved, ok := sc.LookupType(callInfo.TypeCheckName); ok && resolved != nil {
											checkType = resolve.Ref(resolved, sc)
										}
									}
									if checkType != nil {
										baseType := unwrap.Alias(checkType)
										if baseType != nil && !baseType.Kind().IsPlaceholder() && !unwrap.IsOptionalLike(baseType) {
											link.OnFalsy = constraint.And(link.OnFalsy, constraint.FromConstraints(constraint.NotNil{Path: valuePath}))
										}
									}
								}
							}
						}
						varKey := predicate.LinkKey(name, p)
						inputs.PredicateLinks[varKey] = *link
					}
				}

				// Detect keys collector calls: local keys = sorted_keys(table)
				if call, retIndex := info.CallForTarget(i); call != nil {
					var tableSym cfg.SymbolID

					// Try local function analysis first
					if retIndex == 0 && keysCollector != nil {
						tableSym = keysCollector(call, p)
					}

					// Fallback: resolve function literal via module bindings (by symbol or name)
					if tableSym == 0 && retIndex == 0 && fc.ModuleBindings != nil {
						if call.CalleeSymbol != 0 {
							if fn, ok := fc.ModuleBindings.FuncLitBySymbol(call.CalleeSymbol); ok && fn != nil {
								if info := keyscoll.DetectKeysCollector(fn); info != nil {
									tableSym = callsite.RuntimeArgSymbolAt(call, info.ParamIndex, bindings)
								}
							}
						}
						if tableSym == 0 && call.CalleeName != "" {
							for _, sym := range fc.ModuleBindings.AllSymbols() {
								if fc.ModuleBindings.Name(sym) != call.CalleeName {
									continue
								}
								fn, ok := fc.ModuleBindings.FuncLitBySymbol(sym)
								if !ok || fn == nil {
									continue
								}
								if info := keyscoll.DetectKeysCollector(fn); info != nil {
									tableSym = callsite.RuntimeArgSymbolAt(call, info.ParamIndex, bindings)
									break
								}
							}
						}
					}

					// Fallback: check function effect for KeyOf-based keys collector
					if tableSym == 0 && derived.EffectBySym != nil {
						var eff *constraint.FunctionEffect
						if call.CalleeSymbol != 0 {
							eff = derived.EffectBySym(call.CalleeSymbol)
						}
						if eff == nil && fc.ModuleBindings != nil && call.CalleeName != "" {
							for _, sym := range fc.ModuleBindings.AllSymbols() {
								if fc.ModuleBindings.Name(sym) != call.CalleeName {
									continue
								}
								if eff = derived.EffectBySym(sym); eff != nil {
									break
								}
							}
						}
						if eff != nil {
							if paramIdx, keyReturnIdx, ok := eff.KeysCollectorInfo(); ok && retIndex == keyReturnIdx {
								tableSym = callsite.RuntimeArgSymbolAt(call, paramIdx, bindings)
							}
						}
					}

					if tableSym != 0 {
						if inputs.KeysProvenance == nil {
							inputs.KeysProvenance = make(map[cfg.SymbolID]cfg.SymbolID)
						}
						inputs.KeysProvenance[sym] = tableSym
					}
				}

				// Check for container element return effects (e.g., channel:receive())
				var containerElemSrc *flow.ContainerElementSource
				if call, retIndex := info.CallForTarget(i); call != nil {
					assignmentTypesResolver := resolve.BuildAssignmentTypeResolver(inputs)
					if elemInfo := mutator.ContainerElementReturnFromCall(call, p, wrappedSynth, resolverWithSpec, assignmentTypesResolver); elemInfo != nil {
						// Check if this return index matches
						if elemInfo.ReturnIndex == retIndex {
							// For method calls, index 0 is self (receiver)
							if callsite.IsMethodCallInfo(call) && elemInfo.SourceRef.Index == 0 {
								if recvPath := path.FromExprWithBindings(call.Receiver, constResolver, bindings); !recvPath.IsEmpty() && recvPath.Symbol != 0 {
									containerElemSrc = &flow.ContainerElementSource{
										ContainerPath: constraint.Path{
											Root:     resolve.RootNameFromBindings(bindings, recvPath.Symbol, recvPath.Root),
											Symbol:   recvPath.Symbol,
											Segments: recvPath.Segments,
										},
										ReturnIndex: elemInfo.ReturnIndex,
									}
								}
							}
						}
					}
				}

				inputs.Assignments = append(inputs.Assignments, flow.UnifiedAssignment{
					Point:                  p,
					TargetPath:             constraint.Path{Root: resolve.RootName(fc.Graph, sym, name), Symbol: sym},
					SourcePath:             sourcePath,
					Type:                   resolve.Ref(assignedType, sc),
					ContainerElementSource: containerElemSrc,
				})

				// Emit per-field assignments for table literals to enable flow narrowing
				if source != nil {
					if tbl, ok := source.(*ast.TableExpr); ok && !tblutil.TableHasFunctionField(tbl) {
						EmitTableLiteralFieldAssignments(tbl, sym, resolve.RootName(fc.Graph, sym, name), p, bindings, constResolver, wrappedSynth, sc, inputs)
					}
				}
				continue
			}

			// Handle field targets: t.a or t["a"] where "a" is a valid identifier
			if target.Kind == cfg.TargetField && target.BaseName != "" && len(target.FieldPath) > 0 {
				sym := target.BaseSymbol

				// Determine assigned type
				assignedType := typ.Unknown
				// First check expanded values for multi-return assignments
				if value := assignValueAt(values, i); !typ.IsAbsentOrUnknown(value) {
					assignedType = value
				} else if source != nil {
					if tbl, ok := source.(*ast.TableExpr); ok && wrappedSynth != nil && !tblutil.TableHasFunctionField(tbl) {
						assignedType = wrappedSynth(source, p)
					} else if wrappedSynth != nil {
						assignedType = wrappedSynth(source, p)
					}
				}
				if assignedType == nil {
					assignedType = typ.Unknown
				}

				// Build segments from field path
				segments := make([]constraint.Segment, len(target.FieldPath))
				for j, field := range target.FieldPath {
					segments[j] = constraint.Segment{Kind: constraint.SegmentField, Name: field}
				}

				// Create assignment for the field path: root.field1.field2... = assignedType
				inputs.Assignments = append(inputs.Assignments, flow.UnifiedAssignment{
					Point: p,
					TargetPath: constraint.Path{
						Root:     resolve.RootName(fc.Graph, sym, target.BaseName),
						Symbol:   sym,
						Segments: segments,
					},
					Type: resolve.Ref(assignedType, sc),
				})
				continue
			}

			// Handle index targets with string literal or const keys: t["key"] = v
			if target.Kind == cfg.TargetIndex {
				var basePath constraint.Path
				if target.BaseName != "" {
					sym := target.BaseSymbol
					basePath = constraint.Path{
						Root:   resolve.RootNameFromBindings(bindings, sym, target.BaseName),
						Symbol: sym,
					}
				} else if target.Base != nil {
					if bp := path.FromExprWithBindings(target.Base, constResolver, bindings); !bp.IsEmpty() && bp.Symbol != 0 {
						basePath = constraint.Path{
							Root:     resolve.RootNameFromBindings(bindings, bp.Symbol, bp.Root),
							Symbol:   bp.Symbol,
							Segments: bp.Segments,
						}
					}
				}
				if basePath.IsEmpty() {
					continue
				}

				// Determine assigned type
				assignedType := typ.Unknown
				// First check expanded values for multi-return assignments
				if value := assignValueAt(values, i); !typ.IsAbsentOrUnknown(value) {
					assignedType = value
				} else if source != nil {
					if tbl, ok := source.(*ast.TableExpr); ok && wrappedSynth != nil && !tblutil.TableHasFunctionField(tbl) {
						assignedType = wrappedSynth(source, p)
					} else if wrappedSynth != nil {
						assignedType = wrappedSynth(source, p)
					}
				}
				if assignedType == nil {
					assignedType = typ.Unknown
				}

				// Determine the field name from the key
				var fieldName string
				var keyType typ.Type
				switch k := target.Key.(type) {
				case *ast.StringExpr:
					fieldName = k.Value
				case *ast.IdentExpr:
					// Variable key - try const resolution
					if val := constResolver(k.Value); val != nil {
						switch val.Kind {
						case flow.ConstString:
							fieldName = val.Str
						case flow.ConstInt:
							keyType = typ.Integer
						case flow.ConstFloat:
							keyType = typ.Number
						}
					}
				case *ast.NumberExpr:
					// Number literal key - try const resolution
					if val := constprop.ConstValueFromExpr(target.Key); val != nil {
						switch val.Kind {
						case flow.ConstInt:
							keyType = typ.Integer
						case flow.ConstFloat:
							keyType = typ.Number
						}
					}
				}

				// For non-const keys, emit an IndexerAssignment to widen the table
				if fieldName == "" {
					if keyType == nil && target.Key != nil && wrappedSynth != nil {
						keyType = wrappedSynth(target.Key, p)
					}
					// Apply truthy guards to narrow optional fields in table literals.
					valType := assignedType
					if source != nil && bindings != nil && truthyGuards != nil {
						if tbl, ok := source.(*ast.TableExpr); ok {
							valType = guard.NarrowTableFieldsByGuard(valType, tbl, p, bindings, truthyGuards)
						}
					}

					// Extract key variable name and symbol using bindings
					var keyVar string
					var keySym cfg.SymbolID
					if keyIdent, ok := target.Key.(*ast.IdentExpr); ok && bindings != nil {
						keySym, _ = bindings.SymbolOf(keyIdent)
						keyVar = resolve.RootNameFromBindings(bindings, keySym, keyIdent.Value)
					}
					resolved := resolve.Ref(valType, sc)
					inputs.IndexerAssignments = append(inputs.IndexerAssignments, flow.IndexerAssignment{
						Point:     p,
						Root:      basePath.Root,
						Symbol:    basePath.Symbol,
						Segments:  basePath.Segments,
						KeyVar:    keyVar,
						KeySymbol: keySym,
						KeyType:   keyType,
						ValType:   resolved,
					})
					continue
				}

				// Create assignment for the field path: root.fieldName = assignedType
				inputs.Assignments = append(inputs.Assignments, flow.UnifiedAssignment{
					Point: p,
					TargetPath: constraint.Path{
						Root:     basePath.Root,
						Symbol:   basePath.Symbol,
						Segments: append(append([]constraint.Segment{}, basePath.Segments...), constraint.Segment{Kind: constraint.SegmentField, Name: fieldName}),
					},
					Type: resolve.Ref(assignedType, sc),
				})
				continue
			}
		}

		// Handle sibling assignments from expanding trailing calls.
		if sourceCall, start := info.ExpandingSourceCall(); sourceCall != nil {
			count := len(info.Targets) - start
			symbols := make([]cfg.SymbolID, count)
			names := make([]string, count)
			types := make([]typ.Type, count)
			for i := 0; i < count; i++ {
				target, ok := info.TargetAt(start + i)
				if !ok {
					continue
				}
				if target.Kind == cfg.TargetIdent && target.Name != "" {
					names[i] = target.Name
					symbols[i] = target.Symbol
				}
				if value := assignValueAt(values, start+i); value != nil {
					types[i] = value
				}
			}
			correlations, coCorrelations := extractCallCorrelations(sourceCall, wrappedSynth, p, resolverWithSpec)
			sibling := &flow.SiblingAssignment{
				Symbols:        symbols,
				Names:          names,
				Types:          types,
				Correlations:   correlations,
				CoCorrelations: coCorrelations,
			}
			for i, sym := range symbols {
				if sym != 0 && names[i] != "" {
					ver := fc.Graph.VisibleVersion(p, sym)
					if ver.ID == 0 {
						continue
					}
					key := flow.SiblingKey{Symbol: sym, VersionID: ver.ID}
					inputs.SiblingAssignments[key] = sibling
				}
			}
		}
	})
}

// ExtractFuncDefAssignments extracts function definitions as assignments.
// Handles:
// - Local/global function definitions: local function foo() ... end
// - Table field definitions: function M.add() ... end
// - Method definitions: function M:add() ... end
func ExtractFuncDefAssignments(fc *fbcore.FlowContext, inputs *flow.Inputs) {
	fc.Graph.EachFuncDef(func(p cfg.Point, info *cfg.FuncDefInfo) {
		sc := fc.Scopes[p]

		// Handle local/global function definitions
		if info.TargetKind == cfg.FuncDefGlobal {
			if info.Symbol == 0 || info.Name == "" {
				return
			}
			// Skip if already in DeclaredTypes (has explicit return types)
			if _, exists := inputs.DeclaredTypes[info.Symbol]; exists {
				return
			}
			// Synthesize the function type with inferred returns
			var fnType typ.Type
			if fc.API != nil && info.FuncExpr != nil {
				fnType = fc.API.TypeOf(info.FuncExpr, p)
			}
			if fnType == nil {
				fnType = typ.Unknown
			}
			// Create assignment for the function variable
			inputs.Assignments = append(inputs.Assignments, flow.UnifiedAssignment{
				Point: p,
				TargetPath: constraint.Path{
					Root:   resolve.RootName(fc.Graph, info.Symbol, info.Name),
					Symbol: info.Symbol,
				},
				Type: resolve.Ref(fnType, sc),
			})
			return
		}

		// Handle field and method definitions on receivers
		if info.TargetKind != cfg.FuncDefField && info.TargetKind != cfg.FuncDefMethod {
			return
		}

		if info.ReceiverName == "" {
			return
		}
		sym := info.ReceiverSymbol
		// ReceiverSymbol should be populated by the binder. Fallback to bindings lookup
		// if not set (for receivers that are simple identifiers).
		if sym == 0 {
			if bindings := fc.Graph.Bindings(); bindings != nil {
				if recvIdent, ok := info.Receiver.(*ast.IdentExpr); ok {
					sym, _ = bindings.SymbolOf(recvIdent)
				}
			}
		}
		if sym == 0 {
			return
		}
		root := resolve.RootNameFromBindings(fc.Graph.Bindings(), sym, info.ReceiverName)

		// Synthesize the function type
		var fnType typ.Type
		if fc.API != nil && info.FuncExpr != nil {
			fnType = fc.API.TypeOf(info.FuncExpr, p)
		}
		if fnType == nil {
			fnType = typ.Unknown
		}

		// Create sub-path assignment: M.add = function
		inputs.Assignments = append(inputs.Assignments, flow.UnifiedAssignment{
			Point: p,
			TargetPath: constraint.Path{
				Root:     root,
				Symbol:   sym,
				Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: info.Name}},
			},
			Type: resolve.Ref(fnType, sc),
		})
	})
}

// extractCallCorrelations extracts ErrorReturn and CorrelatedReturn correlations from the callee's spec.
// Callee type resolution is delegated to resolve.CalleeType to keep call semantics
// canonical across flowbuild passes.
func extractCallCorrelations(callInfo *cfg.CallInfo, synth func(ast.Expr, cfg.Point) typ.Type, p cfg.Point, symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool)) ([]flow.ReturnCorrelation, []flow.ReturnCorrelation) {
	if callInfo == nil {
		return nil, nil
	}
	fnType := resolve.CalleeType(callInfo, p, synth, symResolver, nil)
	inv, co := correlationsFromFunctionType(fnType)
	return inv, co
}

// correlationsFromFunctionType extracts ErrorReturn and CorrelatedReturn labels from a function's spec effects.
// Returns (inverse correlations, co-correlations).
func correlationsFromFunctionType(fnType typ.Type) ([]flow.ReturnCorrelation, []flow.ReturnCorrelation) {
	if fnType == nil {
		return nil, nil
	}
	spec := contract.ExtractSpec(fnType)
	var inverse []flow.ReturnCorrelation
	var coCorr []flow.ReturnCorrelation
	if spec != nil {
		for _, label := range spec.Effects.Labels {
			if er, ok := label.(effect.ErrorReturn); ok {
				inverse = append(inverse, flow.ReturnCorrelation{
					ValueIndex: er.ValueIndex,
					ErrorIndex: er.ErrorIndex,
				})
			}
			if cr, ok := label.(effect.CorrelatedReturn); ok {
				// Expand pairwise: each pair of indices forms a co-correlation
				for i := 0; i < len(cr.Indices); i++ {
					for j := i + 1; j < len(cr.Indices); j++ {
						coCorr = append(coCorr, flow.ReturnCorrelation{
							ValueIndex: cr.Indices[i],
							ErrorIndex: cr.Indices[j],
						})
					}
				}
			}
		}
	}
	if len(inverse) == 0 && len(coCorr) == 0 {
		convInv, convCo := InferErrorReturnConvention(fnType)
		if len(convInv) > 0 {
			inverse = append(inverse, convInv...)
		}
		if len(convCo) > 0 {
			coCorr = append(coCorr, convCo...)
		}
	}
	return inverse, coCorr
}
