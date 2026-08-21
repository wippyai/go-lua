package equation

import "testing"

func TestTopologyAssemblyConsumesOnlyBoundMaterializedBatch(t *testing.T) {
	fixture := newTemplateMaterializationFixture(t)
	materialized, materializedOK := MaterializeTemplateBoundary(fixture.source, fixture.binding,
		[]Site{fixture.input.Site(), fixture.local, fixture.output.Site()}, fixture.inputs)
	assembly, assemblyOK := SealTopologyAssembly(fixture.actuals, []TemplateMaterialization{materialized})
	if !materializedOK || !assemblyOK || !assembly.Available() || assembly.Batch() == fixture.actuals || assembly.Batch() == materialized.Batch() {
		t.Fatal("closed topology directory")
	}
	base, baseOK := assembly.Site(fixture.actualInput)
	local, localOK := materialized.Site(fixture.local)
	localDirectory, directoryOK := assembly.Site(local)
	first, inputOK := materialized.InputAt(0)
	inputDirectory, reissuedOK := assembly.Input(first)
	if !baseOK || !localOK || !directoryOK || !inputOK || !reissuedOK || !assembly.Batch().OwnsSite(base) || !assembly.Batch().OwnsSite(localDirectory) || !inputDirectory.Available() || inputDirectory.Source().batch != assembly.Batch() || inputDirectory.Target().batch != assembly.Batch() {
		t.Fatal("target rows were not reissued into topology directory")
	}
	for _, row := range assembly.data.directory.operands {
		if !row.realm.Available() || row.realm != materialized.Batch().key {
			t.Fatal("materialized operand lost its private batch realm")
		}
	}
	if _, accepted := SealTopologyAssembly(materialized.Batch(), []TemplateMaterialization{materialized}); accepted {
		t.Fatal("materialization bound to foreign base Batch")
	}
	if _, accepted := SealTopologyAssembly(fixture.actuals, []TemplateMaterialization{materialized, materialized}); accepted {
		t.Fatal("duplicate materialization batch accepted")
	}
}
