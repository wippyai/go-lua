// Package paramevidence owns the parameter-evidence domain.
//
// The domain canonicalizes, joins, and widens observations from call sites,
// body-derived contracts, and signature facts. Orchestration packages decide
// when an observation is produced; this package decides what that observation
// means and how it combines with prior evidence.
//
// # Evidence Collection
//
// For each call site:
//
//	foo(123, "bar")  -- evidence: param1=number, param2=string
//
// Call-site analysis collects argument types and associates them with parameter
// positions. Multiple call sites contribute evidence that is joined here.
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
// Parameter evidence feeds into function signature inference, providing types
// for parameters that lack explicit annotations.
package paramevidence
