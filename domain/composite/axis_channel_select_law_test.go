package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/lua/selectapply"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/snapshot"
	"github.com/wippyai/go-lua/domain/type/channelselect"
)

func TestChannelSelectCaseIsDeclaredAsAnEnginePublishedAxis(t *testing.T) {
	entry, declared := axisForKey(axisKeyChannelSelectCase)
	if !declared {
		t.Fatalf("the composition declares no axis %q", axisKeyChannelSelectCase)
	}
	if entry.Key() != selectapply.AxisKey {
		t.Fatalf("the registered axis is keyed %q, its owner declares %q", entry.Key(), selectapply.AxisKey)
	}
	storage, storageOK := AxisStorage(axisKeyChannelSelectCase)
	if !storageOK || storage != axis.StorageEngine {
		t.Fatalf("axis %q declares storage %d, not the engine-published one", axisKeyChannelSelectCase, storage)
	}
	if storage.Bound() {
		t.Fatal("the engine-published axis reports a bound storage")
	}
	if entry.MountDeclared() {
		t.Fatal("the engine-published axis seals a Link authority of its own")
	}
	roles, rolesOK := SemanticRoles()
	if !rolesOK {
		t.Fatal("semantic role vocabulary")
	}
	expected, expectedOK := roles.Key("semantic/axis/channel-select-case")
	semantic, semanticOK := AxisSemantic(axisKeyChannelSelectCase)
	if !expectedOK || !semanticOK || semantic != expected {
		t.Fatalf("axis %q publishes %x, the vocabulary declares the axis role", axisKeyChannelSelectCase, semantic.Digest())
	}
}

func TestChannelSelectCasePublishesOneEngineWrittenColumn(t *testing.T) {
	entry, declared := axisForKey(axisKeyChannelSelectCase)
	if !declared {
		t.Fatalf("the composition declares no axis %q", axisKeyChannelSelectCase)
	}
	if entry.OutputCount() != 1 {
		t.Fatalf("axis %q publishes %d columns, not the one it declares", axisKeyChannelSelectCase, entry.OutputCount())
	}
	output, outputOK := entry.OutputAt(0)
	if !outputOK || output.Key != selectapply.OutputKey || output.Writer != selectapply.AxisKey {
		t.Fatalf("axis %q publishes column %q written by %q", axisKeyChannelSelectCase, output.Key, output.Writer)
	}
	requests, requestsOK := WriteRequests()
	if !requestsOK {
		t.Fatal("the sealed table issues no write requests")
	}
	issued := 0
	for _, request := range requests {
		if request.Output != selectapply.OutputKey {
			continue
		}
		issued++
		if request.Writer != selectapply.AxisKey {
			t.Fatalf("column %q is requested for writer %q", request.Output, request.Writer)
		}
	}
	if issued != 1 {
		t.Fatalf("column %q is requested %d times", selectapply.OutputKey, issued)
	}
}

func TestChannelSelectCaseColumnStoresAcceptedFactsAndMissesLookalike(t *testing.T) {
	program, err := lualower.Lower(lualower.Source{Name: "select.lua", Text: []byte(`
type Event = {kind: string}
type Stop = {reason: string}

local function handle(events_ch: Channel<Event>, stop_ch: Channel<Stop>)
    channel.select {
        events_ch:case_receive(),
        { channel = events_ch, value = 1, ok = true, default = nil },
        stop_ch:case_receive(),
    }
end
`)})
	if err != nil {
		t.Fatal(err)
	}
	apps := selectapply.Apply(program)
	if len(apps) != 1 {
		t.Fatalf("Apply = %d applications, want 1", len(apps))
	}

	schemaID, schemaOK := PublicationSchema()
	if !schemaOK || !schemaID.Available() {
		t.Fatal("the sealed table publishes no schema identity")
	}
	column, projected := ProjectAxis[identity.ContentID, channelselect.CaseFact](selectapply.OutputKey)
	if !projected || !column.Available() {
		t.Fatalf("the declared column %q projects no address", selectapply.OutputKey)
	}

	requests, requestsOK := WriteRequests()
	if !requestsOK {
		t.Fatal("the sealed table issues no write requests")
	}
	binding := engine.NewColumnBinding()
	if !admitPublicationColumns(binding) || !binding.Seal() {
		t.Fatal("publication admission")
	}
	write, minted := engine.MintColumnWrite[identity.ContentID, channelselect.CaseFact](binding, selectapply.OutputKey, selectapply.AxisKey)
	if !minted || !write.Available() {
		t.Fatal("MintColumnWrite refused the select column")
	}

	builder := snapshot.NewBuilder(schemaID, pilotStore, pilotGeneration)
	for _, request := range requests {
		if request.Output == selectapply.OutputKey {
			if err := selectapply.Publish(write, &builder, apps); err != nil {
				t.Fatalf("publish select facts: %v", err)
			}
			continue
		}
		peer := snapshot.Axis[uint64, uint64]{SchemaID: schemaID, Slot: request.Slot}
		if err := snapshot.PutColumn(&builder, peer, snapshot.Content[uint64, uint64]{
			Denominator: columnDenominator(request.Slot),
			Members:     []uint64{1},
		}); err != nil {
			t.Fatalf("seal the peer column %q: %v", request.Output, err)
		}
	}
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal the publication: %v", err)
	}

	for _, fact := range apps[0].Facts.All() {
		id, idOK := channelselect.CaseFactID(fact)
		if !idOK {
			t.Fatal("accepted fact has no identity")
		}
		if _, status := snapshot.Read(&sealed, column, id); status != snapshot.ReadHit {
			t.Fatalf("accepted fact %d read as %s", fact.Ordinal, status)
		}
	}
	lookalikeID, lookalikeOK := channelselect.CaseFactID(channelselect.CaseFact{Site: apps[0].Site, Ordinal: 1})
	if !lookalikeOK {
		t.Fatal("lookalike identity unavailable")
	}
	if _, status := snapshot.Read(&sealed, column, lookalikeID); status != snapshot.ReadMiss {
		t.Fatalf("lookalike identity read as %s, want miss", status)
	}
}
