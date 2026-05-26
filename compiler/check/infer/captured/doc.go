// Package captured computes captured variable types for nested functions.
//
// This package determines the types of variables captured by nested functions
// at the program point where the closure environment is observed. Captured
// types flow from parent scope analysis into nested function analysis.
//
// # Capture Analysis
//
// When a nested function references a variable from an outer scope:
//
//	local x = getValue()
//	local function inner()
//	    print(x)  -- x is captured
//	end
//
// The package computes x's type at the point where inner is defined or called,
// using flow facts from the parent function's analysis. Call-site capture uses
// the caller pre-state so the callee does not see effects from its own call
// replayed into the parent graph.
//
// # Flow Integration
//
// Captured types are computed from parent [flow.TypeFacts]:
//   - The parent's flow solution provides types at each CFG point
//   - At the nested function's definition/call point, captured variable
//     types are extracted and passed to the nested analysis
//   - Path-sensitive field/container facts below the captured root are
//     projected through the same abstract-state query instead of rebuilt by
//     diagnostics or call checking
//
// # Usage
//
// [FromParentFactsAtPoint] is the main entry point, producing a map from
// captured symbol IDs to their types at the observed point.
package captured
