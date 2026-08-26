package program

import "testing"

// TestSummaryHasExactlyOneAddressingAuthority keeps the three whole-vector
// addressings mutually exclusive. A predicate is a selected span, Parent is a
// nested member-set span, and KeyVector is a directory-published span; a
// Summary cannot borrow two denominators or invent one by declaring none.
func TestSummaryHasExactlyOneAddressingAuthority(t *testing.T) {
	for _, test := range []struct {
		name                         string
		predicate, parent, keyVector bool
		want                         bool
	}{
		{name: "predicate", predicate: true, want: true},
		{name: "parent", parent: true, want: true},
		{name: "key-vector", keyVector: true, want: true},
		{name: "none", want: false},
		{name: "predicate-and-parent", predicate: true, parent: true, want: false},
		{name: "predicate-and-key-vector", predicate: true, keyVector: true, want: false},
		{name: "parent-and-key-vector", parent: true, keyVector: true, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := ReadFormAddressing(Summary, test.predicate, test.parent, test.keyVector); got != test.want {
				t.Fatalf("Summary addressing predicate=%t parent=%t keyVector=%t = %t, want %t", test.predicate, test.parent, test.keyVector, got, test.want)
			}
		})
	}
}

// TestProgramRejectsSummaryWithParentAndKeyVector proves the declaration
// gate asks the same one-addressing law rather than accepting a dual span and
// leaving a downstream compiler to choose which owner won.
func TestProgramRejectsSummaryWithParentAndKeyVector(t *testing.T) {
	program := lawProgram(t)
	join := &program.Joins[0]
	join.Read = lawRead(Summary, "dual-summary", false)
	join.Parent = lawRelation("dual-summary/parent")
	join.KeyVector = lawRelation("dual-summary/key-vector")
	if problem, valid := program.Check(); valid {
		t.Fatalf("dual Parent+KeyVector summary admitted: %+v", problem)
	}
}
