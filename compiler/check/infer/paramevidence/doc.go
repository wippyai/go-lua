// Package paramevidence computes parameter evidence from call-site arguments.
//
// This package analyzes function call sites and body uses to build effective
// parameter types for functions without explicit type annotations.
//
// # Evidence Collection
//
// For each call site:
//
//	foo(123, "bar")  -- evidence: param1=number, param2=string
//
// The package collects argument types and associates them with parameter
// positions. Multiple call sites contribute evidence that is joined.
//
// # Evidence Merging
//
// When multiple calls provide conflicting evidence:
//
//	foo(1)      -- evidence: param1=number
//	foo("a")    -- evidence: param1=string
//
// The evidence is joined to produce: param1 = number | string
//
// # Integration
//
// Parameter evidence feeds into function signature inference, providing
// types for parameters that lack explicit annotations.
package paramevidence
