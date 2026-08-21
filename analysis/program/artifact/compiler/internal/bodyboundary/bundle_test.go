package bodyboundary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

func bodyBoundaryTestID(value byte) identity.ContentID {
	var id identity.ContentID
	id[0] = value
	return id
}

func TestBundleTransfersCanonicalPlanesExactlyOnce(t *testing.T) {
	bodyID, contextID, entryID, formalID := bodyBoundaryTestID(1), bodyBoundaryTestID(2), bodyBoundaryTestID(3), bodyBoundaryTestID(4)
	boundary, ok := programschema.NewFunctionBoundary(bodyBoundaryTestID(5), bodyID, contextID, entryID, formalID, 0, 0, 0, 0, 0, 0)
	if !ok {
		t.Fatal("boundary fixture")
	}
	bundle := &Bundle{
		functionBoundaries: []programschema.FunctionBoundary{boundary},
		functionBoundaryByBody: map[identity.ContentID]programschema.FunctionBoundary{
			bodyID: boundary,
		},
	}
	if got, found := bundle.FunctionBoundaryForBody(bodyID); !found || got.ID() != boundary.ID() {
		t.Fatal("body boundary index did not resolve canonical row")
	}
	planes, transferred := bundle.TakeCanonicalPlanes()
	if !transferred || len(planes.FunctionBoundaries) != 1 || planes.FunctionBoundaries[0].ID() != boundary.ID() {
		t.Fatal("canonical planes were not transferred")
	}
	if _, transferred = bundle.TakeCanonicalPlanes(); transferred {
		t.Fatal("canonical planes transferred twice")
	}
	if bundle.FunctionBoundaries() != nil {
		t.Fatal("taken bundle retained a readable mirror")
	}
	if _, found := bundle.FunctionBoundaryForBody(bodyID); found {
		t.Fatal("taken bundle retained a readable mirror")
	}
}
