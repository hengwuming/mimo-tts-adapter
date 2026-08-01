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

服务可以先调用 OpenAI 兼容的文字模型分析每个段落，再把经过校验的情绪与语气指导发送给 MiMo：

```dotenv
EMOTION_ENABLED=true
EMOTION_ENDPOINT=http://内网模型服务:8000/v1/chat/completions
EMOTION_API_KEY=文字模型API Key
EMOTION_MODEL=模型名称
```

文字模型地址支持绝对的 HTTP 或 HTTPS URL。文字模型只需返回需要控制的原文 Unicode rune 索引区间，不再复制小说正文，例如：

```json
{"styles":[{"start":2,"end":16,"style":"语气惊讶且兴奋"}]}
```

`start`/`end` 是从 0 开始的 Unicode rune 半开区间 `[start,end)`，汉字、中文标点和全角空格通常各计一个 rune。服务会严格校验区间非空、升序、互不重叠且不越界，并直接从原文生成朗读锚点。无需额外控制时返回 `{"styles":[]}`。这种协议不会让模型复写含引号的正文，因此响应更短，也避免正文引号破坏 JSON。

默认 `EMOTION_RESPONSE_FORMAT=true`，服务会向支持该能力的 OpenAI 兼容接口发送严格 `json_schema`。如果你的兼容服务不支持 `json_schema`，可设为 `false`；提示词和本地严格校验仍要求上述索引格式。

按照 MiMo V2.5 TTS 的官方消息协议，语速和情绪指导只放在不会被合成为语音的 `user` 消息，`assistant` 消息始终只包含原始小说正文。服务不会再将 `(情绪标签)` 插入正文，从而避免模型偶尔把标签内容直接读出来。文字模型返回单层 ` ```json ... ``` ` 代码围栏时可以兼容，围栏外存在其他文字时仍会回退原文。

推荐配置如下：

```dotenv
EMOTION_TIMEOUT=7s
EMOTION_MAX_RETRIES=0
EMOTION_RESPONSE_FORMAT=true
```

情绪模型调用固定为单路执行，避免 Legado 预加载同时压入多个请求、让单 worker 模型相互争抢。等待槽位也计入 7 秒总超时，并会响应客户端取消；它与 MiMo 的 `MAX_CONCURRENCY` 独立，不会降低 MiMo 合成并发。文字模型超时、不可用或响应校验失败时，服务会使用原文继续调用 MiMo，不会中断朗读。虽然可将 `EMOTION_MAX_RETRIES` 配为 0-5，但重试会成倍增加故障时的首段等待，通常应保持为 0。

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

该文件以追加方式写入，每行包含时间、请求 ID、成功/失败状态、稳定的 `error_category`、尝试次数、耗时以及文字模型的最终 `content`；校验成功时还包含生成的 `style_instruction`。新文件权限设为 `0600`，路径不可写时服务拒绝启动。

常见失败分类包括 `timeout`、`cancelled`、`provider_status`、`response_too_large`、`provider_json`、`content_json`、`invalid_range` 和 `invalid_style`。如果主要是 `timeout`，依据实测 P95 小幅提高 `EMOTION_TIMEOUT`；如果主要是 `content_json`，检查兼容服务是否支持 `json_schema`，不支持时设 `EMOTION_RESPONSE_FORMAT=false`；`invalid_range` 或 `invalid_style` 表示模型输出已收到但没有通过本地安全校验。

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
