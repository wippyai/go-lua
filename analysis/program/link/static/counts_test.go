package static

import "testing"

func TestStaticCountRowsFailClosedBeforeSeal(t *testing.T) {
	if rows, ok := (*Component)(nil).CountRows(); ok || rows.Available() {
		t.Fatal("nil Static component exposed denominator rows")
	}
	if rows, ok := (Cold{}).CountRows(); ok || rows.Available() {
		t.Fatal("zero Static Cold exposed denominator rows")
	}
}
