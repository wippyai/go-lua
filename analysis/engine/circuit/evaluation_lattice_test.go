package circuit

import "testing"

func TestEvalLatticeHasExplicitBottomConcreteTopOrder(t *testing.T) {
	partition, err := NewBindingPartitionPolicy(1, []ClassID{"apply"}, []ClassID{"target"}, []ClassID{"prov"}, []ClassID{"alias"})
	if err != nil {
		t.Fatal(err)
	}
	key, err := partition.Partition(PartitionInput{Application: "apply", Target: "target", Provenance: "prov", Alias: "alias"})
	if err != nil {
		t.Fatal(err)
	}
	precision, err := NewPrecisionPolicy(1, 2, "binding-top", []GuardID{"a", "b"}, []GuardID{"p1", "p2"}, []GuardID{"x", "y"})
	if err != nil {
		t.Fatal(err)
	}
	domain, err := NewBindingDomain(partition, precision, nil, func(w BindingID, members []BindingID) bool { return w == "binding-top" && len(members) > 0 })
	if err != nil {
		t.Fatal(err)
	}
	makeDisjunct := func(a, p, l GuardID, b BindingID) Disjunct {
		ag, _ := NewGuardSet(a)
		pg, _ := NewGuardSet(p)
		lg, _ := NewGuardSet(l)
		d, e := NewDisjunct(ag, pg, lg, b)
		if e != nil {
			t.Fatal(e)
		}
		return d
	}
	one, _ := domain.Singleton(key, makeDisjunct("a", "p1", "x", "one"))
	two, _ := domain.Singleton(key, makeDisjunct("b", "p2", "y", "two"))
	joined, _, err := domain.Join(one, two)
	if err != nil {
		t.Fatal(err)
	}
	widens := 0
	var evalErr error
	lattice := newEvalLattice(domain, &widens, &evalErr)
	bottom, top := lattice.Bottom(), lattice.Top()
	left, both := evalValue{kind: evalConcrete, cell: one}, evalValue{kind: evalConcrete, cell: joined}
	if !lattice.Equal(bottom, bottom) || !lattice.Equal(top, top) || !lattice.Equal(left, left) || lattice.Equal(bottom, top) || lattice.Equal(top, left) {
		t.Fatal("eval equality does not distinguish bottom/concrete/top")
	}
	if !lattice.LessOrEq(bottom, left) || !lattice.LessOrEq(left, top) || !lattice.LessOrEq(left, both) || lattice.LessOrEq(top, left) || lattice.LessOrEq(left, bottom) {
		t.Fatal("eval partial order is invalid")
	}
	if !lattice.Equal(lattice.Join(bottom, left), left) || !lattice.Equal(lattice.Join(top, left), top) || !lattice.Equal(lattice.Widen(left, top), top) {
		t.Fatal("eval join/widen top-bottom laws are invalid")
	}
	if evalErr != nil {
		t.Fatal(evalErr)
	}
}
