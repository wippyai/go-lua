package mapedit

// Clone returns a copy of in. Nil and empty inputs stay nil.
func Clone[K comparable, V any](in map[K]V) map[K]V {
	if len(in) == 0 {
		return nil
	}
	out := make(map[K]V, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// With returns a copy of in with key set to value.
func With[K comparable, V any](in map[K]V, key K, value V) map[K]V {
	out := Clone(in)
	if out == nil {
		out = make(map[K]V, 1)
	}
	out[key] = value
	return out
}

// Without returns a copy of in without key. Missing keys are a no-op.
func Without[K comparable, V any](in map[K]V, key K) (map[K]V, bool) {
	if _, ok := in[key]; !ok {
		return in, false
	}
	if len(in) == 1 {
		return nil, true
	}
	out := Clone(in)
	delete(out, key)
	return out, true
}
