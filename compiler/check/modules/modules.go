// Package modules provides module export and import functionality for the type checker.
//
// When a Lua file is type-checked as a module, its exports (the return value) are
// captured and stored in a database. Other modules can then require() this module
// and receive accurate type information for its exports.
//
// # Manifest Structure
//
// Each module produces an io.Manifest containing:
//   - Export type: The type of the module's return value
//   - Type definitions: Named types exported by the module
//   - Function summaries: Effect and constraint information for exported functions
//
// # Function Summaries
//
// Function summaries enable cross-module flow analysis. They capture:
//   - Effect rows: Read/write/throw effects
//   - KeyOf constraints: Type narrowing conditions on return
//
// These summaries allow the type checker to perform narrowing and escape analysis
// when calling functions from imported modules.
package modules

import (
	"sort"
	"strings"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// Connect creates a manifest from export data and connects it to the DB.
//
// This is the main entry point for module export. It:
//  1. Creates a new manifest for the module
//  2. Sets the export type (module's return value type)
//  3. Defines any exported types
//  4. Extracts function summaries for cross-module analysis
//  5. Registers the manifest with the database
func Connect(database *db.DB, name string, exportType typ.Type, exportTypes map[string]typ.Type, graph *cfg.Graph, effectsBySym map[cfg.SymbolID]*constraint.FunctionEffect) *io.Manifest {
	manifest := io.NewManifest(name)
	manifest.SetExport(exportType)

	if len(exportTypes) > 0 {
		names := make([]string, 0, len(exportTypes))
		for name := range exportTypes {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, typeName := range names {
			manifest.DefineType(typeName, exportTypes[typeName])
		}
	}

	ExportFunctionSummaries(manifest, exportType, graph, effectsBySym)

	database.Connect(name, manifest)

	return manifest
}

// ExportFunctionSummaries populates manifest with function summaries.
//
// For each exported function with OnReturn constraints, this creates
// a summary containing parameter types, return types, and the ensures constraint.
// Effect rows are also included when available.
//
// OnReturn constraints encode assert-style narrowing (e.g. assert.not_nil),
// enabling callers to narrow types based on imported module behavior.
func ExportFunctionSummaries(manifest *io.Manifest, exportType typ.Type, graph *cfg.Graph, effectsBySym map[cfg.SymbolID]*constraint.FunctionEffect) {
	if graph == nil || len(effectsBySym) == 0 {
		return
	}

	rec, ok := exportType.(*typ.Record)
	if !ok {
		return
	}

	if len(effectsBySym) == 0 {
		return
	}
	for _, sym := range cfg.SortedSymbolIDs(effectsBySym) {
		eff := effectsBySym[sym]
		if eff == nil || !eff.OnReturn.HasConstraints() {
			continue
		}

		fullName := graph.NameOf(sym)
		if fullName == "" {
			continue
		}

		fieldName := fullName
		if idx := strings.LastIndex(fullName, "."); idx != -1 && idx+1 < len(fullName) {
			fieldName = fullName[idx+1:]
		}

		field := rec.GetField(fieldName)
		if field == nil {
			continue
		}
		fn, ok := field.Type.(*typ.Function)
		if !ok {
			continue
		}

		params := make([]typ.Type, len(fn.Params))
		for i, p := range fn.Params {
			params[i] = p.Type
		}
		ioSummary := io.NewSummary(params, fn.Returns)
		ioSummary.Ensures = eff.OnReturn
		if row, ok := eff.Row.(effect.Row); ok {
			ioSummary.Effects = row
		}
		manifest.DefineSummary(fieldName, ioSummary)
	}
}

// Disconnect removes a module's manifest from the DB.
func Disconnect(database *db.DB, name string) {
	database.Disconnect(name)
}
