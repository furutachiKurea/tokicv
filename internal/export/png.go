package export

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"tokicv/internal/render"
)

// ResolvePNGPaths returns the per-Sheet PNG Artifact paths for a destination.
func ResolvePNGPaths(dest string, numPages int) ([]string, error) {
	resolved, err := resolvePNGDestination(dest, numPages)
	if err != nil {
		return nil, err
	}
	return resolved.paths, nil
}

type pngDestination struct {
	paths []string
	dir   string
	stem  string
	ext   string
}

func resolvePNGDestination(dest string, numPages int) (pngDestination, error) {
	if numPages < 1 {
		return pngDestination{}, fmt.Errorf("PNG 导出的页面数必须大于零")
	}
	if dest == "" {
		return pngDestination{}, fmt.Errorf("PNG 输出路径为空")
	}

	info, statErr := os.Stat(dest)
	if statErr == nil && info.IsDir() {
		return pngDirectory(dest, numPages), nil
	}
	if strings.HasSuffix(dest, "/") || strings.HasSuffix(dest, `\`) ||
		!strings.EqualFold(filepath.Ext(dest), ".png") {
		if statErr == nil {
			return pngDestination{}, fmt.Errorf("PNG 输出路径 %q 不是目录", dest)
		}
		return pngDirectory(dest, numPages), nil
	}

	ext := filepath.Ext(dest)
	stem := strings.TrimSuffix(dest, ext)
	if numPages == 1 {
		return pngDestination{paths: []string{dest}, stem: stem, ext: ext}, nil
	}
	paths := make([]string, numPages)
	for index := range paths {
		paths[index] = fmt.Sprintf("%s-%d%s", stem, index+1, ext)
	}
	return pngDestination{paths: paths, stem: stem, ext: ext}, nil
}

func pngDirectory(dest string, numPages int) pngDestination {
	dir := filepath.Clean(dest)
	paths := make([]string, numPages)
	for index := range paths {
		paths[index] = filepath.Join(dir, fmt.Sprintf("page-%d.png", index+1))
	}
	return pngDestination{paths: paths, dir: dir}
}

func rasterizePDF(ctx context.Context, page Page, pdf []byte) ([][]byte, int, error) {
	numPages, err := countPDFPages(ctx, page, pdf)
	if err != nil {
		return nil, 0, err
	}
	pages := make([][]byte, numPages)
	for index := range pages {
		value, err := page.EvalContext(ctx, render.PDFJSRenderPageScript(index))
		if err != nil {
			return nil, 0, fmt.Errorf("栅格化第 %d 个页面: %w", index+1, err)
		}
		encoded, ok := value.(string)
		if !ok {
			return nil, 0, fmt.Errorf("栅格化第 %d 个页面: WebView 返回了无效结果", index+1)
		}
		png, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, 0, fmt.Errorf("解码第 %d 个页面 PNG: %w", index+1, err)
		}
		if !bytes.HasPrefix(png, []byte("\x89PNG\r\n\x1a\n")) {
			return nil, 0, fmt.Errorf("第 %d 个页面不是有效的 PNG", index+1)
		}
		pages[index] = png
	}
	return pages, numPages, nil
}

func countPDFPages(ctx context.Context, page Page, pdf []byte) (int, error) {
	value, err := page.EvalContext(ctx, render.PDFJSPrepareScript(pdf))
	if err != nil {
		return 0, fmt.Errorf("加载 pdfjs-dist: %w", err)
	}
	number, ok := value.(float64)
	if !ok || number < 1 {
		return 0, fmt.Errorf("pdfjs-dist 返回了无效页面数: %v", value)
	}
	return int(number), nil
}

func writePNGArtifacts(dest string, pages [][]byte) error {
	staged, stale, err := stagePNGArtifacts(dest, pages)
	if err != nil {
		return err
	}
	if err := commitArtifacts(staged); err != nil {
		return err
	}
	return removeStalePNGPages(stale)
}

func stagePNGArtifacts(dest string, pages [][]byte) ([]stagedArtifact, []string, error) {
	if len(pages) == 0 {
		return nil, nil, fmt.Errorf("没有可写入的 PNG Artifact")
	}
	resolved, err := resolvePNGDestination(dest, len(pages))
	if err != nil {
		return nil, nil, err
	}
	for _, path := range resolved.paths {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return nil, nil, fmt.Errorf("PNG 输出路径 %q 是目录", path)
		}
	}
	if resolved.dir != "" {
		if err := os.MkdirAll(resolved.dir, 0o755); err != nil {
			return nil, nil, err
		}
	}

	files := make([]artifactFile, 0, len(resolved.paths))
	for index, path := range resolved.paths {
		files = append(files, artifactFile{path: path, data: pages[index]})
	}
	staged, err := stageArtifacts(files)
	if err != nil {
		return nil, nil, err
	}
	stale, err := stalePNGPaths(resolved)
	if err != nil {
		discardArtifacts(staged)
		return nil, nil, err
	}
	return staged, stale, nil
}

func stalePNGPaths(resolved pngDestination) ([]string, error) {
	var stale []string
	if resolved.dir != "" {
		entries, err := os.ReadDir(resolved.dir)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			number, ok := numberedPNG(entry.Name(), "page-", ".png")
			if ok && number > len(resolved.paths) {
				stale = append(stale, filepath.Join(resolved.dir, entry.Name()))
			}
		}
		return stale, nil
	}

	dir := filepath.Dir(resolved.stem)
	base := filepath.Base(resolved.stem)
	ext := resolved.ext
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == base+ext {
			if len(resolved.paths) > 1 {
				stale = append(stale, filepath.Join(dir, name))
			}
			continue
		}
		number, ok := numberedPNG(name, base+"-", ext)
		if ok && (len(resolved.paths) == 1 || number > len(resolved.paths)) {
			stale = append(stale, filepath.Join(dir, name))
		}
	}
	return stale, nil
}

func numberedPNG(name, prefix, ext string) (int, bool) {
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ext) {
		return 0, false
	}
	digits := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ext)
	number, err := strconv.Atoi(digits)
	if err != nil || number < 1 || fmt.Sprintf("%s%d%s", prefix, number, ext) != name {
		return 0, false
	}
	return number, true
}

func removeStalePNGPages(paths []string) error {
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
