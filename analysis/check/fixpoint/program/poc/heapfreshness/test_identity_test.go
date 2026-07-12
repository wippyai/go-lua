package heapfreshness

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"strconv"
)

func testTableIdentity(scope, site uint64) identity.ID {
	return identity.ID{Kind: "lua.table", Site: "test-table:" + strconv.FormatUint(scope, 10), Index: site}
}
func testBodyIdentity(scope uint64) lexicalidentity.StableLexicalBodyID {
	return lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte(strconv.FormatUint(scope, 10))))
}
