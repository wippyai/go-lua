package program

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"strconv"
)

func testTableIdentity(scope, site uint64) identity.ID {
	return identity.ID{Kind: "lua.table", Site: "test-table:" + strconv.FormatUint(scope, 10), Index: site}
}
