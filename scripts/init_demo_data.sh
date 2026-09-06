#!/bin/bash
# Context-Keeper 演示数据初始化脚本

set -e

echo "=== Context-Keeper 演示环境初始化 ==="

# 1. 检查服务状态
echo "[1/5] 检查服务状态..."
if ! curl -s http://localhost:8080/health > /dev/null; then
    echo "❌ 主服务未启动，请先运行: docker-compose up -d"
    exit 1
fi
echo "✅ 主服务运行正常"

if ! curl -s http://localhost:7474 > /dev/null; then
    echo "⚠️  Neo4j未启动，因果推理功能将不可用"
else
    echo "✅ Neo4j运行正常"
fi

if ! curl -s http://localhost:11434/api/tags > /dev/null; then
    echo "⚠️  Ollama未启动，LLM功能将不可用"
else
    echo "✅ Ollama运行正常"
fi

# 2. 清空演示数据
echo -e "\n[2/5] 清空旧演示数据..."
rm -rf demo_data
mkdir -p demo_data/{nursing_records,user_vectors,retrieval_queries}

# 3. 准备护理记录（用于因果推理演示）
echo "[3/5] 准备护理记录..."
cat > demo_data/nursing_records/sample1.txt << 'EOF'
患者李爷爷，78岁，长期服用降压药物（硝苯地平缓释片30mg，每日一次）。
今日上午10:30，护工小王发现李爷爷在洗手间摔倒，意识清醒，诉头晕。
测量血压90/60mmHg，较平时偏低。询问得知患者早餐后立即服药，随后起身如厕时突然眩晕失去平衡。
初步判断：体位性低血压导致跌倒风险。
处理措施：协助患者卧床休息，抬高下肢，监测血压变化，通知家属及主治医生。
EOF

cat > demo_data/nursing_records/sample2.txt << 'EOF'
患者王奶奶，82岁，阿尔茨海默病中期，服用多奈哌齐10mg/日。
夜班护士记录：凌晨2:15，患者独自下床，试图走出病房，称"要回家做饭"。
护士劝阻时患者情绪激动，推搡护士。判断为夜间谵妄发作。
处理：使用安抚技术，陪伴患者回房间，播放其喜爱的老歌，逐渐平复情绪。
未使用约束带，遵循非药物干预原则。家属已告知，建议调整夜间照护策略。
EOF

cat > demo_data/nursing_records/sample3.txt << 'EOF'
患者张大爷，76岁，糖尿病史15年，规律注射胰岛素。
今日下午3点查房发现患者出冷汗、心悸、手抖，测血糖2.8mmol/L。
询问得知患者午饭食量少（仅吃半碗粥），但仍按常规剂量注射胰岛素8单位。
诊断：低血糖反应。
处理：立即口服葡萄糖水50ml，15分钟后复测血糖4.2mmol/L，症状缓解。
已指导患者及家属餐后血糖监测重要性，调整胰岛素方案需医生评估。
EOF

echo "✅ 已创建3个护理记录样本"

# 4. 准备向量数据（用于机器遗忘演示）
echo "[4/5] 准备向量数据..."
cat > demo_data/user_vectors/test_users.json << 'EOF'
{
  "demo_user_001": {
    "name": "张三",
    "role": "护工",
    "vectors": 15
  },
  "demo_user_002": {
    "name": "李四",
    "role": "护士",
    "vectors": 23
  },
  "user_12345": {
    "name": "待遗忘用户",
    "role": "测试账号",
    "vectors": 47,
    "note": "此用户用于演示机器遗忘功能"
  }
}
EOF

echo "✅ 已准备测试用户数据"

# 5. 预热LLM模型
echo "[5/5] 预热LLM模型（避免首次调用延迟）..."
if curl -s http://localhost:11434/api/tags > /dev/null; then
    echo "正在加载Qwen2.5模型到内存..."
    curl -s -X POST http://localhost:11434/api/generate \
        -d '{"model":"qwen2.5:7b","prompt":"测试","stream":false}' > /dev/null
    echo "✅ LLM模型预热完成"
else
    echo "⚠️  跳过LLM预热（Ollama未启动）"
fi

echo -e "\n=========================================="
echo "✅ 演示环境初始化完成！"
echo "=========================================="
echo ""
echo "📋 演示数据位置："
echo "   - 护理记录: demo_data/nursing_records/"
echo "   - 用户数据: demo_data/user_vectors/"
echo ""
echo "🎯 快速测试命令："
echo "   1. 因果推理测试:"
echo "      curl -X POST http://localhost:8080/api/v1/causal/extract \\"
echo "        -H 'Content-Type: application/json' \\"
echo "        -d @demo_data/nursing_records/sample1.txt"
echo ""
echo "   2. 机器遗忘测试:"
echo "      curl -X DELETE http://localhost:8080/api/v1/users/user_12345/forget"
echo ""
echo "   3. 查看Neo4j图谱:"
echo "      打开浏览器访问 http://localhost:7474"
echo ""
echo "📖 完整演示流程请查看: docs/DEMO_GUIDE.md"
echo "=========================================="
