// Package registry provides small helpers for engine-owned ordered registries.
package registry

// Role is one canonical ordered registry entry.
type Role[K comparable, R any] struct {
	Key   K
	Value R
}

// Binding pairs a layer-owned handler with its canonical registry role.
type Binding[K comparable, R, H any] struct {
	Key     K
	Role    R
	Handler H
}

// BindOptions configures BindOrdered.
type BindOptions[K comparable, R, H any] struct {
	Owner    string
	Roles    []Role[K, R]
	Handlers map[K]H
	Valid    func(H) bool
	KeyName  func(K) string
}

// BindOrdered orders layer-owned handlers by canonical roles and rejects
// missing, invalid, or orphan handlers. Registry owners keep the role list;
// consumers keep behavior only.
func BindOrdered[K comparable, R, H any](opts BindOptions[K, R, H]) []Binding[K, R, H] {
	owner := opts.Owner
	if owner == "" {
		owner = "registry"
	}
	name := opts.KeyName
	if name == nil {
		name = func(key K) string { return "<key>" }
	}
	out := make([]Binding[K, R, H], 0, len(opts.Roles))
	seen := make(map[K]struct{}, len(opts.Roles))
	for _, role := range opts.Roles {
		handler, ok := opts.Handlers[role.Key]
		if !ok {
			panic(owner + " missing handler for " + name(role.Key))
		}
		if opts.Valid != nil && !opts.Valid(handler) {
			panic(owner + " has invalid handler for " + name(role.Key))
		}
		out = append(out, Binding[K, R, H]{
			Key:     role.Key,
			Role:    role.Value,
			Handler: handler,
		})
		seen[role.Key] = struct{}{}
	}
	for key := range opts.Handlers {
		if _, ok := seen[key]; !ok {
			panic(owner + " has orphan handler for " + name(key))
		}
	}
	return out
}
