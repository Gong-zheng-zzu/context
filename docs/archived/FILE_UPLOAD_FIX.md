# 前端文件上传功能修复说明

## 问题描述
前端页面的文件上传功能失败，显示"文件上传失败: 无法获取认证令牌"。

## 根本原因
1. 后端文件上传API (`/api/files/upload`) 需要JWT认证
2. 前端没有发送JWT令牌
3. 前端发送的登录字段名与后端不匹配

## 修复内容

### 1. 添加JWT令牌管理
```javascript
let JWT_TOKEN = ''; // 全局JWT令牌

async function getJWTToken() {
    // 从localStorage获取缓存的token
    // 如果没有，则自动登录获取新token
    // 使用后端简化版本的登录接口（任何user_id和password都可以）
}
```

### 2. 修正登录请求字段
**错误的字段名**：
```javascript
{
    username: 'test_user',  // ❌ 后端不识别
    password: 'test123456'
}
```

**正确的字段名**：
```javascript
{
    user_id: USER_ID,       // ✅ 后端期望的字段
    password: 'health_assistant_2024',
    workspace_id: 'default'
}
```

### 3. 文件上传时添加认证头
```javascript
const response = await fetch(`${API_BASE_URL}/api/files/upload`, {
    method: 'POST',
    headers: {
        'Authorization': `Bearer ${token}` // 添加JWT认证头
    },
    body: formData
});
```

### 4. 处理认证失败
```javascript
if (response.status === 401) {
    localStorage.removeItem('jwt_token');
    JWT_TOKEN = '';
    throw new Error('认证失败，请刷新页面重试');
}
```

### 5. 页面加载时自动获取令牌
```javascript
window.addEventListener('load', async function() {
    await getJWTToken(); // 先获取JWT令牌
    initializeSessions(); // 然后加载历史会话
});
```

## 测试步骤

### 1. 启动后端服务器
```bash
cd d:\context\context-keeper-main
go run cmd/server/main_http.go
```

### 2. 打开前端页面
```
http://localhost:8088/web/user_health_assistant.html
```

### 3. 打开浏览器控制台（F12）
查看是否显示：
```
✅ JWT令牌获取成功
```

### 4. 测试文件上传
1. 点击输入框右侧的📎按钮
2. 选择文件（支持：jpg, png, pdf, doc, docx, txt）
3. 文件会显示预览（文件名、大小、图标）
4. 输入消息或直接点击发送
5. 文件会自动上传并显示在消息中

## 预期效果

### 成功的情况
- ✅ 页面加载时控制台显示"✅ JWT令牌获取成功"
- ✅ 文件选择后显示预览
- ✅ 发送消息时文件自动上传
- ✅ 消息中显示文件附件信息
- ✅ 如果文件包含敏感信息，显示警告标记

### 失败的情况及解决方案

#### 1. "❌ 登录失败，状态码: 401"
**原因**：后端认证服务未启动或配置错误
**解决**：确认后端服务器正在运行

#### 2. "❌ 获取JWT令牌失败: NetworkError"
**原因**：后端服务器未启动或端口不对
**解决**：检查后端是否在 http://localhost:8088 运行

#### 3. "文件上传失败: 认证失败"
**原因**：JWT令牌过期或无效
**解决**：刷新页面重新获取令牌

#### 4. "CORS错误"
**原因**：后端CORS配置问题
**解决**：检查后端CORS中间件配置

## 后端API说明

### 登录接口
```
POST /api/auth/login
Content-Type: application/json

{
    "user_id": "user_123456",
    "password": "any_password",
    "workspace_id": "default"
}

响应：
{
    "success": true,
    "data": {
        "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
        "user_id": "user_123456",
        "workspace_id": "default"
    }
}
```

**注意**：当前是简化版本，任何user_id和password都会通过验证。

### 文件上传接口
```
POST /api/files/upload
Authorization: Bearer <JWT_TOKEN>
Content-Type: multipart/form-data

file: <文件数据>
user_id: <用户ID>

响应：
{
    "success": true,
    "data": {
        "file_id": "file_123456",
        "file_name": "report.pdf",
        "file_size": 102400,
        "file_type": "application/pdf",
        "has_sensitive_data": true,
        "sensitive_types": ["身份证号", "手机号"]
    }
}
```

## 安全说明

### JWT令牌缓存
- JWT令牌存储在 `localStorage` 中
- 令牌有效期：24小时（后端配置）
- 过期后自动重新登录获取新令牌

### 文件上传限制
- 最大文件大小：10MB
- 支持的文件类型：
  - 图片：jpg, jpeg, png, gif, bmp
  - 文档：pdf, doc, docx, txt

### 敏感信息检测
- 后端会自动检测文件中的敏感信息
- 检测到敏感信息会在响应中标记
- 前端会显示警告提示用户

## 文件结构

```
web/
└── user_health_assistant.html  # 主页面（已修复）

修改的函数：
- getJWTToken()           # 新增：获取JWT令牌
- uploadFiles()           # 修改：添加认证头
- window.onload           # 修改：页面加载时获取令牌
```

## 版本信息

- 修复日期：2026-05-15
- 修复版本：v1.1.0
- 修复内容：添加JWT认证支持

---

**修复完成！现在文件上传功能应该可以正常工作了。**
