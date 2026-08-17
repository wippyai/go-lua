package semanticpath

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestFunctionCellRoleRequiresAnAuthoredFunctionRow(t *testing.T) {
	body, vararg, ok := functionCellRole(authored.Functions{}, keyspace.MakeTerm(keyspace.FamilyFunction, 1))
	if ok || body != 0 || vararg != 0 {
		t.Fatalf("functionCellRole accepted an unavailable Function: body=%v vararg=%v ok=%t", body, vararg, ok)
	}
}
