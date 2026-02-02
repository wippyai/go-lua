package interproc

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/api"
)

func TestStoreFactsFromResult_NilStore(t *testing.T) {
	StoreFactsFromResult(nil, nil, nil, nil)
}

func TestStoreFactsFromResult_NilResult(t *testing.T) {
	StoreFactsFromResult(nil, nil, nil, nil)
}

func TestStoreFactsFromResult_NilGraph(t *testing.T) {
	result := &api.FuncResult{}
	StoreFactsFromResult(nil, nil, result, nil)
}
