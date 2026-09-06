#!/bin/bash

# AI安全防护模块验证脚本

echo "=========================================="
echo "AI安全防护模块验证脚本"
echo "=========================================="
echo ""

# 设置颜色
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 检查Go环境
echo "1. 检查Go环境..."
if command -v go &> /dev/null; then
    echo -e "${GREEN}✓${NC} Go已安装: $(go version)"
else
    echo -e "${RED}✗${NC} Go未安装，请先安装Go"
    exit 1
fi
echo ""

# 检查项目目录
echo "2. 检查项目目录..."
PROJECT_DIR="d:/context/context-keeper-main"
if [ -d "$PROJECT_DIR" ]; then
    echo -e "${GREEN}✓${NC} 项目目录存在: $PROJECT_DIR"
else
    echo -e "${RED}✗${NC} 项目目录不存在"
    exit 1
fi
echo ""

# 检查新创建的文件
echo "3. 检查AI安全模块文件..."
FILES=(
    "internal/security/output_filter.go"
    "internal/security/output_filter_test.go"
    "internal/security/model_dos_protection.go"
    "internal/security/model_dos_protection_test.go"
    "internal/security/data_poisoning_detector.go"
    "internal/security/confidence_scorer.go"
    "internal/security/confidence_scorer_test.go"
    "internal/security/model_access_monitor.go"
    "internal/security/adversarial_detector.go"
    "internal/security/adversarial_detector_test.go"
)

MISSING_FILES=0
for file in "${FILES[@]}"; do
    if [ -f "$PROJECT_DIR/$file" ]; then
        echo -e "${GREEN}✓${NC} $file"
    else
        echo -e "${RED}✗${NC} $file (缺失)"
        MISSING_FILES=$((MISSING_FILES + 1))
    fi
done

if [ $MISSING_FILES -gt 0 ]; then
    echo -e "${RED}发现 $MISSING_FILES 个文件缺失${NC}"
    exit 1
fi
echo ""

# 检查文档文件
echo "4. 检查文档文件..."
DOCS=(
    "AI_SECURITY_README.md"
    "AI_SECURITY_IMPLEMENTATION_SUMMARY.md"
    "examples/ai_security_demo.go"
)

for doc in "${DOCS[@]}"; do
    if [ -f "$PROJECT_DIR/$doc" ]; then
        echo -e "${GREEN}✓${NC} $doc"
    else
        echo -e "${RED}✗${NC} $doc (缺失)"
    fi
done
echo ""

# 统计代码行数
echo "5. 统计代码行数..."
cd "$PROJECT_DIR/internal/security"
TOTAL_LINES=$(wc -l output_filter.go model_dos_protection.go data_poisoning_detector.go confidence_scorer.go model_access_monitor.go adversarial_detector.go 2>/dev/null | tail -1 | awk '{print $1}')
TEST_LINES=$(wc -l *_test.go 2>/dev/null | tail -1 | awk '{print $1}')

echo "  核心代码: $TOTAL_LINES 行"
echo "  测试代码: $TEST_LINES 行"
echo ""

# 运行测试（如果Go可用）
echo "6. 运行单元测试..."
cd "$PROJECT_DIR"

if command -v go &> /dev/null; then
    echo -e "${YELLOW}正在运行测试...${NC}"

    # 测试输出过滤器
    echo "  测试 OutputFilter..."
    go test -v ./internal/security -run TestOutputFilter 2>&1 | grep -E "(PASS|FAIL|ok|FAIL)" | head -5

    # 测试DoS防护
    echo "  测试 ModelDosProtection..."
    go test -v ./internal/security -run TestModelDosProtection 2>&1 | grep -E "(PASS|FAIL|ok|FAIL)" | head -5

    # 测试置信度评分
    echo "  测试 ConfidenceScorer..."
    go test -v ./internal/security -run TestConfidenceScorer 2>&1 | grep -E "(PASS|FAIL|ok|FAIL)" | head -5

    # 测试对抗样本检测
    echo "  测试 AdversarialDetector..."
    go test -v ./internal/security -run TestAdversarialDetector 2>&1 | grep -E "(PASS|FAIL|ok|FAIL)" | head -5

    echo ""
    echo -e "${GREEN}测试完成！${NC}"
else
    echo -e "${YELLOW}跳过测试（Go不可用）${NC}"
fi
echo ""

# 总结
echo "=========================================="
echo "验证完成！"
echo "=========================================="
echo ""
echo "已创建的模块："
echo "  1. OutputFilter - 输出过滤器 (P0)"
echo "  2. ModelDosProtection - DoS防护 (P0)"
echo "  3. DataPoisoningDetector - 数据投毒检测 (P1)"
echo "  4. ConfidenceScorer - 置信度评分 (P1)"
echo "  5. ModelAccessMonitor - 访问监控 (P2)"
echo "  6. AdversarialDetector - 对抗样本检测 (P2)"
echo ""
echo "文档："
echo "  - AI_SECURITY_README.md (使用说明)"
echo "  - AI_SECURITY_IMPLEMENTATION_SUMMARY.md (实现总结)"
echo "  - examples/ai_security_demo.go (使用示例)"
echo ""
echo "下一步："
echo "  1. 查看文档: cat AI_SECURITY_README.md"
echo "  2. 运行示例: cd examples && go run ai_security_demo.go"
echo "  3. 运行测试: cd internal/security && go test -v"
echo ""
