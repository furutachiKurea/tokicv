package main

import (
	"os"

	"github.com/egoist/mygo"

	"tokicv/internal/cli"
	"tokicv/internal/desktop"
	"tokicv/internal/export"
	"tokicv/internal/render"
)

func main() {
	mygo.App.SetName("tokicv")
	mygo.App.SetVersion(cli.Version)

	os.Exit(cli.RunWithOptions(os.Args[1:], cli.RunOptions{
		Stdin:           os.Stdin,
		StdinRedirected: stdinRedirected(),
		Stdout:          os.Stdout,
		Stderr:          os.Stderr,
		Export: func(request cli.ExportRequest) error {
			layout := render.DefaultLayout()
			layout.FontStack = request.Font
			return export.Run(export.Options{
				Input:          request.Input,
				Output:         request.Output,
				PNGOutput:      request.PNG,
				MarkdownOutput: request.Markdown,
				Timeout:        request.Timeout,
				Layout:         layout,
				FitOnePage:     request.FitOnePage,
				Notice:         os.Stderr,
			})
		},
		Check: func(request cli.CheckRequest) (export.CheckResult, error) {
			layout := render.DefaultLayout()
			layout.FontStack = request.Font
			return export.Check(export.CheckOptions{
				Input:      request.Input,
				Timeout:    request.Timeout,
				Layout:     layout,
				FitOnePage: request.FitOnePage,
			})
		},
		GUI: func(path string) error {
			return desktop.Run(desktop.Options{Path: path})
		},
	}))
}

func stdinRedirected() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice == 0
}
