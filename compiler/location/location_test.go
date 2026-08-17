package location

import "testing"

func TestPositionString(t *testing.T) {
	tests := []struct {
		pos  Position
		want string
	}{
		{Position{File: "test.lua", Line: 10, Column: 5}, "test.lua:10:5"},
		{Position{Line: 10, Column: 5}, "10:5"},
	}

	for _, tt := range tests {
		if got := tt.pos.String(); got != tt.want {
			t.Errorf("Position.String() = %q, want %q", got, tt.want)
		}
	}
}

func TestPositionValid(t *testing.T) {
	tests := []struct {
		pos  Position
		want bool
	}{
		{Position{Line: 1, Column: 1}, true},
		{Position{Line: 0, Column: 1}, false},
		{Position{Line: 1, Column: 0}, false},
		{Position{}, false},
	}

	for _, tt := range tests {
		if got := tt.pos.Valid(); got != tt.want {
			t.Errorf("Position.Valid() = %v, want %v for %+v", got, tt.want, tt.pos)
		}
	}
}

func TestSpan(t *testing.T) {
	tests := []struct {
		span       Span
		valid      bool
		singleLine bool
	}{
		{Span{}, false, true},
		{Span{StartLine: 1}, false, true},
		{Span{StartLine: 1, StartCol: 1}, true, true},
		{Span{StartLine: 1, StartCol: 1, EndLine: 1}, true, true},
		{Span{StartLine: 1, StartCol: 1, EndLine: 2}, true, false},
	}

	for _, tt := range tests {
		if got := tt.span.Valid(); got != tt.valid {
			t.Errorf("Span.Valid() = %v, want %v for %+v", got, tt.valid, tt.span)
		}
		if got := tt.span.SingleLine(); got != tt.singleLine {
			t.Errorf("Span.SingleLine() = %v, want %v for %+v", got, tt.singleLine, tt.span)
		}
	}
}
