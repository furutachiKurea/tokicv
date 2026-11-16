package export

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestResolvePNGPathsForMultiPagePNGFile(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "resume.png")

	got, err := ResolvePNGPaths(dest, 3)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(filepath.Dir(dest), "resume-1.png"),
		filepath.Join(filepath.Dir(dest), "resume-2.png"),
		filepath.Join(filepath.Dir(dest), "resume-3.png"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ResolvePNGPaths() = %v, want %v", got, want)
	}
}

func TestResolvePNGPathsForSinglePagePNGFile(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "resume.png")

	got, err := ResolvePNGPaths(dest, 1)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{dest}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ResolvePNGPaths() = %v, want %v", got, want)
	}
}

func TestResolvePNGPathsForDirectories(t *testing.T) {
	existing := t.TempDir()
	missingSlash := filepath.Join(t.TempDir(), "frames") + string(filepath.Separator)
	missingPlain := filepath.Join(t.TempDir(), "frames")

	for _, tc := range []struct {
		name string
		dest string
	}{
		{name: "existing directory", dest: existing},
		{name: "trailing slash", dest: missingSlash},
		{name: "no extension", dest: missingPlain},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolvePNGPaths(tc.dest, 2)
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Clean(tc.dest)
			want := []string{
				filepath.Join(dir, "page-1.png"),
				filepath.Join(dir, "page-2.png"),
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("ResolvePNGPaths() = %v, want %v", got, want)
			}
		})
	}
}

func TestWritePNGArtifactsLeavesNoPartialPagesOnFailure(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "page-1.png"), []byte("old first"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "page-2.png"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "page-3.png"), []byte("old third"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := writePNGArtifacts(dir, [][]byte{[]byte("first"), []byte("second")})
	if err == nil {
		t.Fatal("writePNGArtifacts() succeeded with a directory as a page target")
	}
	if got, err := os.ReadFile(filepath.Join(dir, "page-1.png")); err != nil || string(got) != "old first" {
		t.Fatalf("page-1.png changed after a failed multi-page write: %q, %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "page-3.png")); err != nil || string(got) != "old third" {
		t.Fatalf("page-3.png changed after a failed multi-page write: %q, %v", got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "page-1.png" && entry.Name() != "page-2.png" && entry.Name() != "page-3.png" {
			t.Errorf("unexpected file remains after failure: %s", entry.Name())
		}
	}
}

func TestWritePNGArtifactsRemovesStalePagesAfterRepeatExport(t *testing.T) {
	t.Run("directory three pages to one", func(t *testing.T) {
		dir := t.TempDir()
		pages := [][]byte{[]byte("one"), []byte("two"), []byte("three")}
		if err := writePNGArtifacts(dir, pages); err != nil {
			t.Fatal(err)
		}
		if err := writePNGArtifacts(dir, pages[:1]); err != nil {
			t.Fatal(err)
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 || entries[0].Name() != "page-1.png" {
			t.Fatalf("directory after repeat export = %v, want only page-1.png", entryNames(entries))
		}
		if got, err := os.ReadFile(filepath.Join(dir, "page-1.png")); err != nil || string(got) != "one" {
			t.Fatalf("page-1.png = %q, %v", got, err)
		}
	})

	t.Run("file page count changes", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "resume.png")
		if err := writePNGArtifacts(dest, [][]byte{[]byte("single")}); err != nil {
			t.Fatal(err)
		}
		if err := writePNGArtifacts(dest, [][]byte{[]byte("first"), []byte("second")}); err != nil {
			t.Fatal(err)
		}

		if _, err := os.Stat(dest); !os.IsNotExist(err) {
			t.Fatalf("single-page file remains after a two-page export: %v", err)
		}
		names := []string{"resume-1.png", "resume-2.png"}
		for index, want := range []string{"first", "second"} {
			path := filepath.Join(filepath.Dir(dest), names[index])
			if got, err := os.ReadFile(path); err != nil || string(got) != want {
				t.Fatalf("%s = %q, %v", filepath.Base(path), got, err)
			}
		}

		if err := writePNGArtifacts(dest, [][]byte{[]byte("latest")}); err != nil {
			t.Fatal(err)
		}
		if got, err := os.ReadFile(dest); err != nil || string(got) != "latest" {
			t.Fatalf("resume.png = %q, %v", got, err)
		}
		for _, name := range []string{"resume-1.png", "resume-2.png"} {
			if _, err := os.Stat(filepath.Join(filepath.Dir(dest), name)); !os.IsNotExist(err) {
				t.Fatalf("%s remains after shrinking to one page: %v", name, err)
			}
		}
	})
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, len(entries))
	for index, entry := range entries {
		names[index] = entry.Name()
	}
	return names
}

func TestStageAtomicRemovesTempOnWriteFailure(t *testing.T) {
	dir := t.TempDir()
	_, err := stageAtomicWith(filepath.Join(dir, "resume.pdf"), func(*os.File) error {
		return errors.New("simulated write failure")
	})
	if err == nil {
		t.Fatal("stageAtomicWith() succeeded after a write failure")
	}

	leftovers, err := filepath.Glob(filepath.Join(dir, ".*.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(leftovers) != 0 {
		t.Fatalf("temporary files remain after a write failure: %v", leftovers)
	}
}

func TestStageAtomicRejectsDirectoryTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "resume.md")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := stageAtomic(target, []byte("content")); err == nil {
		t.Fatal("stageAtomic() succeeded with a directory as its target")
	}
	leftovers, err := filepath.Glob(filepath.Join(dir, ".*.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(leftovers) != 0 {
		t.Fatalf("temporary files remain after rejecting a directory target: %v", leftovers)
	}
}
