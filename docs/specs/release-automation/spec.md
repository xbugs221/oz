# release-automation 规格

## 目的

定义 oz 仓库通过 GitHub Actions 发布版本 tag 时必须满足的测试门禁、跨平台构建、打包、校验和 Release 资产完整性约束。

## 需求

### 需求：Tag 触发发布

系统必须在 `v*` tag 推送到 GitHub 后自动执行发布 workflow。

#### 场景：推送版本 tag
- **当** 维护者推送 `v1.2.3` 形式的 tag
- **则** GitHub Actions 启动 release workflow
- **且** workflow 使用该 tag 作为 Release 版本来源

### 需求：发布前测试门禁

系统必须在构建和上传 Release 资产前运行仓库完整测试门禁。

#### 场景：Go 单测失败
- **当** `go test ./...` 返回非零退出码
- **则** workflow 失败
- **且** 系统不得构建发布压缩包
- **且** 系统不得创建或更新 GitHub Release
- **且** 系统不得上传任何 Release asset

#### 场景：完整测试门禁通过
- **当** `go test ./...` 通过
- **则** workflow 才能进入跨平台构建阶段

// Sources: 18-修复GitHub-CI并更新仓库文档

#### 场景：README 说明 CI/Release 门禁和本地复现入口
- **给定** 仓库存在 `.github/workflows/ci.yml` 和 `.github/workflows/release.yml`
- **当** 维护者阅读 README 和 release automation 规格
- **则** README 必须说明 GitHub Actions、CI、Release 和 `go test ./...`
- **且** README 必须给出本地复现 GitHub CI 失败的入口
- **且** release automation 规格必须继续说明 CI/Release 使用本地 `oz` 和同一套测试门禁
- **且** workflow 文件必须保留 `go test ./...`
- **测试**：`tests/specs/release-automation/test_monorepo_cli_release_contract.sh`
- **关键断言**：仓库文档、长期规格和 GitHub Actions workflow 对同一套门禁保持一致

### 需求：跨平台二进制构建

系统必须为 Linux、macOS、Windows 的 x64 和 arm64 架构构建 `oz` CLI。

#### 场景：构建完整平台矩阵
- **当** release workflow 进入构建阶段
- **则** 系统构建 `linux/amd64`
- **则** 系统构建 `linux/arm64`
- **则** 系统构建 `darwin/amd64`
- **则** 系统构建 `darwin/arm64`
- **则** 系统构建 `windows/amd64`
- **则** 系统构建 `windows/arm64`

#### 场景：Windows 二进制命名
- **当** 系统构建 Windows 目标
- **则** 压缩包内的可执行文件名为 `oz.exe`

#### 场景：Unix-like 二进制命名
- **当** 系统构建 Linux 或 macOS 目标
- **则** 压缩包内的可执行文件名为 `oz`

### 需求：Release 资产完整性

系统必须只在全部测试和构建成功后发布完整资产。

#### 场景：全部目标构建成功
- **当** Go 测试门禁通过
- **且** 六个构建目标全部成功
- **则** GitHub Release 包含六个平台/架构压缩包
- **且** GitHub Release 包含 `sha256sums.txt`

#### 场景：任一目标构建失败
- **当** 任一平台/架构目标构建失败
- **则** workflow 失败
- **且** 系统不得发布缺少目标平台的 Release

### 需求：可校验下载

系统必须为 Release 中的每个压缩包提供 SHA-256 校验值。

#### 场景：生成校验和文件
- **当** release workflow 准备上传 assets
- **则** 系统生成 `sha256sums.txt`
- **且** `sha256sums.txt` 覆盖所有平台/架构压缩包
- **且** `sha256sums.txt` 不以未压缩二进制作为校验对象

### 需求：单仓库单 CLI 发布

系统必须在当前 `oz` 仓库中维护唯一 `oz` CLI，并通过 `oz flow` 命令组承载工作流执行器能力。

#### 场景：同一 checkout 构建唯一 oz
- **给定** 开发者在合并后的仓库根目录
- **当** 运行单仓库 CLI 发布规格测试
- **则** 测试必须能从 `./cmd/oz` 构建 `oz`
- **且** 构建出的 `oz` 必须支持 `flow` 命令组
- **且** `go list -m` 仍返回 `github.com/xbugs221/oz`
- **且** `./cmd/oz` 的依赖中不得出现迁移前的旧工作流模块路径

#### 场景：CI 和 Release 使用本地 oz
- **给定** 合并后的仓库包含 GitHub Actions workflow
- **当** 运行单仓库 CLI 发布规格测试
- **则** workflow 中不得继续下载 `github.com/xbugs221/oz/releases/latest`
- **且** workflow 必须包含从当前 checkout 构建 `./cmd/oz` 并验证 `flow` 命令组的步骤或命令
- **且** CI 和 Release 使用本地 oz、oz flow 后必须继续运行 `go test ./...`
