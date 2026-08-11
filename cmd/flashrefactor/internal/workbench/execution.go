package workbench

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/render"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/semantic"
)

func executionEvidence(files []semantic.VirtualFile, diff []byte) (cutplan.ExecutionEvidence, error) {
	result := cutplan.ExecutionEvidence{}
	for _, file := range files {
		if file.Delete {
			result.Deleted = append(result.Deleted, file.Path)
			continue
		}
		sum := sha256.Sum256(file.Content)
		result.Outputs = append(result.Outputs, cutplan.HashPath{Path: file.Path, SHA256: hex.EncodeToString(sum[:])})
	}
	for _, output := range result.Outputs {
		result.Touched = append(result.Touched, output.Path)
	}
	result.Touched = append(result.Touched, result.Deleted...)
	sort.Strings(result.Touched)
	sort.Strings(result.Deleted)
	sort.Slice(result.Outputs, func(i, j int) bool { return result.Outputs[i].Path < result.Outputs[j].Path })
	sum := sha256.Sum256(diff)
	result.DiffSHA256 = hex.EncodeToString(sum[:])
	return result, nil
}

// renderDiff is a stable byte-level review artifact. It intentionally does
// not claim line-level semantic correspondence; typed source/target evidence
// is the authority for that separate question.
func renderDiff(inputs []render.DiffInput) ([]byte, error) {
	values := append([]render.DiffInput(nil), inputs...)
	sort.Slice(values, func(i, j int) bool { return values[i].Path < values[j].Path })
	var result bytes.Buffer
	for _, value := range values {
		if value.Path == "" {
			return nil, fmt.Errorf("diff has empty path")
		}
		fmt.Fprintf(&result, "--- %s\n", value.Path)
		if value.Delete {
			fmt.Fprintf(&result, "+++ /dev/null\n")
		} else {
			fmt.Fprintf(&result, "+++ %s\n", value.Path)
		}
		writeDiffBytes(&result, '-', value.Before)
		if !value.Delete {
			writeDiffBytes(&result, '+', value.After)
		}
	}
	return result.Bytes(), nil
}

func writeDiffBytes(result *bytes.Buffer, prefix byte, value []byte) {
	if len(value) == 0 {
		return
	}
	for _, line := range bytes.SplitAfter(value, []byte("\n")) {
		result.WriteByte(prefix)
		result.Write(line)
		if len(line) == 0 || line[len(line)-1] != '\n' {
			result.WriteByte('\n')
		}
	}
}

func cloneFiles(values []semantic.VirtualFile) []semantic.VirtualFile {
	result := make([]semantic.VirtualFile, 0, len(values))
	for _, value := range values {
		result = append(result, semantic.VirtualFile{Path: value.Path, Content: append([]byte(nil), value.Content...), Delete: value.Delete})
	}
	return result
}
