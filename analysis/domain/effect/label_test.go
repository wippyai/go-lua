package effect

import "testing"

type testLabel struct {
	name string
	id   int
}

func (l testLabel) EffectLabel() {}
func (l testLabel) String() string {
	if l.name == "" {
		return "test"
	}
	return l.name
}
func (l testLabel) Equals(other Label) bool {
	o, ok := other.(testLabel)
	return ok && l.name == o.name && l.id == o.id
}

func TestParamRef_String(t *testing.T) {
	tests := []struct {
		ref  ParamRef
		want string
	}{
		{ParamRef{Index: 0}, "param[0]"},
		{ParamRef{Index: 1}, "param[1]"},
		{ParamRef{Index: -1}, "param[last]"},
	}

	for _, tt := range tests {
		got := tt.ref.String()
		if got != tt.want {
			t.Errorf("ParamRef{%d}.String() = %q, want %q", tt.ref.Index, got, tt.want)
		}
	}
}

func TestLabelInterface(t *testing.T) {
	labels := []Label{
		testLabel{name: "a"},
		testLabel{name: "b", id: 1},
	}

	for _, l := range labels {
		_ = l.String()
		_ = l.Equals(l)
	}
}

func TestMarkerMethods(t *testing.T) {
	testLabel{}.EffectLabel()
}
