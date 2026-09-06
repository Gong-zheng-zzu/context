#!/bin/bash
# 因果推理实验快速启动脚本

set -e

echo "=========================================="
echo "Context-Keeper 因果推理实验测试"
echo "=========================================="
echo ""

# 颜色定义
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 检查Python
if ! command -v python3 &> /dev/null; then
    echo -e "${RED}[ERROR] Python3 未安装${NC}"
    exit 1
fi

# 检查依赖
echo "[1/5] 检查Python依赖..."
if ! python3 -c "import requests" &> /dev/null; then
    echo -e "${YELLOW}[WARN] 缺少依赖，正在安装...${NC}"
    pip install -r requirements.txt
fi
echo -e "${GREEN}✓ 依赖检查完成${NC}"
echo ""

# 检查服务
echo "[2/5] 检查Context-Keeper服务..."
if curl -s http://localhost:8080/health > /dev/null 2>&1; then
    echo -e "${GREEN}✓ 服务运行正常 (http://localhost:8080)${NC}"
elif curl -s http://localhost:8088/health > /dev/null 2>&1; then
    echo -e "${GREEN}✓ 服务运行正常 (http://localhost:8088)${NC}"
    API_URL="http://localhost:8088"
else
    echo -e "${RED}✗ 服务未运行${NC}"
    echo ""
    echo "请先启动Context-Keeper服务："
    echo "  方式1: docker-compose up -d"
    echo "  方式2: go run cmd/server/main_http.go"
    echo ""
    read -p "是否继续测试（脚本仍会创建，但API调用会失败）? [y/N] " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi
echo ""

# 检查测试数据
echo "[3/5] 检查测试数据..."
if [ -f "../datasets/nursing_data/nursing_records.json" ]; then
    RECORD_COUNT=$(python3 -c "import json; print(len(json.load(open('../datasets/nursing_data/nursing_records.json'))))")
    echo -e "${GREEN}✓ 护理记录数据: ${RECORD_COUNT} 条${NC}"
else
    echo -e "${RED}✗ 护理记录数据缺失${NC}"
    exit 1
fi

if [ -f "../datasets/causal_reasoning/annotated_nursing_records.json" ]; then
    ANNOTATED_COUNT=$(python3 -c "import json; print(len(json.load(open('../datasets/causal_reasoning/annotated_nursing_records.json'))))")
    echo -e "${GREEN}✓ 标注数据: ${ANNOTATED_COUNT} 条${NC}"
else
    echo -e "${YELLOW}[WARN] 标注数据缺失，将使用随机样本${NC}"
fi
echo ""

# 选择测试模式
echo "[4/5] 选择测试模式..."
echo "  1) 快速测试 (约5分钟，小样本)"
echo "  2) 完整测试 (约30-60分钟，大样本)"
echo "  3) 仅准确率测试"
echo "  4) 仅性能测试"
read -p "请选择 [1-4]: " -n 1 -r
echo
echo ""

case $REPLY in
    1)
        echo "[5/5] 运行快速测试..."
        echo ""
        echo "=== 准确率测试（20条样本） ==="
        python3 causal_reasoning_accuracy_test.py --samples 20 ${API_URL:+--url $API_URL}
        echo ""
        echo "=== 性能测试（快速模式） ==="
        python3 causal_reasoning_performance_test.py --quick ${API_URL:+--url $API_URL}
        ;;
    2)
        echo "[5/5] 运行完整测试..."
        echo ""
        echo "=== 准确率测试（100条样本） ==="
        python3 causal_reasoning_accuracy_test.py --samples 100 ${API_URL:+--url $API_URL}
        echo ""
        echo "=== 性能测试（完整模式） ==="
        python3 causal_reasoning_performance_test.py ${API_URL:+--url $API_URL}
        ;;
    3)
        echo "[5/5] 运行准确率测试..."
        echo ""
        python3 causal_reasoning_accuracy_test.py --samples 100 ${API_URL:+--url $API_URL}
        ;;
    4)
        echo "[5/5] 运行性能测试..."
        echo ""
        python3 causal_reasoning_performance_test.py ${API_URL:+--url $API_URL}
        ;;
    *)
        echo -e "${RED}无效选择${NC}"
        exit 1
        ;;
esac

echo ""
echo "=========================================="
echo -e "${GREEN}✓ 测试完成！${NC}"
echo "=========================================="
echo ""
echo "查看结果:"
echo "  - 详细数据: results/*.json"
echo "  - 测试报告: results/*.md"
echo ""
echo "查看最新报告:"
echo "  cat results/accuracy_test_report_*.md | tail -n 100"
echo "  cat results/performance_test_report_*.md | tail -n 100"
echo ""
