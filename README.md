# Claude2Api

将 Claude 网页服务转为 API 服务，支持识图、文件上传、流式传输与思考输出。  
API 兼容 OpenAI 调用格式。

[![Go Report Card](https://goreportcard.com/badge/github.com/yushangxiao/claude2api)](https://goreportcard.com/report/github.com/yushangxiao/claude2api)
[![License](https://img.shields.io/github/license/yushangxiao/claude2api)](LICENSE)

> 提醒：只有 PRO 用户可以使用所有模型；免费用户可使用 `claude-sonnet-4-20250514` 和 `claude-sonnet-4-6-20260217`。

## ✨ 特性

- 🖼️ **图像识别** - 发送图像给 Claude 进行分析
- 📝 **自动对话管理** - 对话可在使用后自动删除
- 🌊 **流式响应** - 获取 Claude 实时流式输出
- 📁 **文件上传支持** - 上传长文本内容
- 🧠 **思考过程** - 访问 Claude 的逐步推理，自动输出 `<tool_call>` 标签
- 🔄 **聊天历史管理** - 控制对话上下文长度，超出将上传为文件
- 🌐 **代理支持** - 通过您首选的代理请求
- 🔐 **API 密钥认证** - 保护您的 API 端点
- 🔁 **自动重试** - 请求失败时，自动切换下一个账号
- 🌐 **直接代理** - 使用 `sk-ant-*` 直接作为 key 使用
- 📊 **管理面板** - 前端可视化管理 Session、配置、查看日志

## 📋 前提条��

- Go 1.23+（从源代码构建）
- Docker（用于容器化部署）

## 🚀 部署选项

### Docker

```bash
docker run -d \
  -p 8080:8080 \
  -e APIKEY=your-api-key \
  -v /path/to/config.yaml:/app/config.yaml \
  --name claude2api \
  34v0wphix/claude2api:latest
```

### Docker Compose

创建一个 `docker-compose.yml` 文件：

```yaml
services:
  claude2api:
    image: 34v0wphix/claude2api:latest
    container_name: claude2api
    ports:
      - "8080:8080"
    environment:
      - APIKEY=your-api-key
    volumes:
      - ./config.yaml:/app/config.yaml
    restart: unless-stopped
```

然后运行：

```bash
docker-compose up -d
```

> **注意**：首次启动前请先创建空的 `config.yaml` 文件：`touch config.yaml`，否则 Docker 会将其创建为目录。

### Hugging Face Spaces

您可以使用 Docker 将此项目部署到 Hugging Face Spaces：

1. Fork Hugging Face Space：[https://huggingface.co/spaces/rclon/claude2api](https://huggingface.co/spaces/rclon/claude2api)
2. 在设置选项卡中配置您的环境变量
3. Space 将自动部署 Docker 镜像

> 注意：在 Hugging Face 中，`/v1` 可能被屏蔽，您可以使用 `/hf/v1` 代替。

### 直接部署

```bash
# 克隆仓库
git clone https://github.com/yushangxiao/claude2api.git
cd claude2api

# 复制配置文件
cp config.yaml.example config.yaml
vim config.yaml

# 构建并运行
go build -o claude2api .
./claude2api
```

## ⚙️ 配置

### YAML 配置

你可以在应用程序的根目录下使用 `config.yaml` 文件来配置 Claude2API。如果此文件存在，将会优先使用它而不是环境变量。

`config.yaml` 示例：

```yaml
# Sessions 配置
sessions:
  - sessionKey: "sk-ant-sid01-xxxx"
    orgID: ""
  - sessionKey: "sk-ant-sid01-yyyy"
    orgID: ""

# 服务地址
address: "0.0.0.0:8080"

# API 认证密钥
apiKey: "your-api-key"

# 其他配置选项
chatDelete: true
maxChatHistoryLength: 10000
noRolePrefix: false
promptDisableArtifacts: false
enableMirrorApi: false
mirrorApiPrefix: ""
```

仓库中提供了一个名为 `config.yaml.example` 的示例配置文件。

### 环境变量

如果 `config.yaml` 不存在，应用程序将使用环境变量进行配置：

| 环境变量 | 描述 | 默认值 |
|---------|------|--------|
| `SESSIONS` | 逗号分隔的 Claude API 会话密钥列表 | 必填 |
| `ADDRESS` | 服务器地址和端口 | `0.0.0.0:8080` |
| `APIKEY` | 用于认证的 API 密钥 | 必填 |
| `PROXY` | HTTP 代理 URL | 可选 |
| `CHAT_DELETE` | 是否在使用后删除聊天会话 | `true` |
| `MAX_CHAT_HISTORY_LENGTH` | 超出此长度将文本转为文件 | `10000` |
| `NO_ROLE_PREFIX` | 不在每条消息前添加角色 | `false` |
| `PROMPT_DISABLE_ARTIFACTS` | 添加提示词尝试禁用 Artifacts | `false` |
| `ENABLE_MIRROR_API` | 允许直接使用 `sk-ant-*` 作为 key 使用 | `false` |
| `MIRROR_API_PREFIX` | 对直接使用增加接口前缀，开启 `ENABLE_MIRROR_API` 时必填 | `` |

### 配置优先级

1. 如果存在 `config.yaml`，优先使用 YAML 配置
2. 如果不存在 `config.yaml`，使用环境变量配置
3. 首次启动时会自动创建默认的 `config.yaml` 文件
4. 可以通过管理面板修改配置并持久化到 `config.yaml`

## 📝 API 使用

### 认证

在请求头中包含您的 API 密钥：

```
Authorization: Bearer YOUR_API_KEY
```

### 聊天完成

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "model": "claude-3-7-sonnet-20250219",
    "messages": [
      {
        "role": "user",
        "content": "你好，Claude！"
      }
    ],
    "stream": true
  }'
```

### 图像分析

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
            "text": "这张图片里有什么？"
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

### 管理面板

访问 `http://localhost:8080/admin` 进入管理面板，可以：

- 查看服务状态和模型列表
- 管理 Session（添加、删除、测试、导出）
- 修改配置并持久化
- 查看请求日志和统计信息

## 🧩 实现原理

本项目并不是直接调用 Anthropic 对外公开 API，而是模拟 Claude 网页端请求流程，并将结果适配为 OpenAI 兼容接口。

### 1）请求处理链路

1. `main.go` 启动 Gin 服务。
2. `router/router.go` 注册 OpenAI 兼容路由（`/v1/chat/completions`、`/v1/models`）以及 Hugging Face 兼容路由（`/hf/v1/...`）。
3. `middleware/auth.go` 执行 API Key 鉴权；如果开启镜像模式则按前缀放行。
4. `service/handle.go` 解析 OpenAI 风格请求，将 `messages` 转换为 Claude 可用 prompt，并提取图片数据。

### 2）会话与账号调度

- `config/config.go` 支持从 `config.yaml` 或环境变量加载配置。
- `SESSIONS` 可配置多个 `sessionKey`，服务会按轮询方式选取账号。
- 请求失败时自动重试并切换到下一个账号（最多 `RetryCount` 次，内部限制上限 5）。

### 3）Claude Web 协议模拟

- `core/api.go` 使用 `req/v3` + Chrome 指纹模拟网页请求，设置 Claude 所需 headers/cookies，并调用 `https://claude.ai/api/...`。
- 若未提供 `orgID`，会先请求组织列表并缓存可用组织。
- 发送请求时会创建临时会话、上传图片/长上下文文件、接收流式事件；按配置异步删除会话。

### 4）OpenAI 兼容策略

- `model/openai.go` 定义 OpenAI 请求/响应结构体。
- 服务层将 Claude 返回内容封装为 OpenAI 兼容格式，支持流式和非流式两种模式。
- 这样可以直接复用常见 OpenAI SDK 与调用方式，减少接入改造成本。

### 5）长上下文与多模态

- `utils/request.go` 负责 role 前缀处理、`text/image_url` 混合消息解析，以及可选的 artifacts 禁用提示词注入。
- 当 prompt 超过 `MAX_CHAT_HISTORY_LENGTH` 时，会切换为上传文件承载上下文，避免超长请求直接失败。

### 6）请求日志与监控

- `logger/stats.go` 提供内存级请求日志记录，支持统计成功率、RPM、平均耗时等指标。
- 管理面板展示请求日志，支持分页浏览（每页 10 条）、错误详情展示和历史成功率统计。
- 日志包含 Session 索引、输入/输出 Token 数、耗时和错误信息。

### 7）限流处理

- 当 Claude Web 返回 HTTP 429（限流）时，系统会尝试从以下来源解析限流重置时间：
  - `Retry-After` 响应头（秒数或日期）
  - `x-ratelimit-reset` 响应头
  - 响应体字段（`reset_at`、`retry_after`、`error.message`）
- 重置时间会包含在错误信息中，便于在日志中查看。

## 🤝 贡献

欢迎贡献！请随时提交 Pull Request。

1. Fork 仓库
2. 创建特性分支（`git checkout -b feature/amazing-feature`）
3. 提交您的更改（`git commit -m '添加一些惊人的特性'`）
4. 推送到分支（`git push origin feature/amazing-feature`）
5. 打开 Pull Request

## 📄 许可证

本项目采用 MIT 许可证 - 详见 [LICENSE](LICENSE) 文件。

## 🙏 致谢

- 感谢 [Anthropic](https://www.anthropic.com/) 创建 Claude
- 感谢 Go 社区提供的优秀生态系统

---

由 [yushangxiao](https://github.com/yushangxiao) 用 ❤️ 制作
