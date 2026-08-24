package execution

import (
	"testing"

	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

type absentAuthorities struct{}

func (absentAuthorities) ValueAuthority() *valueowner.HotOwner { return nil }
func (absentAuthorities) ValueSchema() *valuedomain.Schema     { return nil }

// The positive execution law is the mounted arithmetic oracle vertical. This
// nearest negative pins the other side of the boundary: an executor cannot be
// manufactured without the Value owner's sealed axis and schema identities.
func TestArithmeticExecutionRefusesAnUnsealedOwner(t *testing.T) {
	if InstallFamily(nil, nil, absentAuthorities{}) {
		t.Fatal("arithmetic executor admitted without a sealed Value owner")
	}
	if executor := (&exactProductFamily{}).NewExecutor(nil); executor != nil {
		t.Fatal("arithmetic family minted an executor without sealed state")
	}
}
