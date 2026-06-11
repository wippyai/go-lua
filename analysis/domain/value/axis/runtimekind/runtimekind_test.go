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
