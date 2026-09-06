# 🔍 调试步骤

## 第一步：检查JS文件是否真正加载

1. **打开医生端页面**
2. **按 F12 打开开发者工具**
3. **切换到 Console（控制台）标签**
4. **在Console中输入以下命令并按回车**：

```javascript
console.log(sendMessage.toString().substring(0, 500))
```

5. **查看输出**：
   - ✅ 如果看到 `visualizationKeywords` 字样 → 新代码已加载
   - ❌ 如果没有看到 → 浏览器仍在使用旧代码

## 第二步：强制清除缓存

### 方法A：开发者工具强力清除
1. 保持 F12 开启状态
2. **右键点击浏览器刷新按钮**（地址栏左边的圆圈箭头）
3. 选择 **"清空缓存并硬性重新加载"** 或 **"Empty Cache and Hard Reload"**
4. 再次执行第一步的Console命令验证

### 方法B：手动删除缓存文件
1. 在浏览器地址栏输入：`chrome://settings/clearBrowserData`
2. 时间范围选择：**全部时间**
3. 只勾选：**缓存的图像和文件**
4. 点击 **清除数据**
5. 重新打开医生端页面

## 第三步：测试可视化服务

**在Console中输入**：

```javascript
fetch('http://localhost:5001/health').then(r => r.json()).then(data => console.log('可视化服务状态:', data)).catch(err => console.error('服务错误:', err))
```

**预期输出**：
```
可视化服务状态: {service: "visualization-service", status: "ok"}
```

## 第四步：手动测试拦截器

**在Console中输入**：

```javascript
// 模拟发送可视化请求
const testMessage = "请帮张奶奶生成血压折线图";
const keywords = ['生成.*图', '画.*图', '折线图', '柱状图', '饼图', '趋势图', '可视化'];
const matched = keywords.some(kw => new RegExp(kw).test(testMessage));
console.log('关键词匹配结果:', matched);
console.log('匹配到的关键词:', keywords.filter(kw => new RegExp(kw).test(testMessage)));
```

**预期输出**：
```
关键词匹配结果: true
匹配到的关键词: ["生成.*图", "折线图"]
```

## 第五步：查看网络请求

1. **切换到 Network（网络）标签**
2. **勾选 "Disable cache"**
3. **刷新页面（F5）**
4. **在Filter过滤框输入**：`chat-common.js`
5. **点击 chat-common.js 文件**
6. **切换到 Response 标签**
7. **搜索**：`visualizationKeywords`

如果找到该字符串 → 文件已正确加载  
如果找不到 → 需要检查文件路径或服务器配置

## 第六步：完整测试流程

清除缓存后：

1. 在医生端输入框输入：**"请帮张奶奶生成血压折线图"**
2. **观察Console**，看是否有任何错误或日志
3. **观察Network标签**，看是否发起了到 `localhost:5001/generate` 的请求

### 成功标志：
- ✅ Network中出现对 `localhost:5001/generate` 的 POST 请求
- ✅ 响应状态码 200
- ✅ 系统返回图表路径而非拒绝消息

### 失败排查：
- ❌ 如果Console显示 `CORS error` → 可视化服务需要添加CORS支持
- ❌ 如果Network没有请求 → 拦截器没触发，JS文件未更新
- ❌ 如果返回500错误 → 可视化服务生成图表失败

## 报告格式

请将以下信息发送给我：

1. **第一步结果**：sendMessage函数前500字符是否包含 `visualizationKeywords`？
2. **第三步结果**：可视化服务健康检查返回什么？
3. **第四步结果**：关键词匹配是否为 true？
4. **第五步结果**：Network中看到的chat-common.js内容是否包含拦截器代码？
5. **第六步结果**：发送消息后Console和Network的截图

这样我就能准确定位问题所在！
