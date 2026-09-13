# 忆安智护 Context-Keeper

面向智慧养老的护理数据智能治理与可信辅助决策平台。系统帮助护理机构把记录记清楚、查得到、防得住、说得明、删得净；AI 只提供辅助整理、检索和风险提示，不替代护工或医生。

## 快速启动

要求：Docker Desktop、Docker Compose、Go 1.25（仅源码开发需要）。

```powershell
cd D:\context\context-keeper-main
docker compose up -d --build
Invoke-WebRequest http://127.0.0.1:8088/health
```

浏览器控制台：

- 在线模式：`http://127.0.0.1:8088/web/competition_console.html?mode=ollama`
- 离线夹具：直接打开 `web/competition_console.html?mode=fixture`

离线夹具只展示固定样例，不伪造真实模型或 RRF 证据。在线演示需要在本地配置认证凭据和 Ollama；凭据只通过环境变量提供，不提交到 Git。

## 比赛验证

```powershell
# 离线契约检查
python experiments/scripts/smoke_test.py --mode fixture

# 在线受保护 API 冒烟（先设置 EVAL_USER_ID、EVAL_PASSWORD）
python experiments/scripts/smoke_test.py --mode live --base-url http://127.0.0.1:8088

# 生成本次演示目录和可追溯 JSON
powershell -File experiments/scripts/run_competition_demo.ps1 -Mode fixture
```

正式报告只接受带数据集哈希、配置指纹、清理记录和来源审计的评测结果；在完成同数据集、同配置的三路检索对比前，不宣称 RRF 提升比例。

## 目录职责

| 目录 | 用途 |
| --- | --- |
| `cmd/` | 服务和命令行入口 |
| `internal/` | 认证、因果抽取、检索、存储和安全实现 |
| `pkg/` | 可复用基础包和向量存储适配器 |
| `web/` | 无构建依赖的控制台和演示页面 |
| `experiments/` | 可复现实验、数据集、评测脚本和报告模板 |
| `tests/` | Go 集成/契约测试与基准工具 |
| `docs/competition/` | 比赛定位、创新点和证据边界 |
| `config/` | 配置模板；实际密钥使用本地 `.env` |

## 测试

```powershell
python -m unittest discover -s experiments/scripts -p "test_*.py"
go test ./pkg/vectorstore ./internal/engines/causal_reasoning ./internal/engines
go vet -tags http ./...
```

完整 Docker/Linux 门禁还应执行 `go mod verify`、容器构建、健康检查和受保护 API 冒烟。模型不可用时，因果接口必须明确返回规则模式或“真实模型不可用”，不得生成伪造结果。

## 安全提示

不要提交 `.env`、令牌、运行密钥、`effective.env` 或生产/私有护理数据。公开仓库中的历史实验结果仅作为可追溯材料，不能替代重新运行的正式评测。

