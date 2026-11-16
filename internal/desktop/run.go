package desktop

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"tokicv/internal/library"
	"tokicv/internal/render"
)

type webPreview struct {
	web *mygo.WebView
}

func (p *webPreview) Show(frame render.PreviewFrame) {
	p.web.Page().LoadHTML(frame.HTML, "")
}

func (p *webPreview) Update(frame render.PreviewFrame) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	value, err := p.web.Page().EvalContext(ctx, render.PreviewUpdateScript(frame))
	if err == nil {
		if updated, ok := value.(bool); ok && updated {
			return
		}
	}
	p.Show(frame)
}

func (p *webPreview) View(c *ui.Context) ui.Element {
	return ui.WebView(c.Key("preview-sheet"), p.web).Grow(1)
}

func configureLinuxPortalFileChooser(
	goos string,
	lookup func(string) (string, bool),
	setenv func(string, string) error,
) {
	if goos != "linux" {
		return
	}
	if _, ok := lookup("GTK_USE_PORTAL"); !ok {
		_ = setenv("GTK_USE_PORTAL", "1")
	}
}

// Run starts the desktop authoring workspace.
func Run(opts Options) error {
	configureLinuxPortalFileChooser(runtime.GOOS, os.LookupEnv, os.Setenv)
	mygo.App.SetName("tokicv")
	mygo.App.SetLocale("zh-Hans")
	closeDiagnostic, diagnosticErr := installDiagnosticLog()
	if diagnosticErr != nil {
		fmt.Fprintf(os.Stderr, "tokicv: 诊断日志不可用: %v\n", diagnosticErr)
	}
	if closeDiagnostic != nil {
		defer closeDiagnostic()
	}
	if opts.Store == nil && opts.Path == "" {
		store, err := library.Open()
		if err != nil {
			return err
		}
		opts.Store = store
	}
	if opts.LayoutStore == nil {
		if opts.Store != nil {
			opts.LayoutStore = opts.Store
		} else if opts.Path != "" {
			if store, err := library.OpenLayoutReadOnly(); err == nil {
				opts.LayoutStore = store
			}
		}
	}
	app := New(opts)
	app.SetArtifactPipeline(newDesktopArtifactPipeline())
	if opts.Path != "" {
		path, err := markdownPath(opts.Path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("读取 Resume Document %q: %w", path, err)
		}
		app.loadDocument(path, string(data))
	}
	defer app.CloseAndWait()

	startup := make(chan error, 1)
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:           "tokicv",
			Width:           1280,
			Height:          800,
			UseContentSize:  true,
			MinWidth:        960,
			MinHeight:       640,
			BackgroundColor: "#EDECE8",
			Content:         ui.View(app.View),
		})
		app.SetUpdate(win.Update)
		allowClose := false
		closing := false
		win.OnClose(func(event *mygo.CloseEvent) {
			if allowClose || !app.hasRunningWork() {
				return
			}
			event.PreventDefault()
			if closing {
				return
			}
			closing = true
			go func() {
				app.waitForWork(workCloseGrace)
				win.Update(func() {
					allowClose = true
					win.Close()
				})
			}()
		})
		win.OnClosed(func() {
			app.close()
		})
		app.SetInvalidate(win.Invalidate)
		app.SetTitleHandler(win.SetTitle)
		app.SetPicker(func() (string, error) {
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
				Parent:      win,
				Title:       "打开 Markdown 文件",
				ButtonLabel: "打开",
				Filters: []mygo.FileFilter{{
					Name:       "Markdown",
					Extensions: []string{"md"},
				}},
			})
			if err != nil {
				return "", err
			}
			if len(paths) == 0 {
				return "", nil
			}
			return paths[0], nil
		})
		app.SetExportPath(ArtifactPDF, func(defaultName string) (string, error) {
			return mygo.Dialog.Save(mygo.SaveDialogOptions{
				Parent:      win,
				Title:       "导出 PDF",
				DefaultPath: defaultName,
				ButtonLabel: "导出",
				Filters: []mygo.FileFilter{{
					Name:       "PDF",
					Extensions: []string{"pdf"},
				}},
			})
		})
		app.SetExportPath(ArtifactPNG, func(defaultName string) (string, error) {
			return mygo.Dialog.Save(mygo.SaveDialogOptions{
				Parent:      win,
				Title:       "导出 PNG",
				DefaultPath: defaultName,
				ButtonLabel: "导出",
				Filters: []mygo.FileFilter{{
					Name:       "PNG",
					Extensions: []string{"png"},
				}},
			})
		})
		app.SetExportPath(ArtifactMarkdown, func(defaultName string) (string, error) {
			return mygo.Dialog.Save(mygo.SaveDialogOptions{
				Parent:      win,
				Title:       "导出 Markdown",
				DefaultPath: defaultName,
				ButtonLabel: "导出",
				Filters: []mygo.FileFilter{{
					Name:       "Markdown",
					Extensions: []string{"md"},
				}},
			})
		})

		web, err := win.NewWebView(mygo.WebViewOptions{
			Page: mygo.PageOptions{
				BackgroundThrottling: mygo.ThrottlingOff,
				Untrusted:            true,
			},
			BackgroundColor: "#E0DFDB",
		})
		if err != nil {
			startup <- fmt.Errorf("创建 A4 预览: %w", err)
			win.Close()
			mygo.App.Quit()
			return
		}
		web.Page().OnPageTitleUpdated(func(event *mygo.TitleEvent) {
			if count, ok := render.PreviewPageCount(event.Title); ok {
				event.PreventDefault()
				win.Update(func() { app.SetPageCount(count) })
			}
		})
		app.SetPreview(&webPreview{web: web})
		win.Invalidate()
	})

	if err := mygo.App.Run(); err != nil {
		return fmt.Errorf("启动 tokicv: %w", err)
	}
	select {
	case err := <-startup:
		return err
	default:
		return nil
	}
}
