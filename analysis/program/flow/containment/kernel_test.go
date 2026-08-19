package containment

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func buildKernelForTest(input kernelInput) (*Result, error) {
	result, err := buildKernel(input)
	if result != nil {
		result.sourceID[0] = 1
		result.flowID[0] = 2
		result.staticID[0] = 3
		result.moduleID[0] = 4
	}
	return result, err
}

func TestKernelBuildsCanonicalIntervalsAndStaticBits(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody:   2,
		keyspace.FamilyValues: 2,
	}
	root := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	value1 := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	value2 := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	input := kernelInput{
		counts: counts,
		// Deliberately emit rows in a noncanonical order.  The proof must use
		// family/ordinal order, not emitter order, for child traversal.
		edges: []kernelEdge{
			{child: value2, parent: body2},
			{child: body2, parent: root},
			{child: value1, parent: root},
		},
		roots:  []keyspace.Term{root},
		static: []keyspace.Term{value2, value2},
	}
	result, err := buildKernelForTest(input)
	if err != nil {
		t.Fatalf("buildKernel() error = %v", err)
	}
	if result.Count() != 4 {
		t.Fatalf("Count() = %d, want 4", result.Count())
	}
	if parent, ok := result.Parent(body2); !ok || parent != root {
		t.Fatalf("Parent(body2) = %v/%v, want %v/true", parent, ok, root)
	}
	if _, ok := result.Parent(root); ok {
		t.Fatal("Parent(root) unexpectedly found a parent")
	}
	if !result.Contains(root, value2) || !result.Contains(body2, value2) || result.Contains(body2, value1) {
		t.Fatal("Containment intervals do not describe the sealed forest")
	}
	if result.Contains(value2, body2) || !result.Contains(value2, value2) {
		t.Fatal("Containment identity or direction is incorrect")
	}
	if !result.Static(value2) || result.Static(value1) {
		t.Fatal("static bitset does not retain exact marks")
	}
	for index, want := range []keyspace.Term{value1, value2, root, body2} {
		got, ok := result.At(index)
		if !ok || got != want {
			t.Fatalf("At(%d) = %v/%v, want %v/true", index, got, ok, want)
		}
	}
}

func TestKernelEdgeOrderDoesNotChangeProof(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 3}
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	body3 := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	edges := []kernelEdge{
		{child: body3, parent: body1},
		{child: body2, parent: body1},
	}
	first, err := buildKernelForTest(kernelInput{counts: counts, edges: edges, roots: []keyspace.Term{body1}})
	if err != nil {
		t.Fatalf("first buildKernel() error = %v", err)
	}
	second, err := buildKernelForTest(kernelInput{counts: counts, edges: []kernelEdge{edges[1], edges[0]}, roots: []keyspace.Term{body1}})
	if err != nil {
		t.Fatalf("second buildKernel() error = %v", err)
	}
	for outer := uint32(1); outer <= 3; outer++ {
		for inner := uint32(1); inner <= 3; inner++ {
			left := keyspace.MakeTerm(keyspace.FamilyBody, outer)
			right := keyspace.MakeTerm(keyspace.FamilyBody, inner)
			if first.Contains(left, right) != second.Contains(left, right) {
				t.Fatalf("Contains(%v,%v) changed with edge order", left, right)
			}
		}
	}
}

func TestKernelFallbackEdges(t *testing.T) {
	body := func(ordinal uint32) keyspace.Term { return keyspace.MakeTerm(keyspace.FamilyBody, ordinal) }
	tests := []struct {
		name      string
		input     kernelInput
		wantError bool
		want      keyspace.Term
	}{
		{
			name: "repeated identical requests coalesce",
			input: kernelInput{
				counts:   [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 3},
				edges:    []kernelEdge{{child: body(2), parent: body(1)}},
				fallback: []kernelEdge{{child: body(3), parent: body(2)}, {child: body(3), parent: body(2)}},
				roots:    []keyspace.Term{body(1)},
			},
			want: body(2),
		},
		{
			name: "different fallback parents reject",
			input: kernelInput{
				counts:   [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 4},
				edges:    []kernelEdge{{child: body(2), parent: body(1)}, {child: body(3), parent: body(1)}},
				fallback: []kernelEdge{{child: body(4), parent: body(2)}, {child: body(4), parent: body(3)}},
				roots:    []keyspace.Term{body(1)},
			},
			wantError: true,
		},
		{
			name: "ordinary and fallback overlap rejects",
			input: kernelInput{
				counts:   [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 3},
				edges:    []kernelEdge{{child: body(2), parent: body(1)}, {child: body(3), parent: body(1)}},
				fallback: []kernelEdge{{child: body(2), parent: body(3)}},
				roots:    []keyspace.Term{body(1)},
			},
			wantError: true,
		},
		{
			name: "root child rejects",
			input: kernelInput{
				counts:   [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 2},
				fallback: []kernelEdge{{child: body(1), parent: body(2)}},
				roots:    []keyspace.Term{body(1)},
			},
			wantError: true,
		},
		{
			name: "root parent is accepted",
			input: kernelInput{
				counts:   [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 3},
				edges:    []kernelEdge{{child: body(2), parent: body(1)}},
				fallback: []kernelEdge{{child: body(3), parent: body(1)}},
				roots:    []keyspace.Term{body(1)},
			},
			want: body(1),
		},
		{
			name: "self edge rejects",
			input: kernelInput{
				counts:   [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 2},
				fallback: []kernelEdge{{child: body(2), parent: body(2)}},
				roots:    []keyspace.Term{body(1)},
			},
			wantError: true,
		},
		{
			name: "invalid ordinal rejects",
			input: kernelInput{
				counts:   [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 2},
				fallback: []kernelEdge{{child: body(3), parent: body(2)}},
				roots:    []keyspace.Term{body(1)},
			},
			wantError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := buildKernelForTest(test.input)
			if test.wantError {
				if err == nil {
					t.Fatal("buildKernel() accepted malformed fallback relation")
				}
				return
			}
			if err != nil {
				t.Fatalf("buildKernel() error = %v", err)
			}
			if parent, ok := result.Parent(body(3)); !ok || parent != test.want {
				t.Fatalf("Parent(body3) = %v/%v, want %v/true", parent, ok, test.want)
			}
		})
	}
}

func TestKernelRejectsIncompleteConflictingAndCyclicRelations(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	body3 := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	counts := [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 3}
	tests := []struct {
		name  string
		input kernelInput
	}{
		{
			name:  "missing parent",
			input: kernelInput{counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 2}, roots: []keyspace.Term{body1}},
		},
		{
			name: "conflicting parents",
			input: kernelInput{
				counts: counts,
				roots:  []keyspace.Term{body1},
				edges:  []kernelEdge{{child: body2, parent: body1}, {child: body2, parent: body3}, {child: body3, parent: body1}},
			},
		},
		{
			name: "duplicate edge",
			input: kernelInput{
				counts: counts,
				roots:  []keyspace.Term{body1},
				edges:  []kernelEdge{{child: body2, parent: body1}, {child: body2, parent: body1}, {child: body3, parent: body1}},
			},
		},
		{
			name: "cycle",
			input: kernelInput{
				counts: counts,
				roots:  []keyspace.Term{body1},
				edges:  []kernelEdge{{child: body2, parent: body3}, {child: body3, parent: body2}},
			},
		},
		{
			name: "root has parent",
			input: kernelInput{
				counts: counts,
				roots:  []keyspace.Term{body1},
				edges:  []kernelEdge{{child: body1, parent: body2}, {child: body2, parent: body3}, {child: body3, parent: body1}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := buildKernelForTest(test.input); err == nil {
				t.Fatal("buildKernel() unexpectedly accepted malformed relation")
			}
		})
	}
}

func TestKernelRejectsMalformedTermsAndRoots(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1}
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	badFamily := keyspace.Term(uint32(1)<<8 | uint32(keyspace.FamilyCount))
	tests := []kernelInput{
		{counts: counts, roots: []keyspace.Term{0}},
		{counts: counts, roots: []keyspace.Term{badFamily}},
		{counts: counts, roots: []keyspace.Term{body}, static: []keyspace.Term{badFamily}},
		{counts: counts, roots: []keyspace.Term{body, body}},
		{counts: counts, roots: []keyspace.Term{body}, edges: []kernelEdge{{child: body, parent: badFamily}}},
	}
	for index, input := range tests {
		if _, err := buildKernelForTest(input); err == nil {
			t.Errorf("case %d: buildKernel() accepted malformed input", index)
		}
	}
}

func TestKernelBuildsLongChainIteratively(t *testing.T) {
	const count = uint32(8192)
	counts := [keyspace.FamilyCount]uint32{keyspace.FamilyBody: count}
	edges := make([]kernelEdge, 0, count-1)
	for ordinal := uint32(2); ordinal <= count; ordinal++ {
		edges = append(edges, kernelEdge{
			child:  keyspace.MakeTerm(keyspace.FamilyBody, ordinal),
			parent: keyspace.MakeTerm(keyspace.FamilyBody, ordinal-1),
		})
	}
	result, err := buildKernelForTest(kernelInput{
		counts: counts,
		edges:  edges,
		roots:  []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyBody, 1)},
	})
	if err != nil {
		t.Fatalf("buildKernel() error = %v", err)
	}
	first := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	last := keyspace.MakeTerm(keyspace.FamilyBody, count)
	if !result.Contains(first, last) || result.Contains(last, first) {
		t.Fatal("long chain intervals are incorrect")
	}
}
