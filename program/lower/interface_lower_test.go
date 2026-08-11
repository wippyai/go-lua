package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/static"
)

func TestLowerParsedInterfacePreservesOneAuthoredMemberSequence(t *testing.T) {
	p := parseBindLower(t, `
interface Base end
interface Shape: Base
    id: number @min(1)
    function map<T: string>(value: T): T
    enabled?: boolean
    function empty(): ()
end
return Shape()
`)
	staticView := p.Static()
	interfaces := staticView.Declarations().Interfaces()
	if interfaces.Count() != 2 {
		t.Fatalf("InterfaceCount = %d, want 2", interfaces.Count())
	}
	base, _ := interfaces.At(0)
	shape, ok := interfaces.At(1)
	if !ok {
		t.Fatal("missing Shape interface")
	}
	if count, ok := interfaces.ExtendCount(shape); !ok || count != 1 {
		t.Fatalf("extends = %d/%v, want one", count, ok)
	}
	extends, _ := interfaces.ExtendAt(shape, 0)
	if state, target, _, ok := staticView.References().Get(extends); !ok || state != static.TypeRefDeclaration || target != base {
		t.Fatalf("extends = %v/%v/%v", state, target, ok)
	}
	if count, ok := interfaces.MemberCount(shape); !ok || count != 4 {
		t.Fatalf("members = %d/%v, want 4", count, ok)
	}
	first, _ := interfaces.MemberAt(shape, 0)
	second, _ := interfaces.MemberAt(shape, 1)
	third, _ := interfaces.MemberAt(shape, 2)
	fourth, _ := interfaces.MemberAt(shape, 3)
	if first.Kind != static.InterfaceField || second.Kind != static.InterfaceMethod ||
		third.Kind != static.InterfaceField || fourth.Kind != static.InterfaceMethod {
		t.Fatalf("member kinds = %#v %#v %#v %#v, want field/method/field/method", first, second, third, fourth)
	}
	if _, _, optional, ok := staticView.Types().Fields().Get(first.Field); !ok || optional {
		t.Fatalf("first field = optional %v ok %v", optional, ok)
	}
	if _, _, optional, ok := staticView.Types().Fields().Get(third.Field); !ok || !optional {
		t.Fatalf("third field = optional %v ok %v", optional, ok)
	}
	secondName, secondNameOK := p.Source().Identity().Render(second.NameCoordinate)
	if second.Name == 0 || !secondNameOK || secondName.StartLine == 0 || second.Signature == 0 {
		t.Fatalf("second method identity = %#v", second)
	}
	if scope, _, _, returnsKnown, ok := staticView.Signatures().TypeFunctions().Get(second.Signature); !ok || scope != shape || !returnsKnown {
		t.Fatalf("map signature = scope %v known %v ok %v", scope, returnsKnown, ok)
	}
	if generics, ok := staticView.Signatures().TypeFunctions().TypeParamCount(second.Signature); !ok || generics != 1 {
		t.Fatalf("map generics = %d/%v", generics, ok)
	}
	if _, _, _, returnsKnown, ok := staticView.Signatures().TypeFunctions().Get(fourth.Signature); !ok || !returnsKnown {
		t.Fatalf("empty signature = known %v ok %v", returnsKnown, ok)
	}
	if returns, ok := staticView.Signatures().TypeFunctions().ReturnCount(fourth.Signature); !ok || returns != 0 {
		t.Fatalf("empty returns = %d/%v", returns, ok)
	}
}

func TestLowerParsedInterfaceDuplicateMembersStayDistinctAndDeterministic(t *testing.T) {
	const source = `
interface Repeated
    id: number
    id: number
    function map(): string
    function map(): string
end
`
	left := parseBindLower(t, source)
	right := parseBindLower(t, source)
	for _, p := range []*program.Program{left, right} {
		interfaces := p.Static().Declarations().Interfaces()
		iface, ok := interfaces.At(0)
		if !ok {
			t.Fatal("missing Repeated interface")
		}
		if count, ok := interfaces.MemberCount(iface); !ok || count != 4 {
			t.Fatalf("members = %d/%v, want 4", count, ok)
		}
		first, _ := interfaces.MemberAt(iface, 0)
		second, _ := interfaces.MemberAt(iface, 1)
		third, _ := interfaces.MemberAt(iface, 2)
		fourth, _ := interfaces.MemberAt(iface, 3)
		if first.Kind != static.InterfaceField || second.Kind != static.InterfaceField ||
			third.Kind != static.InterfaceMethod || fourth.Kind != static.InterfaceMethod ||
			first.Field == second.Field || third.Name != fourth.Name || third.Signature == fourth.Signature {
			t.Fatalf("duplicate member identities = %#v %#v %#v %#v", first, second, third, fourth)
		}
	}
	leftInterfaces := left.Static().Declarations().Interfaces()
	rightInterfaces := right.Static().Declarations().Interfaces()
	leftIface, _ := leftInterfaces.At(0)
	rightIface, _ := rightInterfaces.At(0)
	for index := 0; index < 4; index++ {
		leftMember, _ := leftInterfaces.MemberAt(leftIface, index)
		rightMember, _ := rightInterfaces.MemberAt(rightIface, index)
		if leftMember != rightMember {
			t.Fatalf("non-deterministic member[%d] = %#v vs %#v", index, leftMember, rightMember)
		}
	}
}
