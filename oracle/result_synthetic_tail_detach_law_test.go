package oracle

import "testing"

// TestResultDetachAcceptsTheSealedValueCoordinateUniverse pins the public
// projection boundary for programs whose Value schema contains mounted
// call-result tail coordinates. The engine publishes one summary over the
// complete sealed Value coordinate universe; Result must not reject that
// authenticated summary merely because its branch geometry projects only the
// Program boundary prefix.
func TestResultDetachAcceptsTheSealedValueCoordinateUniverse(t *testing.T) {
	for _, name := range []string{
		"types/cast-and-library",
		"soundness/guarded-opaque-key-store-revokes-closure",
	} {
		t.Run(name, func(t *testing.T) {
			run, class, err := corpusHarnessExecute(t, corpusHarnessFixture(t, name), corpusHarnessDiagnosticMode())
			if err != nil || run == nil || run.result == nil {
				t.Fatalf("fixture did not detach its solved Result: class=%s err=%v result=%t", class, err, run != nil && run.result != nil)
			}
		})
	}
}
