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

## 可选：自动情感标注

MiMo V2.5 TTS 支持 `(自然语言表现描述)正文` 形式的行内音频标签。服务可以先调用 OpenAI 兼容的文字模型分析每个段落，再把经过校验的标签文本发送给 MiMo：

```dotenv
EMOTION_ENABLED=true
EMOTION_ENDPOINT=https://你的模型服务/v1/chat/completions
EMOTION_API_KEY=文字模型API Key
EMOTION_MODEL=模型名称
```

文字模型必须返回结构化 JSON 片段；服务会确认所有片段拼接后与原文逐字一致，再生成标签。例如：

```text
(低声、压抑)夜深了，他一个人站在窗前。
(突然提高音量，愤怒地喊)你为什么要骗我！
```

默认最多重试 3 次。文字模型超时、不可用、返回非法 JSON 或改写原文时，服务会使用原文继续调用 MiMo，不会中断朗读。开启后，每个段落会增加一次文字模型请求，因此会增加延迟和费用；原始小说段落也会发送给你配置的文字模型服务。正文、标签和 API Key 均不会写入日志。
