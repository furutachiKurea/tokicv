package diagnostic

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
)

const (
	DefaultMaxBytes        int64 = 1 << 20
	DefaultBackups               = 2
	DefaultRetention             = 7 * 24 * time.Hour
	DefaultCleanupInterval       = 5 * time.Minute

	logName          = "tokicv.log"
	truncationMarker = "\n[truncated]\n"
	logHeaderPrefix  = "#created="
)

var (
	quotedPathPattern   = regexp.MustCompile(`"(?:[A-Za-z]:\\|/)[^"]*"|'(?:[A-Za-z]:\\|/)[^']*'`)
	absolutePathPattern = regexp.MustCompile(`(?:[A-Za-z]:\\|/)[^\s"'<>]+`)
	emailPattern        = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	phonePattern        = regexp.MustCompile(`(?:\+?\d[\s\-]?){7,}\d`)
)

type safeErrorClass struct {
	code    string
	reason  string
	markers []string
}

var knownMyGoPDFErrors = []safeErrorClass{
	{
		code:    "mygo_pdf_page_not_loaded",
		reason:  "the page has loaded nothing",
		markers: []string{"mygo: printing to PDF: the page has loaded nothing"},
	},
	{
		code:    "mygo_pdf_no_output_file",
		reason:  "printing to PDF produced no file",
		markers: []string{"mygo: printing to PDF produced no file"},
	},
	{
		code:    "mygo_pdf_printer_gerror",
		reason:  "找不到打印机（GError）",
		markers: []string{"mygo: printing to PDF: 找不到打印机"},
	},
}

// Options bounds the diagnostic log files.
type Options struct {
	MaxBytes        int64
	Backups         int
	Retention       time.Duration
	CleanupInterval time.Duration
	Now             func() time.Time
}

// DefaultOptions returns the application's diagnostic log budget.
func DefaultOptions() Options {
	return Options{
		MaxBytes:        DefaultMaxBytes,
		Backups:         DefaultBackups,
		Retention:       DefaultRetention,
		CleanupInterval: DefaultCleanupInterval,
		Now:             time.Now,
	}
}

// Logger writes bounded, rotating diagnostic records to one directory.
type Logger struct {
	dir  string
	path string
	opts Options

	mu             sync.Mutex
	file           *os.File
	size           int64
	closed         bool
	installed      bool
	previousOutput io.Writer
	previousFlags  int
	previousPrefix string
	lastCleanup    time.Time
}

// Open opens the diagnostic logger in dir without installing it in the
// process-global logger.
func Open(dir string, opts Options) (*Logger, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("诊断日志目录为空")
	}
	opts = normalizeOptions(opts)
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("解析诊断日志目录: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("创建诊断日志目录: %w", err)
	}

	logger := &Logger{
		dir:  absolute,
		path: filepath.Join(absolute, logName),
		opts: opts,
	}
	if err := logger.maintain(opts.Now()); err != nil {
		if logger.file != nil {
			_ = logger.file.Close()
		}
		return nil, err
	}
	return logger, nil
}

func normalizeOptions(opts Options) Options {
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = DefaultMaxBytes
	}
	if opts.Backups <= 0 {
		opts.Backups = DefaultBackups
	}
	if opts.Retention <= 0 {
		opts.Retention = DefaultRetention
	}
	if opts.CleanupInterval <= 0 {
		opts.CleanupInterval = DefaultCleanupInterval
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return opts
}

func openLogFile(path string) (*os.File, int64, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, 0, fmt.Errorf("打开诊断日志: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, 0, fmt.Errorf("读取诊断日志状态: %w", err)
	}
	return file, info.Size(), nil
}

func (l *Logger) openCurrent() error {
	file, size, err := openLogFile(l.path)
	if err != nil {
		return err
	}
	if size == 0 {
		header := []byte(logHeaderPrefix + l.opts.Now().UTC().Format(time.RFC3339) + "\n")
		n, writeErr := file.Write(header)
		if writeErr != nil {
			_ = file.Close()
			return fmt.Errorf("写入诊断日志时间标记: %w", writeErr)
		}
		if n != len(header) {
			_ = file.Close()
			return fmt.Errorf("写入诊断日志时间标记: %w", io.ErrShortWrite)
		}
		size = int64(n)
	}
	l.file = file
	l.size = size
	return nil
}

func (l *Logger) closeFile() error {
	if l.file == nil {
		l.size = 0
		return nil
	}
	file := l.file
	l.file = nil
	l.size = 0
	var errs []error
	if err := file.Sync(); err != nil {
		errs = append(errs, fmt.Errorf("同步诊断日志: %w", err))
	}
	if err := file.Close(); err != nil {
		errs = append(errs, fmt.Errorf("关闭诊断日志: %w", err))
	}
	return errors.Join(errs...)
}

func expiredBefore(path string, info os.FileInfo, cutoff time.Time) (bool, error) {
	created, err := fileCreated(path, info.ModTime())
	if err != nil {
		return false, fmt.Errorf("读取诊断日志创建时间 %s: %w", filepath.Base(path), err)
	}
	return created.Before(cutoff), nil
}

func fileCreated(path string, fallback time.Time) (time.Time, error) {
	file, err := os.Open(path)
	if err != nil {
		return fallback, err
	}
	defer file.Close()
	line, err := bufio.NewReader(io.LimitReader(file, 1024)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fallback, err
	}
	line = strings.TrimSpace(line)
	if value, ok := strings.CutPrefix(line, logHeaderPrefix); ok {
		if created, err := time.Parse(time.RFC3339, value); err == nil {
			return created, nil
		}
	}
	if len(line) >= len("2006/01/02 15:04:05") {
		if created, err := time.ParseInLocation("2006/01/02 15:04:05", line[:19], fallback.Location()); err == nil {
			return created, nil
		}
	}
	return fallback, nil
}

// Path returns the active diagnostic log path.
func (l *Logger) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

// Install sends the process-global standard logger to stderr and this file.
func (l *Logger) Install() error {
	if l == nil {
		return errors.New("诊断日志为空")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return os.ErrClosed
	}
	if l.installed {
		return nil
	}
	l.previousOutput = log.Writer()
	l.previousFlags = log.Flags()
	l.previousPrefix = log.Prefix()
	log.SetOutput(io.MultiWriter(os.Stderr, l))
	l.installed = true
	return nil
}

// Write implements io.Writer with per-record truncation and rotation.
func (l *Logger) Write(p []byte) (int, error) {
	if l == nil {
		return 0, errors.New("诊断日志为空")
	}
	originalLen := len(p)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return 0, os.ErrClosed
	}
	var err error
	if p, err = l.prepareWrite(p, l.opts.Now()); err != nil {
		return 0, err
	}
	n, err := l.file.Write(p)
	l.size += int64(n)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return n, err
	}
	return originalLen, nil
}

// Close restores the process logger, then flushes and closes the file.
func (l *Logger) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	if l.installed {
		log.SetOutput(l.previousOutput)
		log.SetFlags(l.previousFlags)
		log.SetPrefix(l.previousPrefix)
		l.installed = false
	}
	file := l.file
	l.file = nil
	l.closed = true
	l.mu.Unlock()

	var errs []error
	if file != nil {
		if err := file.Sync(); err != nil {
			errs = append(errs, fmt.Errorf("同步诊断日志: %w", err))
		}
		if err := file.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭诊断日志: %w", err))
		}
	}
	return errors.Join(errs...)
}

func (l *Logger) prepareWrite(p []byte, now time.Time) ([]byte, error) {
	if l.lastCleanup.IsZero() || !now.Before(l.lastCleanup.Add(l.opts.CleanupInterval)) {
		if err := l.maintain(now); err != nil {
			return nil, err
		}
	}
	if l.file == nil {
		if err := l.openCurrent(); err != nil {
			return nil, err
		}
	}
	p = truncateRecord(p, l.opts.MaxBytes)
	if l.size+int64(len(p)) > l.opts.MaxBytes {
		if err := l.rotate(); err != nil {
			return nil, err
		}
	}
	if remaining := l.opts.MaxBytes - l.size; int64(len(p)) > remaining {
		p = truncateRecord(p, remaining)
	}
	return p, nil
}

func (l *Logger) rotate() error {
	if err := l.closeFile(); err != nil {
		return err
	}

	for index := l.opts.Backups; index >= 1; index-- {
		target := backupPath(l.path, index)
		source := l.path
		if index > 1 {
			source = backupPath(l.path, index-1)
		}
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("清理旧诊断日志: %w", err)
		}
		if err := os.Rename(source, target); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("轮转诊断日志: %w", err)
		}
	}
	if err := l.openCurrent(); err != nil {
		return err
	}
	return l.maintain(l.opts.Now())
}

func (l *Logger) maintain(now time.Time) error {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return fmt.Errorf("读取诊断日志目录: %w", err)
	}
	cutoff := now.Add(-l.opts.Retention)
	removeCurrent := false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		remove := false
		switch {
		case name == logName:
			info, err := entry.Info()
			if err != nil {
				return fmt.Errorf("读取当前诊断日志状态: %w", err)
			}
			expired, err := expiredBefore(l.path, info, cutoff)
			if err != nil {
				return err
			}
			removeCurrent = info.Size() > l.opts.MaxBytes || expired
		case strings.HasPrefix(name, logName+"."):
			suffix := strings.TrimPrefix(name, logName+".")
			index, err := strconv.Atoi(suffix)
			if err != nil || suffix != strconv.Itoa(index) || index < 1 {
				continue
			}
			if index > l.opts.Backups {
				remove = true
			} else {
				info, err := entry.Info()
				if err != nil {
					return fmt.Errorf("读取诊断日志备份状态: %w", err)
				}
				expired, err := expiredBefore(filepath.Join(l.dir, name), info, cutoff)
				if err != nil {
					return err
				}
				remove = info.Size() > l.opts.MaxBytes || expired
			}
		default:
			continue
		}
		if !remove {
			continue
		}
		if err := os.Remove(filepath.Join(l.dir, name)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("清理诊断日志 %s: %w", name, err)
		}
	}
	if removeCurrent {
		if err := l.closeFile(); err != nil {
			return err
		}
		if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("清理过期诊断日志: %w", err)
		}
	}
	if l.file == nil {
		if err := l.openCurrent(); err != nil {
			return err
		}
	}
	l.lastCleanup = now
	return nil
}

func backupPath(path string, index int) string {
	return path + "." + strconv.Itoa(index)
}

func truncateRecord(p []byte, limit int64) []byte {
	if int64(len(p)) <= limit {
		return p
	}
	tail := []byte(truncationMarker)
	allowed := limit - int64(len(tail))
	if allowed < 0 {
		allowed = 0
		tail = nil
	}
	p = p[:validPrefix(p, int(allowed))]
	result := make([]byte, 0, int(allowed)+len(tail))
	result = append(result, p...)
	result = append(result, tail...)
	if len(result) > int(limit) {
		result = result[:int(limit)]
	}
	return result
}

func validPrefix(p []byte, limit int) int {
	if limit >= len(p) {
		return len(p)
	}
	for limit > 0 && !utf8.Valid(p[:limit]) {
		limit--
	}
	return limit
}

// Sanitize removes paths and contact-shaped values from a diagnostic value
// and bounds its length.
func Sanitize(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = quotedPathPattern.ReplaceAllString(value, "<path>")
	value = absolutePathPattern.ReplaceAllString(value, "<path>")
	value = emailPattern.ReplaceAllString(value, "<email>")
	value = phonePattern.ReplaceAllString(value, "<phone>")
	value = strings.TrimSpace(value)
	const maxRunes = 512
	if utf8.RuneCountInString(value) > maxRunes {
		value = string([]rune(value)[:maxRunes-1]) + "…"
	}
	return value
}

// SafeError returns a diagnostic detail without filesystem paths or unknown
// error text. Path and link errors keep their operation and safe cause.
func SafeError(err error) string {
	if err == nil {
		return ""
	}
	return Sanitize(safeErrorDetail(err))
}

func safeErrorDetail(err error) string {
	if err == nil {
		return ""
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return safeOperation(pathErr.Op, pathErr.Err)
	}
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		return safeOperation(linkErr.Op, linkErr.Err)
	}
	var syscallErr *os.SyscallError
	if errors.As(err, &syscallErr) {
		return safeOperation(syscallErr.Syscall, syscallErr.Err)
	}
	if detail, ok := knownMyGoPDFErrorDetail(err); ok {
		return detail
	}
	switch {
	case errors.Is(err, context.Canceled):
		return context.Canceled.Error()
	case errors.Is(err, context.DeadlineExceeded):
		return context.DeadlineExceeded.Error()
	case errors.Is(err, io.EOF):
		return io.EOF.Error()
	case errors.Is(err, io.ErrUnexpectedEOF):
		return io.ErrUnexpectedEOF.Error()
	case errors.Is(err, io.ErrShortWrite):
		return io.ErrShortWrite.Error()
	case errors.Is(err, os.ErrClosed):
		return os.ErrClosed.Error()
	}
	if _, ok := err.(syscall.Errno); ok {
		return err.Error()
	}
	if _, ok := err.(*exec.ExitError); ok {
		return err.Error()
	}
	if cause := errors.Unwrap(err); cause != nil {
		return safeErrorDetail(cause)
	}
	return "<redacted>"
}

func knownMyGoPDFErrorDetail(err error) (string, bool) {
	message := err.Error()
	for _, class := range knownMyGoPDFErrors {
		for _, marker := range class.markers {
			if strings.Contains(message, marker) {
				return "reason_code=" + class.code + " reason=" + class.reason, true
			}
		}
	}
	return "", false
}

func safeOperation(operation string, cause error) string {
	operation = strings.TrimSpace(operation)
	detail := safeErrorDetail(cause)
	if operation == "" {
		return detail
	}
	if detail == "" {
		return operation
	}
	return operation + ": " + detail
}
