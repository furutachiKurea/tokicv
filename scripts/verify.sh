#!/usr/bin/env bash

set -u

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
EXPORT_TESTS=(
	TestExportMixedCJKResumeToA4PDF
	TestExportEnglishLocaleResumeToA4PDF
)
DESKTOP_TESTS=(
	TestDesktopLocalePreviewPDFAndPNGParityUnderHeadlessLinux
	TestSmartOnePageFitsAcceptanceFixtureAndRemainsEditableUnderHeadlessLinux
	TestSmartOnePageFitsOnePageResumesInRealWindowUnderHeadlessLinux
	TestPDFJSTextLayerMouseSelectionCopiesExactText
)
REQUIRED_TESTS=("${EXPORT_TESTS[@]}" "${DESKTOP_TESTS[@]}")
LOCALE_CATEGORIES=(
	LC_CTYPE LC_NUMERIC LC_TIME LC_COLLATE LC_MONETARY LC_MESSAGES
	LC_PAPER LC_NAME LC_ADDRESS LC_TELEPHONE LC_MEASUREMENT LC_IDENTIFICATION
)

FAILURES=0
REPORT_NOTES=()
MODE=
WORK=
LOG=
FAST_LOG=
GRAPHICS_LOG=
REPORT=
ACTIVE_LOG=
KEEP=0

cleanup() {
	if [ "$KEEP" = "0" ] && [ -d "$WORK" ]; then
		chmod -R u+rwX "$WORK" 2>/dev/null || true
		find "$WORK" -depth -delete 2>/dev/null || true
	fi
}

add_failure() {
	FAILURES=$((FAILURES + 1))
	REPORT_NOTES+=("$1")
	printf 'FAIL: %s\n' "$1" >&2
}

run_cmd() {
	local label=$1
	shift
	printf '\n### %s\n$' "$label" >>"$ACTIVE_LOG"
	printf ' %q' "$@" >>"$ACTIVE_LOG"
	printf '\n' >>"$ACTIVE_LOG"
	"$@" >>"$ACTIVE_LOG" 2>&1
	local code=$?
	printf 'exit=%d\n' "$code" >>"$ACTIVE_LOG"
	if [ "$code" -ne 0 ]; then
		add_failure "$label (exit $code)"
		tail -n 30 "$ACTIVE_LOG" >&2 || true
	fi
	return "$code"
}

run_fmt_check() {
	local list=$WORK/gofmt-files.list
	local output code
	local -a files=()
	if ! find "$ROOT" -name '*.go' -not -path "$ROOT/.git/*" -print0 >"$list"; then
		printf '\n### gofmt\nfile discovery failed\n' >>"$ACTIVE_LOG"
		add_failure "gofmt file discovery failed"
		return 1
	fi
	mapfile -d '' -t files <"$list"
	if [ "${#files[@]}" -eq 0 ]; then
		printf '\n### gofmt\nno Go files found\n' >>"$ACTIVE_LOG"
		add_failure "gofmt found no Go files"
		return 1
	fi
	output=$(gofmt -l "${files[@]}" 2>&1)
	code=$?
	if [ "$code" -ne 0 ]; then
		printf '\n### gofmt\n%s\nexit=%d\n' "$output" "$code" >>"$ACTIVE_LOG"
		add_failure "gofmt failed (exit $code)"
		return "$code"
	fi
	if [ -n "$output" ]; then
		printf '\n### gofmt\n%s\n' "$output" >>"$ACTIVE_LOG"
		add_failure "gofmt found unformatted files"
		return 1
	else
		printf '\n### gofmt\nclean\n' >>"$ACTIVE_LOG"
	fi
	return 0
}

run_fast() {
	ACTIVE_LOG=$FAST_LOG
	run_fmt_check
	run_cmd "git diff --check" git -C "$ROOT" diff --check
	run_cmd "go tool mygo vet ." go tool mygo vet .
	run_cmd "CGO_ENABLED=0 go test ./..." env CGO_ENABLED=0 go test ./... -count=1 -timeout 5m -v
}

check_locale() {
	local name=$1
	local target c target_categories c_categories charmap code
	target=$(LC_ALL="$name" LANG="$name" LANGUAGE= locale -k "${LOCALE_CATEGORIES[@]}" 2>/dev/null)
	code=$?
	if [ "$code" -ne 0 ]; then
		add_failure "required locale query failed: $name (exit $code)"
		return 1
	fi
	charmap=$(LC_ALL="$name" LANG="$name" LANGUAGE= locale charmap 2>/dev/null)
	code=$?
	if [ "$code" -ne 0 ] || [ "$charmap" != "UTF-8" ]; then
		add_failure "required locale unavailable: $name (charmap=${charmap:-unknown}, exit $code)"
		return 1
	fi
	c=$(LC_ALL=C LANG=C LANGUAGE= locale -k "${LOCALE_CATEGORIES[@]}" 2>/dev/null)
	code=$?
	if [ "$code" -ne 0 ]; then
		add_failure "C locale category query failed (exit $code)"
		return 1
	fi
	target_categories=$(printf '%s\n' "$target" | grep -v '^charmap=')
	c_categories=$(printf '%s\n' "$c" | grep -v '^charmap=')
	if [ "$target_categories" = "$c_categories" ]; then
		add_failure "required locale fell back to C: $name"
		return 1
	fi
	return 0
}

find_minibrowser() {
	if command -v MiniBrowser >/dev/null 2>&1; then
		return 0
	fi
	for path in /usr/libexec/webkit2gtk-4.1/MiniBrowser /usr/lib64/webkit2gtk-4.1/MiniBrowser; do
		if [ -x "$path" ]; then
			return 0
		fi
	done
	return 1
}

run_preflight() {
	for command_name in go gofmt git mutter Xwayland dbus-daemon cc xsel locale pkg-config; do
		if ! command -v "$command_name" >/dev/null 2>&1; then
			add_failure "required command unavailable: $command_name"
		fi
	done
	check_locale zh_CN.UTF-8
	check_locale en_US.UTF-8
	if ! pkg-config --exists webkit2gtk-4.1 && ! pkg-config --exists webkit2gtk-4.0; then
		add_failure "WebKitGTK development/runtime package is unavailable"
	fi
	if ! find_minibrowser; then
		add_failure "MiniBrowser is unavailable for the PDF.js selection test"
	fi
	for fixture in \
		"$ROOT/internal/export/testdata/compact-120.md" \
		"$ROOT/internal/export/testdata/standard-130.md"; do
		if [ ! -f "$fixture" ]; then
			add_failure "required fixture unavailable: ${fixture#"$ROOT/"}"
		fi
	done
	printf 'preflight failures=%d\n' "$FAILURES" >>"$LOG"
}

pattern_for() {
	local pattern='^('
	local name
	for name in "$@"; do
		pattern+="$name|"
	done
	printf '%s)$' "${pattern%|}"
}

check_required_log() {
	local log=$1
	local name
	for name in "${REQUIRED_TESTS[@]}"; do
		if ! grep -Fq -- "=== RUN   $name" "$log"; then
			add_failure "required test did not run: $name"
			continue
		fi
		if grep -Fq -- "--- SKIP: $name" "$log"; then
			add_failure "required test skipped: $name"
		elif ! grep -Fq -- "--- PASS: $name" "$log"; then
			add_failure "required test did not pass: $name"
		fi
	done
	if grep -Eq -- '^--- SKIP:' "$log"; then
		add_failure "required graphics run contains a skip"
	fi
	if grep -Eq -- '^--- FAIL:' "$log"; then
		add_failure "required graphics run contains a failure"
	fi
}

run_required_tests() {
	local label=$1
	local log=$2
	local package=$3
	shift 3
	local pattern
	pattern=$(pattern_for "$@")
	printf '\n### %s\npattern=%s\n' "$label" "$pattern" >>"$LOG"
	run_cmd "$label" env CGO_ENABLED=1 go test "$package" -run "$pattern" -count=1 -v -timeout 15m
}

run_graphics() {
	ACTIVE_LOG=$GRAPHICS_LOG
	local preflight_failures=$FAILURES
	run_preflight
	if [ "$FAILURES" -ne "$preflight_failures" ]; then
		REPORT_NOTES+=("graphics tests skipped after preflight failure")
		return
	fi
	run_required_tests "export required tests" "$GRAPHICS_LOG" ./internal/export "${EXPORT_TESTS[@]}"
	run_required_tests "desktop required tests" "$GRAPHICS_LOG" ./internal/desktop "${DESKTOP_TESTS[@]}"
	check_required_log "$GRAPHICS_LOG" || true
}

write_report() {
	cat "$FAST_LOG" "$GRAPHICS_LOG" >"$LOG"
	local status=PASS
	if [ "$FAILURES" -ne 0 ]; then
		status=FAIL
	fi
	local commit dirty go_version webkit_version locale_value
	commit=$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || printf 'unknown')
	dirty=$(git -C "$ROOT" status --porcelain --untracked-files=all 2>/dev/null | wc -l | tr -d ' ')
	go_version=$(go version 2>/dev/null | awk '{print $3}')
	webkit_version=$(pkg-config --modversion webkit2gtk-4.1 2>/dev/null || pkg-config --modversion webkit2gtk-4.0 2>/dev/null || printf 'unknown')
	locale_value="$LC_ALL"
	local pass_count skip_count fail_count
	pass_count=$(grep -c -- '^--- PASS:' "$LOG" 2>/dev/null || true)
	skip_count=$(grep -c -- '^--- SKIP:' "$LOG" 2>/dev/null || true)
	fail_count=$(grep -c -- '^--- FAIL:' "$LOG" 2>/dev/null || true)
	local fast_pass fast_skip fast_fail graphics_pass graphics_skip graphics_fail
	fast_pass=$(grep -c -- '^--- PASS:' "$FAST_LOG" 2>/dev/null || true)
	fast_skip=$(grep -c -- '^--- SKIP:' "$FAST_LOG" 2>/dev/null || true)
	fast_fail=$(grep -c -- '^--- FAIL:' "$FAST_LOG" 2>/dev/null || true)
	graphics_pass=$(grep -c -- '^--- PASS:' "$GRAPHICS_LOG" 2>/dev/null || true)
	graphics_skip=$(grep -c -- '^--- SKIP:' "$GRAPHICS_LOG" 2>/dev/null || true)
	graphics_fail=$(grep -c -- '^--- FAIL:' "$GRAPHICS_LOG" 2>/dev/null || true)
	local content_matches selection_trusted
	content_matches=$(grep -c 'acceptance export .*content-match=true' "$LOG" 2>/dev/null || true)
	selection_trusted=$(grep -c 'trusted=true' "$LOG" 2>/dev/null || true)
	local visual_evidence=PASS
	if [ "$MODE" = "fast" ]; then
		visual_evidence=NOT-APPLICABLE
	elif ! grep -q 'desktop parity:' "$LOG"; then
		visual_evidence=UNKNOWN
	fi
	local required_pass=0
	local name
	if [ "$MODE" != "fast" ]; then
		for name in "${REQUIRED_TESTS[@]}"; do
			if grep -Fq -- "--- PASS: $name" "$LOG"; then
				required_pass=$((required_pass + 1))
			fi
		done
	fi

	{
		printf '# tokicv verification\n\n'
		printf -- '- status: %s\n' "$status"
		printf -- '- mode: %s\n' "$MODE"
		printf -- '- commit: %s\n' "$commit"
		printf -- '- worktree: %s changed path(s)\n' "$dirty"
		printf -- '- environment: linux/amd64, %s, CGO_ENABLED=%s, WebKitGTK %s\n' \
			"$go_version" "$([ "$MODE" = fast ] && printf 0 || printf 1)" "$webkit_version"
		printf -- '- locale: %s\n' "$locale_value"
		printf -- '- test results: %s pass, %s skip, %s fail\n' "$pass_count" "$skip_count" "$fail_count"
		if [ "$MODE" != "graphics" ]; then
			printf -- '- fast results: %s pass, %s skip, %s fail\n' "$fast_pass" "$fast_skip" "$fast_fail"
		fi
		if [ "$MODE" != "fast" ]; then
			printf -- '- graphics results: %s pass, %s skip, %s fail\n' "$graphics_pass" "$graphics_skip" "$graphics_fail"
		fi
		if [ "$MODE" = "fast" ]; then
			printf -- '- required: not applicable to fast\n'
		else
			printf -- '- required: %d/%d required parent tests passed\n' "$required_pass" "${#REQUIRED_TESTS[@]}"
		fi
		if [ "$MODE" = "fast" ]; then
			printf -- '- evidence: fast checks only; graphics evidence intentionally absent\n'
		else
			printf -- '- evidence: pages recorded below; content-match=%s; trusted-selection-lines=%s\n' "$content_matches" "$selection_trusted"
			printf -- '- visual tolerance: page counts and DOM/PDF/PNG A4 aspect parity (tolerance 0.002)\n'
		fi
		printf -- '- fixture: internal/export/testdata/compact-120.md; redacted acceptance; fixture pixel identity requires a separate baseline\n'
		printf -- '- artifacts: %s\n' "$WORK"
		printf '\n## Required result\n\n'
		if [ "$MODE" = fast ]; then
			printf -- '- fast checks completed; any required graphics evidence is intentionally absent\n'
		else
			printf -- '- graphics evidence: %s\n' "$visual_evidence"
			printf -- '- required tests: %s\n' "${REQUIRED_TESTS[*]}"
		fi
		if [ "${#REPORT_NOTES[@]}" -ne 0 ]; then
			printf '\n## Failures\n\n'
			for note in "${REPORT_NOTES[@]}"; do
				printf -- '- %s\n' "$note"
			done
		fi
		printf '\n## Evidence\n\n'
		grep -E 'acceptance export |desktop parity:|Smart One-Page:|input=|preflight ' "$LOG" 2>/dev/null | head -n 20 || true
	} >"$REPORT"

	printf '%s verification: %s\n' "$MODE" "$status"
	cat "$REPORT"
	if [ "$KEEP" = "0" ]; then
		printf 'report was written to a temporary directory and will be removed; set TOKICV_VERIFY_ARTIFACTS to retain it\n'
	else
		printf 'report: %s\n' "$REPORT"
	fi
	if [ "$FAILURES" -ne 0 ]; then
		return 1
	fi
	return 0
}

main() {
	MODE=${1:-}
	case "$MODE" in
	fast|graphics|full) ;;
	*)
		printf '用法: %s {fast|graphics|full}\n' "$0" >&2
		return 64
		;;
	esac
	cd "$ROOT" || return 1

	if [ -n "${TOKICV_VERIFY_ARTIFACTS:-}" ]; then
		case "$TOKICV_VERIFY_ARTIFACTS" in
		/*) WORK=$TOKICV_VERIFY_ARTIFACTS ;;
		*) WORK=$ROOT/$TOKICV_VERIFY_ARTIFACTS ;;
		esac
		mkdir -p "$WORK" || return 1
		KEEP=1
	else
		WORK=$(mktemp -d /tmp/tokicv-verify.XXXXXX) || return 1
		KEEP=0
	fi
	if [ "${TOKICV_VERIFY_KEEP:-0}" = "1" ]; then
		KEEP=1
	fi
	LOG=$WORK/verify.log
	FAST_LOG=$WORK/fast.log
	GRAPHICS_LOG=$WORK/graphics.log
	REPORT=$WORK/report.md
	: >"$LOG"
	: >"$FAST_LOG"
	: >"$GRAPHICS_LOG"
	ACTIVE_LOG=$LOG
	FAILURES=0
	REPORT_NOTES=()
	trap cleanup EXIT INT TERM

	mkdir -p "$WORK/home" "$WORK/config" "$WORK/data" "$WORK/cache" "$WORK/tmp"
	local original_gomodcache original_gocache
	original_gomodcache=${GOMODCACHE:-$(go env GOMODCACHE 2>/dev/null)}
	original_gocache=${GOCACHE:-$(go env GOCACHE 2>/dev/null)}
	export HOME="$WORK/home"
	export XDG_CONFIG_HOME="$WORK/config"
	export XDG_DATA_HOME="$WORK/data"
	export XDG_CACHE_HOME="$WORK/cache"
	export TMPDIR="$WORK/tmp"
	export TZ=UTC
	unset DISPLAY WAYLAND_DISPLAY WAYLAND_SOCKET GDK_BACKEND XDG_RUNTIME_DIR \
		XDG_SESSION_TYPE DBUS_SESSION_BUS_ADDRESS XAUTHORITY
	export LC_ALL=zh_CN.UTF-8
	export LANG=zh_CN.UTF-8
	export LANGUAGE=zh_CN:en
	export TOKICV_VERIFY_REQUIRED=1
	export GOMODCACHE=$original_gomodcache
	export GOCACHE=$original_gocache

	if [ "$MODE" = fast ] || [ "$MODE" = full ]; then
		run_fast
	fi
	if [ "$MODE" = graphics ] || [ "$MODE" = full ]; then
		run_graphics
	fi

	write_report
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
	main "$@"
	exit $?
fi
