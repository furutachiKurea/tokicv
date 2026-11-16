package library

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TargetKind identifies whether a resolved target is stored in the library or
// at an external filesystem path.
type TargetKind string

// Resolved target kinds.
const (
	TargetLibrary TargetKind = "library"
	TargetFile    TargetKind = "file"
)

// Target is one resolved Resume Document source.
type Target struct {
	Kind     TargetKind
	Document Document
	Path     string
	store    *Store
}

// Read returns the exact unmodified Markdown source bytes.
func (t Target) Read() ([]byte, error) {
	switch t.Kind {
	case TargetFile:
		data, err := os.ReadFile(t.Path)
		if err != nil {
			return nil, fmt.Errorf("读取 Resume Document %q: %w", t.Path, err)
		}
		return data, nil
	case TargetLibrary:
		if t.store == nil {
			return nil, errors.New("Library 目标没有关联的 Store")
		}
		return t.store.readContent(t.Document.ID)
	default:
		return nil, fmt.Errorf("%w: 未知目标类型 %q", ErrInvalidTarget, t.Kind)
	}
}

// LayoutKey is the stable key for per-target Layout Profile overrides.
func (t Target) LayoutKey() string {
	if t.Kind == TargetLibrary {
		return "library:" + t.Document.ID
	}
	return "path:" + t.Path
}

// DisplayName is a human-readable target description.
func (t Target) DisplayName() string {
	if t.Kind == TargetLibrary {
		return fmt.Sprintf("%s (%s)", t.Document.Name, t.Document.ID)
	}
	return t.Path
}

// ResolveTarget resolves raw as an existing relative or absolute Markdown
// path, an exact library ID, or a unique library name, in that order.
func ResolveTarget(store *Store, raw string) (Target, error) {
	if store == nil {
		return Target{}, errors.New("Library Store 为空")
	}
	return store.ResolveTarget(raw)
}

// ResolveFileTarget resolves raw as an existing relative or absolute Markdown
// path without opening the library. The bool reports whether raw named a file.
func ResolveFileTarget(raw string) (Target, bool, error) {
	info, statErr := os.Stat(raw)
	switch {
	case statErr == nil && info.IsDir():
		if looksLikePath(raw) {
			return Target{}, false, fmt.Errorf("%w: %q 是目录", ErrInvalidTarget, raw)
		}
	case statErr == nil && info.Mode().IsRegular() && isMarkdownPath(raw):
		target, err := newFileTarget(raw)
		return target, err == nil, err
	case statErr == nil && looksLikePath(raw):
		return Target{}, false, fmt.Errorf("%w: %q 不是 .md 文件", ErrInvalidTarget, raw)
	case statErr != nil && !os.IsNotExist(statErr):
		return Target{}, false, fmt.Errorf("检查 Resume Document 目标 %q: %w", raw, statErr)
	}
	return Target{}, false, nil
}

// ResolveTarget resolves raw against this library.
func (s *Store) ResolveTarget(raw string) (Target, error) {
	if raw == "" {
		return Target{}, fmt.Errorf("%w: 目标为空", ErrInvalidTarget)
	}

	target, file, err := ResolveFileTarget(raw)
	if err != nil {
		return Target{}, err
	}
	if file {
		target.store = s
		return target, nil
	}

	s.mu.Lock()
	documents, err := s.loadLocked()
	s.mu.Unlock()
	if err != nil {
		return Target{}, err
	}

	for _, document := range documents {
		if document.ID == raw {
			return Target{Kind: TargetLibrary, Document: document, store: s}, nil
		}
	}

	matches := make([]Document, 0, 1)
	for _, document := range documents {
		if document.Name == raw {
			matches = append(matches, document)
		}
	}
	switch len(matches) {
	case 1:
		return Target{Kind: TargetLibrary, Document: matches[0], store: s}, nil
	case 0:
		if looksLikePath(raw) {
			return Target{}, fmt.Errorf("%w: 找不到 Markdown 文件 %q", ErrTargetNotFound, raw)
		}
		return Target{}, fmt.Errorf("%w: 找不到 Resume Document %q（ID 或名称）", ErrTargetNotFound, raw)
	default:
		return Target{}, &AmbiguousTargetError{Target: raw, Documents: matches}
	}
}

func newFileTarget(raw string) (Target, error) {
	absolute, err := filepath.Abs(raw)
	if err != nil {
		return Target{}, fmt.Errorf("解析 Markdown 路径 %q: %w", raw, err)
	}
	return Target{Kind: TargetFile, Path: filepath.Clean(absolute)}, nil
}

func looksLikePath(raw string) bool {
	return filepath.IsAbs(raw) ||
		strings.ContainsAny(raw, `/\`) ||
		isMarkdownPath(raw)
}

func isMarkdownPath(raw string) bool {
	return strings.EqualFold(filepath.Ext(raw), ".md")
}
