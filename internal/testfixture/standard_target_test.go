package testfixture

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/type/channelselect"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typecall"
	"github.com/wippyai/go-lua/manifest"
)

func TestStandardLibraryTargetBindsChannelSelect(t *testing.T) {
	contract, err := StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, ok := contract.InitialBinding(channelselect.ModuleName); !ok {
		t.Fatal("channel is not an initial global")
	}
	op, ok := contract.Lookup(vocabulary.BindingSpec{
		Namespace: vocabulary.BindingModule,
		Owner:     []string{channelselect.ModuleName},
		Member:    []string{"select"},
	})
	if !ok {
		t.Fatal("channel.select is not a target operation")
	}
	_ = op
	member, status := typecall.MemberCall(channelselect.ModuleType(), "select")
	if status != typecall.MemberCallOK || !channelselect.IsSelectFunction(member) {
		t.Fatalf("MemberCall(ModuleType, select) = %v/%v", member, status)
	}
	if !typ.TypeEquals(member, channelselect.SelectFunction()) {
		t.Fatalf("select type = %v, want SelectFunction", member)
	}
	catalogue, err := manifest.Seal(manifest.Provider{
		Identity:    "testfixture.wippy.channel",
		Mount:       manifest.MountModule,
		Declaration: channelHostManifest,
	})
	if err != nil {
		t.Fatal(err)
	}
	fn, ok := catalogue.Function("channel.select")
	if !ok || !channelselect.IsSelectFunction(fn.Signature().Type) {
		t.Fatalf("catalogue channel.select = %v/%v, want SelectFunction", fn.Signature().Type, ok)
	}
}
