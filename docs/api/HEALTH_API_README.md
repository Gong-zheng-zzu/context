# 健康API使用指南

## 概述

健康API已成功集成到Context-Keeper主服务器中，提供健康信息的记录、查询和摘要功能。所有健康信息都会自动进行敏感信息检测和脱敏处理。

## 启动服务器

### HTTP模式启动

```bash
# 设置环境变量
export HTTP_MODE=true

# 使用HTTP模式编译并运行
go run -tags http cmd/server/main_http.go
```

服务器将在 `http://localhost:8088` 启动。

## API端点

### 1. 健康检查

**端点**: `GET /health`

**描述**: 检查服务器状态

**响应示例**:
```json
{
  "status": "ok",
  "timestamp": 1715600000
}
```

### 2. 记录健康信息

**端点**: `POST /api/health/record`

**描述**: 记录用户的健康信息，支持体检、就诊、用药、症状四种类别

**请求体**:
```json
{
  "user_id": "user_12345678",
  "session_id": "session_abc123",
  "content": "今天去医院体检，身高175cm，体重70kg，血压120/80",
  "category": "体检"
}
```

**参数说明**:
- `user_id` (必填): 用户ID
- `session_id` (必填): 会话ID
- `content` (必填): 健康信息内容
- `category` (必填): 类别，可选值：体检/就诊/用药/症状

**响应示例**:
```json
{
  "success": true,
  "data": {
    "record_id": "550e8400-e29b-41d4-a716-446655440000",
    "redacted_content": "今天去医院体检，身高175cm，体重70kg，血压120/80",
    "sensitive_types": [],
    "category": "体检",
    "timestamp": 1715600000
  },
  "error": ""
}
```

**敏感信息脱敏**:
系统会自动检测并脱敏以下类型的敏感信息：
- 身份证号 → `[REDACTED]`
- 手机号 → `[REDACTED]`
- 邮箱地址 → `[REDACTED]`
- 银行卡号 → `[REDACTED]`

### 3. 查询健康记录历史

**端点**: `GET /api/health/history`

**描述**: 查询用户的健康记录历史，支持时间范围和类别过滤

**查询参数**:
- `user_id` (必填): 用户ID
- `days` (可选): 查询天数，默认30天
- `category` (可选): 类别过滤，可选值：体检/就诊/用药/症状

**请求示例**:
```bash
GET /api/health/history?user_id=user_12345678&days=30&category=体检
```

**响应示例**:
```json
{
  "success": true,
  "data": {
    "records": [
      {
        "record_id": "550e8400-e29b-41d4-a716-446655440000",
        "user_id": "user_12345678",
        "session_id": "session_abc123",
        "content": "今天去医院体检，身高175cm，体重70kg",
        "original_hash": "",
        "category": "体检",
        "timestamp": 1715600000,
        "sensitive_types": [],
        "created_at": "2024-05-13 10:00:00"
      }
    ],
    "statistics": {
      "total_records": 1,
      "category_count": {
        "体检": 1
      },
      "date_range": "30天",
      "sensitive_count": 0
    }
  },
  "error": ""
}
```

### 4. 健康档案摘要

**端点**: `GET /api/health/summary`

**描述**: 获取用户的健康档案摘要，包含分类统计和最新记录

**查询参数**:
- `user_id` (必填): 用户ID

**请求示例**:
```bash
GET /api/health/summary?user_id=user_12345678
```

**响应示例**:
```json
{
  "success": true,
  "data": {
    "user_id": "user_12345678",
    "total_records": 4,
    "category_summary": {
      "体检": {
        "count": 1,
        "latest_record": {
          "record_id": "...",
          "content": "...",
          "timestamp": 1715600000
        },
        "first_record": {
          "record_id": "...",
          "content": "...",
          "timestamp": 1715500000
        }
      },
      "就诊": {
        "count": 1,
        "latest_record": {...},
        "first_record": {...}
      }
    },
    "latest_records": [
      {...},
      {...}
    ],
    "time_range": "2024-05-01 至 2024-05-13"
  },
  "error": ""
}
```

## 测试脚本

项目提供了完整的测试脚本 `test_health_api.sh`，可以快速测试所有API功能。

### 运行测试

```bash
# 给脚本添加执行权限
chmod +x test_health_api.sh

# 运行测试
./test_health_api.sh
```

测试脚本会自动执行以下操作：
1. 健康检查
2. 记录多条不同类别的健康信息
3. 查询全部历史记录
4. 按类别过滤查询
5. 获取健康档案摘要
6. 测试错误情况

## 使用示例

### 使用curl测试

```bash
# 1. 检查服务器状态
curl http://localhost:8088/health

# 2. 记录体检信息
curl -X POST http://localhost:8088/api/health/record \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user_test001",
    "session_id": "session_001",
    "content": "今天体检，身高175cm，体重70kg，血压正常",
    "category": "体检"
  }'

# 3. 查询历史记录
curl "http://localhost:8088/api/health/history?user_id=user_test001&days=30"

# 4. 获取健康档案摘要
curl "http://localhost:8088/api/health/summary?user_id=user_test001"
```

### 使用JavaScript/Fetch

```javascript
// 记录健康信息
async function recordHealth() {
  const response = await fetch('http://localhost:8088/api/health/record', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      user_id: 'user_test001',
      session_id: 'session_001',
      content: '今天体检，身高175cm，体重70kg',
      category: '体检'
    })
  });
  
  const data = await response.json();
  console.log(data);
}

// 查询历史记录
async function getHistory() {
  const response = await fetch(
    'http://localhost:8088/api/health/history?user_id=user_test001&days=30'
  );
  
  const data = await response.json();
  console.log(data);
}
```

## 数据存储

当前版本使用内存存储，数据在服务器重启后会丢失。生产环境建议：

1. 集成数据库存储（MySQL、PostgreSQL等）
2. 添加数据持久化机制
3. 实现数据备份和恢复

## 安全特性

1. **敏感信息自动脱敏**: 自动检测并脱敏身份证、手机号、邮箱等敏感信息
2. **CORS支持**: 支持跨域请求
3. **输入验证**: 严格的参数验证和类型检查
4. **错误处理**: 统一的错误响应格式

## 注意事项

1. 确保服务器以HTTP模式启动（设置 `HTTP_MODE=true`）
2. 健康信息类别必须是：体检/就诊/用药/症状 之一
3. 所有必填字段都必须提供，否则会返回错误
4. 敏感信息会被自动脱敏，原始内容不会被存储

## 故障排查

### 服务器无法启动

检查端口8088是否被占用：
```bash
# Linux/Mac
lsof -i :8088

# Windows
netstat -ano | findstr :8088
```

### API返回404

确认服务器以HTTP模式启动：
```bash
export HTTP_MODE=true
go run -tags http cmd/server/main_http.go
```

### 敏感信息未脱敏

检查 `internal/security` 包是否正确初始化，查看服务器日志。

## 后续开发建议

1. **数据持久化**: 集成数据库存储
2. **用户认证**: 添加JWT或OAuth2认证
3. **权限控制**: 实现基于角色的访问控制
4. **数据加密**: 对敏感数据进行加密存储
5. **审计日志**: 记录所有API访问和数据修改
6. **数据导出**: 支持导出健康档案为PDF或Excel
7. **数据分析**: 提供健康趋势分析和可视化

## 联系方式

如有问题或建议，请联系开发团队。
