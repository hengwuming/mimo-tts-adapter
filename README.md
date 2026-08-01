# MiMo TTS Adapter for Legado

一个面向个人使用的 Legado HTTP-TTS 适配服务：接收段落文本，调用小米 MiMo `mimo-v2.5-tts`，将 Base64 MP3 解码后以 `audio/mpeg` 返回。

## 快速开始

### 1. 配置环境变量

```bash
cp .env.example .env
```

至少填写：

- `ADAPTER_AUTH_TOKEN`：Legado 调用适配器时使用的 Bearer Token
- `MIMO_API_KEY`：服务端使用的小米 MiMo API Key
- `PUBLIC_BASE_URL`：公网 HTTPS 地址，例如 `https://tts.example.com`

完整配置见 `.env.example`。`.env`、API Key 和真实 Token 不要提交到 Git，也不要写入 Dockerfile。

### 2. Docker 启动

```bash
docker build -t mimo-tts-adapter .
docker run -d --name mimo-tts-adapter --restart unless-stopped \
  --env-file .env -p 127.0.0.1:8080:8080 mimo-tts-adapter
```

服务默认监听 `:8080`。建议只绑定本机，由 Nginx 负责 HTTPS 反向代理，并保留应用层 Bearer 认证。

### 3. 配置 Legado

启动后访问 `https://你的域名/rule` 获取导入模板，将其中的 `REPLACE_WITH_ADAPTER_TOKEN` 替换为 `ADAPTER_AUTH_TOKEN`。这里不能填写 `MIMO_API_KEY`。

推荐使用 POST 规则，避免长文本进入 GET 查询日志。服务也兼容：

- `GET /tts?text=...&voice=冰糖&speed=25`
- `POST /tts`，请求体：`{"text":"...","voice":"冰糖","speed":25}`
- `GET /healthz`：健康检查
- `GET /rule`：安全的 Legado 规则模板

## Nginx 示例

```nginx
server {
    listen 443 ssl http2;
    server_name tts.example.com;

    client_max_body_size 64k;
    proxy_read_timeout 120s;
    proxy_send_timeout 120s;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header Authorization $http_authorization;
    }
}
```

请按实际情况配置证书。不要依赖域名隐藏服务，`/tts` 仍必须使用 Bearer Token。

## 安全和费用提示

- 小米 API Key 只从服务端环境变量读取，不会返回给 Legado，也不会写入日志或规则。
- `.env` 已被 Git 忽略；若 API Key 曾经误提交，请立即在小米平台撤销并重新生成。
- 小说正文会发送给小米 API，这不是离线 TTS；处理敏感内容前请确认隐私政策和服务条款。
- MiMo TTS 官方价格页目前标记为“限时免费”，不代表永久免费，请自行确认地区、额度和计费状态。
- 网络失败后的有限重试可能导致 POST 重复生成，未来也可能产生重复计费；保守使用时可将 `MAX_RETRIES=0`。

## 开发验证

需要 Go 1.26.5：

```bash
gofmt -w ./cmd ./internal
go test ./...
go test -race ./...
go vet ./...
go mod verify
```

正常测试不会访问真实小米 API。
