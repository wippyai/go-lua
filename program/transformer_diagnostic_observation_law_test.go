package program_test

import (
	"testing"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/lower"
	programstatic "github.com/wippyai/go-lua/program/static"
)

func TestTransformerBranchDiagnosticRequiresScopePreservingRewrite(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		wantUnique int
	}{
		{
			name:       "assignment-only",
			source:     "local flag = true\nif flag then\n  flag = false\nend\nreturn flag\n",
			wantUnique: 1,
		},
		{
			name:       "local-introduction",
			source:     "if true then\n  local scoped = 1\nend\nreturn 0\n",
			wantUnique: 0,
		},
		{
			name:       "static-type-introduction",
			source:     "if true then\n  type Scoped = {x: number}\nend\nreturn 0\n",
			wantUnique: 0,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sealed, err := lower.Lower(lower.Source{Name: "branch-diagnostic-scope.lua", Text: []byte(test.source)})
			if err != nil {
				t.Fatalf("Lower: %v", err)
			}
			input := sealed.TransformerInput()
			branchRoutes := 0
			unique := make(map[keyspace.ContentID]struct{})
			for index := 0; index < input.StructuralRoutes().Count(); index++ {
				route, routeOK := input.StructuralRoutes().At(index)
				if !routeOK {
					continue
				}
				if _, guarded := route.Guard(); !guarded {
					continue
				}
				branchRoutes++
				if route.DiagnosticObservationKind() != program.DiagnosticObservationBranchCondition {
					continue
				}
				observation, observationOK := route.DiagnosticObservation()
				if !observationOK {
					continue
				}
				if !observation.Available() || observation.Kind() != program.DiagnosticObservationBranchCondition || !observation.ID().Available() {
					t.Fatal("issued branch diagnostic observation is malformed")
				}
				unique[observation.ID()] = struct{}{}
			}
			if branchRoutes < 2 {
				t.Fatalf("branch route control = %d, want both arms", branchRoutes)
			}
			if len(unique) != test.wantUnique {
				t.Fatalf("unique branch observations = %d, want %d", len(unique), test.wantUnique)
			}
		})
	}
}

func TestTransformerUnresolvedTypeObservationCarriesExactStaticProof(t *testing.T) {
	left, err := lower.Lower(lower.Source{Name: "diagnostic-observation.lua", Text: []byte("type MissingAlias = Missing\n")})
	if err != nil {
		t.Fatalf("Lower: %v", err)
	}
	right, err := lower.Lower(lower.Source{Name: "diagnostic-observation.lua", Text: []byte("type MissingAlias = Missing\n")})
	if err != nil {
		t.Fatalf("Lower replay: %v", err)
	}
	leftInput, rightInput := left.TransformerInput(), right.TransformerInput()
	var leftObservationID keyspace.ContentID
	var rightObservationID keyspace.ContentID
	leftFound, rightFound := false, false
	for index := 0; index < leftInput.Static().StaticTypes().Count(); index++ {
		ref, ok := leftInput.Static().StaticTypes().At(index)
		if !ok {
			t.Fatalf("left StaticTypes.At(%d)", index)
		}
		resolution, target, _, ok := leftInput.Static().References().Get(ref.Term())
		if !ok || target != 0 || resolution != programstatic.TypeRefUnresolved {
			continue
		}
		observation, observationOK := leftInput.TypeReferenceUnresolvedObservation(ref.Term())
		if !observationOK || !observation.Available() || observation.Kind() == 0 {
			t.Fatal("left unresolved observation was not available")
		}
		payload, payloadOK := observation.UnresolvedTypeReference()
		path, pathOK := payload.Path()
		location, locationOK := observation.Location()
		if !payloadOK || !pathOK || len(path) != 1 || path[0] != "Missing" || !locationOK || location.File != "diagnostic-observation.lua" {
			t.Fatalf("left unresolved payload = %#v/%v path=%v/%v location=%#v/%v", payload, payloadOK, path, pathOK, location, locationOK)
		}
		leftObservationID = observation.ID()
		leftFound = true
	}
	for index := 0; index < rightInput.Static().StaticTypes().Count(); index++ {
		ref, ok := rightInput.Static().StaticTypes().At(index)
		if !ok {
			t.Fatalf("right StaticTypes.At(%d)", index)
		}
		resolution, target, _, ok := rightInput.Static().References().Get(ref.Term())
		if !ok || target != 0 || resolution != programstatic.TypeRefUnresolved {
			continue
		}
		observation, observationOK := rightInput.TypeReferenceUnresolvedObservation(ref.Term())
		if !observationOK || !observation.Available() {
			t.Fatal("right unresolved observation was not available")
		}
		rightObservationID = observation.ID()
		rightFound = true
	}
	if !leftFound || !rightFound || leftObservationID != rightObservationID {
		t.Fatalf("deterministic unresolved observation = %v/%v ids=%x/%x", leftFound, rightFound, leftObservationID, rightObservationID)
	}
}

func TestTransformerQualifiedUnresolvedTypeObservationCarriesRootProof(t *testing.T) {
	p, err := lower.Lower(lower.Source{Name: "qualified-diagnostic-observation.lua", Text: []byte("type MissingAlias = Missing.Namespace\n")})
	if err != nil {
		t.Fatalf("Lower qualified: %v", err)
	}
	input := p.TransformerInput()
	for index := 0; index < input.Static().StaticTypes().Count(); index++ {
		ref, ok := input.Static().StaticTypes().At(index)
		if !ok {
			t.Fatalf("StaticTypes.At(%d)", index)
		}
		resolution, target, root, ok := input.Static().References().Get(ref.Term())
		if !ok || resolution != programstatic.TypeRefUnresolved || target != 0 || root == 0 {
			continue
		}
		observation, observationOK := input.TypeReferenceUnresolvedObservation(ref.Term())
		payload, payloadOK := observation.UnresolvedTypeReference()
		path, pathOK := payload.Path()
		if !observationOK || !observation.Available() || !payloadOK || !pathOK || len(path) != 2 || path[0] != "Missing" || path[1] != "Namespace" || !payload.RootID().Available() {
			t.Fatalf("qualified unresolved observation = available:%v payload:%v/%v path:%v/%v root:%v", observation.Available(), payloadOK, payload, path, pathOK, payload.RootID())
		}
		return
	}
	t.Fatal("qualified unresolved type reference was not issued")
}

func TestTransformerUnresolvedValueObservationCarriesExactImplicitReadProof(t *testing.T) {
	const source = "local total = missing_count + 1\nreturn total\n"
	left, err := lower.Lower(lower.Source{Name: "unresolved-value-observation.lua", Text: []byte(source)})
	if err != nil {
		t.Fatalf("Lower: %v", err)
	}
	right, err := lower.Lower(lower.Source{Name: "unresolved-value-observation.lua", Text: []byte(source)})
	if err != nil {
		t.Fatalf("Lower replay: %v", err)
	}
	leftInput, rightInput := left.TransformerInput(), right.TransformerInput()
	if leftInput.ValueReferenceUnresolvedObservationCount() != 1 || rightInput.ValueReferenceUnresolvedObservationCount() != 1 {
		t.Fatalf("implicit read denominator = %d/%d, want 1/1", leftInput.ValueReferenceUnresolvedObservationCount(), rightInput.ValueReferenceUnresolvedObservationCount())
	}
	leftObservation, leftOK := leftInput.ValueReferenceUnresolvedObservationAt(0)
	rightObservation, rightOK := rightInput.ValueReferenceUnresolvedObservationAt(0)
	leftPayload, leftPayloadOK := leftObservation.UnresolvedValueReference()
	rightPayload, rightPayloadOK := rightObservation.UnresolvedValueReference()
	leftName, leftNameOK := leftPayload.Name()
	rightName, rightNameOK := rightPayload.Name()
	location, locationOK := leftObservation.Location()
	if !leftOK || !rightOK || !leftPayloadOK || !rightPayloadOK || !leftNameOK || !rightNameOK ||
		leftName != "missing_count" || rightName != leftName || !leftPayload.ReadID().Available() || !leftPayload.CellID().Available() ||
		leftPayload.ReadID() == leftPayload.CellID() || leftObservation.ID() != rightObservation.ID() ||
		leftPayload.ReadID() != rightPayload.ReadID() || leftPayload.CellID() != rightPayload.CellID() ||
		!locationOK || location.File != "unresolved-value-observation.lua" || location.StartLine != 1 || location.StartCol != 15 {
		t.Fatalf("unresolved value proof = left:%v/%v/%q right:%v/%v/%q location:%+v/%v", leftOK, leftPayloadOK, leftName, rightOK, rightPayloadOK, rightName, location, locationOK)
	}
	if _, ok := leftInput.ValueReferenceUnresolvedObservationAt(-1); ok {
		t.Fatal("unresolved value observation accepted negative index")
	}
	if _, ok := leftInput.ValueReferenceUnresolvedObservationAt(1); ok {
		t.Fatal("unresolved value observation accepted denominator index")
	}
}
