package library_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"tokicv/internal/dialect"
	"tokicv/internal/library"
)

func TestUpdateContentPreservesExactBytesAndRefreshesTimestamp(t *testing.T) {
	store := openStore(t)
	created, err := store.Create("更新内容", []byte("# 旧内容\n"))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)

	source := []byte("\xef\xbb\xbf# 新内容\r\n\r\n- 精确字节\r\n")
	updated, err := store.UpdateContent(created.ID, source)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != created.ID || updated.Name != created.Name {
		t.Fatalf("UpdateContent() = %#v, want the same identity as %#v", updated, created)
	}
	if !updated.CreatedAt.Equal(created.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", updated.CreatedAt, created.CreatedAt)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Errorf("UpdatedAt = %v, want later than %v", updated.UpdatedAt, created.UpdatedAt)
	}
	if _, got, err := store.Show(created.ID); err != nil || !bytes.Equal(got, source) {
		t.Fatalf("Show() after update = %q, %v; want exact source", got, err)
	}
	if _, err := store.UpdateContent("missing-id", source); !errors.Is(err, library.ErrTargetNotFound) {
		t.Fatalf("UpdateContent(missing) error = %v, want ErrTargetNotFound", err)
	}
}

func TestUpdateContentRestoresBytesWhenIndexCommitFails(t *testing.T) {
	store := openStore(t)
	source := []byte("# 原始标题\r\n\r\n- 原始内容")
	created, err := store.Create("原子更新", source)
	if err != nil {
		t.Fatal(err)
	}
	requireUnwritableDir(t, store.Dir())

	if _, err := store.UpdateContent(created.ID, []byte("# 新标题\n")); err == nil {
		t.Fatal("UpdateContent succeeded while the library index was read-only")
	}
	got, err := store.Get(created.ID)
	if err != nil {
		t.Fatalf("Get() after failed update: %v", err)
	}
	if got != created {
		t.Fatalf("Get() after failed update = %#v, want %#v", got, created)
	}
	if _, content, err := store.Show(created.ID); err != nil || !bytes.Equal(content, source) {
		t.Fatalf("Show() after failed update = %q, %v; want the original bytes", content, err)
	}
}

func TestStarterMarkdownIsAValidCleanResumeDialectSample(t *testing.T) {
	source := library.StarterMarkdown()
	if source == "" {
		t.Fatal("StarterMarkdown() is empty")
	}
	if source != library.StarterMarkdown() {
		t.Fatal("StarterMarkdown() is not stable")
	}
	for _, want := range []string{
		"# 姓名",
		"::: left",
		"::: right",
		"icon:email",
		"icon:github",
		"icon:blog",
		"`标签 A`",
		"## 工作经历",
		"## 项目经历",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("StarterMarkdown() does not contain %q", want)
		}
	}

	analysis := dialect.Analyze(source)
	if len(analysis.Diagnostics) != 0 {
		t.Fatalf("StarterMarkdown() diagnostics = %#v, want none", analysis.Diagnostics)
	}
	if len(analysis.Document.Blocks) == 0 {
		t.Fatal("StarterMarkdown() does not parse into blocks")
	}
	heading, ok := analysis.Document.Blocks[0].(dialect.Heading)
	if !ok || heading.Level != 1 {
		t.Fatalf("first block = %#v, want an H1", analysis.Document.Blocks[0])
	}
}
