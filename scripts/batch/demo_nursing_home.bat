@echo off
chcp 65001 >nul
echo ========================================
echo 养老院护理助手演示脚本
echo ========================================
echo.
echo 本演示将展示养老院护理助手的核心功能：
echo 1. 日常护理记录
echo 2. 生命体征监测
echo 3. 用药管理
echo 4. 异常事件报告
echo 5. 交接班报告生成
echo 6. 敏感信息保护
echo.
pause

echo.
echo ========================================
echo 场景1: 日常护理记录
echo ========================================
echo.
echo 护士记录：张奶奶今天早上7点协助洗漱，精神状态良好，早餐进食量约80%%
echo.
curl -X POST http://localhost:8088/api/chat ^
  -H "Content-Type: application/json" ^
  -d "{\"message\":\"记录：张奶奶今天早上7点协助洗漱，精神状态良好，早餐进食量约80%%\",\"user_id\":\"nurse_001\",\"session_id\":\"demo_session_1\"}" ^
  2>nul | jq -r ".data.reply" 2>nul || echo [请求失败]
echo.
pause

echo.
echo ========================================
echo 场景2: 生命体征监测
echo ========================================
echo.
echo 护士记录：李爷爷上午9点测量，血压135/85，体温36.8℃，心率78次/分，血氧98%%
echo.
curl -X POST http://localhost:8088/api/chat ^
  -H "Content-Type: application/json" ^
  -d "{\"message\":\"记录李爷爷的生命体征：血压135/85，体温36.8℃，心率78次/分，血氧98%%\",\"user_id\":\"nurse_001\",\"session_id\":\"demo_session_1\"}" ^
  2>nul | jq -r ".data.reply" 2>nul || echo [请求失败]
echo.
pause

echo.
echo ========================================
echo 场景3: 用药管理
echo ========================================
echo.
echo 护士记录：王奶奶上午10点服用降压药（硝苯地平缓释片 30mg），服药后无不良反应
echo.
curl -X POST http://localhost:8088/api/chat ^
  -H "Content-Type: application/json" ^
  -d "{\"message\":\"记录王奶奶用药：上午10点服用硝苯地平缓释片30mg，服药后无不良反应\",\"user_id\":\"nurse_001\",\"session_id\":\"demo_session_1\"}" ^
  2>nul | jq -r ".data.reply" 2>nul || echo [请求失败]
echo.
pause

echo.
echo ========================================
echo 场景4: 异常事件报告
echo ========================================
echo.
echo 护士报告：赵爷爷下午2点体温37.8℃，有点发烧，精神状态较差，已通知医生
echo.
curl -X POST http://localhost:8088/api/chat ^
  -H "Content-Type: application/json" ^
  -d "{\"message\":\"赵爷爷下午2点体温37.8℃，有点发烧，精神状态较差，需要关注\",\"user_id\":\"nurse_001\",\"session_id\":\"demo_session_1\"}" ^
  2>nul | jq -r ".data.reply" 2>nul || echo [请求失败]
echo.
pause

echo.
echo ========================================
echo 场景5: 交接班报告生成
echo ========================================
echo.
echo 护士请求：生成今天的交接班记录
echo.
curl -X POST http://localhost:8088/api/chat ^
  -H "Content-Type: application/json" ^
  -d "{\"message\":\"请帮我生成今天的交接班报告，汇总一下今天记录的所有护理情况\",\"user_id\":\"nurse_001\",\"session_id\":\"demo_session_1\"}" ^
  2>nul | jq -r ".data.reply" 2>nul || echo [请求失败]
echo.
pause

echo.
echo ========================================
echo 场景6: 敏感信息保护演示
echo ========================================
echo.
echo 本系统具有多层敏感信息检测能力，可以识别并保护：
echo - 身份证号、手机号、医保卡号
echo - 生命体征数据（血压、血糖等）
echo - 用药信息、病历号
echo.
echo 测试：尝试记录包含敏感信息的护理记录
echo 护士记录：孙奶奶，身份证号110101195001011234，手机13800138000，血压150/95
echo.
curl -X POST http://localhost:8088/api/chat ^
  -H "Content-Type: application/json" ^
  -d "{\"message\":\"记录：孙奶奶，身份证号110101195001011234，手机13800138000，血压150/95\",\"user_id\":\"nurse_001\",\"session_id\":\"demo_session_1\"}" ^
  2>nul | jq -r ".data" 2>nul || echo [请求失败]
echo.
echo 注意：系统会检测到敏感信息并给出安全警告
echo.
pause

echo.
echo ========================================
echo 场景7: 攻防对抗演示
echo ========================================
echo.
echo 本系统具有防绕过能力，即使攻击者尝试各种变形，也能识别敏感信息
echo.
echo 详细的攻防演示请运行：demo_attack_defense.bat
echo.
pause

echo.
echo ========================================
echo 演示完成
echo ========================================
echo.
echo 养老院护理助手的核心优势：
echo ✅ 快速记录：减少护理人员文书负担
echo ✅ 智能识别：自动识别异常情况并提供建议
echo ✅ 记忆功能：记住每位老人的护理情况和健康状况
echo ✅ 隐私保护：多层敏感信息检测，本地加密存储
echo ✅ 交接班辅助：自动生成清晰的交接班报告
echo.
echo 访问 Web 界面：http://localhost:8088/
echo.
pause
