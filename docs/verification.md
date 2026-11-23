# 验证门禁与诊断日志

统一入口为 `scripts/verify.sh fast|graphics|full`

## 模式与隔离

| 模式 | 执行内容 | 判定 |
| --- | --- | --- |
| `fast` | `gofmt`、`git diff --check`、`go tool mygo vet .`、`CGO_ENABLED=0 go test ./...` | 任一检查失败即失败 |
| `graphics` | required WebKitGTK 验收、私有 mutter/Xwayland、独立 D-Bus | 缺工具、locale、fixture，或必需用例未执行、SKIP、FAIL 即失败 |
| `full` | `fast` 后执行 `graphics` | 两阶段全部通过才通过 |

脚本先清除宿主图形环境变量。图形测试只连接测试进程在 `t.TempDir()` 或独立 runtime 目录内启动的 mutter、Xwayland 与 D-Bus。D-Bus 使用 `unix:path=<本次目录>/dbus.sock`，不创建 `/tmp/dbus-*`

测试临时目录使用独立的短路径 `/tmp/tcv.*`，退出时清理。报告目录可以使用较深的路径

`zh_CN.UTF-8` 与 `en_US.UTF-8` 必须真正生效。检查同时核对 UTF-8 charmap 与全部 locale 类别数据；`locale` 返回 0 但回退到 C 数据视为失败

## 报告与产物

报告默认写入 `/tmp/tokicv-verify.*/report.md`，退出时删除

| 变量 | 作用 |
| --- | --- |
| `TOKICV_VERIFY_ARTIFACTS=/path` | 把报告与日志保留到指定目录 |
| `TOKICV_VERIFY_KEEP=1` | 保留默认临时目录 |

报告记录 status、mode、commit、changed path 数、Go/WebKitGTK 版本、locale、测试 pass/skip/fail 数、required 通过数、证据行、视觉容差与产物路径。required 证据包括 DOM/PDF/PNG 页数和 A4 纵横比、PDF 文本、content-match、trusted selection

## 诊断日志

诊断日志位于 mygo `PathLogs` 下的固定文件名 `tokicv.log`、`tokicv.log.1`、`tokicv.log.2`

| 项目 | 策略 |
| --- | --- |
| 单文件上限 | 1 MiB |
| 总预算 | 3 MiB |
| 保存期限 | 7 天，按文件创建时间或首条记录时间计算 |
| 清理间隔 | 启动检查；运行中写入时最多每 5 分钟检查一次 |
| 超限处理 | 当前文件超限时重建；备份超限时删除；多余编号删除 |
| 非本应用文件 | 不匹配固定命名的文件不读取、不修改 |

当前文件首行写入 `#created=<RFC3339>`。旧文件没有标记时读取首条 `2006/01/02 15:04:05` 时间，无法读取时回退到 mtime。append 更新的 mtime 不会延长文件寿命

7 天是保留文件的最大年龄。文件首条时间超过 7 天时整份删除；运行中有持续写入时，过期文件最多再保留一个 5 分钟检查间隔。应用无写入期间不运行后台 daemon，下一次启动或写入执行清理

单条记录超过剩余容量时截断并附加 `[truncated]`，结果保持合法 UTF-8。文件创建标记也计入单文件上限

记录只包含 stage、错误类型和安全错误原因。`os.PathError`、`os.LinkError` 与 wrapped errors 通过 `errors.As` 提取操作和底层原因，路径不进入日志；未知错误正文替换为 `<redacted>`；邮箱和电话占位脱敏；文本上限 512 个字符且包含省略号

日志目录创建、清理或写入失败时编辑与导出继续运行，stderr 保留降级诊断
