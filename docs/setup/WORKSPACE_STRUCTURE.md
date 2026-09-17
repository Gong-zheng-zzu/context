# Context 工作空间结构说明

> 最后更新：2026-07-08  
> 状态：已整理优化

## 📁 目录结构

```
d:/context/
├── context-keeper-main/          # Context-Keeper 主项目
│   ├── cmd/                      # 命令行工具和演示程序
│   ├── internal/                 # 核心业务逻辑
│   │   ├── api/                  # HTTP API处理器
│   │   ├── engines/              # 核心引擎（检索、推理等）
│   │   ├── models/               # 数据模型
│   │   ├── security/             # 安全模块（PCCM、ASDF、CASIA）
│   │   └── services/             # 业务服务
│   ├── pkg/                      # 可复用的公共包
│   ├── config/                   # 统一配置目录 ✨
│   │   ├── .env                  # 环境变量（已合并）
│   │   ├── llm_config.yaml       # LLM配置（已更新）
│   │   ├── security_policy.yaml  # 安全策略（已更新）
│   │   ├── docker-daemon.json    # Docker配置
│   │   └── archived/             # 旧配置备份
│   ├── docs/                     # 项目文档 ✨
│   │   ├── archived/             # 历史文档归档
│   │   ├── competition/          # 信息安全大赛相关文档
│   │   ├── api/                  # API文档
│   │   ├── demo/                 # 演示指南
│   │   ├── ARCHITECTURE.md       # 架构设计
│   │   ├── CORE_TECHNOLOGIES.md  # 核心技术清单
│   │   ├── PROJECT_PROPOSAL.md   # 项目策划书
│   │   └── [其他核心文档...]
│   ├── testdata/                 # 测试数据集
│   │   ├── causal_test_cases.json
│   │   ├── nursing_records.json
│   │   ├── machine_unlearning_test.json
│   │   └── rrf_fusion_test.json
│   ├── tests/                    # 测试代码
│   ├── cursor-integration/       # Cursor IDE集成
│   └── README.md
│
├── deliverables/                 # 交付物目录 ✨
│   └── Context-Keeper-最终提交包/
│
├── .claude/                      # Claude Code配置
│   ├── settings.local.json
│   └── plans/
│
└── WORKSPACE_STRUCTURE.md        # 本文档

```

## 🎯 整理优化说明

### ✅ 已完成的整理

1. **配置目录合并**
   - 删除根目录的 `config/` 和 `configs/`
   - 所有配置统一到 `context-keeper-main/config/`
   - 使用最新版本的配置文件（Jul 6更新）

2. **文档清理**
   - 删除重复的Docker文档（保留 `DOCKER_DEPLOYMENT.md`）
   - 移动15+个临时报告到 `docs/archived/`
   - 核心文档从94个精简到25个

3. **交付物整理**
   - 删除临时脚本（`.bat`文件）
   - 保留最终提交包

4. **备份清理**
   - 删除空的 `workspace-archive/` 目录

### 📊 整理成果

- **删除文件**: 23个冗余文件
- **归档文档**: 15个临时报告
- **合并目录**: 3个配置目录 → 1个
- **文档精简**: 94个 → 25个核心文档
- **目录层级**: 更清晰的结构

## 📚 核心文档索引

### 架构与设计
- [ARCHITECTURE.md](context-keeper-main/docs/ARCHITECTURE.md) - 系统架构设计
- [PROJECT_PROPOSAL.md](context-keeper-main/docs/PROJECT_PROPOSAL.md) - 项目策划书
- [CORE_TECHNOLOGIES.md](context-keeper-main/docs/CORE_TECHNOLOGIES.md) - 核心技术清单

### 安全技术
- [MULTI_LAYER_DETECTION_ARCHITECTURE.md](context-keeper-main/docs/MULTI_LAYER_DETECTION_ARCHITECTURE.md) - 多层检测架构
- [BYPASS_TESTING_GUIDE.md](context-keeper-main/docs/BYPASS_TESTING_GUIDE.md) - 绕过攻击测试
- [ASDF_THRESHOLD_GUIDE.md](context-keeper-main/docs/ASDF_THRESHOLD_GUIDE.md) - ASDF阈值指南
- [docs/archived/AI_SECURITY_TECH.md](context-keeper-main/docs/archived/AI_SECURITY_TECH.md) - AI安全技术总结

### 部署与测试
- [DEPLOYMENT_CHECKLIST.md](context-keeper-main/docs/DEPLOYMENT_CHECKLIST.md) - 部署检查清单
- [DOCKER_DEPLOYMENT.md](context-keeper-main/docs/DOCKER_DEPLOYMENT.md) - Docker部署指南
- [TESTING_PLAN.md](context-keeper-main/docs/TESTING_PLAN.md) - 测试计划
- [DEMO_GUIDE.md](context-keeper-main/docs/DEMO_GUIDE.md) - 演示指南

### 开发指南
- [CONTRIBUTING.md](context-keeper-main/docs/CONTRIBUTING.md) - 贡献指南（英文）
- [CONTRIBUTING-zh-CN.md](context-keeper-main/docs/CONTRIBUTING-zh-CN.md) - 贡献指南（中文）
- [CODE_OF_CONDUCT.md](context-keeper-main/docs/CODE_OF_CONDUCT.md) - 行为准则

## 🔧 配置文件说明

### config/llm_config.yaml
- **用途**: LLM模型配置（Ollama、OpenAI、Qwen等）
- **更新**: 2026-07-06（518行，完整配置）

### config/security_policy.yaml
- **用途**: 安全策略配置（敏感信息规则、ASDF阈值）
- **更新**: 2026-07-06（844行，完整策略）

### config/.env
- **用途**: 环境变量（数据库连接、API密钥）
- **注意**: 包含敏感信息，不要提交到git

## 📦 测试数据集

位于 `context-keeper-main/testdata/`：

| 文件 | 用途 | 数据量 |
|------|------|--------|
| causal_test_cases.json | 因果关系抽取测试 | 20个标注用例 |
| nursing_records.json | 护理记录样本 | 20条记录 |
| machine_unlearning_test.json | 机器遗忘测试 | 5个测试场景 |
| rrf_fusion_test.json | RRF融合算法测试 | 3个查询场景 |

## 🚀 快速开始

```bash
cd context-keeper-main

# 配置环境变量
cp config/env.template config/.env
# 编辑 config/.env 填入实际配置

# 启动服务（需要Ollama、Neo4j、Qdrant）
go run cmd/main.go

# 运行测试
go test ./...
```

## 📝 维护建议

1. **文档管理**
   - 临时报告直接放入 `docs/archived/`
   - 核心文档放在 `docs/` 根目录
   - 定期清理过期文档

2. **配置管理**
   - 配置变更统一在 `config/` 目录
   - 敏感信息使用 `.env`，不提交到git
   - 配置示例使用 `.example` 后缀

3. **版本管理**
   - 使用git标签标记重要版本
   - 大版本发布前打包到 `deliverables/`

## 🔗 相关资源

- **理论基础**: 参见之前的"信息安全大赛核心技术及理论出处"总结
- **实施计划**: 参见 `.claude/plans/stateless-shimmying-haven.md`
- **比赛材料**: 参见 `docs/competition/` 目录

---

**整理完成时间**: 2026-07-08 10:20  
**整理工具**: Claude Code
