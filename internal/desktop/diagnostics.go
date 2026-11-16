package desktop

import (
	"fmt"
	"log"
	"os"
	"runtime"

	"github.com/egoist/mygo"

	"tokicv/internal/diagnostic"
)

func installDiagnosticLog() (func(), error) {
	dir, err := mygo.App.Path(mygo.PathLogs)
	if err != nil {
		return nil, err
	}
	logger, err := diagnostic.Open(dir, diagnostic.DefaultOptions())
	if err != nil {
		return nil, err
	}
	if err := logger.Install(); err != nil {
		_ = logger.Close()
		return nil, err
	}
	log.Printf("[tokicv] stage=startup build=%s goos=%s goarch=%s",
		runtime.Version(), runtime.GOOS, runtime.GOARCH)
	return func() {
		if err := logger.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "tokicv: 关闭诊断日志: %v\n", err)
		}
	}, nil
}

func logDesktopError(stage string, err error) {
	if err == nil {
		return
	}
	log.Printf("[tokicv] stage=%s error_type=%T error=%s", stage, err, diagnostic.SafeError(err))
}
