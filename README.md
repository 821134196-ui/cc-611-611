# 博物馆藏品图片授权与引用撤回系统

馆藏图片用于刊物、网页等场景时，馆方需要掌握每次授权的用途与渠道，在授权撤回或到期后
仍能找到已经发表的引用位置。本系统覆盖 **申请 → 审批 → 分级下载 → 发表登记 → 撤回/过期 →
位置导出** 的完整流程。

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go 1.23 + Gin |
| 前端 | Vue 3 + Vue Router + Vite（构建产物由 `go:embed` 打进单一二进制） |
| 数据 | GORM + SQLite（纯 Go 驱动 `glebarez/sqlite`，无需 CGO） |
| 图片 | 本地文件存储；预览图由系统从原图生成（纯 Go，最长边 ≤1200px） |
| 邮件 | 模拟接口：不发真实 SMTP，消息落库，页面「邮件提醒」可查 |

## 一条命令启动

```bash
./run.sh
# 自定义端口： PORT=9000 ./run.sh
# 强制重建前端： FORCE_WEB=1 ./run.sh
```

脚本会自动安装前端依赖、构建前端、编译 Go 并启动，访问 <http://localhost:8080>。

演示账号（首次启动自动创建）：

- 馆方审批人：`curator / 123456`
- 申请人：`applicant / 123456`（另有 `editor / 123456`）

数据默认写入 `data/`（SQLite 与本地图片），可用环境变量 `DB_PATH`、`STORAGE_DIR`、
`JWT_SECRET` 覆盖。删除 `data/` 即可重置全部演示数据。

## 端到端验证

```bash
./scripts/e2e-check.sh      # 启动临时实例，跑完 22 项 HTTP 级检查后自动清理
go test -race ./...         # Go 集成测试（含 25+15 轮并发审批竞态）
```

## 业务规则与设计要点

### 授权状态机

```
pending 待审批 ──批准──▶ approved 可下载 ──馆方撤回──▶ revoked 已撤回
       └──────驳回────▶ rejected 已驳回          └──到期(实时/巡检)──▶ expired 已过期
```

「我的授权」页面用三个分区清楚呈现 **待审批 / 可下载 / 已失效**；已失效区保留全部历史，
并对仍有已发表引用的授权给出醒目标注。

### 预览图 / 原图分级权限

- 每条申请独立审批，馆方核定允许的最高分辨率（`preview` 或 `original`）与能否二次编辑；
- 仅获预览档的授权请求原图返回 **403**；待审批、已驳回授权返回 **410**；
- 同一张藏品的不同申请各自独立管理到期时间，互不影响。

### 撤回 / 过期后旧链接立即失效

- 下载接口**每次请求实时鉴权**（状态 + 到期时间 + 分辨率档位），不存在长期有效的静态链接；
  撤回或到期后的下一次请求即刻返回 **410 Gone**，无需等待任何定时任务。
- 后台每分钟巡检一次，把到期授权落库为 `expired` 并发送模拟到期提醒；巡检只影响列表状态
  与提醒，不承担安全职责。
- 撤回 / 过期后历史申请与**已发表位置记录全部保留**。

### 并发审批的一致性

审批与撤回都通过**带状态条件的单条 UPDATE** 落库：

```sql
UPDATE applications SET status='approved', ... WHERE id=? AND status='pending'
```

SQLite 写事务串行化，两个审批人同时处理同一申请时，只有一个事务的 `RowsAffected=1`；
另一个得到 **409 Conflict**（页面提示“该申请刚被其他审批人处理”）。因此不可能同时留下
“批准”和“驳回”两条矛盾结果。测试以 25 轮 HTTP 并发（`go test -race`）和脚本 20 轮真实
HTTP 并发双重验证，终态始终与唯一成功的决定一致，版本号恰好跃迁一次。

### 获准下载 ≠ 已发表

- 下载只代表拿到文件；申请人实际发表后须在授权详情里**登记刊物条目或页面 URL**；
- 馆方可在「发表位置」页按申请人、藏品编号、关键字检索，**默认包含已撤回 / 过期授权下
  仍挂在外部的引用**，状态列红色标注并显示撤回原因；
- 「导出 CSV」可导出全部位置（含授权状态与联系方式所需字段），用于联系使用方处理。

## 主要 API

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/auth/register` `/api/auth/login` | 注册申请人 / 登录 |
| GET | `/api/artifacts` | 藏品目录 |
| POST | `/api/artifacts` | 馆方录入藏品并上传原图（multipart） |
| POST | `/api/applications` | 提交使用申请（用途、渠道、期限、档位、编辑需求） |
| GET | `/api/applications` | 申请列表（申请人只见自己；带 `effective_status`） |
| POST | `/api/applications/:id/decide` | 馆方批准/驳回（并发冲突返回 409） |
| POST | `/api/applications/:id/revoke` | 馆方撤回（仅 approved 可撤回，重复撤回 409） |
| GET | `/api/applications/:id/download?resolution=preview\|original` | 实时鉴权下载 |
| POST | `/api/applications/:id/publications` | 登记发表位置 |
| GET | `/api/publications` | 检索发表位置（含失效授权的记录） |
| GET | `/api/publications/export` | 导出 CSV（馆方） |
| GET | `/api/emails` | 模拟邮件记录 |
| GET | `/api/download-logs` | 下载审计日志（含被拒绝的尝试，馆方） |

## 目录结构

```
cmd/server/        程序入口（嵌入前端、启动巡检、HTTP 服务）
internal/
  api/             Gin 路由、处理器、集成测试
  auth/            bcrypt + JWT 与鉴权中间件
  services/        授权状态机（条件更新）、下载鉴权、发表检索/CSV、过期巡检
  models/          GORM 模型与状态常量
  database/        SQLite 连接（WAL、忙等待）与自动迁移
  imaging/         尺寸读取与预览图生成（纯 Go）
  email/           模拟邮件（落库）
  seed/            演示账号、藏品与本地示例图片
  config/          环境变量配置
web/               Vue 3 前端（dist 由 go:embed 嵌入）
run.sh             一条命令构建并启动
scripts/e2e-check.sh  端到端验证脚本
```
