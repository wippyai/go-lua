package userlattice

import (
	"fmt"
	"strings"
)

type verifiedSpec struct {
	id       AxisID
	elements []ElementID
	index    map[ElementID]Element

	bottom Element
	top    Element
	leq    []bool
	join   []Element

	assignMode      AssignMode
	assignMap       []Element
	callBoundary    CallBoundaryMode
	callBoundaryMap []Element
	claims          map[string]Element
}

// VerifiedSpec is an immutable, law-checked lattice descriptor.
type VerifiedSpec = verifiedSpec

func (s *verifiedSpec) ExtensionKind() string { return extensionKind }
func (s *verifiedSpec) ExtensionID() string   { return string(s.id) }

// Verify checks every lattice law and transfer-map monotonicity requirement,
// returning the immutable runtime descriptor used by Register.
func Verify(spec Spec) (*VerifiedSpec, error) {
	if spec.ID == "" {
		return nil, fmt.Errorf("userlattice: spec has empty id")
	}
	if len(spec.Elements) == 0 {
		return nil, fmt.Errorf("userlattice %q: element set is empty", spec.ID)
	}
	if len(spec.Elements) > 1<<16 {
		return nil, fmt.Errorf("userlattice %q: element set has %d elements, max 65536", spec.ID, len(spec.Elements))
	}
	index := make(map[ElementID]Element, len(spec.Elements))
	elements := make([]ElementID, len(spec.Elements))
	for i, name := range spec.Elements {
		if name == "" {
			return nil, fmt.Errorf("userlattice %q: element[%d] has empty name", spec.ID, i)
		}
		if _, exists := index[name]; exists {
			return nil, fmt.Errorf("userlattice %q: duplicate element %q", spec.ID, name)
		}
		index[name] = Element(i)
		elements[i] = name
	}
	bottom, ok := index[spec.Bottom]
	if !ok {
		return nil, fmt.Errorf("userlattice %q: bottom %q is not an element", spec.ID, spec.Bottom)
	}
	top, ok := index[spec.Top]
	if !ok {
		return nil, fmt.Errorf("userlattice %q: top %q is not an element", spec.ID, spec.Top)
	}

	n := len(elements)
	leq := make([]bool, n*n)
	for i := 0; i < n; i++ {
		leq[i*n+i] = true
	}
	for i, pair := range spec.Order {
		lower, lowerOK := index[pair.Lower]
		upper, upperOK := index[pair.Upper]
		switch {
		case !lowerOK:
			return nil, fmt.Errorf("userlattice %q: order[%d] lower %q is not an element", spec.ID, i, pair.Lower)
		case !upperOK:
			return nil, fmt.Errorf("userlattice %q: order[%d] upper %q is not an element", spec.ID, i, pair.Upper)
		}
		leq[int(lower)*n+int(upper)] = true
	}
	transitiveClosure(leq, n)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if leq[i*n+j] && leq[j*n+i] {
				return nil, fmt.Errorf("userlattice %q: order cycle: %s <= %s and %s <= %s",
					spec.ID, elements[i], elements[j], elements[j], elements[i])
			}
		}
	}
	for i := 0; i < n; i++ {
		if !leq[int(bottom)*n+i] {
			return nil, fmt.Errorf("userlattice %q: bottom %q is not <= %q", spec.ID, spec.Bottom, elements[i])
		}
		if !leq[i*n+int(top)] {
			return nil, fmt.Errorf("userlattice %q: top %q is not >= %q", spec.ID, spec.Top, elements[i])
		}
	}

	join, err := buildJoinTable(spec.ID, elements, leq)
	if err != nil {
		return nil, err
	}
	assignMap, err := buildElementMap(spec.ID, "assign map", spec.Hooks.OnAssign.Map, index, elements, leq)
	if err != nil {
		return nil, err
	}
	callMap, err := buildElementMap(spec.ID, "call-boundary map", spec.Hooks.OnCallBoundary.Map, index, elements, leq)
	if err != nil {
		return nil, err
	}
	if spec.Hooks.OnAssign.Mode != AssignDrop && spec.Hooks.OnAssign.Mode != AssignPropagate {
		return nil, fmt.Errorf("userlattice %q: unknown assign mode %d", spec.ID, spec.Hooks.OnAssign.Mode)
	}
	if spec.Hooks.OnCallBoundary.Mode != CallBoundaryDrop && spec.Hooks.OnCallBoundary.Mode != CallBoundaryKeep {
		return nil, fmt.Errorf("userlattice %q: unknown call-boundary mode %d", spec.ID, spec.Hooks.OnCallBoundary.Mode)
	}
	claims := make(map[string]Element, len(spec.Hooks.OnClaim))
	for i, claim := range spec.Hooks.OnClaim {
		if claim.Claim == "" {
			return nil, fmt.Errorf("userlattice %q: claim[%d] has empty name", spec.ID, i)
		}
		elem, ok := index[claim.Element]
		if !ok {
			return nil, fmt.Errorf("userlattice %q: claim[%d] element %q is not an element", spec.ID, i, claim.Element)
		}
		if _, exists := claims[claim.Claim]; exists {
			return nil, fmt.Errorf("userlattice %q: duplicate claim %q", spec.ID, claim.Claim)
		}
		claims[claim.Claim] = elem
	}

	return &verifiedSpec{
		id:              spec.ID,
		elements:        elements,
		index:           index,
		bottom:          bottom,
		top:             top,
		leq:             leq,
		join:            join,
		assignMode:      spec.Hooks.OnAssign.Mode,
		assignMap:       assignMap,
		callBoundary:    spec.Hooks.OnCallBoundary.Mode,
		callBoundaryMap: callMap,
		claims:          claims,
	}, nil
}

func transitiveClosure(leq []bool, n int) {
	for k := 0; k < n; k++ {
		for i := 0; i < n; i++ {
			if !leq[i*n+k] {
				continue
			}
			for j := 0; j < n; j++ {
				if leq[k*n+j] {
					leq[i*n+j] = true
				}
			}
		}
	}
}

func buildJoinTable(id AxisID, elements []ElementID, leq []bool) ([]Element, error) {
	n := len(elements)
	join := make([]Element, n*n)
	for a := 0; a < n; a++ {
		for b := 0; b < n; b++ {
			var minimal []int
			for upper := 0; upper < n; upper++ {
				if !leq[a*n+upper] || !leq[b*n+upper] {
					continue
				}
				isMinimal := true
				for other := 0; other < n; other++ {
					if other == upper {
						continue
					}
					if leq[a*n+other] && leq[b*n+other] && leq[other*n+upper] {
						isMinimal = false
						break
					}
				}
				if isMinimal {
					minimal = append(minimal, upper)
				}
			}
			if len(minimal) != 1 {
				return nil, fmt.Errorf("userlattice %q: pair (%s, %s) has no least upper bound; minimal upper bounds: %s",
					id, elements[a], elements[b], elementList(elements, minimal))
			}
			join[a*n+b] = Element(minimal[0])
		}
	}
	return join, nil
}

func buildElementMap(id AxisID, label string, entries []ElementMapEntry, index map[ElementID]Element, elements []ElementID, leq []bool) ([]Element, error) {
	n := len(elements)
	out := make([]Element, n)
	for i := 0; i < n; i++ {
		out[i] = Element(i)
	}
	seen := make(map[ElementID]struct{}, len(entries))
	for i, entry := range entries {
		from, fromOK := index[entry.From]
		to, toOK := index[entry.To]
		switch {
		case !fromOK:
			return nil, fmt.Errorf("userlattice %q: %s[%d] from %q is not an element", id, label, i, entry.From)
		case !toOK:
			return nil, fmt.Errorf("userlattice %q: %s[%d] to %q is not an element", id, label, i, entry.To)
		}
		if _, exists := seen[entry.From]; exists {
			return nil, fmt.Errorf("userlattice %q: %s maps %q more than once", id, label, entry.From)
		}
		seen[entry.From] = struct{}{}
		out[from] = to
	}
	for a := 0; a < n; a++ {
		for b := 0; b < n; b++ {
			if !leq[a*n+b] {
				continue
			}
			fa, fb := out[a], out[b]
			if !leq[int(fa)*n+int(fb)] {
				return nil, fmt.Errorf("userlattice %q: %s is not monotone: %s <= %s but %s !<= %s",
					id, label, elements[a], elements[b], elements[fa], elements[fb])
			}
		}
	}
	return out, nil
}

func elementList(elements []ElementID, indexes []int) string {
	if len(indexes) == 0 {
		return "<none>"
	}
	out := make([]string, len(indexes))
	for i, index := range indexes {
		out[i] = string(elements[index])
	}
	return strings.Join(out, ", ")
}
