package export

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tokicv/internal/library"
	"tokicv/internal/render"
)

func TestWriteMarkdownPreservesExactSourceForLibraryAndExternalTargets(t *testing.T) {
	store := openExportStore(t)
	source := []byte("\xef\xbb\xbf# 示例文档\r\n\r\n- Go 与 **WebView**\r\n- 末尾无换行")
	created, err := store.Create("示例文档 - Go", source)
	if err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(t.TempDir(), "external.md")
	if err := os.WriteFile(external, source, 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name   string
		target string
	}{
		{name: "library ID", target: created.ID},
		{name: "library name", target: created.Name},
		{name: "relative external path", target: external},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "out.md")
			if err := WriteMarkdown(store, tc.target, output); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, source) {
				t.Fatalf("Markdown Artifact = %q, want exact source %q", got, source)
			}
		})
	}

	if got, err := os.ReadFile(external); err != nil || !bytes.Equal(got, source) {
		t.Fatalf("external source changed: %q, %v", got, err)
	}
}

func TestRunMarkdownOnlyUsesLibraryTargetWithoutRendering(t *testing.T) {
	store := openExportStore(t)
	source := []byte("# 示例文档\n\n## 技能\n\n- Go\n")
	created, err := store.Create("示例文档", source)
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "resume.md")

	err = Run(Options{
		Input:          created.Name,
		MarkdownOutput: output,
		Store:          store,
		Layout:         render.DefaultLayout(),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, source) {
		t.Fatalf("Markdown Artifact = %q, want exact source %q", got, source)
	}
}

func TestWriteMarkdownFailureLeavesNoOutput(t *testing.T) {
	store := openExportStore(t)
	if _, err := store.Create("示例文档", []byte("# 示例文档\n")); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "missing", "out.md")

	err := WriteMarkdown(store, "示例文档", output)
	if err == nil {
		t.Fatal("WriteMarkdown succeeded with a missing output directory")
	}
	if !strings.Contains(err.Error(), output) {
		t.Fatalf("error = %q, want the output path", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("output exists after failure: %v", err)
	}
}

func TestWriteMarkdownReportsMissingAndAmbiguousTargets(t *testing.T) {
	store := openExportStore(t)
	if _, err := store.Create("重名", []byte("first")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("重名", []byte("second")); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "out.md")

	for _, target := range []string{"missing", "missing.md", "重名"} {
		err := WriteMarkdown(store, target, output)
		if err == nil {
			t.Fatalf("WriteMarkdown(%q) succeeded", target)
		}
		if !strings.Contains(err.Error(), target) {
			t.Errorf("error %q does not mention target %q", err, target)
		}
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("failed resolutions created an output file: %v", err)
	}
}

func openExportStore(t *testing.T) *library.Store {
	t.Helper()
	store, err := library.OpenAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return store
}
