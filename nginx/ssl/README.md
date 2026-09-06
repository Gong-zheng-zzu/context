# SSL证书说明

## 如何生成自签名证书（用于测试）

```bash
# 生成私钥和证书
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout key.pem \
  -out cert.pem \
  -subj "/C=CN/ST=Beijing/L=Beijing/O=Context-Keeper/CN=localhost"
```

## 生产环境证书

生产环境请使用正规CA签发的证书，例如：
- Let's Encrypt (免费)
- 阿里云SSL证书
- 腾讯云SSL证书

将证书文件放置在此目录：
- cert.pem - 证书文件
- key.pem - 私钥文件
