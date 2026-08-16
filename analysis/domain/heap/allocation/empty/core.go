package empty

import "github.com/wippyai/go-lua/analysis/engine"

func distinct(keys ...engine.SemanticKey) bool {
	for index, key := range keys {
		if !key.Available() {
			return false
		}
		for prior := 0; prior < index; prior++ {
			if keys[prior] == key {
				return false
			}
		}
	}
	return true
}
