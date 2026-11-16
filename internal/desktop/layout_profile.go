package desktop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"tokicv/internal/library"
	"tokicv/internal/render"
)

const smartReadabilityNotice = "无法压缩成一页"

const (
	smartOnePageBeginWorkStage = "begin work"
	smartOnePageFitStage       = "fit one page"

	smartOnePageBusyMessage    = "智能一页失败（工作区繁忙）"
	smartOnePageTimeoutMessage = "智能一页失败（超时）"
	smartOnePagePrintMessage   = "智能一页处理失败"
	smartOnePageFailureMessage = "智能一页失败"
)

func smartOnePageFailure(stage string, err error) string {
	logDesktopError("smart-onepage-"+stage, err)
	if stage == smartOnePageBeginWorkStage {
		return smartOnePageBusyMessage
	}
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "超时") {
		return smartOnePageTimeoutMessage
	}
	if strings.Contains(err.Error(), "打印") {
		return smartOnePagePrintMessage
	}
	return smartOnePageFailureMessage
}

func (a *App) layoutTarget() (library.Target, bool) {
	if a.path != "" {
		return library.Target{Kind: library.TargetFile, Path: a.path}, true
	}
	if a.currentID != "" {
		return library.Target{
			Kind:     library.TargetLibrary,
			Document: library.Document{ID: a.currentID, Name: a.currentName},
		}, true
	}
	return library.Target{}, false
}

func (a *App) loadEffectiveLayout() {
	if a.layoutStore == nil && a.store != nil {
		a.layoutStore = a.store
	}
	layout := a.fallbackLayout.Normalized()
	origin := library.LayoutOriginDefault
	if a.layoutStore != nil {
		if target, ok := a.layoutTarget(); ok {
			resolved, err := a.layoutStore.ResolveLayoutProfile(target)
			if err != nil {
				a.setFileError("读取排版失败")
			} else {
				layout = resolved.Layout
				origin = resolved.Origin
			}
		}
	}
	a.baseLayout = layout
	a.layout = layout
	a.layoutOrigin = origin
	a.layoutRevision++
	a.stopSmartOnePage()
	a.layoutNotice = ""
	a.artifactError = ""
}

func (a *App) changeBaseLayout(next render.Layout) {
	if err := next.Validate(); err != nil {
		a.setFileError("排版无效")
		return
	}
	a.stopSmartOnePage()
	a.baseLayout = next.Normalized()
	a.layout = a.baseLayout
	a.layoutRevision++
	a.layoutNotice = ""
	a.previewNow()
}

func (a *App) stopSmartOnePage() {
	a.smartRequest++
	if a.smartCancel != nil {
		a.smartCancel()
		a.smartCancel = nil
	}
	a.smartSolving = false
	a.smartOnePage = false
	a.smartFitVersion = 0
	a.smartRefitPending = false
	a.smartRefitDue = time.Time{}
	a.smartSolveDuration = 0
	a.layout = a.baseLayout
}

func (a *App) disableSmartOnePage() {
	a.stopSmartOnePage()
	a.layoutNotice = ""
	a.previewNow()
}

func (a *App) setSmartOnePage(enabled bool) {
	if !enabled {
		a.disableSmartOnePage()
		return
	}
	if a.smartOnePage || a.smartSolving || a.artifacts == nil || !a.hasDocument() {
		return
	}
	a.smartOnePage = true
	a.layoutNotice = ""
	a.setFileError("")
	a.startSmartOnePageSolve()
}

func (a *App) startSmartOnePageSolve() {
	if a.smartSolving || a.artifacts == nil || !a.hasDocument() {
		return
	}
	a.ensureAnalysis()
	html := render.RenderWithLayout(a.analysis.Document, a.baseLayout)
	workCtx, done, err := a.beginWork(artifactTimeout)
	if err != nil {
		a.smartOnePage = false
		a.setFileError(smartOnePageFailure(smartOnePageBeginWorkStage, err))
		a.previewNow()
		return
	}
	ctx, cancel := context.WithCancel(workCtx)
	a.smartRequest++
	request := a.smartRequest
	document := a.document
	revision := a.layoutRevision
	sourceVersion := a.buffer.Version()
	base := a.baseLayout
	a.smartCancel = cancel
	a.smartOnePage = true
	a.smartSolving = true
	a.smartRefitPending = false
	a.layoutNotice = ""
	a.setFileError("")
	a.invalidate()

	go func() {
		defer done()
		fit, fitErr := a.artifacts.FitOnePage(ctx, html, base)
		a.post(func() {
			if a.closed || request != a.smartRequest ||
				document != a.document || revision != a.layoutRevision {
				return
			}
			a.smartSolving = false
			a.smartCancel = nil
			a.smartSolveDuration = fit.Duration
			if sourceVersion != a.buffer.Version() {
				a.smartOnePage = true
				a.layout = a.baseLayout
				a.smartRefitPending = true
				a.smartRefitDue = time.Time{}
				a.previewNow()
				return
			}
			if fitErr != nil {
				a.smartOnePage = false
				a.smartRefitPending = false
				a.layout = a.baseLayout
				a.setFileError(smartOnePageFailure(smartOnePageFitStage, fitErr))
				a.previewNow()
				return
			}
			if fit.Overflowed || fit.NumPages > 1 {
				a.smartOnePage = false
				a.smartRefitPending = false
				a.layout = a.baseLayout
				a.layoutNotice = smartReadabilityNotice
			} else {
				a.smartOnePage = true
				a.smartFitVersion = sourceVersion
				a.smartRefitPending = false
				a.layout = fit.Layout.Normalized()
				a.layoutNotice = ""
				a.pageCount = 1
			}
			a.previewNow()
		})
	}()
}

func (a *App) previewNow() {
	a.previewPending = true
	a.previewDue = time.Time{}
	a.invalidate()
}

func (a *App) saveGlobalLayout() {
	store, err := a.writableLayoutStore()
	if err != nil {
		a.setFileError("保存失败")
		return
	}
	layout := a.layout.Normalized()
	document := a.document
	a.fileIO.Add(1)
	go func() {
		defer a.fileIO.Done()
		err := store.SetGlobalDefault(layout)
		a.post(func() {
			if a.closed || document != a.document || layout != a.layout {
				return
			}
			if err != nil {
				a.setFileError("保存失败")
				return
			}
			a.baseLayout = layout
			a.stopSmartOnePage()
			a.layoutRevision++
			a.previewNow()
			if a.layoutOrigin == library.LayoutOriginDefault {
				a.layoutOrigin = library.LayoutOriginGlobal
			}
			a.setFileError("")
		})
	}()
}

func (a *App) saveTargetLayout() {
	target, ok := a.layoutTarget()
	if !ok {
		return
	}
	store, err := a.writableLayoutStore()
	if err != nil {
		a.setFileError("保存失败")
		return
	}
	layout := a.layout.Normalized()
	origin := library.LayoutOriginDocument
	document := a.document
	if target.Kind == library.TargetFile {
		origin = library.LayoutOriginPath
	}
	a.fileIO.Add(1)
	go func() {
		defer a.fileIO.Done()
		err := store.SetTargetProfile(target, layout)
		a.post(func() {
			if a.closed || document != a.document || layout != a.layout {
				return
			}
			if err != nil {
				a.setFileError("保存失败")
				return
			}
			a.baseLayout = layout
			a.stopSmartOnePage()
			a.layoutRevision++
			a.previewNow()
			a.layoutOrigin = origin
			a.setFileError("")
		})
	}()
}

func (a *App) clearTargetLayout() {
	target, ok := a.layoutTarget()
	if !ok || a.layoutOrigin != library.LayoutOriginDocument &&
		a.layoutOrigin != library.LayoutOriginPath {
		return
	}
	store := a.layoutStore
	if store == nil {
		return
	}
	document := a.document
	revision := a.layoutRevision
	a.fileIO.Add(1)
	go func() {
		defer a.fileIO.Done()
		if err := store.ResetTargetProfile(target); err != nil {
			a.post(func() {
				if a.closed {
					return
				}
				a.setFileError("保存失败")
			})
			return
		}
		resolved, err := store.ResolveLayoutProfile(target)
		a.post(func() {
			if a.closed || document != a.document || revision != a.layoutRevision {
				return
			}
			if err != nil {
				a.setFileError("读取排版失败")
				return
			}
			a.baseLayout = resolved.Layout
			a.layout = resolved.Layout
			a.layoutOrigin = resolved.Origin
			a.layoutRevision++
			a.stopSmartOnePage()
			a.layoutNotice = ""
			a.previewNow()
			a.setFileError("")
		})
	}()
}

func (a *App) writableLayoutStore() (*library.Store, error) {
	if a.layoutStore != nil {
		return a.layoutStore, nil
	}
	store, err := library.Open()
	if err != nil {
		return nil, fmt.Errorf("打开排版方案存储: %w", err)
	}
	a.layoutStore = store
	return store, nil
}

// Keep interface compatibility checked at compile time.
var _ ArtifactPipeline = desktopArtifactPipeline{}
