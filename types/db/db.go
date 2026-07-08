// Package db is a legacy manifest registry facade.
package db

import "github.com/wippyai/go-lua/analysis/module/manifest"

type DB struct {
	imports map[string]*manifest.Manifest
}

func New() *DB {
	return &DB{imports: make(map[string]*manifest.Manifest)}
}

func (db *DB) Connect(path string, m *manifest.Manifest) {
	if db == nil || path == "" || m == nil {
		return
	}
	if db.imports == nil {
		db.imports = make(map[string]*manifest.Manifest)
	}
	db.imports[path] = m
}

func (db *DB) Imports() map[string]*manifest.Manifest {
	if db == nil || len(db.imports) == 0 {
		return nil
	}
	out := make(map[string]*manifest.Manifest, len(db.imports))
	for path, m := range db.imports {
		out[path] = m
	}
	return out
}
