// Package input assembles the inputs the canonical intraprocedural solver needs
// for one function: the CFG and graph topology, the raw graph-event evidence,
// and the declared scope facts (parameter symbols, names, and declared types).
//
// It reuses ONLY the sound leaf extractors of the legacy pipeline:
//
//   - the CFG and its binding table (compiler/cfg.Build / a session's
//     GetOrBuildCFG produce the same *cfg.Graph with bindings attached);
//   - the raw graph-event evidence (compiler/check/abstract/trace.GraphEvidence,
//     the same stream a session's store.EvidenceForGraph caches) — assignments,
//     calls, returns, branches, function definitions, identifier and parameter
//     uses;
//   - declared parameter facts read off the function's own parameter list, with
//     a caller-supplied resolver for the type annotations (annotations, aliases,
//     and base scopes are the caller's; this package does not own them).
//
// It does NOT call InferLocalTypes, CollectSpecNarrowedTypes,
// buildPreflowBranchSolution, or SolveConditionView, and it does not build the
// legacy flow.Inputs. Local inference is not an input here — it is produced by
// the value/condition/numeric transfer equations the solver runs over these
// inputs. Edge conditions and numeric conditions are transfer INPUTS the
// per-node transfer reads from the raw evidence and CFG, not pre-solved facts.
package input

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/abstract/trace"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/typ"
)

// Inputs is everything the canonical solver needs for one function. It is a
// passive carrier: the equation.Builder owns the topology walk, and the injected
// NodeTransfer reads these fields to compute per-node effects.
type Inputs struct {
	// Graph is the function's control-flow graph. Its Entry/Successors/
	// Predecessors drive the equation graph; its per-point node info (Assign,
	// Return, Branch, Call) is the syntactic source the transfer interprets.
	Graph *cfg.Graph

	// Evidence is the raw graph-event trace: assignments, calls, returns,
	// branches, identifier uses, parameter uses, and function definitions. The
	// transfer reads it as syntactic evidence, never as solved facts.
	Evidence api.FlowEvidence

	// Scope holds the declared parameter facts: the parameter symbols and names
	// in declaration order, and any resolved declared parameter types. Body uses
	// refine these into contract demand; declared types seed the entry state.
	Scope ScopeFacts
}

// ScopeFacts are the declared, non-flow facts about a function's parameters.
// They come from the parameter list and a caller-supplied annotation resolver;
// no flow solving is involved.
type ScopeFacts struct {
	// ParamSymbols are the parameter symbol IDs in declaration order.
	ParamSymbols []cfg.SymbolID
	// ParamNames are the parameter names in declaration order, parallel to
	// ParamSymbols.
	ParamNames []string
	// DeclaredParamTypes maps a parameter index to its resolved declared type.
	// An absent index is an unannotated parameter whose type the solver infers
	// from body use.
	DeclaredParamTypes map[int]typ.Type
}

// NumParams is the parameter count, the number of contract cells the equation
// graph allocates.
func (s ScopeFacts) NumParams() int {
	return len(s.ParamSymbols)
}

// TypeResolver resolves a declared parameter type annotation in the function's
// scope. It is the single seam to the caller's annotation/alias/base-scope
// machinery; this package does not own type resolution. A nil resolver, or a
// resolver returning nil for an annotation, leaves that parameter unannotated
// (inferred from body use).
type TypeResolver func(expr ast.TypeExpr, sc *scope.State) typ.Type

// Build assembles Inputs for the function whose graph is g, resolving parameter
// annotations through resolve in the function's base scope baseScope.
//
// g must already carry its binding table (cfg.Build attaches it; a session's
// GetOrBuildCFG returns such a graph). evidence is the raw graph-event trace; a
// caller with a session passes store.EvidenceForGraph(g), and BuildFromFunction
// derives it from the graph when none is supplied.
func Build(g *cfg.Graph, evidence api.FlowEvidence, resolve TypeResolver, baseScope *scope.State) Inputs {
	return Inputs{
		Graph:    g,
		Evidence: evidence,
		Scope:    scopeFacts(g, resolve, baseScope),
	}
}

// BuildFromFunction is the standalone path: it builds the CFG and its binding
// table from fn (the same cfg.Build a session uses), derives the raw evidence
// from the resulting graph (trace.GraphEvidence, the same stream a session
// caches), and assembles Inputs. globals are the predeclared global names the
// binder seeds. It is the entry point for callers that hold an *ast.FunctionExpr
// rather than a session-built graph.
func BuildFromFunction(fn *ast.FunctionExpr, resolve TypeResolver, baseScope *scope.State, globals ...string) Inputs {
	g := cfg.Build(fn, globals...)
	if g == nil {
		return Inputs{}
	}
	evidence := trace.GraphEvidence(g, g.Bindings())
	return Build(g, evidence, resolve, baseScope)
}

// scopeFacts reads the declared parameter facts off the graph's function and
// resolves the declared parameter types through resolve.
func scopeFacts(g *cfg.Graph, resolve TypeResolver, baseScope *scope.State) ScopeFacts {
	if g == nil {
		return ScopeFacts{}
	}
	facts := ScopeFacts{
		ParamSymbols:       g.ParamSymbols(),
		DeclaredParamTypes: make(map[int]typ.Type),
	}
	fn := g.Func()
	if fn == nil || fn.ParList == nil {
		return facts
	}
	facts.ParamNames = append([]string(nil), fn.ParList.Names...)
	if resolve == nil {
		return facts
	}
	for i, ann := range fn.ParList.Types {
		if ann == nil {
			continue
		}
		if t := resolve(ann, baseScope); t != nil {
			facts.DeclaredParamTypes[i] = t
		}
	}
	return facts
}
