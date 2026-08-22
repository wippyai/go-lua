// Package modulecomposition declares resolved module composition rows. They
// are Link-lifetime joins of authored Link identity, the programmount column,
// and stable resolution identities; they are not authored Program or Link
// facts. Constructors validate canonical inputs once, while sealed rows keep
// only scalar identities and keys.
package modulecomposition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// The six axes are separate because each row family has one semantic key
// and one dependency edge. Sharing a column or parallel index would make an
// import, cache ingress, module-call transition, generation, outcome, or
// terminal answer to more than one vocabulary.
const (
	ImportAxisKey                       schema.Key = "module-composition/import"
	CacheAxisKey                        schema.Key = "module-composition/cache-ingress"
	ModuleCallTransitionAxisKey         schema.Key = "module-composition/call-transition"
	GenerationAxisKey                   schema.Key = "module-composition/init-generation"
	OutcomeAxisKey                      schema.Key = "module-composition/init-outcome"
	TerminalAxisKey                     schema.Key = "module-composition/init-terminal"
	ModuleExportCallableOriginAxisKey   schema.Key = "module-composition/module-export-callable-origin"
	ImportOutputKey                     schema.Key = "module-composition/imports"
	CacheOutputKey                      schema.Key = "module-composition/cache-ingresses"
	ModuleCallTransitionOutputKey       schema.Key = "module-composition/call-transitions"
	GenerationOutputKey                 schema.Key = "module-composition/init-generations"
	OutcomeOutputKey                    schema.Key = "module-composition/init-outcomes"
	TerminalOutputKey                   schema.Key = "module-composition/init-terminals"
	ModuleExportCallableOriginOutputKey schema.Key = "module-composition/module-export-callable-origins"

	ImportAxisRole                     = "axis/module-composition/import"
	CacheAxisRole                      = "axis/module-composition/cache-ingress"
	ModuleCallTransitionAxisRole       = "axis/module-composition/call-transition"
	GenerationAxisRole                 = "axis/module-composition/init-generation"
	OutcomeAxisRole                    = "axis/module-composition/init-outcome"
	TerminalAxisRole                   = "axis/module-composition/init-terminal"
	ModuleExportCallableOriginAxisRole = "axis/module-composition/module-export-callable-origin"
)

// AxisEntry declares one frozen, shared Link-lifetime sparse axis. A is the
// row type carried by that axis.
func ImportAxisEntry[A any]() axis.Spec[A] {
	return compositionAxis[A](ImportAxisKey, ImportOutputKey, ImportAxisRole, programmount.AxisKey)
}
func CacheAxisEntry[A any]() axis.Spec[A] {
	return compositionAxis[A](CacheAxisKey, CacheOutputKey, CacheAxisRole, ImportAxisKey)
}
func ModuleCallTransitionAxisEntry[A any]() axis.Spec[A] {
	return compositionAxis[A](ModuleCallTransitionAxisKey, ModuleCallTransitionOutputKey, ModuleCallTransitionAxisRole, CacheAxisKey, programmount.AxisKey)
}
func GenerationAxisEntry[A any]() axis.Spec[A] {
	return compositionAxis[A](GenerationAxisKey, GenerationOutputKey, GenerationAxisRole, CacheAxisKey, programmount.AxisKey)
}
func OutcomeAxisEntry[A any]() axis.Spec[A] {
	return compositionAxis[A](OutcomeAxisKey, OutcomeOutputKey, OutcomeAxisRole, GenerationAxisKey)
}
func TerminalAxisEntry[A any]() axis.Spec[A] {
	return compositionAxis[A](TerminalAxisKey, TerminalOutputKey, TerminalAxisRole, OutcomeAxisKey)
}
func ModuleExportCallableOriginAxisEntry[A any]() axis.Spec[A] {
	return compositionAxis[A](ModuleExportCallableOriginAxisKey, ModuleExportCallableOriginOutputKey, ModuleExportCallableOriginAxisRole,
		ModuleCallTransitionAxisKey, GenerationAxisKey, OutcomeAxisKey, programmount.AxisKey)
}

func compositionAxis[A any](key, output schema.Key, role string, dependencies ...schema.Key) axis.Spec[A] {
	return axis.Spec[A]{
		Key: key, Storage: axis.StorageEngine, Cardinality: axis.CardinalitySparse,
		Lifetime: axis.LifetimeLink, Mutability: axis.MutabilityFrozen, Concurrency: axis.ConcurrencyShared,
		Dependencies: dependencies,
		Frame:        axis.Frame{Outputs: []axis.Output{{Key: output, Writer: key}}}, Semantic: vocabulary.RoleKey(role),
	}
}

// StructureSpecs contributes all six semantic axis roles as one cohesive
// child declaration.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(ImportAxisRole, CacheAxisRole, ModuleCallTransitionAxisRole, GenerationAxisRole, OutcomeAxisRole, TerminalAxisRole, ModuleExportCallableOriginAxisRole)
}
