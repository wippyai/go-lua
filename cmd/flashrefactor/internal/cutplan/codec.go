package cutplan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// DecodeIntent accepts exactly one JSON document and rejects unknown fields so
// a misspelled safety declaration cannot be silently ignored.
func DecodeIntent(data []byte) (Intent, error) {
	var intent Intent
	if err := decodeOne(data, &intent); err != nil {
		return Intent{}, err
	}
	if err := ValidateIntent(intent); err != nil {
		return Intent{}, err
	}
	return intent, nil
}

func DecodeLock(data []byte) (Lock, error) {
	var lock Lock
	if err := decodeOne(data, &lock); err != nil {
		return Lock{}, err
	}
	if err := ValidateLock(lock); err != nil {
		return Lock{}, err
	}
	return lock, nil
}

// CanonicalJSON serializes a validated lock in deterministic order. Intent's
// digest stays independent of generated evidence.
func CanonicalJSON(lock Lock) ([]byte, error) {
	if err := ValidateLock(lock); err != nil {
		return nil, err
	}
	canonical, err := CanonicalIntent(lock.Intent)
	if err != nil {
		return nil, err
	}
	lock.Intent = canonical
	lock.Evidence = canonicalEvidence(lock.Evidence)
	return json.MarshalIndent(lock, "", "  ")
}

// ReadPaths is the complete exact input footprint for fingerprinting.
func ReadPaths(intent Intent) []string {
	set := map[string]bool{}
	for _, operation := range intent.Operations {
		for _, path := range operation.Footprint.Read {
			set[path] = true
		}
	}
	return setPaths(set)
}

// WritePaths is the complete exact output footprint for a dry run or apply.
func WritePaths(intent Intent) []string {
	set := map[string]bool{}
	for _, operation := range intent.Operations {
		for _, path := range operation.Footprint.Write {
			set[path] = true
		}
	}
	return setPaths(set)
}

func createdPaths(intent Intent) []string {
	read := map[string]bool{}
	for _, path := range ReadPaths(intent) {
		read[path] = true
	}
	created := map[string]bool{}
	for _, path := range WritePaths(intent) {
		if !read[path] {
			created[path] = true
		}
	}
	return setPaths(created)
}

func retiredPaths(intent Intent) []string {
	retired := map[string]bool{}
	for _, operation := range intent.Operations {
		for _, edit := range operation.Edits {
			if edit.Kind == EditRetire {
				retired[edit.Retire.Source] = true
			}
		}
	}
	return setPaths(retired)
}

func setPaths(set map[string]bool) []string {
	result := make([]string, 0, len(set))
	for path := range set {
		result = append(result, path)
	}
	return sorted(result)
}

func decodeOne(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("expected one JSON document")
	}
	return nil
}
