# tokicv

`tokicv` 是一个基于 Go、[mygo](https://github.com/egoist/mygo)（`v0.9.0`）与 [goldmark](https://github.com/yuin/goldmark) 的本地 Markdown 简历编写、A4 实时分页预览与导出工具，提供简体中文桌面工作区与等价 CLI

## 核心特性

- **编辑与预览一致**：桌面预览、PDF 导出与 PNG 导出共用同一套 HTML/CSS 版式与嵌入式 WebView 渲染管线；PDF 自动修复可复制文本层与外链注解，PNG 直接由内嵌 `pdfjs-dist` 栅格化生成，无需外部 `pdftoppm` 等工具
- **Resume Dialect 语法**：在标准 Markdown 基础上扩展双栏排版块（`::: left` / `::: right`）、行内徽章（`` `标签` ``）与内联图标指令（`icon:<name>`），锁定 A4 版心与字号层级
- **非破坏性排版与智能一页**：支持在不修改 `.md` 源码的前提下调整字体栈、行高、上下页边距与间距，支持按全局默认或单文档保存覆盖；开启「智能一页」后会在可读上限内自动收紧间距与边距以适配单页，无法收进时保留原多页排版并提示
- **本地简历库与路径无关 CLI**：支持在桌面端或 CLI 中管理本地简历库（新建、模板、导入、删除），也可直接对任意相对或绝对 `.md` 文件原地编辑、检查与导出

## 运行平台与依赖

- **平台边界**：代码基于 Go 与 `mygo v0.9.0` 跨平台抽象编写，但**当前实际运行与图形自动化验收仅覆盖 Linux 桌面环境**（GTK 3 + WebKitGTK）；macOS 与 Windows 尚未在本仓库完成实机图形验证
- **构建工具链**：Go `1.27.3`（见 [go.mod](go.mod)）；`mygo` CLI 已通过 `go.mod` 的 `tool` 指令锁定为 `v0.9.0`，可直接通过 `go tool mygo` 调用，常规构建无需开启 CGO（仅运行 `scripts/verify.sh graphics` 测试需 `CGO_ENABLED=1` 与 `cc`）
- **Linux 运行时依赖**：
  - **GTK 3**（`libgtk-3.so.0`）与 **WebKitGTK**（`libwebkit2gtk-4.1.so.0` 或 `libwebkit2gtk-4.0.so.37`）
  - **中文字体**：建议安装 `Noto Sans CJK SC` 或 `Noto Serif CJK SC` 等中文字体；西文字体 `Nunito` 已内嵌于二进制中
  - **图形会话**：桌面 GUI 及渲染型命令（`check`、`export --pdf`、`export --png`）需要在 X11 或 Wayland 图形会话中运行（无桌面环境可配合 `xvfb-run`）；其余纯 Go 管理命令无需图形会话

## 快速开始

```sh
# 检查本地 Go 与图形运行库环境
go tool mygo doctor

# 编译可执行文件（打包发行产物可运行 go tool mygo build）
go build -o tokicv .

# 启动桌面工作区（打开本地简历库，或传入 .md 文件原地编辑）
./tokicv
./tokicv ./resume.md

# 通过 CLI 从模板创建简历、检查分页并导出单页 PDF
./tokicv create my-resume --template
./tokicv check my-resume --strict-one-page
./tokicv export my-resume --pdf resume.pdf --fit-one-page
```

## Resume Dialect 语法与示例

在标准 Markdown（`**加粗**`、`[链接](https://example.com)`、无序列表 `-` / `*` / `+`）之上，`tokicv` 提供以下简历专用语法：

- **标题层级**：`#` 用于主标题（姓名/抬头），`##` 用于带全宽底线的分节标题，`###` 用于条目小标题；四级及以下标题按普通正文渲染
- **行内徽章**：行内代码 `` `标签` `` 渲染为紧凑徽章样式
- **双栏排版（Split Block）**：连续编写 `::: left ... :::` 与 `::: right ... :::` 组成左右两栏；右栏中的 `YYYY.MM – YYYY.MM` 起止年月会自动应用日期样式；未配对容器会回退为单列并给出诊断提示
- **图标指令**：支持 `icon:info`、`icon:github`、`icon:phone`、`icon:email`、`icon:email2`（兼容 `icon:email 2`）、`icon:weixin`、`icon:blog`；未知指令按纯文本回退并报告诊断错误

以下示例使用信息中立占位字段展示语法结构：

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

## 排版参数与 CLI 用法

### 可调排版参数

所有 A4 字号层级均在样式表中锁定以保证版面一致，界面「排版」面板与 CLI 仅开放以下非破坏性参数：

| 参数 | CLI 选项 | 默认值 | 允许范围 |
| --- | --- | --- | --- |
| 字体栈 | `--font` | `classic` | `classic`、`noto`、`serif`、`system` |
| 行高 | `--line-height` | `1.40` | `1.20` – `2.20` |
| 上/下页边距 | `--margin-v` / `--margin-top` / `--margin-bottom` | `10` | `7.5` – `30` mm |
| 分节间距 | `--section-gap` | `12` | `5` – `40` px |
| 条目间距 | `--item-gap` | `5` | `1.5` – `20` px |

排版优先级为：**单次 CLI 参数 > 目标文档/路径保存覆盖 > 全局默认方案 > 内置默认方案**

### 常用命令

所有接收 `<target>` 的命令按 **已存在的 `.md` 路径 → 简历库文档 ID → 简历库唯一名称** 顺序解析，完整选项可通过 `tokicv --help` 查看：

```sh
# 桌面工作区
tokicv [<path.md>]
tokicv --gui [<path.md>]

# 简历库管理（纯 Go 执行）
tokicv list [--json]
tokicv create <name> [--from <path.md> | --template]
cat draft.md | tokicv create <name>
tokicv show <target>
tokicv import <path.md> [--name <name>]
tokicv delete <target>

# 排版配置（纯 Go 执行）
tokicv layout show [<target>] [--json]
tokicv layout set [<target>] [--font <stack>] [--line-height <ratio>] [--margin-v <mm>] [--section-gap <px>] [--item-gap <px>]
tokicv layout reset [<target>]

# 分页检查与导出（--pdf / --png / check 需图形会话）
tokicv check <target> [--strict-one-page] [--fit-one-page] [--json] [<layout flags>]
tokicv export <target> [--md <out.md>] [--pdf <out.pdf>] [--png <dest>] [--fit-one-page] [<layout flags>]
```

- `check --strict-one-page`：最终页数大于 `1` 时返回退出码 `2`
- `export --png <dest>`：`<dest>` 以 `.png` 结尾时单页写为 `<dest>`、多页写为 `<stem>-1.png` 等；传入目录或非 `.png` 路径时写入 `<dest>/page-1.png` 等，并自动清理多余旧编号文件
- `export --md <out.md>`：按原样输出未经修改的 Markdown 字节；不搭配 `--pdf`/`--png` 时无需图形会话

## 数据目录

本地简历库与排版配置存放在平台标准用户数据目录中，仅在发生写入操作时按需创建（诊断日志路径与策略见 [验证门禁与诊断日志](docs/verification.md)）：

- **Linux**：`$XDG_CONFIG_HOME/tokicv`（默认 `~/.config/tokicv`）
- **macOS**：`~/Library/Application Support/tokicv`
- **Windows**：`%AppData%\tokicv`

## 开发验证与文档

```sh
# 快速门禁：gofmt、git diff --check、go tool mygo vet .、CGO_ENABLED=0 go test ./...
./scripts/verify.sh fast

# 图形验收（隔离 mutter/Xwayland 与独立 D-Bus）与完整门禁
./scripts/verify.sh graphics
./scripts/verify.sh full
```

更多说明见仓库文档：

- [验证门禁与诊断日志](docs/verification.md)
- [领域术语表](GLOSSARY.md)
- [ADR 0001：原生 UI 外壳与嵌入 WebView 渲染器](docs/adr/0001-hybrid-native-ui-and-webview-renderer.md)
- [ADR 0002：通过离屏 WebView 执行渲染型 CLI 命令](docs/adr/0002-headless-cli-export-via-offscreen-webview.md)
- [ADR 0003：自包含简历库与路径无关 CLI](docs/adr/0003-self-contained-library-and-path-independent-cli.md)

## 许可证

本项目采用 `AGPL-3.0-only` 许可证发布，详见 [LICENSE](LICENSE)

内嵌资源中，`Nunito` 字体遵循 SIL Open Font License 1.1，`pdfjs-dist` 遵循 Apache License 2.0
