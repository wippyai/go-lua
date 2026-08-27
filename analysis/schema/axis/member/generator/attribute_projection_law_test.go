package generator

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

// TestColdRendererRetainsAnAttributeProjection keeps an enriched derived row
// in the cold catalog. An Attribute is an owner-issued value carried beside a
// selected row, not a key, tag, or destination that a consumer re-derives.
func TestColdRendererRetainsAnAttributeProjection(t *testing.T) {
	source := externalProviderDefinition()
	provider := source.Relations[0].CandidateProvider
	fact := definition.GoType{PackagePath: "example/placement", Name: "Fact"}
	source.Projections = append(source.Projections, definition.Projection{
		Name: "RetainedParent", Key: "placement/store/retained-parent", Relation: "Route",
		Role: member.Attribute, Result: "Fact", CandidateProvider: provider,
		Accessor: definition.GoSymbol{PackagePath: "example/placement", Name: "Parent", Receiver: fact, ResultIndex: -1},
	})

	artifact, err := Render("placement", source)
	if err != nil {
		t.Fatalf("render attribute projection: %v", err)
	}
	if !strings.Contains(string(artifact.Cold), "Role: member.Attribute") {
		t.Fatalf("cold catalog omitted Attribute projection:\n%s", artifact.Cold)
	}
}
