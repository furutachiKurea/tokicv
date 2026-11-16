//go:build linux && cgo

package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"tokicv/internal/render"
)

const textLayerSelectionSource = `# 行选区验证

示例增长第二页 ①Ａ

## 兼容字符

正文：户门生大一门页风

- 列表折行单行：这是一条用于验证单视觉行拖选的较长条目，文字在版心中自然换行，但本次只选中第一视觉行，不夹带后续内容，也不把列表标记复制进正文
- 列表折行两行：这是一条用于验证跨视觉行拖选的较长条目，第一视觉行应当被完整选中，第二视觉行也应包含在系统剪贴板中，并且不夹带下一条正文与页尾内容
- 列表第二条：不应夹带

尾行：选区顺序和剪贴板内容应保持从首行到中段再到页尾
`

type textLayerPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type textLayerTarget struct {
	Name     string         `json:"name"`
	Expected string         `json:"expected"`
	Start    textLayerPoint `json:"start"`
	End      textLayerPoint `json:"end"`
}

type textLayerReady struct {
	Error     string `json:"error,omitempty"`
	PageCount int    `json:"pageCount"`
	Viewport  struct {
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	} `json:"viewport"`
	SpanCount int               `json:"spanCount"`
	Targets   []textLayerTarget `json:"targets"`
}

type trustedPointerEvent struct {
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Button  int     `json:"button"`
	Buttons int     `json:"buttons"`
	Trusted bool    `json:"trusted"`
}

type textLayerInputReport struct {
	Error            string               `json:"error,omitempty"`
	Selection        string               `json:"selection"`
	PointerDown      int                  `json:"pointerDown"`
	PointerMove      int                  `json:"pointerMove"`
	PointerUp        int                  `json:"pointerUp"`
	MouseDown        int                  `json:"mouseDown"`
	MouseMove        int                  `json:"mouseMove"`
	MouseUp          int                  `json:"mouseUp"`
	SelectionChanges int                  `json:"selectionChanges"`
	AllTrusted       bool                 `json:"allTrusted"`
	LastDown         *trustedPointerEvent `json:"lastDown"`
	LastUp           *trustedPointerEvent `json:"lastUp"`
}

type textLayerCopyReport struct {
	Error     string `json:"error,omitempty"`
	Selection string `json:"selection"`
	Trusted   bool   `json:"trusted"`
}

type textLayerServer struct {
	server    *httptest.Server
	html      []byte
	prepareJS []byte
	ready     chan textLayerReady
	inputs    chan textLayerInputReport
	copies    chan textLayerCopyReport
	errors    chan string
}

type isolatedX11Display struct {
	runtimeDir  string
	waylandName string
	x11Name     string
	xauthority  string
	dbusAddress string
	logPath     string
	process     *exec.Cmd
}

type x11TestInput struct {
	path   string
	env    []string
	width  int
	height int
}

func TestPDFJSTextLayerMouseSelectionCopiesExactText(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Second)
	defer cancel()

	display := startIsolatedX11(t)
	binary := buildSelectionTestBinary(t)
	env := selectionWaylandEnv(t.TempDir(), display)
	pdf := exportSelectionTestPDF(t, binary, env)

	pages := newTextLayerServer(t, pdf)
	t.Cleanup(pages.server.Close)
	userData := t.TempDir()
	browserLog, browserExited, browserWaitErr := startTextLayerBrowser(
		t, display, userData, pages.server.URL,
	)

	var ready textLayerReady
	select {
	case ready = <-pages.ready:
	case message := <-pages.errors:
		t.Fatalf("PDF.js text layer page: %s; browser log: %s", message, browserLog)
	case <-browserExited:
		t.Fatalf("MiniBrowser exited before the text layer was ready: %v; log: %s", *browserWaitErr, browserLog)
	case <-ctx.Done():
		t.Fatalf("wait for PDF.js text layer: %v; browser log: %s", ctx.Err(), browserLog)
	}
	if ready.Error != "" {
		t.Fatalf("PDF.js text layer: %s", ready.Error)
	}
	if ready.PageCount != 1 {
		t.Fatalf("fresh selection PDF has %d pages, want one", ready.PageCount)
	}
	if ready.SpanCount < 5 || len(ready.Targets) != 5 {
		t.Fatalf("text layer spans = %d, targets = %d, want five targets", ready.SpanCount, len(ready.Targets))
	}

	input, err := buildX11TestInput(t, display)
	if err != nil {
		t.Fatal(err)
	}
	if int(ready.Viewport.Width) != input.width || int(ready.Viewport.Height) != input.height {
		t.Fatalf("fullscreen viewport = %.0fx%.0f, isolated screen = %dx%d",
			ready.Viewport.Width, ready.Viewport.Height, input.width, input.height)
	}

	for _, target := range ready.Targets {
		start := image.Pt(int(target.Start.X), int(target.Start.Y))
		end := image.Pt(int(target.End.X), int(target.End.Y))
		if err := input.ClearAndFocus(); err != nil {
			t.Fatalf("%s: clear previous selection: %v", target.Name, err)
		}
		if err := input.Drag(start, end); err != nil {
			t.Fatalf("%s: inject pointer drag: %v", target.Name, err)
		}

		report := waitForInputReport(t, ctx, pages, browserExited, browserWaitErr, browserLog, target.Name)
		assertTrustedMouseReport(t, target, start, end, report)

		if err := input.Copy(); err != nil {
			t.Fatalf("%s: inject Ctrl+C: %v", target.Name, err)
		}
		copied := waitForCopyReport(t, ctx, pages, browserExited, browserWaitErr, browserLog, target.Name)
		if !copied.Trusted {
			t.Fatalf("%s: copy event was not trusted", target.Name)
		}
		if got, want := canonicalSelection(copied.Selection), canonicalSelection(target.Expected); got != want {
			t.Fatalf("%s: copy-event selection = %q, want %q", target.Name, got, want)
		}
		clipboard := waitForClipboard(t, display, target.Expected)

		if radicals := compatibilityRadicals(clipboard); len(radicals) != 0 {
			t.Fatalf("%s: system clipboard contains compatibility radicals %U: %q", target.Name, radicals, clipboard)
		}
		t.Logf("input=%s drag=(%d,%d)->(%d,%d) pointer=%d/%d/%d mouse=%d/%d/%d trusted=%t selection=%q clipboard=%q",
			target.Name, start.X, start.Y, end.X, end.Y,
			report.PointerDown, report.PointerMove, report.PointerUp,
			report.MouseDown, report.MouseMove, report.MouseUp,
			report.AllTrusted, canonicalSelection(report.Selection), clipboard)
	}
}

func newTextLayerServer(t *testing.T, pdf []byte) *textLayerServer {
	t.Helper()
	pages := &textLayerServer{
		html:      bytes.ReplaceAll([]byte(textLayerSelectionHTML), []byte("__TOKICV_PREPARE__"), []byte(`import preparedPages from "./prepare.js";`)),
		prepareJS: []byte("export default await (" + render.PDFJSPrepareScript(pdf) + ");\n"),
		ready:     make(chan textLayerReady, 2),
		inputs:    make(chan textLayerInputReport, 8),
		copies:    make(chan textLayerCopyReport, 8),
		errors:    make(chan string, 4),
	}
	pages.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(pages.html)
		case "/prepare.js":
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			_, _ = w.Write(pages.prepareJS)
		case "/ready":
			var report textLayerReady
			if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
				select {
				case pages.errors <- "decode ready report: " + err.Error():
				default:
				}
				http.Error(w, "bad report", http.StatusBadRequest)
				return
			}
			pages.ready <- report
		case "/input":
			var report textLayerInputReport
			if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
				select {
				case pages.errors <- "decode input report: " + err.Error():
				default:
				}
				http.Error(w, "bad report", http.StatusBadRequest)
				return
			}
			pages.inputs <- report
		case "/copy":
			var report textLayerCopyReport
			if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
				select {
				case pages.errors <- "decode copy report: " + err.Error():
				default:
				}
				http.Error(w, "bad report", http.StatusBadRequest)
				return
			}
			pages.copies <- report
		case "/error":
			var report struct {
				Error string `json:"error"`
			}
			if err := json.NewDecoder(r.Body).Decode(&report); err == nil && report.Error != "" {
				select {
				case pages.errors <- report.Error:
				default:
				}
			}
		default:
			http.NotFound(w, r)
			return
		}
	}))
	return pages
}

func waitForInputReport(
	t *testing.T,
	ctx context.Context,
	pages *textLayerServer,
	browserExited <-chan struct{},
	browserWaitErr *error,
	browserLog string,
	name string,
) textLayerInputReport {
	t.Helper()
	select {
	case report := <-pages.inputs:
		return report
	case message := <-pages.errors:
		t.Fatalf("%s: PDF.js text layer page: %s", name, message)
	case <-browserExited:
		t.Fatalf("%s: MiniBrowser exited before mouse input: %v; log: %s", name, *browserWaitErr, browserLog)
	case <-ctx.Done():
		t.Fatalf("%s: wait for mouse input report: %v; log: %s", name, ctx.Err(), browserLog)
	}
	return textLayerInputReport{}
}

func assertTrustedMouseReport(
	t *testing.T,
	target textLayerTarget,
	start, end image.Point,
	report textLayerInputReport,
) {
	t.Helper()
	if report.Error != "" {
		t.Fatalf("%s: input report: %s", target.Name, report.Error)
	}
	if got, want := canonicalSelection(report.Selection), canonicalSelection(target.Expected); got != want {
		t.Fatalf("%s: selection = %q, want %q", target.Name, got, want)
	}
	if !report.AllTrusted {
		t.Fatalf("%s: at least one pointer or mouse event was not trusted", target.Name)
	}
	if report.PointerDown == 0 || report.PointerMove == 0 || report.PointerUp == 0 ||
		report.MouseDown == 0 || report.MouseMove == 0 || report.MouseUp == 0 ||
		report.SelectionChanges == 0 {
		t.Fatalf("%s: incomplete input evidence: %+v", target.Name, report)
	}
	if report.LastDown == nil || report.LastUp == nil {
		t.Fatalf("%s: input report has no last pointer event: %+v", target.Name, report)
	}
	if !report.LastDown.Trusted || !report.LastUp.Trusted ||
		report.LastDown.Button != 0 || report.LastDown.Buttons != 1 || report.LastUp.Buttons != 0 {
		t.Fatalf("%s: invalid pointer button evidence: down=%+v up=%+v", target.Name, report.LastDown, report.LastUp)
	}
	if delta(report.LastDown.X, float64(start.X)) > 1 || delta(report.LastDown.Y, float64(start.Y)) > 1 ||
		delta(report.LastUp.X, float64(end.X)) > 1 || delta(report.LastUp.Y, float64(end.Y)) > 1 {
		t.Fatalf("%s: page saw drag %+v -> %+v, injected %v -> %v",
			target.Name, report.LastDown, report.LastUp, start, end)
	}
}

func waitForCopyReport(
	t *testing.T,
	ctx context.Context,
	pages *textLayerServer,
	browserExited <-chan struct{},
	browserWaitErr *error,
	browserLog string,
	name string,
) textLayerCopyReport {
	t.Helper()
	select {
	case report := <-pages.copies:
		return report
	case message := <-pages.errors:
		t.Fatalf("%s: PDF.js text layer page: %s", name, message)
	case <-browserExited:
		t.Fatalf("%s: MiniBrowser exited before copy: %v; log: %s", name, *browserWaitErr, browserLog)
	case <-ctx.Done():
		t.Fatalf("%s: wait for copy event: %v; log: %s", name, ctx.Err(), browserLog)
	}
	return textLayerCopyReport{}
}

func waitForClipboard(t *testing.T, display *isolatedX11Display, expected string) string {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	var last string
	var lastErr error
	for time.Now().Before(deadline) {
		value, err := readClipboard(display)
		if err == nil {
			last = value
			if canonicalSelection(value) == canonicalSelection(expected) {
				return value
			}
		} else {
			lastErr = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("system clipboard = %q, want %q; last error: %v", last, expected, lastErr)
	return ""
}

func readClipboard(display *isolatedX11Display) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "xsel", "-b", "-o")
	cmd.Env = selectionX11Env(display)
	output, err := cmd.Output()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return "", fmt.Errorf("xsel: %w: %s", err, bytes.TrimSpace(exit.Stderr))
		}
		return "", err
	}
	return string(output), nil
}

func canonicalSelection(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	return strings.TrimSuffix(value, "\n")
}

func compatibilityRadicals(value string) []rune {
	var found []rune
	for _, r := range value {
		if r >= 0x2E80 && r <= 0x2EFF || r >= 0x2F00 && r <= 0x2FDF {
			found = append(found, r)
		}
	}
	return found
}

func delta(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}

func buildSelectionTestBinary(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	path := filepath.Join(t.TempDir(), "tokicv")
	cmd := exec.Command("go", "build", "-o", path, ".")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build tokicv binary: %v\n%s", err, output)
	}
	return path
}

func exportSelectionTestPDF(t *testing.T, binary string, env []string) []byte {
	t.Helper()
	temp := t.TempDir()
	input := filepath.Join(temp, "selection.md")
	output := filepath.Join(temp, "selection.pdf")
	if err := os.WriteFile(input, []byte(textLayerSelectionSource), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "export", input, "--pdf", output, "--timeout", "30s")
	cmd.Env = env
	combined, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("export fresh selection PDF timed out: %v", ctx.Err())
	}
	if err != nil {
		t.Fatalf("export fresh selection PDF: %v\n%s", err, combined)
	}
	pdf, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatalf("selection export is not a PDF: %q", pdf[:min(len(pdf), 16)])
	}
	return pdf
}

func startTextLayerBrowser(
	t *testing.T,
	display *isolatedX11Display,
	profile string,
	url string,
) (string, chan struct{}, *error) {
	t.Helper()
	browser, err := findMiniBrowser()
	if err != nil {
		t.Skip(err.Error())
	}
	logPath := filepath.Join(t.TempDir(), "minibrowser.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = logFile.Close() })

	cmdArgs := []string{
		"--private",
		"--full-screen",
		"--profile-dir=" + filepath.Join(profile, "profile"),
		url,
	}
	cmd := exec.Command(browser, cmdArgs...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = selectionX11EnvAt(display, profile)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	exited := make(chan struct{})
	waitErr := new(error)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start MiniBrowser: %v", err)
	}
	go func() {
		*waitErr = cmd.Wait()
		close(exited)
	}()
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		select {
		case <-exited:
		case <-time.After(3 * time.Second):
		}
	})
	return logPath, exited, waitErr
}

func findMiniBrowser() (string, error) {
	if path, err := exec.LookPath("MiniBrowser"); err == nil {
		return path, nil
	}
	for _, path := range []string{
		"/usr/libexec/webkit2gtk-4.1/MiniBrowser",
		"/usr/lib64/webkit2gtk-4.1/MiniBrowser",
	} {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", errors.New("MiniBrowser is required for the PDF.js text layer acceptance test")
}

func startIsolatedX11(t *testing.T) *isolatedX11Display {
	t.Helper()
	mutter, err := exec.LookPath("mutter")
	if err != nil {
		t.Skip("mutter is required for the isolated graphical acceptance test")
	}
	root, err := os.MkdirTemp("", "tokicv-x11-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	for _, name := range []string{"home", "config", "cache", "data"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	logPath := filepath.Join(root, "mutter.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	dbusAddress := startIsolatedDBus(t, root)
	display := &isolatedX11Display{
		runtimeDir:  root,
		waylandName: "wayland-0",
		dbusAddress: dbusAddress,
		logPath:     logPath,
	}
	cmd := exec.Command(mutter,
		"--headless",
		"--wayland",
		"--wayland-display", display.waylandName,
		"--virtual-monitor", "1440x900",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = append(environmentWithout(
		"DISPLAY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR", "GDK_BACKEND",
		"HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME",
		"DBUS_SESSION_BUS_ADDRESS",
	),
		"HOME="+filepath.Join(root, "home"),
		"XDG_CONFIG_HOME="+filepath.Join(root, "config"),
		"XDG_DATA_HOME="+filepath.Join(root, "data"),
		"XDG_CACHE_HOME="+filepath.Join(root, "cache"),
		"XDG_RUNTIME_DIR="+display.runtimeDir,
		"WAYLAND_DISPLAY="+display.waylandName,
		"GDK_BACKEND=wayland",
		"GIO_USE_VFS=local",
		"GSETTINGS_BACKEND=memory",
		"DBUS_SESSION_BUS_ADDRESS="+display.dbusAddress,
		"DISPLAY=",
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		t.Fatalf("start isolated mutter: %v", err)
	}
	display.process = cmd
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
		_ = logFile.Close()
	})

	displayPattern := regexp.MustCompile(`Using public X11 display :([0-9]+)`)
	var authPattern = filepath.Join(root, ".mutter-Xwaylandauth.*")
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(logPath)
		match := displayPattern.FindSubmatch(data)
		authFiles, _ := filepath.Glob(authPattern)
		if len(match) == 2 && len(authFiles) > 0 {
			display.x11Name = ":" + string(match[1])
			display.xauthority = authFiles[0]
			return display
		}
		time.Sleep(25 * time.Millisecond)
	}
	data, _ := os.ReadFile(logPath)
	t.Fatalf("isolated mutter did not publish Xwayland: %s", data)
	return nil
}

func selectionWaylandEnv(temp string, display *isolatedX11Display) []string {
	return append(environmentWithout(
		"DISPLAY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR", "GDK_BACKEND",
		"HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME",
		"DBUS_SESSION_BUS_ADDRESS",
	),
		"HOME="+filepath.Join(temp, "home"),
		"XDG_CONFIG_HOME="+filepath.Join(temp, "config"),
		"XDG_DATA_HOME="+filepath.Join(temp, "data"),
		"XDG_CACHE_HOME="+filepath.Join(temp, "cache"),
		"XDG_RUNTIME_DIR="+display.runtimeDir,
		"WAYLAND_DISPLAY="+display.waylandName,
		"GDK_BACKEND=wayland",
		"GIO_USE_VFS=local",
		"GSETTINGS_BACKEND=memory",
		"DBUS_SESSION_BUS_ADDRESS="+display.dbusAddress,
		"DISPLAY=",
	)
}

func selectionX11Env(display *isolatedX11Display) []string {
	return selectionX11EnvAt(display, display.runtimeDir)
}

func selectionX11EnvAt(display *isolatedX11Display, root string) []string {
	for _, name := range []string{"home", "config", "cache", "data"} {
		_ = os.MkdirAll(filepath.Join(root, name), 0o700)
	}
	return append(environmentWithout(
		"DISPLAY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR", "GDK_BACKEND",
		"XAUTHORITY", "HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME",
		"DBUS_SESSION_BUS_ADDRESS",
	),
		"DISPLAY="+display.x11Name,
		"XAUTHORITY="+display.xauthority,
		"GDK_BACKEND=x11",
		"HOME="+filepath.Join(root, "home"),
		"XDG_CONFIG_HOME="+filepath.Join(root, "config"),
		"XDG_DATA_HOME="+filepath.Join(root, "data"),
		"XDG_CACHE_HOME="+filepath.Join(root, "cache"),
		"XDG_RUNTIME_DIR="+display.runtimeDir,
		"DBUS_SESSION_BUS_ADDRESS="+display.dbusAddress,
		"GIO_USE_VFS=local",
		"GSETTINGS_BACKEND=memory",
	)
}

func buildX11TestInput(t *testing.T, display *isolatedX11Display) (*x11TestInput, error) {
	t.Helper()
	compiler, err := exec.LookPath("cc")
	if err != nil {
		return nil, errors.New("a C compiler is required for the XTest acceptance driver")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "tokicv-x11-input.c")
	path := filepath.Join(dir, "tokicv-x11-input")
	if err := os.WriteFile(source, []byte(x11InputHelperSource), 0o600); err != nil {
		return nil, err
	}
	build := exec.Command(compiler, source, "-o", path, "-lXtst", "-lX11")
	if output, err := build.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("compile XTest input driver: %w\n%s", err, output)
	}
	input := &x11TestInput{path: path, env: selectionX11Env(display)}
	size, err := input.run("size")
	if err != nil {
		return nil, err
	}
	fields := strings.Fields(string(size))
	if len(fields) != 2 {
		return nil, fmt.Errorf("XTest input driver size = %q", size)
	}
	input.width, err = strconv.Atoi(fields[0])
	if err != nil {
		return nil, fmt.Errorf("parse X11 width: %w", err)
	}
	input.height, err = strconv.Atoi(fields[1])
	if err != nil {
		return nil, fmt.Errorf("parse X11 height: %w", err)
	}
	return input, nil
}

func (input *x11TestInput) ClearAndFocus() error {
	if _, err := input.run("clear"); err != nil {
		return err
	}
	time.Sleep(80 * time.Millisecond)
	return nil
}

func (input *x11TestInput) Drag(start, end image.Point) error {
	for _, point := range []image.Point{start, end} {
		if point.X < 0 || point.Y < 0 || point.X >= input.width || point.Y >= input.height {
			return fmt.Errorf("pointer target %v is outside %dx%d", point, input.width, input.height)
		}
	}
	if _, err := input.run("drag", strconv.Itoa(start.X), strconv.Itoa(start.Y),
		strconv.Itoa(end.X), strconv.Itoa(end.Y)); err != nil {
		return err
	}
	time.Sleep(120 * time.Millisecond)
	return nil
}

func (input *x11TestInput) Copy() error {
	if _, err := input.run("copy"); err != nil {
		return err
	}
	time.Sleep(120 * time.Millisecond)
	return nil
}

func (input *x11TestInput) run(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, input.path, args...)
	cmd.Env = input.env
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("XTest input driver timed out: %v", ctx.Err())
	}
	if err != nil {
		return nil, fmt.Errorf("XTest input driver %v: %w: %s", args, err, bytes.TrimSpace(output))
	}
	return output, nil
}

const x11InputHelperSource = `#include <X11/Xlib.h>
#include <X11/keysym.h>
#include <X11/extensions/XTest.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

static int focus_under_pointer(Display *display) {
	Window root = DefaultRootWindow(display);
	Window root_return, child_return;
	int root_x, root_y, win_x, win_y;
	unsigned int mask;
	if (!XQueryPointer(display, root, &root_return, &child_return,
			&root_x, &root_y, &win_x, &win_y, &mask) || child_return == None) {
		return 0;
	}
	XSetInputFocus(display, child_return, RevertToPointerRoot, CurrentTime);
	XSync(display, False);
	return 1;
}

static int clear_selection(Display *display) {
	if (!XTestFakeMotionEvent(display, DefaultScreen(display), 4, 4, CurrentTime)) return 0;
	XSync(display, False);
	usleep(40000);
	if (!focus_under_pointer(display)) return 0;
	usleep(80000);
	return 1;
}

static int drag_selection(Display *display, int x1, int y1, int x2, int y2) {
	int screen = DefaultScreen(display);
	if (!XTestFakeMotionEvent(display, screen, x1, y1, CurrentTime)) return 0;
	XSync(display, False);
	usleep(60000);
	if (!XTestFakeButtonEvent(display, 1, True, CurrentTime)) return 0;
	XSync(display, False);
	usleep(120000);
	for (int step = 1; step <= 16; step++) {
		int x = x1 + (x2 - x1) * step / 16;
		int y = y1 + (y2 - y1) * step / 16;
		if (!XTestFakeMotionEvent(display, screen, x, y, CurrentTime)) return 0;
		XSync(display, False);
		usleep(18000);
	}
	if (!XTestFakeButtonEvent(display, 1, False, CurrentTime)) return 0;
	XSync(display, False);
	usleep(120000);
	return 1;
}

static int copy_selection(Display *display) {
	KeyCode control = XKeysymToKeycode(display, XK_Control_L);
	KeyCode c = XKeysymToKeycode(display, XK_c);
	if (control == 0 || c == 0) return 0;
	XTestFakeKeyEvent(display, control, True, CurrentTime);
	XTestFakeKeyEvent(display, c, True, CurrentTime);
	XTestFakeKeyEvent(display, c, False, CurrentTime);
	XTestFakeKeyEvent(display, control, False, CurrentTime);
	XSync(display, False);
	usleep(150000);
	return 1;
}

int main(int argc, char **argv) {
	Display *display = XOpenDisplay(NULL);
	if (!display) {
		fprintf(stderr, "cannot open DISPLAY\n");
		return 2;
	}
	int event_base, error_base, major, minor;
	if (!XTestQueryExtension(display, &event_base, &error_base, &major, &minor)) {
		fprintf(stderr, "XTest extension is unavailable\n");
		XCloseDisplay(display);
		return 3;
	}
	int status = 4;
	if (argc == 2 && strcmp(argv[1], "size") == 0) {
		int screen = DefaultScreen(display);
		printf("%d %d\n", DisplayWidth(display, screen), DisplayHeight(display, screen));
		status = 0;
	} else if (argc == 2 && strcmp(argv[1], "clear") == 0) {
		status = clear_selection(display) ? 0 : 5;
	} else if (argc == 6 && strcmp(argv[1], "drag") == 0) {
		status = drag_selection(display, atoi(argv[2]), atoi(argv[3]),
			atoi(argv[4]), atoi(argv[5])) ? 0 : 6;
	} else if (argc == 2 && strcmp(argv[1], "copy") == 0) {
		status = copy_selection(display) ? 0 : 7;
	}
	if (status != 0) fprintf(stderr, "invalid command or XTest failure\n");
	XCloseDisplay(display);
	return status;
}
`

const textLayerSelectionHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<title>tokicv PDF.js selection acceptance</title>
<style>
html, body { margin: 0; padding: 0; background: #fff; }
#pages { padding: 12px; }
.page { position: relative; margin: 0; background: #fff; }
.page canvas { position: absolute; inset: 0; display: block; }
.textLayer {
  position: absolute; inset: 0; overflow: hidden; line-height: 1; text-align: initial;
  text-size-adjust: none; transform-origin: 0 0; z-index: 2; user-select: text;
  --scale-factor: 1; --total-scale-factor: 1; --scale-round-x: 1px; --scale-round-y: 1px;
}
.textLayer :is(span, br) {
  position: absolute; color: transparent; white-space: pre; cursor: text;
  transform-origin: 0% 0%; user-select: text;
}
.textLayer > :not(.markedContent),
.textLayer .markedContent span:not(.markedContent) {
  font-size: calc(var(--scale-factor) * var(--font-height));
  transform: rotate(var(--rotate, 0deg)) scaleX(var(--scale-x, 1));
}
</style>
</head>
<body>
<div id="pages"></div>
<script type="module">
__TOKICV_PREPARE__

const state = {
  pointerDown: 0,
  pointerMove: 0,
  pointerUp: 0,
  mouseDown: 0,
  mouseMove: 0,
  mouseUp: 0,
  selectionChanges: 0,
  allTrusted: true,
  lastDown: null,
  lastUp: null
};
let reportTimer = 0;

function post(path, value) {
  return fetch(path, {
    method: "POST",
    headers: {"Content-Type": "application/json"},
    body: JSON.stringify(value)
  });
}

function reportError(error) {
  post("/error", {error: String(error && error.stack || error)});
}

window.addEventListener("error", event => reportError(event.error || event.message));
window.addEventListener("unhandledrejection", event => reportError(event.reason));

function recordTrust(event) {
  if (!event.isTrusted) state.allTrusted = false;
}

document.addEventListener("pointerdown", event => {
  recordTrust(event);
  state.pointerDown++;
  state.lastDown = {
    x: event.clientX, y: event.clientY, button: event.button,
    buttons: event.buttons, trusted: event.isTrusted
  };
}, true);
document.addEventListener("pointermove", event => {
  recordTrust(event);
  state.pointerMove++;
}, true);
document.addEventListener("pointerup", event => {
  recordTrust(event);
  state.pointerUp++;
  state.lastUp = {
    x: event.clientX, y: event.clientY, button: event.button,
    buttons: event.buttons, trusted: event.isTrusted
  };
  clearTimeout(reportTimer);
  reportTimer = setTimeout(reportInput, 150);
}, true);
document.addEventListener("mousedown", event => {
  recordTrust(event);
  state.mouseDown++;
}, true);
document.addEventListener("mousemove", event => {
  recordTrust(event);
  state.mouseMove++;
}, true);
document.addEventListener("mouseup", event => {
  recordTrust(event);
  state.mouseUp++;
}, true);
document.addEventListener("selectionchange", () => {
  state.selectionChanges++;
});
document.addEventListener("copy", event => {
  recordTrust(event);
  post("/copy", {
    selection: window.getSelection().toString(),
    trusted: event.isTrusted
  });
}, true);

function reportInput() {
  post("/input", {
    selection: window.getSelection().toString(),
    pointerDown: state.pointerDown,
    pointerMove: state.pointerMove,
    pointerUp: state.pointerUp,
    mouseDown: state.mouseDown,
    mouseMove: state.mouseMove,
    mouseUp: state.mouseUp,
    selectionChanges: state.selectionChanges,
    allTrusted: state.allTrusted,
    lastDown: state.lastDown,
    lastUp: state.lastUp
  });
}

async function main() {
  const pdfjs = await import(globalThis.__tokicvPDFJS.moduleURL);
  const pdf = globalThis.__tokicvPDFJS.document;
  const page = await pdf.getPage(1);
  const viewport = page.getViewport({scale: 1});
  const holder = document.createElement("div");
  holder.className = "page";
  holder.style.width = Math.ceil(viewport.width) + "px";
  holder.style.height = Math.ceil(viewport.height) + "px";
  const canvas = document.createElement("canvas");
  canvas.width = Math.ceil(viewport.width);
  canvas.height = Math.ceil(viewport.height);
  holder.append(canvas);
  const layer = document.createElement("div");
  layer.className = "textLayer";
  holder.append(layer);
  document.getElementById("pages").append(holder);

  await page.render({
    canvasContext: canvas.getContext("2d"),
    viewport,
    intent: "print"
  }).promise;
  const textLayer = new pdfjs.TextLayer({
    textContentSource: await page.getTextContent(),
    container: layer,
    viewport
  });
  await textLayer.render();
  await document.fonts.ready;

  const spans = Array.from(layer.querySelectorAll("span"))
    .filter(span => span.textContent.trim())
    .map(span => ({text: span.textContent, rect: span.getBoundingClientRect()}))
    .sort((a, b) => a.rect.top - b.rect.top || a.rect.left - b.rect.left);
  const rows = [];
  for (const span of spans) {
    let row = rows[rows.length - 1];
    if (!row || Math.abs(span.rect.top - row.top) > 2) {
      row = {
        text: "", left: span.rect.left, right: span.rect.right,
        top: span.rect.top, bottom: span.rect.bottom
      };
      rows.push(row);
    }
    row.text += span.text;
    row.left = Math.min(row.left, span.rect.left);
    row.right = Math.max(row.right, span.rect.right);
    row.top = Math.min(row.top, span.rect.top);
    row.bottom = Math.max(row.bottom, span.rect.bottom);
  }
  for (const row of rows) row.text = row.text.replace(/\s+$/, "");

  const exact = text => rows.find(row => row.text === text);
  const starts = prefix => rows.find(row => row.text.startsWith(prefix));
  const first = exact("行选区验证");
  const middle = rows.find(row => row.text.includes("示例增长第二页") && row.text.includes("①Ａ"));
  const tail = starts("尾行：");
  const listOne = starts("列表折行单行：");
  const listTwoFirst = starts("列表折行两行：");
  const listTwoIndex = rows.indexOf(listTwoFirst);
  const listTwoSecond = listTwoIndex >= 0 ? rows[listTwoIndex + 1] : null;
  if (!first || !middle || !tail || !listOne || !listTwoFirst || !listTwoSecond) {
    throw new Error("selection targets are missing from the PDF.js text layer");
  }
  if (listTwoSecond.text.startsWith("列表第二条：") || listTwoSecond.text.startsWith("尾行：")) {
    throw new Error("the two-line list target did not wrap onto a continuation line");
  }

  const points = (startRow, endRow) => {
    const inset = row => Math.min(3, (row.right - row.left) / 4);
    return {
      start: {x: startRow.left + inset(startRow), y: (startRow.top + startRow.bottom) / 2},
      end: {x: endRow.right - inset(endRow), y: (endRow.top + endRow.bottom) / 2}
    };
  };
  const target = (name, expected, startRow, endRow) => {
    const positions = points(startRow, endRow);
    return {name, expected, start: positions.start, end: positions.end};
  };

  await post("/ready", {
    pageCount: pdf.numPages,
    viewport: {width: window.innerWidth, height: window.innerHeight},
    spanCount: spans.length,
    targets: [
      target("first_visual_line", first.text, first, first),
      target("middle_visual_line", "示例增长第二页 ①Ａ", middle, middle),
      target("tail_visual_line", tail.text, tail, tail),
      target("list_wrap_single_line", listOne.text, listOne, listOne),
      target("list_wrap_two_lines", listTwoFirst.text + "\n" + listTwoSecond.text,
        listTwoFirst, listTwoSecond)
    ]
  });
}

main().catch(reportError);
</script>
</body>
</html>`
