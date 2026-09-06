# 情绪API测试脚本

## 1. 记录情绪
```bash
curl -X POST http://localhost:8088/api/emotion/record \
  -H "Content-Type: application/json" \
  -d '{
    "userId": "user_123",
    "sessionId": "session_456",
    "emotion": "happy",
    "intensity": 7,
    "note": "今天工作很顺利"
  }'
```

**预期响应：**
```json
{
  "success": true,
  "data": {
    "emotionId": "emotion_xxx",
    "timestamp": 1705334400,
    "message": "情绪记录成功"
  }
}
```

---

## 2. 获取情绪历史
```bash
curl -X GET "http://localhost:8088/api/emotion/history?userId=user_123&days=7"
```

**预期响应：**
```json
{
  "success": true,
  "data": {
    "emotions": [
      {
        "emotionId": "emotion_xxx",
        "emotion": "happy",
        "intensity": 7,
        "note": "今天工作很顺利",
        "date": "2024-01-15",
        "time": "14:30:00"
      }
    ],
    "statistics": {
      "totalRecords": 1,
      "avgIntensity": 7.0,
      "dominantEmotion": "happy",
      "emotionDistribution": {
        "happy": 1
      },
      "trend": "stable"
    }
  }
}
```

---

## 3. 情绪感知对话
```bash
curl -X POST http://localhost:8088/api/chat/emotion-aware \
  -H "Content-Type: application/json" \
  -d '{
    "userId": "user_123",
    "sessionId": "session_456",
    "message": "我今天很累",
    "currentEmotion": "sad",
    "intensity": 3
  }'
```

**预期响应：**
```json
{
  "success": true,
  "data": {
    "reply": "听起来你现在有些难过，要不要聊聊发生了什么？",
    "emotion": "empathetic",
    "memories": [],
    "suggestions": ["深呼吸", "听听音乐", "和朋友聊聊"]
  }
}
```

---

## 使用Postman测试

### 导入到Postman
1. 打开Postman
2. 点击 Import
3. 选择 Raw text
4. 粘贴以下内容：

```json
{
  "info": {
    "name": "心理健康陪伴API",
    "schema": "https://schema.getpostman.com/json/collection/v2.1.0/collection.json"
  },
  "item": [
    {
      "name": "记录情绪",
      "request": {
        "method": "POST",
        "header": [{"key": "Content-Type", "value": "application/json"}],
        "body": {
          "mode": "raw",
          "raw": "{\n  \"userId\": \"user_123\",\n  \"sessionId\": \"session_456\",\n  \"emotion\": \"happy\",\n  \"intensity\": 7,\n  \"note\": \"今天工作很顺利\"\n}"
        },
        "url": {"raw": "http://localhost:8088/api/emotion/record"}
      }
    },
    {
      "name": "获取情绪历史",
      "request": {
        "method": "GET",
        "url": {
          "raw": "http://localhost:8088/api/emotion/history?userId=user_123&days=7",
          "query": [
            {"key": "userId", "value": "user_123"},
            {"key": "days", "value": "7"}
          ]
        }
      }
    },
    {
      "name": "情绪感知对话",
      "request": {
        "method": "POST",
        "header": [{"key": "Content-Type", "value": "application/json"}],
        "body": {
          "mode": "raw",
          "raw": "{\n  \"userId\": \"user_123\",\n  \"sessionId\": \"session_456\",\n  \"message\": \"我今天很累\",\n  \"currentEmotion\": \"sad\",\n  \"intensity\": 3\n}"
        },
        "url": {"raw": "http://localhost:8088/api/chat/emotion-aware"}
      }
    }
  ]
}
```

---

## 前端调用示例

### JavaScript (Fetch API)
```javascript
// 记录情绪
async function recordEmotion(emotion, intensity, note) {
  const response = await fetch('http://localhost:8088/api/emotion/record', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({
      userId: 'user_123',
      sessionId: 'session_456',
      emotion: emotion,
      intensity: intensity,
      note: note
    })
  });
  return await response.json();
}

// 获取情绪历史
async function getEmotionHistory(days = 7) {
  const response = await fetch(
    `http://localhost:8088/api/emotion/history?userId=user_123&days=${days}`
  );
  return await response.json();
}

// 情绪感知对话
async function emotionAwareChat(message, emotion, intensity) {
  const response = await fetch('http://localhost:8088/api/chat/emotion-aware', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({
      userId: 'user_123',
      sessionId: 'session_456',
      message: message,
      currentEmotion: emotion,
      intensity: intensity
    })
  });
  return await response.json();
}
```

### 使用示例
```javascript
// 记录开心情绪
recordEmotion('happy', 8, '完成了重要项目').then(data => {
  console.log('记录成功:', data);
});

// 获取最近7天情绪
getEmotionHistory(7).then(data => {
  console.log('情绪历史:', data.data.emotions);
  console.log('统计数据:', data.data.statistics);
});

// 发送消息
emotionAwareChat('我今天很累', 'sad', 3).then(data => {
  console.log('AI回复:', data.data.reply);
});
```
