package factapply

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestPresenceImplicationFormalRekeyTransportsLexicalAccessWithRows(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(1901)
	const triggerSymbol, targetSymbol = symbol.ID(1901), symbol.ID(1902)
	builder := visibility.NewBuilder()
	builder.Define(point, triggerSymbol, "trigger")
	builder.Define(point, targetSymbol, "target")
	resolver := visibility.NewResolver(builder.Build())
	triggerPath := pathdom.NewPath(triggerSymbol, "trigger").Field("ready")
	targetPath := pathdom.NewPath(targetSymbol, "target").Field("value")
	trigger, triggerOK := visibility.AddressAt(resolver, point, triggerPath).RootOrVisibleKeyspaceKey()
	target, targetOK := visibility.AddressAt(resolver, point, targetPath).RootOrVisibleKeyspaceKey()
	if !triggerOK || !targetOK {
		t.Fatal("visible presence endpoints")
	}
	publication := pathevidence.NewPathPresenceImplication(trigger, presence.Present(), target, presence.Present())
	authority := NewPathSemanticAuthority(resolver, nil, nil)
	concrete, err := authority.PreparePresenceImplicationPlan(
		reg, point, []pathevidence.PathPresenceImplication{publication}, ConcretePresenceImplicationTrailingBarrier,
	)
	if err != nil {
		t.Fatal(err)
	}
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name())))
	triggerInput := formal.NewRoot(owner, 1, formal.Input)
	targetInput := formal.NewRoot(owner, 2, formal.Input)
	triggerMiddle := formal.NewRoot(owner, 1, formal.Middle)
	targetMiddle := formal.NewRoot(owner, 2, formal.Middle)
	destination := keyspace.New()
	rekey, err := domain.SealCoordinateFormalRootRekey(owner, resolver.KeySpace(), destination, []state.CoordinateFormalRootBinding{
		{Source: resolver.KeySpace().FromPath(triggerPath.RootOnly()), Target: triggerInput},
		{Source: resolver.KeySpace().FromPath(triggerPath.RootOnly()), Target: triggerMiddle, ResolverVersions: true},
		{Source: resolver.KeySpace().FromPath(targetPath.RootOnly()), Target: targetInput},
		{Source: resolver.KeySpace().FromPath(targetPath.RootOnly()), Target: targetMiddle, ResolverVersions: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	mapped, err := concrete.RekeyFormal(domain, rekey)
	if err != nil {
		t.Fatal(err)
	}
	if mapped.resolver != nil || mapped.keys != destination || !mapped.access.valid() {
		t.Fatal("formal presence plan retained concrete resolver or lost its access certificate")
	}
	if len(mapped.publications) != 1 || !mapped.access.readable(mapped.publications[0].Trigger) || !mapped.access.readable(mapped.publications[0].Target) {
		t.Fatal("formal publication and lexical access did not transport together")
	}
	inventory, err := mapped.CoordinateFactorInventory(domain)
	if err != nil {
		t.Fatal(err)
	}
	dependency, err := mapped.DependencyBlocks(domain, inventory)
	if err != nil {
		t.Fatal(err)
	}
	if !dependency.access.valid() || dependency.source.resolver != nil || len(dependency.Stages()) != 1 {
		t.Fatal("formal dependency plan dropped access or retained a resolver")
	}
}
