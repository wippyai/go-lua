package publication

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestSealValidationOrdersContentIDsStrictly(t *testing.T) {
	var left, right identity.ContentID
	left[0], right[0] = 1, 2
	if !contentIDBefore(left, right) || contentIDBefore(right, left) || contentIDBefore(left, left) {
		t.Fatal("content identity ordering is not strict")
	}
}
