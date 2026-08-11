package workbench

import (
	"bytes"
	"fmt"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
)

func compareLocks(expected, actual cutplan.Lock) error {
	left, err := cutplan.CanonicalJSON(expected)
	if err != nil {
		return err
	}
	right, err := cutplan.CanonicalJSON(actual)
	if err != nil {
		return err
	}
	if !bytes.Equal(left, right) {
		return fmt.Errorf("lock replay evidence changed")
	}
	return nil
}
