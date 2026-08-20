package position

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	bodyproof "github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/outcome"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"
)

func TestSealRejectsEqualCountForeignOwnerIdentities(t *testing.T) {
	counts := positionCounts(1, 0, 2, 2, 0, 0, 2, 0, 0, 0)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	values := []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyValues, 1), keyspace.MakeTerm(keyspace.FamilyValues, 2)}
	integers := []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyInteger, 1), keyspace.MakeTerm(keyspace.FamilyInteger, 2)}
	calls := []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyCall, 1), keyspace.MakeTerm(keyspace.FamilyCall, 2)}
	base := positionSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{calls[0], calls[1]}},
		ints:   []source.IntegerLiteral{{Owner: body, Value: 7}, {Owner: body, Value: 8}},
		static: static.Input{Contracts: staticcontracts.Input{Call: []staticcontracts.CallContract{{}, {}}}},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body}, {Owner: body}}},
			Calls:  []authored.Call{{Owner: body, Callee: integers[0], Actuals: values[0]}, {Owner: body, Callee: integers[1], Actuals: values[1]}},
		},
	}
	current := openPositionFixture(t, base)
	staticID := current.staticView.ContentID()
	moduleID := current.moduleFinalize.View().ContentID()
	if _, err := Seal(current.preimage, current.flow, current.bodies, current.forest, current.outcomes, current.entry, staticID, moduleID); err != nil {
		t.Fatalf("valid position owners rejected: %v", err)
	}

	foreignSourceSpec := base
	foreignSourceSpec.ints = []source.IntegerLiteral{{Owner: body, Value: 10}, {Owner: body, Value: 8}}
	foreignSource := openPositionFixture(t, foreignSourceSpec)
	if current.preimage.Identity().ContentID() == foreignSource.preimage.Identity().ContentID() {
		t.Fatal("foreign Source fixture did not change Source identity")
	}

	foreignFlowSpec := base
	foreignFlowSpec.flow.Calls[0].Callee = integers[1]
	foreignFlowSpec.flow.Calls[1].Callee = integers[0]
	foreignFlow := openPositionFixture(t, foreignFlowSpec)
	if current.flow.ContentID() == foreignFlow.flow.ContentID() {
		t.Fatal("foreign Flow fixture did not change Flow identity")
	}

	foreignStatic := staticID
	foreignStatic[0] ^= 0xff
	foreignModule := moduleID
	foreignModule[0] ^= 0xff
	cases := []struct {
		name    string
		pre     source.Preimage
		flow    authored.View
		bodies  *bodyproof.Result
		forest  *containment.Result
		outcome *outcome.Result
		static  identity.ContentID
		module  identity.ContentID
	}{
		{name: "Source", pre: foreignSource.preimage, flow: current.flow, bodies: current.bodies, forest: current.forest, outcome: current.outcomes, static: staticID, module: moduleID},
		{name: "Flow", pre: current.preimage, flow: foreignFlow.flow, bodies: current.bodies, forest: current.forest, outcome: current.outcomes, static: staticID, module: moduleID},
		{name: "Static", pre: current.preimage, flow: current.flow, bodies: current.bodies, forest: current.forest, outcome: current.outcomes, static: foreignStatic, module: moduleID},
		{name: "Module", pre: current.preimage, flow: current.flow, bodies: current.bodies, forest: current.forest, outcome: current.outcomes, static: staticID, module: foreignModule},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := Seal(test.pre, test.flow, test.bodies, test.forest, test.outcome, current.entry, test.static, test.module)
			if err == nil {
				t.Fatalf("Seal accepted a same-count foreign %s owner", test.name)
			}
		})
	}
}
