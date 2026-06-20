# Claude2API

Claude2API 将 Claude 网页端能力封装为 OpenAI 兼容接口，并提供一个可视化管理面板，用来管理多组 Claude Session、模型别名、限流冷却、请求日志和使用统计。

这个仓库已经在原始复刻基础上加入了较多面向实际运维的能力，README 以当前实现为准。

> 注意：本项目不是 Anthropic 官方 API 客户端，而是适配 Claude 网页端交互。请自行确认账号、平台条款和部署环境风险。

## 功能概览

- OpenAI 兼容接口：支持 `/v1/chat/completions`、`/v1/models`，可接入常见 OpenAI SDK 或客户端。
- Hugging Face 兼容路径：提供 `/hf/v1/chat/completions`、`/hf/v1/models`，用于部分平台路径限制场景。
- 多 Session 轮询：多个 Claude Session 自动轮询，请求失败时按配置重试。
- 限流冷却：识别 Claude Web 的 429、`Retry-After`、重置时间字段，并跳过冷却中的 Session。
- 管理面板：支持 Session 新增、删除、测试、导出、配置保存、请求日志、状态概览。
- Key 质量画像：每个 Session 可查看历史成功次数、Token 使用、限流次数、平均几次后限流、解冻时间。
- OpenAI 请求测试：管理页的测试按钮会发起一次真实的 OpenAI 兼容聊天请求，用于判断 Session 是否可用或限流。
- 模型别名：前端模型列表默认展示无日期后缀的模型 ID，同时保留旧 ID 的真实调用兼容。
- 思考参数：支持 `-think`、`-think-low`、`-think-medium`、`-think-high`、`-think-max` 形式的模型变体。
- 来源参考：会收集 Claude 返回事件中的 citation/source 信息，并转换为 OpenAI 风格 annotations，同时可附加来源列表。
- 多模态与长上下文：支持 `image_url` 输入；超长上下文可按配置转为文件上传。
- Cookie 扩展：Session 可附带 `cfClearance` 或完整 Cookie 串，用于特殊网络/验证环境。

## 接口一览

| 路径 | 说明 |
| --- | --- |
| `GET /health` | 健康检查 |
| `GET /v1/models` | OpenAI 兼容模型列表 |
| `POST /v1/chat/completions` | OpenAI 兼容聊天补全 |
| `GET /hf/v1/models` | Hugging Face 兼容模型列表 |
| `POST /hf/v1/chat/completions` | Hugging Face 兼容聊天补全 |
| `GET /admin` | 管理面板 |
| `POST /admin-api/login` | 管理面板登录 |
| `GET /admin-api/status` | 管理面板状态数据 |

## 快速开始

### 方式一：源码运行

```bash
git clone https://github.com/<your-name>/claude2api.git
cd claude2api
cp config.yaml.example config.yaml
vim config.yaml
go run .
```

默认监听地址为 `0.0.0.0:8080`。

### 方式二：Docker 本地构建

```bash
docker build -t claude2api:local .
docker run -d \
  --name claude2api \
  -p 8080:8080 \
  -v "$PWD/config.yaml:/app/config.yaml" \
  claude2api:local
```

### 方式三：Docker Compose

```yaml
services:
  claude2api:
    build: .
    container_name: claude2api
    ports:
      - "8080:8080"
    volumes:
      - ./config.yaml:/app/config.yaml
    restart: unless-stopped
```

启动：

```bash
docker compose up -d
```

> 如果你使用自己的 Docker Hub 或 GHCR 自动发布，请把镜像命名空间、Actions Secret 和 README 中的示例镜像改成自己的公开名称，不要沿用原始复刻仓库的标识。

## 配置

配置优先级：

1. 如果存在 `config.yaml`，优先读取 YAML。
2. 如果不存在 `config.yaml`，读取环境变量。
3. 首次启动时会生成默认 `config.yaml`。
4. 管理面板保存配置时会写回 `config.yaml`。

### YAML 示例

```yaml
sessions:
  - sessionKey: "REPLACE_WITH_CLAUDE_SESSION_KEY"
    orgID: ""
    cfClearance: ""
    cookieString: ""

address: "0.0.0.0:8080"
apiKey: "REPLACE_WITH_YOUR_API_KEY"
proxy: ""
adminPassword: "REPLACE_WITH_A_STRONG_ADMIN_PASSWORD"

chatDelete: true
maxChatHistoryLength: 10000
retryCount: 0
requestLogRetention: 1000

noRolePrefix: false
promptDisableArtifacts: false

enableMirrorApi: false
mirrorApiPrefix: ""

globalSystemPromptOverride: ""
globalPromptOverrideMode: "append"

modelDefinitions: []
```

### 环境变量

当没有 `config.yaml` 时，可用环境变量启动：

| 变量 | 说明 | 默认值 |
| --- | --- | --- |
| `SESSIONS` | Claude Session 列表，多个用逗号分隔；可写 `sessionKey:orgID:cfClearance:cookieString` | 空 |
| `ADDRESS` | 服务监听地址 | `0.0.0.0:8080` |
| `APIKEY` | OpenAI 兼容接口鉴权密钥 | 空 |
| `PROXY` | HTTP 代理地址 | 空 |
| `ADMIN_PASSWORD` | 管理面板密码 | `claude2apidev` |
| `CHAT_DELETE` | 请求完成后删除 Claude 对话 | `true` |
| `MAX_CHAT_HISTORY_LENGTH` | 超过长度后使用文件上下文 | `10000` |
| `NO_ROLE_PREFIX` | 不向提示词追加角色前缀 | `false` |
| `PROMPT_DISABLE_ARTIFACTS` | 注入禁用 Artifacts 的提示 | `false` |
| `ENABLE_MIRROR_API` | 启用镜像模式 | `false` |
| `MIRROR_API_PREFIX` | 镜像模式前缀 | 空 |
| `REQUEST_LOG_RETENTION` | 管理面板保留的请求日志条数，可选 `100`、`500`、`1000`、`3000` | `1000` |

生产环境请务必修改 `adminPassword` 和 `apiKey`。

`retryCount` 为 `0` 或未配置时，会按当前 Session 数量自动尝试，最多不超过可用 Session 总数。即使你把 `retryCount` 设置得较低，命中限流时也会继续扫描其它未冷却的 Session，避免单个 key 限流直接中断连续请求。

## 模型说明

`/v1/models` 默认只暴露面向用户的模型 ID，不展示带日期后缀的旧 ID。旧 ID 仍可作为兼容调用入口，避免影响已有客户端。

常用公开 ID 示例：

| 模型 ID | 说明 |
| --- | --- |
| `claude-sonnet-4-6` | 默认 Sonnet 4.6 |
| `claude-sonnet-4-6-think` | Sonnet 4.6 思考模式 |
| `claude-sonnet-4-6-think-low` | 低思考强度 |
| `claude-sonnet-4-6-think-medium` | 中思考强度 |
| `claude-sonnet-4-6-think-high` | 高思考强度 |
| `claude-sonnet-4-6-think-max` | 最大思考强度 |
| `claude-haiku-4-5` | Haiku 4.5 |
| `claude-opus-4-6`、`claude-opus-4-7`、`claude-opus-4-8` | Opus 系列，是否可用取决于账号权限 |

可通过 `modelDefinitions` 增加、隐藏或覆盖模型配置。

## OpenAI 兼容调用

### Chat Completions

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer REPLACE_WITH_YOUR_API_KEY" \
  -d '{
    "model": "claude-sonnet-4-6",
    "messages": [
      {
        "role": "user",
        "content": "你好"
      }
    ],
    "stream": false
  }'
```

### 思考强度

可以直接使用模型后缀：

```json
{
  "model": "claude-sonnet-4-6-think-high",
  "messages": [
    {
      "role": "user",
      "content": "请一步步分析这个问题。"
    }
  ]
}
```

也可以使用兼容字段 `reasoning_effort`：

```json
{
  "model": "claude-sonnet-4-6",
  "reasoning_effort": "high",
  "messages": [
    {
      "role": "user",
      "content": "请一步步分析这个问题。"
    }
  ]
}
```

### 图像输入

```json
{
  "model": "claude-sonnet-4-6",
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
            "url": "data:image/jpeg;base64,REPLACE_WITH_BASE64_IMAGE"
          }
        }
      ]
    }
  ]
}
```

### 来源参考

当 Claude 返回来源信息时，本项目会尝试提取 URL 和标题，并写入 OpenAI 兼容响应的 `annotations` 字段。非流式响应也可能在正文末尾附加来源列表，具体取决于 Claude 网页端返回事件。

## 管理面板

访问：

```text
http://localhost:8080/admin
```

管理面板能力：

- 查看服务状态、模型列表和请求统计。
- 新增、删除、导出 Claude Session。
- 通过真实 OpenAI 兼容请求测试 Session 是否可用。
- 查看每个 Session 的成功次数、Token 总量、平均 Token、限流次数、平均几次后限流。
- 查看冻结状态、解冻时间、冷却来源和最近错误。
- 手动解除运行时冻结，适合处理估算冷却误判或新账号实际可用的场景。
- 调整配置、模型定义、日志保留数量。

## 限流与解冻

当 Claude Web 返回限流相关错误时，系统会尝试解析：

- `Retry-After`
- `x-ratelimit-reset`
- `X-RateLimit-Reset`
- 响应体中的 `reset_at`、`retry_after`、`resetsAt`、`error.message`
- 文本中的自然语言时间提示

解析成功且时间明显在未来时，冷却来源会标记为 `official`，表示解冻时间来自 Claude 返回；解析失败、没有返回可用时间、或返回时间等于当前请求时间时，冷却来源会标记为 `fallback`。`fallback` 是短探测冷却，默认 6 分钟，用来避开 Claude Web 的短频率窗口，不能当作官网解冻时间。管理面板显示的时间按中国时间格式展示。冷却信息保存在运行内存中，服务重启后会清空，也可以在管理面板手动解除。

如果 Claude 返回的 reset 时间等于当前请求时间，或距离当前时间太近，项目会认为这个时间不可用于确认真实解冻窗口，并使用短探测冷却兜底。此时页面会显示“项目估算冷却”，不应把它当作 claude.ai 官方解冻时间。

## 安全与隐私

请不要把以下内容提交到公开仓库、Issue、截图或 README：

- 真实 Claude Session Key。
- 真实 `orgID`。
- `cf_clearance`、完整 Cookie、浏览器导出的 Cookie 串。
- 管理员密码、API Key、Docker Hub Token、GitHub Token。
- 包含真实账号、邮箱、请求日志、错误栈、请求内容的截图。

推荐做法：

- `config.yaml`、`.env` 保持在 `.gitignore` 中。
- 公开文档只使用 `REPLACE_WITH_...` 占位符。
- 提交前运行 `git diff`，确认没有把真实密钥写入文档或配置。
- 如果密钥曾经被提交，立即在 Claude/GitHub/Docker Hub 等平台重新生成或撤销。

## 开发与测试

```bash
go test ./...
```

常见文件：

| 文件 | 说明 |
| --- | --- |
| `main.go` | 服务入口 |
| `router/router.go` | 路由注册 |
| `service/handle.go` | OpenAI 兼容请求处理 |
| `service/models.go` | 模型别名、可见性和思考变体 |
| `service/admin.go` | 管理接口 |
| `core/api.go` | Claude Web 请求适配 |
| `logger/stats.go` | 请求日志、Session 统计 |
| `web/static/index.html` | 管理面板前端 |
| `config/config.go` | 配置加载与 Session 冷却 |

## 发布

仓库包含 Docker 多架构构建 workflow。使用前请检查：

- `.github/workflows/docker-build-push.yml` 默认使用 `DOCKERHUB_USERNAME` 作为镜像命名空间。
- GitHub Actions Secrets 是否配置了 `DOCKERHUB_USERNAME` 和 `DOCKERHUB_TOKEN`。
- README 中的部署示例是否已经替换成你的镜像地址。

如果不需要公开镜像发布，可以删除或禁用相关 workflow。

## 许可证

本项目基于 MIT License 发布，详见 `LICENSE`。

## 致谢

感谢原始开源项目、Go 生态以及相关依赖库。本仓库的说明文档聚焦当前分支实际功能与部署方式。
