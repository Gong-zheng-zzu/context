# 数据可视化功能使用指南

## 功能说明

Context-Keeper现在支持在对话中**动态生成真实的数据可视化图表**，不再是幻想！

当医生在对话中说"生成血压折线图"、"帮我画个图表"等需求时，系统会：
1. 理解可视化需求
2. 从数据库检索相关数据
3. 调用可视化工具生成真实的PNG图表
4. 返回图表文件路径和访问链接

## 架构设计

```
用户对话
    ↓
Context-Keeper LLM (识别需要可视化)
    ↓
ReAct Agent调用 generate_chart 工具
    ↓
VisualizationTool (Go)
    ↓
HTTP API调用
    ↓
visualization_service.py (Python Flask)
    ↓
matplotlib生成PNG图表
    ↓
返回图表路径给用户
```

## 启动步骤

### 1. 启动可视化服务（必需）

```bash
cd d:/context/context-keeper-main
python tools/visualization_service.py
```

输出：
```
[OK] Visualization Service starting on http://localhost:5001
[OK] API endpoint: POST http://localhost:5001/generate
[OK] Health check: GET http://localhost:5001/health
```

### 2. 启动Context-Keeper主服务

```bash
cd d:/context/context-keeper-main
go run cmd/server/main.go
```

注意：由于Go依赖问题，如果无法编译，可以先使用可视化服务的独立API。

### 3. 测试可视化功能

#### 方法A：直接调用可视化服务API

```bash
curl -X POST http://localhost:5001/generate \
  -H "Content-Type: application/json" \
  -d '{
    "chart_type": "line",
    "title": "血压趋势图",
    "data_context": "2024-07-01 血压 138/85\n2024-07-02 血压 142/88"
  }'
```

响应：
```json
{
  "success": true,
  "chart_url": "http://localhost:5001/chart/chart_abc123.png",
  "chart_path": "D:\\context\\context-keeper-main\\charts\\chart_abc123.png",
  "message": "图表已生成：chart_abc123.png"
}
```

#### 方法B：在对话中使用（需要Context-Keeper主服务运行）

医生端对话：
```
医生: 帮我生成最近7天的血压折线图
```

系统识别到可视化需求后，会自动：
1. 调用 `generate_chart` 工具
2. 传入参数：`{"chart_type":"line", "data_query":"最近7天血压", "title":"血压趋势图"}`
3. 检索相关血压数据
4. 调用可视化服务生成图表
5. 返回：✅ 图表已生成成功！📊 图表文件：D:\...\chart_abc123.png

## 支持的图表类型

1. **折线图 (line)**: 适合显示趋势变化，如血压、血糖、体重等
2. **柱状图 (bar)**: 适合显示分类统计，如各类事件数量
3. **饼图 (pie)**: 适合显示占比分布，如事件类型分布

## 工具参数格式

LLM需要按以下JSON格式调用工具：

```json
{
  "chart_type": "line",
  "data_query": "最近7天血压数据",
  "title": "血压趋势图"
}
```

## 数据解析能力

可视化服务能够从文本中智能解析：
- 血压数据：`2024-07-01 血压 138/85` → 提取日期、收缩压、舒张压
- 如果没有真实数据，会使用示例数据并标注警告

## 解决的问题

### 问题1：LLM承诺生成图表但没有真正生成
**解决方案**：提供真实的工具能力，LLM调用工具后能返回真实的图表文件路径

### 问题2：幻想图表链接
**解决方案**：可视化服务生成真实的PNG文件，返回真实的文件路径和HTTP访问链接

### 问题3：Go依赖问题阻止编译
**解决方案**：可视化服务独立运行，通过HTTP API提供服务，不依赖Go编译成功

## 文件列表

新增文件：
- `internal/agent/visualization_tool.go` - 可视化工具（Go Agent工具）
- `tools/visualization_service.py` - 独立可视化服务（Python Flask）
- `tools/VISUALIZATION_GUIDE.md` - 本文档

修改文件：
- `internal/api/chat_handlers.go` - 注册可视化工具

## 依赖要求

Python依赖：
```bash
pip install flask matplotlib
```

## 故障排查

### 问题：可视化服务启动失败
检查端口5001是否被占用：
```bash
netstat -ano | findstr :5001
```

### 问题：图表生成失败，中文显示乱码
确保已安装中文字体（Microsoft YaHei或SimHei）

### 问题：Context-Keeper无法调用可视化工具
检查可视化服务是否运行：
```bash
curl http://localhost:5001/health
```

应返回：`{"status":"ok","service":"visualization-service"}`

## 下一步优化

1. 支持更多图表类型（散点图、热力图等）
2. 改进数据解析算法，支持更复杂的数据格式
3. 添加图表样式自定义参数
4. 支持多指标对比图表
5. 图表缓存机制避免重复生成

## 测试验证

运行以下命令测试完整流程：

```bash
# 1. 启动可视化服务
python tools/visualization_service.py &

# 2. 测试健康检查
curl http://localhost:5001/health

# 3. 测试图表生成
curl -X POST http://localhost:5001/generate \
  -H "Content-Type: application/json" \
  -d '{"chart_type":"line","title":"测试图表","data_context":"test"}'

# 4. 查看生成的图表
ls -lh charts/
```

预期结果：
- 可视化服务正常启动
- 健康检查返回OK
- 生成真实的PNG图表文件
- 图表可通过浏览器访问
