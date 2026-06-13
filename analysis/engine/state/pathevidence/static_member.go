package pathevidence

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// ReadPathStaticMember reads a must static-member fact.
func (l Lane) ReadPathStaticMember(pathKey pathdom.PathKey) (product.Value, bool) {
	if pathKey == "" || l.staticMembersBottom {
		return product.Value{}, false
	}
	v, ok := l.staticMembers[pathKey]
	return v, ok
}

// WritePathStaticMember returns a lane with a must static-member fact recorded
// and whether this write made the surrounding state reachable.
func (l Lane) WritePathStaticMember(pathKey pathdom.PathKey, value product.Value) (Lane, bool) {
	if pathKey == "" {
		return l, false
	}
	members := clonePathValueMap(l.staticMembers)
	if members == nil {
		members = make(map[pathdom.PathKey]product.Value, 1)
	}
	members[pathKey] = value
	out := l.Reachable()
	out.staticMembers = members
	return out, true
}
