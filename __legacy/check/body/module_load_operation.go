package body

import (
	"github.com/wippyai/go-lua/__legacy/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/calloutcome"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// moduleLoadOperations compiles every statically identified require site, not
// merely literal-argument sites. The exact ValueSource is retained for the
// executor to resolve against the point-local state.
func moduleLoadOperations(reg *axis.Registry, graph cfg.Graph, facts factflow.Facts, identity *signatureIdentityResolver, modules importlookup.Source, values *typevalue.Cache) map[cfg.Point]operationplan.ModuleLoadOperation {
	if reg == nil || graph == nil || identity == nil {
		return nil
	}
	exports := effectiveModuleLoadExports(reg, modules, values)
	if len(exports) == 0 {
		return nil
	}
	table, ok := operationplan.NewModuleLoadExportTable(reg, exports)
	if !ok {
		return nil
	}
	out := make(map[cfg.Point]operationplan.ModuleLoadOperation)
	for point := cfg.Point(0); int(point) < graph.Size(); point++ {
		site, ok := facts.CallSiteView(point)
		if !ok || site.ArgumentSourceCount() != 1 {
			continue
		}
		ctx := transfer.NodeContext{Graph: graph, Point: point, Node: graph.Node(point)}
		name, ok := identity.nameForCallSiteView(ctx, site)
		if !ok || name != "require" {
			continue
		}
		argument, ok := site.ArgumentSourceAt(0)
		if !ok || !argument.Valid() {
			continue
		}
		operation, ok := operationplan.NewModuleLoadOperationWithTable(argument, table)
		if ok {
			out[point] = operation
		}
	}
	return out
}

// effectiveModuleLoadExports applies importlookup's later-manifest-wins rule
// before materialization. The operation constructor supplies canonical path
// ordering independently of manifest input order.
func effectiveModuleLoadExports(reg *axis.Registry, modules importlookup.Source, values *typevalue.Cache) []operationplan.ModuleLoadExport {
	seen := make(map[string]struct{}, len(modules.Manifests))
	out := make([]operationplan.ModuleLoadExport, 0, len(modules.Manifests))
	for index := len(modules.Manifests) - 1; index >= 0; index-- {
		manifest := modules.Manifests[index]
		if manifest == nil || manifest.Path == "" || manifest.Export == nil {
			continue
		}
		if _, duplicate := seen[manifest.Path]; duplicate {
			continue
		}
		seen[manifest.Path] = struct{}{}
		exportType := manifest.ScopeType(manifest.Export)
		if exportType == nil {
			continue
		}
		value := values.FromTypeWithWitness(reg, exportType)
		outcome := callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: value}}}
		out = append(out, operationplan.ModuleLoadExport{
			Path: manifest.Path, Value: value,
			PostReturnAuthority: calloutcome.HasAuthoritativePostReturnEvidence(reg, outcome),
		})
	}
	return out
}
