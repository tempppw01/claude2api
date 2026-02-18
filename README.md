# Claude2Api
将 Claude 网页服务转为 API 服务，支持识图、文件上传、流式传输与思考输出。  
API 兼容 OpenAI 调用格式。

[![Go Report Card](https://goreportcard.com/badge/github.com/yushangxiao/claude2api)](https://goreportcard.com/report/github.com/yushangxiao/claude2api)
[![License](https://img.shields.io/github/license/yushangxiao/claude2api)](LICENSE)

**文档语言：** [中文（优先）](https://github.com/yushangxiao/claude2api/blob/main/docs/chinses.md) | [English](https://github.com/yushangxiao/claude2api/blob/main/README.md)

> 提醒：只有 PRO 用户可以使用所有模型；免费用户仅可使用 `claude-sonnet-4-20250514`。

---

Transform Claude's web service into an API service, supporting image recognition, file upload, streaming transmission, and thinking output.  
The API supports OpenAI-compatible access.

> NOTICE: ONLY PRO USERS CAN USE ALL MODELS. FREE USERS CAN ONLY USE `claude-sonnet-4-20250514`.

## ✨ Features

- 🖼️ **Image Recognition** - Send images to Claude for analysis
- 📝 **Automatic Conversation Management** -  Conversation can be automatically deleted after use
- 🌊 **Streaming Responses** - Get real-time streaming outputs from Claude
- 📁 **File Upload Support** - Upload long context
- 🧠 **Thinking Process** - Access Claude's step-by-step reasoning, support <think>
- 🔄 **Chat History Management** - Control the length of conversation context , exceeding will upload file
- 🌐 **Proxy Support** - Route requests through your preferred proxy
- 🔐 **API Key Authentication** - Secure your API endpoints
- 🔁 **Automatic Retry** - Feature to automatically retry requests when request fail
- 🌐 **Direct Proxy** -let sk-ant-sid01* as key to use

## 📋 Prerequisites

- Go 1.23+ (for building from source)
- Docker (for containerized deployment)

## 🚀 Deployment Options

### Docker

```bash
docker run -d \
  -p 8080:8080 \
  -e SESSIONS=sk-ant-sid01-xxxx,sk-ant-sid01-yyyy \
  -e APIKEY=123 \
  -e CHAT_DELETE=true \
  -e MAX_CHAT_HISTORY_LENGTH=10000 \
  -e NO_ROLE_PREFIX=false \
  -e PROMPT_DISABLE_ARTIFACTS=false \
  -e ENABLE_MIRROR_API=false \
  -e MIRROR_API_PREFIX=/mirror \
  --name claude2api \
  ghcr.io/yushangxiao/claude2api:latest
```

### Docker Compose

Create a `docker-compose.yml` file:

```yaml
version: '3'
services:
  claude2api:
    image: ghcr.io/yushangxiao/claude2api:latest
    container_name: claude2api
    ports:
      - "8080:8080"
    environment:
      - SESSIONS=sk-ant-sid01-xxxx,sk-ant-sid01-yyyy
      - ADDRESS=0.0.0.0:8080
      - APIKEY=123
      - PROXY=http://proxy:2080  # Optional
      - CHAT_DELETE=true
      - MAX_CHAT_HISTORY_LENGTH=10000
      - NO_ROLE_PREFIX=false
      - PROMPT_DISABLE_ARTIFACTS=true
      - ENABLE_MIRROR_API=false
      - MIRROR_API_PREFIX=/mirror
    restart: unless-stopped

```

Then run:

```bash
docker-compose up -d
```

### Hugging Face Spaces

You can deploy this project to Hugging Face Spaces with Docker:

1. Fork the Hugging Face Space at [https://huggingface.co/spaces/rclon/claude2api](https://huggingface.co/spaces/rclon/claude2api)
2. Configure your environment variables in the Settings tab
3. The Space will automatically  deploy the Docker image

notice: In Hugging Face, /v1 might be blocked, you can use /hf/v1 instead.
### Direct Deployment

```bash
# Clone the repository
git clone https://github.com/yushangxiao/claude2api.git
cd claude2api
cp .env.example .env  
vim .env  
# Build the binary
go build -o claude2api .

./claude2api
```

## ⚙️ Configuration

### YAML Configuration

You can configure Claude2API using a `config.yaml` file in the application's root directory. If this file exists, it will be used instead of environment variables.

Example `config.yaml`:

```yaml
# Sessions configuration
sessions:
  - sessionKey: "sk-ant-sid01-xxxx"
    orgID: ""
  - sessionKey: "sk-ant-sid01-yyyy"
    orgID: ""

# Server address
address: "0.0.0.0:8080"

# API authentication key
apiKey: "123"

# Other configuration options...
chatDelete: true
maxChatHistoryLength: 10000
noRolePrefix: false
promptDisableArtifacts: false
enableMirrorApi: false
mirrorApiPrefix: ""
```

A sample configuration file is provided as `config.yaml.example` in the repository.

### Environment Variables

If `config.yaml` doesn't exist, the application will use environment variables for configuration:

| Environment Variable | Description | Default |
|----------------------|-------------|---------|
| `SESSIONS` | Comma-separated list of Claude API session keys | Required |
| `ADDRESS` | Server address and port | `0.0.0.0:8080` |
| `APIKEY` | API key for authentication | Required |
| `PROXY` | HTTP proxy URL | Optional |
| `CHAT_DELETE` | Whether to delete chat sessions after use | `true` |
| `MAX_CHAT_HISTORY_LENGTH` | Exceeding will text to file | `10000` |
| `NO_ROLE_PREFIX` | Do not add role in every message | `false` |
| `PROMPT_DISABLE_ARTIFACTS` | Add Prompt try to disable Artifacts | `false` |
| `ENABLE_MIRROR_API` | Enable direct use sk-ant-* as key | `false` |
| `MIRROR_API_PREFIX` | Add Prefix to protect Mirror，required when ENABLE_MIRROR_API is true | `` |


## 📝 API Usage

### Authentication

Include your API key in the request header:

```
Authorization: Bearer YOUR_API_KEY
```

### Chat Completion

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "model": "claude-3-7-sonnet-20250219",
    "messages": [
      {
        "role": "user",
        "content": "Hello, Claude!"
      }
    ],
    "stream": true
  }'
```

### Image Analysis

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "model": "claude-3-7-sonnet-20250219",
    "messages": [
      {
        "role": "user",
        "content": [
          {
            "type": "text",
            "text": "What\'s in this image?"
          },
          {
            "type": "image_url",
            "image_url": {
              "url": "data:image/jpeg;base64,..."
            }
          }
        ]
      }
    ]
  }'
```

## 🧩 Implementation Principles

This project does not call Anthropic's public API directly. Instead, it simulates Claude Web requests and converts the interaction into an OpenAI-compatible API surface.

### 1) Request pipeline

1. `main.go` starts a Gin server.
2. `router/router.go` mounts OpenAI-compatible routes (`/v1/chat/completions`, `/v1/models`) and HuggingFace-compatible routes (`/hf/v1/...`).
3. `middleware/auth.go` validates the bearer API key (or enables mirror mode when configured).
4. `service/handle.go` parses OpenAI-style requests and transforms `messages` into Claude prompt text + image list.

### 2) Session and account scheduling

- `config/config.go` loads configuration from `config.yaml` (if present) or environment variables.
- `SESSIONS` supports multiple Claude web session keys; the service uses round-robin + retry to improve stability.
- When a request fails, it automatically switches to another configured session (up to `RetryCount`, max 5).

### 3) Claude Web emulation layer

- `core/api.go` builds a browser-like HTTP client (`req/v3` + Chrome impersonation), sets Claude-required headers/cookies, and calls `https://claude.ai/api/...` endpoints.
- If `orgID` is missing, it fetches organizations first and caches the chosen org for later requests.
- It creates a new conversation, sends messages, optionally uploads images/large context files, streams response events, and deletes the conversation asynchronously when enabled.

### 4) OpenAI compatibility strategy

- `model/openai.go` defines OpenAI-style request/response structures.
- The service converts incoming OpenAI `messages` into Claude-compatible prompt content.
- Both streaming (`text/event-stream`) and non-streaming responses are wrapped in OpenAI-compatible JSON shapes so OpenAI SDKs can call this service with minimal changes.

### 5) Context and multimodal handling

- `utils/request.go` handles role prefixing, mixed message blocks (`text`, `image_url`), and optional artifact-suppression prompt injection.
- If prompt context exceeds `MAX_CHAT_HISTORY_LENGTH`, the project switches to file-based context upload mode to avoid oversized direct prompt payloads.


## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- [Anthropic](https://www.anthropic.com/) for creating Claude
- The Go community for the amazing ecosystem

---
 ## 🎁 Support

If you find this project helpful, consider supporting me on [Afdian](https://afdian.com/a/iscoker)  😘

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=yushangxiao/claude2api&type=Date)](https://www.star-history.com/#yushangxiao/claude2api&Date)

Made with ❤️ by [yushangxiao](https://github.com/yushangxiao)
