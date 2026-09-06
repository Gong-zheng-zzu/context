# 养老院护理助手 - 多角色系统

## 系统概述

本系统为养老院护理场景设计，实现了4个不同角色的端口，每个角色有不同的功能、界面风格和数据推送逻辑。

## 系统架构

```
┌─────────────────────────────────────────────────────────────┐
│                        前端界面层                              │
├──────────────┬──────────────┬──────────────┬─────────────────┤
│   护工端      │    医生端     │    家属端     │     老人端       │
│ (移动优先)    │  (桌面优先)   │  (移动优先)   │  (超大字体)      │
└──────────────┴──────────────┴──────────────┴─────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                        API网关层                              │
│              JWT认证 + 角色权限验证                            │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                       业务逻辑层                              │
├──────────────┬──────────────┬──────────────┬─────────────────┤
│  角色管理     │  告警引擎     │  推荐引擎     │   推送服务       │
│ (roles.go)   │(alert_engine)│(recommendation)│(push_service)  │
└──────────────┴──────────────┴──────────────┴─────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                       数据存储层                              │
├──────────────┬──────────────┬──────────────┬─────────────────┤
│  内存存储     │  InfluxDB     │  健康记录     │   关系映射       │
│ (健康记录)    │ (时序数据)    │  (脱敏后)     │  (用户-老人)     │
└──────────────┴──────────────┴──────────────┴─────────────────┘
```

## 角色定位

### 1. 护工端 (Caregiver Portal)

**用户画像**: 30-50岁，初中到高中学历，工作繁忙

**核心功能**:
- 快速记录生命体征（血压、心率、体温、血糖）
- 记录日常护理（进食、饮水、翻身、活动）
- 异常上报（跌倒、走失等）
- 待办清单（今天需要关注的老人）
- 实时告警推送

**界面特点**:
- 移动优先设计
- 大按钮，易点击
- 蓝绿色调（医疗感）
- 快捷操作面板

**权限**:
- ✅ 查看负责的老人
- ✅ 编辑健康记录
- ✅ 接收护理告警
- ❌ 查看所有老人
- ❌ 开处方

### 2. 医生端 (Doctor Portal)

**用户画像**: 30-60岁，医学专业，需要专业数据

**核心功能**:
- 查看所有患者列表（按病情严重程度排序）
- 健康趋势图表（血压、血糖曲线）
- 异常预警面板（红色高亮）
- 电子病历查看
- 医嘱开具（处方、检查单）
- 大数据分析（本周健康报告、疾病分布）
- AI辅助诊断建议

**界面特点**:
- 桌面优先设计
- 多列布局，数据密集
- 深蓝色调（专业严谨）
- 数据可视化图表

**权限**:
- ✅ 查看所有老人
- ✅ 编辑健康记录
- ✅ 开处方
- ✅ 查看敏感数据
- ✅ 接收医疗告警
- ✅ 管理老人信息

### 3. 家属端 (Family Portal)

**用户画像**: 30-60岁，子女或亲属，关心老人但不在身边

**核心功能**:
- 老人今日状态（一句话总结+笑脸图标）
- 最近护理记录（时间线展示）
- 健康趋势（简化版图表，易懂）
- 异常通知（推送到手机）
- 留言板（给老人留言，护工代读）
- 费用查询（护理费、医疗费）
- 探视预约

**界面特点**:
- 移动优先设计
- 卡片式布局
- 暖色调（橙色、粉色）
- 温馨亲切的文案

**权限**:
- ✅ 查看自己的亲属
- ✅ 接收异常通知
- ❌ 编辑健康记录
- ❌ 查看敏感数据（脱敏后）
- ❌ 查看所有老人

### 4. 老人端 (Elder Portal)

**用户画像**: 70-90岁，部分有智能手机，部分没有

**核心功能**:
- 一键呼叫护工（超大按钮）
- 我的健康（用笑脸和简单文字显示）
- 今日活动（今天有什么活动）
- 家人留言（语音播放）
- 用药提醒（弹窗+语音）
- 娱乐功能（听歌、看新闻）

**界面特点**:
- 超大字体（24px+）
- 高对比度
- 极简设计，每屏1-2个功能
- 语音交互支持

**权限**:
- ✅ 查看自己的信息
- ✅ 呼叫护工
- ✅ 接收用药提醒
- ❌ 编辑健康记录
- ❌ 查看其他老人

**特殊设计**:
- 提供"访客模式"：家属可以用老人的账号登录，帮老人查看
- 提供"床头屏模式"：简化版，只显示呼叫按钮和基本信息
- 系统核心功能不依赖老人端，老人不用也不影响护理流程

## 角色权限矩阵

| 权限 | 护工 | 医生 | 家属 | 老人 |
|------|------|------|------|------|
| 查看所有老人 | ❌ | ✅ | ❌ | ❌ |
| 查看负责的老人 | ✅ | ✅ | ✅ | ✅ (仅自己) |
| 编辑健康记录 | ✅ | ✅ | ❌ | ❌ |
| 开处方 | ❌ | ✅ | ❌ | ❌ |
| 查看敏感数据 | ❌ | ✅ | ❌ | ❌ |
| 接收告警 | ✅ | ✅ | ✅ | ✅ |
| 呼叫护工 | ❌ | ❌ | ❌ | ✅ |
| 管理老人信息 | ❌ | ✅ | ❌ | ❌ |

## 推送规则

### 护工端推送
- **血压异常**: "张奶奶血压偏高（145/92 mmHg），建议测量并记录"
- **进食不足**: "李爷爷今天进食量不足，需要关注"
- **翻身提醒**: "该为王奶奶翻身了，预防褥疮"
- **饮水提醒**: "提醒张奶奶喝水，保持充足水分"

### 医生端推送
- **连续异常**: "301房间王奶奶连续3天血压升高，建议调整用药"
- **趋势分析**: "本周有5位老人出现发烧，需要排查感染"
- **用药建议**: "张奶奶近30天平均血压145/92 mmHg，建议调整降压药剂量"
- **检查建议**: "李爷爷血压异常率60%，建议进行心血管系统检查"

### 家属端推送
- **日常报告**: "您的母亲今天精神状态良好，血压正常"
- **异常通知**: "您的父亲今天有轻微发烧，医生已查看"
- **探视建议**: "今天是周末，张奶奶很想念您，有空可以来探望"

### 老人端推送
- **用药提醒**: "该吃药了"（带语音）
- **活动建议**: "今天天气不错，适合散步"
- **休息提醒**: "该准备休息了，早睡早起身体好"

## 大数据分析

### 告警引擎 (Alert Engine)

**检测逻辑**:
1. 实时检测生命体征异常
2. 检测连续3天的趋势异常
3. 检测护理记录异常（进食不足、饮水不足）
4. 生成不同角色的告警消息

**告警级别**:
- `info`: 信息提醒
- `warning`: 警告（需关注）
- `critical`: 严重（需立即处理）

### 推荐引擎 (Recommendation Engine)

**推荐类型**:
- **护理提醒**: 翻身、饮水、活动
- **监测建议**: 增加测量频率
- **诊疗建议**: 用药调整、检查建议
- **探视建议**: 基于老人情绪状态

**优先级**: 1-5（5最高）

### 差分隐私保护

对统计数据应用拉普拉斯噪声，保护个人隐私：
- 平均值、最大值、最小值添加噪声
- epsilon=1.0, delta=1e-5
- 标记 `privacy_protected: true`

## API接口

### 认证接口

#### 角色登录
```
POST /api/role/login
Content-Type: application/json

{
  "user_id": "caregiver_001",
  "password": "password123",
  "role": "caregiver"
}

Response:
{
  "success": true,
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIs...",
    "user_id": "caregiver_001",
    "role": "caregiver",
    "role_name": "护工",
    "expires_in": 86400
  }
}
```

### 仪表盘接口

#### 获取仪表盘数据
```
GET /api/dashboard
Authorization: Bearer {token}

Response:
{
  "success": true,
  "data": {
    "role": "caregiver",
    "role_name": "护工",
    "user_id": "caregiver_001",
    "elders": [...],
    "alerts": [...],
    "recommendations": [...],
    "statistics": {...}
  }
}
```

#### 获取告警信息
```
GET /api/alerts
Authorization: Bearer {token}
```

#### 获取推荐信息
```
GET /api/recommendations
Authorization: Bearer {token}
```

### 健康记录接口

#### 记录生命体征
```
POST /api/health/vital-signs
Authorization: Bearer {token}
Content-Type: application/json

{
  "resident_id": "elder_001",
  "type": "blood_pressure",
  "value": 145,
  "value2": 92,
  "unit": "mmHg",
  "recorded_by": "caregiver_001"
}
```

#### 查询生命体征历史
```
GET /api/health/vital-signs/history?resident_id=elder_001&type=blood_pressure&days=7
Authorization: Bearer {token}
```

#### 查询生命体征统计（带差分隐私）
```
GET /api/health/vital-signs/stats?resident_id=elder_001&type=blood_pressure&days=30
Authorization: Bearer {token}
```

### 特殊功能接口

#### 老人呼叫护工
```
POST /api/call-caregiver
Authorization: Bearer {token}
Content-Type: application/json

{
  "elder_id": "elder_001",
  "reason": "需要帮助"
}
```

#### 家属留言
```
POST /api/family-message
Authorization: Bearer {token}
Content-Type: application/json

{
  "elder_id": "elder_001",
  "message": "妈妈，周末我会来看您"
}
```

## 数据模型

### 老人-用户关系
```go
type ElderRelation struct {
    ElderID  string // 老人ID
    UserID   string // 用户ID
    Role     Role   // 用户角色
    Relation string // 关系描述（如"负责护工"、"女儿"）
}
```

### 告警信息
```go
type Alert struct {
    ID          string
    ElderID     string
    ElderName   string
    Level       AlertLevel // info/warning/critical
    Type        string
    Title       string
    Message     string
    Timestamp   time.Time
    TargetRoles []string
    Data        map[string]interface{}
}
```

### 推荐信息
```go
type Recommendation struct {
    ID          string
    ElderID     string
    ElderName   string
    Type        string
    Title       string
    Content     string
    Priority    int // 1-5
    TargetRoles []string
    Timestamp   time.Time
}
```

## 演示数据

### 老人信息
- **elder_001**: 张奶奶，82岁，301房间
- **elder_002**: 李爷爷，78岁，302房间
- **elder_003**: 王奶奶，85岁，303房间

### 用户信息
- **caregiver_001**: 护工（负责张奶奶、李爷爷）
- **caregiver_002**: 护工（负责王奶奶）
- **doctor_001**: 医生（负责所有老人）
- **family_001**: 家属（张奶奶的女儿）
- **family_002**: 家属（李爷爷的儿子）
- **family_003**: 家属（王奶奶的女儿）

## 部署指南

### 1. 启动服务器
```bash
# 编译
go build -o server.exe cmd/server/main.go

# 运行
./server.exe
```

### 2. 初始化InfluxDB（可选）
```bash
# 设置环境变量
export INFLUXDB_URL=http://localhost:8086
export INFLUXDB_TOKEN=your-token
export INFLUXDB_ORG=your-org
export INFLUXDB_BUCKET=nursing_home
```

### 3. 运行演示脚本
```bash
demo_multi_role.bat
```

### 4. 访问Web界面
- 护工端: `web/caregiver_portal.html`
- 医生端: `web/doctor_portal.html`
- 家属端: `web/family_portal.html`
- 老人端: `web/elder_portal.html`

## 安全特性

1. **JWT认证**: 所有API请求需要Bearer Token
2. **角色验证**: 中间件检查用户角色和权限
3. **数据隔离**: 用户只能访问有权限的数据
4. **敏感信息脱敏**: 健康记录自动脱敏
5. **差分隐私**: 统计数据添加噪声保护

## 技术栈

- **后端**: Go + Gin
- **数据库**: InfluxDB (时序数据) + 内存存储 (健康记录)
- **认证**: JWT
- **前端**: 原生HTML/CSS/JavaScript
- **推送**: WebSocket (计划中)

## 未来扩展

1. **WebSocket实时推送**: 实现真正的实时通知
2. **视频通话**: 家属与老人视频通话
3. **AI辅助诊断**: 基于历史数据的疾病预测
4. **语音交互**: 老人端语音控制
5. **床头屏终端**: 专用硬件设备
6. **移动App**: iOS/Android原生应用

## 联系方式

如有问题，请联系开发团队。
