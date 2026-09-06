# Context-Keeper 代码归类重构计划

## 📋 目标
整理项目根目录，将 32 个散落的脚本文件按功能归类到合适的目录

## 🎯 归类方案

### 1️⃣ 演示脚本 → `scripts/demo/`
```bash
mkdir -p scripts/demo
mv demo_ai_security.bat scripts/demo/
mv demo_attack_defense.bat scripts/demo/
mv demo_attack_matrix.bat scripts/demo/
mv demo_health_assistant.bat scripts/demo/
mv demo_jwt_auth.bat scripts/demo/
mv demo_multi_role.bat scripts/demo/
mv demo_nursing_home.bat scripts/demo/
mv demo.sh scripts/demo/
mv demo-api.sh scripts/demo/
mv run_attack_demo.sh scripts/demo/
```

### 2️⃣ 测试脚本 → `scripts/test/`
```bash
mkdir -p scripts/test
mv analyze_asdf_threshold.py scripts/test/
mv analyze_failures.py scripts/test/
mv analyze_id_card.py scripts/test/
mv diagnose_redaction.py scripts/test/
mv final_test.py scripts/test/
mv quick_test_redaction.py scripts/test/
mv verify_ai_security.sh scripts/test/
mv quick_verify.sh scripts/test/
mv rebuild_and_test.sh scripts/test/
```

### 3️⃣ 数据生成脚本 → `scripts/data/`
```bash
mkdir -p scripts/data
mv generate_test_data.go scripts/data/
mv generate_200_test_data.py scripts/data/
mv generate_large_test_data.py scripts/data/
```

### 4️⃣ 部署脚本 → `scripts/deploy/`
```bash
# scripts/deploy/ 已存在
mv docker.sh scripts/deploy/
mv docker.bat scripts/deploy/
```

### 5️⃣ 开发辅助脚本 → `scripts/dev/`
```bash
mkdir -p scripts/dev
mv start-local.bat scripts/dev/
mv start_all_services.bat scripts/dev/
mv start_frontend.bat scripts/dev/
mv start-health-assistant.bat scripts/dev/
mv start-health-assistant.sh scripts/dev/
mv restart_server.bat scripts/dev/
mv check_services.bat scripts/dev/
mv quick_start.bat scripts/dev/
```

### 6️⃣ 部署文档 → `docs/deployment/`
```bash
mkdir -p docs/deployment
mv DEPLOYMENT_CHECKLIST.md docs/deployment/
mv DEPLOYMENT_REPORT.md docs/deployment/
mv DEPLOYMENT_SUMMARY.md docs/deployment/
mv DOCKER_DEPLOYMENT.md docs/deployment/
mv DOCKER_QUICKSTART.md docs/deployment/
mv DOCKER_QUICK_START.md docs/deployment/
mv CONFIGURATION_REPORT.md docs/deployment/
```

### 7️⃣ 测试文档 → `docs/testing/`
```bash
mkdir -p docs/testing
mv TESTING_PLAN.md docs/testing/
mv BYPASS_TESTING_GUIDE.md docs/testing/
mv ASDF_THRESHOLD_GUIDE.md docs/testing/
mv FILE_UPLOAD_FIX_REPORT.md docs/testing/
mv FILE_UPLOAD_FIX_SUMMARY.md docs/testing/
mv FILE_UPLOAD_IMPLEMENTATION.txt docs/testing/
mv FILE_UPLOAD_VERIFICATION_CHECKLIST.md docs/testing/
mv FINAL_TEST_REPORT.md docs/testing/
mv SYSTEM_VERIFICATION_REPORT.md docs/testing/
mv manual_test_guide.md docs/testing/
```

### 8️⃣ 架构文档 → `docs/architecture/`
```bash
mkdir -p docs/architecture
mv CORE_TECHNOLOGIES.md docs/architecture/
mv MULTI_LAYER_DETECTION_ARCHITECTURE.md docs/architecture/
mv TEMPORAL_MEMORY_UPGRADE.md docs/architecture/
mv PROJECT_PROPOSAL.md docs/architecture/
```

### 9️⃣ 临时/输出文件 → 清理或移动
```bash
# 这些文件可以删除或移到 test_results/
mv debug_output.txt test_results/
mv id_card_analysis_result.txt test_results/
mv server.log logs/
mv server.pid logs/
```

## ⚠️ 注意事项

1. **更新引用路径**：移动脚本后，需要更新：
   - 其他脚本中的相对路径引用
   - 文档中的脚本路径说明
   - CI/CD 配置文件
   - README 中的使用说明

2. **保留根目录的文件**：
   - README.md / README-en.md
   - LICENSE
   - go.mod / go.sum
   - package.json / package-lock.json
   - Dockerfile / docker-compose.yml
   - .gitignore / .dockerignore / .env.example
   - 配置文件（如果有）

3. **Git 操作**：使用 `git mv` 而不是 `mv` 来保留文件历史

## 📊 预期效果

**重构前：** 根目录 70+ 个文件/文件夹
**重构后：** 根目录 ~25 个文件/文件夹（减少 60%+）

## 🚀 执行步骤

1. 创建备份
2. 创建新目录结构
3. 移动文件（使用 git mv）
4. 更新路径引用
5. 测试关键脚本
6. 更新文档
7. 提交变更
