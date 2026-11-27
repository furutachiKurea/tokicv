# 软件工厂技能索引

本目录保存 TokiCV GitHub 软件工厂自动任务使用的工程技能。Actions 的实现、独立审查、交付复盘和架构巡检任务按路径读取这些文件；用户不调用技能，也不需要为技能增加人工步骤。触发、并发、权限、提交、审查、打包和回填仍由 `.github/workflows/` 与 `scripts/` 固定

## 来源

- 上游：`https://github.com/mattpocock/skills`
- 固定修订：`6f7f7831a625f040a8e06a0e7954826b87ce775a`
- 许可：MIT，全文见 `LICENSES/Matt-Pocock-skills-MIT.txt`
- 技能原文、相对资源和 `agents/openai.yaml` 按上游目录保留；工厂只按下表引用章节，不修改原文
- 原文中的向用户提问、人工确认、并行子代理、HTML 报告、Issue/PR 写入等交互步骤，由 Actions 任务、工单时间线和既有 JSON schema 替代
- `improve-codebase-architecture` 与 `retro` 的上游 frontmatter 含 `disable-model-invocation: true`，`agents/openai.yaml` 也保留 `allow_implicit_invocation: false`。工厂不通过 skill 自动发现调用这两个文件，而是在 Actions 提示词中按路径读取指定章节；因此保留上游原文和这些策略

## 自动入口

| Actions 入口 | 工厂任务 | 技能与章节 | 工厂输入 | 工厂输出 |
| --- | --- | --- | --- | --- |
| `delivery.yml` → `implement` | 行为改动实现 | `tdd/SKILL.md`：What a good test is；Seams: where tests go；Anti-patterns；Rules of the loop；`tdd/tests.md`；`tdd/mocking.md` | 工单验收条件、现有公共测试 seam、`GLOSSARY.md`、ADR | 先红后绿的最小实现与回归测试，工作树交回 Actions |
| `delivery.yml` → `implement` | 复现型修复 | `diagnosing-bugs/SKILL.md`：Redact；Phase 1 至 Phase 6；`diagnosing-bugs/scripts/hitl-loop.template.sh` 作为上游资源保留 | 工单复现步骤、失败日志、现有测试/CLI/fixture、ADR 与术语 | 紧反馈循环、最小复现、回归测试与修复；无法无人复现时返回 `needs_decision` |
| `delivery.yml` → `implement` | 复杂模块设计 | `codebase-design/SKILL.md`：Glossary；Deep vs shallow；Principles；Designing for testability；`codebase-design/DEEPENING.md`；`codebase-design/DESIGN-IT-TWICE.md` 作为接口备选设计参考 | 工单范围、已有模块、职责边界、`GLOSSARY.md`、ADR | 深模块接口与 seam 的实现决策，限制在工单范围 |
| `delivery.yml` → `implement` | 术语与 ADR 变更 | `domain-modeling/SKILL.md`：Challenge against the glossary；Sharpen fuzzy language；Discuss concrete scenarios；Cross-reference with code；Update GLOSSARY.md inline；Offer ADRs sparingly；`GLOSSARY-FORMAT.md`；`ADR-FORMAT.md` | 工单正文、`/tokicv` 决策时间线、现有 `GLOSSARY.md`、`docs/adr/` | 已有明确决策时更新术语或 ADR；需要新决策时返回 `needs_decision` |
| `delivery.yml` → `review` | 独立代码审查 | `code-review/SKILL.md`：Identify the spec source；Identify the standards sources；Fowler smell baseline | `BASE..CANDIDATE` diff、工单上下文、可信基线 ADR 与仓库规则、本索引 | `review.json` 的 `pass` 或 `blocked` 与具体文件/行为证据 |
| `delivery-retro.yml` → `retro` | 交付失败复盘 | `retro/SKILL.md`（按路径读取）：Steps 第 3 步检查分类；Reference 的 Implementation vs Review；Reference 的 Files | 两次失败运行的 ID、链接、工单号、job/step、现有验收与验证规则 | `proposal.json` 的一项改进、拟改文件、验收条件与两次证据 |
| `architecture-audit.yml` → `audit` | 架构自动巡检 | `improve-codebase-architecture/SKILL.md`（按路径读取）：Explore；`codebase-design/SKILL.md`：Glossary、Depth、Seam、Deletion test；`domain-modeling/SKILL.md`：Challenge against the glossary、Cross-reference with code | 最近七天的有界提交、路径与补丁、ADR、术语、仓库规则 | `proposal.json` 的 `summary`、至多三条带提交 SHA 的建议 |

## 章节约定

### tdd

工厂把工单验收条件和现有公共接口视为预先确认的测试 seam。原文要求向用户确认 seam 的步骤由工单正文和已记录的 `/tokicv` 决策替代；验收条件不足以确定 seam 时，实现任务返回 `needs_decision`。Actions 先创建候选提交，交给独立审查任务，审查和打包成功后再发布候选分支

### diagnosing-bugs

工厂优先把工单复现步骤转成仓库内可无人执行的测试、CLI 或 fixture 循环。原文 Phase 3 向用户展示排序的步骤，改为在实现结果中记录假设排序及其对应的工单、日志、ADR 或术语依据；无法建立紧反馈循环时，在 message 列出已尝试路径和所需证据并返回 `needs_decision`。HITL 模板保留为上游资源，自动任务不等待人工输入

### codebase-design

实现任务使用原文的模块、接口、深度、seam、adapter、leverage、locality 词汇，以及删除测试和接口即测试面的判断。`DEEPENING.md` 用于依赖分类、seam 纪律和替换式测试；`DESIGN-IT-TWICE.md` 的并行子代理步骤只在主会话显式安排时使用，工厂自动实现采用其中的接口比较维度作为检查清单

### domain-modeling

工厂以工单正文和 `/tokicv` 决策时间线替代原文的当面追问。只有仓库所有者已在工单明确决定术语或架构方向时，才按 `GLOSSARY-FORMAT.md` 或 `ADR-FORMAT.md` 写入；需要新决策时返回 `needs_decision`。架构巡检只读使用术语冲突和代码一致性检查，不修改 `GLOSSARY.md` 或 ADR

### code-review

固定点、spec 来源和 standards 来源已由 `scripts/github-delivery.py` 的 `BASE..CANDIDATE`、`issue.json` 与 `review_baseline(base)` 提供。审查使用原文的仓库规则优先、Fowler smell baseline 仅作判断项、跳过工具已强制检查等准则；不启动并行子代理，不生成两轴报告，输出沿用既有 `review.json` schema

### improve-codebase-architecture

巡检使用 Explore 的 YAGNI 范围选择、提交热点、领域术语、ADR 相关性、浅模块与职责泄漏问题清单，以及删除测试。原文的探索子代理由巡检任务在单次只读运行中读取有界证据替代；原文的 HTML 报告和 grilling 由既有架构巡检 schema 替代；巡检不生成 HTML、不向用户提问、不写 Issue 或 PR，建议只引用证据中真实存在的提交 SHA

### retro

复盘使用 Steps 第 3 步的 Navigation、Automated checks、Coding standards、Global AGENTS.md、Tool economy、No-ops、Information access 分类，以及 Reference 的 Implementation vs Review 和 Files 规则。原文第 1 步的 writing-for-agents 调用由仓库现有文档风格和既有 schema 替代；原文的 session 日志搜索与用户汇报由两次失败运行的结构化证据和 Actions Summary 替代；输出沿用既有 `proposal.json` schema，只提出一项最小改进

## 接线位置

- 实现任务在架构预检后读取本索引和 `tdd`、`diagnosing-bugs`、`codebase-design`、`domain-modeling` 的相关章节
- 独立审查任务把本索引加入可信基线规则，并读取 `code-review` 的准则章节
- 交付复盘任务读取本索引和 `retro` 的检查分类
- 架构巡检任务读取本索引、`improve-codebase-architecture` 的 Explore，以及 `codebase-design` 和 `domain-modeling` 的只读检查
