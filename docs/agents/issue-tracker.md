# 本地 Markdown 任务跟踪

## 文件约定

- 每项功能使用 `.scratch/<feature-slug>/` 目录
- 规格存放于 `spec.md`
- 每张任务单独存放于 `issues/<NN>-<slug>.md`，编号从 `01` 开始
- 任务状态通过文件顶部的 `Status:` 行记录
- 评论与讨论追加到文件末尾的 `## Comments` 下

## 技能操作

- 发布任务：在对应功能目录中创建文件
- 获取任务：读取用户指定路径或编号对应的文件

## Wayfinder

- 工作地图：`.scratch/<effort>/map.md`，记录 Notes、Decisions-so-far 和 Fog
- 子任务：`issues/NN-<slug>.md`，正文记录问题
- `Type:` 使用 research、prototype、grilling 或 task
- `Blocked by: NN, NN` 记录依赖，所列任务全部 resolved 后解除阻塞
- 从可执行且尚待认领的任务中，优先选择编号最小的任务
- 认领时先写入 `Status: claimed`
- 完成时在 `## Answer` 下追加答案，写入 `Status: resolved`，并将摘要与链接追加到地图的 Decisions-so-far
