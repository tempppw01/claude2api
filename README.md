# Claude2Api

将 Claude 网页端能力封装为 OpenAI 兼容 API，支持图像识别、文件上传、流式输出、思考过程透传与管理面板。

适合以下场景：
- 将现有 OpenAI SDK 或应用快速接入 Claude
- 通过容器部署统一管理多个 Claude Session
- 需要可视化查看配置、会话状态与请求日志

[![Go Report Card](https://goreportcard.com/badge/github.com/yushangxiao/claude2api)](https://goreportcard.com/report/github.com/yushangxiao/claude2api)
[![License](https://img.shields.io/github/license/yushangxiao/claude2api)](LICENSE)

> 提示：可用模型范围取决于 Claude 账号权限与当前网页端可用性。

## ✨ 核心特性

- 🖼️ **图像识别**：支持 `image_url` 输入，兼容 OpenAI 风格多模态消息格式
- 📝 **自动会话管理**：可在请求完成后自动清理临时对话
- 🌊 **流式输出**：支持流式响应与增量内容转发
- 📁 **长文本/文件支持**：超长上下文可自动转为文件上传
- 🧠 **思考过程透传**：支持输出思考内容，并自动整理 `<tool_call>` 标签
- 🔄 **多 Session 轮询与重试**：请求失败时自动切换账号重试
- 🌐 **代理支持**：支持自定义 HTTP 代理
- 🔐 **API Key 鉴权**：可保护对外 API
- 🪞 **镜像模式**：可直接使用 `sk-ant-*` 形式访问
- 📊 **管理面板**：支持查看状态、管理 Session、更新配置与浏览日志

## 📋 运行要求

- Go 1.23+（源码运行时）
- Docker（容器部署时）

## 🚀 快速开始

### 方式一：Docker

```bash
docker run -d \
  --name claude2api \
  -p 8080:8080 \
  -e APIKEY=your-api-key \
  -v /path/to/config.yaml:/app/config.yaml \
  34v0wphix/claude2api:latest
```

### 方式二：Docker Compose

创建 `docker-compose.yml`：

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

启动服务：

```bash
docker compose up -d
```

> 注意：首次启动前请先创建空的 `config.yaml` 文件，例如执行 `touch config.yaml`，否则 Docker 可能会将其创建为目录。

### 方式三：源码运行

```bash
git clone https://github.com/yushangxiao/claude2api.git
cd claude2api
cp config.yaml.example config.yaml
vim config.yaml
go build -o claude2api .
./claude2api
```

### 方式四：Hugging Face Spaces

你也可以将本项目部署到 Hugging Face Spaces：

1. Fork Space：https://huggingface.co/spaces/rclon/claude2api
2. 在 Space 设置页配置环境变量
3. 等待 Docker 镜像自动部署完成

> 注意：在 Hugging Face 环境中，`/v1` 可能被平台规则拦截，此时可改用 `/hf/v1`。

## 🐳 Docker Hub 自动发布

仓库已配置 GitHub Actions 自动构建并推送 Docker Hub 多架构镜像，工作流文件为 `.github/workflows/docker-build-push.yml`。

### 发布触发条件

- 推送到 `main` 分支时自动触发
- 推送 Git Tag 时也会触发，例如 `v1.0.0`
- 支持在 GitHub Actions 页面手动触发 `workflow_dispatch`

### 发布内容

- Docker Hub 仓库：`34v0wphix/claude2api`
- 目标架构：`linux/amd64`、`linux/arm64`
- `main` 分支更新时推送：`latest`、`dev`
- Git Tag 发布时推送：`latest`、对应版本标签（如 `v1.0.0`）

### GitHub Secrets 配置

在 GitHub 仓库 `Settings` → `Secrets and variables` → `Actions` 中配置以下密钥：

| Secret 名称 | 用途 |
| --- | --- |
| `DOCKERHUB_USERNAME` | Docker Hub 用户名，例如 `34v0wphix` |
| `DOCKERHUB_TOKEN` | Docker Hub Personal Access Token |

### Docker Hub Token 创建方法

1. 登录 Docker Hub
2. 进入 `Account Settings` → `Personal access tokens`
3. 创建具备写权限的 Token
4. 将 Token 保存到 GitHub Secret `DOCKERHUB_TOKEN`

### 触发示例

推送代码到 `main` 后会自动构建并发布 Docker Hub：

```bash
git add .
git commit -m "chore: update project"
git push origin main
```

此时会自动推送：
- `34v0wphix/claude2api:latest`
- `34v0wphix/claude2api:dev`

如需发布明确版本，也可以继续使用 Tag：

```bash
git tag v1.0.0
git push origin v1.0.0
```

此时会自动推送：
- `34v0wphix/claude2api:latest`
- `34v0wphix/claude2api:v1.0.0`

## ⚙️ 配置说明

### 配置优先级

1. 若存在 `config.yaml`，优先使用 YAML 配置
2. 若不存在 `config.yaml`，回退到环境变量配置
3. 首次启动时会自动生成默认 `config.yaml`
4. 也可通过管理面板更新配置并持久化到 `config.yaml`

### YAML 配置示例

```yaml
# Claude Session 列表
sessions:
  - sessionKey: "sk-ant-sid01-xxxx"
    orgID: ""
  - sessionKey: "sk-ant-sid01-yyyy"
    orgID: ""

# 服务监听地址
address: "0.0.0.0:8080"

# API 访问密钥
apiKey: "your-api-key"

# 其他选项
chatDelete: true
maxChatHistoryLength: 10000
noRolePrefix: false
promptDisableArtifacts: false
enableMirrorApi: false
mirrorApiPrefix: ""
```

仓库已提供示例文件 `config.yaml.example`。

### 环境变量

当 `config.yaml` 不存在时，程序会从环境变量读取配置：

| 环境变量 | 描述 | 默认值 |
| --- | --- | --- |
| `SESSIONS` | 逗号分隔的 Claude Session Key 列表 | 必填 |
| `ADDRESS` | 服务监听地址 | `0.0.0.0:8080` |
| `APIKEY` | API 认证密钥 | 必填 |
| `PROXY` | HTTP 代理地址 | 可选 |
| `CHAT_DELETE` | 请求结束后是否删除聊天 | `true` |
| `MAX_CHAT_HISTORY_LENGTH` | 超出后将上下文改为文件上传 | `10000` |
| `NO_ROLE_PREFIX` | 不在消息前添加角色前缀 | `false` |
| `PROMPT_DISABLE_ARTIFACTS` | 注入提示词尝试禁用 Artifacts | `false` |
| `ENABLE_MIRROR_API` | 允许直接使用 `sk-ant-*` 方式访问 | `false` |
| `MIRROR_API_PREFIX` | 镜像模式接口前缀，开启后建议必填 | 空 |

## 📝 API 使用

### 认证方式

请求头中携带 API Key：

```text
Authorization: Bearer YOUR_API_KEY
```

### 聊天补全

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

访问 `http://localhost:8080/admin` 可进入管理面板，支持：

- 查看服务状态与模型列表
- 管理 Session（新增、删除、测试、导出）
- 修改并保存配置
- 查看请求日志、成功率与统计信息

## 🧩 实现说明

本项目并非直接调用 Anthropic 官方公开 API，而是模拟 Claude 网页端交互流程，并将结果适配为 OpenAI 兼容接口。

### 请求处理链路

1. `main.go` 启动 Gin 服务
2. `router/router.go` 注册 OpenAI 兼容路由与 Hugging Face 兼容路由
3. `middleware/auth.go` 处理 API Key 鉴权与镜像模式放行逻辑
4. `service/handle.go` 解析请求、转换消息并组织 Claude 请求参数

### Session 调度与重试

- `config/config.go` 支持从 YAML 与环境变量加载配置
- 多个 Session 会以轮询方式调度使用
- 请求失败时会自动切换下一个账号重试

### Claude Web 协议适配

- `core/api.go` 使用 `req/v3` 与浏览器指纹模拟网页请求
- 若未提供 `orgID`，会自动获取组织信息并缓存
- 支持创建临时会话、上传文件、接收流式事件及异步删除会话

### OpenAI 兼容层

- `model/openai.go` 定义 OpenAI 风格请求/响应结构
- 服务层会将 Claude 返回结果转换为 OpenAI 兼容格式
- 可直接复用常见 OpenAI SDK 与调用方式

### 长上下文与多模态

- `utils/request.go` 负责 role 前缀处理、多模态消息解析与提示词注入
- 当上下文超过 `MAX_CHAT_HISTORY_LENGTH` 时，会自动切换为文件上传模式

### 日志与监控

- `logger/stats.go` 记录请求日志与基础统计数据
- 管理面板支持查看成功率、平均耗时、错误详情与历史记录

### 限流处理

当 Claude Web 返回 HTTP 429 时，系统会尝试从以下位置解析重置时间：

- `Retry-After` 响应头
- `x-ratelimit-reset` 响应头
- 响应体中的 `reset_at`、`retry_after`、`error.message` 字段

## 🤝 贡献

欢迎通过 Issue 或 Pull Request 参与改进。

1. Fork 仓库
2. 创建分支：`git checkout -b feature/your-feature`
3. 提交修改：`git commit -m "feat: describe your change"`
4. 推送分支：`git push origin feature/your-feature`
5. 发起 Pull Request

## 📄 许可证

本项目基于 MIT License 发布，详见 `LICENSE`。

## 🙏 致谢

- 感谢 Anthropic 提供 Claude
- 感谢 Go 社区与相关开源生态
