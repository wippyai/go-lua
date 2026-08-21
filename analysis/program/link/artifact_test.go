package link_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/link"
	linkartifact "github.com/wippyai/go-lua/analysis/program/link/artifact"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	linkboundary "github.com/wippyai/go-lua/analysis/program/link/boundary"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	contractvalue "github.com/wippyai/go-lua/analysis/program/target/contract"
)

func artifactProgramPool(programs ...*program.Program) map[identity.ContentID]*program.Program {
	pool := make(map[identity.ContentID]*program.Program, len(programs))
	for _, item := range programs {
		if item != nil && item.ContentID().Available() {
			pool[item.ContentID()] = item
		}
	}
	return pool
}

func TestArtifactReplaysDetachedHostReferencesAndRejectsUnknownID(t *testing.T) {
	linked, sealed, program, _, _, _ := capabilityFixture(t, false)
	replay, ok := linked.Host().Cold().ReplaySpec()
	if !ok {
		t.Fatal("sealed Host replay contract unavailable")
	}
	data, err := linkartifact.Encode(linked)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := linkartifact.Decode(data, sealed, artifactProgramPool(program))
	if err != nil {
		t.Fatal(err)
	}
	got, ok := replayed.Host().Cold().ReplaySpec()
	if !ok || !reflect.DeepEqual(got, replay) {
		t.Fatalf("artifact Host replay changed detached contract: %#v/%t want %#v", got, ok, replay)
	}
	var targetID identity.ContentID
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
	if got, err := linkartifact.Decode(corrupt, sealed, artifactProgramPool(program)); got != nil || !errors.Is(err, linkartifact.ErrCanonical) {
		t.Fatalf("unknown Host detached ID = %v/%v", got, err)
	}
	if got, err := linkartifact.Decode(artifactReplaceFirstCount(t, data), sealed, artifactProgramPool(program)); got != nil || !errors.Is(err, linkartifact.ErrCanonical) {
		t.Fatalf("malformed artifact count = %v/%v", got, err)
	}
}

// artifactReplaceContentID changes one fixed-width canonical Bytes payload
// without changing framing. It therefore reaches replay-reference admission,
// rather than merely proving malformed-byte rejection.
func artifactReplaceContentID(t *testing.T, data []byte, want identity.ContentID) []byte {
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

func artifactAssertProjectionRoundTrip(t *testing.T, l *link.Link, sealed *contractvalue.Contract, programs ...*program.Program) *link.Link {
	t.Helper()
	data, err := linkartifact.Encode(l)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := linkartifact.Decode(data, sealed, artifactProgramPool(programs...))
	if err != nil {
		t.Fatal(err)
	}
	if replayed.ContentID() != l.ContentID() {
		t.Fatal("artifact round trip changed Link identity")
	}
	return replayed
}

func TestArtifactRoundTripAndTamperRejection(t *testing.T) {
	binding := vocabulary.BindingSpec{Namespace: vocabulary.BindingProvider, Owner: []string{"actor"}, Member: []string{"send"}}
	sealed := contract(t, binding)
	p := source(t, ``)
	l, err := link.Seal(&link.Spec{Target: sealed, Modules: []linkproject.Module{{Name: "main", Program: p}}, EndpointRequests: []linkboundary.EndpointRequest{{Identity: "actor.send", Binding: binding}}})
	if err != nil {
		t.Fatal(err)
	}
	if replayed := artifactAssertProjectionRoundTrip(t, l, sealed, p); replayed.Boundary().Endpoints().Count() != 1 {
		t.Fatalf("replayed endpoint count = %d, want 1", replayed.Boundary().Endpoints().Count())
	}
	data, err := linkartifact.Encode(l)
	if err != nil {
		t.Fatal(err)
	}
	wrong := contract(t, vocabulary.BindingSpec{Namespace: vocabulary.BindingProvider, Owner: []string{"actor"}, Member: []string{"other"}})
	if got, err := linkartifact.Decode(data, wrong, artifactProgramPool(p)); got != nil || !errors.Is(err, linkartifact.ErrTarget) {
		t.Fatalf("wrong target = %v/%v", got, err)
	}
	tampered := append([]byte(nil), data...)
	tampered[len(tampered)-1] ^= 1
	if got, err := linkartifact.Decode(tampered, sealed, artifactProgramPool(p)); got != nil || err == nil {
		t.Fatalf("tampered artifact = %v/%v", got, err)
	}
	if got, err := linkartifact.Decode(append(data, 0), sealed, artifactProgramPool(p)); got != nil || !errors.Is(err, linkartifact.ErrCanonical) {
		t.Fatalf("noncanonical artifact = %v/%v", got, err)
	}
}

// TestArtifactReplayPreservesProjectCallIdentity proves that Project's
// mounted scalar call identity is rebuilt from the portable Program mount
// during artifact replay. The hot Shard/Application handles are intentionally
// new, while their exact stable Application identity and source grounding
// remain unchanged.
func TestArtifactReplayPreservesProjectCallIdentity(t *testing.T) {
	sealed := contract(t)
	p := source(t, `require("dependency")`)
	dependency := source(t, `return 1`)
	linked := linked(t, sealed,
		linkproject.Module{Name: "main", Program: p},
		linkproject.Module{Name: "dependency", Program: dependency},
	)
	term := call(t, p, 0)
	originalApplication, originalApplicationOK := callApplicationForTerm(t, linked, term)
	if !originalApplicationOK {
		t.Fatal("original Project Application unavailable")
	}
	originalProof, originalProofOK := linked.Project().Applications().Calls().ForApplication(originalApplication)
	original, originalOK := originalProof.Application()
	if !originalProofOK || !originalOK {
		t.Fatal("original Project Call inverse unavailable")
	}
	originalCallID := originalProof.CallID()
	if !originalCallID.Available() {
		t.Fatal("original Project scalar call identity unavailable")
	}
	originalID, originalIDOK := linked.Project().ApplicationID(original)
	if !originalIDOK {
		t.Fatal("original Project Application identity unavailable")
	}
	data, err := linkartifact.Encode(linked)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := linkartifact.Decode(data, sealed, artifactProgramPool(p, dependency))
	if err != nil {
		t.Fatal(err)
	}
	reboundApplication, reboundApplicationOK := callApplicationForTerm(t, replayed, term)
	if !reboundApplicationOK {
		t.Fatal("replayed Project Application unavailable")
	}
	reboundProof, reboundProofOK := replayed.Project().Applications().Calls().ForApplication(reboundApplication)
	rebound, reboundOK := reboundProof.Application()
	if !reboundProofOK || !reboundOK {
		t.Fatal("replayed Project Call inverse unavailable")
	}
	if reboundProof.CallID() != originalCallID {
		t.Fatalf("replayed Project scalar call identity = %x, want %x", reboundProof.CallID(), originalCallID)
	}
	reboundID, reboundIDOK := replayed.Project().ApplicationID(rebound)
	if !reboundIDOK || reboundID != originalID {
		t.Fatalf("replayed Project Application identity = %x/%v, want %x", reboundID, reboundIDOK, originalID)
	}
}

func callApplicationForTerm(t testing.TB, linked *link.Link, term keyspace.Term) (linkproject.Application, bool) {
	t.Helper()
	applications := linked.Project().Applications()
	calls := applications.Calls()
	for index := 0; index < calls.Count(); index++ {
		application, ok := calls.At(index)
		if !ok {
			continue
		}
		_, gotTerm, callOK := applications.Call(application)
		if callOK && gotTerm == term {
			return application, true
		}
	}
	return linkproject.Application{}, false
}
