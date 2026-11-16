package export

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"tokicv/internal/dialect"
	"tokicv/internal/library"
	"tokicv/internal/render"
)

// Options configures a Resume Document Artifact export.
type Options struct {
	Input          string
	Output         string
	PNGOutput      string
	MarkdownOutput string
	Timeout        time.Duration
	Layout         render.Layout
	LayoutSet      bool
	Store          *library.Store
	FitOnePage     bool
	Notice         io.Writer
}

const (
	defaultTimeout = 30 * time.Second

	cssPixelsPerInch = 96.0
	pdfPointsPerInch = 72.0
	pageWidthPX      = 210.0 / 25.4 * cssPixelsPerInch
	pageHeightPX     = 297.0 / 25.4 * cssPixelsPerInch
	pageWidthPT      = 210.0 / 25.4 * pdfPointsPerInch
	pageHeightPT     = 297.0 / 25.4 * pdfPointsPerInch
)

// Run renders a Resume Document and writes its requested Markdown, PDF, and
// PNG Artifacts.
func Run(opts Options) error {
	if opts.Output == "" && opts.PNGOutput == "" && opts.MarkdownOutput == "" {
		return errors.New("需要 Markdown、PDF 或 PNG 输出路径")
	}
	if err := validateOutputDestinations(opts); err != nil {
		return err
	}
	if opts.Timeout <= 0 {
		opts.Timeout = defaultTimeout
	}
	target, err := resolveTarget(opts.Store, opts.Input)
	if err != nil {
		return fmt.Errorf("读取 Resume Document %q: %w", opts.Input, err)
	}
	source, err := target.Read()
	if err != nil {
		return err
	}
	if opts.MarkdownOutput != "" && opts.Output == "" && opts.PNGOutput == "" {
		return writeTargetMarkdown(target, opts.MarkdownOutput)
	}

	layout, err := effectiveLayout(opts.Store, target, opts.Layout, opts.LayoutSet)
	if err != nil {
		return err
	}
	html := render.RenderWithLayout(dialect.Parse(string(source)), layout)

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()
	var after postPrint
	if opts.PNGOutput != "" {
		after = rasterizePDF
	}
	var output renderOutput
	if opts.FitOnePage {
		output, err = renderFitWebView(ctx, html, opts.Timeout, layout, after, opts.Notice)
	} else {
		output, err = renderWebView(ctx, html, opts.Timeout, after)
	}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, errTimeout) {
			return timeoutError(opts.Timeout)
		}
		return err
	}
	if ctx.Err() != nil {
		return timeoutError(opts.Timeout)
	}

	var pngPaths []string
	if opts.PNGOutput != "" {
		pngPaths, err = ResolvePNGPaths(opts.PNGOutput, output.NumPages)
		if err != nil {
			return fmt.Errorf("解析 PNG 输出路径: %w", err)
		}
	}
	resolvedOutputs := []artifactOutput{
		{name: "--md", path: opts.MarkdownOutput},
		{name: "--pdf", path: opts.Output},
	}
	for index, path := range pngPaths {
		name := fmt.Sprintf("--png 第 %d 个页面", index+1)
		resolvedOutputs = append(resolvedOutputs, artifactOutput{name: name, path: path})
	}
	if err := validateArtifactPaths(resolvedOutputs); err != nil {
		return err
	}

	var staged []stagedArtifact
	if opts.MarkdownOutput != "" {
		temp, err := stageAtomic(opts.MarkdownOutput, source)
		if err != nil {
			return fmt.Errorf("写入 Markdown %q: %w", opts.MarkdownOutput, err)
		}
		staged = append(staged, stagedArtifact{path: opts.MarkdownOutput, temp: temp})
	}

	if opts.Output != "" {
		temp, err := stageAtomic(opts.Output, output.PDF)
		if err != nil {
			discardArtifacts(staged)
			return fmt.Errorf("写入 PDF %q: %w", opts.Output, err)
		}
		staged = append(staged, stagedArtifact{path: opts.Output, temp: temp})
	}

	var stalePNGs []string
	if opts.PNGOutput != "" {
		pngStaged, stale, err := stagePNGArtifacts(opts.PNGOutput, output.PNGs)
		if err != nil {
			discardArtifacts(staged)
			return fmt.Errorf("写入 PNG %q: %w", opts.PNGOutput, err)
		}
		staged = append(staged, pngStaged...)
		stalePNGs = stale
	}

	if err := commitArtifacts(staged); err != nil {
		return fmt.Errorf("提交导出结果: %w", err)
	}
	if err := removeStalePNGPages(stalePNGs); err != nil {
		return fmt.Errorf("清理旧 PNG 页面: %w", err)
	}
	return nil
}

var errTimeout = errors.New("渲染超时")

func timeoutError(timeout time.Duration) error {
	return fmt.Errorf("%w（%s）", errTimeout, timeout)
}

type webViewState struct {
	mu  sync.Mutex
	web *mygo.WebView
}

func (s *webViewState) set(web *mygo.WebView) {
	s.mu.Lock()
	s.web = web
	s.mu.Unlock()
}

func (s *webViewState) get() *mygo.WebView {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.web
}

type renderOutput struct {
	PDF      []byte
	PNGs     [][]byte
	NumPages int
	err      error
}

type postPrint func(context.Context, Page, []byte) ([][]byte, int, error)
type pageOperation func(context.Context, Page) (renderOutput, error)

func renderPDF(ctx context.Context, html string, timeout time.Duration) ([]byte, error) {
	output, err := renderWebView(ctx, html, timeout, nil)
	return output.PDF, err
}

func renderWebView(ctx context.Context, html string, timeout time.Duration, after postPrint) (renderOutput, error) {
	return withOffscreenPage(ctx, html, timeout, func(ctx context.Context, page Page) (renderOutput, error) {
		return exportWebView(ctx, page, after)
	})
}

func renderFitWebView(
	ctx context.Context,
	html string,
	timeout time.Duration,
	base render.Layout,
	after postPrint,
	notice io.Writer,
) (renderOutput, error) {
	return withOffscreenPage(ctx, html, timeout, func(ctx context.Context, page Page) (renderOutput, error) {
		fit, err := solveOnePageOnPage(ctx, page, base)
		if err != nil {
			return renderOutput{}, err
		}
		output, err := exportWebView(ctx, page, after)
		if err != nil {
			return renderOutput{}, err
		}
		if err := writeOnePageNotice(notice, fit.Notice); err != nil {
			return renderOutput{}, fmt.Errorf("写入 Smart One-Page 提示: %w", err)
		}
		return output, nil
	})
}

func withOffscreenPage(
	ctx context.Context,
	html string,
	_ time.Duration,
	operation pageOperation,
) (renderOutput, error) {
	return runPageOperation(ctx, offscreenPageRunner{}, html, operation)
}

type offscreenPageRunner struct{}

func (offscreenPageRunner) WithPage(
	ctx context.Context,
	html string,
	operation func(context.Context, Page) error,
) error {
	mygo.App.SetName("tokicv")
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)

	results := make(chan error, 1)
	var finishOnce sync.Once
	finish := func(err error) {
		finishOnce.Do(func() {
			results <- err
			mygo.App.Quit()
		})
	}

	mygo.App.OnReady(func() {
		if ctx.Err() != nil {
			finish(ctx.Err())
			return
		}

		state := &webViewState{}
		win := mygo.NewWindow(mygo.WindowOptions{
			Hidden:         true,
			Width:          794,
			Height:         1122,
			UseContentSize: true,
			Content: ui.View(func(c *ui.Context) {
				if web := state.get(); web != nil {
					ui.WebView(c, web).Fill()
				}
			}),
		})
		web, err := win.NewWebView(mygo.WebViewOptions{
			Page:            mygo.PageOptions{BackgroundThrottling: mygo.ThrottlingOff},
			BackgroundColor: "#ffffff",
		})
		if err != nil {
			finish(fmt.Errorf("创建离屏 WebView: %w", err))
			return
		}
		state.set(web)
		win.Invalidate()

		go func() {
			<-ctx.Done()
			finish(ctx.Err())
		}()
		web.Page().LoadHTML(html, "")
		go func() {
			finish(operation(ctx, web.Page()))
		}()
	})

	if err := mygo.App.Run(); err != nil {
		return fmt.Errorf("启动离屏 WebView: %w", err)
	}
	select {
	case err := <-results:
		return err
	default:
		return errors.New("离屏 WebView 在导出完成前退出")
	}
}

const readyScript = `(async () => {
  await document.fonts.ready;
  await new Promise(resolve => {
    let settled = false;
    const done = () => {
      if (!settled) {
        settled = true;
        resolve();
      }
    };
    requestAnimationFrame(() => requestAnimationFrame(done));
    setTimeout(done, 100);
  });
  const root = document.querySelector("#sheet-root");
  if (!root) return false;
  const rect = root.getBoundingClientRect();
  return document.readyState === "complete" &&
    document.fonts.status === "loaded" &&
    rect.width > 0 && rect.height > 0;
})()`

func waitForPage(ctx context.Context, page Page) error {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		value, err := page.EvalContext(ctx, readyScript)
		if err == nil {
			if ready, _ := value.(bool); ready {
				return nil
			}
		} else if ctx.Err() != nil {
			return ctx.Err()
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

const layoutScript = `(() => {
  const root = document.querySelector("#sheet-root");
  if (!root) return {text: "", lines: [], links: []};

  let text = "";
  const textWalker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  for (let node = textWalker.nextNode(); node; node = textWalker.nextNode()) {
    const parent = node.parentElement;
    if (!parent || parent.closest("svg, script, style")) continue;
    text += node.nodeValue || "";
  }

  const lines = [];
  for (const block of root.querySelectorAll("h1, h2, h3, p, li")) {
    const fontSize = parseFloat(getComputedStyle(block).fontSize);
    const range = document.createRange();
    range.selectNodeContents(block);
    const rects = Array.from(range.getClientRects())
      .filter(rect => rect.width > 0 && rect.height > 0)
      .sort((a, b) => a.top - b.top || a.left - b.left);
    let line = null;
    for (const rect of rects) {
      const next = {
        left: rect.left + window.scrollX,
        top: rect.top + window.scrollY,
        right: rect.right + window.scrollX,
        bottom: rect.bottom + window.scrollY,
        fontSize: fontSize
      };
      if (line && overlaps(line, next)) {
        line.left = Math.min(line.left, next.left);
        line.top = Math.min(line.top, next.top);
        line.right = Math.max(line.right, next.right);
        line.bottom = Math.max(line.bottom, next.bottom);
      } else {
        line = next;
        lines.push(line);
      }
    }
  }

  const links = Array.from(root.querySelectorAll("a[href]")).flatMap(link => {
    const href = link.getAttribute("href");
    if (!href) return [];
    const fontSize = parseFloat(getComputedStyle(link).fontSize);
    return Array.from(link.getClientRects())
      .filter(rect => rect.width > 0 && rect.height > 0)
      .map(rect => ({
        href: href,
        x: rect.left + window.scrollX,
        y: rect.top + window.scrollY,
        width: rect.width,
        height: rect.height,
        fontSize: fontSize
      }));
  });
  return {text: text, lines: lines, links: links};

  function overlaps(a, b) {
    const top = Math.max(a.top, b.top);
    const bottom = Math.min(a.bottom, b.bottom);
    return bottom - top > Math.min(a.bottom - a.top, b.bottom - b.top) * 0.5;
  }
})()`

func collectPageLayout(ctx context.Context, page Page) (pageLayout, error) {
	var layout pageLayout
	value, err := page.EvalContext(ctx, layoutScript)
	if err != nil {
		return layout, fmt.Errorf("读取 PDF 链接位置: %w", err)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return layout, errors.New("读取 PDF 链接位置: WebView 返回了无效结果")
	}
	layout.Text, _ = object["text"].(string)

	lineItems, ok := object["lines"].([]any)
	if !ok {
		return layout, errors.New("读取 PDF 链接位置: WebView 没有返回文本行")
	}
	for _, item := range lineItems {
		line, ok := item.(map[string]any)
		if !ok {
			continue
		}
		left, leftOK := line["left"].(float64)
		top, topOK := line["top"].(float64)
		right, rightOK := line["right"].(float64)
		bottom, bottomOK := line["bottom"].(float64)
		fontSize, fontSizeOK := line["fontSize"].(float64)
		if !leftOK || !topOK || !rightOK || !bottomOK || !fontSizeOK {
			continue
		}
		layout.Lines = append(layout.Lines, layoutLine{
			Left:     left,
			Top:      top,
			Right:    right,
			Bottom:   bottom,
			FontSize: fontSize,
		})
	}

	linkItems, ok := object["links"].([]any)
	if !ok {
		return layout, errors.New("读取 PDF 链接位置: WebView 没有返回链接")
	}
	for _, item := range linkItems {
		rect, ok := item.(map[string]any)
		if !ok {
			continue
		}
		href, _ := rect["href"].(string)
		x, xOK := rect["x"].(float64)
		y, yOK := rect["y"].(float64)
		width, widthOK := rect["width"].(float64)
		height, heightOK := rect["height"].(float64)
		fontSize, fontSizeOK := rect["fontSize"].(float64)
		if href == "" || !xOK || !yOK || !widthOK || !heightOK || !fontSizeOK {
			continue
		}
		layout.Links = append(layout.Links, linkTarget{
			URI:      href,
			X:        x,
			Y:        y,
			Width:    width,
			Height:   height,
			FontSize: fontSize,
		})
	}
	if len(layout.Links) > 0 && len(layout.Lines) == 0 {
		return layout, errors.New("读取 PDF 链接位置: 链接所在页面没有文本行")
	}
	return layout, nil
}

func exportWebView(ctx context.Context, page Page, after postPrint) (renderOutput, error) {
	if err := waitForPage(ctx, page); err != nil {
		return renderOutput{}, err
	}
	layout, err := collectPageLayout(ctx, page)
	if err != nil {
		return renderOutput{}, err
	}
	if ctx.Err() != nil {
		return renderOutput{}, ctx.Err()
	}

	pdf, err := printPDF(ctx, page)
	if err != nil {
		return renderOutput{}, err
	}
	pdf, err = repairPrintedPDF(pdf, layout.Text)
	if err != nil {
		return renderOutput{}, fmt.Errorf("修复 PDF 文本层: %w", err)
	}
	if len(layout.Links) > 0 && !pdfLinkRE.Match(pdf) {
		annotations, err := locateLinkAnnotations(pdf, layout)
		if err != nil {
			return renderOutput{}, err
		}
		pdf, err = addLinkAnnotations(pdf, annotations)
		if err != nil {
			return renderOutput{}, fmt.Errorf("写入 PDF 链接注解: %w", err)
		}
	}
	output := renderOutput{PDF: pdf}
	if after != nil {
		output.PNGs, output.NumPages, err = after(ctx, page, pdf)
		if err != nil {
			return renderOutput{}, err
		}
	}
	return output, nil
}

func printPDF(ctx context.Context, page Page) ([]byte, error) {
	pdf, err := withPrintLocale(ctx, runtime.GOOS, os.LookupEnv, os.Setenv, os.Unsetenv, func() ([]byte, error) {
		return page.PrintToPDF(mygo.PDFOptions{
			PageSize:   mygo.PageA4,
			Margins:    &mygo.Margins{},
			Background: true,
		})
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("打印 PDF: %w", err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		return nil, errors.New("打印结果不是有效的 PDF")
	}
	return pdf, nil
}

func solveOnePageOnPage(ctx context.Context, page Page, base render.Layout) (OnePageFit, error) {
	if err := waitForPage(ctx, page); err != nil {
		return OnePageFit{}, err
	}
	started := time.Now()
	base = base.Normalized()
	baselinePages := 0
	baseEstimate, err := measureDOMPages(ctx, page)
	if err != nil {
		return OnePageFit{}, err
	}
	if baseEstimate <= 1 {
		baselinePages, err = printPageCount(ctx, page)
		if err != nil {
			return OnePageFit{}, err
		}
		if baselinePages <= 1 {
			return OnePageFit{
				Layout:   base,
				NumPages: baselinePages,
				Duration: time.Since(started),
			}, nil
		}
	}

	var estimated []render.Layout
	for _, candidate := range render.SmartOnePageCandidates(base) {
		if err := applyLayout(ctx, page, candidate); err != nil {
			return OnePageFit{}, err
		}
		pages, err := measureDOMPages(ctx, page)
		if err != nil {
			return OnePageFit{}, err
		}
		if pages <= 1 {
			estimated = append(estimated, candidate)
		}
	}

	for _, candidate := range estimated {
		if err := applyLayout(ctx, page, candidate); err != nil {
			return OnePageFit{}, err
		}
		pages, err := printPageCount(ctx, page)
		if err != nil {
			return OnePageFit{}, err
		}
		if pages <= 1 {
			return OnePageFit{
				Layout:   candidate,
				NumPages: pages,
				Duration: time.Since(started),
			}, nil
		}
	}

	if baselinePages == 0 {
		if err := applyLayout(ctx, page, base); err != nil {
			return OnePageFit{}, err
		}
		baselinePages, err = printPageCount(ctx, page)
		if err != nil {
			return OnePageFit{}, err
		}
	}
	if err := applyLayout(ctx, page, base); err != nil {
		return OnePageFit{}, err
	}
	if baselinePages <= 1 {
		return OnePageFit{
			Layout:   base,
			NumPages: baselinePages,
			Duration: time.Since(started),
		}, nil
	}
	return OnePageFit{
		Layout:     base,
		NumPages:   baselinePages,
		Overflowed: true,
		Notice:     ReadabilityLimitNotice,
		Duration:   time.Since(started),
	}, nil
}

func measureCurrentPage(ctx context.Context, page Page) (int, error) {
	if err := waitForPage(ctx, page); err != nil {
		return 0, err
	}
	return printPageCount(ctx, page)
}

func printPageCount(ctx context.Context, page Page) (int, error) {
	pdf, err := printPDF(ctx, page)
	if err != nil {
		return 0, err
	}
	return countPDFPages(ctx, page, pdf)
}

const fitMeasureScript = `(() => {
  const root = document.querySelector("#sheet-root");
  if (!root) return {pages: 0, error: "missing sheet"};
  const style = getComputedStyle(root);
  const pageHeight = 297 / 25.4 * 96;
  const marginTop = parseFloat(style.paddingTop) || 0;
  const marginBottom = parseFloat(style.paddingBottom) || 0;
  const usable = Math.max(1, pageHeight - marginTop - marginBottom);
  const rootTop = root.getBoundingClientRect().top;
  let contentBottom = marginTop;
  for (const child of Array.from(root.children)) {
    const rect = child.getBoundingClientRect();
    contentBottom = Math.max(contentBottom, rect.bottom - rootTop);
  }
  const flowHeight = Math.max(0, contentBottom - marginTop);
  const pages = Math.max(1, Math.ceil((flowHeight - 0.01) / usable));
  return {
    pages: pages,
    contentHeight: flowHeight,
    usableHeight: usable,
    marker: "tokicv-fit-measure"
  };
})()`

func measureDOMPages(ctx context.Context, page Page) (int, error) {
	value, err := page.EvalContext(ctx, fitMeasureScript)
	if err != nil {
		return 0, fmt.Errorf("测量 DOM 分页: %w", err)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return 0, errors.New("测量 DOM 分页: WebView 返回了无效结果")
	}
	if message, _ := object["error"].(string); message != "" {
		return 0, fmt.Errorf("测量 DOM 分页: %s", message)
	}
	pages, ok := object["pages"].(float64)
	if !ok || pages < 1 {
		return 0, errors.New("测量 DOM 分页: WebView 没有返回页数")
	}
	return int(pages), nil
}

func applyLayout(ctx context.Context, page Page, layout render.Layout) error {
	value, err := page.EvalContext(ctx, render.ApplyLayoutScript(layout))
	if err != nil {
		return fmt.Errorf("更新排版方案: %w", err)
	}
	applied, ok := value.(bool)
	if !ok || !applied {
		return errors.New("更新排版方案: WebView 返回了无效结果")
	}
	return nil
}

func writeAtomic(path string, data []byte) (err error) {
	tempPath, err := stageAtomic(path, data)
	if err != nil {
		return err
	}
	defer func() {
		_ = os.Remove(tempPath)
	}()
	return os.Rename(tempPath, path)
}

func stageAtomic(path string, data []byte) (string, error) {
	return stageAtomicWith(path, func(file *os.File) error {
		_, err := file.Write(data)
		return err
	})
}

func stageAtomicWith(path string, write func(*os.File) error) (string, error) {
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return "", fmt.Errorf("输出路径 %q 是目录", path)
	}
	dir := filepath.Dir(path)
	pattern := "." + filepath.Base(path) + ".tmp-*"
	file, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	tempPath := file.Name()
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(tempPath)
		}
	}()

	if err := write(file); err != nil {
		_ = file.Close()
		return "", err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return "", err
	}
	if err := file.Chmod(0o644); err != nil {
		_ = file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	keep = true
	return tempPath, nil
}

type artifactFile struct {
	path string
	data []byte
}

type stagedArtifact struct {
	path string
	temp string
}

func stageArtifacts(files []artifactFile) ([]stagedArtifact, error) {
	staged := make([]stagedArtifact, 0, len(files))
	for _, file := range files {
		temp, err := stageAtomic(file.path, file.data)
		if err != nil {
			discardArtifacts(staged)
			return nil, err
		}
		staged = append(staged, stagedArtifact{path: file.path, temp: temp})
	}
	return staged, nil
}

func commitArtifacts(staged []stagedArtifact) error {
	for index, artifact := range staged {
		if err := os.Rename(artifact.temp, artifact.path); err != nil {
			discardArtifacts(staged[index:])
			return err
		}
	}
	return nil
}

func discardArtifacts(staged []stagedArtifact) {
	for _, artifact := range staged {
		_ = os.Remove(artifact.temp)
	}
}
