package wippyv1

import (
	"fmt"
	"os"
	"path/filepath"

	manifestwire "github.com/wippyai/go-lua/manifest/wire"
)

// ManifestDataRelativePath names the checked-in wire-encoded JSON fixture for
// one transcribed module, relative to the repository root. The Go constructor
// in this package remains the source of truth; the file at this path is the
// consumable artifact GenerateManifestData derives from it.
func ManifestDataRelativePath(module string) string {
	return filepath.Join("testdata", "manifests", "wippyv1", module+".json")
}

// LoadManifestData reads the checked-in wire-encoded manifest data for one
// transcribed module and decodes it through the ordinary module-boundary
// codec, the same codec a compiled module uses to exchange a manifest.
func LoadManifestData(repository, module string) (*manifestwire.Manifest, error) {
	if repository == "" || module == "" {
		return nil, fmt.Errorf("wippyv1: empty repository root or module name")
	}
	path := filepath.Join(repository, ManifestDataRelativePath(module))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("wippyv1: read manifest data %s: %w", path, err)
	}
	decoded, err := manifestwire.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("wippyv1: decode manifest data %s: %w", path, err)
	}
	return decoded, nil
}

// GenerateManifestData re-derives the checked-in wire-encoded JSON fixture for
// every transcribed module from its Go constructor and writes it to the
// checked-in location under repository. It is the sole writer of that
// location; go:generate is its only caller, and every other consumer reads
// the written files back through LoadManifestData.
func GenerateManifestData(repository string) error {
	if repository == "" {
		return fmt.Errorf("wippyv1: empty repository root")
	}
	for _, module := range Modules() {
		encoded, err := manifestwire.Encode(module.Declaration())
		if err != nil {
			return fmt.Errorf("wippyv1: encode %s manifest data: %w", module.Name, err)
		}
		path := filepath.Join(repository, ManifestDataRelativePath(module.Name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("wippyv1: create manifest data directory for %s: %w", module.Name, err)
		}
		if err := os.WriteFile(path, encoded, 0o644); err != nil {
			return fmt.Errorf("wippyv1: write manifest data %s: %w", module.Name, err)
		}
	}
	return nil
}
