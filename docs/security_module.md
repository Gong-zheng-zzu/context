# 安全模块文档

## 概述

安全模块(Security Module)提供敏感信息检测、自动脱敏和加密存储功能，专门解决 AI IDE 记忆管理系统中的隐私保护问题。

## 功能特性

### 1. 敏感信息检测

支持检测 12 种敏感信息类型：

| 类型 | 标识 | 说明 |
|------|------|------|
| API Key | api_key | API 密钥 |
| Password | password | 密码 |
| Token | token | 认证令牌 |
| Private Key | private_key | 私钥 |
| Bearer Token | bearer_token | Bearer 令牌 |
| AWS Key | aws_key | AWS 凭证 |
| Database | database_credential | 数据库凭证 |
| Credit Card | credit_card | 信用卡号 |
| SSN | ssn | 社会安全号 |
| Phone | phone | 手机号 |
| Email | email | 邮箱 |
| IP Address | ip_address | IP 地址 |

### 2. 自动脱敏

存储时自动将敏感信息替换为脱敏标记：

- 密码/私钥 → `[暗文]`
- Token/API Key → `[令牌]`
- 信用卡 → `[信用卡]`
- 社保号 → `[社保号]`
- 其他 → `[已脱敏]`

### 3. 审计日志

记录所有敏感信息访问，支持后续安全审计。

### 4. 加密存储

可选的 AES-GCM 加密存储，使用 PBKDF2 密钥派生。

## 使用方法

### 创建检测器

```go
detector := security.NewDetector()
```

### 检测敏感信息

```go
infos := detector.Detect(text)
// 返回所有检测到的敏感信息
```

### 检测并脱敏

```go
redacted, infos := detector.DetectAndRedact(text)
// redacted: 脱敏后的文本
// infos: 检测到的敏感信息列表
```

### 扫描内容

```go
isSensitive, types, summary := detector.ScanContent(text)
// isSensitive: 是否有敏感信息
// types: 敏感类型列表
// summary: 汇总描述
```

### 加密存储

```go
enc, _ := security.NewEncryptor(password)
cipherText, _ := enc.Encrypt(plainText)
```

## 快速函数

```go
// 检测敏感信息
infos := security.DetectSensitiveInfo(text)

// 自动脱敏
redacted := security.RedactSensitiveInfo(text)

// 加密 JSON
cipherText, _ := security.EncryptJSON(data, password)

// 解密 JSON
security.DecryptJSON(cipherText, password, &dest)
```

## 会话存储集成

### 创建安全存储

```go
store, _ := store.NewSessionStoreWithOptions("./data", true, true)
// 参数2: 开启自动脱敏
// 参数3: 开启审计日志
```

### 安全存储消息

```go
store.StoreMessagesWithSecurity(sessionID, messages)
// 自动检测并脱敏后存储
```

### 扫描会话敏感信息

```go
infos, _ := store.ScanSessionForSensitive(sessionID)
```

### 获取审计日志

```go
auditLog := store.GetAuditLog()
```

### 获取安全统计

```go
total, typeCount := store.GetSecurityStats()
```

## 示例

### 输入
```
我的 API key 是 sk-abcdef123456789，密码是 admin123
```

### 输出
```
我的 API key 是 [令牌]，密码是 [暗文]
```

## 参赛亮点

1. **实时保护** - 存储时自动检测脱敏，无需用户手动操作
2. **全面检测** - 支持 12 种常见敏感信息类型
3. **可审计** - 记录所有敏感信息访问，支持追溯
4. **可加密** - 可选 AES 加密存储，军事级安全
5. **即插即用** - 简单几行代码即可集成到现有系统