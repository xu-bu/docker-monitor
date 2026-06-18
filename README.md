# Docker Service Monitor

自动发现同一 Docker 网络内的 HTTP 服务（端口 3000），提供实时健康仪表盘和 OpenAI 兼容的状态查询 API，辅助判断服务是否可以安全下线或重启。

## 架构

```
                    ┌──────────────────────┐
                    │     Docker Host       │
                    │                       │
                    │  ┌─────────────────┐  │
                    │  │   svc-monitor    │  │
                    │  │  ┌───────────┐   │  │
                    │  │  │ Go binary  │   │  │
                    │  │  │ :8080      │   │  │
                    │  │  └─────┬─────┘   │  │
                    │  │  ┌─────┴─────┐   │  │
                    │  │  │ Next.js   │   │  │
                    │  │  │ static    │   │  │
                    │  │  └───────────┘   │  │
                    │  └────────┬────────┘  │
                    │           │           │
                    │  ┌────────┴────────┐  │
                    │  │ Docker Socket   │  │
                    │  │ /var/run/       │  │
                    │  └────────┬────────┘  │
                    │           │           │
              ┌─────┴───────────┴──────────┐│
              │     my-shared-net          ││
              └─────┬───────────┬──────────┘│
                    │           │           │
              ┌─────┴────┐ ┌────┴──────┐   │
              │  svc-a   │ │  svc-b    │   │
              │  :3000   │ │  :3000    │   │
              └──────────┘ └───────────┘   │
                    └──────────────────────┘
```

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go 1.23, `docker/docker` SDK |
| 前端 | Next.js 14 (App Router, Tailwind CSS), static export |
| 运行时 | Alpine Linux 3.20, single binary |
| 通信 | REST API + Server-Sent Events |

## 快速开始

### 前置条件

- Docker Engine ≥ 24.0
- 目标服务容器与 monitor **在同一 Docker user-defined 网络中**
- 目标服务在端口 **3000** 提供 HTTP 接口

### 1. 构建

```bash
git clone <this-repo> docker-monitor
cd docker-monitor
docker compose build
```

### 2. 创建共享网络

```bash
docker network create my-shared-net

# 将目标服务加入该网络
docker network connect my-shared-net my-service-a
docker network connect my-shared-net my-service-b
```

### 3. 启动监控器

```bash
NETWORK_NAME=my-shared-net docker compose up -d
```

打开 [http://localhost:8080](http://localhost:8080) 查看仪表盘。

### 4. 使用测试服务验证

```bash
# 创建共享网络
docker network create test-net

# 启动 5 个模拟 OpenAI 风格的服务
docker compose -f docker-compose.test.yml up -d

# 启动监控器
NETWORK_NAME=test-net docker compose up -d

# 打开仪表盘
open http://localhost:8080
```

测试服务：

| 容器 | 行为 |
|---|---|
| `svc-a` | 稳定在线，报告 `active_connections: 2` |
| `svc-b` | 稳定在线，每次响应延迟 150ms |
| `svc-c` | 支持 SSE 流式响应 |
| `svc-d` | 每 15 秒切换在线/离线 |
| `svc-e` | `GET /v1/models` 返回空数组 |

## 项目结构

```
docker-monitor/
├── Dockerfile                       # 三阶段构建
├── docker-compose.yml               # Compose 编排
├── docker-compose.test.yml          # 测试服务
├── .dockerignore
├── README.md
├── backend/
│   ├── go.mod / go.sum
│   ├── main.go                      # 入口：HTTP 服务器、路由、启动
│   ├── types.go                     # 数据类型：ServiceState、ProbeResult
│   ├── discover.go                  # Docker 容器发现
│   ├── probe.go                     # HTTP 健康探测
│   ├── monitor.go                   # 监控循环 + 指标聚合
│   ├── sse.go                       # SSE 广播器
│   ├── handlers.go                  # REST API 处理器
│   └── openai.go                    # OpenAI 兼容接口
└── frontend/
    ├── package.json
    ├── next.config.mjs               # static export 配置
    ├── tsconfig.json
    ├── tailwind.config.ts
    ├── postcss.config.mjs
    └── src/
        ├── app/
        │   ├── layout.tsx
        │   ├── page.tsx              # 仪表盘主页面
        │   └── globals.css
        ├── components/
        │   ├── Header.tsx            # 顶栏：品牌 + 统计
        │   ├── StatusBar.tsx         # 连接状态指示器
        │   ├── ServiceGrid.tsx       # 响应式卡片网格
        │   └── ServiceCard.tsx       # 单个服务卡片
        └── lib/
            └── api.ts                # TypeScript 类型 + SSE 客户端
```

## 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `MONITOR_PORT` | `8080` | 监控器 Web 服务端口 |
| `MONITOR_TARGET_PORT` | `3000` | 探测目标容器的端口 |
| `MONITOR_INTERVAL` | `5000` | 健康检查间隔 (ms) |
| `MONITOR_TIMEOUT` | `3000` | HTTP 请求超时 (ms) |
| `DISCOVERY_INTERVAL` | `15000` | Docker 容器扫描间隔 (ms) |
| `MONITOR_NETWORK` | `""` | 指定要监控的网络（为空时自动检测） |
| `HOST_PORT` | `8080` | 宿主机映射端口（用于 docker-compose） |

## API 参考

### 仪表盘

```
GET /  → index.html
```

### REST API

```bash
# 全部服务状态
curl http://localhost:8080/api/services | jq .

# 聚合统计
curl http://localhost:8080/api/stats

# 单个服务（替换为实际 container ID）
curl http://localhost:8080/api/services/<container-id>

# SSE 实时流
curl -N http://localhost:8080/api/events
```

### OpenAI 兼容查询

```bash
# JSON 响应
curl http://localhost:8080/v1/chat/completions

# SSE 流式
curl -N 'http://localhost:8080/v1/chat/completions?stream=true'

# POST（OpenAI SDK 兼容）
curl -X POST http://localhost:8080/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"service-monitor-v1","messages":[{"role":"user","content":"Show services"}]}'
```

响应格式遵循 OpenAI Chat Completions 规范。

### 监控器自身健康

```bash
curl http://localhost:8080/health
# → {"status":"healthy","uptime":123.45,"services":5,"online":5}
```

## 工作方式

### 发现

1. 通过挂载的 Docker socket 列出所有容器
2. 根据 hostname 识别自身容器
3. 遍历自身容器所属网络中的所有其他容器
4. 每 15 秒重新扫描，检测上线/下线

### 探测

对每个发现的容器按顺序尝试以下端点：

1. `GET /v1/models` — OpenAI 标准模型列表（优先）
2. `GET /health` — 通用健康检查
3. `GET /` — 兜底

任一端点返回 2xx–4xx 即视为在线。每 30 次探测检查一次 SSE 流式能力。

### 指标

- **在线状态** — 根据 HTTP 响应码判断
- **响应时间** — 请求往返耗时
- **活跃连接数** — 从目标响应中读取 `active_connections` 字段，未上报则为空
- **错误率** — 最近 20 次探测中离线比例
- **SSE 能力** — 是否支持 `text/event-stream` 响应

### 展示

- 前端通过 `/api/events` SSE 端点接收实时推送
- 服务卡片以颜色区分在线/离线状态
- 响应时间、连接数、错误率、运行时长一目了然

## 判断服务能否安全下线

仪表盘上的关键指标：

| 指标 | 含义 |
|---|---|
| **活跃连接数** | 持续为 0 说明无流量 |
| **响应时间** | 异常升高可能表示负载异常 |
| **错误率** | 高错误率说明服务可能已有问题 |
| **持续离线** | 若已离线可忽略 |

建议观察至少 5 分钟，确认连接数持续为 0 或极低，再执行下线操作。

## 本地开发

```bash
# 1. 构建 Go 后端
cd backend
source ~/.gvm/scripts/gvm && gvm use go1.23
GOPROXY=https://proxy.golang.org go build -o monitor .
./monitor

# 2. 构建前端（另一个终端）
cd frontend
source ~/.nvm/nvm.sh && nvm use 22
npm install
npm run build      # 输出到 out/
cp -r out/ ../backend/public/

# 3. 启动监控器
MONITOR_NETWORK=test-net ./monitor
```

## 安全说明

- 监控器需要挂载 Docker socket (`/var/run/docker.sock`) 才能自动发现容器。
  这授予了容器对 Docker 守护进程的访问权限。在敏感环境中，建议使用
  [docker-socket-proxy](https://github.com/Tecnativa/docker-socket-proxy) 限制 API 权限。
- 监控器仅执行 `GET` / `HEAD` 类请求探测目标服务，不修改任何目标状态。
