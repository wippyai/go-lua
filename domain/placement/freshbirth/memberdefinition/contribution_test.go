package memberdefinition

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	placementbase "github.com/wippyai/go-lua/domain/placement/memberdefinition"
	valuebase "github.com/wippyai/go-lua/domain/value/memberdefinition"
)

func TestFreshBirthContributionsComposeIntoTheirOwners(t *testing.T) {
	roster, ok := definition.NewRoster(
		definition.Source{Package: "value", Name: "value", Base: valuebase.StorageTransfer()},
		definition.Source{Package: "placement", Name: "placement", Base: placementbase.Storage(), Contributions: []definition.Contribution{Contribution()}},
	)
	if !ok || roster.Count() != 2 {
		t.Fatal("fresh-birth rows did not compose into their two owners")
	}
	for index := 0; index < roster.Count(); index++ {
		source, sourceOK := roster.At(index)
		composed, composedOK := source.Compose()
		if !sourceOK || !composedOK {
			t.Fatalf("owner source %d refused fresh birth", index)
		}
		if _, catalogOK := composed.Catalog(); !catalogOK {
			t.Fatalf("owner source %d produced an invalid catalog", index)
		}
	}
}
