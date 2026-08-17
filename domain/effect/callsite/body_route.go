package callsite

import effectfactor "github.com/wippyai/go-lua/domain/effect/factor"

// bodyRoute is the sealed runtime selector witness for one body role.
type bodyRoute struct {
	tag  uint64
	root effectfactor.Root
}
