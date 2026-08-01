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
EMOTION_ENDPOINT=http://内网模型服务:8000/v1/chat/completions
EMOTION_API_KEY=文字模型API Key
EMOTION_MODEL=模型名称
```

文字模型地址支持绝对的 HTTP 或 HTTPS URL；可信内网服务可直接使用 HTTP。文字模型必须返回结构化 JSON 片段；服务会确认所有片段拼接后与原文逐字一致，再生成标签。例如：

```text
(低声、压抑)夜深了，他一个人站在窗前。
(突然提高音量，愤怒地喊)你为什么要骗我！
```

默认不重试文字模型。文字模型超时、不可用、返回非法 JSON 或改写原文时，服务会立即使用原文继续调用 MiMo，不会中断朗读。可通过 `EMOTION_MAX_RETRIES` 配置 0-5 次重试，但每次重试都会增加故障时的首段等待时间。

开启后，每个段落会增加一次文字模型请求，因此会增加延迟和费用；原始小说段落也会发送给你配置的文字模型服务。容器标准输出只记录请求 ID、阶段状态和耗时等脱敏元数据，不记录正文、标签或 API Key。日志中的 `emotion_completed`、`mimo_completed` 和 `http_request` 可分别用于判断文字模型、MiMo 合成和端到端耗时。

### 将文字模型结果写入独立文件

如需排查模型标注，可显式配置独立 JSONL 日志：

```dotenv
EMOTION_RESPONSE_LOG_FILE=/var/log/mimo-tts/emotion-responses.jsonl
```

并将宿主目录挂载进容器：

```bash
install -d -m 700 ./logs
docker run -d --name mimo-tts-adapter --restart unless-stopped \
  --user "$(id -u):$(id -g)" \
  --env-file .env -p 8080:8080 \
  -v "$(pwd)/logs:/var/log/mimo-tts" \
  mimo-tts-adapter
```

示例让容器以宿主机当前的非 root UID/GID 运行，使它能够写入权限为 `0700` 的挂载目录。若保持镜像默认用户运行，则需要改为向 UID/GID `65532:65532` 授予该目录的写权限。

该文件以追加方式写入，每行包含时间、请求 ID、成功/失败状态、尝试次数、耗时以及文字模型的最终 `content`；校验成功时还包含最终 `annotated_text`。新文件权限设为 `0600`，路径不可写时服务拒绝启动。

这个文件会包含小说正文和情感标签，属于敏感数据，而且服务不会自动轮转或删除它。请只挂载到可信目录，并在宿主机配置日志轮转和保留期限。未配置 `EMOTION_RESPONSE_LOG_FILE` 时不会创建全文日志。

## 延迟与 Legado 预加载

Legado 会缓存和预加载 HTTP TTS 音频，但当前章节仍按段落逐个请求；第一段必须依次等待文字模型和 MiMo 返回完整 MP3 后才能开始播放，预加载无法消除这部分首段延迟。下一章最多预下载若干段，也要在当前队列推进后才能进行。

`/rule` 返回的 `concurrentRate` 为 `"0"`，不再由 Legado 为每个请求额外强制 1 秒间隔。适配器仍通过以下服务端配置限制上游请求，避免移除客户端间隔后失控：

```dotenv
MAX_CONCURRENCY=2
RATE_PER_SECOND=1
RATE_BURST=2
```

如果后续段落仍来不及预加载，可在确认 MiMo 配额和机器资源允许后逐步提高 `RATE_PER_SECOND`、`RATE_BURST` 或 `MAX_CONCURRENCY`。这些调整不会改善文字模型与 MiMo 串行造成的第一段固有等待。
