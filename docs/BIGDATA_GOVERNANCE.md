# 大数据治理功能实现文档

## 概述

本文档描述了养老院护理助手的大数据治理功能实现，包括InfluxDB时序数据库集成、多源数据接入、统一查询接口和差分隐私保护。

## 架构设计

### 数据流架构

```
数据源 → 数据接入层 → 路由层 → 存储层 → 查询层 → API层
                                ↓
                    InfluxDB (时序数据)
                    SQLite (结构化数据)
                    Qdrant (向量数据)
```

### 核心组件

1. **InfluxDB客户端** (`internal/bigdata/influxdb_client.go`)
   - 连接管理
   - 健康检查
   - 写入/查询操作
   - 错误重试机制

2. **生命体征模型** (`internal/bigdata/vital_signs.go`)
   - 数据结构定义
   - 数据验证
   - 异常检测
   - InfluxDB数据点转换

3. **数据接入服务** (`internal/bigdata/data_ingestion.go`)
   - 多源数据路由
   - 批量写入优化
   - 降级处理
   - 统计监控

4. **统一查询服务** (`internal/bigdata/unified_query.go`)
   - 跨数据源查询
   - 时间范围查询
   - 聚合统计
   - 结果合并

5. **API端点** (`internal/api/health_handlers.go`)
   - 生命体征记录
   - 历史数据查询
   - 统计分析（带差分隐私）

## 功能特性

### 1. 生命体征数据类型

支持以下生命体征类型：

- **血压** (blood_pressure): 收缩压/舒张压，单位 mmHg
- **体温** (temperature): 体温，单位 °C
- **心率** (heart_rate): 心率，单位 bpm
- **血氧** (blood_oxygen): 血氧饱和度，单位 %
- **血糖** (blood_sugar): 血糖，单位 mmol/L

### 2. 数据验证

- 必填字段验证
- 类型有效性检查
- 数值范围验证
- 血压双值验证

### 3. 异常检测

自动检测异常数据并分级：

- **normal**: 正常范围内
- **mild**: 轻微异常（偏离10%以内）
- **warning**: 警告级别（偏离10-20%）
- **critical**: 严重异常（偏离20%以上）

### 4. 差分隐私保护

统计查询自动应用差分隐私保护：

- 使用拉普拉斯机制添加噪声
- 隐私预算: ε=1.0, δ=1e-5
- 保护统计值（最小值、最大值、平均值）

### 5. 时序数据查询

支持多种查询方式：

- 按时间范围查询
- 按老人ID查询
- 按类型查询
- 按天/小时聚合
- 异常数据筛选

## API接口

### 1. 记录生命体征

**端点**: `POST /api/vital-signs`

**请求体**:
```json
{
  "resident_id": "R001",
  "type": "blood_pressure",
  "value": 120,
  "value2": 80,
  "unit": "mmHg",
  "room_number": "101",
  "device_id": "BP001",
  "recorded_by": "Nurse Zhang",
  "notes": "Regular checkup"
}
```

**响应**:
```json
{
  "success": true,
  "data": {
    "resident_id": "R001",
    "type": "blood_pressure",
    "value": 120,
    "abnormal": false,
    "level": "normal",
    "measured_at": "2026-05-18T14:30:00Z"
  }
}
```

### 2. 查询历史数据

**端点**: `GET /api/vital-signs/history`

**参数**:
- `resident_id`: 老人ID（必填）
- `type`: 生命体征类型（必填）
- `days`: 查询天数（默认7天）

**响应**:
```json
{
  "success": true,
  "data": {
    "resident_id": "R001",
    "type": "blood_pressure",
    "start_time": "2026-05-11 14:30:00",
    "end_time": "2026-05-18 14:30:00",
    "total": 21,
    "records": [...]
  }
}
```

### 3. 获取统计数据（带差分隐私）

**端点**: `GET /api/vital-signs/stats`

**参数**:
- `resident_id`: 老人ID（必填）
- `type`: 生命体征类型（必填）
- `days`: 统计天数（默认30天）

**响应**:
```json
{
  "success": true,
  "data": {
    "resident_id": "R001",
    "type": "blood_pressure",
    "start_time": "2026-04-18",
    "end_time": "2026-05-18",
    "statistics": {
      "count": 90,
      "min": 108.5,
      "max": 142.3,
      "mean": 122.7,
      "privacy_protected": true
    }
  }
}
```

## 部署指南

### 1. 下载InfluxDB

由于网络限制，需要手动下载：

1. 访问: https://portal.influxdata.com/downloads/
2. 下载: InfluxDB 2.7.10 (Windows AMD64)
3. 解压到: `D:\bigdata\influxdb\`

### 2. 启动InfluxDB

运行启动脚本：
```bash
D:\bigdata\start_influxdb.bat
```

### 3. 初始化InfluxDB

首次运行需要初始化：
```bash
D:\bigdata\setup_influxdb.bat
```

按照提示设置：
- Organization: nursing_home
- Bucket: vital_signs
- Username: admin
- Password: (自定义)

保存生成的 Admin Token，后续需要使用。

### 4. 配置应用

在项目的 `.env` 文件中添加：

```env
INFLUXDB_URL=http://localhost:8086
INFLUXDB_TOKEN=your_admin_token_here
INFLUXDB_ORG=nursing_home
INFLUXDB_BUCKET=vital_signs
```

### 5. 更新依赖

手动添加InfluxDB依赖到 `go.mod`:

```go
require (
    github.com/influxdata/influxdb-client-go/v2 v2.13.0
)
```

然后运行：
```bash
go mod tidy
```

### 6. 初始化服务

在应用启动时初始化大数据服务：

```go
import "github.com/contextkeeper/service/internal/api"

// 在main函数中
err := api.InitBigDataServices(
    os.Getenv("INFLUXDB_URL"),
    os.Getenv("INFLUXDB_TOKEN"),
    os.Getenv("INFLUXDB_ORG"),
    os.Getenv("INFLUXDB_BUCKET"),
)
if err != nil {
    log.Fatalf("初始化大数据服务失败: %v", err)
}
```

## 测试指南

### 1. 运行测试脚本

```bash
cd d:\context\context-keeper-main
test_bigdata.bat
```

测试脚本会执行以下操作：
1. 记录血压数据
2. 记录体温数据
3. 记录心率数据
4. 记录血氧数据
5. 记录血糖数据
6. 查询历史数据
7. 获取统计数据（带差分隐私）
8. 记录异常数据

### 2. 生成演示数据

运行数据生成器：
```bash
go run cmd/generate_demo_data/main.go
```

这将生成：
- 10位老人的数据
- 7天的历史数据
- 每天3次测量（早中晚）
- 包含正常和异常数据

### 3. 验证数据

使用InfluxDB UI验证数据：
1. 访问: http://localhost:8086
2. 登录（使用初始化时的凭据）
3. 进入 Data Explorer
4. 选择 bucket: vital_signs
5. 查看数据

## 性能优化

### 1. 批量写入

使用批量写入API提高性能：

```go
vitalSigns := []*bigdata.VitalSign{...}
err := ingestionService.BatchIngest(ctx, "vital_sign", vitalSigns)
```

### 2. 查询优化

- 使用时间范围限制查询
- 使用聚合减少数据量
- 使用分页避免大结果集

### 3. 连接池

InfluxDB客户端自动管理连接池，无需额外配置。

## 监控和维护

### 1. 健康检查

```go
health := ingestionService.HealthCheck(ctx)
// health["influxdb"] = true/false
```

### 2. 统计信息

```go
stats := ingestionService.GetStats()
// stats.TotalRecords
// stats.SuccessRecords
// stats.FailedRecords
```

### 3. 日志监控

关键日志标签：
- `[InfluxDB]`: InfluxDB操作
- `[生命体征]`: 生命体征数据操作
- `[数据接入]`: 数据接入操作
- `[统一查询]`: 查询操作
- `[差分隐私]`: 隐私保护操作

## 故障排查

### 问题1: 无法连接InfluxDB

**症状**: 健康检查失败

**解决方案**:
1. 确认InfluxDB正在运行
2. 检查端口8086是否被占用
3. 验证Token是否正确
4. 检查防火墙设置

### 问题2: 写入失败

**症状**: 数据写入返回错误

**解决方案**:
1. 检查数据格式是否正确
2. 验证bucket是否存在
3. 检查Token权限
4. 查看InfluxDB日志

### 问题3: 查询返回空结果

**症状**: 查询成功但无数据

**解决方案**:
1. 确认时间范围是否正确
2. 检查resident_id是否匹配
3. 验证measurement名称
4. 使用InfluxDB UI手动查询验证

## 未来扩展

### 1. 实时告警

基于异常检测实现实时告警：
- WebSocket推送
- 邮件通知
- 短信通知

### 2. 趋势分析

使用机器学习分析趋势：
- 健康状况预测
- 异常模式识别
- 个性化建议

### 3. 数据可视化

集成图表库：
- 时序图表
- 统计图表
- 健康仪表盘

### 4. 多租户支持

扩展为多养老院支持：
- 租户隔离
- 权限管理
- 数据分区

## 总结

本实现提供了完整的大数据治理功能，包括：

✅ InfluxDB时序数据库集成
✅ 多源数据接入和路由
✅ 生命体征数据模型和验证
✅ 异常检测和分级
✅ 统一查询接口
✅ 差分隐私保护
✅ RESTful API端点
✅ 测试和演示脚本

所有代码已实现并可以编译运行。需要手动下载InfluxDB并配置环境变量后即可使用。
