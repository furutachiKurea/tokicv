package library

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"syscall"

	"github.com/egoist/mygo"

	"tokicv/internal/render"
)

const (
	legacyMinMarginMM = 4.0
	legacyMinGapPX    = 0.5
)

// LayoutOrigin identifies the most specific saved profile behind an effective
// Layout Profile.
type LayoutOrigin string

// Layout Profile origins.
const (
	LayoutOriginDefault  LayoutOrigin = "default"
	LayoutOriginGlobal   LayoutOrigin = "global"
	LayoutOriginDocument LayoutOrigin = "document"
	LayoutOriginPath     LayoutOrigin = "path"
)

// ResolvedLayout is an effective Layout Profile and where it came from.
type ResolvedLayout struct {
	Layout render.Layout `json:"layout"`
	Origin LayoutOrigin  `json:"origin"`
}

// LayoutOverrides keeps the global default and per-target Layout Profile
// overrides outside Markdown source files.
type LayoutOverrides struct {
	Default   *render.Layout           `json:"default,omitempty"`
	Documents map[string]render.Layout `json:"documents,omitempty"`
	Paths     map[string]render.Layout `json:"paths,omitempty"`
}

// ProfileFor returns the most specific saved Layout Profile for a target.
// CLI flags still take precedence over the returned profile.
func (overrides LayoutOverrides) ProfileFor(target Target) (render.Layout, bool) {
	if target.Kind == TargetFile {
		if profile, ok := overrides.Paths[target.Path]; ok {
			return profile, true
		}
	}
	if target.Kind == TargetLibrary {
		if profile, ok := overrides.Documents[target.Document.ID]; ok {
			return profile, true
		}
	}
	if overrides.Default != nil {
		return *overrides.Default, true
	}
	return render.Layout{}, false
}

// OpenLayoutReadOnly opens the default library store without creating
// PathUserData or its library directory.
func OpenLayoutReadOnly() (*Store, error) {
	mygo.App.SetName("tokicv")
	appData, err := mygo.App.Path(mygo.PathAppData)
	if err != nil {
		return nil, fmt.Errorf("定位 tokicv 用户数据目录: %w", err)
	}
	root := filepath.Join(appData, mygo.App.Name())
	if info, statErr := os.Stat(root); statErr == nil && info.IsDir() {
		if userData, pathErr := mygo.App.Path(mygo.PathUserData); pathErr == nil {
			root = userData
		}
	}
	return OpenAt(filepath.Join(root, "library"))
}

// ResolveGlobalLayout returns the saved global default, or DefaultLayout when
// no global default has been saved.
func (s *Store) ResolveGlobalLayout() (ResolvedLayout, error) {
	s.mu.Lock()
	overrides, err := s.loadLayoutOverridesLocked()
	s.mu.Unlock()
	if err != nil {
		return ResolvedLayout{}, err
	}
	if overrides.Default == nil {
		return ResolvedLayout{
			Layout: render.DefaultLayout().Normalized(),
			Origin: LayoutOriginDefault,
		}, nil
	}
	layout, err := normalizeSavedLayout(*overrides.Default)
	if err != nil {
		return ResolvedLayout{}, err
	}
	return ResolvedLayout{Layout: layout, Origin: LayoutOriginGlobal}, nil
}

// ResolveLayoutProfile returns the target profile after applying
// document/path override > global default > DefaultLayout.
func (s *Store) ResolveLayoutProfile(target Target) (ResolvedLayout, error) {
	target, err := normalizeLayoutTarget(target)
	if err != nil {
		return ResolvedLayout{}, err
	}

	s.mu.Lock()
	overrides, err := s.loadLayoutOverridesLocked()
	s.mu.Unlock()
	if err != nil {
		return ResolvedLayout{}, err
	}

	profile, origin := overrides.profileFor(target)
	layout, err := normalizeSavedLayout(profile)
	if err != nil {
		return ResolvedLayout{}, err
	}
	return ResolvedLayout{Layout: layout, Origin: origin}, nil
}

// ResolveLayout resolves a target through its backing Store, or through the
// default read-only store for a standalone external target.
func ResolveLayout(target Target, cli render.LayoutPatch) (render.Layout, error) {
	if target.store != nil {
		return target.store.ResolveLayout(target, cli)
	}
	store, err := OpenLayoutReadOnly()
	if err != nil {
		return render.Layout{}, err
	}
	return store.ResolveLayout(target, cli)
}

// ResolveLayout composes CLI flag > document/path override > global default >
// DefaultLayout.
func (s *Store) ResolveLayout(target Target, cli render.LayoutPatch) (render.Layout, error) {
	if err := cli.Validate(); err != nil {
		return render.Layout{}, fmt.Errorf("CLI Layout 覆盖无效: %w", err)
	}
	resolved, err := s.ResolveLayoutProfile(target)
	if err != nil {
		return render.Layout{}, err
	}
	return cli.Apply(resolved.Layout).Normalized(), nil
}

// SetGlobalDefault saves the global default Layout Profile.
func (s *Store) SetGlobalDefault(profile render.Layout) error {
	normalized, err := normalizeSavedLayout(profile)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	overrides, err := s.loadLayoutOverridesLocked()
	if err != nil {
		return err
	}
	overrides.Default = &normalized
	return s.saveLayoutOverridesLocked(overrides)
}

// ResetGlobalDefault removes the saved global default Layout Profile.
func (s *Store) ResetGlobalDefault() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	overrides, err := s.loadLayoutOverridesLocked()
	if err != nil {
		return err
	}
	if overrides.Default == nil {
		return nil
	}
	overrides.Default = nil
	return s.saveLayoutOverridesLocked(overrides)
}

// SetTargetProfile saves a Layout Profile for one Library Resume Document or
// one normalized absolute external Markdown path.
func (s *Store) SetTargetProfile(target Target, profile render.Layout) error {
	target, err := normalizeLayoutTarget(target)
	if err != nil {
		return err
	}
	normalized, err := normalizeSavedLayout(profile)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	overrides, err := s.loadLayoutOverridesLocked()
	if err != nil {
		return err
	}
	if target.Kind == TargetLibrary {
		if overrides.Documents == nil {
			overrides.Documents = make(map[string]render.Layout)
		}
		overrides.Documents[target.Document.ID] = normalized
	} else {
		if overrides.Paths == nil {
			overrides.Paths = make(map[string]render.Layout)
		}
		overrides.Paths[target.Path] = normalized
	}
	return s.saveLayoutOverridesLocked(overrides)
}

// ResetTargetProfile removes one saved document or path override.
func (s *Store) ResetTargetProfile(target Target) error {
	target, err := normalizeLayoutTarget(target)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	overrides, err := s.loadLayoutOverridesLocked()
	if err != nil {
		return err
	}
	changed := false
	if target.Kind == TargetLibrary {
		if _, ok := overrides.Documents[target.Document.ID]; ok {
			delete(overrides.Documents, target.Document.ID)
			changed = true
		}
		if len(overrides.Documents) == 0 {
			overrides.Documents = nil
		}
	} else {
		if _, ok := overrides.Paths[target.Path]; ok {
			delete(overrides.Paths, target.Path)
			changed = true
		}
		if len(overrides.Paths) == 0 {
			overrides.Paths = nil
		}
	}
	if !changed {
		return nil
	}
	return s.saveLayoutOverridesLocked(overrides)
}

// LoadLayoutOverrides reads layouts.json, treating a missing file as empty.
func (s *Store) LoadLayoutOverrides() (LayoutOverrides, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLayoutOverridesLocked()
}

func (s *Store) loadLayoutOverridesLocked() (LayoutOverrides, error) {
	path := filepath.Join(s.dir, layoutsName)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) || errors.Is(err, syscall.ENOTDIR) {
		return LayoutOverrides{}, nil
	}
	if err != nil {
		return LayoutOverrides{}, fmt.Errorf("读取排版方案: %w", err)
	}
	var overrides LayoutOverrides
	if err := json.Unmarshal(data, &overrides); err != nil {
		return LayoutOverrides{}, fmt.Errorf("解析排版方案 %q: %w", path, err)
	}
	migrateLegacyLayoutOverrides(&overrides)
	if err := validateLayoutOverrides(overrides); err != nil {
		return LayoutOverrides{}, fmt.Errorf("校验排版方案 %q: %w", path, err)
	}
	return overrides, nil
}

func migrateLegacyLayoutOverrides(overrides *LayoutOverrides) {
	if overrides.Default != nil {
		*overrides.Default = migrateLegacyLayout(*overrides.Default)
	}
	for id, profile := range overrides.Documents {
		overrides.Documents[id] = migrateLegacyLayout(profile)
	}
	for path, profile := range overrides.Paths {
		overrides.Paths[path] = migrateLegacyLayout(profile)
	}
}

func migrateLegacyLayout(layout render.Layout) render.Layout {
	layout.MarginTopMM = raiseLegacyFloor(layout.MarginTopMM, legacyMinMarginMM, render.MinMarginMM)
	layout.MarginBottomMM = raiseLegacyFloor(layout.MarginBottomMM, legacyMinMarginMM, render.MinMarginMM)
	layout.MarginVMM = raiseLegacyFloor(layout.MarginVMM, legacyMinMarginMM, render.MinMarginMM)
	layout.SectionGapPX = raiseLegacyFloor(layout.SectionGapPX, legacyMinGapPX, render.MinSectionGapPX)
	layout.ItemGapPX = raiseLegacyFloor(layout.ItemGapPX, legacyMinGapPX, render.MinItemGapPX)
	return layout
}

func raiseLegacyFloor(value, legacyMin, currentMin float64) float64 {
	if !math.IsNaN(value) && value >= legacyMin && value < currentMin {
		return currentMin
	}
	return value
}

// SaveLayoutOverrides writes layouts.json atomically.
func (s *Store) SaveLayoutOverrides(overrides LayoutOverrides) error {
	if err := validateLayoutOverrides(overrides); err != nil {
		return fmt.Errorf("排版方案无效: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLayoutOverridesLocked(overrides)
}

func (s *Store) saveLayoutOverridesLocked(overrides LayoutOverrides) error {
	data, err := json.MarshalIndent(overrides, "", "  ")
	if err != nil {
		return fmt.Errorf("编码排版方案: %w", err)
	}
	data = append(data, '\n')
	if err := writeFileAtomic(filepath.Join(s.dir, layoutsName), data); err != nil {
		return fmt.Errorf("写入排版方案: %w", err)
	}
	return nil
}

func (overrides LayoutOverrides) profileFor(target Target) (render.Layout, LayoutOrigin) {
	switch target.Kind {
	case TargetFile:
		if profile, ok := overrides.Paths[target.Path]; ok {
			return profile, LayoutOriginPath
		}
	case TargetLibrary:
		if profile, ok := overrides.Documents[target.Document.ID]; ok {
			return profile, LayoutOriginDocument
		}
	}
	if overrides.Default != nil {
		return *overrides.Default, LayoutOriginGlobal
	}
	return render.DefaultLayout(), LayoutOriginDefault
}

func normalizeLayoutTarget(target Target) (Target, error) {
	switch target.Kind {
	case TargetLibrary:
		if target.Document.ID == "" {
			return Target{}, fmt.Errorf("%w: Library 目标缺少文档 ID", ErrInvalidTarget)
		}
		return target, nil
	case TargetFile:
		if target.Path == "" {
			return Target{}, fmt.Errorf("%w: 外部目标缺少路径", ErrInvalidTarget)
		}
		absolute, err := filepath.Abs(target.Path)
		if err != nil {
			return Target{}, fmt.Errorf("规范化排版方案路径 %q: %w", target.Path, err)
		}
		target.Path = filepath.Clean(absolute)
		return target, nil
	default:
		return Target{}, fmt.Errorf("%w: 未知目标类型 %q", ErrInvalidTarget, target.Kind)
	}
}

func normalizeSavedLayout(profile render.Layout) (render.Layout, error) {
	if err := profile.Validate(); err != nil {
		return render.Layout{}, fmt.Errorf("排版方案无效: %w", err)
	}
	return profile.Normalized(), nil
}

func validateLayoutOverrides(overrides LayoutOverrides) error {
	if overrides.Default != nil {
		if err := overrides.Default.Validate(); err != nil {
			return fmt.Errorf("全局默认: %w", err)
		}
	}
	for id, profile := range overrides.Documents {
		if id == "" {
			return fmt.Errorf("文档覆盖包含空 Library ID")
		}
		if err := profile.Validate(); err != nil {
			return fmt.Errorf("文档 %q: %w", id, err)
		}
	}
	for path, profile := range overrides.Paths {
		if path == "" {
			return fmt.Errorf("路径覆盖包含空路径")
		}
		if err := profile.Validate(); err != nil {
			return fmt.Errorf("路径 %q: %w", path, err)
		}
	}
	return nil
}
