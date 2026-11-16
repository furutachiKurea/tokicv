package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo"

	"tokicv/internal/export"
	"tokicv/internal/library"
)

func TestParseLibraryCommands(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want Command
	}{
		{name: "list", args: []string{"list"}, want: Command{Kind: CommandList}},
		{
			name: "list json",
			args: []string{"list", "--json"},
			want: Command{Kind: CommandList, List: ListRequest{JSON: true}},
		},
		{
			name: "create",
			args: []string{"create", "示例文档 - Go"},
			want: Command{Kind: CommandCreate, Create: CreateRequest{Name: "示例文档 - Go"}},
		},
		{
			name: "create from",
			args: []string{"create", "示例文档", "--from", "resume.md"},
			want: Command{Kind: CommandCreate, Create: CreateRequest{Name: "示例文档", From: "resume.md"}},
		},
		{
			name: "create template",
			args: []string{"create", "示例文档", "--template"},
			want: Command{Kind: CommandCreate, Create: CreateRequest{Name: "示例文档", Template: true}},
		},
		{
			name: "show",
			args: []string{"show", "示例文档"},
			want: Command{Kind: CommandShow, Show: ShowRequest{Target: "示例文档"}},
		},
		{
			name: "import",
			args: []string{"import", "resume.md"},
			want: Command{Kind: CommandImport, Import: ImportRequest{Path: "resume.md"}},
		},
		{
			name: "import name",
			args: []string{"import", "resume.md", "--name", "示例文档"},
			want: Command{Kind: CommandImport, Import: ImportRequest{Path: "resume.md", Name: "示例文档"}},
		},
		{
			name: "delete",
			args: []string{"delete", "示例文档"},
			want: Command{Kind: CommandDelete, Delete: DeleteRequest{Target: "示例文档"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.args)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("Parse(%v) = %#v, want %#v", tc.args, got, tc.want)
			}
		})
	}
}

func TestParseExportMarkdownOutput(t *testing.T) {
	cmd, err := Parse([]string{"export", "示例文档", "--md", "out.md"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Kind != CommandExport || cmd.Export.Markdown != "out.md" {
		t.Fatalf("Parse() = %#v, want a Markdown export", cmd)
	}
	if cmd.Export.Output != "" || cmd.Export.PNG != "" {
		t.Fatalf("export request = %#v, want no PDF or PNG output", cmd.Export)
	}

	combined, err := Parse([]string{"export", "示例文档", "--md=out.md", "--pdf=out.pdf", "--png=pages/"})
	if err != nil {
		t.Fatal(err)
	}
	if combined.Export.Markdown != "out.md" || combined.Export.Output != "out.pdf" || combined.Export.PNG != "pages/" {
		t.Fatalf("combined export request = %#v", combined.Export)
	}
}

func TestParseRejectsInvalidLibraryCommandArguments(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "list argument", args: []string{"list", "extra"}, want: "list"},
		{name: "list option", args: []string{"list", "--wide"}, want: "--wide"},
		{name: "create missing name", args: []string{"create"}, want: "create"},
		{name: "create extra name", args: []string{"create", "a", "b"}, want: "一个"},
		{name: "create from and template", args: []string{"create", "a", "--from", "a.md", "--template"}, want: "不能同时"},
		{name: "create missing from", args: []string{"create", "a", "--from"}, want: "--from"},
		{name: "show missing target", args: []string{"show"}, want: "show"},
		{name: "show two targets", args: []string{"show", "a", "b"}, want: "一个"},
		{name: "import missing path", args: []string{"import"}, want: "import"},
		{name: "import empty name", args: []string{"import", "a.md", "--name="}, want: "--name"},
		{name: "delete missing target", args: []string{"delete"}, want: "delete"},
		{name: "export no output", args: []string{"export", "a.md"}, want: "--pdf"},
		{name: "export empty md", args: []string{"export", "a.md", "--md="}, want: "--md"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse(tc.args); err == nil {
				t.Fatalf("Parse(%v) succeeded, want an error", tc.args)
			} else if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Parse(%v) error = %q, want it to contain %q", tc.args, err, tc.want)
			}
		})
	}
}

func TestLibraryCommandsRunWithoutRendering(t *testing.T) {
	store := openCLIStore(t)
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.md")
	source := []byte("# 示例文档\r\n\r\n- 未修改的字节\r\n")
	if err := os.WriteFile(sourcePath, source, 0o644); err != nil {
		t.Fatal(err)
	}
	options := RunOptions{
		Store: store,
		Export: func(ExportRequest) error {
			t.Fatal("library command dispatched the renderer")
			return nil
		},
		Check: func(CheckRequest) (export.CheckResult, error) {
			t.Fatal("library command dispatched the Page Fit renderer")
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

	created := run("create", "示例文档 - Go", "--from", sourcePath)
	if !strings.Contains(created, "示例文档 - Go") {
		t.Fatalf("create stdout = %q, want the document name", created)
	}
	documents, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(documents) != 1 {
		t.Fatalf("library has %d documents, want 1", len(documents))
	}
	id := documents[0].ID

	if got := run("show", id); got != string(source) {
		t.Fatalf("show stdout = %q, want exact source %q", got, source)
	}
	list := run("list")
	if !strings.Contains(list, id) || !strings.Contains(list, "示例文档 - Go") {
		t.Fatalf("list stdout = %q, want the document ID and name", list)
	}

	imported := run("import", sourcePath, "--name", "导入稿")
	if !strings.Contains(imported, "导入稿") {
		t.Fatalf("import stdout = %q, want the imported name", imported)
	}
	deleted := run("delete", id)
	if !strings.Contains(deleted, "示例文档 - Go") {
		t.Fatalf("delete stdout = %q, want the deleted document name", deleted)
	}
	if _, err := store.Get(id); err == nil {
		t.Fatal("deleted document still resolves")
	}
}

func TestListJSONOutputsDocumentMetadata(t *testing.T) {
	store := openCLIStore(t)
	created, err := store.Create("示例文档", []byte("# 示例文档\n"))
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := RunWithOptions([]string{"list", "--json"}, RunOptions{
		Store:  store,
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	var documents []library.Document
	if err := json.Unmarshal(stdout.Bytes(), &documents); err != nil {
		t.Fatalf("list --json output is not a document array: %v\n%s", err, stdout.String())
	}
	if len(documents) != 1 || documents[0].ID != created.ID || documents[0].Name != created.Name {
		t.Fatalf("documents = %#v, want the created document", documents)
	}
	if documents[0].CreatedAt.IsZero() || documents[0].UpdatedAt.IsZero() {
		t.Fatalf("document timestamps = %#v, want non-zero", documents[0])
	}
}

func TestListJSONEmptyLibraryIsArray(t *testing.T) {
	store := openCLIStore(t)
	var stdout, stderr bytes.Buffer
	code := RunWithOptions([]string{"list", "--json"}, RunOptions{
		Store:  store,
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "[]" {
		t.Fatalf("empty list --json output = %q, want []", got)
	}
}

func TestCreateSupportsStdinTemplateAndEmptyDocument(t *testing.T) {
	store := openCLIStore(t)

	var stdout, stderr bytes.Buffer
	stdinSource := []byte("# 标准输入\r\n")
	code := RunWithOptions([]string{"create", "标准输入"}, RunOptions{
		Store:           store,
		Stdin:           bytes.NewReader(stdinSource),
		StdinRedirected: true,
		Stdout:          &stdout,
		Stderr:          &stderr,
	})
	if code != 0 {
		t.Fatalf("create from stdin exit code = %d, stderr = %s", code, stderr.String())
	}
	documents, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(documents) != 1 {
		t.Fatalf("library has %d documents, want 1", len(documents))
	}
	if _, got, err := store.Show(documents[0].ID); err != nil || !bytes.Equal(got, stdinSource) {
		t.Fatalf("stdin document bytes = %q, %v; want exact stdin", got, err)
	}

	stdout.Reset()
	stderr.Reset()
	code = RunWithOptions([]string{"create", "模板稿", "--template"}, RunOptions{
		Store:  store,
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if code != 0 {
		t.Fatalf("create --template exit code = %d, stderr = %s", code, stderr.String())
	}
	_, template, err := store.Show("模板稿")
	if err != nil {
		t.Fatal(err)
	}
	if len(template) == 0 || !bytes.Contains(template, []byte("# ")) {
		t.Fatalf("template bytes = %q, want a starter Resume Document", template)
	}

	stdout.Reset()
	stderr.Reset()
	code = RunWithOptions([]string{"create", "空文档"}, RunOptions{
		Store:  store,
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if code != 0 {
		t.Fatalf("create empty exit code = %d, stderr = %s", code, stderr.String())
	}
	_, empty, err := store.Show("空文档")
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("empty document bytes = %q, want empty", empty)
	}
}

func TestExportMarkdownOnlyIsPureGoAndByteExact(t *testing.T) {
	store := openCLIStore(t)
	source := []byte("\xef\xbb\xbf# 示例文档\r\n\r\n- Go\r\n")
	if _, err := store.Create("示例文档", source); err != nil {
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
		{name: "library name", target: "示例文档"},
		{name: "external path", target: external},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "out.md")
			var stdout, stderr bytes.Buffer
			code := RunWithOptions([]string{"export", tc.target, "--md", output}, RunOptions{
				Store:  store,
				Stdout: &stdout,
				Stderr: &stderr,
				Export: func(ExportRequest) error {
					t.Fatal("--md-only export dispatched the WebView renderer")
					return nil
				},
				Check: func(CheckRequest) (export.CheckResult, error) {
					t.Fatal("--md-only export dispatched the Page Fit renderer")
					return export.CheckResult{}, nil
				},
			})
			if code != 0 {
				t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
			}
			got, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, source) {
				t.Fatalf("Markdown Artifact = %q, want exact source %q", got, source)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
		})
	}
}

func TestExportMarkdownOnlyFailureLeavesNoOutput(t *testing.T) {
	store := openCLIStore(t)
	if _, err := store.Create("示例文档", []byte("# 示例文档\n")); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "missing", "out.md")

	var stdout, stderr bytes.Buffer
	code := RunWithOptions([]string{"export", "示例文档", "--md", output}, RunOptions{
		Store:  store,
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "out.md") {
		t.Fatalf("stderr = %q, want the failed output path", stderr.String())
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("output exists after failure: %v", err)
	}
}

func TestLibraryCommandsUsePathUserDataByDefault(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })

	var stdout, stderr bytes.Buffer
	code := RunWithOptions([]string{"create", "默认库"}, RunOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if code != 0 {
		t.Fatalf("create exit code = %d, stderr = %s", code, stderr.String())
	}
	store, err := library.OpenAt(filepath.Join(root, "library"))
	if err != nil {
		t.Fatal(err)
	}
	documents, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(documents) != 1 || documents[0].Name != "默认库" {
		t.Fatalf("PathUserData library documents = %#v, want the created document", documents)
	}
}

func openCLIStore(t *testing.T) *library.Store {
	t.Helper()
	store, err := library.OpenAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return store
}
