package index

import (
	"testing"

	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

// TestTopologyWriteAccessAdmitsOnlySealedWriteGeometry keeps the first write
// law deliberately cold: Access exposes the existing Heap row and payload,
// while a read row can never enter the indexed-write Rule operand surface.
func TestTopologyWriteAccessAdmitsOnlySealedWriteGeometry(t *testing.T) {
	program, err := lower.Lower(lower.Source{
		Name: "raw_set_geometry.lua",
		Text: []byte(`local id = {}; local tags = {}; tags["source"] = id; return tags`),
	})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	heapSchema, heapOK := heapdomain.Seal(linked)
	valueSchema, valueOK := valuedomain.Seal(linked, heapSchema)
	callAlgebra, callOK := calldomain.New(linked)
	topology, topologyOK := Seal(heapSchema, valueSchema, callAlgebra)
	if !heapOK || !valueOK || !callOK || !topologyOK {
		t.Fatal("write geometry schemas")
	}
	var reads, writes int
	for index := 0; index < heapSchema.IndexAccessCount(); index++ {
		candidate, candidateOK := heapSchema.IndexAccessAt(index)
		access, accessOK := topology.Access(candidate)
		if !candidateOK || !accessOK {
			t.Fatalf("IndexAccess %d", index)
		}
		if access.Read() {
			reads++
			if access.Write() {
				t.Fatal("read access also admitted as write")
			}
			if _, payloadOK := heapSchema.PayloadForIndexAccess(candidate); payloadOK {
				t.Fatal("read access retained a write payload")
			}
			continue
		}
		if !access.Write() {
			t.Fatal("non-read index row was neither read nor write")
		}
		writes++
		if _, resultOK := access.Result(); resultOK {
			t.Fatal("write access exposed a read result coordinate")
		}
		if _, payloadOK := heapSchema.PayloadForIndexAccess(candidate); !payloadOK {
			t.Fatal("write access lost its sealed RHS payload")
		}
	}
	if writes == 0 {
		t.Fatalf("write geometry rows reads=%d writes=%d", reads, writes)
	}
}
