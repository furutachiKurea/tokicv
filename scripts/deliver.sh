#!/usr/bin/env bash

set -euo pipefail

if [[ $# -ne 1 || $1 != /* ]]; then
	printf '用法: %s <输出根目录的绝对路径>\n' "$0" >&2
	exit 64
fi

output_root=$1
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

commit=$(git rev-parse --verify HEAD)
status=$(git status --porcelain --untracked-files=all)
if [[ -n $status ]]; then
	printf '请从候选提交的干净工作树交付\n' >&2
	exit 1
fi

mkdir -p -- "$output_root"
delivery=$(mktemp -d -- "${output_root%/}/${commit:0:12}.XXXXXX")
printf '交付目录: %s\n' "$delivery"

TOKICV_VERIFY_ARTIFACTS="$delivery/verification" scripts/verify.sh full
if ! go tool mygo build -platform linux/amd64 -o "$delivery/packages" >"$delivery/build.log" 2>&1; then
	printf '打包失败，日志: %s/build.log\n' "$delivery" >&2
	exit 1
fi

packages=$delivery/packages/linux-amd64
shopt -s nullglob
for suffix in deb rpm tar.gz; do
	matches=("$packages"/tokicv*."$suffix")
	if (( ${#matches[@]} == 0 )); then
		printf '缺少 %s 发行包，日志: %s/build.log\n' "$suffix" "$delivery" >&2
		exit 1
	fi
done
"$packages/tokicv" --version | tee "$delivery/version.txt"
printf '%s\n' "$commit" >"$delivery/commit.txt"
