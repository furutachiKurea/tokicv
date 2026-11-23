# AGENTS.md

## 约定

- git commit message 使用 Linux kernel 风格
- 禁止在 UI 中使用 emoji，而是应该使用 icon 替代

## Agent skills

### Issue tracker

任务与规格使用本地 Markdown 跟踪，读写前查阅 `docs/agents/issue-tracker.md`

### Domain docs

领域文档采用 single-context 布局，探索代码前查阅 `docs/agents/domain.md`

### Verification

统一验证入口为 `scripts/verify.sh fast|graphics|full`；门禁、报告、产物与诊断日志生命周期见 `docs/verification.md`

### 需求交付

收到 TokiCV 实现需求时，默认按 `docs/agents/delivery.md` 推进到审查、验证及 Linux 安装包。用户指定交付范围时，以其要求为准
