package factflow

func copyMap[K comparable, V any](in map[K]V) map[K]V {
	if len(in) == 0 {
		return nil
	}
	out := make(map[K]V, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func copyValueMap[K comparable, V any](in map[K]V, copyValue func(V) V) map[K]V {
	if len(in) == 0 {
		return nil
	}
	out := make(map[K]V, len(in))
	for key, value := range in {
		out[key] = copyValue(value)
	}
	return out
}
