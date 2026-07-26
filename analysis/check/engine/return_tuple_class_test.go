package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/module/exportrelation"
)

func scalarTuple(scalars ...string) exportrelation.ReturnTuple {
	values := make([]exportrelation.Value, 0, len(scalars))
	present := make([]bool, 0, len(scalars))
	for _, scalar := range scalars {
		values = append(values, exportrelation.Value{Scalar: scalar})
		present = append(present, scalar != "" && scalar != "scalar/nil")
	}
	return exportrelation.ReturnTuple{Values: values, Present: present}
}

// TestTriggerClassesComeFromTheCatalog pins that the classes a correlation may
// be stated over are read off the returns themselves. A producer whose trigger
// slot returns a value no rule anticipated still gets that value's own class.
func TestTriggerClassesComeFromTheCatalog(t *testing.T) {
	classes := returnTupleTriggerClasses([]exportrelation.ReturnTuple{
		scalarTuple("scalar/bool/false", "scalar/string/x"),
		scalarTuple("scalar/string/retry", "scalar/nil"),
	}, 0)
	want := map[returnTupleClass]bool{
		returnTupleLiteralClass("scalar/bool/false"):   false,
		returnTupleLiteralClass("scalar/string/retry"): false,
		returnTupleClassTruthy:                         false,
		returnTupleClassNotNil:                         false,
	}
	for _, class := range classes {
		if _, expected := want[class]; !expected {
			t.Errorf("catalog produced unexpected class %q", class)
			continue
		}
		want[class] = true
	}
	for class, seen := range want {
		if !seen {
			t.Errorf("class %q was not derived from the catalog", class)
		}
	}
}

// TestTruthyClassCoversEveryTruthyReturn pins the soundness the class keying
// buys. A catalog that returns a truthy value other than `true` without the
// companion slot refutes a truthiness correlation, even though it never returns
// `true` itself — a value-keyed family would never have looked at that tuple.
func TestTruthyClassCoversEveryTruthyReturn(t *testing.T) {
	truthy := returnTupleClassTruthy
	for _, item := range []struct {
		scalar string
		holds  bool
	}{
		{"scalar/bool/true", true},
		{"scalar/number/1", true},
		{"scalar/string/x", true},
		{"scalar/bool/false", false},
		{"scalar/nil", false},
	} {
		holds, decided := truthy.contains(exportrelation.Value{Scalar: item.scalar}, item.scalar != "scalar/nil")
		if !decided {
			t.Errorf("truthiness of %s was left undecided by an exact scalar", item.scalar)
			continue
		}
		if holds != item.holds {
			t.Errorf("truthiness of %s = %v, want %v", item.scalar, holds, item.holds)
		}
	}
}

// TestOccupiedSlotDecidesOnlyTheNilClasses pins the three-valued reading. A slot
// the export proves occupied is known not to be nil, which decides both classes
// that turn on nil; it does not decide truthiness, because an occupied slot may
// hold false.
func TestOccupiedSlotDecidesOnlyTheNilClasses(t *testing.T) {
	unstated := exportrelation.Value{}
	if holds, decided := returnTupleClassNotNil.contains(unstated, true); !decided || !holds {
		t.Errorf("an occupied slot did not decide the nil-presence class: holds=%v decided=%v", holds, decided)
	}
	if holds, decided := returnTupleLiteralClass("scalar/nil").contains(unstated, true); !decided || holds {
		t.Errorf("an occupied slot did not fall outside the nil singleton: holds=%v decided=%v", holds, decided)
	}
	if _, decided := returnTupleClassTruthy.contains(unstated, true); decided {
		t.Error("an occupied slot decided truthiness, but it may hold false")
	}
	if _, decided := returnTupleClassNotNil.contains(unstated, false); decided {
		t.Error("an unoccupied, unstated slot decided the nil-presence class")
	}
}

// TestBranchClassNamesTheSetThePredicateDecides pins the consumer half: the
// predicate families that decide a value set map onto the sets the catalog
// publishes, and negation is left to the edge rather than changing the set.
func TestBranchClassNamesTheSetThePredicateDecides(t *testing.T) {
	for _, item := range []struct {
		predicate branchPredicateWire
		want      returnTupleClass
	}{
		{branchPredicateWire{Kind: "nil"}, returnTupleLiteralClass("scalar/nil")},
		{branchPredicateWire{Kind: "nil", Negated: true}, returnTupleLiteralClass("scalar/nil")},
		{branchPredicateWire{Kind: "not-nil"}, returnTupleClassNotNil},
		{branchPredicateWire{Kind: "truthy"}, returnTupleClassTruthy},
		{branchPredicateWire{Kind: "literal-equal", Literal: "scalar/bool/false"}, returnTupleLiteralClass("scalar/bool/false")},
	} {
		class, decides := returnTupleBranchClass(item.predicate)
		if !decides || class != item.want {
			t.Errorf("predicate %q decided class %q (%v), want %q", item.predicate.Kind, class, decides, item.want)
		}
	}
	for _, predicate := range []branchPredicateWire{
		{Kind: "literal-equal"},
		{Kind: "len-ge"},
		{Kind: "type-equal", TypeName: "string"},
	} {
		if _, decides := returnTupleBranchClass(predicate); decides {
			t.Errorf("predicate %q decided a correlation class it does not name", predicate.Kind)
		}
	}
}

// TestPublishedClassTokensAreRecognized pins that the transport admits exactly
// the tokens this family defines, so a class added at the producer travels
// without a second list having to learn its name.
func TestPublishedClassTokensAreRecognized(t *testing.T) {
	for _, class := range returnTupleTriggerClasses([]exportrelation.ReturnTuple{
		scalarTuple("scalar/nil", "scalar/string/x"),
		scalarTuple("scalar/string/e", "scalar/nil"),
	}, 0) {
		if !class.stated() {
			t.Errorf("class %q is published but the transport does not recognize it", class)
		}
	}
	for _, token := range []returnTupleClass{"", "proven", "is/"} {
		if token.stated() {
			t.Errorf("token %q is not a class this family defines", token)
		}
	}
}
