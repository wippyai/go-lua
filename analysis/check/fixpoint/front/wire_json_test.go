package front

import (
	"errors"
	"testing"
)

func TestDecodeWireJSONDistinguishesAbsentFromMalformed(t *testing.T) {
	var destination struct {
		Version int `json:"version"`
	}
	if present, err := DecodeWireJSON(nil, &destination); present || err != nil {
		t.Fatalf("absent decode = present %v, err %v", present, err)
	}
	if present, err := DecodeWireJSON([]byte("{"), &destination); !present || !errors.Is(err, ErrWireMalformed) {
		t.Fatalf("malformed decode = present %v, err %v", present, err)
	}
	if present, err := DecodeWireJSON([]byte(`{"version":1}`), &destination); !present || err != nil || destination.Version != 1 {
		t.Fatalf("valid decode = present %v, err %v, version %d", present, err, destination.Version)
	}
}

func TestRequiredWireDecodeSurfacesMalformedValue(t *testing.T) {
	var destination any
	if err := DecodeRequiredWireJSON([]byte("{"), &destination); !errors.Is(err, ErrWireMalformed) {
		t.Fatalf("required decode error = %v, want ErrWireMalformed", err)
	}
}
