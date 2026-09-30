package captured

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/assign"
	"github.com/wippyai/go-lua/compiler/check/nested"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// ParentContext supplies the parent facts and mutation information for captures.
type ParentContext struct {
	ParentGraph   *cfg.Graph
	ChildGraph    *cfg.Graph
	Point         cfg.Point
	Facts         flow.TypeFacts
	Solution      *flow.Solution
	Flow          api.FlowOps
	FallbackTypes map[cfg.SymbolID]typ.Type
	TypeOf        func(ast.Expr, cfg.Point) typ.Type
	Classes       api.ClassSelfSource
	Mutations     api.TableMutationSource
}

// Types computes the captures a closure body observes, including field writes
// and the existing normalization of tables mutated after creation.
func Types(input ParentContext) map[cfg.SymbolID]typ.Type {
	nestedGraph := input.ChildGraph
	if nestedGraph == nil || nestedGraph.Bindings() == nil {
		return nil
	}
	isAnnotated := func(sym cfg.SymbolID) bool {
		return input.Facts != nil && input.Facts.IsAnnotated(sym) && nestedGraph.Bindings().TypeAnnotation(sym) != nil
	}
	parameters := make(map[cfg.SymbolID]cfg.Point)
	if input.ParentGraph != nil {
		for _, slot := range input.ParentGraph.ParamSlotsReadOnly() {
			parameters[slot.Symbol] = slot.DeclPoint
		}
	}
	valueAtCapture := func(sym cfg.SymbolID) typ.Type {
		value := input.FallbackTypes[sym]
		declPoint, parameter := parameters[sym]
		if !parameter {
			return value
		}
		flowOps := input.Flow
		if flowOps == nil && input.Solution != nil {
			flowOps = input.Solution
		}
		if flowOps != nil {
			if current := flowOps.NarrowedTypeAt(input.Point, constraint.Path{Symbol: sym}); current != nil && !typ.IsUnresolved(current) {
				return current
			}
		}
		if value != nil && !value.Kind().IsPlaceholder() {
			return value
		}
		entry := input.ParentGraph.VisibleVersion(declPoint, sym)
		current := input.ParentGraph.VisibleVersion(input.Point, sym)
		// A reassigned parameter needs capture-point evidence. Its declaration
		// describes the entry value and cannot type this later SSA version.
		if !current.IsZero() && current.ID != entry.ID {
			return typ.Unresolved
		}
		if input.Facts != nil {
			if fact := input.Facts.EffectiveTypeAt(input.Point, sym); fact.State == flow.StateResolved && fact.Type != nil {
				return fact.Type
			}
		}
		return value
	}
	// When synthesis runs inside the closure's own graph, its declarations
	// already include the captures supplied by nested body checking.
	if input.ParentGraph == nestedGraph {
		if input.Facts == nil {
			return nil
		}
		out := make(map[cfg.SymbolID]typ.Type)
		for _, sym := range nestedGraph.Bindings().CapturedSymbols(nestedGraph.Func()) {
			if tv := input.Facts.DeclaredAt(input.Point, sym); tv.State == flow.StateResolved && tv.Type != nil {
				out[sym] = tv.Type
			}
			if value := valueAtCapture(sym); value != nil {
				out[sym] = value
			}
		}
		return out
	}
	classTypes := make(map[cfg.SymbolID]typ.Type)
	if nestedGraph != nil && nestedGraph.Bindings() != nil && input.Classes != nil {
		for _, sym := range nestedGraph.Bindings().CapturedSymbols(nestedGraph.Func()) {
			classTypes[sym] = input.Classes.ClassSelfType(input.ParentGraph, sym)
		}
	}
	var capturedTypes map[cfg.SymbolID]typ.Type
	if nestedGraph != nil {
		capturedTypes = FromParentFacts(input.Facts, nestedGraph, input.Point, nestedGraph.Bindings())
		for _, sym := range nestedGraph.Bindings().CapturedSymbols(nestedGraph.Func()) {
			if value := valueAtCapture(sym); value != nil {
				if capturedTypes == nil {
					capturedTypes = make(map[cfg.SymbolID]typ.Type)
				}
				capturedTypes[sym] = value
			}
		}
	}
	if nestedGraph != nil && input.TypeOf != nil {
		bindings := nestedGraph.Bindings()
		if bindings != nil {
			capturedSyms := bindings.CapturedSymbols(nestedGraph.Func())
			if len(capturedSyms) > 0 {
				capturedSet := make(map[cfg.SymbolID]bool, len(capturedSyms))
				for _, sym := range capturedSyms {
					// Supplied capture values already incorporate their preceding writes.
					// Aggregated writes also include non-dominating later assignments.
					if sym != 0 && classTypes[sym] == nil && !isAnnotated(sym) && input.FallbackTypes[sym] == nil {
						capturedSet[sym] = true
					}
				}
				if len(capturedSet) > 0 {
					fields := assign.CollectFieldAssignments(input.ParentGraph, input.TypeOf, capturedSet)
					if len(fields) > 0 {
						if capturedTypes == nil {
							capturedTypes = make(map[cfg.SymbolID]typ.Type, len(fields))
						}
						for _, sym := range cfg.SortedSymbolIDs(fields) {
							fieldMap := fields[sym]
							if sym == 0 {
								continue
							}
							base := capturedTypes[sym]
							captured := returns.MergeFieldsIntoType(base, fieldMap)
							if input.Solution != nil {
								// A closure sees the table after its preceding writes. The
								// declaration may leave a field as any even though the
								// solved value at this definition is a typed function.
								for _, name := range cfg.SortedFieldNames(fieldMap) {
									path := constraint.Path{Symbol: sym, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: name}}}
									if t := input.Solution.TypeAt(input.Point, path); t != nil && !typ.IsAny(t) && !typ.IsUnknown(t) {
										captured = typ.ExtendRecordWithField(captured, name, t)
									}
									if t := initializedCapturedField(input.ParentGraph, input.Point, sym, name, input.TypeOf); t != nil {
										captured = typ.ExtendRecordWithField(captured, name, t)
									}
								}
							}
							capturedTypes[sym] = captured
						}
					}
				}
			}
		}
	}

	if nestedGraph != nil && nestedGraph.Bindings() != nil {
		captureBounds := make(map[cfg.SymbolID]typ.Type, len(capturedTypes))
		for sym, t := range capturedTypes {
			if isAnnotated(sym) {
				captureBounds[sym] = input.Facts.DeclaredAt(input.Point, sym).Type
			} else {
				// Widen before field narrowing erases aliases, which bound inference.
				captureBounds[sym] = subtype.WidenForInference(t)
			}
		}
		if input.Solution != nil {
			unstable := make(map[cfg.SymbolID]bool)
			if input.ParentGraph != nil {
				input.ParentGraph.EachAssign(func(point cfg.Point, assignment *cfg.AssignInfo) {
					if point <= input.Point || assignment == nil {
						return
					}
					for _, target := range assignment.Targets {
						if target.Symbol != 0 {
							unstable[target.Symbol] = true
						}
						if target.BaseSymbol != 0 {
							unstable[target.BaseSymbol] = true
						}
					}
				})
			}
			for sym, t := range capturedTypes {
				if !unstable[sym] {
					capturedTypes[sym] = NarrowRecordFields(t, input.Solution, input.Point, sym)
				}
			}
		}
		for _, sym := range nestedGraph.Bindings().CapturedSymbols(nestedGraph.Func()) {
			if bound := classTypes[sym]; bound != nil {
				if capturedTypes == nil {
					capturedTypes = make(map[cfg.SymbolID]typ.Type)
				}
				capturedTypes[sym] = bound
				continue
			}
			if t := capturedTypes[sym]; t != nil && input.Mutations != nil {
				capturedTypes[sym] = nested.NormalizeCapturedTableType(t, input.Mutations.TableMutation(sym), captureBounds[sym])
			}
		}
	}

	return capturedTypes
}
