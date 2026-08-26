package relcompile_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
	heapclosed "github.com/wippyai/go-lua/domain/heap/allocation/closed/program"
	heapempty "github.com/wippyai/go-lua/domain/heap/allocation/empty/program"
	valueallocation "github.com/wippyai/go-lua/domain/value/allocation/program"
	valuefreshresult "github.com/wippyai/go-lua/domain/value/freshresult/program"
)

// TestProductionCarryDeclarationsNameTheAuthoredDestinationCell is the
// declaration/compile law for the four transforming carry shapes. The
// transform signatures are installed from the owner registries by the
// relcompile census: their first input is the writer relation's address
// column, and that owner-installed column is the first cell in the carried
// tuple. The declarations therefore author scalar(0,0) explicitly; the
// compiler only transports that address and proves it against the sealed
// signature. If an owner changes the relation geometry, this law fails at the
// declaration boundary instead of allowing a positional guess to drift.
func TestProductionCarryDeclarationsNameTheAuthoredDestinationCell(t *testing.T) {
	cases := []struct {
		name string
		spec rule.Spec
	}{
		{name: "value allocation source carry", spec: valueallocation.RuleEntry()},
		{name: "value fresh result routed carry", spec: valuefreshresult.RuleEntry()},
		{name: "heap empty allocation exact carry", spec: heapempty.RuleEntry()},
		{name: "heap closed allocation exact carry", spec: heapclosed.RuleEntry()},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			surfaces := newOwners(t)
			placement := surfaces.install(test.spec)
			rules, err := relcompile.Resolve(surfaces.registry, test.spec, placement)
			if err != nil {
				t.Fatalf("resolve %s: %v", test.spec.Key, err)
			}
			var carryRule *relcompile.Rule
			for index := range rules {
				if rules[index].Carry == nil || rules[index].Carry.Transform == nil {
					continue
				}
				if carryRule != nil {
					t.Fatalf("resolved more than one transforming carry rule")
				}
				carryRule = &rules[index]
			}
			if carryRule == nil {
				t.Fatalf("resolved carry shape = %#v, want a transforming carry", rules)
			}

			transformName := relcompile.NewName(test.spec.Program.Carry.Transform.Axis, test.spec.Program.Carry.Transform.Member)
			semantic, err := surfaces.registry.SealedSignature(relcompile.Site{Path: "test.carry.signature"}, transformName)
			if err != nil {
				t.Fatalf("resolve transform signature %v: %v", transformName, err)
			}
			input, ok := semantic.InputAt(0)
			if !ok {
				t.Fatal("carry transform has no first input")
			}

			owner, err := surfaces.registry.Owner(relcompile.Site{Path: "test.carry.schema"}, schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: test.spec.Writes})
			if err != nil {
				t.Fatalf("resolve writer owner: %v", err)
			}
			schemaID, ok := model.IssueSchemaID(owner, surfaces.token("schema", relcompile.EntryName(schema.SurfaceKindRule, test.spec.Key)))
			if !ok {
				t.Fatal("issue schema identity")
			}
			declaration := surfaces.registry.Declaration(schemaID)
			var carried model.RelationSchema
			found := false
			for _, relation := range declaration.Relations {
				if relation.ID() == carryRule.Carry.Relation {
					carried, found = relation, true
					break
				}
			}
			if !found {
				t.Fatal("carry relation is not in the owner declaration")
			}
			columns := carried.Columns()
			if len(columns) == 0 || columns[0] != input.Column {
				t.Fatalf("transform input column %v is not the owner-declared first address cell %v", input.Column, columns)
			}
			want := algebra.ScalarSource(algebra.NewSlotSource(0, 0))
			if got := carryRule.Carry.Output; got != want {
				t.Fatalf("authored carry output = %#v, want signature-backed %#v", got, want)
			}

			declaration.Rules = rules
			compiled, err := relcompile.Compile(declaration)
			if err != nil {
				t.Fatalf("compile %s: %v", test.spec.Key, err)
			}
			assertApplySlotSources(t, compiled)
		})
	}
}
