package query

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	queryschema "github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
)

func TestQuerySpecIsObservationOnlyExactCallProjection(t *testing.T) {
	spec := QuerySpec()
	if spec.Family != CalleeSetResultFamily || spec.Semantic != querySemantic || spec.Codec != queryCodec {
		t.Fatalf("query identities = %q/%q/%q", spec.Family, spec.Semantic, spec.Codec)
	}
	if spec.Fold != queryschema.FoldGeneral || spec.Contract != queryContract {
		t.Fatalf("query fold = %d/%q", spec.Fold, spec.Contract)
	}
	if len(spec.Subjects) != 1 || spec.Subjects[0] != "call" {
		t.Fatalf("query subjects = %v, want exactly [call]", spec.Subjects)
	}
	if spec.Population != queryschema.PopulationObservation || spec.Projection != queryschema.ProjectionExact {
		t.Fatalf("query population/projection = %q/%q", spec.Population, spec.Projection)
	}
}

func TestObservationQueryDeclaresAgainstCallExactRead(t *testing.T) {
	builder := engine.NewSchema()
	factorRole, factorOK := vocabulary.Key("factor/call")
	semantic, semanticOK := vocabulary.Key("query/call-callee-set")
	freezer, freezerOK := vocabulary.Key("query-result/call-callee-set")
	if !factorOK || !semanticOK || !freezerOK {
		t.Fatal("Call query roles")
	}
	factor, factorOK := callowner.DeclareSchema(builder, factorRole)
	if !factorOK || factor == nil || factor.ExactRead().Kind() != engine.SchemaFormReadExact {
		t.Fatal("Call factor declaration")
	}
	fragment, fragmentOK := DeclareQuery(builder, queryschema.Declaration{
		Semantic:   semantic,
		Freezer:    freezer,
		Population: queryschema.PopulationKindObservation,
		Subjects:   queryschema.NewSubjects(map[schema.Key]axis.Cell{"call": axis.NewCell(factor)}),
	})
	if !fragmentOK || fragment == nil || !fragment.Available() {
		t.Fatal("Call observation query declaration")
	}
	sealed, sealedOK := builder.Seal()
	if !sealedOK || sealed == nil {
		t.Fatal("Call observation query schema")
	}
}

func TestObservationQueryOwnsOnlyItsDeclaredRoles(t *testing.T) {
	specs := StructureSpecs()
	if len(specs) != 3 {
		t.Fatalf("Call query declared %d roles, want semantic/codec/contract only", len(specs))
	}
	entries, collected := structure.Collect(specs)
	if !collected || len(entries) != len(specs) {
		t.Fatal("Call query role vocabulary did not collect")
	}
	for _, spelling := range []string{"query/call-callee-set", "query-result/call-callee-set", "fold-contract/call-callee-set"} {
		key := vocabulary.RoleKey(spelling)
		found := false
		for _, entry := range entries {
			if entry.Key() == key {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("Call query role %q not declared", spelling)
		}
	}
}
