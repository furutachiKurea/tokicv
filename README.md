# tokicv

`tokicv` 是一个本地 Markdown 简历编写、A4 实时分页预览与导出工具，提供简体中文桌面工作区和脚本化 CLI，确保编辑预览与导出产物保持同一套 A4 版式

## 功能特性

- **所见即所得的 A4 导出**：桌面端与 CLI 共用同一套版式与分页结果；支持导出 PDF（保留可复制文本层与外链注解）、逐页 PNG 以及原始 Markdown
- **Resume Dialect 简历语法**：在常用 Markdown 基础上扩展双栏排版块（`::: left` / `::: right`）、行内徽章（`` `标签` ``）与内联图标指令（`icon:<name>`），锁定 A4 版心与字号层级，并在编辑器内实时标出语法错误与警告
- **非破坏性排版与智能一页**：所有排版调节均保存在 `.md` 源码之外，支持全局默认与单文档/单路径覆盖；开启「智能一页」后，会在不缩小字号且不突破最小可读边界的前提下，按**适度收紧节与条目间距（`8px` / `3px`）→ 逐级收紧行高（`1.40` → `1.30` → `1.20`）→ 收紧至最小间距（`5px` / `1.5px`）→ 收紧上下页边距（`7.5mm`）**的顺序试探单页适配，仍超出时保留原多页排版并给出提示
- **本地简历库与独立文件双模式**：桌面端与 CLI 均可管理内置的本地简历库，也支持直接打开、检查或导出任意相对或绝对路径下的 `.md` 文件

## 运行平台与环境依赖

### 平台边界

- **应用能力与代码边界**：应用功能本身按跨平台方式设计，不包含仅限 Linux 用户的产品特性，也不依赖外部 PDF/图像转换命令行工具
- **实机验证范围**：**当前实机运行、图形导出与自动化验收仅覆盖 Linux 桌面**；仓库中的 Linux 专属部分为图形验收测试、隔离图形验证脚本以及少量桌面/打印环境适配，**macOS 与 Windows 尚未在本仓库完成实机运行与图形验证**

### 构建与运行时依赖

- **构建工具链**：Go `1.27.3`（见 [go.mod](go.mod)）
- **Linux 运行时依赖**：
  - **系统图形库**：GTK 3（`libgtk-3.so.0`）与 WebKitGTK（`libwebkit2gtk-4.1.so.0` 或 `libwebkit2gtk-4.0.so.37`）
  - **字体**：西文字体 `Nunito` 已内置；建议系统安装 `Noto Sans CJK SC` 或 `Noto Serif CJK SC` 等中文字体以获得完整中文排版效果
  - **图形会话**：桌面工作区以及渲染型 CLI 命令（`check`、`export --pdf`、`export --png`）需要在 X11 或 Wayland 图形会话中运行（无桌面环境可配合 `xvfb-run` 等虚拟显示服务）；其余查询、简历库管理、排版配置及仅含 `--md` 的导出命令无需图形会话

## 快速开始

根据使用目的选择**本地编译运行**或**应用打包**，两者的命令与产出不同：

### 本地编译与运行

适合直接在本机开发、调试或立即作为命令行与桌面工具使用，无需经过应用打包流程

```sh
# 可选：检查本机 Go、GTK 3 与 WebKitGTK 环境是否就绪
go tool mygo doctor

# 编译当前平台的单文件可执行程序（同时包含桌面端与 CLI）
go build -o tokicv .
```

- **产物**：当前目录下的独立可执行文件 `./tokicv`

运行示例：

```sh
# 1. 启动桌面工作区（无参数打开本地简历库，传入路径则原地编辑该 .md 文件）
./tokicv
./tokicv ./resume.md

# 2. 使用 CLI 从模板创建简历、检查单页适配并导出 PDF
./tokicv create my-resume --template
./tokicv check my-resume --strict-one-page
./tokicv export my-resume --pdf resume.pdf --fit-one-page
```

> 若本地简历库中已存在同名文档，后续 `check`、`export` 等命令需改用 `tokicv list` 输出的文档 ID（`doc-...`）作为目标

### 应用打包

适合生成用于分发或安装到系统的平台发行包与桌面应用包

```sh
# 默认打包当前宿主机平台与架构
go tool mygo build

# 在 Linux 上交叉生成其他平台的基础产物（需跳过受宿主机限制的安装器）
go tool mygo build -platform darwin/arm64
go tool mygo build -platform windows/amd64 -skip-installer
```

- **产物目录**：`build/<平台>-<架构>/`（输出根目录由 [mygo.json](mygo.json) 配置）
- **已验证的 Linux 产物（`build/linux-amd64/`）**：
  - 可执行文件：`tokicv`
  - 安装与发行包（文件名中的版本号由 [mygo.json](mygo.json) 的 `version` 决定）：`tokicv-<version>-linux-amd64.tar.gz`、`install.sh`、`tokicv_<version>_amd64.deb`、`tokicv-<version>-1.x86_64.rpm`、Arch `PKGBUILD`
  - 桌面集成与许可文件：`tokicv.desktop`、应用图标（`icon.png`、`tokicv.png`、`tokicv-256x256.png`、`tokicv-512x512.png`）、`LICENSE`、`THIRD_PARTY_NOTICES.md`、`LICENSES/`
  - 命令入口（由 [mygo.json](mygo.json) 的 `linux.command` 配置）：安装 `.deb`、`.rpm` 或通过 `PKGBUILD` 安装时，会创建 `/usr/bin/tokicv` 软链接；使用 `install.sh` 用户级安装时，会创建 `~/.local/bin/tokicv` 软链接（需确保 `~/.local/bin` 在 `PATH` 中），安装后即可在终端直接运行 `tokicv`
- **交叉生成边界**：在 Linux 宿主机上已可交叉生成 `build/darwin-arm64/tokicv.app` 与 `build/windows-amd64/tokicv.exe`（及随附资源文件），但 `.dmg` 磁盘镜像与苹果签名受限于 macOS 宿主机，Windows NSIS 安装器在未安装 `makensis` 的 Linux 环境下无法生成；且 macOS 与 Windows 产物均尚未在对应系统实机运行验证

## Resume Dialect 语法

在常用 Markdown 元素（`**加粗**`、`[链接](https://example.com)`、自动链接 `<https://example.com>`、无序列表 `-` / `*` / `+`）之上，`tokicv` 定义了以下简历专用写法；斜体、代码块、引用块、分割线与图片等其余元素不会渲染为额外富样式，而是按纯文本或内部段落内容处理：

- **标题层级**：`#` 为姓名/主标题，`##` 为带全宽底线的分节标题，`###` 为条目小标题；四级及以下标题（`####` 起）按普通正文渲染
- **行内徽章**：行内代码 `` `标签` `` 渲染为紧凑无边框徽章
- **双栏排版**：连续编写 `::: left ... :::` 与 `::: right ... :::`（`:::` 后空白可选，大小写不敏感）组成左右两栏；右栏中形如 `YYYY.MM – YYYY.MM`（支持 `-`、`–`、`—`）的起止年月会自动应用日期样式；未闭合、未配对或多余的 `:::` 会回退为单列渲染并给出诊断提示
- **图标指令**：支持 `icon:info`、`icon:github`、`icon:phone`、`icon:email`、`icon:email2`（兼容 `icon:email 2`）、`icon:weixin`、`icon:blog`（大小写不敏感）；未知指令按纯文本原样输出并报告语法错误

以下示例展示完整结构：

```markdown
# 候选人名称

`标签 A` `标签 B` `标签 C`

::: left
icon:phone 联系电话占位
icon:email name@example.com
:::

::: right
icon:github [github.com/example/profile](https://example.com/github)
icon:blog [example.com](https://example.com)
icon:info 所在城市
:::

## 分节标题一（如：个人简介）

简要概述方向、关注领域或背景信息的占位段落

## 分节标题二（如：工作或项目经历）

::: left
### 机构或项目名称｜角色或职位
:::

::: right
2022.04 – 2026.03
:::

- **条目名称**｜`技术栈 A` `技术栈 B`
  - 一级要点占位文本，用于说明职责范围或实现方式
    - 二级要点占位文本，用于展示嵌套列表样式

## 分节标题三（如：教育背景）

::: left
**学校名称**｜专业名称｜学位层次
:::

::: right
2018.09 – 2022.06
:::
```

## 排版参数与 CLI

### 非破坏性排版参数

A4 版心与各级字号固定不变，桌面端「排版」面板与 CLI 仅开放以下非破坏性参数：

| 参数 | CLI 选项 | 默认值 | 允许范围 |
| --- | --- | --- | --- |
| 字体栈 | `--font` | `classic` | `classic`、`noto`、`serif`、`system` |
| 行高 | `--line-height` | `1.40` | `1.20` – `2.20` |
| 上/下页边距 | `--margin-v` / `--margin-top` / `--margin-bottom` | `10` | `7.5` – `30` mm |
| 分节间距 | `--section-gap` | `12` | `5` – `40` px |
| 条目间距 | `--item-gap` | `5` | `1.5` – `20` px |

- `--margin-v` 会同时设置上、下页边距
- 生效优先级为：**单次 CLI 参数 > 目标文档/路径保存覆盖 > 全局默认方案 > 内置默认方案**
- `--fit-one-page` 会以上述解析结果为基线，仅在本次检查或导出中临时试探收紧，不会覆盖已保存的排版配置

### CLI 命令参考

所有接收 `<target>` 的命令均按 **已存在的 `.md` 文件路径 → 简历库文档 ID → 简历库唯一名称** 的顺序解析目标，完整选项可执行 `tokicv --help` 查看：

```sh
# 信息查询（无需图形会话）
tokicv --help
tokicv --version

# 桌面工作区（需图形会话；传入路径时须为单个已存在的 .md 文件）
tokicv [<path.md>]
tokicv --gui [<path.md>]

# 简历库管理（无需图形会话）
tokicv list [--json]
tokicv create <name> [--from <path.md> | --template]
cat draft.md | tokicv create <name>
tokicv show <target>
tokicv import <path.md> [--name <name>]
tokicv delete <target>

# 排版配置管理（无需图形会话；不传 <target> 时操作全局默认方案）
tokicv layout show [<target>] [--json]
tokicv layout set [<target>] [--font <stack>] [--line-height <ratio>] [--margin-v <mm>] [--margin-top <mm>] [--margin-bottom <mm>] [--section-gap <px>] [--item-gap <px>]
tokicv layout reset [<target>]

# 分页检查与导出（check 及包含 --pdf / --png 的 export 需图形会话）
tokicv check <target> [--strict-one-page] [--fit-one-page] [--json] [<layout flags>] [--timeout <duration>]
tokicv export <target> [--md <out.md>] [--pdf <out.pdf>] [--png <dest>] [--fit-one-page] [<layout flags>] [--timeout <duration>]
```

行为说明：

- `create`：可通过 `--from <path.md>`、`--template` 或标准输入管道提供初始内容；三者均不提供时创建空文档
- `delete`：仅允许删除本地简历库中的文档，传入外部 `.md` 路径会拒绝操作，不会删除外部文件
- `check`：默认输出 `Page Fit: 1 / <N>`；启用 `--strict-one-page` 且最终页数大于 `1` 时返回退出码 `2`；启用 `--json` 时同时包含解析出的排版方案与语法诊断列表
- `export`：至少需指定 `--md`、`--pdf`、`--png` 之一；默认渲染超时 `--timeout` 为 `30s`
  - `--md <out.md>`：按原样写出未修改的 Markdown 字节；不搭配 `--pdf`/`--png` 时无需图形会话
  - `--png <dest>`：若 `<dest>` 是已存在目录或非 `.png` 路径（且非已存在的普通文件），将逐页写入 `<dest>/page-1.png`、`page-2.png` 等；若 `<dest>` 以 `.png` 结尾且不是目录，单页写为 `<dest>`，多页写为 `<stem>-1.png`、`<stem>-2.png` 等，并自动清理多出的旧编号文件（桌面端导出 PNG 遵循同一规则）

## 数据目录

本地简历库（`library/index.json`、`library/documents/`）与排版配置（`library/layouts.json`）存放在平台标准用户数据目录中（诊断日志路径与清理策略见 [验证门禁与诊断日志](docs/verification.md)）：

- **Linux**：`$XDG_CONFIG_HOME/tokicv`（默认 `~/.config/tokicv`）
- **macOS**：`~/Library/Application Support/tokicv`（按平台目录规则推算，尚未在本仓库实机验证）
- **Windows**：`%AppData%\tokicv`（按平台目录规则推算，尚未在本仓库实机验证）

初始化时机：

- `--help`、`--version`、`layout show`/`reset`（无既有配置时）以及针对外部 `.md` 文件的 `export --md` 不会创建该目录
- 启动桌面工作区、保存排版配置（`layout set`）以及调用 `list`、`create`、`show`、`import`、`delete` 等简历库命令时会初始化顶层目录；实际索引、文档与排版配置文件在首次写入时落盘

## 开发与验证

```sh
# 快速门禁（格式检查、静态检查与纯单元测试，无需图形会话）
./scripts/verify.sh fast

# 图形验收（仅限 Linux 隔离图形环境）与完整门禁
./scripts/verify.sh graphics
./scripts/verify.sh full
```

更多工程与设计文档：

- [验证门禁与诊断日志](docs/verification.md)
- [领域术语表](GLOSSARY.md)
- [ADR 0001：桌面工作区与 A4 预览](docs/adr/0001-hybrid-native-ui-and-webview-renderer.md)
- [ADR 0002：CLI 分页检查与导出](docs/adr/0002-headless-cli-export-via-offscreen-webview.md)
- [ADR 0003：自包含简历库与路径无关 CLI](docs/adr/0003-self-contained-library-and-path-independent-cli.md)
- [TokiCV 需求交付与 Linux 安装包](docs/agents/delivery.md)

## 许可证

本项目采用 `AGPL-3.0-only` 许可证发布，详见 [LICENSE](LICENSE)

第三方资源许可参见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)：`Nunito` 字体遵循 SIL Open Font License 1.1（[LICENSES/Nunito-OFL-1.1.txt](LICENSES/Nunito-OFL-1.1.txt)），`PDF.js`（`pdfjs-dist`）遵循 Apache License 2.0（[LICENSES/PDF.js-Apache-2.0.txt](LICENSES/PDF.js-Apache-2.0.txt)）
