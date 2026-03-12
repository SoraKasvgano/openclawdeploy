[简体中文](README.md)
# OpenClaw Deploy

OpenClaw Deploy 是一个使用 Go 编写的双端项目，用于管理和部署 OpenClaw 节点。

- `server`：提供用户管理、设备绑定、设备筛选与删除、远程下发 `openclaw.json`、AI Token API 调用、Swagger UI、SMTP 和注册开关等能力
- `client`：提供本地 Web 控制台、本地登录验证、机器识别码生成、本地 `openclaw.json` 编辑、服务端通信状态展示、心跳同步和自动生成部署脚本

客户端和服务端前端资源都通过 `embed` 打包进二进制，运行时不依赖 CDN。

## 当前实现状态

目前已实现：

- 单文件二进制客户端与服务端
- 前后端分离，静态资源内嵌
- 基于 Go 标准库 `net/http` 的 REST 风格 API
- 服务端注册、登录、忘记密码、重置密码
- 管理员后台、SMTP 配置、注册开关、用户管理
- 普通用户自助修改邮箱和密码
- 设备绑定、备注编辑、删除、远程下发 `openclaw.json`
- 管理员按用户名筛选设备
- 外部程序通过 AI Token 直接调用服务端 API
- Swagger UI 与 OpenAPI JSON
- 客户端本地 Web 界面和本地 API 的独立登录验证
- 客户端默认本地账号 `admin / admin`，可自行修改
- 客户端定时心跳同步并自动应用远程配置
- 客户端页面展示服务端配置状态和最近通信结果
- 服务端 `serverconfig.json` 热加载，端口和监听地址变更时自动重绑
- 机器识别码压缩为纯字母数字，并兼容旧格式识别
- 服务端状态持久化已切换为无 CGO 的 SQLite
- 旧版 `data/server-state.json` 启动时自动迁移到 SQLite
- 设备心跳热数据只保存在内存，不写入持久化数据库

与最初设计相比，当前仍保留的差距：

- 客户端显示的是身份矩阵图，不是标准二维码
- `clientdeploy.sh` 仍偏 Linux 场景，依赖 `bash` / `systemd`
- 服务端状态存储仍是单机 SQLite，不是多节点分布式方案

## 目录结构

```text
.
|-- client/
|   |-- main.go
|   |-- backend/
|   |-- frontend/
|   `-- build.bat
|-- server/
|   |-- main.go
|   |-- backend/
|   |-- frontend/
|   |-- swaggerui/
|   `-- build.bat
|-- internal/
|   `-- shared/
|-- design.md
|-- install.sh
`-- openclaw.json
```

其中 `internal/shared` 不是独立程序，而是客户端和服务端共用的内部工具包，主要放原子写文件、JSON 处理、哈希、运行目录判断、设备码规范化等通用逻辑。

## 运行要求

- Go `1.25.0`
- 主要运行目标是 Linux / macOS
- Windows 可用于编译和界面联调
- 服务端 SQLite 使用 `modernc.org/sqlite`，无 CGO 依赖

## 快速启动

在仓库根目录执行：

```bash
go run ./server
```

启动后访问：

- 服务端界面：`http://127.0.0.1:18080/`
- Swagger UI：`http://127.0.0.1:18080/swagger/`
- OpenAPI JSON：`http://127.0.0.1:18080/openapi.json`

再启动客户端：

```bash
go run ./client
```

启动后访问：

- 客户端界面：`http://127.0.0.1:17896/`

如果使用 `go run`，运行期生成的配置文件会写到当前工作目录，因为项目已经显式规避了 Go 临时构建目录。

## 服务端说明

### 默认监听

- 默认监听地址：`0.0.0.0:18080`
- 日志中展示的本地访问地址：`http://127.0.0.1:18080/`

### 首次启动

服务端首次启动会自动创建：

- `serverconfig.json`
- `data/server-state.sqlite`

同时自动保证：

- 默认管理员账号：`admin / admin`
- 自动生成 `ai_token` 并明文写入 `serverconfig.json`
- 如果发现旧版 `data/server-state.json`，会在启动时自动迁移到 SQLite

### 热加载

运行中的服务端会轮询 `serverconfig.json`，检测到文件变化后自动热加载。

当前支持热加载的配置项：

- `web_port`
- `listen_addr`
- `public_base_url`
- `session_ttl_hours`
- `ai_token`
- `smtp`

如果修改了 `web_port` 或 `listen_addr`，服务端会自动关闭旧监听并绑定到新地址。

### 状态存储

服务端状态持久化现在使用 SQLite，默认文件为：

- `data/server-state.sqlite`

当前持久化内容主要包括：

- 注册开关
- 用户
- 会话
- 密码重置令牌
- 设备归属、备注、待下发配置

以下热数据只保存在内存，不会持久化到 SQLite：

- `last_seen_at`
- `sync_interval_seconds`
- 运行状态 `status`
- 客户端当前上报的 `openclaw_json/openclaw_hash`

这样可以避免大量心跳把硬盘写放大到不合理的程度。

### 外部程序 API 调用

所有受保护接口都支持以下认证方式：

- `X-API-Token: <ai_token>`
- `Authorization: Bearer <ai_token>`
- 普通登录后的会话 cookie / token

示例：

```bash
curl -H "X-API-Token: <ai_token>" http://127.0.0.1:18080/api/v1/admin/summary
```

### 设备管理接口补充

当前设备相关能力包括：

- `POST /api/v1/devices/bind`
- `PUT /api/v1/devices/{deviceID}/remark`
- `PUT /api/v1/devices/{deviceID}/config`
- `DELETE /api/v1/devices/{deviceID}`

管理员还可以通过下面的查询参数筛选设备：

```text
GET /api/v1/devices?owner_username=<用户名关键字>
```

## 客户端说明

### 默认监听

- 默认监听地址：`0.0.0.0:17896`
- 日志中展示的本地访问地址：`http://127.0.0.1:17896/`

### 首次启动

客户端首次启动会自动创建：

- `config.json`
- `clientdeploy.sh`
- 目标 `openclaw.json` 文件（若原本不存在）

同时会生成机器识别码，并在命令行输出识别码和对应的 ASCII 身份矩阵图。

### 本地登录验证

客户端本地网页和本地 API 现在有一套独立验证，不影响客户端与服务端通信。

默认本地账号：

- 用户名：`admin`
- 密码：`admin`

可在客户端页面中自行修改。客户端心跳同步不依赖这套本地登录，因此不会因为本地 Web 未登录而中断和服务端的通信。

### 机器识别码

机器识别码现在只保留字母和数字，便于存储、搜索和展示。

示例：

```text
旧格式：233|00:22:5d:a3:46:da|2026-03-11 17:07:15|100.64.0.3
新格式：23300225da346da202603111707151006403
```

兼容策略：

- 新客户端生成的新设备码使用紧凑格式
- 旧 `config.json` 中的设备码会在启动时自动规范化
- 服务端会把旧格式和新格式识别为同一台设备，不会因为升级多出重复节点

### OpenClaw 配置路径

客户端默认目标路径为：

- macOS：`~/.openclaw/openclaw.json`
- Linux：`~/.openclaw/openclaw.json`

实际代码中会通过用户家目录拼接成：

```text
filepath.Join(home, ".openclaw", "openclaw.json")
```

### 同步机制

客户端会向下面的地址发送心跳：

```text
<server_url>/api/v1/client/heartbeat
```

默认同步间隔：

- `30` 秒

补充行为：

- 如果 `server_url` 为空，客户端不会进行服务端轮询，但本地 Web 界面和本地配置编辑功能仍然可用
- 如果 `server_url` 写成 `127.0.0.1:18080` 这种裸地址，客户端会自动规范化为 `http://127.0.0.1:18080`
- 客户端页面会显示服务端是否已配置、最近一次同步时间和通信是否成功

## 配置文件

### 服务端 `serverconfig.json`

典型示例：

```json
{
  "web_port": 18080,
  "listen_addr": "0.0.0.0",
  "public_base_url": "",
  "session_ttl_hours": 72,
  "ai_token": "generated-on-first-start",
  "smtp": {
    "host": "",
    "port": 25,
    "username": "",
    "password": "",
    "from": ""
  }
}
```

### 客户端 `config.json`

典型示例：

```json
{
  "device_id": "generated-on-first-start",
  "device_created_at": "2026-03-12 10:00:00",
  "web_username": "admin",
  "web_password": "admin",
  "web_port": 17896,
  "listen_addr": "0.0.0.0",
  "server_url": "http://127.0.0.1:18080",
  "sync_interval_seconds": 30,
  "openclaw_config_path": "/home/user/.openclaw/openclaw.json",
  "allow_remote_reboot": false
}
```

## 构建

### 本机构建

```bash
go build ./server
go build ./client
```

### 测试

```bash
go test ./...
```

### 批处理脚本

当前已提供：

- `client/build.bat`
- `server/build.bat`

它们会把 Linux / macOS 产物输出到 `client/dist` 和 `server/dist`。

### 手动编译 Windows 示例

客户端：

```powershell
$env:CGO_ENABLED='0'
$env:GOOS='windows'
$env:GOARCH='amd64'
go build -trimpath -o client/dist/openclaw-client-windows-amd64.exe ./client
```

服务端：

```powershell
$env:CGO_ENABLED='0'
$env:GOOS='windows'
$env:GOARCH='amd64'
go build -trimpath -o server/dist/openclaw-server-windows-amd64.exe ./server
```

## 路由与前端资源说明

- 前端资源全部嵌入二进制
- Swagger 资源全部本地嵌入
- 运行时不依赖外部 CDN
- 服务端当前使用 `net/http.ServeMux`，不是 Gin
- `/Swagger` 和 `/swagger` 会统一跳转到内置 Swagger UI

## 权限模型

- 普通用户只能查看和管理自己可见的设备
- 普通用户只能删除自己名下的设备记录
- 普通用户只能修改自己的邮箱和密码，不能改用户名
- SMTP、注册开关、用户管理、管理员概览等功能仅管理员可见且后端受权限保护
- 管理员可以查看全部设备，并按用户名筛选设备
- 外部程序使用 `ai_token` 时可直接访问受保护接口
- 客户端本地登录和服务端登录是两套独立体系

## 其他说明

- 仓库根目录保留了 `install.sh` 作为参考安装脚本
- 自动生成的 `clientdeploy.sh` 会尝试写入 `systemd` 服务，并执行上游安装命令
- 当前服务端配置解析兼容带 UTF-8 BOM 的 `serverconfig.json`
- SQLite 状态库用于单进程本地服务，当前不面向多写入节点共享同一个状态文件
