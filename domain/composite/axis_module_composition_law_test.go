package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/modulecomposition"
)

// TestModuleExportCallableOriginHasOneEngineColumnGrant pins the declaration
// needed by Link composition: building a callable-origin row is not enough;
// the sealed schema must grant that owner exactly one writable Snapshot column.
func TestModuleExportCallableOriginHasOneEngineColumnGrant(t *testing.T) {
	compilation, publication := publicationForTest(t)
	entry, declared := axisForKey(compilation.catalog, modulecomposition.ModuleExportCallableOriginAxisKey)
	if !declared {
		t.Fatalf("the composition declares no axis %q", modulecomposition.ModuleExportCallableOriginAxisKey)
	}
	if entry.Storage() != axis.StorageEngine || entry.OutputCount() != 1 {
		t.Fatalf("callable-origin axis storage/outputs = %d/%d", entry.Storage(), entry.OutputCount())
	}
	output, outputOK := entry.OutputAt(0)
	if !outputOK || output.Key != modulecomposition.ModuleExportCallableOriginOutputKey || output.Writer != modulecomposition.ModuleExportCallableOriginAxisKey {
		t.Fatalf("callable-origin output = %+v/%t", output, outputOK)
	}
	binding := engine.NewColumnBinding()
	if !publication.AdmitColumns(binding) || !binding.Seal() {
		t.Fatal("publication admission")
	}
	write, minted := engine.MintColumnWrite[identity.ContentID, modulecomposition.ModuleExportCallableOrigin](
		binding,
		modulecomposition.ModuleExportCallableOriginOutputKey,
		modulecomposition.ModuleExportCallableOriginAxisKey,
	)
	if !minted || !write.Available() {
		t.Fatal("callable-origin writer was not granted its declared column")
	}
}
