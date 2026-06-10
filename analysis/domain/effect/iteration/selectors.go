package iteration

import "github.com/wippyai/go-lua/analysis/domain/effect"

func HasIterator(r effect.Row) bool {
	return r.Has(func(l effect.Label) bool { _, ok := l.(Iterator); return ok })
}

func GetIterator(r effect.Row) *Iterator {
	for _, l := range r.Labels {
		if iter, ok := l.(Iterator); ok {
			return &iter
		}
	}
	return nil
}

func IsIndexedIterator(r effect.Row) bool {
	iter := GetIterator(r)
	return iter != nil && iter.Kind == IterateIndexed
}

func IsKeyedIterator(r effect.Row) bool {
	iter := GetIterator(r)
	return iter != nil && iter.Kind == IterateKeyed
}
