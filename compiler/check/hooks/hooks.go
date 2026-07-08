// Package hooks exposes legacy checker hook options as no-op adapters.
package hooks

import "github.com/wippyai/go-lua/compiler/check"

func WithAssign() check.Option { return func(*check.Checker) {} }

func WithReturn() check.Option { return func(*check.Checker) {} }

func WithCall() check.Option { return func(*check.Checker) {} }

func WithField() check.Option { return func(*check.Checker) {} }
