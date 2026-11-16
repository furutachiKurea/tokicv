package export

import (
	"fmt"

	"tokicv/internal/library"
	"tokicv/internal/render"
)

// WriteMarkdown writes the exact unmodified Resume Document Markdown source
// bytes to output.
func WriteMarkdown(store *library.Store, target, output string) error {
	resolved, err := resolveTarget(store, target)
	if err != nil {
		return fmt.Errorf("读取 Resume Document %q: %w", target, err)
	}
	return writeTargetMarkdown(resolved, output)
}

func writeTargetMarkdown(target library.Target, output string) error {
	source, err := target.Read()
	if err != nil {
		return err
	}
	if err := writeAtomic(output, source); err != nil {
		return fmt.Errorf("写入 Markdown %q: %w", output, err)
	}
	return nil
}

func resolveTarget(store *library.Store, input string) (library.Target, error) {
	if input == "" {
		return library.Target{}, fmt.Errorf("%w: 目标为空", library.ErrInvalidTarget)
	}
	if target, file, err := library.ResolveFileTarget(input); err != nil {
		return library.Target{}, err
	} else if file {
		return target, nil
	}
	if store == nil {
		opened, err := library.Open()
		if err != nil {
			return library.Target{}, err
		}
		store = opened
	}
	return library.ResolveTarget(store, input)
}

func effectiveLayout(store *library.Store, target library.Target, explicit render.Layout, layoutSet bool) (render.Layout, error) {
	if layoutSet || explicit.Normalized() != render.DefaultLayout().Normalized() {
		if err := explicit.Validate(); err != nil {
			return render.Layout{}, fmt.Errorf("排版方案无效: %w", err)
		}
		return explicit.Normalized(), nil
	}
	if store == nil {
		return library.ResolveLayout(target, render.LayoutPatch{})
	}
	return store.ResolveLayout(target, render.LayoutPatch{})
}
