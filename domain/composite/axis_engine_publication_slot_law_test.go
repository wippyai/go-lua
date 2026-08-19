package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/selectapply"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/snapshot"
	"github.com/wippyai/go-lua/domain/type/channelselect"
)

// TestEnginePublishedColumnsLeadTheSnapshotSlotRange is the composition
// publication law that unblocks Link-lifetime StorageEngine columns.
// Snapshot Seal is dense from slot zero. Factor axes are solve-lifetime and
// occupy declaration positions 1..N so artifact lane ordinals stay put.
// Snapshot slots are a different numbering: engine-published outputs occupy
// the leading prefix, so a compile-time publication can seal that prefix
// without filling factor columns that do not exist yet.
func TestEnginePublishedColumnsLeadTheSnapshotSlotRange(t *testing.T) {
	requests, ok := WriteRequests()
	if !ok || len(requests) == 0 {
		t.Fatal("the sealed table issues no write requests")
	}
	engineCount := 0
	for _, request := range requests {
		writer, writerOK := axisForKey(request.Writer)
		if !writerOK {
			t.Fatalf("column %q is written by unknown axis %q", request.Output, request.Writer)
		}
		if writer.Storage() == axis.StorageEngine {
			if request.Slot != uint32(engineCount) {
				t.Fatalf("engine-published column %q occupies slot %d, not prefix slot %d", request.Output, request.Slot, engineCount)
			}
			engineCount++
			continue
		}
		if uint32(engineCount) == 0 {
			t.Fatal("no engine-published column leads the snapshot slot range")
		}
		if request.Slot < uint32(engineCount) {
			t.Fatalf("bound column %q occupies engine prefix slot %d", request.Output, request.Slot)
		}
	}
	if engineCount == 0 {
		t.Fatal("the sealed table publishes no engine-written column")
	}

	selectColumn, projected := ProjectAxis[identity.ContentID, channelselect.CaseFact](selectapply.OutputKey)
	if !projected || !selectColumn.Available() {
		t.Fatal("channel-select-case/facts projects no address")
	}
	if selectColumn.Slot != 0 {
		t.Fatalf("channel-select-case/facts occupies slot %d, so a select-only publication cannot seal", selectColumn.Slot)
	}

	schemaID, schemaOK := PublicationSchema()
	if !schemaOK {
		t.Fatal("publication schema")
	}
	builder := snapshot.NewBuilder(schemaID, pilotStore, pilotGeneration)
	if err := snapshot.PutColumn(&builder, selectColumn, snapshot.Content[identity.ContentID, channelselect.CaseFact]{}); err != nil {
		t.Fatalf("fill the leading engine column: %v", err)
	}
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("a snapshot that fills only the leading engine-published column must seal: %v", err)
	}
	if _, status := snapshot.Read(&sealed, selectColumn, identity.ContentID{0x01}); status != snapshot.ReadMiss {
		t.Fatalf("empty select column read as %s", status)
	}
}
