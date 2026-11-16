package library

import (
	"bytes"
	"errors"
	"fmt"
	"time"
)

// UpdateContent atomically replaces a Resume Document's exact Markdown bytes
// and refreshes its UpdatedAt timestamp.
func (s *Store) UpdateContent(id string, source []byte) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	documents, err := s.loadLocked()
	if err != nil {
		return Document{}, err
	}
	index := -1
	for i := range documents {
		if documents[i].ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return Document{}, fmt.Errorf("%w: %q", ErrTargetNotFound, id)
	}

	previous, err := s.readContent(id)
	if err != nil {
		return Document{}, err
	}
	if err := s.writeContentLocked(id, source); err != nil {
		return Document{}, err
	}

	now := time.Now().UTC()
	if !now.After(documents[index].UpdatedAt) {
		now = documents[index].UpdatedAt.Add(time.Nanosecond)
	}
	documents[index].UpdatedAt = now
	if err := s.saveLocked(documents); err != nil {
		if bytes.Equal(previous, source) {
			return Document{}, err
		}
		if restoreErr := s.writeContentLocked(id, previous); restoreErr != nil {
			return Document{}, errors.Join(
				err,
				fmt.Errorf("恢复 Resume Document %q 的原有内容: %w", id, restoreErr),
			)
		}
		return Document{}, err
	}
	return documents[index], nil
}

// StarterMarkdown returns the built-in Chinese software-engineer Resume
// Document used by the desktop onboarding flow.
func StarterMarkdown() string {
	return starterMarkdown
}

const starterMarkdown = `# 姓名

` + "`标签 A`" + ` ` + "`标签 B`" + ` ` + "`标签 C`" + `

::: left
icon:email 邮箱字段
icon:github 链接字段
:::

::: right
icon:blog 博客字段
icon:info 信息字段
:::

## 个人简介

此处为个人简介占位段落，用于展示正文样式与段落间距

## 工作经历

### 组织名称｜职位名称

**起止时间字段**

- **条目名称**｜` + "`标签 A`" + ` ` + "`标签 B`" + `
  - 一级要点占位文本，用于展示列表缩进与续行样式
  - 第二条要点占位文本，用于展示同层条目的垂直间距
- **条目名称二**
  - 一级要点占位文本，用于展示条目之间的间距

## 项目经历

### 项目名称

` + "`标签 A`" + ` ` + "`标签 B`" + `

- 项目描述占位文本，用于展示正文换行与段落间距
- 第二条项目描述占位文本

## 技能

- **技能类别一**：技能描述占位文本
- **技能类别二**：技能描述占位文本

## 教育经历

**学校名称**｜专业名称｜学历层次

**起止时间字段**
`
