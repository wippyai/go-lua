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
	mu       sync.Mutex
	byReg    map[*axis.Registry]T
	inFlight map[*axis.Registry]*buildEntry[T]
}

type buildEntry[T any] struct {
	ready chan struct{}
}

// Get returns the cached value for reg, building and publishing one when absent.
// Concurrent misses for the same registry share a single in-flight build.
func (c *Cache[T]) Get(reg *axis.Registry, build func() T) T {
	return c.get(reg, func(*axis.Registry) T {
		return build()
	})
}

// GetFor returns the cached value for reg, passing reg to build only on cache
// miss. It lets hot callers avoid allocating a closure just to close over the
// registry key.
func (c *Cache[T]) GetFor(reg *axis.Registry, build func(*axis.Registry) T) T {
	return c.get(reg, build)
}

func (c *Cache[T]) get(reg *axis.Registry, build func(*axis.Registry) T) T {
	for {
		c.mu.Lock()
		if c.byReg != nil {
			if value, ok := c.byReg[reg]; ok {
				c.mu.Unlock()
				return value
			}
		}
		if entry, ok := c.inFlight[reg]; ok {
			ready := entry.ready
			c.mu.Unlock()
			<-ready
			continue
		}
		if c.inFlight == nil {
			c.inFlight = make(map[*axis.Registry]*buildEntry[T])
		}
		entry := &buildEntry[T]{ready: make(chan struct{})}
		c.inFlight[reg] = entry
		c.mu.Unlock()

		published := false
		defer func() {
			if published {
				return
			}
			c.mu.Lock()
			if c.inFlight[reg] == entry {
				delete(c.inFlight, reg)
			}
			c.mu.Unlock()
			close(entry.ready)
		}()

		value := build(reg)

		c.mu.Lock()
		if c.byReg == nil {
			c.byReg = make(map[*axis.Registry]T)
		}
		if existing, ok := c.byReg[reg]; ok {
			value = existing
		} else {
			c.byReg[reg] = value
		}
		if c.inFlight[reg] == entry {
			delete(c.inFlight, reg)
		}
		c.mu.Unlock()
		published = true
		close(entry.ready)
		return value
	}
}
