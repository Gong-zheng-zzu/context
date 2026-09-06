# 贡献指南

[English](CONTRIBUTING.md) | **简体中文**

首先，感谢您考虑为 Context-Keeper 做出贡献！🎉

我们欢迎各种形式的贡献：报告 Bug、提出新功能、改进文档、修复错别字，或实现新特性。无论您是首次贡献者还是经验丰富的开发者，您的参与都能让 Context-Keeper 变得更好。

## 目录

- [贡献指南](#贡献指南)
  - [目录](#目录)
  - [前置条件](#前置条件)
  - [环境搭建](#环境搭建)
  - [贡献方式 1 - 报告问题](#贡献方式-1---报告问题)
    - [问题处理流程](#问题处理流程)
  - [贡献方式 2 - 更新文档](#贡献方式-2---更新文档)
    - [轻微的文档修改](#轻微的文档修改)
    - [重要的文档修改](#重要的文档修改)
  - [贡献方式 3 - 代码变更](#贡献方式-3---代码变更)
    - [轻微的代码变更](#轻微的代码变更)
    - [重要的代码变更](#重要的代码变更)
    - [更新 API 文档](#更新-api-文档)
  - [贡献者资源](#贡献者资源)
    - [PR 和提交规范](#pr-和提交规范)
    - [理解标签系统](#理解标签系统)
    - [贡献者成长路径](#贡献者成长路径)
    - [获取支持](#获取支持)
    - [更多资源](#更多资源)

---

## 前置条件

### 签署 DCO

对于每一次提交，**您必须签署 [DCO（开发者原创证明）](https://developercertificate.org/)**。这确认您有权提交您的贡献。

```bash
# 在提交时添加 -s 标志
git commit -s -m "feat: 添加新功能"
```

### 遵守行为准则

本项目及其所有参与者均受我们的[行为准则](CODE_OF_CONDUCT.md)约束。参与即表示您同意遵守此准则。请将不可接受的行为报告给 contact@context-keeper.com。

---

## 环境搭建

### Fork 和克隆仓库

首先，在 GitHub 上 **Fork 本仓库**，然后克隆您的 Fork：

```bash
# 1. 克隆您的 Fork
git clone https://github.com/YOUR_USERNAME/context-keeper.git
cd context-keeper

# 2. 添加上游远程仓库
git remote add upstream https://github.com/redleaves/context-keeper.git

# 3. 验证远程仓库配置
git remote -v
```

### 安装依赖

**前置要求：**
- **Go 1.21+**（后端必需）
- **Node.js 16+**（VSCode 扩展必需）
- **Docker**（可选，用于运行依赖服务）
- 基本了解：
  - Go 编程语言
  - LLM 和向量数据库
  - MCP 协议（有了解更好，但非必需）

**安装步骤：**

```bash
# 安装 Go 依赖
go mod download

# 安装 Node.js 依赖（VSCode 扩展）
cd cursor-integration
npm install
cd ..

# 复制并配置环境变量
cp config/env.template config/.env
# 编辑 config/.env 填写您的配置
```

### 搭建开发环境

**方式 1：使用 Docker Compose（推荐）**

```bash
# 启动必需的服务（Vearch、TimescaleDB、Neo4j）
docker-compose up -d

# 验证服务运行状态
docker-compose ps
```

**方式 2：手动搭建**

您需要搭建以下服务：
- 向量数据库（Vearch 或 DashVector，也可基于接口自定义扩展自己需要的向量数据库）
- 时序数据库（TimescaleDB）
- 图数据库（Neo4j）

详见[部署指南](README-zh-CN.md#4-部署与集成)。任何细节问题也可联系微信：**arm386x86**

### 运行测试

```bash
# 运行单元测试
go test ./...

# 运行集成测试
go test ./tests/integration/...

# 运行覆盖率测试
go test -cover ./...
```

### 启动开发服务器

```bash
# 启动 HTTP 服务器
./scripts/manage.sh deploy http --port 8088

# 验证服务运行
curl http://localhost:8088/health
```

---

## 贡献方式 1 - 报告问题

**对于安全相关的问题，请直接发送邮件到 contact@context-keeper.com。**

发现了 Bug 或有功能建议？以下是报告方法：

### 步骤 1：搜索现有 Issue

在创建新 Issue 前，**搜索现有 Issue** 以避免重复：

- [开放的 Issue](https://github.com/redleaves/context-keeper/issues)
- [已关闭的 Issue](https://github.com/redleaves/context-keeper/issues?q=is%3Aissue+is%3Aclosed)

有用的标签帮助您导航：
- [`good first issue`](https://github.com/redleaves/context-keeper/labels/good%20first%20issue) - 适合新手
- [`help wanted`](https://github.com/redleaves/context-keeper/labels/help%20wanted) - 需要额外关注
- [`bug`](https://github.com/redleaves/context-keeper/labels/bug) - 某些功能不正常
- [`enhancement`](https://github.com/redleaves/context-keeper/labels/enhancement) - 新功能或请求
- [`documentation`](https://github.com/redleaves/context-keeper/labels/documentation) - 文档改进

### 步骤 2：创建 Issue

如果没有相关 Issue，创建新的：

**Bug 报告：**
1. 使用 Bug 报告模板
2. 包含：
   - 清晰的 Bug 描述
   - 重现步骤
   - 期望行为 vs 实际行为
   - 环境详情（操作系统、Go 版本等）
   - 最小可复现示例
   - 相关日志或截图

**功能请求：**
1. 使用功能请求模板
2. 包含：
   - 清晰的功能描述
   - 使用场景和动机
   - 提议的解决方案（如果有）
   - 考虑过的替代方案
   - 示例或效果图（如适用）

**文档问题：**
1. 描述缺失或不清楚的内容
2. 提出改进建议
3. 链接到相关章节

### 步骤 3：等待分类

项目维护者会：
1. 审查并分类 Issue
2. 应用适当的标签
3. 可能会要求补充信息
4. 决定是否接受该 Issue

**请耐心等待** - 我们处理大量 Issue。除非有新信息，否则请勿催促。

### 问题处理流程

Issue 会被标记为以下之一：
- [`duplicate`](https://github.com/redleaves/context-keeper/labels/duplicate) - Issue 已存在
- [`wontfix`](https://github.com/redleaves/context-keeper/labels/wontfix) - 不会被处理（附理由）
- [`good first issue`](https://github.com/redleaves/context-keeper/labels/good%20first%20issue) - 适合新手
- [`help wanted`](https://github.com/redleaves/context-keeper/labels/help%20wanted) - 欢迎社区贡献

---

## 贡献方式 2 - 更新文档

文档改进永远受欢迎！清晰全面的文档帮助每个人。

### 轻微的文档修改

对于简单修复，如错别字、错误链接或格式问题：

1. **直接修改** 您 Fork 仓库中的文档
2. **创建 Pull Request** 遵循我们的 [PR 规范](#pr-和提交规范)
3. **维护者审查** 并合并您的修改

### 重要的文档修改

对于实质性的变更，例如：
- 重构文档结构
- 添加新的指南或教程
- 重写主要章节

**遵循以下流程：**

1. **创建 Issue** 描述您提议的变更
2. **等待批准** 来自维护者
3. **进行修改** 在您的 Fork 中
4. **创建 Pull Request** 附上清晰的描述

**文档位置：**
- 主 README：`README.md`、`README-zh-CN.md`
- 架构文档：`docs/ARCHITECTURE.md`
- API 文档：`docs/api/`
- 开发者文档：`docs/dev/`
- 使用指南：`docs/usage/`

---

## 贡献方式 3 - 代码变更

对于任何代码变更，您**需要有关联的 Issue**：要么是现有的，要么[创建新的](#贡献方式-1---报告问题)。

> ⚠️ **为避免重复工作，请在 Issue 中评论说明您正在处理它。**

### 轻微的代码变更

**示例：** 修复代码注释中的错别字、小规模重构、更新依赖

**流程：**
1. **认领 Issue** 在 Issue 下评论
2. **在 Fork 中进行修改**：
   ```bash
   git checkout -b fix/issue-description
   # 进行修改
   go test ./...
   git add .
   git commit -s -m "fix: 修复描述"
   ```
3. **推送并创建 PR**：
   ```bash
   git push origin fix/issue-description
   # 到 GitHub 创建 Pull Request
   ```
4. **遵循我们的 [PR 规范](#pr-和提交规范)**

### 重要的代码变更

**示例：** 新功能、架构变更、重大重构

**流程：**

1. **认领 Issue** 在 Issue 下评论

2. **编写设计方案**：
   - 描述问题和提议的解决方案
   - 包含：
     - 架构变更
     - API 变更
     - 数据库模式变更
     - 迁移策略
     - 测试策略
     - 性能影响

3. **提交设计审查**：
   - 创建包含设计文档的新 PR
   - 命名为：`[RFC] 您的设计标题`
   - 放置在：`docs/dev/rfcs/YYYY-MM-DD-design-name.md`

4. **等待设计批准**：
   - 维护者会审查并提供反馈
   - 根据反馈迭代设计直到批准
   - 设计 PR 必须合并后才能开始实现

5. **实现已批准的设计**：
   ```bash
   git checkout -b feature/your-feature-name
   # 实现您的变更
   go test ./...
   git commit -s -m "feat: 实现功能"
   ```

6. **我们鼓励原子化 PR**：
   - 将大的变更拆分为较小的子任务
   - 为每个子任务创建一个 PR
   - 每个 PR 应该是独立可审查和可合并的

7. **创建 Pull Request** 遵循我们的规范

### 更新 API 文档

Context-Keeper 通过 MCP 协议暴露 API。如果您修改了任何 API：

**对于 HTTP API：**
1. 更新 `docs/api/` 中的 API 文档
2. 包含请求/响应示例
3. 如适用，更新 OpenAPI 规范

**对于 MCP 工具：**
1. 更新代码中的工具描述
2. 更新 MCP 清单：`config/manifests/context-keeper-manifest.json`
3. 使用 MCP 客户端测试：
   ```bash
   # 测试您的变更
   ./scripts/test-mcp-tools.sh
   ```

**文档检查清单：**
- [ ] 更新 API 文档
- [ ] 添加请求/响应示例
- [ ] 如需要，更新 MCP 清单
- [ ] 使用实际 MCP 客户端测试
- [ ] 如适用，更新集成指南

---

## 贡献者资源

### PR 和提交规范

**Pull Request 要求：**
- [ ] PR 标题遵循[约定式提交](https://www.conventionalcommits.org/zh-hans/)
- [ ] PR 描述清晰解释变更
- [ ] 所有测试通过（`go test ./...`）
- [ ] 代码遵循样式指南（见下文）
- [ ] 如需要，更新文档
- [ ] 所有提交都签署了 DCO
- [ ] 与 main 分支无合并冲突

**提交消息格式：**

我们遵循[约定式提交](https://www.conventionalcommits.org/zh-hans/)：

```
type(scope): subject

body (可选)

footer (可选)
```

**类型：**
- `feat`: 新功能
- `fix`: Bug 修复
- `docs`: 仅文档变更
- `style`: 代码样式变更（格式化、缺失分号等）
- `refactor`: 代码重构（无功能变更）
- `perf`: 性能改进
- `test`: 添加或更新测试
- `chore`: 维护任务（依赖、构建等）

**示例：**

```bash
# 功能
git commit -s -m "feat(retrieval): 添加多维融合引擎

- 实现语义+时间+知识融合
- 添加 LLM 驱动的排序策略
- 包含完整测试覆盖

Closes #123"

# Bug 修复
git commit -s -m "fix(session): 解决工作空间隔离问题

不同工作空间的会话可能相互干扰。
添加工作空间路径验证和隔离检查。

Fixes #456"

# 文档
git commit -s -m "docs(readme): 更新快速开始指南"

# 杂务
git commit -s -m "chore(deps): 更新 Go 依赖"
```

**代码样式指南：**

**Go 代码：**
- 遵循 [Effective Go](https://golang.org/doc/effective_go)
- 使用 `gofmt` 格式化代码
- 遵循 [Uber Go 样式指南](https://github.com/uber-go/guide/blob/master/style.md)
- 提交前运行 `golangci-lint run`

**TypeScript/JavaScript：**
- 使用 ESLint 和 Prettier
- 遵循 [Airbnb JavaScript 样式指南](https://github.com/airbnb/javascript)
- 提交前运行 `npm run lint`

**测试要求：**
- 为新功能编写单元测试
- 目标代码覆盖率 >80%
- 为复杂功能包含集成测试
- 测试边界情况和错误条件

### 理解标签系统

我们使用标签来组织 Issue 和 PR：

**优先级：**
- `priority: high` - 关键问题
- `priority: medium` - 标准优先级
- `priority: low` - 不错但非必需

**类型：**
- `bug` - 某些功能不正常
- `enhancement` - 新功能或改进
- `documentation` - 文档改进
- `question` - 问题或讨论

**状态：**
- `good first issue` - 适合新手
- `help wanted` - 需要额外关注
- `in progress` - 有人正在处理
- `needs review` - 等待审查
- `blocked` - 被其他 Issue 阻塞

**领域：**
- `area: api` - API 相关
- `area: storage` - 存储层
- `area: retrieval` - 检索引擎
- `area: mcp` - MCP 协议
- `area: integration` - IDE/工具集成

### 贡献者成长路径

我们重视并认可我们的贡献者：

| 等级 | 条件 | 徽章 | 权限 |
|------|------|------|------|
| **贡献者** | 1+ 个已合并 PR | ![Contributor](https://img.shields.io/badge/Contributor-green) | 在 README 中致谢 |
| **核心贡献者** | 5+ 个已合并 PR 或重要功能 | ![Core](https://img.shields.io/badge/Core%20Contributor-blue) | 可审查 PR、分类 Issue |
| **维护者** | 持续的重要贡献 | ![Maintainer](https://img.shields.io/badge/Maintainer-purple) | 合并权限、版本管理 |

**如何成为维护者：**
- 展示持续的高质量贡献
- 积极参与代码审查
- 帮助 Issue 分类和社区支持
- 展示对项目的深入理解
- 维护者是邀请制，不接受申请

### 获取支持

**问题和讨论：**
- [GitHub 讨论](https://github.com/redleaves/context-keeper/discussions) - 问题和一般讨论
- [GitHub Issues](https://github.com/redleaves/context-keeper/issues) - Bug 和功能请求

**联系方式：**
- 邮箱：contact@context-keeper.com
- 微信：**arm386x86**

**响应时间：**
- Issue：1-3 个工作日进行分类
- PR：2-5 个工作日进行初步审查
- 安全问题：24 小时内

### 更多资源

**学习资源：**
- [Go 官方文档](https://golang.org/doc/)
- [Effective Go](https://golang.org/doc/effective_go)
- [MCP 协议文档](https://github.com/modelcontextprotocol)
- [向量数据库概念](https://www.pinecone.io/learn/vector-database/)

**项目资源：**
- [架构设计](docs/ARCHITECTURE.md)
- [项目结构](docs/dev/README_PROJECT_STRUCTURE.md)
- [部署指南](README-zh-CN.md#4-部署与集成)
- [API 参考](docs/api/)

**开发工具：**
- [golangci-lint](https://golangci-lint.run/) - Go 代码检查工具
- [约定式提交](https://www.conventionalcommits.org/zh-hans/) - 提交消息标准
- [GitHub CLI](https://cli.github.com/) - GitHub 命令行工具

---

## 常见问题

**Q: 我是开源新手，应该从哪里开始？**

A: 从标记为 [`good first issue`](https://github.com/redleaves/context-keeper/labels/good%20first%20issue) 的 Issue 开始。这些是专门选择的对初学者友好的任务。不要犹豫提问！

**Q: 审查我的 PR 需要多长时间？**

A: 我们的目标是在 2-5 个工作日内提供初步反馈。复杂的 PR 可能需要更长时间。如果一周内没有回复，欢迎礼貌地提醒我们。

**Q: 我可以同时处理多个 Issue 吗？**

A: 可以，但我们建议一次专注于一个，特别是刚开始时。这有助于保持质量并允许更快的迭代。

**Q: 我的 PR 有合并冲突，怎么办？**

A: 在最新的 main 分支上 rebase 您的分支：
```bash
git fetch upstream
git rebase upstream/main
# 解决冲突
git push --force-with-lease origin your-branch
```

**Q: 如何将我的 Fork 与上游仓库同步？**

A:
```bash
git checkout main
git fetch upstream
git merge upstream/main
git push origin main
```

**Q: 我可以提交破坏性变更吗？**

A: 破坏性变更需要强有力的理由，必须通过设计提案流程。请先与维护者讨论。

---

## 感谢您！

感谢您为 Context-Keeper 做出贡献！🎉

每一份贡献，无论大小，都帮助我们构建更好的智能记忆系统。我们期待您的贡献！

---

**有疑问？** 随时：
1. 查看 [GitHub 讨论](https://github.com/redleaves/context-keeper/discussions)
2. 审查[现有 Issue](https://github.com/redleaves/context-keeper/issues)
3. 联系我们 contact@context-keeper.com

*愉快贡献！💪*
