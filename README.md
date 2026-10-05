# 博物馆藏品图片授权与引用撤回系统

馆藏图片对外授权（刊物 / 网页 / 新媒体）的申请、审批、分级下载、发表登记与撤回追溯系统。

- **后端**：Go + Gin + GORM（纯 Go 驱动 SQLite，无需 CGO）
- **前端**：Vue 3 + Vite（无第三方 UI 框架，构建后由后端直接托管）
- **图片**：保存在本地，入库时自动生成 **预览版（600px）/ 网页版（1600px）/ 原图（印刷级）** 三个文件
- **邮件**：模拟接口，所有提醒落库，在「邮件记录」页可查
- **启动**：`./run.sh` 一条命令

## 快速开始

```bash
./run.sh
# 打开 http://localhost:8080
```

演示账号（登录页可一键填入）：

| 账号 | 角色 |
| --- | --- |
| `curator` | 馆方：审批、撤回、录入图片、查看全部引用位置、导出 CSV |
| `alice` / `bob` | 申请人：提交申请、分级下载、登记发表位置 |

首次启动自动生成 6 件演示藏品、若干待审批/已批准申请与一条已登记引用记录。
数据位于 `backend/data/`（SQLite + 图片文件），删除即可重置。

开发模式（前端热更新）：

```bash
(cd backend && go run ./cmd/server)      # :8080 API
(cd frontend && npm install && npm run dev)  # :5173 页面，/api 代理到 8080
```

## 业务规则

1. **申请**：申请人选择藏品，填写用途、渠道（刊物/网页/新媒体）、授权起止日期。同一图片可提交多条申请，**各自独立管理期限**。
2. **审批**（仅馆方）：确定允许下载的最高分辨率（预览/网页/原图）、能否二次编辑，可调整批准期限。审批只对当前这条申请生效。
3. **下载**：申请人在「可下载」分区按授权等级取件；下载链接为短时（10 分钟）签名令牌，**取件时实时校验数据库中的授权状态**，不做静态文件直出。
4. **发表登记**：获准下载不等于已经发表。申请人发表后登记刊物名/页面地址；馆方可按网址搜索并**导出 CSV**（含申请人邮箱）联系使用方。
5. **撤回 / 到期**：
   - 馆方可随时撤回；到期判断在取件时实时计算（另每分钟同步一次数据库状态并发到期提醒邮件）；
   - 撤回或到期后，**旧下载链接立即停止提供原图**（返回 403，不返回任何图片字节），也不能换发新链接；
   - **历史申请与发表记录永久保留**，仍可检索与导出。
6. **并发审批一致性**：审批在事务中执行条件更新
   `UPDATE applications SET ... WHERE id=? AND status='pending'`，
   仅第一个请求影响 1 行（200），其余请求影响 0 行并返回 409，最终状态唯一、不可能出现互相矛盾的批准结果。

页面分区：**待审批 / 可下载 / 已失效（拒绝·撤回·过期）** 三区清晰分离。

## 验证（用户要求的三项保证）

```bash
cd backend && go test ./... -count=1
```

集成测试（httptest 全链路）覆盖：

| 测试 | 验证内容 |
| --- | --- |
| `TestRevokedDownloadBlocked` | 批准后可取原图；撤回后**旧链接 403**、不能换新链接；历史申请仍在 |
| `TestExpiredDownloadBlocked` | 到期瞬间旧链接 403（不依赖后台扫描协程） |
| `TestResolutionTierEnforced` | 仅批到网页级时，预览/网页可下、原图 403 |
| `TestConcurrentReviewConsistency` | 24 个并发批准/拒绝 × 5 轮：恰好 1 个成功、其余 409，最终状态唯一稳定（已用 `-race` 跑过） |
| `TestReviewOnlyActsOnTarget` | 审批一条申请不影响其他申请 |
| `TestPublicationsSurviveRevocation` | 撤回后已发表位置仍可搜索、CSV 仍可导出（含邮箱与撤回状态） |

## 目录结构

```
backend/
  cmd/server/main.go          入口
  internal/
    models/    数据模型（User/Image/Application/Publication/DownloadRecord/MailMessage）
    store/     SQLite 连接、迁移、种子数据
    media/     图片三级分辨率生成（纯 Go，含演示占位图生成）
    auth/      Cookie 令牌会话
    mail/      模拟邮件（落库）
    api/       Gin 路由、处理器、下载签名令牌、过期扫描；api_test.go 集成测试
frontend/
  src/views/   Collection / MyGrants(三区) / Review / Publications / Mails
run.sh         一条命令启动
```

## 主要 API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/auth/login` | 用户名登录（演示） |
| GET | `/api/images` | 藏品列表（预览图） |
| POST | `/api/images` | 馆方录入图片（自动生成三级文件） |
| GET/POST | `/api/applications` | 申请列表/提交申请 |
| POST | `/api/applications/:id/review` | 审批（并发冲突返回 409） |
| POST | `/api/applications/:id/revoke` | 撤回授权 |
| GET | `/api/applications/:id/download-token?level=1-3` | 换取短时下载链接 |
| GET | `/api/download?token=...` | 取件（撤回/到期实时 403） |
| GET/POST | `/api/publications`、`/api/applications/:id/publications` | 引用位置查询/登记 |
| GET | `/api/publications/export.csv` | 导出引用位置 CSV |
| GET | `/api/mails` | 模拟邮件记录 |
