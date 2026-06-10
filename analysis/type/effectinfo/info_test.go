package effectinfo

import "testing"

func TestInfoInterfaceExists(t *testing.T) {
	var _ Info = nil
}
