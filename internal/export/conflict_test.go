package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRejectsConflictingOutputPathsBeforeReadingInput(t *testing.T) {
	dir := t.TempDir()
	markdownPath := filepath.Join(dir, "artifact.md")
	pdfPath := filepath.Join(dir, "nested", "..", "artifact.md")

	err := Run(Options{
		Input:          filepath.Join(dir, "missing.md"),
		Output:         pdfPath,
		MarkdownOutput: markdownPath,
	})
	if err == nil {
		t.Fatal("Run succeeded with --md and --pdf targeting the same path")
	}
	if !strings.Contains(err.Error(), "输出路径冲突") {
		t.Fatalf("error = %q, want an output path conflict", err)
	}
	if !strings.Contains(err.Error(), markdownPath) {
		t.Fatalf("error = %q, want the conflicting normalized path %q", err, markdownPath)
	}
	if _, err := os.Stat(markdownPath); !os.IsNotExist(err) {
		t.Fatalf("conflicting output exists: %v", err)
	}
}

func TestRunRejectsPNGExpandedOutputConflicts(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "missing.md")
	for _, tc := range []struct {
		name     string
		opts     Options
		conflict string
	}{
		{
			name: "directory page",
			opts: Options{
				Input:     input,
				Output:    filepath.Join(dir, "pages", "page-2.png"),
				PNGOutput: filepath.Join(dir, "pages"),
			},
			conflict: filepath.Join(dir, "pages", "page-2.png"),
		},
		{
			name: "multi-page file",
			opts: Options{
				Input:          input,
				MarkdownOutput: filepath.Join(dir, "resume-2.png"),
				PNGOutput:      filepath.Join(dir, "resume.png"),
			},
			conflict: filepath.Join(dir, "resume-2.png"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := Run(tc.opts)
			if err == nil {
				t.Fatal("Run succeeded with an output path reserved by --png")
			}
			if !strings.Contains(err.Error(), "输出路径冲突") || !strings.Contains(err.Error(), tc.conflict) {
				t.Fatalf("error = %q, want a conflict naming %q", err, tc.conflict)
			}
		})
	}
}
