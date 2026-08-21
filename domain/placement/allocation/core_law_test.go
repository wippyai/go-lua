package allocation

import (
	"testing"

	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

func TestAllocationCoreRejectsUnavailableAndMissingCoordinates(t *testing.T) {
	if _, ok := allocationOperandForSchema(placementdomain.Schema{}, heapdomain.Key{}); ok {
		t.Fatal("unavailable Placement schema admitted a missing coordinate")
	}
	if _, _, ok := allocationOperandContentForSchema(placementdomain.Schema{}, operand{}); ok {
		t.Fatal("unavailable Placement schema admitted an empty operand")
	}
	if _, ok := allocationCoordinateForSchema(placementdomain.Schema{}, operand{}); ok {
		t.Fatal("unavailable Placement schema projected an optimistic coordinate")
	}
}
