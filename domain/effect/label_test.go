package effect

import "testing"

var _ Label = testLabel{}

type testLabel struct {
	name string
	id   int
}

func (l testLabel) CapabilityID() string { return "test.Label" }
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
	tests := []struct {
		label Label
		want  string
	}{
		{label: testLabel{name: "a"}, want: "a"},
		{label: testLabel{name: "b", id: 1}, want: "b"},
	}
	for _, test := range tests {
		if got := test.label.String(); got != test.want {
			t.Errorf("Label.String() = %q, want %q", got, test.want)
		}
		if !test.label.Equals(test.label) {
			t.Errorf("Label.Equals(%#v) = false, want true", test.label)
		}
	}
}

func TestNormalizeLabelHandlesPointersAndTypedNil(t *testing.T) {
	label := testLabel{name: "ptr", id: 7}
	if got := NormalizeLabel(&label); got != label {
		t.Fatalf("NormalizeLabel(pointer) = %#v, want value %#v", got, label)
	}

	var nilLabel *testLabel
	if got := NormalizeLabel(nilLabel); got != nil {
		t.Fatalf("NormalizeLabel(typed nil) = %#v, want nil", got)
	}
	if got := NormalizeLabel(nil); got != nil {
		t.Fatalf("NormalizeLabel(nil) = %#v, want nil", got)
	}
}

func TestMarkerMethods(t *testing.T) {
	label := testLabel{name: "marker", id: 9}
	if got := label.String(); got != "marker" {
		t.Fatalf("testLabel.String() = %q, want marker", got)
	}
	if !label.Equals(testLabel{name: "marker", id: 9}) || label.Equals(testLabel{name: "marker", id: 10}) {
		t.Fatalf("testLabel equality did not retain the marker label identity")
	}
}
