#!/bin/bash
# Context-Keeper 交互式验证入口
# 用途：评委提问时即时验证
# 支持3类交互：自定义查询、攻击测试、数据遗忘

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "=============================================="
echo "Context-Keeper 交互式验证"
echo "=============================================="
echo ""

# 检查服务状态
echo "[检查] 验证服务状态..."
if ! curl -s http://localhost:8088/health > /dev/null 2>&1; then
    echo "[错误] Context-Keeper服务未运行"
    echo "请先启动服务：docker-compose up -d"
    exit 1
fi
echo "[OK] 服务运行正常"
echo ""

# 主菜单
while true; do
    echo "=============================================="
    echo "选择演示模式："
    echo "=============================================="
    echo "  1) 自定义查询 - 展示三路检索融合"
    echo "  2) 攻击测试 - 展示安全防护决策链"
    echo "  3) 用户级删除 - 展示隔离会话向量删除验证"
    echo "  4) 因果推理 - 展示PCCM融合算法"
    echo "  q) 退出"
    echo ""
    read -p "请选择 [1-4/q]: " mode
    echo ""

    case $mode in
        1)
            echo "========================================"
            echo "模式1: 自定义查询（检索融合演示）"
            echo "========================================"
            echo "输入查询文本，展示三路检索过程"
            echo "- 向量检索"
            echo "- 知识图谱检索"
            echo "- 时序检索"
            echo "按 Ctrl+C 返回主菜单"
            echo ""
            python3 retrieval_eval.py --interactive
            ;;
        2)
            echo "========================================"
            echo "模式2: 攻击测试（安全防护演示）"
            echo "========================================"
            echo "输入攻击文本，展示安全决策链"
            echo "- ASDF层：异常检测"
            echo "- CASIA层：因果分析"
            echo "- PCCM层：置信度融合"
            echo "按 Ctrl+C 返回主菜单"
            echo ""
            python3 security_eval.py --interactive
            ;;
        3)
            echo "========================================"
            echo "模式3: 用户级删除（隔离会话验证）"
            echo "========================================"
            echo "输入user_id，展示遗忘过程"
            echo "- 遗忘前查询命中率"
            echo "- Qdrant 用户级向量删除"
            echo "- 遗忘后查询命中率"
            echo "- 副作用检测"
            echo "按 Ctrl+C 返回主菜单"
            echo ""
            python3 unlearning_eval.py --interactive
            ;;
        4)
            echo "========================================"
            echo "模式4: 因果推理（PCCM演示）"
            echo "========================================"
            echo "演示因果链抽取过程"
            echo ""

            echo "请输入护理记录文本："
            read -r record_text

            if [ -n "$record_text" ]; then
                # 创建临时JSON输入
                temp_file=$(mktemp)
                echo "{\"text\": \"$record_text\"}" > "$temp_file"

                echo ""
                echo "[请求] 抽取因果链..."
                curl -s -X POST http://localhost:8088/api/v1/causal/extract \
                    -H "Content-Type: application/json" \
                    -d @"$temp_file" | python3 -m json.tool

                rm "$temp_file"
            fi

            echo ""
            read -p "按Enter继续..."
            ;;
        q|Q)
            echo "退出交互式演示"
            exit 0
            ;;
        *)
            echo "[错误] 无效选择，请重试"
            echo ""
            ;;
    esac

    echo ""
done
