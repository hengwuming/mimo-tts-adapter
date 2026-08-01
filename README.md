# MiMo TTS Adapter for Legado

将小米 MiMo TTS 接入 Legado，返回可直接播放的 MP3 音频。

## 快速开始

1. 创建配置文件：

```bash
cp .env.example .env
```

编辑 `.env`，至少填写：

```dotenv
ADAPTER_AUTH_TOKEN=自定义访问令牌
MIMO_API_KEY=小米MiMo API Key
PUBLIC_BASE_URL=https://你的服务地址
```

2. 构建并启动：

```bash
docker build -t mimo-tts-adapter .
docker run -d --name mimo-tts-adapter --restart unless-stopped \
  --env-file .env -p 8080:8080 mimo-tts-adapter
```

3. 在浏览器中访问 `https://你的服务地址/rule`，将规则里的 `REPLACE_WITH_ADAPTER_TOKEN` 替换为 `.env` 中的 `ADAPTER_AUTH_TOKEN`，然后导入 Legado。

健康检查：

```bash
curl http://localhost:8080/healthz
```
