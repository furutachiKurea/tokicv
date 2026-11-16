//go:build linux

package export

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestExportEnglishLocaleResumeToA4PDF(t *testing.T) {
	requireGraphicalLinux(t)
	binary := testBinary(t)
	temp := t.TempDir()
	input := filepath.Join(temp, "resume.md")
	output := filepath.Join(temp, "resume.pdf")

	const source = `# Sample Candidate

::: left
icon:info Profile/InfoField
icon:github [github.com/example/profile](https://github.com/example/profile)
:::
::: right
icon:email [hello@example.com](mailto:hello@example.com)
:::

## Experience

::: left
**Example Labs**
:::
::: right
**2025**
:::

- Built **A4 PDF** export and DOM preview parity
- Maintained a [project page](https://example.com/project)
`
	if err := os.WriteFile(input, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	env := localeEnvironment(temp, map[string]string{
		"LANG":     "en_US.UTF-8",
		"LC_ALL":   "en_US.UTF-8",
		"LANGUAGE": "en_US:en",
	})
	requireUTF8Locale(t, env, "en_US.UTF-8")
	result := runBinary(t, binary, env, "export", input, "--pdf", output, "--timeout", "25s")
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", result.code, result.stderr)
	}
	pdf, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatalf("output does not start with a PDF header: %q", pdf[:min(len(pdf), 16)])
	}
	mediaBoxes := regexp.MustCompile(`/MediaBox\s*\[\s*0\s+0\s+[\d.]+\s+[\d.]+\s*\]`).FindAll(pdf, -1)
	if len(mediaBoxes) == 0 {
		t.Fatal("PDF has no MediaBox")
	}
	lines := pdfTextLines(pdfTextGlyphs(t, pdf))
	textParts := make([]string, 0, len(lines))
	for _, line := range lines {
		textParts = append(textParts, line.Text)
	}
	text := strings.Join(textParts, "\n")
	for _, want := range []string{"Sample Candidate", "Experience", "A4 PDF", "project"} {
		if !strings.Contains(text, want) {
			t.Errorf("PDF text does not contain %q: %q", want, text)
		}
	}
	if !bytes.Contains(pdf, []byte(`/URI (https://example.com/project)`)) {
		t.Error("PDF link annotation does not contain the expected URI")
	}
	t.Logf("acceptance export language=en-US pages=%d content-match=true", len(mediaBoxes))
}

func requireRequiredChineseLocale(t *testing.T) {
	t.Helper()
	if os.Getenv("TOKICV_VERIFY_REQUIRED") != "1" {
		return
	}
	requireUTF8Locale(t, os.Environ(), "zh_CN.UTF-8")
}

func requireUTF8Locale(t *testing.T, env []string, name string) {
	t.Helper()
	if got := environmentValue(env, "LC_ALL"); got != name {
		t.Fatalf("LC_ALL = %q, want %q", got, name)
	}
	if got := environmentValue(env, "LANG"); got != name {
		t.Fatalf("LANG = %q, want %q", got, name)
	}
	if strings.EqualFold(strings.TrimSpace(environmentValue(env, "LANGUAGE")), "C") {
		t.Fatal("LANGUAGE = C would hide locale failures")
	}
	if _, err := exec.LookPath("locale"); err != nil {
		t.Fatalf("locale tool unavailable: %v", err)
	}

	target, err := localeCategoryData(env)
	if err != nil {
		t.Fatalf("query %s locale categories: %v", name, err)
	}
	cEnv := setEnvironmentValues(env, map[string]string{
		"LC_ALL":   "C",
		"LANG":     "C",
		"LANGUAGE": "C",
	})
	cData, err := localeCategoryData(cEnv)
	if err != nil {
		t.Fatalf("query C locale categories: %v", err)
	}
	if localeCategoriesOnly(target) == localeCategoriesOnly(cData) {
		t.Fatalf("%s locale categories fell back to C", name)
	}
	if !strings.Contains(target, `charmap="UTF-8"`) {
		t.Fatalf("%s locale charmap is not UTF-8", name)
	}
}

func localeCategoriesOnly(data string) string {
	lines := strings.Split(data, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if strings.HasPrefix(line, "charmap=") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func localeCategoryData(env []string) (string, error) {
	args := append([]string{"-k"},
		"LC_CTYPE", "LC_NUMERIC", "LC_TIME", "LC_COLLATE", "LC_MONETARY",
		"LC_MESSAGES", "LC_PAPER", "LC_NAME", "LC_ADDRESS", "LC_TELEPHONE",
		"LC_MEASUREMENT", "LC_IDENTIFICATION",
	)
	cmd := exec.Command("locale", args...)
	cmd.Env = env
	output, err := cmd.Output()
	return string(output), err
}

func environmentValue(env []string, name string) string {
	prefix := name + "="
	value := ""
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			value = strings.TrimPrefix(entry, prefix)
		}
	}
	return value
}

func setEnvironmentValues(env []string, values map[string]string) []string {
	out := make([]string, 0, len(env)+len(values))
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if _, replace := values[name]; replace {
			continue
		}
		out = append(out, entry)
	}
	for name, value := range values {
		out = append(out, name+"="+value)
	}
	return out
}

func localeEnvironment(temp string, values map[string]string) []string {
	env := isolatedEnv(temp)
	for key, value := range values {
		prefix := key + "="
		found := false
		for index, entry := range env {
			if strings.HasPrefix(entry, prefix) {
				env[index] = prefix + value
				found = true
				break
			}
		}
		if !found {
			env = append(env, prefix+value)
		}
	}
	return env
}
