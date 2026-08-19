package authored

import "testing"

func TestDenseFlowTwoPhaseIdentityAndQueries(t *testing.T) {
	authored, terms := flowFixture()
	draft, err := Build(authored)
	if err != nil {
		t.Fatal(err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	component, err := finalizer.Commit()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := draft.Finalizer(); err == nil {
		t.Fatal("draft taken twice")
	}
	if !component.Cold().ContentID().Available() {
		t.Fatal("Flow identity unavailable")
	}
	if got, ok := component.Values().Member(terms.values, 0); !ok || got != terms.nil {
		t.Fatalf("Values member = %08x, %v", uint32(got), ok)
	}
	if position, ok := component.Values().Position(terms.values, 7); !ok || !position.NilFill {
		t.Fatalf("Values nil adjustment = %#v, %v", position, ok)
	}
	if field, ok := component.Tables().FieldAt(terms.table, 0); !ok || field != terms.field {
		t.Fatalf("Table field = %08x, %v", uint32(field), ok)
	}
	if values, open, ok := component.Fields().Values(terms.field); !ok || open || values != terms.values {
		t.Fatalf("TableField Values = %08x, %v, %v", uint32(values), open, ok)
	}
}
