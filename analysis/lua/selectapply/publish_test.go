package selectapply

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
	"github.com/wippyai/go-lua/domain/type/channelselect"
	"github.com/wippyai/go-lua/domain/type/typ"
)

func TestPublishWritesAcceptedFactsAndMissesLookalike(t *testing.T) {
	p := lowerProgram(t, `
type Event = {kind: string}
type Stop = {reason: string}
type Time = {sec: number}

local function handle(events_ch: Channel<Event>, stop_ch: Channel<Stop>, timeout_ch: Channel<Time>)
    local result = channel.select {
        events_ch:case_receive(),
        { channel = events_ch, value = 1, ok = true, default = nil },
        stop_ch:case_receive(),
        timeout_ch:case_receive(),
    }
    return result
end
`)
	apps := Apply(p)
	if len(apps) != 1 {
		t.Fatalf("Apply = %d applications, want 1", len(apps))
	}
	content, ok := Content(apps)
	if !ok {
		t.Fatal("Content refused accepted select facts")
	}
	if len(content.Rows) != 3 {
		t.Fatalf("Content rows = %d, want 3 accepted arms", len(content.Rows))
	}
	lookalikeID, lookalikeOK := channelselect.CaseFactID(channelselect.CaseFact{Site: apps[0].Site, Ordinal: 1})
	if !lookalikeOK {
		t.Fatal("lookalike identity unavailable")
	}
	if _, stored := content.Rows[lookalikeID]; stored {
		t.Fatal("lookalike ordinal was published as an accepted fact")
	}

	schemaID := identity.ContentID{0xC5, 0x01}
	write := mintSelectWrite(t, schemaID)
	builder := snapshot.NewBuilder(schemaID, identity.StoreID(1), identity.Generation(1))
	if err := Publish(write, &builder, apps); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	column := snapshot.Axis[identity.ContentID, channelselect.CaseFact]{SchemaID: schemaID, Slot: 0}
	for _, fact := range apps[0].Facts.All() {
		id, idOK := channelselect.CaseFactID(fact)
		if !idOK {
			t.Fatal("accepted fact has no identity")
		}
		got, status := snapshot.Read(&sealed, column, id)
		if status != snapshot.ReadHit {
			t.Fatalf("accepted fact %d read as %s", fact.Ordinal, status)
		}
		if got.Site != fact.Site || got.Ordinal != fact.Ordinal {
			t.Fatalf("stored fact = %+v, want site/ordinal of %+v", got, fact)
		}
		if !typ.TypeEquals(got.Channel, fact.Channel) || !typ.TypeEquals(got.Payload, fact.Payload) {
			t.Fatalf("stored types drifted for ordinal %d", fact.Ordinal)
		}
	}
	if _, status := snapshot.Read(&sealed, column, lookalikeID); status != snapshot.ReadMiss {
		t.Fatalf("lookalike identity read as %s, want miss", status)
	}
}

func TestPublishRefusesAForgedCapability(t *testing.T) {
	schemaID := identity.ContentID{0xC5, 0x02}
	builder := snapshot.NewBuilder(schemaID, identity.StoreID(1), identity.Generation(1))
	var forged engine.ColumnWrite[identity.ContentID, channelselect.CaseFact]
	if err := Publish(forged, &builder, nil); err == nil {
		t.Fatal("Publish accepted a zero write capability")
	}
}

func mintSelectWrite(t *testing.T, schemaID identity.ContentID) engine.ColumnWrite[identity.ContentID, channelselect.CaseFact] {
	t.Helper()
	binding := engine.NewColumnBinding()
	if !engine.AdmitColumns(binding, []engine.ColumnAdmission{{
		Schema: schemaID,
		Output: OutputKey,
		Writer: AxisKey,
		Slot:   0,
	}}) || !binding.Seal() {
		t.Fatal("column admission")
	}
	write, minted := engine.MintColumnWrite[identity.ContentID, channelselect.CaseFact](binding, OutputKey, AxisKey)
	if !minted || !write.Available() {
		t.Fatal("MintColumnWrite refused the select column")
	}
	return write
}
