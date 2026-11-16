package desktop

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"github.com/egoist/mygo"
)

func TestInstallDiagnosticLogWritesStartupAndRestoresLogger(t *testing.T) {
	dir := t.TempDir()
	mygo.App.SetPath(mygo.PathLogs, dir)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathLogs, "") })

	previousOutput := log.Writer()
	previousFlags := log.Flags()
	previousPrefix := log.Prefix()
	t.Cleanup(func() {
		log.SetOutput(previousOutput)
		log.SetFlags(previousFlags)
		log.SetPrefix(previousPrefix)
	})

	closeLog, err := installDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	if closeLog == nil {
		t.Fatal("installDiagnosticLog returned no close function")
	}
	log.Print("stage=diagnostic-test error=可诊断")
	closeLog()

	data, err := os.ReadFile(filepath.Join(dir, "tokicv.log"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"stage=startup", "stage=diagnostic-test", "可诊断"} {
		if !strings.Contains(text, want) {
			t.Errorf("log = %q, want %q", text, want)
		}
	}
	if !reflect.DeepEqual(log.Writer(), previousOutput) {
		t.Fatal("closing the diagnostic log did not restore the global logger")
	}
}

func TestInstallDiagnosticLogFailureKeepsCallerUsable(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "logs")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	mygo.App.SetPath(mygo.PathLogs, blocker)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathLogs, "") })

	previousOutput := log.Writer()
	previousFlags := log.Flags()
	previousPrefix := log.Prefix()
	t.Cleanup(func() {
		log.SetOutput(previousOutput)
		log.SetFlags(previousFlags)
		log.SetPrefix(previousPrefix)
	})
	closeLog, err := installDiagnosticLog()
	if err == nil {
		t.Fatal("installDiagnosticLog accepted a file as its log directory")
	}
	if closeLog != nil {
		t.Fatal("failed diagnostic setup returned a close function")
	}
	if !reflect.DeepEqual(log.Writer(), previousOutput) {
		t.Fatal("failed diagnostic setup changed the global logger")
	}

	var logs bytes.Buffer
	log.SetOutput(&logs)
	log.Print("stage=fallback")
	if !strings.Contains(logs.String(), "stage=fallback") {
		t.Fatalf("stderr fallback logger = %q", logs.String())
	}

	app := New(Options{})
	app.buffer.Set("# 编辑仍可用\n")
	if got := app.buffer.String(); got != "# 编辑仍可用\n" {
		t.Fatalf("buffer after diagnostic failure = %q", got)
	}
}

func TestLogDesktopErrorStripsPathBeforeLogging(t *testing.T) {
	previousOutput := log.Writer()
	previousFlags := log.Flags()
	previousPrefix := log.Prefix()
	t.Cleanup(func() {
		log.SetOutput(previousOutput)
		log.SetFlags(previousFlags)
		log.SetPrefix(previousPrefix)
	})

	var logs bytes.Buffer
	log.SetOutput(&logs)
	log.SetFlags(0)
	log.SetPrefix("")
	err := &os.PathError{Op: "open", Path: "/tmp/中文 目录/My Resume.md", Err: syscall.ENOENT}
	logDesktopError("open", err)
	text := logs.String()
	for _, banned := range []string{"/tmp", "中文 目录", "My Resume", "Resume.md"} {
		if strings.Contains(text, banned) {
			t.Errorf("diagnostic log = %q, still contains %q", text, banned)
		}
	}
	for _, want := range []string{"stage=open", "error_type=", "PathError", "open", "no such file"} {
		if !strings.Contains(text, want) {
			t.Errorf("diagnostic log = %q, want %q", text, want)
		}
	}
}

func TestLogDesktopErrorClassifiesKnownMyGoPDFFailures(t *testing.T) {
	previousOutput := log.Writer()
	previousFlags := log.Flags()
	previousPrefix := log.Prefix()
	t.Cleanup(func() {
		log.SetOutput(previousOutput)
		log.SetFlags(previousFlags)
		log.SetPrefix(previousPrefix)
	})

	for _, tc := range []struct {
		name   string
		err    error
		code   string
		reason string
	}{
		{
			name:   "page not loaded",
			err:    errors.New("mygo: printing to PDF: the page has loaded nothing"),
			code:   "mygo_pdf_page_not_loaded",
			reason: "the page has loaded nothing",
		},
		{
			name:   "wrapped no output file",
			err:    fmt.Errorf("导出 PDF: %w", errors.New("mygo: printing to PDF produced no file")),
			code:   "mygo_pdf_no_output_file",
			reason: "printing to PDF produced no file",
		},
		{
			name:   "printer error",
			err:    errors.New("mygo: printing to PDF: 找不到打印机（GError）"),
			code:   "mygo_pdf_printer_gerror",
			reason: "找不到打印机（GError）",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			log.SetOutput(&logs)
			log.SetFlags(0)
			log.SetPrefix("")

			logDesktopError("export", tc.err)
			text := logs.String()
			for _, want := range []string{"stage=export", "reason_code=" + tc.code, "reason=" + tc.reason} {
				if !strings.Contains(text, want) {
					t.Errorf("diagnostic log = %q, want %q", text, want)
				}
			}
			for _, banned := range []string{"<redacted>", "/tmp", "My Resume"} {
				if strings.Contains(text, banned) {
					t.Errorf("diagnostic log = %q, still contains %q", text, banned)
				}
			}
		})
	}
}
