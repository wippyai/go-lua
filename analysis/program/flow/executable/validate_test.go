package executable

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestSealRootlessRuntimeStillRequiresProvenance(t *testing.T) {
	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyLabel] = 1
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	label := keyspace.MakeTerm(keyspace.FamilyLabel, 1)
	flow := authored.Input{Control: authored.ControlInput{Labels: []authored.Label{{Owner: body}}}}
	fixture := openSealFixture(t, "rootless-provenance.lua", counts, [][]keyspace.Term{{label}}, flow, nil, nil, nil)
	result, err := Seal(fixture.sourceView, fixture.flow, fixture.forest, fixture.control,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID(), fixture.paths)
	if err != nil {
		t.Fatalf("rootless executable.Seal: %v", err)
	}
	if result.Count() != 1 || !result.Contains(body) || result.Contains(label) {
		t.Fatalf("rootless membership = count %d body %v label %v", result.Count(), result.Contains(body), result.Contains(label))
	}
}
