//go:build linux

package scripts

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

var (
	failuresPattern  = regexp.MustCompile(`failures=(\d+)`)
	statusPattern    = regexp.MustCompile(`function_status=(\d+)\nscript_status=(\d+)`)
	localeCategories = []string{
		"LC_CTYPE", "LC_NUMERIC", "LC_TIME", "LC_COLLATE", "LC_MONETARY",
		"LC_MESSAGES", "LC_PAPER", "LC_NAME", "LC_ADDRESS", "LC_TELEPHONE",
		"LC_MEASUREMENT", "LC_IDENTIFICATION",
	}
)

func TestRunFmtCheckPropagatesStatusAndOutput(t *testing.T) {
	script := verifyScriptPath(t)
	for _, tc := range []struct {
		name          string
		mode          string
		withGoFile    bool
		findFails     bool
		wantFailures  int
		wantStatus    int
		wantLog       string
		wantOutput    string
		wantGofmtCall bool
	}{
		{name: "clean files including spaces", mode: "pass", withGoFile: true, wantStatus: 0, wantFailures: 0, wantLog: "clean", wantGofmtCall: true},
		{name: "empty file list", mode: "pass", wantStatus: 1, wantFailures: 1, wantLog: "no Go files", wantOutput: "gofmt found no Go files"},
		{name: "gofmt exit one", mode: "fail", withGoFile: true, wantStatus: 1, wantFailures: 1, wantLog: "synthetic gofmt failure", wantOutput: "gofmt failed (exit 1)", wantGofmtCall: true},
		{name: "gofmt output with success status", mode: "list", withGoFile: true, wantStatus: 1, wantFailures: 1, wantLog: "简历.go", wantOutput: "gofmt found unformatted files", wantGofmtCall: true},
		{name: "find failure", mode: "pass", findFails: true, wantStatus: 1, wantFailures: 1, wantLog: "file discovery failed", wantOutput: "gofmt file discovery failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.withGoFile {
				for _, name := range []string{"space name.go", filepath.Join("中文 目录", "简历.go")} {
					path := filepath.Join(root, name)
					if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte("package sample\n"), 0o644); err != nil {
						t.Fatal(err)
					}
				}
			}
			work := t.TempDir()
			bin := t.TempDir()
			calls := filepath.Join(work, "gofmt.calls")
			writeExecutable(t, filepath.Join(bin, "gofmt"), `#!/usr/bin/env bash
printf '%s\n' "$@" >>"$FAKE_GOFMT_CALLS"
case "$FAKE_GOFMT_MODE" in
fail)
	printf 'synthetic gofmt failure\n' >&2
	exit 1
	;;
list)
	shift
	printf '%s\n' "$@"
	exit 0
	;;
esac
exit 0
`)
			if tc.findFails {
				writeExecutable(t, filepath.Join(bin, "find"), "#!/usr/bin/env bash\nexit 9\n")
			}
			result := runVerifySnippet(t, script, []string{
				"PATH=" + bin + ":" + os.Getenv("PATH"),
				"FAKE_GOFMT_CALLS=" + calls,
				"FAKE_GOFMT_MODE=" + tc.mode,
				"TEST_VERIFY_ROOT=" + root,
			}, `
source "$TEST_VERIFY_SCRIPT"
ROOT="$TEST_VERIFY_ROOT"
WORK="$TEST_VERIFY_WORK"
ACTIVE_LOG="$WORK/fmt.log"
FAILURES=0
REPORT_NOTES=()
run_fmt_check
function_status=$?
MODE=fast
LC_ALL=${LC_ALL:-C}
LOG="$WORK/verify.log"
FAST_LOG="$WORK/fast.log"
GRAPHICS_LOG="$WORK/graphics.log"
REPORT="$WORK/report.md"
: >"$FAST_LOG"
: >"$GRAPHICS_LOG"
write_report >/dev/null
script_status=$?
printf '\nfunction_status=%d\nscript_status=%d\n' "$function_status" "$script_status"
printf '\nfailures=%d\n' "$FAILURES"
exit "$script_status"
`)
			if got := parseFailures(t, result.output); got != tc.wantFailures {
				t.Fatalf("failures = %d, want %d; output:\n%s", got, tc.wantFailures, result.output)
			}
			functionStatus, scriptStatus := parseStatuses(t, result.output)
			if functionStatus != tc.wantStatus {
				t.Errorf("run_fmt_check status = %d, want %d; output:\n%s", functionStatus, tc.wantStatus, result.output)
			}
			if scriptStatus != tc.wantStatus {
				t.Errorf("script status = %d, want %d; output:\n%s", scriptStatus, tc.wantStatus, result.output)
			}
			if (result.err == nil) != (tc.wantStatus == 0) {
				t.Errorf("shell status error = %v, want success=%t; output:\n%s", result.err, tc.wantStatus == 0, result.output)
			}
			logData := readFileOrEmpty(filepath.Join(result.work, "fmt.log"))
			if !strings.Contains(logData, tc.wantLog) {
				t.Fatalf("fmt log = %q, want %q", logData, tc.wantLog)
			}
			if tc.wantOutput != "" && !strings.Contains(result.output, tc.wantOutput) {
				t.Fatalf("output = %q, want %q", result.output, tc.wantOutput)
			}
			callData := readFileOrEmpty(calls)
			if tc.wantGofmtCall {
				for _, fragment := range []string{"-l", "space name.go", "简历.go"} {
					if !strings.Contains(callData, fragment) {
						t.Errorf("gofmt calls = %q, want %q", callData, fragment)
					}
				}
			} else if callData != "" {
				t.Errorf("gofmt was called for an empty file list: %q", callData)
			}
		})
	}
}

func TestCheckLocaleRejectsExitAndSilentFallback(t *testing.T) {
	script := verifyScriptPath(t)
	localePath, err := exec.LookPath("locale")
	if err != nil {
		t.Skip("locale command is required")
	}

	t.Run("command failure", func(t *testing.T) {
		bin := t.TempDir()
		writeExecutable(t, filepath.Join(bin, "locale"), "#!/usr/bin/env bash\nexit 7\n")
		result := runLocaleCheck(t, script, "en_US.UTF-8", "PATH="+bin+":"+os.Getenv("PATH"))
		if got := parseFailures(t, result.output); got != 1 {
			t.Fatalf("failures = %d, want 1; output:\n%s", got, result.output)
		}
		if !strings.Contains(result.output, "required locale query failed: en_US.UTF-8 (exit 7)") {
			t.Fatalf("missing command status in output:\n%s", result.output)
		}
	})

	t.Run("zero exit fallback", func(t *testing.T) {
		bin := t.TempDir()
		writeExecutable(t, filepath.Join(bin, "locale"), `#!/usr/bin/env bash
for arg in "$@"; do
	case "$arg" in
	charmap)
		printf 'UTF-8\n'
		exit 0
		;;
	-k)
		LC_ALL=C LANG=C LANGUAGE= "$FAKE_REAL_LOCALE" "$@" | sed 's/^charmap=.*/charmap="UTF-8"/'
		exit 0
		;;
	esac
done
exec "$FAKE_REAL_LOCALE" "$@"
`)
		result := runLocaleCheck(t, script, "en_US.UTF-8",
			"PATH="+bin+":"+os.Getenv("PATH"),
			"FAKE_REAL_LOCALE="+localePath)
		if got := parseFailures(t, result.output); got != 1 {
			t.Fatalf("failures = %d, want 1; output:\n%s", got, result.output)
		}
		if !strings.Contains(result.output, "required locale fell back to C") {
			t.Fatalf("missing category fallback failure:\n%s", result.output)
		}
	})

	t.Run("real missing locale data", func(t *testing.T) {
		empty := t.TempDir()
		result := runLocaleCheck(t, script, "zz_ZZ.UTF-8", "LOCPATH="+empty)
		if got := parseFailures(t, result.output); got != 1 {
			t.Fatalf("failures = %d, want 1; output:\n%s", got, result.output)
		}
		probe := exec.Command(localePath, "charmap")
		probe.Env = append(os.Environ(), "LOCPATH="+empty, "LC_ALL=zz_ZZ.UTF-8", "LANG=zz_ZZ.UTF-8", "LANGUAGE=")
		charmap, err := probe.Output()
		if err != nil {
			t.Fatalf("locale charmap did not reproduce the zero-exit fallback: %v", err)
		}
		if strings.TrimSpace(string(charmap)) == "UTF-8" {
			t.Fatalf("locale charmap unexpectedly returned UTF-8 for missing locale data: %q", charmap)
		}
	})
}

func TestCheckLocaleAcceptsInstalledLocales(t *testing.T) {
	script := verifyScriptPath(t)
	for _, name := range []string{"zh_CN.UTF-8", "en_US.UTF-8"} {
		if !localeHasCategoryData(t, name) {
			t.Logf("%s locale data is unavailable; the strict verify preflight still requires it", name)
			continue
		}
		result := runLocaleCheck(t, script, name)
		if got := parseFailures(t, result.output); got != 0 {
			t.Fatalf("%s failures = %d, want 0; output:\n%s", name, got, result.output)
		}
	}
}

func TestVerificationDocsAreReachableWithoutIgnoredScratch(t *testing.T) {
	root := filepath.Dir(filepath.Dir(verifyScriptPath(t)))
	cmd := exec.Command("git", "-C", root, "ls-files", "--cached", "--others", "--exclude-standard")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("list checkout files: %v", err)
	}
	checkout := make(map[string]bool)
	for _, path := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		checkout[filepath.ToSlash(path)] = true
	}
	for _, path := range []string{
		"docs/verification.md",
		"AGENTS.md",
		".agents/skills/mygo-maintenance/SKILL.md",
	} {
		if !checkout[path] {
			t.Errorf("%s is absent from the non-ignored checkout file set", path)
		}
	}
	for _, path := range []string{"AGENTS.md", ".agents/skills/mygo-maintenance/SKILL.md"} {
		data := readFileOrEmpty(filepath.Join(root, path))
		if !strings.Contains(data, "docs/verification.md") {
			t.Errorf("%s does not point to docs/verification.md", path)
		}
		if strings.Contains(data, ".scratch/") {
			t.Errorf("%s points into ignored scratch state", path)
		}
	}
}

func localeHasCategoryData(t *testing.T, name string) bool {
	t.Helper()
	output, err := commandOutput("locale", name, localeCategories)
	if err != nil {
		return false
	}
	cOutput, err := commandOutput("locale", "C", localeCategories)
	return err == nil && output != cOutput
}

func commandOutput(command, locale string, categories []string) (string, error) {
	args := append([]string{"-k"}, categories...)
	cmd := exec.Command(command, args...)
	cmd.Env = append(os.Environ(), "LC_ALL="+locale, "LANG="+locale, "LANGUAGE=")
	output, err := cmd.Output()
	return string(output), err
}

func runLocaleCheck(t *testing.T, script, locale string, extraEnv ...string) shellResult {
	t.Helper()
	env := append([]string{
		"TEST_LOCALE=" + locale,
	}, extraEnv...)
	return runVerifySnippet(t, script, env, `
source "$TEST_VERIFY_SCRIPT"
WORK="$TEST_VERIFY_WORK"
ACTIVE_LOG="$WORK/locale.log"
FAILURES=0
REPORT_NOTES=()
check_locale "$TEST_LOCALE"
printf '\nfailures=%d\n' "$FAILURES"
`)
}

type shellResult struct {
	output string
	err    error
	work   string
}

func runVerifySnippet(t *testing.T, script string, extraEnv []string, snippet string) shellResult {
	t.Helper()
	work := t.TempDir()
	if len(extraEnv) > 0 {
		for _, entry := range extraEnv {
			if strings.HasPrefix(entry, "TEST_VERIFY_WORK=") {
				t.Fatalf("runVerifySnippet owns TEST_VERIFY_WORK")
			}
		}
	}
	env := append(os.Environ(),
		"TEST_VERIFY_SCRIPT="+script,
		"TEST_VERIFY_WORK="+work,
	)
	env = append(env, extraEnv...)
	cmd := exec.Command("bash", "--noprofile", "--norc", "-c", snippet)
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	return shellResult{output: string(output), err: err, work: work}
}

func verifyScriptPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate verify test source")
	}
	return filepath.Join(filepath.Dir(file), "verify.sh")
}

func writeExecutable(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatal(err)
	}
}

func parseFailures(t *testing.T, output string) int {
	t.Helper()
	match := failuresPattern.FindStringSubmatch(output)
	if len(match) != 2 {
		t.Fatalf("output has no failures count:\n%s", output)
	}
	var failures int
	if _, err := fmt.Sscanf(match[1], "%d", &failures); err != nil {
		t.Fatal(err)
	}
	return failures
}

func parseStatuses(t *testing.T, output string) (int, int) {
	t.Helper()
	match := statusPattern.FindStringSubmatch(output)
	if len(match) != 3 {
		t.Fatalf("output has no function and script status:\n%s", output)
	}
	var functionStatus, scriptStatus int
	if _, err := fmt.Sscanf(match[1], "%d", &functionStatus); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Sscanf(match[2], "%d", &scriptStatus); err != nil {
		t.Fatal(err)
	}
	return functionStatus, scriptStatus
}

func readFileOrEmpty(path string) string {
	data, _ := os.ReadFile(path)
	return string(data)
}
