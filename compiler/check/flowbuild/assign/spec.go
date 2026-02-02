package assign

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/transform"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// CollectSpecNarrowedTypes collects spec-narrowed and inferred types using a worklist fixpoint.
// This handles transitive dependencies (e.g., ch = listen({message=true}), msg = ch:receive()).
//
// Algorithm:
//  1. Build dependency map: symbol -> assign points that use it as receiver
//  2. Seed worklist with points that have known types (spec-narrowed or synthesized)
//  3. Process worklist: synth with current SpecTypes, enqueue dependents if type changes
//  4. Continue until fixpoint (no new types added)
//
// Convergence: The algorithm terminates because:
//   - Each symbol can only be added to bySymbol once (monotone growth)
//   - Worklist only adds points when a new symbol is narrowed
//   - Finite number of symbols bounds iterations
//
// Determinism: Guaranteed by sorted point processing and stable queue order.
func CollectSpecNarrowedTypes(graph *cfg.Graph, scopes map[cfg.Point]*scope.State, synth func(ast.Expr, cfg.Point) typ.Type, symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool), synthAPI api.SynthAPI) api.SpecTypes {
	bySymbol := make(api.SpecTypes)

	// Build dependency map: symbol -> points where it's used as method receiver
	deps := BuildReceiverDependencies(graph)

	// Collect and sort assign points for deterministic iteration
	points := graph.AssignPoints()
	sortPoints(points)

	// Seed phase: collect spec-narrowed types AND inferred types from method calls
	var worklist []cfg.Point
	for _, p := range points {
		info := graph.Assign(p)
		if info == nil {
			continue
		}
		sc := scopes[p]
		for i, target := range info.Targets {
			if target.Kind != cfg.TargetIdent || target.Name == "" {
				continue
			}
			if i >= len(info.SourceCalls) || info.SourceCalls[i] == nil {
				continue
			}
			sym := target.Symbol

			// Try spec narrowing first
			if narrowed := NarrowReturnTypeBySpec(info.SourceCalls[i], sc, synth, p, symResolver); narrowed != nil {
				bySymbol[sym] = narrowed
				worklist = append(worklist, deps[sym]...)
				continue
			}

			// Fall back to regular synthesis for method calls only
			// This captures t = time.now() where the return type is known
			// Only capture non-union types to avoid interfering with narrowing
			call := info.SourceCalls[i]
			if call != nil && call.Method != "" && i < len(info.Sources) && info.Sources[i] != nil && synth != nil {
				if inferred := synth(info.Sources[i], p); inferred != nil && !isUnknownOrNil(inferred) {
					// Skip union types - they may need narrowing later
					if _, isUnion := unwrap.Alias(inferred).(*typ.Union); !isUnion {
						bySymbol[sym] = inferred
						worklist = append(worklist, deps[sym]...)
					}
				}
			}
		}
	}

	// Fixpoint phase: process worklist until no new types are added
	processed := make(map[cfg.Point]bool)
	for len(worklist) > 0 {
		// Sort for deterministic processing order
		sortPoints(worklist)
		p := worklist[0]
		worklist = worklist[1:]

		if processed[p] {
			continue
		}
		processed[p] = true

		info := graph.Assign(p)
		if info == nil {
			continue
		}

		for i, target := range info.Targets {
			if target.Kind != cfg.TargetIdent || target.Name == "" {
				continue
			}
			sym := target.Symbol

			// Skip if already has a type (monotone: don't overwrite)
			if _, exists := bySymbol[sym]; exists {
				continue
			}

			// Check if this is a method call on a known receiver
			if i >= len(info.SourceCalls) || info.SourceCalls[i] == nil {
				continue
			}
			call := info.SourceCalls[i]
			if call.Receiver == nil {
				continue
			}
			recvSym := call.ReceiverSymbol
			if recvSym == 0 {
				continue
			}
			if _, hasKnownRecv := bySymbol[recvSym]; !hasKnownRecv {
				continue
			}

			// Synth method call with known receiver via synthAPI
			if synthAPI != nil && i < len(info.Sources) && info.Sources[i] != nil {
				values := synthAPI.ExpandValuesWithSpecTypes(info.Sources[i:i+1], 1, p, bySymbol)
				if len(values) > 0 && values[0] != nil && !isUnknownOrNil(values[0]) {
					bySymbol[sym] = values[0]
					// Enqueue points that depend on this newly typed symbol
					worklist = append(worklist, deps[sym]...)
				}
			}
		}
	}

	return bySymbol
}

// BuildReceiverDependencies builds a map from symbol to points where it's used as method receiver.
func BuildReceiverDependencies(graph *cfg.Graph) map[cfg.SymbolID][]cfg.Point {
	deps := make(map[cfg.SymbolID][]cfg.Point)
	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		for i := range info.Targets {
			if i >= len(info.SourceCalls) || info.SourceCalls[i] == nil {
				continue
			}
			call := info.SourceCalls[i]
			if call.Receiver == nil || call.ReceiverSymbol == 0 {
				continue
			}
			deps[call.ReceiverSymbol] = append(deps[call.ReceiverSymbol], p)
		}
	})
	return deps
}

func isUnknownOrNil(t typ.Type) bool {
	if t == nil {
		return true
	}
	return t == typ.Unknown
}

func sortPoints(points []cfg.Point) {
	for i := 1; i < len(points); i++ {
		for j := i; j > 0 && points[j] < points[j-1]; j-- {
			points[j], points[j-1] = points[j-1], points[j]
		}
	}
}

// NarrowReturnTypeBySpec checks if a call's return type should be narrowed
// based on the function's contract spec and inline literal argument values.
//
// This only handles inline table constructor literals like {message = true}.
// It does NOT handle variable references (e.g., local opts = {...}; f(opts)).
func NarrowReturnTypeBySpec(callInfo *cfg.CallInfo, sc *scope.State, synth func(ast.Expr, cfg.Point) typ.Type, p cfg.Point, symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool)) typ.Type {
	if callInfo == nil || synth == nil {
		return nil
	}

	// Get the function type via synthesis
	var fnType typ.Type
	if callInfo.Callee != nil {
		fnType = synth(callInfo.Callee, p)
	}
	// Use SymbolTypeResolver for identity-based lookup when CalleeSymbol is available
	if fnType == nil && callInfo.CalleeSymbol != 0 && symResolver != nil {
		fnType, _ = symResolver(0, callInfo.CalleeSymbol)
	}
	if fnType == nil {
		return nil
	}

	spec := contract.ExtractSpec(fnType)
	if spec == nil || spec.Return == nil || len(spec.Return.Cases) == 0 {
		return nil
	}

	// Check each return case against inline literal arguments
	if t := transform.ReturnTypeFromSpec(spec, callInfo.Args); t != nil {
		return t
	}

	return nil
}
