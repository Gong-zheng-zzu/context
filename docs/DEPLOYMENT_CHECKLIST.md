# Context-Keeper 部署验证清单

## 🎯 部署前检查

### 代码修改确认
- [x] handlers.go: InitChatService调用已添加（第226行）
- [x] main_http.go: JWT中间件已应用（第384行）
- [x] main_http.go: 速率限制中间件已应用（第385行）
- [x] main_http.go: CORS配置已修复（第70-87行）
- [x] config/.env: CORS和速率限制配置已添加
- [x] 前端3个HTML文件已引用config.js
- [x] caregiver_chat.html重复代码已清理（减少850行）

### 构建状态
- [ ] Docker镜像构建完成
- [ ] 容器成功启动
- [ ] 健康检查通过

---

## 🔍 启动后验证步骤

### 1. 检查服务启动日志
```bash
docker logs context-keeper 2>&1 | grep -E "✅|❌|初始化"
```

**必须看到的日志：**
- ✅ [全局LLM服务] 已通过InitChatService设置全局LLM服务实例
- ✅ 聊天服务初始化完成
- ✅ 聊天上下文服务初始化完成
- ✅ 公开路由注册成功: /api/auth/login
- ✅ 受保护路由注册成功: JWT认证 + 速率限制已应用

### 2. 测试健康检查
```bash
curl http://localhost:8088/health
```
**期望结果：** `{"status":"ok"}`

### 3. 测试4个角色登录

#### 护工登录
```bash
curl -s -X POST http://localhost:8088/api/role/login \
  -H "Content-Type: application/json" \
  -d '{"user_id":"caregiver_001","password":"health_assistant_2024","workspace_id":"default","role":"caregiver"}'
```

#### 医生登录
```bash
curl -s -X POST http://localhost:8088/api/role/login \
  -H "Content-Type: application/json" \
  -d '{"user_id":"doctor_001","password":"health_assistant_2024","workspace_id":"default","role":"doctor"}'
```

#### 家属登录
```bash
curl -s -X POST http://localhost:8088/api/role/login \
  -H "Content-Type: application/json" \
  -d '{"user_id":"family_001","password":"health_assistant_2024","workspace_id":"default","role":"family"}'
```

#### 老人登录
```bash
curl -s -X POST http://localhost:8088/api/role/login \
  -H "Content-Type: application/json" \
  -d '{"user_id":"elder_001","password":"health_assistant_2024","workspace_id":"default","role":"elder"}'
```

**期望结果：** 每个都返回 `{"success":true,"data":{"token":"..."}}`

### 4. 测试JWT认证保护

#### 无token访问（应该失败）
```bash
curl -s -X POST http://localhost:8088/api/chat \
  -H "Content-Type: application/json" \
  -d '{"user_id":"test","message":"hello"}'
```
**期望结果：** `{"success":false,"error":"未提供认证令牌"}` 或 401错误

#### 有token访问（应该成功）
```bash
# 先获取token
TOKEN=$(curl -s -X POST http://localhost:8088/api/role/login \
  -H "Content-Type: application/json" \
  -d '{"user_id":"caregiver_001","password":"health_assistant_2024","workspace_id":"default","role":"caregiver"}' \
  | grep -o '"token":"[^"]*"' | cut -d'"' -f4)

# 使用token访问聊天API
curl -s -X POST http://localhost:8088/api/chat \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"user_id":"caregiver_001","message":"你好，请介绍一下你自己","session_id":"test_001"}'
```
**期望结果：** `{"success":true,"data":{"response":"..."}}`（包含LLM响应）

### 5. 测试速率限制

```bash
# 快速发送10个请求
for i in {1..10}; do
  curl -s -w "\nHTTP_CODE:%{http_code}\n" -X POST http://localhost:8088/api/chat \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $TOKEN" \
    -d "{\"user_id\":\"caregiver_001\",\"message\":\"测试$i\",\"session_id\":\"test_001\"}"
  echo "---"
done
```
**期望结果：** 前几个请求返回200，后面的请求返回429（速率限制）

### 6. 测试前端页面

访问以下URL，确保页面正常加载：
- http://localhost:8088/web/role_selection.html （角色选择页）
- http://localhost:8088/web/caregiver_chat.html （护工端）
- http://localhost:8088/web/doctor_chat.html （医生端）
- http://localhost:8088/web/family_chat.html （家属端）
- http://localhost:8088/web/elder_voice_chat.html （老人端）

**验证点：**
- [ ] 页面加载无JavaScript错误（F12查看控制台）
- [ ] 登录功能正常
- [ ] 聊天功能正常（能收到LLM回复）
- [ ] 文件上传按钮可见
- [ ] 界面主题颜色正确

### 7. 测试文件上传

```bash
# 创建测试文件
echo "这是一个测试文件" > /tmp/test.txt

# 上传文件
curl -s -X POST http://localhost:8088/api/files/upload \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@/tmp/test.txt" \
  -F "user_id=caregiver_001"
```
**期望结果：** `{"success":true,"data":{"file_id":"..."}}`

### 8. 测试安全检测

```bash
curl -s -X POST http://localhost:8088/api/security/scan \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"content":"我的身份证号是110101199001011234，手机号是13800138000","user_id":"caregiver_001"}'
```
**期望结果：** 检测到敏感信息（身份证号、手机号）

---

## ✅ 成功标准

所有以下条件必须满足：

1. ✅ 服务启动无错误
2. ✅ InitChatService日志出现
3. ✅ 4个角色都能成功登录
4. ✅ JWT认证正常工作（无token访问被拒绝）
5. ✅ 聊天API返回LLM响应（不再是"LLM服务未初始化"）
6. ✅ 速率限制生效（快速请求触发429）
7. ✅ 前端4个页面都能正常使用
8. ✅ 文件上传功能正常
9. ✅ 安全检测功能正常

---

## 🚨 如果出现问题

### 问题1: "LLM服务未初始化"
**检查：**
```bash
docker logs context-keeper 2>&1 | grep "InitChatService"
```
**解决：** 如果没有看到InitChatService日志，说明构建没有包含最新代码，需要重新构建

### 问题2: JWT认证不工作
**检查：**
```bash
docker logs context-keeper 2>&1 | grep "受保护路由"
```
**解决：** 确认看到"受保护路由注册成功: JWT认证 + 速率限制已应用"

### 问题3: 前端页面JavaScript错误
**检查：** 浏览器F12控制台
**解决：** 确认config.js和chat-common.js都正确加载

### 问题4: CORS错误
**检查：**
```bash
grep ALLOWED_ORIGINS config/.env
```
**解决：** 确认ALLOWED_ORIGINS包含你访问的域名

---

## 📊 测试报告模板

```
测试时间: ____________________
测试人员: ____________________

[ ] 服务启动正常
[ ] LLM初始化成功
[ ] 护工端登录成功
[ ] 医生端登录成功
[ ] 家属端登录成功
[ ] 老人端登录成功
[ ] JWT认证工作正常
[ ] 聊天功能正常
[ ] 速率限制生效
[ ] 文件上传正常
[ ] 安全检测正常
[ ] 前端页面正常

总体评估: [ ] 通过  [ ] 失败

备注:
_________________________________
_________________________________
```

---

## 🎉 部署完成后

1. 通知团队服务已就绪
2. 提供访问地址和测试账号
3. 监控日志确保无异常
4. 准备好回滚方案（如果需要）

**访问地址：**
- 角色选择: http://localhost:8088/web/role_selection.html
- API文档: 见 web/README.md

**测试账号：**
- 护工: caregiver_001 / health_assistant_2024
- 医生: doctor_001 / health_assistant_2024
- 家属: family_001 / health_assistant_2024
- 老人: elder_001 / health_assistant_2024
