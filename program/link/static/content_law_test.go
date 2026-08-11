package static

import (
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/target"
)

func TestStaticContentObservesConstituentsAndRowsCanonically(t *testing.T) {
	base := &Component{}
	targetID, projectID := keyspace.ContentID{1}, keyspace.ContentID{2}
	want := staticContentID(targetID, projectID, base)
	replay := *base
	replay.namespaces = append([]namespaceRow(nil), base.namespaces...)
	if !want.Available() || staticContentID(targetID, projectID, &replay) != want {
		t.Fatal("Static content did not replay canonically")
	}
	if staticContentID(keyspace.ContentID{4}, projectID, &replay) == want {
		t.Fatal("Static content omitted Target constituent")
	}
	if staticContentID(targetID, keyspace.ContentID{5}, &replay) == want {
		t.Fatal("Static content omitted Project constituent")
	}
}

func TestStaticContentObservesInputKindIndependently(t *testing.T) {
	component, targetID, projectID := staticContentMutationFixture(t)
	want := staticContentID(targetID, projectID, component)
	mutated := *component
	mutated.inputs = append([]inputRow(nil), component.inputs...)
	if mutated.inputs[0].kind != InputTypeOf {
		t.Fatalf("fixture first input kind = %v, want TypeOf", mutated.inputs[0].kind)
	}
	mutated.inputs[0].kind = InputAnnotation
	if got := staticContentID(targetID, projectID, &mutated); got == want {
		t.Fatal("Static content omitted StaticInput kind")
	}
}

func TestStaticContentObservesInputTargetIndependently(t *testing.T) {
	component, targetID, projectID := staticContentMutationFixture(t)
	want := staticContentID(targetID, projectID, component)
	mutated := *component
	mutated.inputs = append([]inputRow(nil), component.inputs...)
	original := mutated.inputs[0].target
	mutated.inputs[0].target++
	if mutated.inputs[0].target == 0 || mutated.inputs[0].target == original {
		t.Fatal("failed to create independent StaticInput target mutation")
	}
	if got := staticContentID(targetID, projectID, &mutated); got == want {
		t.Fatal("Static content omitted StaticInput target")
	}
}

func TestStaticInputIDPreimageObservesKindAndTarget(t *testing.T) {
	component, _, _ := staticContentMutationFixture(t)
	baseInput, ok := component.Inputs().At(0)
	if !ok {
		t.Fatal("fixture has no StaticInput")
	}
	baseID, ok := component.Inputs().ID(baseInput)
	if !ok {
		t.Fatal("fixture StaticInput has no identity")
	}
	if component.inputs[0].kind != InputTypeOf {
		t.Fatalf("fixture first input kind = %v, want TypeOf", component.inputs[0].kind)
	}

	kindMutated := *component
	kindMutated.inputs = append([]inputRow(nil), component.inputs...)
	kindMutated.inputs[0].resolver.source = &kindMutated
	kindMutated.inputs[0].kind = InputAnnotation
	kindInput, ok := kindMutated.Inputs().At(0)
	if !ok {
		t.Fatal("kind mutation lost StaticInput")
	}
	kindID, ok := kindMutated.Inputs().ID(kindInput)
	if !ok || kindID == baseID {
		t.Fatal("StaticInput ID omitted kind preimage")
	}

	targetMutated := *component
	targetMutated.inputs = append([]inputRow(nil), component.inputs...)
	targetMutated.inputs[0].resolver.source = &targetMutated
	targetMutated.inputs[0].target++
	if targetMutated.inputs[0].target == 0 {
		t.Fatal("failed to create target mutation")
	}
	targetInput, ok := targetMutated.Inputs().At(0)
	if !ok {
		t.Fatal("target mutation lost StaticInput")
	}
	targetID, ok := targetMutated.Inputs().ID(targetInput)
	if !ok || targetID == baseID {
		t.Fatal("StaticInput ID omitted target preimage")
	}
}

func TestStaticContentObservesQualifiedConsumerShardIndependently(t *testing.T) {
	component, targetID, projectID := staticContentMutationFixture(t)
	want := staticContentID(targetID, projectID, component)
	mutated := *component
	mutated.qualified = append([]qualifiedRow(nil), component.qualified...)
	firstIndex, ok := component.mounts.Index(mutated.qualified[0].consumerShard)
	if !ok || component.mounts.Count() < 2 {
		t.Fatal("fixture lacks two mounted consumer shards")
	}
	other, ok := component.mounts.At((firstIndex + 1) % component.mounts.Count())
	if !ok || other == mutated.qualified[0].consumerShard {
		t.Fatal("fixture lacks an alternate consumer shard")
	}
	mutated.qualified[0].consumerShard = other
	if got := staticContentID(targetID, projectID, &mutated); got == want {
		t.Fatal("Static content omitted qualified consumer shard")
	}
}

func TestStaticContentRejectsInvalidIdentityRows(t *testing.T) {
	component, targetID, projectID := staticContentMutationFixture(t)
	invalidKind := *component
	invalidKind.inputs = append([]inputRow(nil), component.inputs...)
	invalidKind.inputs[0].kind = InputInvalid
	if got := staticContentID(targetID, projectID, &invalidKind); got.Available() {
		t.Fatal("Static content admitted invalid StaticInput kind")
	}
	invalidTarget := *component
	invalidTarget.inputs = append([]inputRow(nil), component.inputs...)
	invalidTarget.inputs[0].target = 0
	if got := staticContentID(targetID, projectID, &invalidTarget); got.Available() {
		t.Fatal("Static content admitted unavailable StaticInput target")
	}
	invalidConsumer := *component
	invalidConsumer.qualified = append([]qualifiedRow(nil), component.qualified...)
	invalidConsumer.qualified[0].consumerShard = linkproject.Shard{}
	if got := staticContentID(targetID, projectID, &invalidConsumer); got.Available() {
		t.Fatal("Static content admitted unavailable qualified consumer shard")
	}
}

func staticContentMutationFixture(t *testing.T) (*Component, keyspace.ContentID, keyspace.ContentID) {
	t.Helper()
	provider := lowerStaticProgram(t, "provider.lua", `
type User = { id: string }
local M = {}
M.Schema.User = User
return M
`)
	consumer := lowerStaticProgram(t, "consumer.lua", `
local value = 1
type Subject = typeof(value)
local API = require("provider")
type Qualified = API.Schema.User
`)
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	projectDraft, err := linkproject.Build(linkproject.Input{Target: contract, Modules: []linkproject.Module{
		{Name: "consumer", Program: consumer},
		{Name: "provider", Program: provider},
	}})
	if err != nil {
		t.Fatal(err)
	}
	project, err := projectDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	draft, err := Build(Input{Project: project})
	if err != nil {
		t.Fatal(err)
	}
	component, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	if len(component.inputs) == 0 || len(component.qualified) == 0 || component.mounts.Count() < 2 {
		t.Fatalf("fixture rows = inputs %d, qualified %d, mounts %d", len(component.inputs), len(component.qualified), component.mounts.Count())
	}
	return component, contract.ContentID(), project.Cold().ContentID()
}
