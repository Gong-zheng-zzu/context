# 比赛现场演示与部署

## 固定演示流程

1. 本地 `docker compose ps` 显示 Context-Keeper、Qdrant、TimescaleDB、Neo4j 健康。
2. 用已配置的演示账号登录，运行一次真实三路检索并展示来源、`doc_id` 和延迟。
3. 输入一条代表性攻击样本，展示轻量输入安全判定。
4. 仅在隔离实验用户已 seed 时，执行一次前后计数的遗忘验证。
5. 打开赛前生成的因果报告，不把慢速因果抽取包装成实时能力。

PowerShell 入口：

```powershell
$env:EVAL_USER_ID = 'eval_user_001'
$env:EVAL_PASSWORD = '<演示环境密码>'
cd D:\context\context-keeper-main\experiments\scripts
.\run_competition_demo.ps1
```

检索 P95 大于 1 秒时脚本会失败并保留日志；此时只展示离线报告，不要现场等待或重复运行。遗忘步骤默认跳过，必须同时传入 `-RunIsolatedUnlearning` 和 `EVAL_DEMO_ALLOW_UNLEARNING=true`。

## 离线彩排清单

- 重启比赛笔记本后，断开网络，再启动 Docker Compose。
- 预先拉取 `qwen2.5:3b` 和 embedding 模型，确认容器能访问本机 Ollama。
- 预 seed 30 条快速演示语料；500 条语料仅用于赛前正式评测。
- 将 `results/reports/full_report.html`、图表、原始 JSON、PPT 和本文件复制到 U 盘。
- 演示前以 `measure_retrieval_latency.py` 运行 3 次预热和 10 次测量，记录 P95。

## 2核4G 云端边界

2核4G 云服务器只部署静态报告、前端静态页和 HTTPS 反向代理。不要在同一实例运行 Qdrant、TimescaleDB、Neo4j 与 Ollama。完整 API 仅在本地演示机运行；若必须公网调用，应将数据库改为托管服务、模型改为外部 API，并重新完成性能和安全验证。
