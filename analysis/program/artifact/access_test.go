package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/internal/framing"
)

func TestAccessRoleIdentityFailsClosedBeforeProgramSeal(t *testing.T) {
	input := &program.Program{}
	if id := roleID("program/transformer/access", input, func(*framing.Writer) bool { return true }); id.Available() {
		t.Fatal("access role identity admitted an unavailable Program")
	}
	var writer framing.Writer
	if writeAccessSemantic(&writer, input, 1) {
		t.Fatal("access semantic writer admitted an unavailable Flow path")
	}
}
