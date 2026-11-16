package desktop

import (
	"context"
	"errors"
	"sync"
	"time"
)

const workCloseGrace = 5 * time.Second

func (a *App) beginWork(timeout time.Duration) (context.Context, func(), error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	a.workMu.Lock()
	if a.closed || a.workSealed {
		a.workMu.Unlock()
		cancel()
		return nil, nil, errors.New("工作区正在关闭")
	}
	a.workNext++
	id := a.workNext
	a.workCancels[id] = cancel
	a.workRunning++
	a.work.Add(1)
	a.workMu.Unlock()

	var once sync.Once
	done := func() {
		once.Do(func() {
			cancel()
			a.workMu.Lock()
			delete(a.workCancels, id)
			a.workRunning--
			a.workMu.Unlock()
			a.work.Done()
		})
	}
	return ctx, done, nil
}

func (a *App) startArtifactWork(run func(context.Context) error, finish func(error)) {
	ctx, done, err := a.beginWork(artifactTimeout)
	if err != nil {
		finish(err)
		return
	}
	go func() {
		defer done()
		err := run(ctx)
		a.post(func() {
			if a.closed {
				return
			}
			finish(err)
		})
	}()
}

func (a *App) cancelWork() {
	a.workMu.Lock()
	a.workSealed = true
	cancels := make([]context.CancelFunc, 0, len(a.workCancels))
	for _, cancel := range a.workCancels {
		cancels = append(cancels, cancel)
	}
	a.workMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func (a *App) hasRunningWork() bool {
	a.workMu.Lock()
	defer a.workMu.Unlock()
	return a.workRunning > 0
}

func (a *App) waitForWork(timeout time.Duration) bool {
	a.workMu.Lock()
	a.workSealed = true
	running := a.workRunning
	a.workMu.Unlock()
	if running == 0 {
		return true
	}
	done := make(chan struct{})
	go func() {
		a.work.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		a.cancelWork()
		return false
	}
}
