package memberroster_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	memberdefinition "github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/domain/memberroster"
)

type scheduledDeathKey struct {
	axis     schema.Key
	relation schema.Key
	pkg      string
	name     string
}

// TestTheMigrationSetIsExactlyTheAuthoredDerivations is the completion
// condition of fence four.
//
// Every authored relation derivation is registered - the declaration layer
// refuses one that is not, so a straggler cannot compose - and this states the
// direction that layer cannot: no row survives its derivation. A ledger that
// kept rows for derivations nobody declares any more would overcount the work
// left, and the migration would have no honest end. The migration is finished
// when this table is empty and this law still passes.
func TestTheMigrationSetIsExactlyTheAuthoredDerivations(t *testing.T) {
	roster, rosterOK := memberroster.Composition()
	if !rosterOK {
		t.Fatal("member definition roster is not admissible")
	}
	authored := map[scheduledDeathKey]struct{}{}
	for index := 0; index < roster.Count(); index++ {
		source, _ := roster.At(index)
		composed, composedOK := source.Compose()
		if !composedOK {
			t.Fatalf("member definition source %q does not compose", source.Name)
		}
		for _, relation := range composed.Relations {
			build := relation.Derivation.Build
			if !build.Available() {
				continue
			}
			authored[scheduledDeathKey{axis: composed.Axis, relation: relation.Key, pkg: build.PackagePath, name: build.Name}] = struct{}{}
		}
	}

	registered := map[scheduledDeathKey]struct{}{}
	var stale []string
	for _, row := range memberdefinition.ScheduledDeaths() {
		key := scheduledDeathKey{axis: row.Axis, relation: row.Relation, pkg: row.Build.PackagePath, name: row.Build.Name}
		if _, duplicate := registered[key]; duplicate {
			t.Fatalf("the migration set registers %s.%s on %s twice", row.Build.PackagePath, row.Build.Name, row.Relation)
		}
		registered[key] = struct{}{}
		if _, declared := authored[key]; !declared {
			stale = append(stale, fmt.Sprintf("%s on %s/%s", row.Build.Name, row.Axis, row.Relation))
		}
	}
	if len(stale) != 0 {
		t.Fatalf("migration rows whose derivation no source declares any more:\n\t%s", strings.Join(stale, "\n\t"))
	}

	var unregistered []string
	for key := range authored {
		if _, ok := registered[key]; !ok {
			unregistered = append(unregistered, fmt.Sprintf("%s.%s on %s/%s", key.pkg, key.name, key.axis, key.relation))
		}
	}
	if len(unregistered) != 0 {
		t.Fatalf("authored derivations the migration set does not know about:\n\t%s", strings.Join(unregistered, "\n\t"))
	}
}
