package artifact

import "github.com/wippyai/go-lua/analysis/identity"

func valuesLawID(value byte) identity.ContentID {
	var id identity.ContentID
	id[0] = value
	return id
}
