package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tokicv/internal/export"
	"tokicv/internal/library"
	"tokicv/internal/render"
)

func TestParseLayoutFlagsForExportAndCheck(t *testing.T) {
	args := []string{
		"--font", "serif",
		"--line-height=1.32",
		"--margin-v", "8",
		"--margin-top=7.5",
		"--margin-bottom", "8.5",
		"--section-gap", "9",
		"--item-gap=3.5",
	}
	exportCommand, err := Parse(append([]string{"export", "resume.md", "--pdf", "out.pdf"}, args...))
	if err != nil {
		t.Fatal(err)
	}
	assertLayoutPatch(t, exportCommand.Export.Layout, render.FontSerif, 1.32, 7.5, 8.5, 9, 3.5)
	if exportCommand.Export.Font != FontSerif {
		t.Fatalf("export font = %q, want serif", exportCommand.Export.Font)
	}

	checkCommand, err := Parse(append([]string{"check", "resume.md"}, args...))
	if err != nil {
		t.Fatal(err)
	}
	assertLayoutPatch(t, checkCommand.Check.Layout, render.FontSerif, 1.32, 7.5, 8.5, 9, 3.5)
	if checkCommand.Check.Font != FontSerif {
		t.Fatalf("check font = %q, want serif", checkCommand.Check.Font)
	}

	marginOnly, err := Parse([]string{"check", "resume.md", "--margin-v", "7.5"})
	if err != nil {
		t.Fatal(err)
	}
	if marginOnly.Check.Layout.MarginTopMM == nil || *marginOnly.Check.Layout.MarginTopMM != 7.5 ||
		marginOnly.Check.Layout.MarginBottomMM == nil || *marginOnly.Check.Layout.MarginBottomMM != 7.5 {
		t.Fatalf("--margin-v patch = %#v, want both margins set to 7.5mm", marginOnly.Check.Layout)
	}
}

func TestParseRejectsInvalidLayoutFlagsBeforeRendering(t *testing.T) {
	for _, tc := range []struct {
		name string
		flag string
		args []string
	}{
		{"line height below floor", "--line-height", []string{"--line-height", "1.19"}},
		{"line height above ceiling", "--line-height", []string{"--line-height=2.21"}},
		{"line height not a number", "--line-height", []string{"--line-height", "tall"}},
		{"margin below minimum", "--margin-top", []string{"--margin-top", "7.4"}},
		{"margin above maximum", "--margin-bottom", []string{"--margin-bottom=30.1"}},
		{"margin V below minimum", "--margin-v", []string{"--margin-v", "7.4"}},
		{"section gap below minimum", "--section-gap", []string{"--section-gap", "4.9"}},
		{"item gap below minimum", "--item-gap", []string{"--item-gap", "1.4"}},
		{"item gap above maximum", "--item-gap", []string{"--item-gap", "20.1"}},
		{"missing font", "--font", []string{"--font"}},
		{"missing line height", "--line-height", []string{"--line-height"}},
		{"font size", "--font-size", []string{"--font-size", "12pt"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			args := append([]string{"export", "resume.md", "--pdf", "out.pdf"}, tc.args...)
			code := Run(args, &stdout, &stderr, func(ExportRequest) error {
				t.Fatal("renderer was dispatched for invalid layout flags")
				return nil
			})
			if code != 1 {
				t.Fatalf("exit code = %d, want 1", code)
			}
			if !strings.Contains(stderr.String(), tc.flag) {
				t.Fatalf("stderr = %q, want it to mention %s", stderr.String(), tc.flag)
			}
			if (tc.name == "line height below floor" || tc.name == "line height above ceiling") &&
				!strings.Contains(stderr.String(), "1.20") {
				t.Fatalf("stderr = %q, want the readable 1.20 floor", stderr.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
		})
	}
}

func TestParseLayoutCommands(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want Command
	}{
		{
			name: "show global",
			args: []string{"layout", "show"},
			want: Command{Kind: CommandLayout, Layout: LayoutRequest{Action: LayoutShow}},
		},
		{
			name: "show target json",
			args: []string{"layout", "show", "示例文档", "--json"},
			want: Command{Kind: CommandLayout, Layout: LayoutRequest{
				Action: LayoutShow,
				Target: "示例文档",
				JSON:   true,
			}},
		},
		{
			name: "set global",
			args: []string{"layout", "set", "--font", "noto", "--line-height", "1.3"},
			want: Command{Kind: CommandLayout, Layout: LayoutRequest{
				Action: LayoutSet,
				Patch:  mustLayoutPatch(t, render.FontNoto, 1.3, 0, 0, 0, 0, false, true),
			}},
		},
		{
			name: "set target",
			args: []string{"layout", "set", "resume.md", "--margin-v", "8", "--item-gap", "3"},
			want: Command{Kind: CommandLayout, Layout: LayoutRequest{
				Action: LayoutSet,
				Target: "resume.md",
				Patch:  mustLayoutPatch(t, "", 0, 8, 8, 0, 3, true, false),
			}},
		},
		{
			name: "reset target",
			args: []string{"layout", "reset", "resume.md"},
			want: Command{Kind: CommandLayout, Layout: LayoutRequest{
				Action: LayoutReset,
				Target: "resume.md",
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.args)
			if err != nil {
				t.Fatal(err)
			}
			assertCommand(t, got, tc.want)
		})
	}

	for _, args := range [][]string{
		{"layout"},
		{"layout", "unknown"},
		{"layout", "set"},
		{"layout", "set", "--font", "comic"},
		{"layout", "set", "--line-height", "1.1"},
		{"layout", "show", "a", "b"},
		{"layout", "show", "--font", "serif"},
		{"layout", "reset", "--json"},
	} {
		if _, err := Parse(args); err == nil {
			t.Fatalf("Parse(%v) succeeded, want an error", args)
		}
	}
}

func TestLayoutCommandsPersistShowAndResetProfiles(t *testing.T) {
	store := openCLIStore(t)
	source := []byte("# 示例文档\r\n\r\n- 原始字节\r\n")
	document, err := store.Create("示例文档", source)
	if err != nil {
		t.Fatal(err)
	}
	options := RunOptions{
		Store: store,
		Export: func(ExportRequest) error {
			t.Fatal("layout command dispatched the renderer")
			return nil
		},
		Check: func(CheckRequest) (export.CheckResult, error) {
			t.Fatal("layout command dispatched the Page Fit renderer")
			return export.CheckResult{}, nil
		},
	}
	run := func(args ...string) string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		options.Stdout = &stdout
		options.Stderr = &stderr
		if code := RunWithOptions(args, options); code != 0 {
			t.Fatalf("RunWithOptions(%v) exit code = %d, stderr = %s", args, code, stderr.String())
		}
		return stdout.String()
	}

	run("layout", "set", "--font", "serif", "--line-height", "1.45", "--margin-v", "9",
		"--section-gap", "14", "--item-gap", "6")
	global := decodeLayoutView(t, run("layout", "show", "--json"))
	if global.Origin != library.LayoutOriginGlobal || global.Layout.FontStack != render.FontSerif ||
		global.Layout.LineHeight != 1.45 || global.Layout.MarginTopMM != 9 ||
		global.Layout.MarginBottomMM != 9 || global.Layout.SectionGapPX != 14 ||
		global.Layout.ItemGapPX != 6 {
		t.Fatalf("global layout view = %#v", global)
	}

	run("layout", "set", document.ID, "--font", "system", "--line-height", "1.25")
	target := decodeLayoutView(t, run("layout", "show", document.ID, "--json"))
	if target.Origin != library.LayoutOriginDocument || target.Layout.FontStack != render.FontSystem ||
		target.Layout.LineHeight != 1.25 || target.Layout.MarginTopMM != 9 {
		t.Fatalf("document layout view = %#v", target)
	}

	run("layout", "reset", document.ID)
	target = decodeLayoutView(t, run("layout", "show", document.ID, "--json"))
	if target.Origin != library.LayoutOriginGlobal || target.Layout.FontStack != render.FontSerif {
		t.Fatalf("document reset layout view = %#v", target)
	}

	run("layout", "reset")
	global = decodeLayoutView(t, run("layout", "show", "--json"))
	if global.Origin != library.LayoutOriginDefault || global.Layout != render.DefaultLayout().Normalized() {
		t.Fatalf("global reset layout view = %#v", global)
	}
	if _, got, err := store.Show(document.ID); err != nil || !bytes.Equal(got, source) {
		t.Fatalf("layout commands changed Markdown: %q, %v", got, err)
	}

	externalPath := filepath.Join(t.TempDir(), "external.md")
	if err := os.WriteFile(externalPath, source, 0o644); err != nil {
		t.Fatal(err)
	}
	run("layout", "set", externalPath, "--font", "noto")
	pathView := decodeLayoutView(t, run("layout", "show", externalPath, "--json"))
	if pathView.Origin != library.LayoutOriginPath || pathView.Layout.FontStack != render.FontNoto {
		t.Fatalf("path layout view = %#v", pathView)
	}
	run("layout", "reset", externalPath)
	pathView = decodeLayoutView(t, run("layout", "show", externalPath, "--json"))
	if pathView.Origin != library.LayoutOriginDefault {
		t.Fatalf("path reset layout view = %#v", pathView)
	}
	if got, err := os.ReadFile(externalPath); err != nil || !bytes.Equal(got, source) {
		t.Fatalf("layout commands changed external Markdown: %q, %v", got, err)
	}
}

func decodeLayoutView(t *testing.T, output string) LayoutView {
	t.Helper()
	var view LayoutView
	if err := json.Unmarshal([]byte(output), &view); err != nil {
		t.Fatalf("layout show --json = %q: %v", output, err)
	}
	return view
}

func assertLayoutPatch(t *testing.T, patch render.LayoutPatch, font render.FontStack, lineHeight, top, bottom, section, item float64) {
	t.Helper()
	if patch.FontStack == nil || *patch.FontStack != font ||
		patch.LineHeight == nil || *patch.LineHeight != lineHeight ||
		patch.MarginTopMM == nil || *patch.MarginTopMM != top ||
		patch.MarginBottomMM == nil || *patch.MarginBottomMM != bottom ||
		patch.SectionGapPX == nil || *patch.SectionGapPX != section ||
		patch.ItemGapPX == nil || *patch.ItemGapPX != item {
		t.Fatalf("layout patch = %#v", patch)
	}
}

func mustLayoutPatch(t *testing.T, font render.FontStack, lineHeight, top, bottom, section, item float64, margins, fontSet bool) render.LayoutPatch {
	t.Helper()
	patch := render.LayoutPatch{}
	if fontSet {
		patch.FontStack = &font
	}
	if lineHeight != 0 {
		patch.LineHeight = &lineHeight
	}
	if margins {
		patch.MarginTopMM = &top
		patch.MarginBottomMM = &bottom
	}
	if section != 0 {
		patch.SectionGapPX = &section
	}
	if item != 0 {
		patch.ItemGapPX = &item
	}
	return patch
}

func assertCommand(t *testing.T, got, want Command) {
	t.Helper()
	if got.Kind != want.Kind || got.Layout.Action != want.Layout.Action ||
		got.Layout.Target != want.Layout.Target || got.Layout.JSON != want.Layout.JSON ||
		got.Layout.Patch.Any() != want.Layout.Patch.Any() {
		t.Fatalf("command = %#v, want %#v", got, want)
	}
	if got.Layout.Patch.Any() {
		gotJSON, _ := json.Marshal(got.Layout.Patch)
		wantJSON, _ := json.Marshal(want.Layout.Patch)
		if string(gotJSON) != string(wantJSON) {
			t.Fatalf("layout patch = %s, want %s", gotJSON, wantJSON)
		}
	}
}
