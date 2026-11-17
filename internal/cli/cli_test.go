package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"tokicv/internal/export"
	"tokicv/internal/render"
)

func TestRunVersionAndHelpWithoutExporter(t *testing.T) {
	const version = "test-version"
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"version", []string{"--version"}, "tokicv " + version + "\n"},
		{"help", []string{"--help"}, "用法:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			called := false
			code := RunWithOptions(tc.args, RunOptions{
				Version: version,
				Stdout:  &stdout,
				Stderr:  &stderr,
				Export: func(ExportRequest) error {
					called = true
					return nil
				},
			})

			if code != 0 {
				t.Errorf("exit code = %d, want 0", code)
			}
			if called {
				t.Error("exporter was called")
			}
			if !strings.Contains(stdout.String(), tc.want) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tc.want)
			}
			if stderr.Len() != 0 {
				t.Errorf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

func TestParseExportRequest(t *testing.T) {
	cmd, err := Parse([]string{"export", "resume.md", "--pdf", "out.pdf", "--timeout", "2s"})
	if err != nil {
		t.Fatal(err)
	}
	want := Command{
		Kind: CommandExport,
		Export: ExportRequest{
			Input:   "resume.md",
			Output:  "out.pdf",
			Timeout: 2 * time.Second,
			Font:    FontClassic,
		},
	}
	if cmd != want {
		t.Fatalf("Parse() = %#v, want %#v", cmd, want)
	}
}

func TestRunExportUsesExporter(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var got ExportRequest
	code := Run([]string{"export", "resume.md", "--pdf=out.pdf"}, &stdout, &stderr, func(req ExportRequest) error {
		got = req
		return nil
	})

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if got.Input != "resume.md" || got.Output != "out.pdf" ||
		got.Timeout != DefaultTimeout || got.Font != FontClassic {
		t.Errorf("exporter request = %#v", got)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Errorf("stdout = %q, stderr = %q, want both empty", stdout.String(), stderr.String())
	}
}

func TestRunRejectsInvalidArguments(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"missing input", []string{"export", "--pdf", "out.pdf"}, "Markdown"},
		{"missing pdf", []string{"export", "resume.md"}, "--pdf"},
		{"too many inputs", []string{"export", "a.md", "b.md", "--pdf", "out.pdf"}, "一个"},
		{"unknown option", []string{"export", "resume.md", "--html", "out.html"}, "--html"},
		{"bad timeout", []string{"export", "resume.md", "--pdf", "out.pdf", "--timeout", "soon"}, "timeout"},
		{"bad font", []string{"export", "resume.md", "--pdf", "out.pdf", "--font", "comic"}, "--font"},
		{"missing font", []string{"export", "resume.md", "--pdf", "out.pdf", "--font"}, "--font"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(tc.args, &stdout, &stderr, func(ExportRequest) error {
				t.Fatal("exporter was called")
				return nil
			})

			if code != 1 {
				t.Errorf("exit code = %d, want 1", code)
			}
			if !strings.Contains(stderr.String(), tc.want) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tc.want)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
		})
	}
}

func TestParseExportFontStacks(t *testing.T) {
	for _, font := range []FontStack{FontClassic, FontNoto, FontSerif, FontSystem} {
		t.Run(string(font), func(t *testing.T) {
			cmd, err := Parse([]string{"export", "resume.md", "--pdf", "out.pdf", "--font", string(font)})
			if err != nil {
				t.Fatal(err)
			}
			if cmd.Export.Font != font {
				t.Errorf("font = %q, want %q", cmd.Export.Font, font)
			}
		})
	}
}

func TestParseExportFontEqualsForm(t *testing.T) {
	cmd, err := Parse([]string{"export", "resume.md", "--pdf=out.pdf", "--font=serif"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Export.Font != FontSerif {
		t.Errorf("font = %q, want %q", cmd.Export.Font, FontSerif)
	}
}

func TestParseFitOnePageFlags(t *testing.T) {
	export, err := Parse([]string{"export", "resume.md", "--pdf", "out.pdf", "--fit-one-page"})
	if err != nil {
		t.Fatal(err)
	}
	if !export.Export.FitOnePage {
		t.Error("export --fit-one-page did not set FitOnePage")
	}

	check, err := Parse([]string{"check", "resume.md", "--fit-one-page"})
	if err != nil {
		t.Fatal(err)
	}
	if !check.Check.FitOnePage {
		t.Error("check --fit-one-page did not set FitOnePage")
	}

	if !strings.Contains(usageText, "--fit-one-page") {
		t.Error("usage text does not document --fit-one-page")
	}
	if !strings.Contains(usageText, "按节/条目间距、行高、上下页边距的顺序") {
		t.Error("usage text does not document the Smart One-Page tightening order")
	}
}

func TestRunReportsExportFailure(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"export", "resume.md", "--pdf", "out.pdf"}, &stdout, &stderr, func(ExportRequest) error {
		return errors.New("渲染失败")
	})

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "渲染失败") {
		t.Errorf("stderr = %q, want the export error", stderr.String())
	}
}

func TestParseExportPNGDestination(t *testing.T) {
	cmd, err := Parse([]string{"export", "resume.md", "--png", "pages/"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Kind != CommandExport {
		t.Fatalf("kind = %v, want export", cmd.Kind)
	}
	if cmd.Export.PNG != "pages/" {
		t.Fatalf("PNG = %q, want pages/", cmd.Export.PNG)
	}
	if cmd.Export.Output != "" {
		t.Fatalf("PDF output = %q, want empty", cmd.Export.Output)
	}
}

func TestParseCheckRequest(t *testing.T) {
	cmd, err := Parse([]string{"check", "resume.md", "--strict-one-page", "--json", "--font", "serif", "--timeout", "3s"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Kind != CommandCheck {
		t.Fatalf("kind = %v, want check", cmd.Kind)
	}
	want := CheckRequest{
		Input:         "resume.md",
		StrictOnePage: true,
		JSON:          true,
		Font:          FontSerif,
		Timeout:       3 * time.Second,
	}
	if cmd.Check.Layout.FontStack == nil || *cmd.Check.Layout.FontStack != FontSerif {
		t.Fatalf("check layout font = %#v, want serif", cmd.Check.Layout.FontStack)
	}
	got := cmd.Check
	got.Layout = render.LayoutPatch{}
	if got != want {
		t.Fatalf("check request = %#v, want %#v", got, want)
	}
}

func TestRunCheckHumanPageFitAndStrictExitCode(t *testing.T) {
	for _, tc := range []struct {
		name         string
		strict       bool
		wantCode     int
		wantOverflow bool
	}{
		{name: "normal", wantCode: 0},
		{name: "strict overflow", strict: true, wantCode: 2, wantOverflow: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			args := []string{"check", "resume.md"}
			if tc.strict {
				args = append(args, "--strict-one-page")
			}
			code := RunWithCheck(args, &stdout, &stderr, nil, func(req CheckRequest) (export.CheckResult, error) {
				if req.Input != "resume.md" || req.StrictOnePage != tc.strict {
					t.Fatalf("check request = %#v", req)
				}
				return export.CheckResult{NumPages: 2, Overflowed: true}, nil
			})

			if code != tc.wantCode {
				t.Fatalf("exit code = %d, want %d; stdout=%q stderr=%q", code, tc.wantCode, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), "1 / 2") {
				t.Errorf("stdout = %q, want Page Fit 1 / 2", stdout.String())
			}
			if got := strings.Contains(stdout.String(), "超出单页"); got != tc.wantOverflow {
				t.Errorf("stdout overflow notice = %t, want %t: %q", got, tc.wantOverflow, stdout.String())
			}
			if stderr.Len() != 0 {
				t.Errorf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

func TestRunCheckJSONSchema(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunWithCheck([]string{"check", "resume.md", "--json"}, &stdout, &stderr, nil,
		func(CheckRequest) (export.CheckResult, error) {
			return export.CheckResult{NumPages: 1, Overflowed: false}, nil
		})

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if got := payload["numPages"]; got != float64(1) {
		t.Errorf("numPages = %#v, want 1", got)
	}
	if got := payload["overflowed"]; got != false {
		t.Errorf("overflowed = %#v, want false", got)
	}
	profile, ok := payload["appliedProfile"].(map[string]any)
	if !ok {
		t.Fatalf("appliedProfile = %#v, want an object", payload["appliedProfile"])
	}
	for _, key := range []string{
		"fontStack", "lineHeight", "marginTopMM", "marginBottomMM",
		"marginVMM", "sectionGapPX", "itemGapPX",
	} {
		if _, ok := profile[key]; !ok {
			t.Errorf("appliedProfile has no %q", key)
		}
	}
	diagnostics, ok := payload["diagnostics"].([]any)
	if !ok {
		t.Fatalf("diagnostics = %#v, want an array", payload["diagnostics"])
	}
	if len(diagnostics) != 0 {
		t.Errorf("diagnostics = %#v, want empty", diagnostics)
	}
}

func TestRunCheckReportsFailureWithExitCodeOne(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunWithCheck([]string{"check", "resume.md"}, &stdout, &stderr, nil,
		func(CheckRequest) (export.CheckResult, error) {
			return export.CheckResult{}, errors.New("渲染失败")
		})

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "渲染失败") {
		t.Errorf("stderr = %q, want the check error", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}

func TestRunCheckJSONStrictOverflowExitsTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunWithCheck([]string{"check", "resume.md", "--json", "--strict-one-page"}, &stdout, &stderr, nil,
		func(CheckRequest) (export.CheckResult, error) {
			return export.CheckResult{NumPages: 2, Overflowed: true}, nil
		})

	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if payload["numPages"] != float64(2) || payload["overflowed"] != true {
		t.Fatalf("JSON payload = %#v", payload)
	}
	if strings.Contains(stdout.String(), "超出单页") {
		t.Errorf("JSON mode emitted a human overflow notice: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunCheckPrintsReadabilityLimitNotice(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		t.Run(map[bool]string{false: "human", true: "json"}[jsonOutput], func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			args := []string{"check", "resume.md", "--fit-one-page"}
			if jsonOutput {
				args = append(args, "--json")
			}
			code := RunWithCheck(args, &stdout, &stderr, nil,
				func(CheckRequest) (export.CheckResult, error) {
					return export.CheckResult{
						NumPages:   2,
						Overflowed: true,
						Notice:     export.ReadabilityLimitNotice,
					}, nil
				})
			if code != 0 {
				t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
			}
			if !strings.Contains(stdout.String(), export.ReadabilityLimitNotice) {
				t.Fatalf("stdout = %q, want the readability-limit notice", stdout.String())
			}
			if jsonOutput {
				var payload map[string]any
				if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
					t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
				}
				if payload["notice"] != export.ReadabilityLimitNotice {
					t.Fatalf("notice = %#v, want %q", payload["notice"], export.ReadabilityLimitNotice)
				}
			}
		})
	}
}

func TestParseDesktopWorkspace(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		path string
	}{
		{"no arguments", nil, ""},
		{"markdown path", []string{"resume.md"}, "resume.md"},
		{"gui path", []string{"--gui", "resume.md"}, "resume.md"},
		{"gui equals path", []string{"--gui=resume.md"}, "resume.md"},
		{"gui without path", []string{"--gui"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command, err := Parse(tc.args)
			if err != nil {
				t.Fatal(err)
			}
			if command.Kind != CommandGUI || command.Path != tc.path {
				t.Fatalf("Parse(%q) = %#v", tc.args, command)
			}
		})
	}
	for _, args := range [][]string{
		{"resume.txt"},
		{"--gui", "resume.txt"},
		{"resume.md", "other.md"},
		{"--gui", "resume.md", "other.md"},
	} {
		if _, err := Parse(args); err == nil {
			t.Errorf("Parse(%q) accepted an invalid desktop invocation", args)
		}
	}
}

func TestRunWithGUIOpensDesktopWorkspace(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var got string
	called := false
	code := RunWithGUI([]string{"resume.md"}, &stdout, &stderr, nil, func(path string) error {
		called = true
		got = path
		return nil
	})
	if code != 0 || !called || got != "resume.md" {
		t.Fatalf("RunWithGUI() = %d, called %v with %q", code, called, got)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Errorf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}

	stderr.Reset()
	code = RunWithGUI(nil, &stdout, &stderr, nil, nil)
	if code != 1 || !strings.Contains(stderr.String(), "桌面界面") {
		t.Errorf("RunWithGUI(nil) = %d, stderr %q", code, stderr.String())
	}
}
