package link

import (
	"bytes"
	"encoding/binary"
	"errors"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkhost "github.com/wippyai/go-lua/program/link/host"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/target"
)

func artifactProgramPool(programs ...*program.Program) map[keyspace.ContentID]*program.Program {
	pool := make(map[keyspace.ContentID]*program.Program, len(programs))
	for _, item := range programs {
		if item != nil && item.ContentID().Available() {
			pool[item.ContentID()] = item
		}
	}
	return pool
}

func TestArtifactHostReplayWireHasNoHotCoordinates(t *testing.T) {
	term := reflect.TypeOf(keyspace.Term(0))
	operation := reflect.TypeOf(target.Operation(0))
	formal := reflect.TypeOf(target.ValueFormal(0))
	for _, item := range []reflect.Type{
		reflect.TypeOf(linkhost.ReplaySpec{}),
		reflect.TypeOf(linkhost.ReplayCapabilitySeed{}),
		reflect.TypeOf(linkhost.ReplaySelector{}),
		reflect.TypeOf(linkhost.ReplayMemberSelector{}),
	} {
		for field := 0; field < item.NumField(); field++ {
			got := item.Field(field)
			if got.Type == term || got.Type == operation || got.Type == formal {
				t.Fatalf("portable Host replay field %s leaks hot coordinate %v", got.Name, got.Type)
			}
		}
	}
	seed := reflect.TypeOf(linkhost.ReplayCapabilitySeed{})
	for _, name := range []string{"Capability", "Source", "InitialRoot", "InputFormal", "OutcomeResult", "Value"} {
		if _, ok := seed.FieldByName(name); !ok {
			t.Fatalf("portable Host replay seed missing %s", name)
		}
	}
	for _, forbidden := range []string{"Formal", "Outcome", "Result", "Access", "Module", "Binding"} {
		if _, ok := seed.FieldByName(forbidden); ok {
			t.Fatalf("portable Host replay seed retains hot field %s", forbidden)
		}
	}
}

func TestArtifactReplaysDetachedHostReferencesAndRejectsUnknownID(t *testing.T) {
	linked, sealed, program, _, _, _ := capabilityFixture(t, false)
	replay, ok := linked.Host().Cold().ReplaySpec()
	if !ok {
		t.Fatal("sealed Host replay contract unavailable")
	}
	data, err := EncodeArtifact(linked)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := DecodeArtifact(data, sealed, artifactProgramPool(program))
	if err != nil {
		t.Fatal(err)
	}
	got, ok := replayed.Host().Cold().ReplaySpec()
	if !ok || !reflect.DeepEqual(got, replay) {
		t.Fatalf("artifact Host replay changed detached contract: %#v/%t want %#v", got, ok, replay)
	}
	var targetID keyspace.ContentID
	for _, seed := range replay.Seeds {
		if seed.InputFormal.Available() {
			targetID = seed.InputFormal
			break
		}
		if seed.OutcomeResult.Available() {
			targetID = seed.OutcomeResult
			break
		}
		if seed.Value.Available() {
			targetID = seed.Value
			break
		}
	}
	if !targetID.Available() {
		t.Fatal("fixture has no Host detached reference")
	}
	corrupt := artifactReplaceContentID(t, data, targetID)
	if got, err := DecodeArtifact(corrupt, sealed, artifactProgramPool(program)); got != nil || !errors.Is(err, ErrArtifactCanonical) {
		t.Fatalf("unknown Host detached ID = %v/%v", got, err)
	}
	if got, err := DecodeArtifact(artifactReplaceFirstCount(t, data), sealed, artifactProgramPool(program)); got != nil || !errors.Is(err, ErrArtifactCanonical) {
		t.Fatalf("malformed artifact count = %v/%v", got, err)
	}
}

// artifactReplaceContentID changes one fixed-width canonical Bytes payload
// without changing framing. It therefore reaches replay-reference admission,
// rather than merely proving malformed-byte rejection.
func artifactReplaceContentID(t *testing.T, data []byte, want keyspace.ContentID) []byte {
	t.Helper()
	result := append([]byte(nil), data...)
	for offset := 0; offset < len(result); {
		if offset+1 > len(result) {
			break
		}
		tag := result[offset]
		offset++
		length, size := binary.Uvarint(result[offset:])
		if size <= 0 || length > uint64(len(result)-offset-size) {
			t.Fatal("malformed encoded artifact")
		}
		offset += size
		end := offset + int(length)
		if tag == 7 && length == uint64(len(want)) && bytes.Equal(result[offset:end], want[:]) {
			result[end-1] ^= 1
			return result
		}
		offset = end
	}
	t.Fatal("Host detached ContentID absent from artifact")
	return nil
}

func artifactReplaceFirstCount(t *testing.T, data []byte) []byte {
	t.Helper()
	result := append([]byte(nil), data...)
	for offset := 0; offset < len(result); {
		tag := result[offset]
		offset++
		length, size := binary.Uvarint(result[offset:])
		if size <= 0 || length > uint64(len(result)-offset-size) {
			t.Fatal("malformed encoded artifact")
		}
		offset += size
		end := offset + int(length)
		if tag == 4 && length == 1 && result[offset] < 0x7f {
			result[offset]++
			return result
		}
		offset = end
	}
	t.Fatal("artifact has no mutable count")
	return nil
}

func artifactAssertProjectionRoundTrip(t *testing.T, l *Link, sealed *target.Contract, programs ...*program.Program) *Link {
	t.Helper()
	data, err := EncodeArtifact(l)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := DecodeArtifact(data, sealed, artifactProgramPool(programs...))
	if err != nil {
		t.Fatal(err)
	}
	if replayed.ContentID() != l.ContentID() {
		t.Fatal("artifact round trip changed Link identity")
	}
	return replayed
}

func TestArtifactRoundTripAndTamperRejection(t *testing.T) {
	binding := target.BindingSpec{Namespace: target.BindingProvider, Owner: []string{"actor"}, Member: []string{"send"}}
	sealed := contract(t, binding)
	p := source(t, ``)
	l, err := Seal(&Spec{Target: sealed, Modules: []linkproject.Module{{Name: "main", Program: p}}, EndpointRequests: []linkboundary.EndpointRequest{{Identity: "actor.send", Binding: binding}}})
	if err != nil {
		t.Fatal(err)
	}
	if replayed := artifactAssertProjectionRoundTrip(t, l, sealed, p); replayed.Boundary().Endpoints().Count() != 1 {
		t.Fatalf("replayed endpoint count = %d, want 1", replayed.Boundary().Endpoints().Count())
	}
	data, err := EncodeArtifact(l)
	if err != nil {
		t.Fatal(err)
	}
	wrong := contract(t, target.BindingSpec{Namespace: target.BindingProvider, Owner: []string{"actor"}, Member: []string{"other"}})
	if got, err := DecodeArtifact(data, wrong, artifactProgramPool(p)); got != nil || !errors.Is(err, ErrArtifactTarget) {
		t.Fatalf("wrong target = %v/%v", got, err)
	}
	tampered := append([]byte(nil), data...)
	tampered[len(tampered)-1] ^= 1
	if got, err := DecodeArtifact(tampered, sealed, artifactProgramPool(p)); got != nil || err == nil {
		t.Fatalf("tampered artifact = %v/%v", got, err)
	}
	if got, err := DecodeArtifact(append(data, 0), sealed, artifactProgramPool(p)); got != nil || !errors.Is(err, ErrArtifactCanonical) {
		t.Fatalf("noncanonical artifact = %v/%v", got, err)
	}
}

// TestArtifactReplayPreservesProjectCallInverse proves that Project's
// source-occurrence inverse is rebuilt from the portable Program mount during
// artifact replay. The hot Shard/Application handles are intentionally new,
// while their exact stable Application identity and source grounding remain
// unchanged.
func TestArtifactReplayPreservesProjectCallInverse(t *testing.T) {
	sealed := contract(t)
	p := source(t, `require("dependency")`)
	linked := linked(t, sealed, linkproject.Module{Name: "main", Program: p})
	shard := onlyProjectShardFor(t, linked, p)
	term := call(t, p, 0)
	original, originalOK := linked.Project().Applications().Calls().ForCall(shard, p, term)
	if !originalOK {
		t.Fatal("original Project Call inverse unavailable")
	}
	originalID, originalIDOK := linked.Project().ApplicationID(original)
	if !originalIDOK {
		t.Fatal("original Project Application identity unavailable")
	}
	data, err := EncodeArtifact(linked)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := DecodeArtifact(data, sealed, artifactProgramPool(p))
	if err != nil {
		t.Fatal(err)
	}
	replayedShard := onlyProjectShardFor(t, replayed, p)
	rebound, reboundOK := replayed.Project().Applications().Calls().ForCall(replayedShard, p, term)
	if !reboundOK {
		t.Fatal("replayed Project Call inverse unavailable")
	}
	reboundID, reboundIDOK := replayed.Project().ApplicationID(rebound)
	if !reboundIDOK || reboundID != originalID {
		t.Fatalf("replayed Project Application identity = %x/%v, want %x", reboundID, reboundIDOK, originalID)
	}
	foreign := source(t, `require("dependency")`)
	if _, ok := replayed.Project().Applications().Calls().ForCall(replayedShard, foreign, term); ok {
		t.Fatal("replayed Project accepted equal-term foreign Program")
	}
}
