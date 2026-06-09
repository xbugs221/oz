# windows-platform-support 规格

## 目的
定义 wo 在 Windows 上运行 sealed workflow 时必须满足的路径、命令、进程、锁文件和终端输出约束。

## 需求

### 需求：跨平台路径处理

系统必须使用跨平台路径 API 访问文件，并在状态文件中使用稳定的相对路径表示。

#### 场景：Windows 路径写入状态文件
- **当** 系统在 Windows 上保存阶段日志路径
- **则** `state.json` 中的路径使用 slash 形式，例如 `.wo/runs/run-1/state.json`
- **则** 系统读取该路径时能转换为 Windows 本机路径

### 需求：Windows 命令发现

系统必须能在 Windows 上发现 Codex、oz 和 Git 的可执行入口。

#### 场景：通过 npm shim 安装的 Codex
- **当** `codex` 通过 npm 或 pnpm shim 安装
- **则** 系统使用 Go 的命令查找机制找到 `codex.cmd` 或等价入口
- **则** 系统不要求用户手写完整路径

### 需求：无 shell 子进程调用

系统必须直接调用子进程并通过 stdin 传递 prompt，避免依赖平台 shell quoting。

#### 场景：prompt 包含换行和引号
- **当** 自动阶段 prompt 包含多行文本和引号
- **则** 系统通过 stdin 传递 prompt
- **则** 子进程参数列表不包含拼接后的 shell 命令字符串

### 需求：跨平台中断恢复

系统必须在 Windows 上处理中断并保留可恢复状态。

#### 场景：用户在 Windows Terminal 中中断运行
- **当** 用户按 Ctrl-C
- **则** 系统取消当前 Codex 子进程
- **则** 系统刷新当前 JSONL 日志
- **则** 系统把当前 stage 标记为 `interrupted`
- **则** 下一次启动时可以选择恢复该 run

### 需求：不依赖 tmux

系统必须不依赖 tmux 或 Unix-only 终端能力。

#### 场景：Windows PowerShell 中运行
- **当** 用户在 Windows PowerShell 中启动程序
- **则** 系统使用普通文本 checklist 显示状态
- **则** 系统不调用 tmux、curses 或 Unix-only signal API
