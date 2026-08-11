package programlaw

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/parse"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	programlower "github.com/wippyai/go-lua/program/lower"
)

const lawFile = "semantic.lua"

type sourceCase struct {
	requirement Requirement
	source      string
}

// TestExactProgramSourceLaws discharges only the independently defined rows
// in Requirements.  Each case uses public parse, bind, lower, and sealed
// Program projection APIs, then Verify starts from an exact parsed source
// anchor.  No test checks lowerer package composition or treats a family count
// as a semantic witness.
func TestExactProgramSourceLaws(t *testing.T) {
	cases := sourceCases()
	assertExactDenominator(t, cases, Requirements())
	for _, sourceCase := range cases {
		t.Run(caseName(sourceCase.requirement), func(t *testing.T) {
			statements, err := parse.ParseString(sourceCase.source, lawFile)
			if err != nil {
				t.Fatal(err)
			}
			if binding := bind.BindChunk(statements); binding == nil {
				t.Fatal("public binder returned nil result")
			}
			p, err := programlower.Lower(programlower.Source{Name: lawFile, Text: []byte(sourceCase.source)})
			if err != nil {
				t.Fatal(err)
			}
			if !p.ContentID().Available() {
				t.Fatal("sealed Program has zero content identity")
			}
			if err := Verify(sourceCase.requirement, statements, p, lawFile); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func sourceCases() []sourceCase {
	return []sourceCase{
		{Requirement{Site: SiteUnary, Unary: flowkind.UnaryNeg}, "return -x"},
		{Requirement{Site: SiteUnary, Unary: flowkind.UnaryNot}, "return not x"},
		{Requirement{Site: SiteUnary, Unary: flowkind.UnaryLen}, "return #x"},
		{Requirement{Site: SiteUnary, Unary: flowkind.UnaryBitNot}, "return ~x"},

		{Requirement{Site: SiteBinary, Binary: flowkind.BinaryAdd}, "return x + y"},
		{Requirement{Site: SiteBinary, Binary: flowkind.BinarySub}, "return x - y"},
		{Requirement{Site: SiteBinary, Binary: flowkind.BinaryMul}, "return x * y"},
		{Requirement{Site: SiteBinary, Binary: flowkind.BinaryDiv}, "return x / y"},
		{Requirement{Site: SiteBinary, Binary: flowkind.BinaryIDiv}, "return x // y"},
		{Requirement{Site: SiteBinary, Binary: flowkind.BinaryMod}, "return x % y"},
		{Requirement{Site: SiteBinary, Binary: flowkind.BinaryPow}, "return x ^ y"},
		{Requirement{Site: SiteBinary, Binary: flowkind.BinaryConcat}, "return x .. y"},
		{Requirement{Site: SiteBinary, Binary: flowkind.BinaryBitAnd}, "return x & y"},
		{Requirement{Site: SiteBinary, Binary: flowkind.BinaryBitOr}, "return x | y"},
		{Requirement{Site: SiteBinary, Binary: flowkind.BinaryBitXor}, "return x ~ y"},
		{Requirement{Site: SiteBinary, Binary: flowkind.BinaryShiftLeft}, "return x << y"},
		{Requirement{Site: SiteBinary, Binary: flowkind.BinaryShiftRight}, "return x >> y"},
		{Requirement{Site: SiteBinary, Binary: flowkind.BinaryEqual}, "return x == y"},
		{Requirement{Site: SiteBinary, Binary: flowkind.BinaryNotEqual}, "return x ~= y"},
		{Requirement{Site: SiteBinary, Binary: flowkind.BinaryLess}, "return x < y"},
		{Requirement{Site: SiteBinary, Binary: flowkind.BinaryLessEqual}, "return x <= y"},
		{Requirement{Site: SiteBinary, Binary: flowkind.BinaryGreater}, "return x > y"},
		{Requirement{Site: SiteBinary, Binary: flowkind.BinaryGreaterEqual}, "return x >= y"},

		{Requirement{Site: SiteSelect, Select: flowkind.SelectAnd}, "return x and y"},
		{Requirement{Site: SiteSelect, Select: flowkind.SelectOr}, "return x or y"},

		{Requirement{Site: SiteCall, Call: CallPlain}, "local f = function(a) return a end\nreturn f(x)"},
		{Requirement{Site: SiteCall, Call: CallMethod}, "local object = {}\nfunction object:m(a) return a end\nreturn object:m(x)"},
		{Requirement{Site: SiteValues, Values: ValuesNonFinalScalar}, "local f = function() return x, y end\nreturn f(), x"},
		{Requirement{Site: SiteValues, Values: ValuesFinalOpen}, "local f = function() return x, y end\nreturn x, f()"},
		{Requirement{Site: SiteOutcome, Outcome: flowkind.OutcomeReturn}, "return x"},
		{Requirement{Site: SiteOutcome, Outcome: flowkind.OutcomeThrow}, "local f = function() end\nreturn f()"},
		{Requirement{Site: SiteOutcome, Outcome: flowkind.OutcomeYield}, "local f = function() end\nreturn f()"},
		{Requirement{Site: SiteOutcome, Outcome: flowkind.OutcomeCancel}, "local f = function() end\nreturn f()"},
	}
}

func assertExactDenominator(t *testing.T, cases []sourceCase, requirements []Requirement) {
	t.Helper()
	want := make(map[Requirement]bool, len(requirements))
	for _, requirement := range requirements {
		if want[requirement] {
			t.Fatalf("duplicate exact Program requirement %#v", requirement)
		}
		want[requirement] = true
	}
	if len(want) == 0 {
		t.Fatal("empty exact Program requirement denominator")
	}
	seen := make(map[Requirement]bool, len(cases))
	for _, sourceCase := range cases {
		if !want[sourceCase.requirement] {
			t.Fatalf("source witness has no independently required Program law %#v", sourceCase.requirement)
		}
		if seen[sourceCase.requirement] {
			t.Fatalf("duplicate source witness for Program law %#v", sourceCase.requirement)
		}
		seen[sourceCase.requirement] = true
	}
	for requirement := range want {
		if !seen[requirement] {
			t.Fatalf("missing source witness for required Program law %#v", requirement)
		}
	}
}

func caseName(requirement Requirement) string {
	return fmt.Sprintf("site-%d-unary-%d-binary-%d-select-%d-call-%d-values-%d-outcome-%d", requirement.Site, requirement.Unary, requirement.Binary, requirement.Select, requirement.Call, requirement.Values, requirement.Outcome)
}
