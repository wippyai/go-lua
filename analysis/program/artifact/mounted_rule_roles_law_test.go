package artifact_test

import (
	"testing"

	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
)

func TestMountedRuleRoleCatalogIsClosedOrderedAndOwnerScoped(t *testing.T) {
	want := []programartifact.RuleRole{
		programartifact.RuleRoleValueSource,
		programartifact.RuleRolePackSource,
		programartifact.RuleRoleHeapIngress,
		programartifact.RuleRoleValueAllocation,
		programartifact.RuleRoleHeapEmpty,
		programartifact.RuleRoleHeapClosed,
		programartifact.RuleRoleRawGet,
		programartifact.RuleRoleRawSet,
		programartifact.RuleRoleCallDispatch,
		programartifact.RuleRoleEffectSelected,
		programartifact.RuleRoleEffectOpaque,
		programartifact.RuleRoleEffectBody,
		programartifact.RuleRoleCallActivation,
		programartifact.RuleRoleValueStorageTransfer,
		programartifact.RuleRoleValueBinaryArithmetic,
		programartifact.RuleRoleValueBinaryEquality,
		programartifact.RuleRoleValueBinaryOrder,
		programartifact.RuleRoleValuePresenceRefinement,
	}
	if got := programartifact.MountedRuleRoleCount(); got != len(want) {
		t.Fatalf("mounted role count = %d, want %d", got, len(want))
	}
	seen := make(map[programartifact.RuleRole]struct{}, len(want))
	for index, expected := range want {
		role, ok := programartifact.MountedRuleRoleAt(index)
		if !ok || role != expected {
			t.Fatalf("mounted role %d = %d/%t, want %d", index, role, ok, expected)
		}
		if _, duplicate := seen[role]; duplicate {
			t.Fatalf("mounted role %d repeated", role)
		}
		seen[role] = struct{}{}
	}
	for _, index := range []int{-1, len(want), len(want) + 1} {
		if _, ok := programartifact.MountedRuleRoleAt(index); ok {
			t.Fatalf("MountedRuleRoleAt(%d) accepted an invalid ordinal", index)
		}
	}
	for _, foreign := range []programartifact.RuleRole{
		programartifact.RuleRoleInvalid,
		programartifact.RuleRoleValueBootstrap,
		programartifact.RuleRoleHeapBootstrap,
		programartifact.RuleRole(255),
	} {
		if _, ok := seen[foreign]; ok {
			t.Fatalf("foreign role %d entered mounted vocabulary", foreign)
		}
	}
	var unavailable *programartifact.Artifact
	for _, role := range want {
		if unavailable.RuleRoleSupported(role) {
			t.Fatalf("unavailable artifact supported mounted role %d", role)
		}
	}
}
