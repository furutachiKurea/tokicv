package export

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var printLocaleMu sync.Mutex

func withPrintLocale(
	ctx context.Context,
	goos string,
	lookupEnv func(string) (string, bool),
	setenv func(string, string) error,
	unsetenv func(string) error,
	print func() ([]byte, error),
) (pdf []byte, err error) {
	if goos != "linux" {
		return print()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	printLocaleMu.Lock()
	defer printLocaleMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	previous, wasSet := lookupEnv("LANGUAGE")
	if err := setenv("LANGUAGE", "C"); err != nil {
		return nil, fmt.Errorf("设置 PDF 打印语言环境: %w", err)
	}
	defer func() {
		var restoreErr error
		if wasSet {
			restoreErr = setenv("LANGUAGE", previous)
		} else {
			restoreErr = unsetenv("LANGUAGE")
		}
		if restoreErr == nil {
			return
		}
		restoreErr = fmt.Errorf("恢复 PDF 打印语言环境: %w", restoreErr)
		if err != nil {
			err = errors.Join(err, restoreErr)
			return
		}
		pdf, err = nil, restoreErr
	}()

	return print()
}
