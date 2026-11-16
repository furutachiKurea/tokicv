package export

import (
	"fmt"
	"path/filepath"
)

type artifactOutput struct {
	name string
	path string
}

func validateOutputDestinations(opts Options) error {
	outputs := []artifactOutput{
		{name: "--md", path: opts.MarkdownOutput},
		{name: "--pdf", path: opts.Output},
	}
	if err := validateArtifactPaths(outputs); err != nil {
		return err
	}
	if opts.PNGOutput == "" {
		return nil
	}

	resolved, err := resolvePNGDestination(opts.PNGOutput, 1)
	if err != nil {
		return fmt.Errorf("解析 --png 输出路径: %w", err)
	}
	pngDestination, err := normalizeOutputPath(opts.PNGOutput)
	if err != nil {
		return err
	}
	for _, output := range outputs {
		if output.path == "" {
			continue
		}
		candidate, err := normalizeOutputPath(output.path)
		if err != nil {
			return err
		}
		conflict := candidate == pngDestination
		if !conflict && resolved.dir != "" {
			dir, err := normalizeOutputPath(resolved.dir)
			if err != nil {
				return err
			}
			if filepath.Dir(candidate) == dir {
				_, conflict = numberedPNG(filepath.Base(candidate), "page-", ".png")
			}
		}
		if !conflict && resolved.dir == "" && filepath.Dir(candidate) == filepath.Dir(pngDestination) {
			_, conflict = numberedPNG(
				filepath.Base(candidate),
				filepath.Base(resolved.stem)+"-",
				resolved.ext,
			)
		}
		if conflict {
			return fmt.Errorf("输出路径冲突: %s 与 --png 的输出路径 %q 冲突", output.name, candidate)
		}
	}
	return nil
}

func validateArtifactPaths(outputs []artifactOutput) error {
	seen := make(map[string]string, len(outputs))
	for _, output := range outputs {
		if output.path == "" {
			continue
		}
		normalized, err := normalizeOutputPath(output.path)
		if err != nil {
			return err
		}
		if previous, ok := seen[normalized]; ok {
			return fmt.Errorf("输出路径冲突: %s 与 %s 都指向 %q", previous, output.name, normalized)
		}
		seen[normalized] = output.name
	}
	return nil
}

func normalizeOutputPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("解析输出路径 %q: %w", path, err)
	}
	return filepath.Clean(absolute), nil
}
