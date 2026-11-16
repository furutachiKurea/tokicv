package library_test

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"tokicv/internal/library"
	"tokicv/internal/render"
)

func TestResolveLayoutPrecedenceAcrossDefaultGlobalDocumentPathAndCLI(t *testing.T) {
	store := openStore(t)
	source := []byte("# 示例文档\n\n## 技能\n\n- Go\n")
	document, err := store.Create("示例文档", source)
	if err != nil {
		t.Fatal(err)
	}
	externalPath := filepath.Join(t.TempDir(), "external.md")
	if err := os.WriteFile(externalPath, source, 0o644); err != nil {
		t.Fatal(err)
	}
	libraryTarget, err := store.ResolveTarget(document.ID)
	if err != nil {
		t.Fatal(err)
	}
	externalTarget, err := store.ResolveTarget(externalPath)
	if err != nil {
		t.Fatal(err)
	}

	assertLayoutOrigin(t, store, libraryTarget, library.LayoutOriginDefault, render.DefaultLayout())

	global := render.DefaultLayout()
	global.FontStack = render.FontSerif
	global.LineHeight = 1.50
	global.MarginVMM = 12
	global.SectionGapPX = 16
	global.ItemGapPX = 8
	if err := store.SetGlobalDefault(global); err != nil {
		t.Fatal(err)
	}
	assertLayoutOrigin(t, store, libraryTarget, library.LayoutOriginGlobal, global)
	assertLayoutOrigin(t, store, externalTarget, library.LayoutOriginGlobal, global)

	documentProfile := global
	documentProfile.FontStack = render.FontNoto
	documentProfile.LineHeight = 1.35
	documentProfile.MarginTopMM = 9
	documentProfile.MarginBottomMM = 7.5
	if err := store.SetTargetProfile(libraryTarget, documentProfile); err != nil {
		t.Fatal(err)
	}
	pathProfile := documentProfile
	pathProfile.FontStack = render.FontSystem
	pathProfile.LineHeight = 1.30
	pathProfile.SectionGapPX = 9
	pathProfile.ItemGapPX = 4
	if err := store.SetTargetProfile(externalTarget, pathProfile); err != nil {
		t.Fatal(err)
	}
	assertLayoutOrigin(t, store, libraryTarget, library.LayoutOriginDocument, documentProfile)
	assertLayoutOrigin(t, store, externalTarget, library.LayoutOriginPath, pathProfile)

	font := render.FontClassic
	lineHeight := 1.25
	marginTop := 8.5
	itemGap := 2.0
	patch := render.LayoutPatch{
		FontStack:   &font,
		LineHeight:  &lineHeight,
		MarginTopMM: &marginTop,
		ItemGapPX:   &itemGap,
	}
	effective, err := store.ResolveLayout(externalTarget, patch)
	if err != nil {
		t.Fatal(err)
	}
	want := pathProfile.WithPatch(patch).Normalized()
	if effective != want {
		t.Fatalf("ResolveLayout() = %#v, want CLI > path > global > default result %#v", effective, want)
	}
	if effective.FontStack != render.FontClassic || effective.LineHeight != 1.25 ||
		effective.MarginTopMM != 8.5 || effective.ItemGapPX != 2 {
		t.Fatalf("CLI fields did not win: %#v", effective)
	}
	if effective.MarginBottomMM != pathProfile.MarginBottomMM {
		t.Fatalf("MarginBottomMM = %v, want inherited %v", effective.MarginBottomMM, pathProfile.MarginBottomMM)
	}

	invalid := 1.19
	if _, err := store.ResolveLayout(externalTarget, render.LayoutPatch{LineHeight: &invalid}); err == nil {
		t.Fatal("ResolveLayout accepted a line height below the readable floor")
	}
}

func TestLayoutProfilesPersistOutsideMarkdownSources(t *testing.T) {
	store := openStore(t)
	librarySource := []byte("# 库内\r\n\r\n- 原始字节\r\n")
	document, err := store.Create("库内", librarySource)
	if err != nil {
		t.Fatal(err)
	}
	externalSource := []byte("\xef\xbb\xbf# 外部\r\n\r\n- 原始字节")
	externalPath := filepath.Join(t.TempDir(), "external.md")
	if err := os.WriteFile(externalPath, externalSource, 0o644); err != nil {
		t.Fatal(err)
	}
	libraryTarget, err := store.ResolveTarget(document.ID)
	if err != nil {
		t.Fatal(err)
	}
	externalTarget, err := store.ResolveTarget(externalPath)
	if err != nil {
		t.Fatal(err)
	}

	global := render.DefaultLayout()
	global.FontStack = render.FontSerif
	if err := store.SetGlobalDefault(global); err != nil {
		t.Fatal(err)
	}
	documentProfile := global
	documentProfile.LineHeight = 1.30
	if err := store.SetTargetProfile(libraryTarget, documentProfile); err != nil {
		t.Fatal(err)
	}
	pathProfile := global
	pathProfile.FontStack = render.FontNoto
	if err := store.SetTargetProfile(externalTarget, pathProfile); err != nil {
		t.Fatal(err)
	}

	layoutsPath := filepath.Join(store.Dir(), "layouts.json")
	data, err := os.ReadFile(layoutsPath)
	if err != nil {
		t.Fatalf("layouts.json was not persisted: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("layouts.json is invalid JSON: %v", err)
	}
	documents := raw["documents"].(map[string]any)
	if _, ok := documents[document.ID]; !ok {
		t.Fatalf("documents profile does not use the Library ID: %#v", documents)
	}
	paths := raw["paths"].(map[string]any)
	if _, ok := paths[externalPath]; !ok {
		t.Fatalf("paths profile does not use the normalized absolute path: %#v", paths)
	}

	if got, err := os.ReadFile(externalPath); err != nil || !bytes.Equal(got, externalSource) {
		t.Fatalf("external Markdown changed: %q, %v", got, err)
	}
	if _, got, err := store.Show(document.ID); err != nil || !bytes.Equal(got, librarySource) {
		t.Fatalf("library Markdown changed: %q, %v", got, err)
	}
}

func TestTargetLayoutProfileStoresNormalizedAbsolutePath(t *testing.T) {
	store := openStore(t)
	dir := t.TempDir()
	t.Chdir(dir)
	profile := render.DefaultLayout()
	profile.FontStack = render.FontSystem
	target := library.Target{Kind: library.TargetFile, Path: "resume.md"}
	if err := store.SetTargetProfile(target, profile); err != nil {
		t.Fatal(err)
	}
	overrides, err := store.LoadLayoutOverrides()
	if err != nil {
		t.Fatal(err)
	}
	absolute := filepath.Join(dir, "resume.md")
	if got, ok := overrides.Paths[absolute]; !ok || got != profile.Normalized() {
		t.Fatalf("Paths profile for %q = %#v, want %#v", absolute, got, profile.Normalized())
	}
	resolved, err := store.ResolveLayout(target, render.LayoutPatch{})
	if err != nil {
		t.Fatal(err)
	}
	if resolved != profile.Normalized() {
		t.Fatalf("ResolveLayout(relative target) = %#v, want %#v", resolved, profile.Normalized())
	}
}

func TestLayoutReadsDoNotCreateUserDataOrLayoutsFile(t *testing.T) {
	config := filepath.Join(t.TempDir(), "missing-config")
	t.Setenv("XDG_CONFIG_HOME", config)
	store, err := library.OpenLayoutReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	externalPath := filepath.Join(t.TempDir(), "resume.md")
	if err := os.WriteFile(externalPath, []byte("# 外部\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := library.Target{Kind: library.TargetFile, Path: externalPath}

	got, err := store.ResolveLayout(target, render.LayoutPatch{})
	if err != nil {
		t.Fatal(err)
	}
	if got != render.DefaultLayout().Normalized() {
		t.Fatalf("ResolveLayout() = %#v, want the default layout", got)
	}
	if _, err := os.Stat(config); !os.IsNotExist(err) {
		t.Fatalf("layout read created PathUserData: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.Dir(), "layouts.json")); !os.IsNotExist(err) {
		t.Fatalf("layout read created layouts.json: %v", err)
	}
}

func TestLegacyLayoutValuesMigrateAcrossGlobalDocumentAndPathProfiles(t *testing.T) {
	store := openStore(t)
	document, err := store.Create("旧版排版", []byte("# 旧版排版\n"))
	if err != nil {
		t.Fatal(err)
	}
	libraryTarget, err := store.ResolveTarget(document.ID)
	if err != nil {
		t.Fatal(err)
	}
	externalPath := filepath.Join(t.TempDir(), "legacy-path.md")
	if err := os.WriteFile(externalPath, []byte("# 外部旧版\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	externalTarget, err := store.ResolveTarget(externalPath)
	if err != nil {
		t.Fatal(err)
	}
	unrelatedPath := filepath.Join(t.TempDir(), "unrelated.md")

	legacy := render.DefaultLayout()
	legacy.MarginTopMM = 6
	legacy.MarginBottomMM = 4
	legacy.MarginVMM = 4
	legacy.SectionGapPX = 4
	legacy.ItemGapPX = 1
	legacy.LineHeight = 1.35

	documentProfile := legacy
	documentProfile.FontStack = render.FontNoto
	documentProfile.MarginTopMM = 7.49
	documentProfile.MarginBottomMM = 6
	documentProfile.SectionGapPX = 4.99
	documentProfile.ItemGapPX = 1.49

	pathProfile := legacy
	pathProfile.FontStack = render.FontSerif
	pathProfile.MarginBottomMM = 7.49
	pathProfile.SectionGapPX = 0.5
	pathProfile.ItemGapPX = 0.5

	unrelatedProfile := legacy
	unrelatedProfile.FontStack = render.FontSystem
	unrelatedProfile.MarginTopMM = 4
	unrelatedProfile.SectionGapPX = 0.5
	unrelatedProfile.ItemGapPX = 0.5

	overrides := library.LayoutOverrides{
		Default: &legacy,
		Documents: map[string]render.Layout{
			document.ID: documentProfile,
		},
		Paths: map[string]render.Layout{
			externalPath:  pathProfile,
			unrelatedPath: unrelatedProfile,
		},
	}
	data, err := json.MarshalIndent(overrides, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	layoutsPath := filepath.Join(store.Dir(), "layouts.json")
	if err := os.WriteFile(layoutsPath, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(layoutsPath)
	if err != nil {
		t.Fatal(err)
	}

	global, err := store.ResolveGlobalLayout()
	if err != nil {
		t.Fatalf("ResolveGlobalLayout rejected a legacy profile: %v", err)
	}
	assertMigratedFloors(t, global.Layout)
	if global.Origin != library.LayoutOriginGlobal || global.Layout.LineHeight != 1.35 {
		t.Fatalf("global layout = %#v from %q", global.Layout, global.Origin)
	}

	libraryResolved, err := store.ResolveLayoutProfile(libraryTarget)
	if err != nil {
		t.Fatalf("ResolveLayoutProfile rejected a legacy document profile: %v", err)
	}
	assertMigratedFloors(t, libraryResolved.Layout)
	if libraryResolved.Origin != library.LayoutOriginDocument ||
		libraryResolved.Layout.FontStack != render.FontNoto {
		t.Fatalf("document layout = %#v from %q", libraryResolved.Layout, libraryResolved.Origin)
	}

	pathResolved, err := store.ResolveLayoutProfile(externalTarget)
	if err != nil {
		t.Fatalf("ResolveLayoutProfile rejected a legacy path profile: %v", err)
	}
	assertMigratedFloors(t, pathResolved.Layout)
	if pathResolved.Origin != library.LayoutOriginPath ||
		pathResolved.Layout.FontStack != render.FontSerif {
		t.Fatalf("path layout = %#v from %q", pathResolved.Layout, pathResolved.Origin)
	}

	loaded, err := store.LoadLayoutOverrides()
	if err != nil {
		t.Fatal(err)
	}
	assertMigratedFloors(t, *loaded.Default)
	assertMigratedFloors(t, loaded.Documents[document.ID])
	assertMigratedFloors(t, loaded.Paths[externalPath])
	assertMigratedFloors(t, loaded.Paths[unrelatedPath])
	if got, err := os.ReadFile(layoutsPath); err != nil || !bytes.Equal(got, before) {
		t.Fatalf("legacy layout reads changed layouts.json: %v", err)
	}

	saved := render.DefaultLayout().Normalized()
	if err := store.SetTargetProfile(libraryTarget, saved); err != nil {
		t.Fatal(err)
	}
	raw := readRawLayoutOverrides(t, layoutsPath)
	assertMigratedFloors(t, *raw.Default)
	assertMigratedFloors(t, raw.Paths[externalPath])
	assertMigratedFloors(t, raw.Paths[unrelatedPath])

	if err := store.ResetGlobalDefault(); err != nil {
		t.Fatal(err)
	}
	raw = readRawLayoutOverrides(t, layoutsPath)
	if raw.Default != nil {
		t.Fatalf("reset global default persisted %#v", raw.Default)
	}
	assertMigratedFloors(t, raw.Documents[document.ID])
	assertMigratedFloors(t, raw.Paths[externalPath])
	assertMigratedFloors(t, raw.Paths[unrelatedPath])
}

func TestLegacyLayoutMigrationKeepsInvalidValuesInvalid(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
	}{
		{
			name: "below legacy floor",
			raw:  `{"default":{"fontStack":"classic","lineHeight":1.4,"marginTopMM":3.99,"marginBottomMM":7.5,"marginVMM":7.5,"sectionGapPX":5,"itemGapPX":1.5}}`,
		},
		{
			name: "bad type",
			raw:  `{"default":{"fontStack":"classic","lineHeight":"NaN","marginTopMM":7.5,"marginBottomMM":7.5,"marginVMM":7.5,"sectionGapPX":5,"itemGapPX":1.5}}`,
		},
		{
			name: "overflowing number",
			raw:  `{"default":{"fontStack":"classic","lineHeight":1e999,"marginTopMM":7.5,"marginBottomMM":7.5,"marginVMM":7.5,"sectionGapPX":5,"itemGapPX":1.5}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := openStore(t)
			if _, err := store.Create("非法旧值", []byte("# 非法旧值\n")); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(store.Dir(), "layouts.json"), []byte(tc.raw), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := store.ResolveGlobalLayout(); err == nil {
				t.Fatal("ResolveGlobalLayout accepted a non-migratable legacy profile")
			}
		})
	}

	store := openStore(t)
	invalid := render.DefaultLayout()
	invalid.MarginTopMM = math.NaN()
	if err := store.SaveLayoutOverrides(library.LayoutOverrides{Default: &invalid}); err == nil {
		t.Fatal("SaveLayoutOverrides accepted NaN")
	}
}

func TestLayoutReadsTreatBlockedUserDataAsAbsent(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "config-file")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", blocker)
	store, err := library.OpenLayoutReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	target := library.Target{
		Kind: library.TargetFile,
		Path: filepath.Join(t.TempDir(), "resume.md"),
	}
	got, err := store.ResolveLayout(target, render.LayoutPatch{})
	if err != nil {
		t.Fatalf("ResolveLayout() failed with blocked user data: %v", err)
	}
	if got != render.DefaultLayout().Normalized() {
		t.Fatalf("ResolveLayout() = %#v, want the default layout", got)
	}
}

func TestSetAndResetLayoutProfilesFallBackCleanly(t *testing.T) {
	store := openStore(t)
	document, err := store.Create("重置", []byte("# 重置\n"))
	if err != nil {
		t.Fatal(err)
	}
	target, err := store.ResolveTarget(document.ID)
	if err != nil {
		t.Fatal(err)
	}

	global := render.DefaultLayout()
	global.FontStack = render.FontSerif
	global.LineHeight = 1.50
	if err := store.SetGlobalDefault(global); err != nil {
		t.Fatal(err)
	}
	profile := global
	profile.FontStack = render.FontSystem
	profile.LineHeight = 1.30
	if err := store.SetTargetProfile(target, profile); err != nil {
		t.Fatal(err)
	}
	assertResolvedWidth(t, store, target, render.FontSystem, 1.30)

	if err := store.ResetTargetProfile(target); err != nil {
		t.Fatal(err)
	}
	assertResolvedWidth(t, store, target, render.FontSerif, 1.50)

	if err := store.ResetGlobalDefault(); err != nil {
		t.Fatal(err)
	}
	assertResolvedWidth(t, store, target, render.FontClassic, 1.40)
}

func TestPackageResolveLayoutUsesResolvedTargetStore(t *testing.T) {
	store := openStore(t)
	document, err := store.Create("包级解析", []byte("# 包级解析\n"))
	if err != nil {
		t.Fatal(err)
	}
	target, err := store.ResolveTarget(document.ID)
	if err != nil {
		t.Fatal(err)
	}
	profile := render.DefaultLayout()
	profile.FontStack = render.FontSystem
	if err := store.SetTargetProfile(target, profile); err != nil {
		t.Fatal(err)
	}

	got, err := library.ResolveLayout(target, render.LayoutPatch{})
	if err != nil {
		t.Fatal(err)
	}
	if got != profile.Normalized() {
		t.Fatalf("ResolveLayout() = %#v, want %#v", got, profile.Normalized())
	}
}

func TestSetLayoutProfilesRejectsInvalidBoundsWithoutWriting(t *testing.T) {
	store := openStore(t)
	document, err := store.Create("无效", []byte("# 无效\n"))
	if err != nil {
		t.Fatal(err)
	}
	target, err := store.ResolveTarget(document.ID)
	if err != nil {
		t.Fatal(err)
	}
	invalid := render.DefaultLayout()
	invalid.LineHeight = 1.19

	if err := store.SetGlobalDefault(invalid); err == nil {
		t.Fatal("SetGlobalDefault accepted line-height 1.19")
	}
	if err := store.SetTargetProfile(target, invalid); err == nil {
		t.Fatal("SetTargetProfile accepted line-height 1.19")
	}
	if _, err := os.Stat(filepath.Join(store.Dir(), "layouts.json")); !os.IsNotExist(err) {
		t.Fatalf("invalid profile created layouts.json: %v", err)
	}
}

func TestResetMissingLayoutProfilesDoesNotCreateStorage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing-library")
	store, err := library.OpenAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	external := library.Target{
		Kind: library.TargetFile,
		Path: filepath.Join(t.TempDir(), "resume.md"),
	}
	if err := store.ResetGlobalDefault(); err != nil {
		t.Fatal(err)
	}
	if err := store.ResetTargetProfile(external); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("resetting missing profiles created the library: %v", err)
	}
}

func assertMigratedFloors(t *testing.T, layout render.Layout) {
	t.Helper()
	if layout.MarginTopMM < render.MinMarginMM || layout.MarginBottomMM < render.MinMarginMM ||
		layout.MarginVMM < render.MinMarginMM {
		t.Fatalf("margins were not migrated to the current floor: %#v", layout)
	}
	if layout.SectionGapPX < render.MinSectionGapPX {
		t.Fatalf("section gap was not migrated to the current floor: %#v", layout)
	}
	if layout.ItemGapPX < render.MinItemGapPX {
		t.Fatalf("item gap was not migrated to the current floor: %#v", layout)
	}
}

func readRawLayoutOverrides(t *testing.T, path string) library.LayoutOverrides {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var overrides library.LayoutOverrides
	if err := json.Unmarshal(data, &overrides); err != nil {
		t.Fatal(err)
	}
	return overrides
}

func assertLayoutOrigin(t *testing.T, store *library.Store, target library.Target, wantOrigin library.LayoutOrigin, want render.Layout) {
	t.Helper()
	resolved, err := store.ResolveLayoutProfile(target)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Origin != wantOrigin {
		t.Fatalf("Origin = %q, want %q", resolved.Origin, wantOrigin)
	}
	if resolved.Layout != want.Normalized() {
		t.Fatalf("Layout = %#v, want %#v", resolved.Layout, want.Normalized())
	}
}

func assertResolvedWidth(t *testing.T, store *library.Store, target library.Target, font render.FontStack, lineHeight float64) {
	t.Helper()
	got, err := store.ResolveLayout(target, render.LayoutPatch{})
	if err != nil {
		t.Fatal(err)
	}
	if got.FontStack != font || got.LineHeight != lineHeight {
		t.Fatalf("ResolveLayout() = %#v, want font %q and line-height %v", got, font, lineHeight)
	}
}
