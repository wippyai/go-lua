package lua

import (
	"math"
	"testing"
)

func TestIntegerNegation(t *testing.T) {
	cases := []struct {
		name, src string
		want      LInteger
	}{
		{"literal", "return -1", -1},
		{"large_literal", "return -9223372036854775807", -math.MaxInt64},
		{"nested_literal", "return -(-1)", 1},
		{"variable", "local n = 2 return -n", -2},
		{"maxinteger", "return -math.maxinteger", -math.MaxInt64},
		{"mininteger", "return -math.mininteger", math.MinInt64},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			if err := L.DoString(tc.src); err != nil {
				t.Fatal(err)
			}
			if got := L.Get(-1); got != tc.want {
				t.Fatalf("want integer %d, got %v (%T)", tc.want, got, got)
			}
		})
	}
}
