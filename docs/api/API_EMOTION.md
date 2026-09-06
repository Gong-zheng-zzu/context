# 心理健康陪伴系统 - API接口文档

## 📋 接口概览

| 接口 | 方法 | 说明 | 优先级 |
|------|------|------|--------|
| `/api/emotion/record` | POST | 记录情绪 | ⭐⭐⭐⭐⭐ |
| `/api/emotion/history` | GET | 获取情绪历史 | ⭐⭐⭐⭐⭐ |
| `/api/emotion/analysis` | GET | 情绪分析 | ⭐⭐⭐⭐ |
| `/api/chat/emotion-aware` | POST | 情绪感知对话 | ⭐⭐⭐⭐⭐ |
| `/api/user/register` | POST | 用户注册 | ⭐⭐⭐ |
| `/api/user/login` | POST | 用户登录 | ⭐⭐⭐ |

---

## 1. 记录情绪

### 接口信息
- **URL**: `/api/emotion/record`
- **方法**: `POST`
- **说明**: 记录用户的情绪状态

### 请求参数
```json
{
  "userId": "user_123",           // 用户ID（必填）
  "sessionId": "session_456",     // 会话ID（必填）
  "emotion": "happy",             // 情绪类型（必填）
  "intensity": 7,                 // 情绪强度 1-10（必填）
  "note": "今天工作很顺利",        // 备注（可选）
  "timestamp": 1705334400         // Unix时间戳（可选，后端可自动生成）
}
```

### 情绪类型枚举
```
happy    - 开心 😊
neutral  - 平静 😐
sad      - 难过 😔
anxious  - 焦虑 😢
angry    - 愤怒 😡
```

### 响应示例
```json
{
  "success": true,
  "data": {
    "emotionId": "emotion_789",
    "timestamp": 1705334400,
    "message": "情绪记录成功"
  }
}
```

### 错误响应
```json
{
  "success": false,
  "error": {
    "code": "INVALID_EMOTION",
    "message": "无效的情绪类型"
  }
}
```

---

## 2. 获取情绪历史

### 接口信息
- **URL**: `/api/emotion/history`
- **方法**: `GET`
- **说明**: 获取用户的情绪历史记录

### 请求参数（Query）
```
userId: user_123      // 用户ID（必填）
days: 7               // 获取最近N天的数据（可选，默认7天）
limit: 100            // 返回记录数量（可选，默认100）
```

### 响应示例
```json
{
  "success": true,
  "data": {
    "emotions": [
      {
        "emotionId": "emotion_001",
        "emotion": "happy",
        "intensity": 7,
        "note": "今天工作很顺利",
        "timestamp": 1705334400,
        "date": "2024-01-15",
        "time": "14:30:00"
      },
      {
        "emotionId": "emotion_002",
        "emotion": "sad",
        "intensity": 3,
        "note": "项目被批评了",
        "timestamp": 1705248000,
        "date": "2024-01-14",
        "time": "10:15:00"
      }
    ],
    "statistics": {
      "totalRecords": 15,
      "avgIntensity": 6.2,
      "dominantEmotion": "happy",
      "emotionDistribution": {
        "happy": 6,
        "neutral": 4,
        "sad": 3,
        "anxious": 1,
        "angry": 1
      },
      "trend": "improving"  // improving/stable/declining
    }
  }
}
```

---

## 3. 情绪分析

### 接口信息
- **URL**: `/api/emotion/analysis`
- **方法**: `GET`
- **说明**: 使用LLM分析用户的情绪模式

### 请求参数（Query）
```
userId: user_123      // 用户ID（必填）
days: 7               // 分析最近N天的数据（可选，默认7天）
```

### 响应示例
```json
{
  "success": true,
  "data": {
    "summary": "最近7天你的情绪整体偏积极，平均强度为6.2分。周一到周三情绪较低，可能与工作压力有关，周四之后明显改善。",
    "pattern": {
      "type": "weekly_cycle",
      "description": "工作日情绪偏低，周末情绪较好"
    },
    "triggers": [
      {
        "keyword": "工作",
        "frequency": 8,
        "sentiment": "negative"
      },
      {
        "keyword": "朋友",
        "frequency": 5,
        "sentiment": "positive"
      }
    ],
    "suggestions": [
      "建议每天安排30分钟户外活动，有助于缓解工作压力",
      "尝试深呼吸放松练习，每天3次，每次5分钟",
      "保持与朋友的联系，社交活动对情绪有积极影响"
    ],
    "riskLevel": "low",  // low/medium/high
    "riskWarning": null  // 如果有风险，这里会有提示信息
  }
}
```

### 风险等级说明
```
low     - 情绪健康，无需特别关注
medium  - 情绪波动较大，建议关注
high    - 持续低落或出现危机信号，建议寻求专业帮助
```

---

## 4. 情绪感知对话

### 接口信息
- **URL**: `/api/chat/emotion-aware`
- **方法**: `POST`
- **说明**: 根据用户当前情绪调整AI回复语气

### 请求参数
```json
{
  "userId": "user_123",
  "sessionId": "session_456",
  "message": "我今天很累，不想做任何事",
  "currentEmotion": "sad",      // 当前情绪（可选）
  "intensity": 3,               // 情绪强度（可选）
  "includeMemory": true         // 是否检索历史记忆（可选，默认true）
}
```

### 响应示例
```json
{
  "success": true,
  "data": {
    "reply": "听起来你今天确实很辛苦。我记得上周你也提到过工作压力大，这次是同样的原因吗？要不要聊聊发生了什么？",
    "emotion": "empathetic",
    "memories": [
      {
        "content": "上周用户提到工作压力大，加班到很晚",
        "timestamp": 1704729600,
        "relevance": 0.85
      }
    ],
    "suggestions": [
      "休息一下",
      "听听音乐",
      "和朋友聊聊"
    ]
  }
}
```

---

## 5. 用户注册

### 接口信息
- **URL**: `/api/user/register`
- **方法**: `POST`
- **说明**: 用户注册

### 请求参数
```json
{
  "username": "zhangsan",
  "password": "password123",
  "email": "zhangsan@example.com",  // 可选
  "nickname": "小张"                 // 可选
}
```

### 响应示例
```json
{
  "success": true,
  "data": {
    "userId": "user_123",
    "username": "zhangsan",
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "expiresIn": 86400  // 24小时
  }
}
```

---

## 6. 用户登录

### 接口信息
- **URL**: `/api/user/login`
- **方法**: `POST`
- **说明**: 用户登录

### 请求参数
```json
{
  "username": "zhangsan",
  "password": "password123"
}
```

### 响应示例
```json
{
  "success": true,
  "data": {
    "userId": "user_123",
    "username": "zhangsan",
    "nickname": "小张",
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "expiresIn": 86400
  }
}
```

---

## 🔐 认证方式

### 请求头
所有需要认证的接口都需要在请求头中携带token：

```
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

---

## 📊 数据库设计

### emotions表
```sql
CREATE TABLE emotions (
    id SERIAL PRIMARY KEY,
    emotion_id VARCHAR(50) UNIQUE NOT NULL,
    user_id VARCHAR(50) NOT NULL,
    session_id VARCHAR(50) NOT NULL,
    emotion VARCHAR(20) NOT NULL,  -- happy/neutral/sad/anxious/angry
    intensity INT NOT NULL CHECK (intensity >= 1 AND intensity <= 10),
    note TEXT,
    timestamp BIGINT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_user_timestamp (user_id, timestamp),
    INDEX idx_session (session_id)
);
```

### users表
```sql
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    user_id VARCHAR(50) UNIQUE NOT NULL,
    username VARCHAR(50) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    email VARCHAR(100),
    nickname VARCHAR(50),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

---

## 🧪 Mock数据（供前端开发使用）

### 情绪历史Mock数据
```json
{
  "success": true,
  "data": {
    "emotions": [
      {"emotion": "happy", "intensity": 7, "date": "2024-01-15", "note": "工作顺利"},
      {"emotion": "neutral", "intensity": 5, "date": "2024-01-14", "note": "平常的一天"},
      {"emotion": "sad", "intensity": 3, "date": "2024-01-13", "note": "项目被批评"},
      {"emotion": "happy", "intensity": 8, "date": "2024-01-12", "note": "周末和朋友聚会"},
      {"emotion": "anxious", "intensity": 4, "date": "2024-01-11", "note": "deadline临近"},
      {"emotion": "neutral", "intensity": 6, "date": "2024-01-10", "note": ""},
      {"emotion": "happy", "intensity": 7, "date": "2024-01-09", "note": "完成了重要任务"}
    ],
    "statistics": {
      "avgIntensity": 5.7,
      "dominantEmotion": "happy",
      "emotionDistribution": {
        "happy": 3,
        "neutral": 2,
        "sad": 1,
        "anxious": 1,
        "angry": 0
      }
    }
  }
}
```

---

## 🚀 开发优先级

### 第一阶段（MVP）
1. ✅ 情绪记录 (`POST /api/emotion/record`)
2. ✅ 情绪历史 (`GET /api/emotion/history`)
3. ✅ 基础对话 (`POST /api/chat/emotion-aware`)

### 第二阶段
4. ✅ 情绪分析 (`GET /api/emotion/analysis`)
5. ✅ 用户认证 (`POST /api/user/register`, `POST /api/user/login`)

---

## 📝 前后端协作流程

### 第1天：接口对齐
- 后端：创建API接口框架，返回Mock数据
- 前端：根据接口文档开始开发

### 第2-7天：并行开发
- 后端：实现真实逻辑（数据库、LLM集成）
- 前端：使用Mock数据完成UI和交互

### 第8-10天：联调测试
- 前端切换到真实API
- 解决对接问题
- 完整流程测试

---

## 🔧 后端技术栈

```go
// 框架
- Gin (HTTP Server)
- GORM (ORM)
- JWT (认证)

// 数据库
- PostgreSQL (关系数据)
- Qdrant (向量数据库)

// AI
- Ollama (本地LLM)
- qwen2.5:7b (情绪分析)
```

---

## 📞 联系方式

如有接口问题，请及时沟通：
- 后端负责人：[你的联系方式]
- 前端负责人：[队友的联系方式]

---

**版本**: v1.0  
**更新日期**: 2024-01-15  
**维护者**: Context-Keeper Team
