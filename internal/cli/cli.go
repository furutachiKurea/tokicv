package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"tokicv/internal/dialect"
	"tokicv/internal/export"
	"tokicv/internal/library"
	"tokicv/internal/render"
)

// Version is the tokicv release version.
const Version = "0.1.1"

// DefaultTimeout is the default rendering timeout.
const DefaultTimeout = 30 * time.Second

// FontStack is the Resume Sheet font family selected by the CLI.
type FontStack = render.FontStack

// Supported Resume Sheet font stacks.
const (
	FontClassic = render.FontClassic
	FontNoto    = render.FontNoto
	FontSerif   = render.FontSerif
	FontSystem  = render.FontSystem
)

// CommandKind identifies a parsed CLI command.
type CommandKind int

// CLI commands.
const (
	CommandHelp CommandKind = iota
	CommandVersion
	CommandExport
	CommandCheck
	CommandGUI
	CommandList
	CommandCreate
	CommandShow
	CommandImport
	CommandDelete
	CommandLayout
)

// Command is one parsed CLI invocation.
type Command struct {
	Kind   CommandKind
	Export ExportRequest
	Check  CheckRequest
	Path   string
	List   ListRequest
	Create CreateRequest
	Show   ShowRequest
	Import ImportRequest
	Delete DeleteRequest
	Layout LayoutRequest
}

// ExportRequest describes one Resume Document Artifact export.
type ExportRequest struct {
	Input      string
	Output     string
	PNG        string
	Markdown   string
	Timeout    time.Duration
	Font       FontStack
	Layout     render.LayoutPatch
	FitOnePage bool
}

// ExportFunc performs an export request.
type ExportFunc func(ExportRequest) error

// CheckRequest describes one Resume Document Page Fit check.
type CheckRequest struct {
	Input         string
	StrictOnePage bool
	FitOnePage    bool
	JSON          bool
	Timeout       time.Duration
	Font          FontStack
	Layout        render.LayoutPatch
}

// CheckFunc performs a Page Fit check.
type CheckFunc func(CheckRequest) (export.CheckResult, error)

// GUIFunc opens the desktop authoring workspace.
type GUIFunc func(path string) error

// ListRequest describes a library listing.
type ListRequest struct {
	JSON bool
}

// CreateRequest describes creation of a library Resume Document.
type CreateRequest struct {
	Name     string
	From     string
	Template bool
}

// ShowRequest describes printing one Resume Document source.
type ShowRequest struct {
	Target string
}

// ImportRequest describes importing an external Markdown file.
type ImportRequest struct {
	Path string
	Name string
}

// DeleteRequest describes deleting one library Resume Document.
type DeleteRequest struct {
	Target string
}

// LayoutAction identifies one Layout Profile management operation.
type LayoutAction int

// Layout Profile management operations.
const (
	LayoutShow LayoutAction = iota
	LayoutSet
	LayoutReset
)

// LayoutRequest describes a Layout Profile management invocation.
type LayoutRequest struct {
	Action LayoutAction
	Target string
	Patch  render.LayoutPatch
	JSON   bool
}

// LayoutView is the JSON representation returned by layout show --json.
type LayoutView struct {
	Target string               `json:"target,omitempty"`
	Origin library.LayoutOrigin `json:"origin"`
	Layout render.Layout        `json:"layout"`
}

// RunOptions configures command execution. Store may be nil for commands that
// need the default library, which is opened lazily only when required.
type RunOptions struct {
	Stdin           io.Reader
	StdinRedirected bool
	Stdout          io.Writer
	Stderr          io.Writer
	Store           *library.Store
	Export          ExportFunc
	Check           CheckFunc
	GUI             GUIFunc
}

// Run parses args, runs the selected command, and returns its exit code.
func Run(args []string, stdout, stderr io.Writer, export ExportFunc) int {
	return RunWithOptions(args, RunOptions{
		Stdout: stdout,
		Stderr: stderr,
		Export: export,
	})
}

// RunWithCheck parses args and runs export and check commands.
func RunWithCheck(args []string, stdout, stderr io.Writer, export ExportFunc, check CheckFunc) int {
	return RunWithOptions(args, RunOptions{
		Stdout: stdout,
		Stderr: stderr,
		Export: export,
		Check:  check,
	})
}

// RunWithGUI parses args and runs export and desktop commands.
func RunWithGUI(args []string, stdout, stderr io.Writer, export ExportFunc, gui GUIFunc) int {
	return RunWithOptions(args, RunOptions{
		Stdout: stdout,
		Stderr: stderr,
		Export: export,
		GUI:    gui,
	})
}

// RunWithGUIAndCheck parses args and dispatches export, check, and desktop
// commands.
func RunWithGUIAndCheck(args []string, stdout, stderr io.Writer, export ExportFunc, check CheckFunc, gui GUIFunc) int {
	return RunWithOptions(args, RunOptions{
		Stdout: stdout,
		Stderr: stderr,
		Export: export,
		Check:  check,
		GUI:    gui,
	})
}

// RunWithOptions parses args and executes the selected command.
func RunWithOptions(args []string, opts RunOptions) int {
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	if opts.Stdin == nil {
		opts.Stdin = strings.NewReader("")
	}

	command, err := Parse(args)
	if err != nil {
		fmt.Fprintf(opts.Stderr, "tokicv: %v\n\n", err)
		fmt.Fprintln(opts.Stderr, usageText)
		return 1
	}

	switch command.Kind {
	case CommandHelp:
		fmt.Fprintln(opts.Stdout, usageText)
	case CommandVersion:
		fmt.Fprintf(opts.Stdout, "tokicv %s\n", Version)
	case CommandList:
		return runList(opts, command.List)
	case CommandCreate:
		return runCreate(opts, command.Create)
	case CommandShow:
		return runShow(opts, command.Show)
	case CommandImport:
		return runImport(opts, command.Import)
	case CommandDelete:
		return runDelete(opts, command.Delete)
	case CommandExport:
		return runExportCommand(opts, command.Export)
	case CommandCheck:
		return runCheckCommand(opts, command.Check)
	case CommandLayout:
		return runLayoutCommand(opts, command.Layout)
	case CommandGUI:
		if opts.GUI == nil {
			fmt.Fprintln(opts.Stderr, "tokicv: 桌面界面不可用")
			return 1
		}
		if err := opts.GUI(command.Path); err != nil {
			fmt.Fprintf(opts.Stderr, "tokicv: %v\n", err)
			return 1
		}
	}
	return 0
}

func (opts RunOptions) openLibrary() (*library.Store, error) {
	if opts.Store != nil {
		return opts.Store, nil
	}
	return library.Open()
}

func (opts RunOptions) openLayoutStore(write bool) (*library.Store, error) {
	if opts.Store != nil {
		return opts.Store, nil
	}
	if write {
		return opts.openLibrary()
	}
	return library.OpenLayoutReadOnly()
}

func runExportCommand(opts RunOptions, request ExportRequest) int {
	if request.Markdown != "" && request.Output == "" && request.PNG == "" {
		if err := export.WriteMarkdown(opts.Store, request.Input, request.Markdown); err != nil {
			fmt.Fprintf(opts.Stderr, "tokicv: %v\n", err)
			return 1
		}
		return 0
	}

	if request.Layout.Any() {
		store, target, layout, err := resolveCommandLayout(opts, request.Input, request.Layout)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "tokicv: %v\n", err)
			return 1
		}
		err = export.Run(export.Options{
			Input:          targetDisplayInput(target),
			Output:         request.Output,
			PNGOutput:      request.PNG,
			MarkdownOutput: request.Markdown,
			Timeout:        request.Timeout,
			Layout:         layout,
			LayoutSet:      true,
			Store:          store,
			FitOnePage:     request.FitOnePage,
			Notice:         opts.Stderr,
		})
		if err != nil {
			fmt.Fprintf(opts.Stderr, "tokicv: %v\n", err)
			return 1
		}
		return 0
	}

	if opts.Export == nil {
		fmt.Fprintln(opts.Stderr, "tokicv: 导出器不可用")
		return 1
	}
	if err := opts.Export(request); err != nil {
		fmt.Fprintf(opts.Stderr, "tokicv: %v\n", err)
		return 1
	}
	return 0
}

func runCheckCommand(opts RunOptions, request CheckRequest) int {
	var (
		result export.CheckResult
		err    error
	)
	if request.Layout.Any() {
		store, target, layout, resolveErr := resolveCommandLayout(opts, request.Input, request.Layout)
		if resolveErr != nil {
			fmt.Fprintf(opts.Stderr, "tokicv: %v\n", resolveErr)
			return 1
		}
		result, err = export.Check(export.CheckOptions{
			Input:      targetDisplayInput(target),
			Timeout:    request.Timeout,
			Layout:     layout,
			LayoutSet:  true,
			Store:      store,
			FitOnePage: request.FitOnePage,
		})
	} else {
		if opts.Check == nil {
			fmt.Fprintln(opts.Stderr, "tokicv: Page Fit 检查器不可用")
			return 1
		}
		result, err = opts.Check(request)
	}
	if err != nil {
		fmt.Fprintf(opts.Stderr, "tokicv: %v\n", err)
		return 1
	}
	return writeCheckResult(opts, request, result)
}

func writeCheckResult(opts RunOptions, request CheckRequest, result export.CheckResult) int {
	if request.JSON {
		if result.Diagnostics == nil {
			result.Diagnostics = make([]dialect.Diagnostic, 0)
		}
		if err := json.NewEncoder(opts.Stdout).Encode(result); err != nil {
			fmt.Fprintf(opts.Stderr, "tokicv: 写入 JSON: %v\n", err)
			return 1
		}
		if request.StrictOnePage && result.NumPages > 1 {
			return 2
		}
		return 0
	}
	fmt.Fprintf(opts.Stdout, "Page Fit: 1 / %d\n", result.NumPages)
	if result.Notice != "" {
		fmt.Fprintln(opts.Stdout, result.Notice)
	}
	if request.StrictOnePage && result.NumPages > 1 {
		fmt.Fprintln(opts.Stdout, "超出单页：内容无法放入一个页面")
		return 2
	}
	return 0
}

func resolveCommandLayout(opts RunOptions, input string, patch render.LayoutPatch) (*library.Store, library.Target, render.Layout, error) {
	store, err := opts.openLayoutStore(false)
	if err != nil {
		return nil, library.Target{}, render.Layout{}, err
	}
	target, err := store.ResolveTarget(input)
	if err != nil {
		return nil, library.Target{}, render.Layout{}, err
	}
	layout, err := store.ResolveLayout(target, patch)
	if err != nil {
		return nil, library.Target{}, render.Layout{}, err
	}
	return store, target, layout, nil
}

func targetDisplayInput(target library.Target) string {
	if target.Kind == library.TargetLibrary {
		return target.Document.ID
	}
	return target.Path
}

func runLayoutCommand(opts RunOptions, request LayoutRequest) int {
	switch request.Action {
	case LayoutShow:
		return showLayout(opts, request)
	case LayoutSet:
		return setLayout(opts, request)
	case LayoutReset:
		return resetLayout(opts, request)
	default:
		fmt.Fprintln(opts.Stderr, "tokicv: 未知 layout 操作")
		return 1
	}
}

func showLayout(opts RunOptions, request LayoutRequest) int {
	store, err := opts.openLayoutStore(false)
	if err != nil {
		fmt.Fprintf(opts.Stderr, "tokicv: %v\n", err)
		return 1
	}

	var resolved library.ResolvedLayout
	if request.Target == "" {
		resolved, err = store.ResolveGlobalLayout()
	} else {
		target, resolveErr := store.ResolveTarget(request.Target)
		if resolveErr != nil {
			fmt.Fprintf(opts.Stderr, "tokicv: %v\n", resolveErr)
			return 1
		}
		resolved, err = store.ResolveLayoutProfile(target)
	}
	if err != nil {
		fmt.Fprintf(opts.Stderr, "tokicv: %v\n", err)
		return 1
	}

	view := LayoutView{
		Target: request.Target,
		Origin: resolved.Origin,
		Layout: resolved.Layout,
	}
	if request.JSON {
		if err := json.NewEncoder(opts.Stdout).Encode(view); err != nil {
			fmt.Fprintf(opts.Stderr, "tokicv: 写入 JSON: %v\n", err)
			return 1
		}
		return 0
	}
	writeLayoutView(opts, view)
	return 0
}

func setLayout(opts RunOptions, request LayoutRequest) int {
	if !request.Patch.Any() {
		fmt.Fprintln(opts.Stderr, "tokicv: layout set 至少需要一个布局参数")
		return 1
	}
	if err := request.Patch.Validate(); err != nil {
		fmt.Fprintf(opts.Stderr, "tokicv: 布局参数无效: %v\n", err)
		return 1
	}
	store, err := opts.openLayoutStore(true)
	if err != nil {
		fmt.Fprintf(opts.Stderr, "tokicv: %v\n", err)
		return 1
	}

	if request.Target == "" {
		resolved, err := store.ResolveGlobalLayout()
		if err != nil {
			fmt.Fprintf(opts.Stderr, "tokicv: %v\n", err)
			return 1
		}
		profile := request.Patch.Apply(resolved.Layout).Normalized()
		if err := store.SetGlobalDefault(profile); err != nil {
			fmt.Fprintf(opts.Stderr, "tokicv: %v\n", err)
			return 1
		}
		fmt.Fprintln(opts.Stdout, "已更新全局默认排版方案")
		return 0
	}

	target, err := store.ResolveTarget(request.Target)
	if err != nil {
		fmt.Fprintf(opts.Stderr, "tokicv: %v\n", err)
		return 1
	}
	current, err := store.ResolveLayout(target, render.LayoutPatch{})
	if err != nil {
		fmt.Fprintf(opts.Stderr, "tokicv: %v\n", err)
		return 1
	}
	profile := request.Patch.Apply(current).Normalized()
	if err := store.SetTargetProfile(target, profile); err != nil {
		fmt.Fprintf(opts.Stderr, "tokicv: %v\n", err)
		return 1
	}
	fmt.Fprintf(opts.Stdout, "已更新 %s 的排版方案\n", target.DisplayName())
	return 0
}

func resetLayout(opts RunOptions, request LayoutRequest) int {
	store, err := opts.openLayoutStore(false)
	if err != nil {
		fmt.Fprintf(opts.Stderr, "tokicv: %v\n", err)
		return 1
	}
	if request.Target == "" {
		if err := store.ResetGlobalDefault(); err != nil {
			fmt.Fprintf(opts.Stderr, "tokicv: %v\n", err)
			return 1
		}
		fmt.Fprintln(opts.Stdout, "已重置全局默认排版方案")
		return 0
	}
	target, err := store.ResolveTarget(request.Target)
	if err != nil {
		fmt.Fprintf(opts.Stderr, "tokicv: %v\n", err)
		return 1
	}
	if err := store.ResetTargetProfile(target); err != nil {
		fmt.Fprintf(opts.Stderr, "tokicv: %v\n", err)
		return 1
	}
	fmt.Fprintf(opts.Stdout, "已重置 %s 的排版方案\n", target.DisplayName())
	return 0
}

func writeLayoutView(opts RunOptions, view LayoutView) {
	name := "全局默认"
	if view.Target != "" {
		name = view.Target
	}
	fmt.Fprintf(opts.Stdout, "排版方案: %s\n", name)
	fmt.Fprintf(opts.Stdout, "来源: %s\n", layoutOriginLabel(view.Origin))
	fmt.Fprintf(opts.Stdout, "字体: %s\n", view.Layout.FontStack)
	fmt.Fprintf(opts.Stdout, "行高: %s\n", formatLayoutNumber(view.Layout.LineHeight))
	fmt.Fprintf(opts.Stdout, "上边距: %smm\n", formatLayoutNumber(view.Layout.MarginTopMM))
	fmt.Fprintf(opts.Stdout, "下边距: %smm\n", formatLayoutNumber(view.Layout.MarginBottomMM))
	fmt.Fprintf(opts.Stdout, "节间距: %spx\n", formatLayoutNumber(view.Layout.SectionGapPX))
	fmt.Fprintf(opts.Stdout, "条目间距: %spx\n", formatLayoutNumber(view.Layout.ItemGapPX))
}

func layoutOriginLabel(origin library.LayoutOrigin) string {
	switch origin {
	case library.LayoutOriginGlobal:
		return "全局默认"
	case library.LayoutOriginDocument:
		return "Library 文档覆盖"
	case library.LayoutOriginPath:
		return "外部路径覆盖"
	default:
		return "内置默认"
	}
}

func formatLayoutNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func runList(opts RunOptions, request ListRequest) int {
	store, err := opts.openLibrary()
	if err != nil {
		fmt.Fprintf(opts.Stderr, "tokicv: %v\n", err)
		return 1
	}
	documents, err := store.List()
	if err != nil {
		fmt.Fprintf(opts.Stderr, "tokicv: %v\n", err)
		return 1
	}
	if request.JSON {
		if err := json.NewEncoder(opts.Stdout).Encode(documents); err != nil {
			fmt.Fprintf(opts.Stderr, "tokicv: 写入 JSON: %v\n", err)
			return 1
		}
		return 0
	}
	if len(documents) == 0 {
		fmt.Fprintln(opts.Stdout, "Library 中还没有 Resume Document")
		return 0
	}

	writer := tabwriter.NewWriter(opts.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "ID\t名称\t更新时间")
	for _, document := range documents {
		fmt.Fprintf(writer, "%s\t%s\t%s\n",
			document.ID, document.Name, document.UpdatedAt.Local().Format("2006-01-02 15:04"))
	}
	if err := writer.Flush(); err != nil {
		fmt.Fprintf(opts.Stderr, "tokicv: 写入列表: %v\n", err)
		return 1
	}
	return 0
}

func runCreate(opts RunOptions, request CreateRequest) int {
	source, err := createSource(opts, request)
	if err != nil {
		fmt.Fprintf(opts.Stderr, "tokicv: %v\n", err)
		return 1
	}
	store, err := opts.openLibrary()
	if err != nil {
		fmt.Fprintf(opts.Stderr, "tokicv: %v\n", err)
		return 1
	}
	document, err := store.Create(request.Name, source)
	if err != nil {
		fmt.Fprintf(opts.Stderr, "tokicv: %v\n", err)
		return 1
	}
	fmt.Fprintf(opts.Stdout, "已创建 %s: %s\n", document.Name, document.ID)
	return 0
}

func createSource(opts RunOptions, request CreateRequest) ([]byte, error) {
	if request.From != "" {
		if !strings.EqualFold(filepath.Ext(request.From), ".md") {
			return nil, fmt.Errorf("--from 需要 .md 文件: %s", request.From)
		}
		source, err := os.ReadFile(request.From)
		if err != nil {
			return nil, fmt.Errorf("读取 Resume Document %q: %w", request.From, err)
		}
		return source, nil
	}
	if request.Template {
		return []byte(ResumeTemplate), nil
	}
	if opts.StdinRedirected {
		source, err := io.ReadAll(opts.Stdin)
		if err != nil {
			return nil, fmt.Errorf("读取标准输入: %w", err)
		}
		return source, nil
	}
	return nil, nil
}

func runShow(opts RunOptions, request ShowRequest) int {
	store, err := opts.openLibrary()
	if err != nil {
		fmt.Fprintf(opts.Stderr, "tokicv: %v\n", err)
		return 1
	}
	_, source, err := store.Show(request.Target)
	if err != nil {
		fmt.Fprintf(opts.Stderr, "tokicv: %v\n", err)
		return 1
	}
	if _, err := opts.Stdout.Write(source); err != nil {
		fmt.Fprintf(opts.Stderr, "tokicv: 写入标准输出: %v\n", err)
		return 1
	}
	return 0
}

func runImport(opts RunOptions, request ImportRequest) int {
	store, err := opts.openLibrary()
	if err != nil {
		fmt.Fprintf(opts.Stderr, "tokicv: %v\n", err)
		return 1
	}
	document, err := store.Import(request.Path, request.Name)
	if err != nil {
		fmt.Fprintf(opts.Stderr, "tokicv: %v\n", err)
		return 1
	}
	fmt.Fprintf(opts.Stdout, "已导入 %s: %s\n", document.Name, document.ID)
	return 0
}

func runDelete(opts RunOptions, request DeleteRequest) int {
	store, err := opts.openLibrary()
	if err != nil {
		fmt.Fprintf(opts.Stderr, "tokicv: %v\n", err)
		return 1
	}
	document, err := store.Delete(request.Target)
	if err != nil {
		fmt.Fprintf(opts.Stderr, "tokicv: %v\n", err)
		return 1
	}
	fmt.Fprintf(opts.Stdout, "已删除 %s: %s\n", document.Name, document.ID)
	return 0
}

// ResumeTemplate is the starter Resume Document used by create --template.
const ResumeTemplate = `# 姓名

icon:phone 手机号　icon:email 邮箱

## 教育背景

**学校**　专业 · 学历
2020.09 – 2024.06

## 工作经历

- 公司　职位　2020.09 – 2024.06
  - 描述工作内容与可量化成果

## 技能

- 技能一、技能二
`

// Parse parses a CLI invocation without executing it.
func Parse(args []string) (Command, error) {
	if len(args) == 0 {
		return Command{Kind: CommandGUI}, nil
	}

	switch args[0] {
	case "--help", "-h":
		return Command{Kind: CommandHelp}, nil
	case "--version", "-v":
		return Command{Kind: CommandVersion}, nil
	case "--gui":
		return parseGUI(args[1:])
	case "export":
		return parseExport(args[1:])
	case "check":
		return parseCheck(args[1:])
	case "list":
		return parseList(args[1:])
	case "create":
		return parseCreate(args[1:])
	case "show":
		return parseShow(args[1:])
	case "import":
		return parseImport(args[1:])
	case "delete":
		return parseDelete(args[1:])
	case "layout":
		return parseLayout(args[1:])
	default:
		if value, ok := strings.CutPrefix(args[0], "--gui="); ok {
			return parseGUI([]string{value})
		}
		if strings.HasPrefix(args[0], "-") {
			return Command{}, fmt.Errorf("未知选项 %s", args[0])
		}
		if isMarkdownPath(args[0]) {
			if len(args) > 1 {
				return Command{}, errors.New("只能打开一个 Markdown 文件")
			}
			return Command{Kind: CommandGUI, Path: args[0]}, nil
		}
		return Command{}, fmt.Errorf("未知命令 %s", args[0])
	}
}

func parseGUI(args []string) (Command, error) {
	command := Command{Kind: CommandGUI}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			return Command{}, fmt.Errorf("未知选项 %s", arg)
		}
		if command.Path != "" {
			return Command{}, errors.New("只能打开一个 Markdown 文件")
		}
		if !isMarkdownPath(arg) {
			return Command{}, fmt.Errorf("桌面编辑需要 .md 文件: %s", arg)
		}
		command.Path = arg
	}
	return command, nil
}

func isMarkdownPath(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".md")
}

func parseList(args []string) (Command, error) {
	request := ListRequest{}
	for _, arg := range args {
		switch arg {
		case "--help", "-h":
			return Command{Kind: CommandHelp}, nil
		case "--json":
			request.JSON = true
		default:
			if strings.HasPrefix(arg, "-") {
				return Command{}, fmt.Errorf("list 未知选项 %s", arg)
			}
			return Command{}, fmt.Errorf("list 不接受位置参数 %s", arg)
		}
	}
	return Command{Kind: CommandList, List: request}, nil
}

func parseCreate(args []string) (Command, error) {
	request := CreateRequest{}
	hasName := false
	fromSet := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h":
			return Command{Kind: CommandHelp}, nil
		case arg == "--from":
			if i+1 >= len(args) {
				return Command{}, errors.New("--from 缺少 Markdown 路径")
			}
			i++
			request.From = args[i]
			fromSet = true
		case strings.HasPrefix(arg, "--from="):
			request.From = strings.TrimPrefix(arg, "--from=")
			fromSet = true
		case arg == "--template":
			request.Template = true
		case strings.HasPrefix(arg, "-"):
			return Command{}, fmt.Errorf("create 未知选项 %s", arg)
		default:
			if hasName {
				return Command{}, errors.New("create 只能指定一个 Resume Document 名称")
			}
			request.Name = arg
			hasName = true
		}
	}

	if !hasName || request.Name == "" {
		return Command{}, errors.New("create 需要一个 Resume Document 名称")
	}
	if fromSet && request.From == "" {
		return Command{}, errors.New("--from 缺少 Markdown 路径")
	}
	if request.From != "" && request.Template {
		return Command{}, errors.New("--from 与 --template 不能同时使用")
	}
	return Command{Kind: CommandCreate, Create: request}, nil
}

func parseShow(args []string) (Command, error) {
	target, help, err := parseSingleTarget("show", args)
	if err != nil {
		return Command{}, err
	}
	if help {
		return Command{Kind: CommandHelp}, nil
	}
	return Command{Kind: CommandShow, Show: ShowRequest{Target: target}}, nil
}

func parseImport(args []string) (Command, error) {
	request := ImportRequest{}
	hasPath := false
	nameSet := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h":
			return Command{Kind: CommandHelp}, nil
		case arg == "--name":
			if i+1 >= len(args) {
				return Command{}, errors.New("--name 缺少名称")
			}
			i++
			request.Name = args[i]
			nameSet = true
		case strings.HasPrefix(arg, "--name="):
			request.Name = strings.TrimPrefix(arg, "--name=")
			nameSet = true
		case strings.HasPrefix(arg, "-"):
			return Command{}, fmt.Errorf("import 未知选项 %s", arg)
		default:
			if hasPath {
				return Command{}, errors.New("import 只能指定一个 Markdown 路径")
			}
			request.Path = arg
			hasPath = true
		}
	}

	if !hasPath || request.Path == "" {
		return Command{}, errors.New("import 需要一个 Markdown 路径")
	}
	if nameSet && request.Name == "" {
		return Command{}, errors.New("--name 缺少名称")
	}
	return Command{Kind: CommandImport, Import: request}, nil
}

func parseDelete(args []string) (Command, error) {
	target, help, err := parseSingleTarget("delete", args)
	if err != nil {
		return Command{}, err
	}
	if help {
		return Command{Kind: CommandHelp}, nil
	}
	return Command{Kind: CommandDelete, Delete: DeleteRequest{Target: target}}, nil
}

func parseSingleTarget(command string, args []string) (string, bool, error) {
	target := ""
	hasTarget := false
	for _, arg := range args {
		switch {
		case arg == "--help" || arg == "-h":
			return "", true, nil
		case strings.HasPrefix(arg, "-"):
			return "", false, fmt.Errorf("%s 未知选项 %s", command, arg)
		default:
			if hasTarget {
				return "", false, fmt.Errorf("%s 只能指定一个 Resume Document 目标", command)
			}
			target = arg
			hasTarget = true
		}
	}
	if !hasTarget || target == "" {
		return "", false, fmt.Errorf("%s 需要一个 Resume Document 目标", command)
	}
	return target, false, nil
}

func parseLayout(args []string) (Command, error) {
	if len(args) == 0 {
		return Command{}, errors.New("layout 需要 show、set 或 reset 操作")
	}
	if args[0] == "--help" || args[0] == "-h" {
		return Command{Kind: CommandHelp}, nil
	}
	switch args[0] {
	case "show":
		return parseLayoutShow(args[1:])
	case "set":
		return parseLayoutSet(args[1:])
	case "reset":
		return parseLayoutReset(args[1:])
	default:
		return Command{}, fmt.Errorf("未知 layout 操作 %s", args[0])
	}
}

func parseLayoutShow(args []string) (Command, error) {
	request := LayoutRequest{Action: LayoutShow}
	for _, arg := range args {
		switch {
		case arg == "--help" || arg == "-h":
			return Command{Kind: CommandHelp}, nil
		case arg == "--json":
			request.JSON = true
		case strings.HasPrefix(arg, "-"):
			return Command{}, fmt.Errorf("layout show 未知选项 %s", arg)
		default:
			if request.Target != "" {
				return Command{}, errors.New("layout show 只能指定一个目标")
			}
			request.Target = arg
		}
	}
	return Command{Kind: CommandLayout, Layout: request}, nil
}

func parseLayoutSet(args []string) (Command, error) {
	request := LayoutRequest{Action: LayoutSet}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h":
			return Command{Kind: CommandHelp}, nil
		case isLayoutOption(arg):
			if _, err := parseLayoutOption(args, &i, &request.Patch); err != nil {
				return Command{}, err
			}
		case strings.HasPrefix(arg, "-"):
			return Command{}, fmt.Errorf("layout set 未知选项 %s", arg)
		default:
			if request.Target != "" {
				return Command{}, errors.New("layout set 只能指定一个目标")
			}
			request.Target = arg
		}
	}
	if !request.Patch.Any() {
		return Command{}, errors.New("layout set 至少需要一个布局参数")
	}
	return Command{Kind: CommandLayout, Layout: request}, nil
}

func parseLayoutReset(args []string) (Command, error) {
	request := LayoutRequest{Action: LayoutReset}
	for _, arg := range args {
		switch {
		case arg == "--help" || arg == "-h":
			return Command{Kind: CommandHelp}, nil
		case strings.HasPrefix(arg, "-"):
			return Command{}, fmt.Errorf("layout reset 未知选项 %s", arg)
		default:
			if request.Target != "" {
				return Command{}, errors.New("layout reset 只能指定一个目标")
			}
			request.Target = arg
		}
	}
	return Command{Kind: CommandLayout, Layout: request}, nil
}

func isLayoutOption(arg string) bool {
	for _, name := range []string{
		"--font",
		"--line-height",
		"--margin-v",
		"--margin-top",
		"--margin-bottom",
		"--section-gap",
		"--item-gap",
	} {
		if arg == name || strings.HasPrefix(arg, name+"=") {
			return true
		}
	}
	return false
}

func parseLayoutOption(args []string, index *int, patch *render.LayoutPatch) (bool, error) {
	arg := args[*index]
	switch {
	case isOption(arg, "--font"):
		value, next, err := optionValue(args, *index, "--font")
		if err != nil {
			return true, err
		}
		font, ok := render.ParseFontStack(value)
		if !ok {
			return true, fmt.Errorf("--font 无效: %s", value)
		}
		*index = next
		patch.FontStack = &font
		return true, nil
	case isOption(arg, "--line-height"):
		value, next, err := optionValue(args, *index, "--line-height")
		if err != nil {
			return true, err
		}
		lineHeight, err := parseLayoutFloat("--line-height", value, render.MinLineHeight, render.MaxLineHeight)
		if err != nil {
			return true, err
		}
		*index = next
		patch.LineHeight = &lineHeight
		return true, nil
	case isOption(arg, "--margin-v"):
		value, next, err := optionValue(args, *index, "--margin-v")
		if err != nil {
			return true, err
		}
		margin, err := parseLayoutFloat("--margin-v", value, render.MinMarginMM, render.MaxMarginMM)
		if err != nil {
			return true, err
		}
		*index = next
		top, bottom := margin, margin
		patch.MarginTopMM = &top
		patch.MarginBottomMM = &bottom
		return true, nil
	case isOption(arg, "--margin-top"):
		value, next, err := optionValue(args, *index, "--margin-top")
		if err != nil {
			return true, err
		}
		margin, err := parseLayoutFloat("--margin-top", value, render.MinMarginMM, render.MaxMarginMM)
		if err != nil {
			return true, err
		}
		*index = next
		patch.MarginTopMM = &margin
		return true, nil
	case isOption(arg, "--margin-bottom"):
		value, next, err := optionValue(args, *index, "--margin-bottom")
		if err != nil {
			return true, err
		}
		margin, err := parseLayoutFloat("--margin-bottom", value, render.MinMarginMM, render.MaxMarginMM)
		if err != nil {
			return true, err
		}
		*index = next
		patch.MarginBottomMM = &margin
		return true, nil
	case isOption(arg, "--section-gap"):
		value, next, err := optionValue(args, *index, "--section-gap")
		if err != nil {
			return true, err
		}
		gap, err := parseLayoutFloat("--section-gap", value, render.MinSectionGapPX, render.MaxSectionGapPX)
		if err != nil {
			return true, err
		}
		*index = next
		patch.SectionGapPX = &gap
		return true, nil
	case isOption(arg, "--item-gap"):
		value, next, err := optionValue(args, *index, "--item-gap")
		if err != nil {
			return true, err
		}
		gap, err := parseLayoutFloat("--item-gap", value, render.MinItemGapPX, render.MaxItemGapPX)
		if err != nil {
			return true, err
		}
		*index = next
		patch.ItemGapPX = &gap
		return true, nil
	default:
		return false, nil
	}
}

func isOption(arg, name string) bool {
	return arg == name || strings.HasPrefix(arg, name+"=")
}

func optionValue(args []string, index int, name string) (string, int, error) {
	arg := args[index]
	if arg == name {
		if index+1 >= len(args) {
			return "", index, fmt.Errorf("%s 缺少值", name)
		}
		return args[index+1], index + 1, nil
	}
	return strings.TrimPrefix(arg, name+"="), index, nil
}

func parseLayoutFloat(name, raw string, min, max float64) (float64, error) {
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("%s 无效: %s", name, raw)
	}
	if value < min || value > max {
		return 0, fmt.Errorf("%s 必须在 %s 到 %s 之间", name, formatLayoutBound(min), formatLayoutBound(max))
	}
	return value, nil
}

func formatLayoutBound(value float64) string {
	if value == math.Trunc(value) {
		return formatLayoutNumber(value)
	}
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func parseCheck(args []string) (Command, error) {
	request := CheckRequest{Timeout: DefaultTimeout, Font: FontClassic}
	hasInput := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h":
			return Command{Kind: CommandHelp}, nil
		case arg == "--strict-one-page":
			request.StrictOnePage = true
		case arg == "--fit-one-page":
			request.FitOnePage = true
		case arg == "--json":
			request.JSON = true
		case arg == "--timeout":
			if i+1 >= len(args) {
				return Command{}, errors.New("--timeout 缺少时长")
			}
			i++
			timeout, err := time.ParseDuration(args[i])
			if err != nil {
				return Command{}, fmt.Errorf("--timeout 无效: %w", err)
			}
			request.Timeout = timeout
		case strings.HasPrefix(arg, "--timeout="):
			timeout, err := time.ParseDuration(strings.TrimPrefix(arg, "--timeout="))
			if err != nil {
				return Command{}, fmt.Errorf("--timeout 无效: %w", err)
			}
			request.Timeout = timeout
		case isLayoutOption(arg):
			_, err := parseLayoutOption(args, &i, &request.Layout)
			if err != nil {
				return Command{}, err
			}
			if request.Layout.FontStack != nil {
				request.Font = *request.Layout.FontStack
			}
		case strings.HasPrefix(arg, "-"):
			return Command{}, fmt.Errorf("未知选项 %s", arg)
		default:
			if hasInput {
				return Command{}, errors.New("check 只能指定一个 Markdown 输入路径")
			}
			request.Input = arg
			hasInput = true
		}
	}

	if !hasInput {
		return Command{}, errors.New("check 需要一个 Markdown 输入路径")
	}
	if request.Timeout <= 0 {
		return Command{}, errors.New("--timeout 必须大于零")
	}
	return Command{Kind: CommandCheck, Check: request}, nil
}

func parseExport(args []string) (Command, error) {
	request := ExportRequest{Timeout: DefaultTimeout, Font: FontClassic}
	hasInput := false
	pdfSet := false
	pngSet := false
	mdSet := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h":
			return Command{Kind: CommandHelp}, nil
		case arg == "--fit-one-page":
			request.FitOnePage = true
		case arg == "--pdf":
			if i+1 >= len(args) {
				return Command{}, errors.New("--pdf 缺少输出路径")
			}
			i++
			request.Output = args[i]
			pdfSet = true
		case strings.HasPrefix(arg, "--pdf="):
			request.Output = strings.TrimPrefix(arg, "--pdf=")
			pdfSet = true
		case arg == "--png":
			if i+1 >= len(args) {
				return Command{}, errors.New("--png 缺少输出路径")
			}
			i++
			request.PNG = args[i]
			pngSet = true
		case strings.HasPrefix(arg, "--png="):
			request.PNG = strings.TrimPrefix(arg, "--png=")
			pngSet = true
		case arg == "--md":
			if i+1 >= len(args) {
				return Command{}, errors.New("--md 缺少输出路径")
			}
			i++
			request.Markdown = args[i]
			mdSet = true
		case strings.HasPrefix(arg, "--md="):
			request.Markdown = strings.TrimPrefix(arg, "--md=")
			mdSet = true
		case arg == "--timeout":
			if i+1 >= len(args) {
				return Command{}, errors.New("--timeout 缺少时长")
			}
			i++
			timeout, err := time.ParseDuration(args[i])
			if err != nil {
				return Command{}, fmt.Errorf("--timeout 无效: %w", err)
			}
			request.Timeout = timeout
		case strings.HasPrefix(arg, "--timeout="):
			timeout, err := time.ParseDuration(strings.TrimPrefix(arg, "--timeout="))
			if err != nil {
				return Command{}, fmt.Errorf("--timeout 无效: %w", err)
			}
			request.Timeout = timeout
		case isLayoutOption(arg):
			_, err := parseLayoutOption(args, &i, &request.Layout)
			if err != nil {
				return Command{}, err
			}
			if request.Layout.FontStack != nil {
				request.Font = *request.Layout.FontStack
			}
		case strings.HasPrefix(arg, "-"):
			return Command{}, fmt.Errorf("未知选项 %s", arg)
		default:
			if hasInput {
				return Command{}, errors.New("export 只能指定一个 Markdown 输入路径")
			}
			request.Input = arg
			hasInput = true
		}
	}

	if !hasInput {
		return Command{}, errors.New("export 需要一个 Markdown 输入路径")
	}
	if pdfSet && request.Output == "" {
		return Command{}, errors.New("--pdf 缺少输出路径")
	}
	if pngSet && request.PNG == "" {
		return Command{}, errors.New("--png 缺少输出路径")
	}
	if mdSet && request.Markdown == "" {
		return Command{}, errors.New("--md 缺少输出路径")
	}
	if !pdfSet && !pngSet && !mdSet {
		return Command{}, errors.New("必须使用 --md、--pdf 或 --png 指定输出路径")
	}
	if request.Timeout <= 0 {
		return Command{}, errors.New("--timeout 必须大于零")
	}
	return Command{Kind: CommandExport, Export: request}, nil
}

const usageText = `用法:
  tokicv [<path.md>]
  tokicv --gui [<path.md>]
  tokicv list [--json]
  tokicv create <name> [--from <path.md> | --template]
  tokicv show <target>
  tokicv import <path.md> [--name <name>]
  tokicv delete <target>
  tokicv layout show [<target>] [--json]
  tokicv layout set [<target>] [--font <stack>] [--line-height <ratio>] [--margin-v <mm>] [--margin-top <mm>] [--margin-bottom <mm>] [--section-gap <px>] [--item-gap <px>]
  tokicv layout reset [<target>]
  tokicv export <target> --md <out.md> [--pdf <out.pdf>] [--png <dest>] [--fit-one-page] [<layout flags>] [--timeout <duration>]
  tokicv export <target> --pdf <out.pdf> [--md <out.md>] [--png <dest>] [--fit-one-page] [<layout flags>] [--timeout <duration>]
  tokicv export <target> --png <dest> [--md <out.md>] [--pdf <out.pdf>] [--fit-one-page] [<layout flags>] [--timeout <duration>]
  tokicv check <target> [--strict-one-page] [--fit-one-page] [--json] [<layout flags>] [--timeout <duration>]
  tokicv --version
  tokicv --help

命令:
  --gui               打开桌面写作工作区
  list    列出本地 Library 中的 Resume Document
  create  创建 Resume Document
  show    输出 Resume Document 的原始 Markdown
  import  将外部 Markdown 导入本地 Library
  delete  删除本地 Library 中的 Resume Document
  layout  查看、更新或重置全局与单目标排版方案
  export  将 Resume Document 导出为 Markdown、A4 PDF 或 PNG Artifact
  check   检查 Resume Document 的精确 Page Fit

目标:
  <target> 可以是 Library 文档 ID、唯一名称，或相对/绝对 .md 路径

选项:
  --json              以 JSON 输出 Library 列表、Page Fit 或排版方案
  --from <path>       使用外部 Markdown 创建 Resume Document
  --template          使用内置模板创建 Resume Document
  --name <name>       指定导入后的 Resume Document 名称
  --md <path>         输出原始 Markdown Artifact
  --pdf <path>        输出 PDF 路径
  --png <dest>        输出 PNG 路径或每页 PNG 目录
  --strict-one-page   多页时以退出码 2 报告 Page Fit 溢出
  --fit-one-page      按节/条目间距、行高、上下页边距的顺序智能收紧到单页
  --font <stack>      字体栈: classic, noto, serif, system
  --line-height <ratio>
                      无单位行高倍率，范围 1.20 到 2.20
  --margin-v <mm>     同时设置上下页面边距，范围 7.5 到 30mm
  --margin-top <mm>   上页面边距，范围 7.5 到 30mm
  --margin-bottom <mm>
                      下页面边距，范围 7.5 到 30mm
  --section-gap <px>  节间距，范围 5 到 40px
  --item-gap <px>     条目间距，范围 1.5 到 20px
  --timeout <duration>
                      渲染超时，默认 30s
  -h, --help          显示帮助
  -v, --version       显示版本`
