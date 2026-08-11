package cutplan

import (
	"strings"
	"testing"
)

func TestDisjointOperationsHaveOrderIndependentDigest(t *testing.T) {
	left := Intent{Schema: Version, Name: "parallel", Operations: []Operation{criticalOperation("b", "old-b", "new-b", "internal/b.go"), criticalOperation("a", "old-a", "new-a", "internal/a.go")}}
	right := Intent{Schema: Version, Name: "parallel", Operations: []Operation{criticalOperation("a", "old-a", "new-a", "internal/a.go"), criticalOperation("b", "old-b", "new-b", "internal/b.go")}}
	leftDigest, err := IntentDigest(left)
	if err != nil {
		t.Fatal(err)
	}
	rightDigest, err := IntentDigest(right)
	if err != nil {
		t.Fatal(err)
	}
	if leftDigest != rightDigest {
		t.Fatalf("disjoint operation order changed digest: %s != %s", leftDigest, rightDigest)
	}
}

func TestUnorderedReadWriteOverlapIsRejected(t *testing.T) {
	left := criticalOperation("a", "old-a", "new-a", "internal/shared.go")
	right := criticalOperation("b", "old-b", "new-b", "internal/b.go")
	consumePath(t, &right, "internal/shared.go", "internal/b.generated.go")
	intent := Intent{Schema: Version, Name: "overlap", Operations: []Operation{left, right}}
	if err := ValidateIntent(intent); err == nil || !strings.Contains(err.Error(), "unordered operations") || !strings.Contains(err.Error(), "read/write footprint") {
		t.Fatalf("unordered overlap accepted or misreported: %v", err)
	}
}

func TestOrderedReadAfterWriteRequiresAuthorityChain(t *testing.T) {
	first := criticalOperation("extract", "link", "flow", "internal/flow.go")
	second := criticalOperation("consume", "flow", "rules", "internal/rules.go")
	second.After = []string{"extract"}
	consumePath(t, &second, "internal/flow.go", "internal/rules.generated.go")
	intent := Intent{Schema: Version, Name: "chain", Operations: []Operation{second, first}}
	if err := ValidateIntent(intent); err != nil {
		t.Fatalf("lawful ordered consume rejected: %v", err)
	}
	second.Authority.From = "other"
	if err := ValidateIntent(Intent{Schema: Version, Name: "broken-chain", Operations: []Operation{first, second}}); err == nil || !strings.Contains(err.Error(), "authority chain") {
		t.Fatalf("unowned consume accepted: %v", err)
	}
}

func consumePath(t *testing.T, operation *Operation, input, destination string) {
	t.Helper()
	operation.Edits = append(operation.Edits, Edit{Kind: EditGenerate, Generate: &Generate{
		Provider: "fixture-generator", Inputs: []string{input}, Destination: destination,
	}})
	operation.Footprint.Read = append(operation.Footprint.Read, input)
	operation.Footprint.Write = append(operation.Footprint.Write, destination)
}

func TestOrderedDoubleWriteIsRejected(t *testing.T) {
	first := criticalOperation("one", "old", "mid", "internal/shared.go")
	second := criticalOperation("two", "mid", "new", "internal/shared.go")
	second.After = []string{"one"}
	if err := ValidateIntent(Intent{Schema: Version, Name: "double-write", Operations: []Operation{first, second}}); err == nil || !strings.Contains(err.Error(), "both write") {
		t.Fatalf("ordered double write accepted: %v", err)
	}
}

func TestAfterRejectsCycleAndUnknownOperation(t *testing.T) {
	left := criticalOperation("a", "old-a", "new-a", "internal/a.go")
	right := criticalOperation("b", "old-b", "new-b", "internal/b.go")
	left.After, right.After = []string{"b"}, []string{"a"}
	if err := ValidateIntent(Intent{Schema: Version, Name: "cycle", Operations: []Operation{left, right}}); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle accepted: %v", err)
	}
	left.After, right.After = []string{"missing"}, nil
	if err := ValidateIntent(Intent{Schema: Version, Name: "unknown", Operations: []Operation{left, right}}); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("unknown dependency accepted: %v", err)
	}
}

func criticalOperation(id, from, to, path string) Operation {
	old := SymbolRef{Object: "example.invalid/" + from + "#package:" + strings.ToUpper(id)}
	new := SymbolRef{Object: "example.invalid/" + to + "#package:New" + strings.ToUpper(id)}
	return Operation{
		ID: id, Authority: Authority{From: from, To: to},
		Edits: []Edit{{Kind: EditRelocate, Relocate: &Relocate{
			Source: path, Destination: Destination{Path: path + ".next", Package: "internal"}, Subjects: []Relocation{{From: old, To: new}},
		}}},
		Footprint: Footprint{Read: []string{path}, Write: []string{path, path + ".next"}},
		Verify: Verification{
			Laws:  []Law{{ID: id, Package: "./internal", Test: "Test" + strings.ToUpper(id)}},
			Gates: []Gate{GateDiagnostics},
		},
		Bindings: []Binding{{Consumer: path, From: old, To: new, Form: BindingDirect}},
	}
}
