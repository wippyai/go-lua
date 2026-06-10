// Package effects provides operations for function effect extraction and export.
//
// This package handles effect annotations for functions. Effects include
// termination guarantees, IO markers, and type predicates.
//
// # Type Extraction
//
// [EffectFromType] extracts function refinements from declared function type
// annotations and never-returning return types.
//
// # Export
//
// [EnrichExportWithEffects] attaches effect information to exported types:
//   - Adds termination annotations to function types
//   - Preserves type guard effects on methods
//   - Ensures effects are visible to importers
package effects
