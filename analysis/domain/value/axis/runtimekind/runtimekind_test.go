package runtimekind

import "testing"

func TestSingletonJoinAndWidenUnion(t *testing.T) {
	number := Singleton(Number)
	stringValue := Singleton(String)

	joined := Join(number, stringValue)
	if !joined.Contains(Number) || !joined.Contains(String) {
		t.Fatalf("Join(number,string) = %s, want both tags", joined)
	}
	if joined.Contains(Table) {
		t.Fatalf("Join(number,string) unexpectedly contains table: %s", joined)
	}
	if widened := Widen(number, stringValue); !Equal(widened, joined) {
		t.Fatalf("Widen(number,string) = %s, want %s", widened, joined)
	}
}

func TestIntersectContradictionGoesBottom(t *testing.T) {
	if got := Intersect(Singleton(Number), Singleton(String)); !got.IsBottom() {
		t.Fatalf("number intersect string = %s, want bottom", got)
	}
}

func TestTopWithoutTableExcludesTable(t *testing.T) {
	got := Top().Without(Table)
	if got.Contains(Table) {
		t.Fatalf("Top().Without(Table) still contains table: %s", got)
	}
	if got.IsBottom() {
		t.Fatalf("Top().Without(Table) = bottom, want non-bottom")
	}
	if got.IsTop() {
		t.Fatalf("Top().Without(Table) = top, want strict subset")
	}
}

func TestInvalidTagsFailClosedWithEvidence(t *testing.T) {
	for _, invalid := range []Tag{tagCount, Tag(255)} {
		t.Run(invalid.String(), func(t *testing.T) {
			if got := Singleton(invalid); !got.IsBottom() {
				t.Fatalf("Singleton(%s) = %s, want bottom", invalid, got)
			}
			if Top().Without(invalid) != Top() {
				t.Fatalf("Top().Without(%s) changed valid evidence", invalid)
			}
			if Singleton(Number).Without(invalid) != Singleton(Number) {
				t.Fatalf("number.Without(%s) changed valid evidence", invalid)
			}
			if Singleton(Number).Contains(invalid) {
				t.Fatalf("number unexpectedly contains invalid tag %s", invalid)
			}
			want := "runtimekind-tag(invalid:"
			if got := invalid.String(); len(got) <= len(want) || got[:len(want)] != want {
				t.Fatalf("invalid tag string = %q, want numeric evidence prefix %q", got, want)
			}
		})
	}
}

func TestTagsAccessorIsStableAndSafe(t *testing.T) {
	value := Join(Singleton(Nil), Singleton(Userdata))
	tags := value.Tags()
	if len(tags) != 2 || tags[0] != Nil || tags[1] != Userdata {
		t.Fatalf("Tags() = %v, want [nil userdata]", tags)
	}

	tags[0] = Table
	if value.Contains(Table) {
		t.Fatalf("mutating Tags result changed value: %s", value)
	}
}

func TestParseTagAcceptsLuaRuntimeTypeNames(t *testing.T) {
	tests := []struct {
		name string
		tag  Tag
	}{
		{"nil", Nil},
		{"boolean", Boolean},
		{"number", Number},
		{"string", String},
		{"table", Table},
		{"function", Function},
		{"thread", Thread},
		{"userdata", Userdata},
	}

	for _, tt := range tests {
		got, ok := ParseTag(tt.name)
		if !ok || got != tt.tag {
			t.Fatalf("ParseTag(%q) = %v/%v, want %v/true", tt.name, got, ok, tt.tag)
		}
		if got.String() != tt.name {
			t.Fatalf("ParseTag(%q).String() = %q", tt.name, got.String())
		}
	}
}

func TestParseTagRejectsUnknownRuntimeTypeName(t *testing.T) {
	if got, ok := ParseTag("integer"); ok {
		t.Fatalf("ParseTag(integer) = %v/true, want false", got)
	}
}
