package architecture_test

import (
	"path/filepath"
	"strings"
	"testing"
)

// Mount is the first physical boundary below the independent checker.  Each
// mount package has a small, explicit input surface. A mounted package may
// also depend on a physically lower mount package (address < arrangement <
// witness).
var w2MountInputsByLayer = map[string][]string{
	"analysis/relation/mount/address": {
		"analysis/identity",
		"analysis/relation/check/certificate",
		"analysis/relation/schema/model",
		"analysis/relation/semantic/binding",
	},
	"analysis/relation/mount/arrangement": {
		"analysis/identity",
		"analysis/relation/check/certificate",
		"analysis/relation/schema/algebra",
		"analysis/relation/schema/model",
		"analysis/relation/schema/plan",
		"analysis/relation/semantic/signature",
	},
	// Certificate currently exposes a few signature/plan value views for
	// callers that need them. Witness may name those views, but may not reach
	// any checker subpass or the declaration compiler behind the certificate.
	"analysis/relation/mount/witness": {
		"analysis/identity",
		"analysis/relation/check/certificate",
		"analysis/relation/schema/model",
		"analysis/relation/schema/plan",
		"analysis/relation/semantic/binding",
		"analysis/relation/semantic/lineage",
		"analysis/relation/semantic/signature",
	},
}

var w2MountLayers = []struct {
	prefix string
	rank   int
}{
	{prefix: "analysis/relation/mount/address", rank: 0},
	{prefix: "analysis/relation/mount/arrangement", rank: 1},
	{prefix: "analysis/relation/mount/witness", rank: 2},
}

// State is not one undifferentiated engine package.  Each child owns one
// authority and may consume only the lower, named seams below it.  Keeping
// this allow-list explicit makes a new state package fail closed: adding a
// broad analysis/engine/relation/state/** exception would let physical
// mutation leak upward into the aggregate or transaction layers.
var w2StateInputsByLayer = map[string][]string{
	"analysis/engine/relation/state/geometry": {
		"analysis/relation/mount/witness",
		"analysis/relation/schema/model",
		"analysis/relation/semantic/binding",
	},
	"analysis/engine/relation/state/recurrence": {
		"analysis/relation/mount/witness",
		"analysis/relation/schema/model",
		"analysis/relation/semantic/binding",
	},
	"analysis/engine/relation/state/internal/column": {
		"analysis/engine/relation/state/geometry",
		"analysis/relation/schema/model",
		"analysis/relation/semantic/binding",
	},
	"analysis/engine/relation/state/index": {
		"analysis/engine/relation/state/geometry",
		"analysis/engine/relation/state/internal/column",
		"analysis/relation/schema/model",
		"analysis/relation/semantic/binding",
	},
	"analysis/engine/relation/state/store": {
		"analysis/engine/relation/state/geometry",
		"analysis/engine/relation/state/internal/column",
		"analysis/relation/mount/witness",
		"analysis/relation/schema/model",
		"analysis/relation/semantic/binding",
	},
	"analysis/engine/relation/state/bootstrap": {
		"analysis/engine/relation/state/internal/column",
		"analysis/engine/relation/state/store",
		"analysis/relation/mount/witness",
	},
	"analysis/engine/relation/state/transaction": {
		"analysis/engine/relation/state/geometry",
		"analysis/engine/relation/state/internal/column",
		"analysis/engine/relation/state/recurrence",
		"analysis/engine/relation/state/store",
		"analysis/relation/schema/model",
		"analysis/relation/semantic/binding",
		"analysis/relation/semantic/lineage",
	},
}

func TestW2MountPackagesConsumeOnlySealedInputs(t *testing.T) {
	root := repositoryRoot(t)
	for _, layer := range w2MountLayers {
		source, ok := altitudeFor(layer.prefix)
		if !ok {
			t.Fatalf("mount package %s is not registered in the architecture inventory", layer.prefix)
		}
		walkPackageImports(t, root, source, func(importPath string) {
			if reason := w2MountImportViolation(layer.prefix, importPath); reason != "" {
				t.Errorf("mount package %s imports %s: %s", layer.prefix, importPath, reason)
			}
		})
	}
}

func TestW2StatePackagesConsumeOnlyTheirDeclaredLowerSeams(t *testing.T) {
	root := repositoryRoot(t)
	for prefix, inputs := range w2StateInputsByLayer {
		source, ok := altitudeFor(prefix)
		if !ok {
			t.Fatalf("state package %s is not registered in the architecture inventory", prefix)
		}
		walkPackageImports(t, root, source, func(importPath string) {
			if reason := w2StateImportViolation(prefix, inputs, importPath); reason != "" {
				t.Errorf("state package %s imports %s: %s", prefix, importPath, reason)
			}
		})
	}
}

func TestW2RelationEngineRejectsRawLogicalAndLegacyProtocol(t *testing.T) {
	for _, source := range w0SourcesUnder(t, "analysis/engine/relation") {
		if strings.HasSuffix(source.path, "_test.go") {
			continue
		}
		packagePath := filepath.ToSlash(filepath.Dir(source.path))
		for _, importPath := range w0Imports(t, source) {
			if reason := w2EngineImportViolation(importPath); reason != "" {
				t.Errorf("relation engine package %s imports %s: %s", packagePath, importPath, reason)
			}
		}
	}
}

func TestW2ReferenceOracleCannotImportPhysicalOrDomainProtocol(t *testing.T) {
	root := repositoryRoot(t)
	source, ok := altitudeFor("internal/relationoracle")
	if !ok {
		t.Fatal("reference oracle is not registered in the architecture inventory")
	}
	walkPackageImports(t, root, source, func(importPath string) {
		if reason := w2OracleImportViolation(importPath); reason != "" {
			t.Errorf("reference oracle imports %s: %s", importPath, reason)
		}
	})
}

func w2MountImportViolation(sourcePrefix, importPath string) string {
	relative, controlled := w2RelativeImport(importPath)
	if !controlled {
		return ""
	}
	for _, allowed := range w2MountInputsByLayer[w2MountLayerPrefix(sourcePrefix)] {
		if w2Within(relative, allowed) {
			return ""
		}
	}

	sourceRank, sourceOK := w2MountRank(sourcePrefix)
	targetRank, targetOK := w2MountRank(relative)
	if sourceOK && targetOK && targetRank < sourceRank {
		return ""
	}
	return "mount may consume only certificate, model, identity, semantic binding, and lower mount packages"
}

func w2StateImportViolation(sourcePrefix string, allowed []string, importPath string) string {
	if !strings.HasPrefix(importPath, modulePath+"/") {
		return ""
	}
	relative := strings.TrimPrefix(importPath, modulePath+"/")
	if reason := w2ForbiddenGenericImport(relative); reason != "" {
		return reason
	}
	if _, controlled := controlledImport(importPath); !controlled {
		return ""
	}
	for _, input := range allowed {
		if w2Within(relative, input) {
			return ""
		}
	}
	return "state may consume only its declared lower seams; add a reviewed edge rather than broad-allowing the state subtree"
}

func w2ForbiddenGenericImport(relative string) string {
	switch {
	case w2Within(relative, "domain"):
		return "generic state cannot import domain implementations"
	case w2Within(relative, "analysis/engine/execution"):
		return "the old execution protocol is not a state dependency"
	case w2Within(relative, "analysis/engine/internal/carrier"),
		w2Within(relative, "analysis/engine/internal/factbinding"),
		w2Within(relative, "analysis/engine/internal/demand"),
		w2Within(relative, "analysis/engine/internal/equation"),
		w2Within(relative, "analysis/engine/internal/linkexecutionplan"),
		w2Within(relative, "analysis/engine/internal/executioncatalog"):
		return "the old form protocol is not a state dependency"
	}
	return ""
}

func w2MountLayerPrefix(path string) string {
	best := ""
	for prefix := range w2MountInputsByLayer {
		if w2Within(path, prefix) && len(prefix) > len(best) {
			best = prefix
		}
	}
	return best
}

func w2EngineImportViolation(importPath string) string {
	relative, controlled := w2RelativeImport(importPath)
	if !controlled {
		return ""
	}
	switch {
	case w2Within(relative, "domain"):
		return "domain implementations are outside the generic relation engine"
	case w2Within(relative, "analysis/schema/rule/relcompile"):
		return "declaration compilation must not cross into the relation engine"
	case w2Within(relative, "analysis/relation/schema/plan"):
		return "the engine consumes mounted artifacts, not raw logical plans"
	case w2Within(relative, "analysis/relation/check/registry"):
		return "the engine must not read the check registry"
	case w2Within(relative, "analysis/engine/execution"):
		return "the old execution protocol is not a relation-engine dependency"
	case w2Within(relative, "analysis/engine/internal/carrier"),
		w2Within(relative, "analysis/engine/internal/factbinding"),
		w2Within(relative, "analysis/engine/internal/demand"),
		w2Within(relative, "analysis/engine/internal/equation"),
		w2Within(relative, "analysis/engine/internal/linkexecutionplan"),
		w2Within(relative, "analysis/engine/internal/executioncatalog"):
		return "the old form protocol is not a relation-engine dependency"
	}
	return ""
}

func w2OracleImportViolation(importPath string) string {
	relative, controlled := w2RelativeImport(importPath)
	if !controlled {
		return ""
	}
	switch {
	case w2Within(relative, "analysis/engine"):
		return "the reference oracle cannot inherit physical engine assumptions"
	case w2Within(relative, "analysis/relation/mount"):
		return "the reference oracle cannot depend on mounted physical layout"
	case w2Within(relative, "domain"):
		return "the reference oracle cannot depend on domain implementations"
	case w2Within(relative, "analysis/engine/execution"),
		w2Within(relative, "analysis/engine/internal/carrier"),
		w2Within(relative, "analysis/engine/internal/factbinding"),
		w2Within(relative, "analysis/engine/internal/demand"),
		w2Within(relative, "analysis/engine/internal/equation"),
		w2Within(relative, "analysis/engine/internal/linkexecutionplan"),
		w2Within(relative, "analysis/engine/internal/executioncatalog"):
		return "the old form protocol is not a reference-oracle dependency"
	}
	return ""
}

func w2MountRank(path string) (int, bool) {
	bestRank := 0
	bestLength := -1
	for _, layer := range w2MountLayers {
		if w2Within(path, layer.prefix) && len(layer.prefix) > bestLength {
			bestRank = layer.rank
			bestLength = len(layer.prefix)
		}
	}
	return bestRank, bestLength >= 0
}

func w2RelativeImport(importPath string) (string, bool) {
	if !strings.HasPrefix(importPath, modulePath+"/") {
		return "", false
	}
	return strings.TrimPrefix(importPath, modulePath+"/"), true
}

func w2Within(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}
