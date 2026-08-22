package placement

import "testing"

func TestAuthenticateFactorCellPinsIntrinsicStackDefault(t *testing.T) {
	for _, value := range []Placement{Stack, OwnedHeap, SharedHeap, Unknown} {
		if got, ok := AuthenticateFactorCell(value, true, true); !ok || got != value {
			t.Fatalf("present %v = %v/%t", value, got, ok)
		}
	}
	if got, ok := AuthenticateFactorCell(Stack, false, true); !ok || got != Stack {
		t.Fatalf("sparse Stack = %v/%t", got, ok)
	}
	for _, input := range []struct {
		value     Placement
		present   bool
		available bool
	}{
		{Bottom, true, true},
		{Bottom, false, true},
		{OwnedHeap, false, true},
		{Interpreter, true, true},
		{Register, true, true},
		{Stack, false, false},
	} {
		if got, ok := AuthenticateFactorCell(input.value, input.present, input.available); ok || got != invalidPlacementResult {
			t.Fatalf("malformed cell %v/%t/%t = %v/%t", input.value, input.present, input.available, got, ok)
		}
	}
}

func TestDisplaceMatrix(t *testing.T) {
	placements := allPlacementValues()
	escapes := allEscapeValues()

	for _, current := range placements {
		for _, escape := range escapes {
			want, wantOK := invalidPlacementResult, false
			switch {
			case !validAnalysisPlacement(current), !validAnalysisEscape(escape):
				// Refused input: Unknown is a semantic top, not an error value.
			case escape == None || escape == Borrow:
				want, wantOK = current, true
			case current == Bottom:
				// Bottom is outside the allocation factor for an applying escape.
			default:
				required, _ := escape.Placement()
				want, wantOK = JoinChecked(current, required)
			}
			if got, ok := DisplaceChecked(current, escape); got != want || ok != wantOK {
				t.Fatalf("Displace(%v, %v) = %v/%t, want %v/%t", current, escape.Name(), got, ok, want, wantOK)
			}
		}
	}
}

func TestDisplaceMonotonicityIdempotenceAndOrder(t *testing.T) {
	placements := allPlacementValues()
	escapes := allEscapeValues()

	for _, current := range placements {
		for _, escape := range escapes {
			got, ok := DisplaceChecked(current, escape)
			if !ok {
				continue
			}
			if !LessOrEq(current, got) {
				t.Fatalf("Displace(%v, %v) decreased to %v", current, escape.Name(), got)
			}
			if repeated, repeatOK := DisplaceChecked(got, escape); !repeatOK || repeated != got {
				t.Fatalf("Displace is not idempotent for (%v, %v): first %v, second %v/%t", current, escape.Name(), got, repeated, repeatOK)
			}
		}
	}

	// Allocation-factor classes are monotone under the same escape. Bottom is
	// the lattice sentinel below that factor and is excluded from this law.
	classes := []Placement{Stack, OwnedHeap, SharedHeap, Unknown}
	for _, left := range classes {
		for _, right := range classes {
			if !LessOrEq(left, right) {
				continue
			}
			for _, escape := range escapes {
				leftResult, leftOK := DisplaceChecked(left, escape)
				rightResult, rightOK := DisplaceChecked(right, escape)
				if !leftOK || !rightOK {
					continue
				}
				if !LessOrEq(leftResult, rightResult) {
					t.Fatalf("order not preserved for %v <= %v under %v: %v <= %v", left, right, escape.Name(), leftResult, rightResult)
				}
			}
		}
	}

	// Escape requirements are ordered by their conservative placement. Invalid
	// escapes have no requirement and are excluded from this law.
	for _, current := range placements {
		for _, left := range escapes {
			for _, right := range escapes {
				leftRequirement, leftOK := escapeRequirement(left)
				rightRequirement, rightOK := escapeRequirement(right)
				if !leftOK || !rightOK || !LessOrEq(leftRequirement, rightRequirement) {
					continue
				}
				leftResult, leftResultOK := DisplaceChecked(current, left)
				rightResult, rightResultOK := DisplaceChecked(current, right)
				if !leftResultOK || !rightResultOK {
					continue
				}
				if !LessOrEq(leftResult, rightResult) {
					t.Fatalf("escape order not preserved for %v <= %v under %v: %v <= %v", left, right, current, leftResult, rightResult)
				}
			}
		}
	}
}

func TestDisplaceInvalidEscapeIsConservative(t *testing.T) {
	for _, current := range allPlacementValues() {
		for raw := int(Return) + 1; raw <= 255; raw++ {
			escape := Escape(raw)
			if got, ok := DisplaceChecked(current, escape); ok || got != invalidPlacementResult {
				t.Fatalf("Displace(%v, invalid escape %d) = %v/%t, want refusal", current, escape, got, ok)
			}
		}
	}
}

func allPlacementValues() []Placement {
	values := make([]Placement, 0, 256)
	for raw := 0; raw <= 255; raw++ {
		values = append(values, Placement(raw))
	}
	return values
}

func allEscapeValues() []Escape {
	values := make([]Escape, 0, 256)
	for raw := 0; raw <= 255; raw++ {
		values = append(values, Escape(raw))
	}
	return values
}

func escapeRequirement(escape Escape) (Placement, bool) {
	if !validAnalysisEscape(escape) {
		return Bottom, false
	}
	required, applies := escape.Placement()
	if !applies {
		return Bottom, true
	}
	return required, true
}
