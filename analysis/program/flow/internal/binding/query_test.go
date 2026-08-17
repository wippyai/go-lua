package binding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestResultQueriesFailClosedForZeroForeignAndMalformedTerms(t *testing.T) {
	var zero Result
	terms := []keyspace.Term{
		0,
		keyspace.MakeTerm(keyspace.FamilyBind, 1),
		keyspace.MakeTerm(keyspace.FamilyCell, 0),
		keyspace.MakeTerm(keyspace.FamilyCell, 1),
	}
	for _, term := range terms {
		if role, ok := zero.Role(term); ok || role != 0 {
			t.Fatalf("zero.Role(%v) = %v/%v", term, role, ok)
		}
		if host, ok := zero.Host(term); ok || host != 0 {
			t.Fatalf("zero.Host(%v) = %v/%v", term, host, ok)
		}
	}
	if zero.CellCount() != 0 {
		t.Fatalf("zero.CellCount = %d", zero.CellCount())
	}
	if chunk, ok := zero.ChunkVararg(); ok || chunk != 0 {
		t.Fatalf("zero.ChunkVararg = %v/%v", chunk, ok)
	}

	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	malformed := Result{
		sourceID: bindingTestSourceID(), flowID: bindingTestFlowID(),
		roles: []kind.CellRole{0, kind.CellGlobal}, hosts: []keyspace.Term{0}, chunk: cell,
	}
	if role, ok := malformed.Role(cell); ok || role != 0 {
		t.Fatalf("malformed.Role = %v/%v", role, ok)
	}
	if host, ok := malformed.Host(cell); ok || host != 0 {
		t.Fatalf("malformed.Host = %v/%v", host, ok)
	}
	if chunk, ok := malformed.ChunkVararg(); ok || chunk != 0 {
		t.Fatalf("malformed.ChunkVararg = %v/%v", chunk, ok)
	}
}

func TestResultGlobalHostIsZeroAndPresent(t *testing.T) {
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1, keyspace.FamilyCell: 1}}
	input.Storage.Cells = []authored.Cell{{Kind: authored.CellGlobal, Key: 1}}
	result, finish := sealLawFixture(t, input, [][]keyspace.Term{{}}, nil, nil,
		[]keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "global"}})
	defer finish()
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	if role, ok := result.Role(cell); !ok || role != kind.CellGlobal {
		t.Fatalf("Role(global) = %v/%v", role, ok)
	}
	if host, ok := result.Host(cell); !ok || host != 0 {
		t.Fatalf("Host(global) = %v/%v, want 0/true", host, ok)
	}
}
