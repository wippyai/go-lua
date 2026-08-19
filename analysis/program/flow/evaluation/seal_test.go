package evaluation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestPortsLeafQueriesAreAllocationFree(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	nilTerm := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	counts := [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1, keyspace.FamilyNil: 1, keyspace.FamilyValues: 1, keyspace.FamilyReturn: 1}
	fixture := openPortsFixture(t, counts, authored.Input{
		Values:  authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}}, Terms: []keyspace.Term{nilTerm}},
		Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values}}},
	}, 1)
	defer fixture.close()
	ports, err := SealPorts(fixture.identity, fixture.view, fixture.forest,
		fixture.staticView.ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatal(err)
	}
	if entry, ok := ports.Entry(nilTerm); !ok || entry != nilTerm {
		t.Fatalf("Entry(nil) = %v, %v", entry, ok)
	}
	if finish, ok := ports.Finish(nilTerm); !ok || finish != nilTerm {
		t.Fatalf("Finish(nil) = %v, %v", finish, ok)
	}
	key := keyspace.MakeTerm(keyspace.FamilyKey, 1)
	if _, ok := ports.Entry(key); ok {
		t.Fatal("static Key unexpectedly received an Entry port")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		_, _ = ports.Entry(nilTerm)
		_, _ = ports.Finish(nilTerm)
	}); allocations != 0 {
		t.Fatalf("port queries allocate %v times", allocations)
	}
}
