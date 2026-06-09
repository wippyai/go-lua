// Package api defines interfaces and types for the Lua type checker.
//
// This package provides the contract between different phases of type analysis.
// It defines store interfaces for accessing analysis results, session interfaces
// for managing analysis state, and data types for public/export projections.
//
// # Store Interfaces
//
// The package defines a hierarchy of store interfaces with increasing capability:
//
//   - [ModuleStore]: Module-level bindings and alias maps
//   - [GraphStore]: Access to built CFGs by ID
//   - [ParentScopes]: Parent scope lookup for nested functions
//   - [StoreReader]: Read contract combining the immutable stores above
//   - [CanonicalStore]: Canonical-owned metadata and final fact projection
//
// These interfaces allow different phases to declare their dependencies and
// enable testing with mock implementations.
//
// # Analysis Session
//
// [AnalysisSession] is the minimal interface for the fixpoint driver. It
// provides access to the query context, CFG construction, result storage,
// and diagnostic collection. The concrete implementation lives in the
// check package.
//
// # Final Projections
//
// Canonical checking uses Summary as its interprocedural authority. This package
// exposes final/public projection facts; noncanonical postflow lane carriers live
// under compiler/check/domain/postflow and their convergence laws live under
// compiler/check/domain/interproc:
//
//   - [FunctionFacts]: final/public per-function projection facts
//   - compiler/check/domain/postflow captured/constructor lanes: noncanonical
//     postflow compatibility projections
//
// Projection lanes are keyed by a (graph, parent-scope) [GraphKey]; module-wide
// lanes use [ModuleFactsKey].
//
// # Function References
//
// [FunctionRef] maps function symbols to their defining context:
//
//   - Symbol ID for identity
//   - Graph ID for CFG lookup
//   - Parent graph ID for nested functions
//   - Definition point within the parent
//
// The [FunctionRefs] interface provides bidirectional lookup between symbols,
// AST nodes, and CFG graphs.
//
// # Synthesis Modes
//
// The [SynthMode] type identifies the active synthesis view, enabling callers to
// choose declared/static or flow-refined reads without creating a second analysis
// path.
//
// # Synthesis Interface
//
// [BaseSynth] provides the type synthesis interface for expression analysis.
// It is implemented by the synth.Engine and used by diagnostics and flow input
// construction to evaluate expressions in context.
package api
