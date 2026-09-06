#!/bin/bash
# Context-Keeper 快速演示入口
# 耗时：3-5分钟
# 用途：答辩现场快速展示核心能力

set -e

echo "=============================================="
echo "Context-Keeper 快速演示（答辩现场）"
echo "=============================================="
echo "预计耗时：3-5分钟"
echo "开始时间：$(date '+%Y-%m-%d %H:%M:%S')"
echo ""

# 进入scripts目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# P0验证：运行烟雾测试
echo "=============================================="
echo "P0验证：烟雾测试（验证四类API可用性）"
echo "=============================================="
echo ""

python smoke_test.py

if [ $? -ne 0 ]; then
    echo ""
    echo "[FAIL] 烟雾测试失败，请修复API后再运行实验"
    exit 1
fi

echo ""
echo "[SUCCESS] 烟雾测试通过，继续运行实验..."
echo ""

# ========================================
# 实验1：安全防护（6类攻击各1条）
# ========================================
echo "========================================"
echo "演示1/4: 安全防护"
echo "========================================"
echo "测试6类攻击各1条 × 2配置对比"
echo "预计耗时：60秒"
echo ""

python security_eval.py \
    --samples 6 \
    --sample-mode representative \
    --configs baseline_vanilla_llm,full_system

echo ""

# ========================================
# 实验2：因果推理（10条典型样本）
# ========================================
echo "========================================"
echo "演示2/4: 因果推理"
echo "========================================"
echo "测试10条样本 × 2配置对比"
echo "预计耗时：60秒"
echo ""

python causal_eval.py \
    --samples 10 \
    --configs baseline_naive_rag,full_system

echo ""

# ========================================
# 实验3：检索融合（10对查询）
# ========================================
echo "========================================"
echo "演示3/4: 检索融合"
echo "========================================"
echo "测试10对查询 × 2配置对比"
echo "预计耗时：60秒"
echo ""

python retrieval_eval.py \
    --queries 10 \
    --configs baseline_rag_with_filter,full_system

echo ""

# ========================================
# 实验4：机器遗忘（1个场景演示）
# ========================================
echo "========================================"
echo "演示4/4: 机器遗忘"
echo "========================================"
echo "测试1个场景 × full_system"
echo "预计耗时：60秒"
echo ""

python unlearning_eval.py \
    --scenarios 1 \
    --scenario-name user_data_deletion \
    --config full_system

echo ""

# ========================================
# 生成演示报告
# ========================================
echo "========================================"
echo "生成演示报告"
echo "========================================"
echo ""

python report_builder.py \
    --mode demo \
    --output ../results/reports/demo_slides.html

echo ""
echo "=============================================="
echo "快速演示结束"
echo "=============================================="
echo "结束时间：$(date '+%Y-%m-%d %H:%M:%S')"
echo ""
echo "报告位置："
echo "  - 演示报告: $(pwd)/../results/reports/demo_slides.html"
echo ""
echo "查看报告："
echo "  open ../results/reports/demo_slides.html"
echo ""
echo "提示：完整实验请运行 bash run_all.sh"
echo ""
