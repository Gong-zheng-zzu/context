# Context-Keeper 系统安装指南（简化版）

## 🚀 5分钟快速安装

### 前置条件
1. **安装Docker Desktop**
   - Windows/Mac: https://www.docker.com/products/docker-desktop/
   - Linux: `curl -fsSL https://get.docker.com | sh`

2. **克隆代码**
   ```bash
   git clone <仓库地址> context-keeper
   cd context-keeper
   ```

### 一键启动（推荐）

#### Windows用户：
双击运行 `quick-start.bat`

#### Linux/Mac用户：
```bash
chmod +x quick-start.sh
./quick-start.sh
```

### 手动启动

```bash
# 1. 复制配置文件
cp .env.example .env

# 2. 编辑.env，修改密码
nano .env  # 或 notepad .env

# 3. 启动所有服务
docker-compose up -d

# 4. 等待30秒后访问
# http://localhost:8088
```

---

## 📱 访问系统

- **Web界面**: http://localhost:8088
- **默认账号**: `doctor_wang`
- **默认密码**: 查看`.env`中的`DEMO_AUTH_PASSWORD`

---

## 🧪 插入测试数据

```bash
cd tools
go run insert_test_data.go
```

然后在聊天界面输入："查询张奶奶最近的血压数据"

---

## 🔧 常见问题

### 启动失败？
```bash
# 查看日志
docker-compose logs -f

# 重启服务
docker-compose restart
```

### 端口冲突？
编辑 `docker-compose.yml`，修改端口映射：
```yaml
ports:
  - "8089:8088"  # 将8088改为8089
```

### 内存不足？
关闭不需要的服务：
```bash
docker-compose stop qdrant  # 关闭向量数据库（可选）
```

---

## 📚 完整文档

详细安装步骤请查看：[INSTALLATION_GUIDE.md](./INSTALLATION_GUIDE.md)

---

## 🛑 停止服务

```bash
docker-compose down
```

---

## 💡 最低配置要求

- CPU: 4核
- 内存: 16GB（推荐32GB）
- 存储: 100GB

**配置不够？** 查看文档中的"云端混合部署"方案。

---

## ✅ 安装成功检查

访问以下地址，全部正常即安装成功：

- ✅ http://localhost:8088 （主界面）
- ✅ http://localhost:7474 （Neo4j）
- ✅ http://localhost:6333/dashboard （Qdrant）
- ✅ http://localhost:5001/health （可视化服务）

---

祝使用愉快！🎉
