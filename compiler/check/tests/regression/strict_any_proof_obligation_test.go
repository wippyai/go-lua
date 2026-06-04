package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// `any` is a compatibility atom, not proof of a concrete type. Assignment to a
// concrete annotation must be justified by a narrowing/assertion/cast.
func TestStrictAny_AssignmentRequiresProof(t *testing.T) {
	source := `
		local raw: any = "name"
		local name: string = raw
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatalf("expected concrete assignment from any to require proof")
	}
}

// Unannotated parameters may project to gradual `any`, but that still does not
// satisfy a concrete local annotation without evidence.
func TestStrictAny_UnannotatedParamAssignmentRequiresProof(t *testing.T) {
	source := `
		local function f(raw)
			local name: string = raw
			return name
		end
		return { f = f }
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatalf("expected concrete assignment from unannotated param to require proof")
	}
}

// Declared returns are proof obligations. Returning `any` to a concrete return
// type must not be accepted by expected-type coercion.
func TestStrictAny_ReturnRequiresProof(t *testing.T) {
	source := `
		local function f(raw): string
			return raw
		end
		return { f = f }
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatalf("expected concrete return from unannotated param to require proof")
	}
}

// Call arguments are also proof obligations: a concrete parameter does not
// accept `any` unless the argument has been narrowed or asserted first.
func TestStrictAny_CallArgumentRequiresProof(t *testing.T) {
	source := `
		local function takes_name(name: string)
		end
		local raw: any = "name"
		takes_name(raw)
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatalf("expected concrete call argument from any to require proof")
	}
}

func TestStrictAny_ExplicitCastProvidesProof(t *testing.T) {
	source := `
		local raw: any = "name"
		local name: string = raw :: string
		return name
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected explicit cast to provide proof, got %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestStrictAny_LengthRequiresContainerProof(t *testing.T) {
	source := `
		local raw: any = { "one" }
		local len: integer = #raw
		return len
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatalf("expected length over any to require proof")
	}
}

func TestStrictAny_LengthCastProvidesContainerProof(t *testing.T) {
	source := `
		local raw: any = { "one" }
		local len: integer = #(raw :: {string})
		return len
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected explicit container cast to prove length, got %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestStrictAny_LengthTypeGuardProvidesContainerProof(t *testing.T) {
	source := `
		local raw: any = { "one" }
		if type(raw) ~= "table" then
			return 0
		end
		local len: integer = #raw
		return len
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected table guard to prove lengthability, got %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestStrictAny_ConcatRequiresStringableProof(t *testing.T) {
	source := `
		local raw: any = "name"
		local label: string = raw .. "-suffix"
		return label
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatalf("expected concatenation over any to require proof")
	}
}

func TestStrictAny_ConcatCastProvidesStringableProof(t *testing.T) {
	source := `
		local raw: any = "name"
		local label: string = (raw :: string) .. "-suffix"
		return label
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected explicit string cast to prove concatenation, got %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestStrictAny_OrderedComparisonRequiresProof(t *testing.T) {
	source := `
		local raw: any = 1
		local ok: boolean = raw < 10
		return ok
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatalf("expected ordered comparison over any to require proof")
	}
}

func TestStrictAny_OrderedComparisonCastProvidesProof(t *testing.T) {
	source := `
		local raw: any = 1
		local ok: boolean = (raw :: number) < 10
		return ok
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected explicit numeric cast to prove ordered comparison, got %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
