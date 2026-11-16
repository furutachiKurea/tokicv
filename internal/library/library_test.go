package library_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"

	"tokicv/internal/library"
	"tokicv/internal/render"
)

func TestOpenUsesPathUserData(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })

	store, err := library.Open()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "library"); store.Dir() != want {
		t.Fatalf("Store.Dir() = %q, want %q", store.Dir(), want)
	}
}

func TestCreateListAndGetKeepStableDocumentIdentity(t *testing.T) {
	store := openStore(t)
	before := time.Now().Add(-time.Second)

	first, err := store.Create("示例文档 - Go 后端", []byte("# 示例文档\n"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Create("示例文档 - 基础设施", []byte("# 示例文档\n"))
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || second.ID == "" || first.ID == second.ID {
		t.Fatalf("document IDs = %q and %q, want distinct non-empty IDs", first.ID, second.ID)
	}
	if !first.CreatedAt.After(before) || !first.UpdatedAt.Equal(first.CreatedAt) {
		t.Fatalf("timestamps = created %v, updated %v, want created after %v and equal updated",
			first.CreatedAt, first.UpdatedAt, before)
	}

	documents, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(documents) != 2 {
		t.Fatalf("List() returned %d documents, want 2", len(documents))
	}
	got, err := store.Get(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != first {
		t.Fatalf("Get(%q) = %#v, want %#v", first.ID, got, first)
	}
	if _, err := store.Get("missing-id"); !errors.Is(err, library.ErrTargetNotFound) {
		t.Fatalf("Get(missing) error = %v, want ErrTargetNotFound", err)
	}
}

func TestLibraryPersistsIDsAndContentAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	first, err := library.OpenAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	source := []byte("# 持久化\r\n\r\n精确字节")
	created, err := first.Create("持久化", source)
	if err != nil {
		t.Fatal(err)
	}

	reopened, err := library.OpenAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Get(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != created {
		t.Fatalf("reopened document = %#v, want %#v", got, created)
	}
	if _, content, err := reopened.Show(created.ID); err != nil || !bytes.Equal(content, source) {
		t.Fatalf("reopened content = %q, %v; want exact source", content, err)
	}
}

func TestCreateRejectsInvalidNames(t *testing.T) {
	store := openStore(t)
	for _, name := range []string{"", "   ", " 示例文档", "示例文档 ", "张\n三", "张\x00三"} {
		t.Run(strings.ReplaceAll(name, "\n", "\\n"), func(t *testing.T) {
			if _, err := store.Create(name, nil); err == nil {
				t.Fatalf("Create(%q) succeeded, want an invalid-name error", name)
			}
		})
	}
}

func TestShowPreservesExactMarkdownBytes(t *testing.T) {
	store := openStore(t)
	source := []byte("\xef\xbb\xbf# 示例文档\r\n\r\n- Go 与 **WebView**\r\n- 末尾没有换行")
	created, err := store.Create("字节保真", source)
	if err != nil {
		t.Fatal(err)
	}

	for _, target := range []string{created.ID, created.Name} {
		t.Run(target, func(t *testing.T) {
			doc, got, err := store.Show(target)
			if err != nil {
				t.Fatal(err)
			}
			if doc.ID != created.ID {
				t.Fatalf("Show(%q) document = %q, want %q", target, doc.ID, created.ID)
			}
			if !bytes.Equal(got, source) {
				t.Fatalf("Show(%q) bytes = %q, want exact source %q", target, got, source)
			}
		})
	}
}

func TestImportCopiesMarkdownWithoutChangingSource(t *testing.T) {
	store := openStore(t)
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "master-resume.md")
	source := []byte("# 李四\n\n## 项目\n\n- 原始内容\r\n")
	if err := os.WriteFile(sourcePath, source, 0o600); err != nil {
		t.Fatal(err)
	}

	doc, err := store.Import(sourcePath, "")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Name != "master-resume" {
		t.Fatalf("imported name = %q, want the source stem", doc.Name)
	}

	named, err := store.Import(sourcePath, "李四 - 项目岗")
	if err != nil {
		t.Fatal(err)
	}
	if named.Name != "李四 - 项目岗" {
		t.Fatalf("imported name = %q, want the explicit name", named.Name)
	}
	if _, got, err := store.Show(named.ID); err != nil || !bytes.Equal(got, source) {
		t.Fatalf("imported bytes = %q, %v; want exact source bytes", got, err)
	}
	after, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, source) {
		t.Fatalf("source file changed after import: %q", after)
	}
	if info, err := os.Stat(sourcePath); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("source file mode changed after import: %v, %v", info, err)
	}
}

func TestRenameKeepsIDAndContent(t *testing.T) {
	store := openStore(t)
	source := []byte("# 王五\n\n未修改的内容")
	created, err := store.Create("旧名称", source)
	if err != nil {
		t.Fatal(err)
	}

	renamed, err := store.Rename(created.ID, "新名称")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.ID != created.ID || renamed.Name != "新名称" {
		t.Fatalf("Rename() = %#v, want the same ID with the new name", renamed)
	}
	if created.Name == renamed.Name {
		t.Fatal("Rename changed the value returned by the earlier Create")
	}
	if _, got, err := store.Show(created.ID); err != nil || !bytes.Equal(got, source) {
		t.Fatalf("Show(%q) after rename = %q, %v; want exact source", created.ID, got, err)
	}
	if _, err := store.Rename(created.ID, ""); err == nil {
		t.Fatal("Rename to an empty name succeeded")
	}
}

func TestDeleteRemovesLibraryDocumentOnly(t *testing.T) {
	store := openStore(t)
	created, err := store.Create("待删除", []byte("# 待删除\n"))
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := store.Delete(created.Name)
	if err != nil {
		t.Fatal(err)
	}
	if deleted.ID != created.ID {
		t.Fatalf("Delete() = %#v, want %#v", deleted, created)
	}
	if _, err := store.Get(created.ID); !errors.Is(err, library.ErrTargetNotFound) {
		t.Fatalf("Get(deleted) error = %v, want ErrTargetNotFound", err)
	}
	if _, _, err := store.Show(created.ID); !errors.Is(err, library.ErrTargetNotFound) {
		t.Fatalf("Show(deleted) error = %v, want ErrTargetNotFound", err)
	}
	documents, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(documents) != 0 {
		t.Fatalf("List() after delete = %#v, want empty", documents)
	}

	external := filepath.Join(t.TempDir(), "external.md")
	if err := os.WriteFile(external, []byte("# 外部\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Delete(external); !errors.Is(err, library.ErrExternalTarget) {
		t.Fatalf("Delete(external) error = %v, want ErrExternalTarget", err)
	}
	if _, err := os.Stat(external); err != nil {
		t.Fatalf("external file was removed: %v", err)
	}
}

func TestDeleteKeepsDocumentWhenContentRemovalFails(t *testing.T) {
	store := openStore(t)
	source := []byte("# 不可删除\r\n\r\n- 内容保持完整\r\n")
	created, err := store.Create("不可删除", source)
	if err != nil {
		t.Fatal(err)
	}
	requireUnwritableDir(t, filepath.Join(store.Dir(), "documents"))

	if _, err := store.Delete(created.ID); err == nil {
		t.Fatal("Delete succeeded while the documents directory was read-only")
	}
	got, err := store.Get(created.ID)
	if err != nil {
		t.Fatalf("Get after failed Delete: %v", err)
	}
	if got != created {
		t.Fatalf("Get after failed Delete = %#v, want %#v", got, created)
	}
	if _, content, err := store.Show(created.ID); err != nil || !bytes.Equal(content, source) {
		t.Fatalf("Show after failed Delete = %q, %v; want exact source", content, err)
	}
}

func TestDeleteRestoresContentWhenIndexUpdateFails(t *testing.T) {
	store := openStore(t)
	source := []byte("# 索引失败\r\n\r\n- 内容恢复\r\n")
	created, err := store.Create("索引失败", source)
	if err != nil {
		t.Fatal(err)
	}
	requireUnwritableDir(t, store.Dir())

	if _, err := store.Delete(created.ID); err == nil {
		t.Fatal("Delete succeeded while the library root was read-only")
	}
	got, err := store.Get(created.ID)
	if err != nil {
		t.Fatalf("Get after failed index update: %v", err)
	}
	if got != created {
		t.Fatalf("Get after failed index update = %#v, want %#v", got, created)
	}
	if _, content, err := store.Show(created.ID); err != nil || !bytes.Equal(content, source) {
		t.Fatalf("Show after failed index update = %q, %v; want exact source", content, err)
	}
}

func TestResolveTargetKindsAndPrecedence(t *testing.T) {
	store := openStore(t)
	dir := t.TempDir()
	t.Chdir(dir)

	libraryDoc, err := store.Create("示例文档 - Go 后端", []byte("# Library\n"))
	if err != nil {
		t.Fatal(err)
	}
	sameName, err := store.Create("resume.md", []byte("# Named Like A Path\n"))
	if err != nil {
		t.Fatal(err)
	}
	externalPath := filepath.Join(dir, "resume.md")
	externalSource := []byte("# External\n")
	if err := os.WriteFile(externalPath, externalSource, 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name       string
		target     string
		kind       library.TargetKind
		documentID string
		path       string
	}{
		{name: "id", target: libraryDoc.ID, kind: library.TargetLibrary, documentID: libraryDoc.ID},
		{name: "name", target: libraryDoc.Name, kind: library.TargetLibrary, documentID: libraryDoc.ID},
		{name: "relative path", target: "resume.md", kind: library.TargetFile, path: externalPath},
		{name: "dot relative path", target: "./resume.md", kind: library.TargetFile, path: externalPath},
		{name: "absolute path", target: externalPath, kind: library.TargetFile, path: externalPath},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target, err := store.ResolveTarget(tc.target)
			if err != nil {
				t.Fatal(err)
			}
			if target.Kind != tc.kind {
				t.Fatalf("kind = %v, want %v", target.Kind, tc.kind)
			}
			if target.Document.ID != tc.documentID {
				t.Fatalf("document ID = %q, want %q", target.Document.ID, tc.documentID)
			}
			if target.Path != tc.path {
				t.Fatalf("path = %q, want %q", target.Path, tc.path)
			}
		})
	}

	// A missing .md path falls back to an exact library name.
	if err := os.Remove(externalPath); err != nil {
		t.Fatal(err)
	}
	target, err := store.ResolveTarget("resume.md")
	if err != nil {
		t.Fatal(err)
	}
	if target.Kind != library.TargetLibrary || target.Document.ID != sameName.ID {
		t.Fatalf("missing path target = %#v, want the library document named resume.md", target)
	}
}

func TestResolveTargetReportsMissingAndAmbiguousTargets(t *testing.T) {
	store := openStore(t)
	first, err := store.Create("重名", []byte("first"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Create("重名", []byte("second"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.ResolveTarget("重名")
	if !errors.Is(err, library.ErrAmbiguousTarget) {
		t.Fatalf("ambiguous name error = %v, want ErrAmbiguousTarget", err)
	}
	for _, want := range []string{"重名", first.ID, second.ID} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("ambiguous error %q does not mention %q", err, want)
		}
	}

	for _, target := range []string{"missing", "missing.md", filepath.Join(t.TempDir(), "missing.md")} {
		_, err := store.ResolveTarget(target)
		if !errors.Is(err, library.ErrTargetNotFound) {
			t.Errorf("ResolveTarget(%q) error = %v, want ErrTargetNotFound", target, err)
		}
		if !strings.Contains(err.Error(), target) {
			t.Errorf("missing error %q does not mention %q", err, target)
		}
	}
}

func TestTargetLayoutKeySeparatesLibraryAndExternalPaths(t *testing.T) {
	store := openStore(t)
	created, err := store.Create("布局", []byte("# 布局\n"))
	if err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(t.TempDir(), "external.md")
	if err := os.WriteFile(external, []byte("# 外部\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	libraryTarget, err := store.ResolveTarget(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	externalTarget, err := store.ResolveTarget(external)
	if err != nil {
		t.Fatal(err)
	}
	if libraryTarget.LayoutKey() == externalTarget.LayoutKey() {
		t.Fatalf("layout keys are equal: %q", libraryTarget.LayoutKey())
	}
	if !strings.Contains(libraryTarget.LayoutKey(), created.ID) {
		t.Fatalf("library layout key %q does not contain the document ID", libraryTarget.LayoutKey())
	}
	if !strings.Contains(externalTarget.LayoutKey(), external) {
		t.Fatalf("external layout key %q does not contain the absolute path", externalTarget.LayoutKey())
	}
}

func TestLayoutOverridesRoundTrip(t *testing.T) {
	store := openStore(t)
	profile := render.DefaultLayout()
	profile.FontStack = render.FontSerif
	profile.LineHeight = 1.30
	overrides := library.LayoutOverrides{
		Default:   &profile,
		Documents: map[string]render.Layout{"doc-1": profile},
		Paths:     map[string]render.Layout{"/tmp/resume.md": profile},
	}

	if err := store.SaveLayoutOverrides(overrides); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadLayoutOverrides()
	if err != nil {
		t.Fatal(err)
	}
	if got.Default == nil || *got.Default != profile {
		t.Fatalf("default profile = %#v, want %#v", got.Default, profile)
	}
	if got.Documents["doc-1"] != profile || got.Paths["/tmp/resume.md"] != profile {
		t.Fatalf("layout overrides = %#v, want the saved profiles", got)
	}
}

func openStore(t *testing.T) *library.Store {
	t.Helper()
	store, err := library.OpenAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func requireUnwritableDir(t *testing.T, dir string) {
	t.Helper()
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dir, info.Mode().Perm())
	})
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	probe, err := os.CreateTemp(dir, "write-probe-*")
	if err == nil {
		name := probe.Name()
		_ = probe.Close()
		_ = os.Remove(name)
		t.Skip("filesystem permissions do not make the directory read-only")
	}
}
