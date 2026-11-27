# TokiCV 需求交付

## GitHub 线上交付

仓库所有者提交 GitHub Issue 时，在 GitHub 托管的 Ubuntu 运行器启动交付。先由 Codex 实现，再用独立的 Codex 运行审查候选提交，通过后执行完整验证和打包。首次交付回填原 Issue；后续返工回填触发评论所在的 Issue 或 PR。用户负责审查和合并 PR

- 提新需求直接新建 Issue；带 `discussion` 或 `no-factory` 标签的 Issue 不自动开工，之后可用 `/tokicv` 启动。原 Issue 与对应 PR 的 Conversation 等效，所有者用 `/tokicv` 开头发微调、决定、空命令刷新或重试。普通评论只作讨论，不进入模型上下文；`startsWith` 在 runner 启动前过滤，因此 `/tokicv` 必须位于评论第一行，不能加前导空白，大小写不敏感，后接结尾、空白或换行
- 父工作流保留 `TokiCV delivery` 名称及 Issue、评论、手动和测试推送事件。只读 intake 先从 Issue 或工厂 PR 分支解析 canonical Issue ID，再把该 ID 传给本地可复用交付工作流。可复用工作流用 `tokicv-delivery-<issue>` 的 workflow-level `concurrency` 串行同一工单的所有阶段，`cancel-in-progress: false` 与 `queue: max` 保留最多 100 条待处理评论；不同工单并行。所有入口共用同一原生锁，不使用运行列表轮询
- 决策上下文按时间合并 Issue 与关联 PR 评论，只含仓库所有者的合法 `/tokicv` 评论和真实 `github-actions[bot]` 的工厂 marker。问题和回复原文一并交给实现器，不解析固定选项字母或回复模板
- 状态 marker 以 canonical Issue 编号为真相源，保留原有 `version: 1`，增加 `diff_base`、`trigger`、`consumed_comment_ids`、回填目标和候选发布信息。连续评论被同一次交付吸收后，后续由其中任一评论触发的运行按已消费 ID 和 PR HEAD 幂等退出
- 默认分支推进不阻塞在途候选。可信规则始终从本次基线读取；完整候选 diff 从 `git merge-base <base> <candidate>` 或已记录的分叉点计算。已推送分支可直接追加提交，未推送候选从 7 天 artifact 恢复；只有恢复基线不再是从当前基线的祖先、artifact 损坏或历史失联时才回填恢复失败
- 审查阻塞时在候选之上追加修复；空 `/tokicv` 可沿用同一 SHA 重新审查、打包并刷新安装包，待决策时空命令只提示需要明确决定。审查、验证、打包和回填使用同一候选 SHA；推送失败或评论失败保留可重入的触发、运行和候选证据
- 分支只在审查通过和完整打包成功后推送。已有 OPEN PR 直接复用，不改 Draft/Ready；交付以该 PR 和含相同候选 SHA 的本次回填共同确认。旧工单的 `github-actions` 标准完成文案加同一 PR URL 继续识别为已完成。PR 已合并时提示新开 Issue，旧 PR 已关闭时提示回原 Issue 或新开 Issue
- `/tokicv 从当前基线重新实现` 只在仍有 OPEN PR、所有者明确命令且审查与完整打包全部通过后更新原分支。更新使用 `--force-with-lease=refs/heads/factory/issue-N:<旧远端 HEAD>`，门禁失败或 lease 不匹配时旧分支保持不变
- 审查阻塞、审查超时或打包失败时，工厂 marker 回填当前候选 SHA、基线、差异基线、运行 ID、运行 attempt、候选 artifact 名称和失败分类。候选 artifact 保留 7 天；`gh run download` 只使用 job 已有的 `actions: read`
- 交付复盘按 canonical Issue 归并 `TokiCV issue #N` 与 `TokiCV PR #P`，PR 标题通过只读 pull request API 解析工厂分支；复盘收集只增加 `pull-requests: read`，同工单两端失败可归入同一 job/step
- 实现和审查都固定使用 OpenCode Go 的 DeepSeek V4.1 Flash `max`。GitHub Actions 的 `OPENCODE_GO_API_KEY` 提供模型凭据。额度用完时，运行结束并保留日志，恢复后由仓库所有者重跑。可在 OpenCode Go 关闭 Extra Usage，以订阅额度作为消费上限
- 极端复杂的需求先由 Codex 在工单写清待设计的问题。仓库所有者决定是否用有可用额度的官方 Codex 或 Google Gemini 产品做设计，必要时用同一产品做一次验收。把设计结论用 `/tokicv` 回复到工单，后续实现和独立审查仍走 DeepSeek。官方订阅的授权与运行在各自的产品里完成
- 首次接入时，向 `factory/tokicv-delivery` 分支推送可试跑模型和打包，产物位于 Actions 的 Artifacts。正式启用后，仓库所有者也可在 Actions 手动运行，工单编号填 `0` 只试跑模型接入和现有提交的打包。工单交付需要仓库允许 GitHub Actions 创建草稿 PR

## 架构预检与审查

实现器在修改文件前读取 `GLOSSARY.md`、`AGENTS.md`、`docs/agents/domain.md` 和 `docs/adr/` 中与工单相关的 ADR。ADR 数量较少时可先全部浏览再按相关性采用。按工单涉及的行为变动、难复现故障、模块设计或术语/ADR 变更，读取 `.agents/skills/README.md` 指向的 `tdd`、`diagnosing-bugs`、`codebase-design`、`domain-modeling` 相关章节；涉及 MyGo UI 或渲染时，显式读取 `.agents/skills/mygo-maintenance/SKILL.md`。技能只从仓库内读取，由 Actions 自动执行，上游的用户确认、并行子代理、写文件和创建 Issue/PR 步骤由工单时间线与工厂流程替代。实现前先搜索并复用已有模块，保持职责边界。需求、验收条件或方案与 ADR、术语冲突，或需要改变既有架构决策时，实现器返回 `needs_decision` 作为重大决策，引用具体规则、冲突影响和选项，不能自行绕过

独立审查仍使用 OpenCode Go 的 DeepSeek V4.1 Flash `max`，在无 shell 的只读环境执行，并作为打包前门禁。审查输入包含可信基线的全部 ADR、术语和仓库规则、`code-review` 技能中关于 spec 来源、standards 来源和 smell baseline 的原文节选，以及候选 diff；规则按相关性判断。上游的询问用户、并行子代理和报告生成步骤由既有审查 schema 替代。阻塞项必须对应具体规则违规、可验证的重复实现或职责错放，给出候选文件、行为和修复要求。没有具体证据时不能因风格偏好、推测性未来需求或可另做的重构返回 `blocked`

## 架构巡检

架构巡检每周运行一次，仓库所有者也可在 Actions 手动运行。巡检使用 DeepSeek V4.1 Flash `max`，只读且无 shell，读取最近七天默认分支上的有界提交证据、现有 ADR 和术语规则，以及 `improve-codebase-architecture` 的 Explore、`codebase-design` 的 Glossary/Depth/Seam/Deletion test 和 `domain-modeling` 的术语一致性原文节选。上游的交互、子代理、HTML 报告和写入步骤由巡检 schema 替代。没有近期非空变更时跳过模型

巡检最多提出三条有提交证据的架构建议，写入 Actions Summary 和保留 30 天的 artifact。建议由仓库所有者决定是否按正常交付流程处理；巡检不创建 Issue 或 PR、不修改源代码，也不改变发布权限

## 本地交付

1. **接收需求**。从当前 TokiCV 对话或已有的本地 Markdown 工单获取需求，明确可观察的验收条件。新增功能取得仓库改动授权后，按 [本地任务跟踪约定](issue-tracker.md) 将规格和任务建档。常规实现细节自行决定。若选择会改变默认行为、已有数据的兼容性或交付范围，且需求和已有决策都无法确定方向，列出具体选项与影响，将本次运行停在该决策处。用户答复后以同一条需求继续
2. **实现**。取得仓库改动授权后，以目标分支的当前提交为基线，为这条需求使用一个独立工作树和分支。复用该需求已有的分支与工作树，同一时间只执行一次。实现验收条件，在该分支提交改动。已有本地工单按 `issue-tracker.md` 记录状态
3. **独立审查**。将需求原文、验收条件、工作树路径及基线和候选提交交给独立审查智能体，要求其按仓库规范及需求审查差异并给出可操作的阻塞项。简单审查使用 Codex 的 DeepSeek V4.1 Flash，推理强度为 `max`。修复后审查新提交，最多自动修复两轮。仍有阻塞时，把具体问题和候选提交交回用户决定
4. **验证与打包**。独立审查通过后，在候选提交的干净工作树执行 `scripts/deliver.sh <主工作目录>/build/delivery`。该命令执行完整验证、构建 Linux amd64 发行包，并试运行构建产物的版本命令。验证或构建失败时用所保留的日志定位问题；修复涉及源码时，提交后重新审查该提交。重复交付同一提交时，若存有对应提交的 `commit.txt`、安装包和验证报告，复用这次成功交付
5. **交付**。在原对话给出可安装包、验证报告及提交 SHA，有本地工单时也回填这些结果。合入目标分支或发布按用户授权执行

## 交付复盘

GitHub 交付结束后，复盘任务在只读且无 shell 的环境中检查最近 30 条交付记录。同一工单的同一 job 和 step 第二次失败时，用 DeepSeek V4.1 Flash `max` 对照两次运行，并读取可信默认分支 `retro` 技能的检查分类、Implementation vs Review 和 Files 原文节选；上游的交互、会话日志检索和写入步骤由结构化证据与既有 schema 替代。仓库所有者也可手动指定两条默认分支的工单失败记录。报告在 Actions 的 **TokiCV delivery retro** 运行摘要和报告制品中，保留 14 天

报告附两次运行链接。证据足够时只提出一项改进，写清拟改文件和验收条件；证据不足时记录缺少什么。仓库所有者决定是否把建议作为新需求交付。使用现有候选制品恢复的窗口为 7 天，安装包和验证报告保留 14 天

本地交付遇到同类问题复发时，可对照两次验证报告、日志或审查结论，在原对话和已有工单记录改进建议
