package constraint

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

func TestVisitSemanticAffectedPathsMatchesMaterializedProjection(t *testing.T) {
	root := NewPath(cfg.SymbolID(7), "node")
	other := NewPath(cfg.SymbolID(8), "other")
	cases := []Constraint{
		Truthy{Path: root.Field("ready")},
		HasField{Path: root, Field: "kind"},
		FieldEqualsPath{Target: root, Field: "owner", Value: other.Field("id")},
		IndexEquals{Target: root.Field("items"), Key: typ.LiteralString("active"), Value: typ.LiteralBool(true)},
		IndexNotEqualsPath{Target: root.Field("items"), Key: typ.LiteralInt(3), Value: other},
		KeyOf{Table: root.Field("map"), Key: other.Field("key")},
	}

	for _, c := range cases {
		materialized := SemanticAffectedPaths(c)
		var visited []Path
		VisitSemanticAffectedPaths(c, func(path Path) bool {
			visited = append(visited, path)
			return false
		})
		if len(visited) != len(materialized) {
			t.Fatalf("%T visited %d paths, materialized %d", c, len(visited), len(materialized))
		}
		for i := range materialized {
			if !visited[i].Equal(materialized[i]) {
				t.Fatalf("%T path %d = %#v, want %#v", c, i, visited[i], materialized[i])
			}
		}
	}
}

func TestConstraintAffectedByWriteUsesSemanticReadPaths(t *testing.T) {
	root := NewPath(cfg.SymbolID(9), "node")
	other := NewPath(cfg.SymbolID(10), "other")
	fieldFact := FieldEqualsPath{Target: root, Field: "kind", Value: other.Field("kind")}
	indexFact := IndexEquals{Target: root.Field("items"), Key: typ.LiteralString("active"), Value: typ.LiteralBool(true)}

	if !ConstraintAffectedByWrite(fieldFact, root.Field("kind")) {
		t.Fatal("field discriminant write should invalidate field equality")
	}
	if ConstraintAffectedByWrite(fieldFact, root.Field("value")) {
		t.Fatal("sibling field write should not invalidate field equality")
	}
	if !ConstraintAffectedByWrite(fieldFact, other) {
		t.Fatal("value-path root write should invalidate path-valued field equality")
	}
	if !ConstraintAffectedByWrite(indexFact, root.Field("items").IndexStr("active")) {
		t.Fatal("literal index write should invalidate indexed equality")
	}
	if ConstraintAffectedByWrite(indexFact, root.Field("items").IndexStr("inactive")) {
		t.Fatal("different literal index write should not invalidate indexed equality")
	}
}
