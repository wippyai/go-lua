package composite

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/schema"
)

// TestFormalAuthoritiesAreOneMountedTypedJoin states the composition cut for
// the formal Placement consumer. Target is read from Link's exact Boundary,
// while every Call mounted row is checked directly against Pack's exact
// owner-fenced projection. The binding receives Target and Pack through typed
// accessors; it does not reconstruct either authority from a generic cell.
func TestFormalAuthoritiesAreOneMountedTypedJoin(t *testing.T) {
	record := mountedRecord(t, "formal-authority-join", "return 42")
	if record.Source == nil || record.Source.Boundary() == nil {
		t.Fatal("mounted record has no Link Boundary")
	}
	target, targetOK := record.Source.Boundary().Target()
	if !targetOK || target == nil || record.targetContract != target {
		t.Fatal("formal authority did not retain Link Boundary's exact Target contract")
	}
	if !mountedActualsComplete(record.CallAlgebra, record.PackSchema) {
		t.Fatal("formal Call mounted rows are incomplete against their exact Pack owner")
	}
	if record.CallAlgebra.MountedCallCount() != 0 {
		t.Fatalf("the no-call fixture did not exercise an empty mounted actual denominator: %d", record.CallAlgebra.MountedCallCount())
	}

	bound := materializerBinding(t, record)
	formalKey := schema.Key("placement-formal")
	capability, capabilityOK := bound.Rules().CapabilityByKey(formalKey)
	if !capabilityOK || !capability.Mounted() || capability.Link() {
		t.Fatal("formal rule was not published as a mounted capability")
	}
	cell, cellOK := bound.Rules().cellByKey(formalKey)
	if !cellOK || !cell.Available() || capability.Activation() {
		t.Fatal("formal rule did not publish its sealed canonical cell and capability")
	}
}

// TestFormalAuthoritySurfaceDoesNotEraseItsTypedFields keeps the authority
// record's formal Target join private while proving the public composition
// interface names its concrete type.
func TestFormalAuthoritySurfaceDoesNotEraseItsTypedFields(t *testing.T) {
	var authoritiesType authorities
	formalContractType := reflect.TypeOf((*contract.Contract)(nil))
	for _, field := range []struct {
		name string
		want reflect.Type
	}{
		{"targetContract", formalContractType},
	} {
		member, found := reflect.TypeOf(authoritiesType).FieldByName(field.name)
		if !found || member.Type != field.want || member.PkgPath == "" {
			t.Fatalf("authority field %q is not a private typed join: found=%t type=%v", field.name, found, member.Type)
		}
	}
}
