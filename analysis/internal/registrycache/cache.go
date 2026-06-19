package registrycache

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

// Cache memoizes immutable values keyed by an axis registry pointer.
//
// The registry is intentionally the whole key: analysis domains are defined by
// the exact frozen registry instance used to construct product values.
type Cache[T any] struct {
	mu    sync.Mutex
	byReg map[*axis.Registry]T
}

// Get returns the cached value for reg, building and publishing one when absent.
// Concurrent builders may race; the first published value wins.
func (c *Cache[T]) Get(reg *axis.Registry, build func() T) T {
	c.mu.Lock()
	if c.byReg != nil {
		if value, ok := c.byReg[reg]; ok {
			c.mu.Unlock()
			return value
		}
	}
	c.mu.Unlock()

	value := build()

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.byReg == nil {
		c.byReg = make(map[*axis.Registry]T)
	}
	if existing, ok := c.byReg[reg]; ok {
		return existing
	}
	c.byReg[reg] = value
	return value
}
