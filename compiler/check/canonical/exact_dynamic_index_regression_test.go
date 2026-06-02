package canonical_test

import "testing"

func TestCanonicalExactDynamicKeyWritePreservesReadPrecision(t *testing.T) {
	requireCanonicalClean(t, `
		local t: {[string]: number} = {}
		local key: string = "foo"
		t[key] = 42
		local val: number = t["foo"]
	`)
}
