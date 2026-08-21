package placement

import "testing"

func TestDisplaceMatrix(t *testing.T) {
	placements := allPlacementValues()
	escapes := allEscapeValues()

	for _, current := range placements {
		for _, escape := range escapes {
			want := Unknown
			switch {
			case !validAnalysisPlacement(current), !validAnalysisEscape(escape):
				want = Unknown
			case escape == None || escape == Borrow:
				want = current
			case current == Bottom:
				want = Unknown
			default:
				required, _ := escape.Placement()
				want = Join(current, required)
			}
			if got := Displace(current, escape); got != want {
				t.Fatalf("Displace(%v, %v) = %v, want %v", current, escape.Name(), got, want)
			}
		}
	}
}

func TestDisplaceMonotonicityIdempotenceAndOrder(t *testing.T) {
	placements := allPlacementValues()
	escapes := allEscapeValues()

	for _, current := range placements {
		for _, escape := range escapes {
			got := Displace(current, escape)
			if validAnalysisPlacement(current) && !LessOrEq(current, got) {
				t.Fatalf("Displace(%v, %v) decreased to %v", current, escape.Name(), got)
			}
			if Displace(got, escape) != got {
				t.Fatalf("Displace is not idempotent for (%v, %v): first %v, second %v", current, escape.Name(), got, Displace(got, escape))
			}
		}
	}

	// Once a placement is seeded, applying the same escape is monotone in the
	// current placement order. Bottom is intentionally a missing-seed
	// sentinel: an applying escape maps it directly to Unknown, so it is not
	// an order-preserving input below Stack.
	seeded := []Placement{Stack, OwnedHeap, SharedHeap, Unknown}
	for _, left := range seeded {
		for _, right := range seeded {
			if !LessOrEq(left, right) {
				continue
			}
			for _, escape := range escapes {
				leftResult := Displace(left, escape)
				rightResult := Displace(right, escape)
				if !LessOrEq(leftResult, rightResult) {
					t.Fatalf("order not preserved for %v <= %v under %v: %v <= %v", left, right, escape.Name(), leftResult, rightResult)
				}
			}
		}
	}

	// Escape requirements are ordered by their conservative placement. This
	// law includes all invalid escape values, which are normalized to Unknown.
	for _, current := range placements {
		for _, left := range escapes {
			for _, right := range escapes {
				if !LessOrEq(escapeRequirement(left), escapeRequirement(right)) {
					continue
				}
				leftResult := Displace(current, left)
				rightResult := Displace(current, right)
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
			if got := Displace(current, escape); got != Unknown {
				t.Fatalf("Displace(%v, invalid escape %d) = %v, want unknown", current, escape, got)
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

func escapeRequirement(escape Escape) Placement {
	if !validAnalysisEscape(escape) {
		return Unknown
	}
	required, applies := escape.Placement()
	if !applies {
		return Bottom
	}
	return required
}
