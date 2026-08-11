package value

import (
	"math"
	"testing"
)

func TestSourceFloatRepresentabilitySelectsOpaqueFallback(t *testing.T) {
	for _, scenario := range []struct {
		name  string
		value float64
		want  bool
	}{
		{name: "finite", value: 3.5, want: true},
		{name: "positive-zero", value: 0, want: true},
		{name: "nan", value: math.NaN(), want: false},
		{name: "negative-zero", value: math.Copysign(0, -1), want: true},
	} {
		if got := sourceFloatRepresentable(scenario.value); got != scenario.want {
			t.Errorf("%s representable = %t, want %t", scenario.name, got, scenario.want)
		}
	}
}
