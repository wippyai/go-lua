package returns

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
)

func TestPassThroughLabel(t *testing.T) {
	p := PassThrough{ParamIndex: 0, ReturnIndex: 0}

	var _ effect.Label = p

	s := p.String()
	if s == "" {
		t.Error("String() should not be empty")
	}
}

func TestPassThroughEquals(t *testing.T) {
	p1 := PassThrough{ParamIndex: 0, ReturnIndex: 0}
	p2 := PassThrough{ParamIndex: 0, ReturnIndex: 0}
	p3 := PassThrough{ParamIndex: 1, ReturnIndex: 0}
	p4 := PassThrough{ParamIndex: 0, ReturnIndex: 1}

	if !p1.Equals(p2) {
		t.Error("same PassThrough should be equal")
	}

	if p1.Equals(p3) {
		t.Error("different ParamIndex should not be equal")
	}

	if p1.Equals(p4) {
		t.Error("different ReturnIndex should not be equal")
	}

	if p1.Equals(Return{}) {
		t.Error("PassThrough should not equal other label types")
	}
}

func TestFlowIntoLabel(t *testing.T) {
	f := FlowInto{ParamIndex: 0, ReturnIndex: 0, TargetPath: FieldPath("inner")}

	var _ effect.Label = f

	s := f.String()
	if s == "" {
		t.Error("String() should not be empty")
	}
}

func TestFlowIntoEquals(t *testing.T) {
	f1 := FlowInto{ParamIndex: 0, ReturnIndex: 0, TargetPath: FieldPath("a")}
	f2 := FlowInto{ParamIndex: 0, ReturnIndex: 0, TargetPath: FieldPath("a")}
	f3 := FlowInto{ParamIndex: 1, ReturnIndex: 0, TargetPath: FieldPath("a")}
	f4 := FlowInto{ParamIndex: 0, ReturnIndex: 0, TargetPath: FieldPath("b")}

	if !f1.Equals(f2) {
		t.Error("same FlowInto should be equal")
	}

	if f1.Equals(f3) {
		t.Error("different ParamIndex should not be equal")
	}

	if f1.Equals(f4) {
		t.Error("different Path should not be equal")
	}

	if f1.Equals(Return{}) {
		t.Error("FlowInto should not equal other label types")
	}
}

func TestPathSuffixUsesSegmentVocabulary(t *testing.T) {
	suffix := PathSuffixFromSegments([]segment.Segment{
		{Kind: segment.SegmentField, Name: "items"},
		{Kind: segment.SegmentIndexString, Name: "primary"},
	}).Append(segment.Segment{Kind: segment.SegmentIndexInt, Index: 0})

	if got := suffix.String(); got != `.items["primary"][0]` {
		t.Fatalf("PathSuffix.String() = %q", got)
	}

	segments := suffix.Segments()
	if len(segments) != 3 {
		t.Fatalf("Segments() length = %d, want 3", len(segments))
	}
	segments[0].Name = "mutated"
	if got := suffix.String(); got != `.items["primary"][0]` {
		t.Fatalf("Segments() should return a copy, got mutated suffix %q", got)
	}
}
