package typ

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/type/annotation"
)

func TestAnnotated_String(t *testing.T) {
	tests := []struct {
		name        string
		inner       Type
		annotations []annotation.Annotation
		want        string
	}{
		{
			name:        "number with min",
			inner:       Number,
			annotations: []annotation.Annotation{{Name: "min", Arg: annotation.Float64Arg(0)}},
			want:        "number @min(0)",
		},
		{
			name:        "number with min and max",
			inner:       Number,
			annotations: []annotation.Annotation{{Name: "min", Arg: annotation.Float64Arg(0)}, {Name: "max", Arg: annotation.Float64Arg(100)}},
			want:        "number @min(0) @max(100)",
		},
		{
			name:        "string with pattern",
			inner:       String,
			annotations: []annotation.Annotation{{Name: "pattern", Arg: annotation.StringArg("^.+@.+$")}},
			want:        `string @pattern("^.+@.+$")`,
		},
		{
			name:        "integer annotation no args",
			inner:       Number,
			annotations: []annotation.Annotation{{Name: "integer"}},
			want:        "number @integer",
		},
		{
			name:        "int arg",
			inner:       Number,
			annotations: []annotation.Annotation{{Name: "min", Arg: annotation.IntArg(5)}},
			want:        "number @min(5)",
		},
		{
			name:        "int64 arg",
			inner:       Number,
			annotations: []annotation.Annotation{{Name: "min", Arg: annotation.Int64Arg(10)}},
			want:        "number @min(10)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ann := NewAnnotated(tt.inner, tt.annotations)
			if ann.String() != tt.want {
				t.Errorf("got %q, want %q", ann.String(), tt.want)
			}
		})
	}
}

func TestAnnotated_Kind(t *testing.T) {
	ann := NewAnnotated(Number, []annotation.Annotation{{Name: "min", Arg: annotation.Float64Arg(0)}})
	if ann.Kind() != Number.Kind() {
		t.Errorf("annotated should preserve inner kind")
	}
}

func TestAnnotated_Hash(t *testing.T) {
	ann1 := NewAnnotated(Number, []annotation.Annotation{{Name: "min", Arg: annotation.Float64Arg(0)}})
	ann2 := NewAnnotated(Number, []annotation.Annotation{{Name: "min", Arg: annotation.Float64Arg(0)}})
	ann3 := NewAnnotated(Number, []annotation.Annotation{{Name: "max", Arg: annotation.Float64Arg(100)}})
	ann4 := NewAnnotated(Number, []annotation.Annotation{{Name: "min", Arg: annotation.Float64Arg(1)}})

	if ann1.Hash() != ann2.Hash() {
		t.Error("same annotations should have same hash")
	}
	if ann1.Hash() == ann3.Hash() {
		t.Error("different annotations should have different hash")
	}
	if ann1.Hash() == ann4.Hash() {
		t.Error("different annotation args should have different hash")
	}
}

func TestAnnotated_Equals(t *testing.T) {
	ann1 := NewAnnotated(Number, []annotation.Annotation{{Name: "min", Arg: annotation.Float64Arg(0)}})
	ann2 := NewAnnotated(Number, []annotation.Annotation{{Name: "min", Arg: annotation.Float64Arg(0)}})
	ann3 := NewAnnotated(Number, []annotation.Annotation{{Name: "max", Arg: annotation.Float64Arg(100)}})
	ann4 := NewAnnotated(String, []annotation.Annotation{{Name: "min", Arg: annotation.Float64Arg(0)}})
	ann5 := NewAnnotated(Number, []annotation.Annotation{{Name: "min", Arg: annotation.Float64Arg(1)}})

	if !ann1.Equals(ann2) {
		t.Error("same annotated types should be equal")
	}
	if ann1.Equals(ann3) {
		t.Error("different annotations should not be equal")
	}
	if ann1.Equals(ann4) {
		t.Error("different inner types should not be equal")
	}
	if ann1.Equals(ann5) {
		t.Error("different annotation args should not be equal")
	}
	if ann1.Equals(Number) {
		t.Error("annotated should not equal non-annotated")
	}
}

func TestNewAnnotated_EmptyReturnsInner(t *testing.T) {
	result := NewAnnotated(Number, nil)
	if result != Number {
		t.Error("empty annotations should return inner type")
	}

	result = NewAnnotated(Number, []annotation.Annotation{})
	if result != Number {
		t.Error("empty annotations slice should return inner type")
	}
}

func TestNewAnnotated_NilInner(t *testing.T) {
	result := NewAnnotated(nil, []annotation.Annotation{{Name: "min", Arg: annotation.Float64Arg(0)}})
	if a, ok := result.(*Annotated); !ok || a.Inner != Unknown {
		t.Error("nil inner should become Unknown")
	}
}
