package diagnostic

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"
)

func TestBoundedWriterTruncatesLargeRecordAsValidUTF8(t *testing.T) {
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	logger, err := Open(t.TempDir(), Options{
		MaxBytes:  96,
		Backups:   2,
		Retention: time.Hour,
		Now:       func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()

	record := []byte("stage=test error=" + strings.Repeat("界", 80) + "\n")
	if _, err := logger.Write(record); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(logger.Path())
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(data)) > 96 {
		t.Fatalf("truncated log length = %d, want <= 96", len(data))
	}
	if !utf8.Valid(data) {
		t.Fatalf("truncated log is not valid UTF-8: %q", data)
	}
	if !bytes.Contains(data, []byte("[truncated]")) {
		t.Fatalf("truncated log = %q, want a truncation marker", data)
	}
}

func TestBoundedWriterRotatesAndKeepsOnlyConfiguredFiles(t *testing.T) {
	dir := t.TempDir()
	logger, err := Open(dir, Options{
		MaxBytes:  80,
		Backups:   2,
		Retention: time.Hour,
		Now:       time.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 12; index++ {
		if _, err := logger.Write([]byte("stage=rotate index=" + strconvItoa(index) + strings.Repeat("x", 24) + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("log files = %v, want current plus two backups", entryNames(entries))
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() > 80 {
			t.Errorf("%s size = %d, want <= 80", entry.Name(), info.Size())
		}
	}
}

func TestBoundedWriterRestartsDoNotAccumulateFiles(t *testing.T) {
	dir := t.TempDir()
	opts := Options{
		MaxBytes:  64,
		Backups:   2,
		Retention: time.Hour,
		Now:       time.Now,
	}
	for restart := 0; restart < 6; restart++ {
		logger, err := Open(dir, opts)
		if err != nil {
			t.Fatal(err)
		}
		for index := 0; index < 5; index++ {
			if _, err := logger.Write([]byte("stage=restart restart=" + strconvItoa(restart) + "\n")); err != nil {
				t.Fatal(err)
			}
		}
		if err := logger.Close(); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) > 3 {
		t.Fatalf("after restarts log files = %v, want at most three", entryNames(entries))
	}
}

func TestBoundedWriterRemovesExpiredFilesOnOpen(t *testing.T) {
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	old := now.Add(-2 * time.Hour)
	for _, name := range []string{logName, logName + ".1", logName + ".2"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
	unrelated := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(unrelated, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	logger, err := Open(dir, Options{
		MaxBytes:  128,
		Backups:   2,
		Retention: time.Hour,
		Now:       func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{logName + ".1", logName + ".2"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("expired %s still exists: %v", name, err)
		}
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Errorf("unrelated file was removed: %v", err)
	}
}

func TestLoggerInstallRestoresGlobalLoggerAndFlushes(t *testing.T) {
	originalOutput := log.Writer()
	originalFlags := log.Flags()
	originalPrefix := log.Prefix()
	t.Cleanup(func() {
		log.SetOutput(originalOutput)
		log.SetFlags(originalFlags)
		log.SetPrefix(originalPrefix)
	})

	logger, err := Open(t.TempDir(), DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Install(); err != nil {
		t.Fatal(err)
	}
	log.Print("stage=install-restore")
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(log.Writer(), originalOutput) {
		t.Fatal("Close did not restore the original logger output")
	}
	if log.Flags() != originalFlags || log.Prefix() != originalPrefix {
		t.Fatal("Close did not restore the original logger settings")
	}
	data, err := os.ReadFile(logger.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("stage=install-restore")) {
		t.Fatalf("log = %q, want the installed record", data)
	}
}

func TestOpenFailsWhenLogPathIsAFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "logs")
	if err := os.WriteFile(path, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path, DefaultOptions()); err == nil {
		t.Fatal("Open accepted a regular file as the log directory")
	} else if !errors.Is(err, os.ErrExist) && !strings.Contains(err.Error(), "诊断日志目录") {
		t.Fatalf("Open error = %v, want a readable directory error", err)
	}
}

func TestSanitizeRemovesPathsContactsAndBoundsText(t *testing.T) {
	got := Sanitize("文件 /home/alice/简历.md 路径 \"/home/alice/My Resume.md\" 邮箱 alice@example.com 电话 +86 138 0013 8000")
	for _, banned := range []string{"/home/alice", "My Resume", "alice@example.com", "138"} {
		if strings.Contains(got, banned) {
			t.Errorf("Sanitize() = %q, still contains %q", got, banned)
		}
	}
	if !strings.Contains(got, "<path>") || !strings.Contains(got, "<email>") || !strings.Contains(got, "<phone>") {
		t.Errorf("Sanitize() = %q, want redaction markers", got)
	}
	long := Sanitize(strings.Repeat("界", 600))
	if utf8.RuneCountInString(long) > 512 || !strings.HasSuffix(long, "…") {
		t.Errorf("Sanitize() long value length = %d, want a bounded marker", utf8.RuneCountInString(long))
	}
}

func TestRetentionUsesCreationTimeAcrossAppendRestartAndThirtyDays(t *testing.T) {
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	dir := t.TempDir()
	opts := Options{
		MaxBytes:        1 << 20,
		Backups:         2,
		Retention:       7 * 24 * time.Hour,
		CleanupInterval: 5 * time.Minute,
		Now:             clock,
	}
	logger, err := Open(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := logger.Write([]byte("stage=day0\n")); err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, logName)
	now = now.Add(10 * 24 * time.Hour)
	if err := os.Chtimes(path, now, now); err != nil {
		t.Fatal(err)
	}

	logger, err = Open(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()
	if _, err := logger.Write([]byte("stage=day10\n")); err != nil {
		t.Fatal(err)
	}
	if data := readLog(t, path); bytes.Contains(data, []byte("stage=day0")) {
		t.Fatalf("log = %q, want the pre-restart record removed despite its fresh mtime", data)
	}

	for day := 11; day <= 30; day++ {
		now = time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC).Add(time.Duration(day) * 24 * time.Hour)
		if _, err := logger.Write([]byte(fmt.Sprintf("stage=day%d\n", day))); err != nil {
			t.Fatal(err)
		}
		if day == 18 {
			if data := readLog(t, path); bytes.Contains(data, []byte("stage=day10")) {
				t.Fatalf("log = %q, want records older than seven days removed", data)
			}
		}
	}
	data := readLog(t, path)
	if bytes.Contains(data, []byte("stage=day10")) || bytes.Contains(data, []byte("stage=day24")) {
		t.Fatalf("log = %q, want 30-day retention to expire old appends", data)
	}
	if !bytes.Contains(data, []byte("stage=day30")) {
		t.Fatalf("log = %q, want the latest record retained", data)
	}
	created, err := fileCreated(path, opts.Now())
	if err != nil {
		t.Fatal(err)
	}
	if now.Sub(created) > 7*24*time.Hour {
		t.Fatalf("active file age = %v, want at most seven days", now.Sub(created))
	}
}

func TestRetentionRemovesAgedBackupsWithoutRotation(t *testing.T) {
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	old := now.Add(-10 * 24 * time.Hour)
	writeAgedLog(t, filepath.Join(dir, logName+".1"), old, "stage=aged-backup\n")
	writeAgedLog(t, filepath.Join(dir, logName+".2"), now.Add(-time.Hour), "stage=recent-backup\n")
	recent := filepath.Join(dir, logName+".1")
	if err := os.Chtimes(recent, now, now); err != nil {
		t.Fatal(err)
	}

	logger, err := Open(dir, Options{
		MaxBytes:        1024,
		Backups:         2,
		Retention:       7 * 24 * time.Hour,
		CleanupInterval: time.Minute,
		Now:             func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(recent); !os.IsNotExist(err) {
		t.Fatalf("aged backup still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, logName+".2")); err != nil {
		t.Fatalf("recent backup was removed: %v", err)
	}
}

func TestCleanupIntervalBoundsExpiryLatency(t *testing.T) {
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	logger, err := Open(dir, Options{
		MaxBytes:        4096,
		Backups:         2,
		Retention:       7 * 24 * time.Hour,
		CleanupInterval: 5 * time.Minute,
		Now:             func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()
	if _, err := logger.Write([]byte("stage=initial\n")); err != nil {
		t.Fatal(err)
	}

	now = now.Add(7*24*time.Hour - time.Minute)
	if _, err := logger.Write([]byte("stage=before-expiry\n")); err != nil {
		t.Fatal(err)
	}
	now = now.Add(4 * time.Minute)
	if _, err := logger.Write([]byte("stage=within-latency\n")); err != nil {
		t.Fatal(err)
	}
	if data := readLog(t, logger.Path()); !bytes.Contains(data, []byte("stage=initial")) {
		t.Fatalf("log = %q, want retention kept until the next five-minute check", data)
	}

	now = now.Add(2 * time.Minute)
	if _, err := logger.Write([]byte("stage=after-latency\n")); err != nil {
		t.Fatal(err)
	}
	data := readLog(t, logger.Path())
	if bytes.Contains(data, []byte("stage=initial")) {
		t.Fatalf("log = %q, want expiry after seven days plus one cleanup interval", data)
	}
	if !bytes.Contains(data, []byte("stage=after-latency")) {
		t.Fatalf("log = %q, want the triggering record retained", data)
	}
}

func TestOversizedFixedFilesAreCleanedAndUnrelatedFilesPreserved(t *testing.T) {
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	maxBytes := int64(1024)
	for _, name := range []string{logName, logName + ".1"} {
		path := filepath.Join(dir, name)
		writeAgedLog(t, path, now, strings.Repeat("x", 4096))
	}
	writeAgedLog(t, filepath.Join(dir, logName+".2"), now, "stage=recent\n")
	unrelated := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(unrelated, bytes.Repeat([]byte("u"), 5000), 0o600); err != nil {
		t.Fatal(err)
	}
	similarNames := []string{"tokicv.log.01", "tokicv.log.notes"}
	for _, name := range similarNames {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("user data"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeAgedLog(t, filepath.Join(dir, logName+".3"), now, "stage=extra\n")

	logger, err := Open(dir, Options{
		MaxBytes:        maxBytes,
		Backups:         2,
		Retention:       7 * 24 * time.Hour,
		CleanupInterval: time.Minute,
		Now:             func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := logger.Write([]byte("stage=after-cleanup\n")); err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}

	var total int64
	for _, name := range []string{logName, logName + ".1", logName + ".2"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() > maxBytes {
			t.Errorf("%s size = %d, want <= %d", name, info.Size(), maxBytes)
		}
		total += info.Size()
	}
	if total > 3*maxBytes {
		t.Errorf("fixed log total = %d, want <= %d", total, 3*maxBytes)
	}
	info, err := os.Stat(unrelated)
	if err != nil {
		t.Fatalf("unrelated file was removed: %v", err)
	}
	if info.Size() != 5000 {
		t.Errorf("unrelated file size = %d, want 5000", info.Size())
	}
	for _, name := range similarNames {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(data) != "user data" {
			t.Errorf("similar file %s was changed: data=%q err=%v", name, data, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, logName+".3")); !os.IsNotExist(err) {
		t.Errorf("extra fixed backup was not removed: %v", err)
	}
}

func TestDefaultBudgetNeverExceedsThreeMiB(t *testing.T) {
	dir := t.TempDir()
	logger, err := Open(dir, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	record := []byte("stage=budget " + strings.Repeat("x", 256*1024) + "\n")
	for index := 0; index < 20; index++ {
		if _, err := logger.Write(record); err != nil {
			t.Fatal(err)
		}
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, name := range []string{logName, logName + ".1", logName + ".2"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() > DefaultMaxBytes {
			t.Errorf("%s size = %d, want <= %d", name, info.Size(), DefaultMaxBytes)
		}
		total += info.Size()
	}
	if total > int64(DefaultBackups+1)*DefaultMaxBytes {
		t.Errorf("fixed log total = %d, want <= %d", total, int64(DefaultBackups+1)*DefaultMaxBytes)
	}
}

func TestSafeErrorRemovesStructuredPathsAndRedactsUnknownMessages(t *testing.T) {
	pathErr := &os.PathError{Op: "open", Path: "/tmp/My Resume.md", Err: syscall.ENOENT}
	for _, tc := range []struct {
		name   string
		err    error
		banned []string
	}{
		{
			name:   "path with spaces",
			err:    pathErr,
			banned: []string{"/tmp", "My Resume"},
		},
		{
			name:   "Chinese directory with spaces",
			err:    &os.PathError{Op: "read", Path: "/tmp/中文 目录/My Resume.md", Err: syscall.EACCES},
			banned: []string{"/tmp", "中文 目录", "Resume.md"},
		},
		{
			name:   "wrapped path",
			err:    fmt.Errorf("load document: %w", pathErr),
			banned: []string{"/tmp", "My Resume"},
		},
		{
			name:   "link old and new paths",
			err:    &os.LinkError{Op: "link", Old: "/tmp/My Resume.md", New: "/tmp/Private Resume.md", Err: syscall.EEXIST},
			banned: []string{"/tmp", "My Resume", "Private Resume"},
		},
		{
			name:   "unknown message",
			err:    errors.New("send /tmp/My Resume.md to alice@example.com or +86 138 0013 8000"),
			banned: []string{"/tmp", "My Resume", "alice@example.com", "138"},
		},
		{
			name:   "known context error",
			err:    context.Canceled,
			banned: []string{"/tmp"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := SafeError(tc.err)
			if got == "" {
				t.Fatal("SafeError returned an empty value")
			}
			for _, banned := range tc.banned {
				if strings.Contains(got, banned) {
					t.Errorf("SafeError() = %q, still contains %q", got, banned)
				}
			}
			if !utf8.ValidString(got) {
				t.Errorf("SafeError() = %q, want valid UTF-8", got)
			}
		})
	}
	if got := SafeError(fmt.Errorf("open: %w", pathErr)); !strings.Contains(got, "open") ||
		!strings.Contains(got, "no such file") {
		t.Errorf("SafeError() = %q, want operation and safe underlying cause", got)
	}
}

func TestSafeErrorClassifiesKnownMyGoPDFErrors(t *testing.T) {
	const (
		pageNotLoaded = "mygo: printing to PDF: the page has loaded nothing"
		noOutputFile  = "mygo: printing to PDF produced no file"
		printerError  = "mygo: printing to PDF: 找不到打印机（GError）"
	)
	for _, tc := range []struct {
		name   string
		err    error
		code   string
		reason string
		banned []string
	}{
		{
			name:   "page not loaded",
			err:    errors.New(pageNotLoaded),
			code:   "mygo_pdf_page_not_loaded",
			reason: "the page has loaded nothing",
		},
		{
			name:   "wrapped page not loaded",
			err:    fmt.Errorf("导出失败: %w", errors.New(pageNotLoaded)),
			code:   "mygo_pdf_page_not_loaded",
			reason: "the page has loaded nothing",
		},
		{
			name:   "wrapped page not loaded with path",
			err:    fmt.Errorf("save /tmp/My Resume.md: %w", errors.New(pageNotLoaded)),
			code:   "mygo_pdf_page_not_loaded",
			reason: "the page has loaded nothing",
			banned: []string{"/tmp", "My Resume"},
		},
		{
			name:   "page not loaded with path",
			err:    errors.New(pageNotLoaded + " at /tmp/My Resume.md"),
			code:   "mygo_pdf_page_not_loaded",
			reason: "the page has loaded nothing",
			banned: []string{"/tmp", "My Resume"},
		},
		{
			name:   "no output file",
			err:    errors.New(noOutputFile),
			code:   "mygo_pdf_no_output_file",
			reason: "printing to PDF produced no file",
		},
		{
			name:   "fmt value wrapper no output file",
			err:    fmt.Errorf("导出失败: %v", errors.New(noOutputFile)),
			code:   "mygo_pdf_no_output_file",
			reason: "printing to PDF produced no file",
		},
		{
			name:   "printer error",
			err:    errors.New(printerError),
			code:   "mygo_pdf_printer_gerror",
			reason: "找不到打印机（GError）",
		},
		{
			name:   "wrapped printer error",
			err:    fmt.Errorf("导出失败: %w", errors.New(printerError)),
			code:   "mygo_pdf_printer_gerror",
			reason: "找不到打印机（GError）",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := SafeError(tc.err)
			for _, want := range []string{"reason_code=" + tc.code, "reason=" + tc.reason} {
				if !strings.Contains(got, want) {
					t.Errorf("SafeError() = %q, want %q", got, want)
				}
			}
			for _, banned := range tc.banned {
				if strings.Contains(got, banned) {
					t.Errorf("SafeError() = %q, still contains %q", got, banned)
				}
			}
			if !utf8.ValidString(got) {
				t.Errorf("SafeError() = %q, want valid UTF-8", got)
			}
		})
	}
}

func TestSafeErrorRedactsUnknownMyGoPDFText(t *testing.T) {
	for _, err := range []error{
		errors.New("mygo: printing to PDF: failed to save /tmp/My Resume.md: 正文内容"),
		fmt.Errorf("导出失败: %w", errors.New("mygo: printing to PDF: failed to save /tmp/My Resume.md: 正文内容")),
	} {
		got := SafeError(err)
		if got != "<redacted>" {
			t.Errorf("SafeError() = %q, want %q", got, "<redacted>")
		}
	}
}

func readLog(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeAgedLog(t *testing.T, path string, created time.Time, body string) {
	t.Helper()
	content := logHeaderPrefix + created.UTC().Format(time.RFC3339) + "\n" + body
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, len(entries))
	for index, entry := range entries {
		names[index] = entry.Name()
	}
	return names
}

func strconvItoa(value int) string {
	return strconv.Itoa(value)
}
