package artifact

import "testing"

func TestSealValidationOrdersContentIDsStrictly(t *testing.T) {
	left, right := valuesLawID(1), valuesLawID(2)
	if !contentIDBefore(left, right) || contentIDBefore(right, left) || contentIDBefore(left, left) {
		t.Fatal("content identity ordering is not strict")
	}
	if sortedPoints([]Point{{id: right}, {id: left}}) {
		t.Fatal("seal accepted unsorted point rows")
	}
}
