#!/bin/bash
# Context-Keeper 完整测试套件运行脚本

set -e

echo "=========================================="
echo "Context-Keeper Test Suite"
echo "=========================================="
echo ""

# 颜色定义
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# 检查Python环境
echo "[CHECK] Checking Python environment..."
if ! command -v python &> /dev/null; then
    echo -e "${RED}[ERROR] Python is not installed${NC}"
    exit 1
fi
echo -e "${GREEN}[OK] Python found: $(python --version)${NC}"

# 安装依赖
echo ""
echo "[SETUP] Installing Python dependencies..."
pip install -q requests pyyaml

# 1. 生成测试数据（如果不存在）
echo ""
echo "=========================================="
echo "[STEP 1] Generating Test Datasets"
echo "=========================================="

if [ ! -f "tests/datasets/attack_samples/all_attack_samples.json" ]; then
    echo "[GENERATE] Generating attack samples..."
    python tests/datasets/attack_samples_generator.py
else
    echo -e "${YELLOW}[SKIP] Attack samples already exist${NC}"
fi

if [ ! -f "tests/datasets/nursing_data/nursing_records.json" ]; then
    echo "[GENERATE] Generating nursing records..."
    python tests/datasets/nursing_data_generator.py
else
    echo -e "${YELLOW}[SKIP] Nursing records already exist${NC}"
fi

# 2. 数据集统计
echo ""
echo "=========================================="
echo "[STEP 2] Dataset Statistics"
echo "=========================================="

echo "[STATS] Attack samples:"
for file in tests/datasets/attack_samples/*.json; do
    if [ -f "$file" ]; then
        count=$(python -c "import json; print(len(json.load(open('$file'))))" 2>/dev/null || echo "0")
        echo "  - $(basename $file): $count samples"
    fi
done

echo ""
echo "[STATS] Nursing data:"
if [ -f "tests/datasets/nursing_data/elder_profiles.json" ]; then
    elder_count=$(python -c "import json; print(len(json.load(open('tests/datasets/nursing_data/elder_profiles.json'))))" 2>/dev/null || echo "0")
    echo "  - Elder profiles: $elder_count"
fi

if [ -f "tests/datasets/nursing_data/nursing_records.json" ]; then
    record_count=$(python -c "import json; print(len(json.load(open('tests/datasets/nursing_data/nursing_records.json'))))" 2>/dev/null || echo "0")
    echo "  - Nursing records: $record_count"
fi

# 3. 检查服务状态
echo ""
echo "=========================================="
echo "[STEP 3] Checking Service Status"
echo "=========================================="

BASE_URL="http://localhost:8080"

if curl -s "$BASE_URL/health" > /dev/null 2>&1; then
    echo -e "${GREEN}[OK] Context-Keeper service is running${NC}"
else
    echo -e "${RED}[ERROR] Context-Keeper service is not running${NC}"
    echo "Please start the service first:"
    echo "  cd context-keeper-main"
    echo "  go run cmd/server/main.go"
    exit 1
fi

# 4. 运行攻击防御测试
echo ""
echo "=========================================="
echo "[STEP 4] Running Attack Defense Benchmark"
echo "=========================================="
echo ""
echo "This will test 4 system configurations:"
echo "  1. Baseline - Vanilla LLM"
echo "  2. Baseline - Naive RAG"
echo "  3. Baseline - RAG with Rule Filter"
echo "  4. Full System - Context-Keeper"
echo ""
read -p "Run attack defense benchmark? (y/n) " -n 1 -r
echo ""

if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "[RUN] Starting attack defense benchmark..."
    echo "[NOTE] This will take approximately 10-15 minutes"
    echo ""

    # 先运行快速测试（50个样本）
    python tests/benchmark/attack_defense_benchmark.py

    echo ""
    echo -e "${GREEN}[OK] Attack defense benchmark completed${NC}"
    echo "Results saved to: tests/benchmark/results/"
else
    echo -e "${YELLOW}[SKIP] Attack defense benchmark skipped${NC}"
fi

# 5. 运行机器遗忘测试
echo ""
echo "=========================================="
echo "[STEP 5] Running Machine Unlearning Test"
echo "=========================================="
echo ""
echo "This will test the gradient orthogonal projection unlearning algorithm."
echo ""
read -p "Run machine unlearning test? (y/n) " -n 1 -r
echo ""

if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "[RUN] Starting machine unlearning test..."
    echo "[NOTE] This will take approximately 5 minutes"
    echo ""

    python tests/benchmark/unlearning_benchmark.py

    echo ""
    echo -e "${GREEN}[OK] Machine unlearning test completed${NC}"
    echo "Results saved to: tests/benchmark/results/"
else
    echo -e "${YELLOW}[SKIP] Machine unlearning test skipped${NC}"
fi

# 6. 总结
echo ""
echo "=========================================="
echo "[SUMMARY] Test Suite Completed"
echo "=========================================="
echo ""
echo "Results location:"
echo "  - Attack defense: tests/benchmark/results/"
echo "  - Machine unlearning: tests/benchmark/results/"
echo "  - Comparison tables: tests/benchmark/results/comparison_table.md"
echo ""
echo "Next steps:"
echo "  1. Review test results"
echo "  2. Generate final report: python tests/benchmark/generate_report.py"
echo "  3. Check docs/TEST_REPORT.md for detailed analysis"
echo ""
echo -e "${GREEN}[DONE] All tests completed successfully!${NC}"
