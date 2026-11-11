# 更新日志

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循[语义化版本](https://semver.org/lang/zh-CN/)。

## [0.1.2] - 2026-11-10

### 新增

- 一键更新：下载安装程序并校验 SHA-256 后静默升级，完成后自动重启
- 快捷键：`Enter` 继续会话，`Delete` 移入回收站，批量选择时 `Ctrl A` 全选
- 批量选择支持 Shift 连选；右键已选条目时对全部所选生效
- 安装程序检测 WebView2 Runtime，缺失时可下载安装

### 变更

- “关于”重新排版：名称链接到项目主页，版本号与名称同行，更新操作合为一行，移除底部按钮
- “关于”中的更新只显示版本与操作，不再显示发布说明正文
- 会话列表不再显示模型与大小，详情中保留
- 移除侧边栏底部的数据源按钮
- 批量选择栏只保留复选框与数量
- 图标库换为 `@lucide/vue`

### 修复

- `↑` `↓` 按列表显示顺序切换会话，跳过已折叠的项目

## [0.1.1] - 2026-11-10

### 新增

- 会话与回收站条目的右键菜单

### 变更

- Windows 环境标签改为蓝色，WSL 改为绿色
- 主题切换按钮显示当前主题
- “关于”只显示版本号
- 侧边栏环境计数对齐
- 移除项目分组下的路径行

### 修复

- 项目分组标题吸顶时与列表内容重叠

## [0.1.0] - 2026-11-10

### 新增

- 浏览 Windows 与 WSL 中 Claude Code、Codex、OpenCode 和 Antigravity CLI 的会话
- 搜索标题与正文，查看对话、思考过程和工具调用
- 在 Windows Terminal 中继续会话，导出 Markdown
- 回收站：删除、恢复、彻底删除
- 浅色与深色主题、键盘快捷键
- 检查 GitHub Releases 新版本
- 安装程序与便携版

[Unreleased]: https://github.com/jewel-robot/session-desk/compare/v0.1.2...HEAD
[0.1.2]: https://github.com/jewel-robot/session-desk/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/jewel-robot/session-desk/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/jewel-robot/session-desk/releases/tag/v0.1.0
