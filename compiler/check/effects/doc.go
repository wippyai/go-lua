// Package effects provides operations for function effect export.
//
// This package attaches already-known effect annotations to exported function
// types so they remain visible across module boundaries.
//
// # Export
//
// [EnrichExportWithEffects] attaches effect information to exported types:
//   - Adds termination annotations to function types
//   - Preserves type guard effects on methods
//   - Ensures effects are visible to importers
package effects
