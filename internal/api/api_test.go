package api

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"museumlicense/internal/auth"
	"museumlicense/internal/database"
	"museumlicense/internal/email"
	"museumlicense/internal/imaging"
	"museumlicense/internal/models"
	"museumlicense/internal/services"
)

type testEnv struct {
	router    http.Handler
	db        *gorm.DB
	lic       *services.LicenseService
	pub       *services.PublicationService
	dir       string
	curator1  string
	curator2  string
	applicant string
	artifact  models.Artifact
}

func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	storage := filepath.Join(dir, "storage")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	// 用户
	mkUser := func(username, role string) *models.User {
		h, _ := auth.HashPassword("123456")
		u := models.User{Username: username, Password: h, Role: role, Email: username + "@x.local"}
		if err := db.Create(&u).Error; err != nil {
			t.Fatalf("create user: %v", err)
		}
		return &u
	}
	mkUser("curator1", models.RoleCurator)
	mkUser("curator2", models.RoleCurator)
	mkUser("applicant", models.RoleApplicant)

	// 生成原图 + 预览
	origDir := filepath.Join(storage, "originals")
	prevDir := filepath.Join(storage, "previews")
	_ = os.MkdirAll(origDir, 0o755)
	origPath := filepath.Join(origDir, "T001.jpg")
	makeJPEG(t, origPath, 800, 600)
	previewPath, err := imaging.MakePreview(origPath, prevDir, 400)
	if err != nil {
		t.Fatalf("make preview: %v", err)
	}
	w, ih, _ := imaging.Dimensions(origPath)
	art := models.Artifact{
		Code: "T001", Name: "测试藏品", Dynasty: "测试代", Category: "测试类",
		OriginalPath: origPath, PreviewPath: previewPath, OriginalW: w, OriginalH: ih,
	}
	if err := db.Create(&art).Error; err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	mailer := email.New(db)
	lic := services.NewLicenseService(db, mailer)
	pub := services.NewPublicationService(db)
	h := &Handlers{DB: db, Lic: lic, Pub: pub, Mailer: mailer,
		Auth: auth.NewManager("test-secret"), Storage: storage}

	env := &testEnv{
		router:    h.Router(nil),
		db:        db,
		lic:       lic,
		pub:       pub,
		dir:       dir,
		curator1:  loginToken(t, h, "curator1", "123456"),
		curator2:  loginToken(t, h, "curator2", "123456"),
		applicant: loginToken(t, h, "applicant", "123456"),
		artifact:  art,
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return env
}

func makeJPEG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 120, 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create image: %v", err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
}

func loginToken(t *testing.T, h *Handlers, username, password string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Router(nil).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login %s: status %d body %s", username, w.Code, w.Body.String())
	}
	var resp struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return resp.Token
}

func (e *testEnv) do(t *testing.T, method, path, token string, payload interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *bytes.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	return w
}

// createAndApprove 走完整 HTTP 流程：申请人提交、馆方批准，返回申请 ID。
func (e *testEnv) createAndApprove(t *testing.T, tier string, expire time.Time) uint {
	t.Helper()
	payload := map[string]interface{}{
		"artifact_id":    e.artifact.ID,
		"purpose":        "学术期刊配图",
		"channel":        "刊物《典藏研究》",
		"requested_tier": tier,
		"requested_edit": true,
		"start_at":       time.Now().Format(time.RFC3339),
		"end_at":         time.Now().Add(72 * time.Hour).Format(time.RFC3339),
	}
	w := e.do(t, http.MethodPost, "/api/applications", e.applicant, payload)
	if w.Code != http.StatusCreated {
		t.Fatalf("create application: %d %s", w.Code, w.Body.String())
	}
	var created struct {
		Item models.Application `json:"item"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)

	dec := map[string]interface{}{
		"approve":         true,
		"resolution_tier": tier,
		"allow_edit":      tier == models.TierOriginal,
		"expires_at":      expire.Format(time.RFC3339),
		"note":            "同意",
	}
	w = e.do(t, http.MethodPost,
		"/api/applications/"+strconv.FormatUint(uint64(created.Item.ID), 10)+"/decide", e.curator1, dec)
	if w.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", w.Code, w.Body.String())
	}
	return created.Item.ID
}

func itoa(u uint) string {
	return strconv.FormatUint(uint64(u), 10)
}

// ---- 场景 1：撤回后原图与预览立即停止下载，但历史与发表位置保留 ----

func TestRevokeStopsDownloadButKeepsPublications(t *testing.T) {
	e := setupTestEnv(t)
	appID := e.createAndApprove(t, models.TierOriginal, time.Now().Add(48*time.Hour))

	// 批准后：原图、预览均可下载。
	if w := e.do(t, http.MethodGet,
		"/api/applications/"+itoa(appID)+"/download?resolution=original", e.applicant, nil); w.Code != http.StatusOK {
		t.Fatalf("授权期内原图应可下载，得到 %d %s", w.Code, w.Body.String())
	}
	if w := e.do(t, http.MethodGet,
		"/api/applications/"+itoa(appID)+"/download?resolution=preview", e.applicant, nil); w.Code != http.StatusOK {
		t.Fatalf("授权期内预览应可下载，得到 %d", w.Code)
	}

	// 申请人登记已发表位置（下载 ≠ 发表）。
	pubPayload := map[string]interface{}{
		"title": "典藏研究 2026 年第 3 期 第 12 页",
		"url":   "https://press.example.com/journal/2026/3/p12",
		"note":  "已见刊",
	}
	if w := e.do(t, http.MethodPost,
		"/api/applications/"+itoa(appID)+"/publications", e.applicant, pubPayload); w.Code != http.StatusCreated {
		t.Fatalf("登记发表位置失败: %d %s", w.Code, w.Body.String())
	}

	// 馆方撤回。
	revoke := map[string]interface{}{"reason": "用途与申请不符"}
	if w := e.do(t, http.MethodPost,
		"/api/applications/"+itoa(appID)+"/revoke", e.curator1, revoke); w.Code != http.StatusOK {
		t.Fatalf("撤回失败: %d %s", w.Code, w.Body.String())
	}

	// 撤回后：原图立即无法下载（410 Gone）。
	if w := e.do(t, http.MethodGet,
		"/api/applications/"+itoa(appID)+"/download?resolution=original", e.applicant, nil); w.Code != http.StatusGone {
		t.Fatalf("撤回后原图必须立即停止提供，得到 %d %s", w.Code, w.Body.String())
	}
	// 预览同样停止。
	if w := e.do(t, http.MethodGet,
		"/api/applications/"+itoa(appID)+"/download?resolution=preview", e.applicant, nil); w.Code != http.StatusGone {
		t.Fatalf("撤回后预览也应停止，得到 %d %s", w.Code, w.Body.String())
	}
	// 无 token 的旧链接更不能用。
	req := httptest.NewRequest(http.MethodGet,
		"/api/applications/"+itoa(appID)+"/download?resolution=original", nil)
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("旧链接无凭证应 401，得到 %d", w.Code)
	}

	// 历史申请仍在且状态为 revoked。
	w = e.do(t, http.MethodGet, "/api/applications/"+itoa(appID), e.curator1, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("撤回后历史申请应可查询，得到 %d", w.Code)
	}
	var detail struct {
		EffectiveStatus string `json:"effective_status"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &detail)
	if detail.EffectiveStatus != models.StatusRevoked {
		t.Fatalf("状态应为 revoked，得到 %q", detail.EffectiveStatus)
	}

	// 已发表位置仍可被馆方检索到。
	w = e.do(t, http.MethodGet, "/api/publications?keyword=press.example.com", e.curator1, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("检索发表位置失败: %d", w.Code)
	}
	var pubs struct {
		Items []struct {
			URL             string `json:"url"`
			EffectiveStatus string `json:"effective_status"`
		} `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &pubs)
	if len(pubs.Items) != 1 || pubs.Items[0].URL != "https://press.example.com/journal/2026/3/p12" {
		t.Fatalf("撤回后发表位置应仍可查找，得到 %+v", pubs.Items)
	}
	if pubs.Items[0].EffectiveStatus != models.StatusRevoked {
		t.Fatalf("导出检索中应标明该引用对应授权已撤回")
	}

	// CSV 导出包含该 URL 与撤回状态，供馆方联系使用方。
	w = e.do(t, http.MethodGet, "/api/publications/export", e.curator1, nil)
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte("press.example.com")) ||
		!bytes.Contains(w.Body.Bytes(), []byte(models.StatusRevoked)) {
		t.Fatalf("CSV 导出须包含撤回授权的发表链接与状态，body=%s", w.Body.String())
	}

	// 申请人收到撤回模拟邮件。
	w = e.do(t, http.MethodGet, "/api/emails", e.applicant, nil)
	if !bytes.Contains(w.Body.Bytes(), []byte("撤回")) {
		t.Fatalf("应生成撤回提醒邮件，body=%s", w.Body.String())
	}

	// 重复撤回必须冲突，不能改变最终结果。
	w = e.do(t, http.MethodPost,
		"/api/applications/"+itoa(appID)+"/revoke", e.curator2, map[string]interface{}{"reason": "再撤一次"})
	if w.Code != http.StatusConflict {
		t.Fatalf("重复撤回应 409，得到 %d", w.Code)
	}
}

// ---- 场景 2：到期后旧链接立即失效（无需等待巡检），巡检后状态持久化 ----

func TestExpiredStopsOriginalDownload(t *testing.T) {
	e := setupTestEnv(t)
	appID := e.createAndApprove(t, models.TierOriginal, time.Now().Add(48*time.Hour))

	// 时间流逝：直接把到期时间改到过去，模拟授权到期（尚未经任何下载触发）。
	res := e.db.Model(&models.Application{}).Where("id = ?", appID).
		Update("expires_at", time.Now().Add(-time.Minute))
	if res.Error != nil {
		t.Fatalf("backdate: %v", res.Error)
	}

	// 巡检把状态标记为 expired 并发到期邮件（此时还没有下载请求触发过状态变更）。
	var app models.Application
	_ = e.db.First(&app, appID).Error
	svc := services.NewLicenseService(e.db, email.New(e.db))
	expired, err := svc.SweepExpired()
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if len(expired) != 1 {
		t.Fatalf("应巡检出 1 条到期授权，得到 %d", len(expired))
	}
	_ = e.db.First(&app, appID).Error
	if app.Status != models.StatusExpired {
		t.Fatalf("巡检后状态应为 expired，得到 %q", app.Status)
	}
	// 巡检幂等：第二次不应重复处理 / 重复发信。
	again, _ := svc.SweepExpired()
	if len(again) != 0 {
		t.Fatalf("重复巡检不应再次处理，得到 %d 条", len(again))
	}

	// 巡检（或实时判期）之后，旧下载链接一律 410。
	if w := e.do(t, http.MethodGet,
		"/api/applications/"+itoa(appID)+"/download?resolution=original", e.applicant, nil); w.Code != http.StatusGone {
		t.Fatalf("过期后原图旧链接应失效，得到 %d %s", w.Code, w.Body.String())
	}
	if w := e.do(t, http.MethodGet,
		"/api/applications/"+itoa(appID)+"/download?resolution=preview", e.applicant, nil); w.Code != http.StatusGone {
		t.Fatalf("过期后预览也应失效，得到 %d", w.Code)
	}

	// 另一条授权：不执行巡检，直接下载，也必须被实时拒绝（无等待窗口）。
	idRealtime := e.createAndApprove(t, models.TierOriginal, time.Now().Add(48*time.Hour))
	e.db.Model(&models.Application{}).Where("id = ?", idRealtime).
		Update("expires_at", time.Now().Add(-time.Minute))
	if w := e.do(t, http.MethodGet,
		"/api/applications/"+itoa(idRealtime)+"/download?resolution=original", e.applicant, nil); w.Code != http.StatusGone {
		t.Fatalf("未巡检的过期授权，原图请求也应实时拒绝，得到 %d %s", w.Code, w.Body.String())
	}

	// 到期后发表记录依然可登记、可查（历史记录保留）。
	pubPayload := map[string]interface{}{
		"title": "过期后补登记的网页引用",
		"url":   "https://web.example.com/article/expired-ok",
	}
	if w := e.do(t, http.MethodPost,
		"/api/applications/"+itoa(appID)+"/publications", e.applicant, pubPayload); w.Code != http.StatusCreated {
		t.Fatalf("授权过期后历史申请仍应允许补登记发表位置: %d %s", w.Code, w.Body.String())
	}
	w := e.do(t, http.MethodGet, "/api/publications", e.curator1, nil)
	if !bytes.Contains(w.Body.Bytes(), []byte("expired-ok")) {
		t.Fatalf("过期授权下的发表位置仍须可查找，body=%s", w.Body.String())
	}
}

// ---- 场景 3：预览/原图分级权限 ----

func TestResolutionTierEnforced(t *testing.T) {
	e := setupTestEnv(t)
	// 只批准预览档位。
	appID := e.createAndApprove(t, models.TierPreview, time.Now().Add(24*time.Hour))

	if w := e.do(t, http.MethodGet,
		"/api/applications/"+itoa(appID)+"/download?resolution=preview", e.applicant, nil); w.Code != http.StatusOK {
		t.Fatalf("预览档位应可下载预览图，得到 %d", w.Code)
	}
	if w := e.do(t, http.MethodGet,
		"/api/applications/"+itoa(appID)+"/download?resolution=original", e.applicant, nil); w.Code != http.StatusForbidden {
		t.Fatalf("预览档位不得下载原图，应 403，得到 %d %s", w.Code, w.Body.String())
	}

	// 待审批状态不能下载。
	pending := e.createPendingApplication(t)
	if w := e.do(t, http.MethodGet,
		"/api/applications/"+itoa(pending)+"/download?resolution=preview", e.applicant, nil); w.Code != http.StatusGone {
		t.Fatalf("待审批申请不能下载，应 410，得到 %d", w.Code)
	}
}

func (e *testEnv) createPendingApplication(t *testing.T) uint {
	t.Helper()
	payload := map[string]interface{}{
		"artifact_id":    e.artifact.ID,
		"purpose":        "网页配图",
		"channel":        "网站",
		"requested_tier": models.TierPreview,
		"end_at":         time.Now().Add(72 * time.Hour).Format(time.RFC3339),
	}
	w := e.do(t, http.MethodPost, "/api/applications", e.applicant, payload)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var created struct {
		Item models.Application `json:"item"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	return created.Item.ID
}

// ---- 场景 4：并发审批同一申请，最终结果唯一且不矛盾 ----

func TestConcurrentDecisionConsistency(t *testing.T) {
	e := setupTestEnv(t)
	const rounds = 25
	for r := 0; r < rounds; r++ {
		appID := e.createPendingApplication(t)
		path := "/api/applications/" + itoa(appID) + "/decide"

		expire := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
		approveReq, _ := json.Marshal(map[string]interface{}{
			"approve": true, "resolution_tier": models.TierOriginal,
			"allow_edit": true, "expires_at": expire, "note": "A 批准",
		})
		rejectReq, _ := json.Marshal(map[string]interface{}{
			"approve": false, "note": "B 驳回",
		})

		var wg sync.WaitGroup
		statuses := make([]int, 2)
		var mu sync.Mutex
		fire := func(token string, body []byte, idx int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			e.router.ServeHTTP(w, req)
			mu.Lock()
			statuses[idx] = w.Code
			mu.Unlock()
		}
		wg.Add(2)
		go fire(e.curator1, approveReq, 0)
		go fire(e.curator2, rejectReq, 1)
		wg.Wait()

		// 恰有一个 200，另一个必须是 409（不能两个都成功）。
		ok := (statuses[0] == http.StatusOK && statuses[1] == http.StatusConflict) ||
			(statuses[1] == http.StatusOK && statuses[0] == http.StatusConflict)
		if !ok {
			t.Fatalf("第 %d 轮并发审批结果矛盾: %v（应恰有一个 200，另一个 409）", r, statuses)
		}

		// 数据库中只能有一个终态，且与该 200 的决定一致。
		var app models.Application
		if err := e.db.First(&app, appID).Error; err != nil {
			t.Fatalf("reload: %v", err)
		}
		if app.Status != models.StatusApproved && app.Status != models.StatusRejected {
			t.Fatalf("终态非法: %q", app.Status)
		}
		if statuses[0] == http.StatusOK && app.Status != models.StatusApproved {
			t.Fatalf("批准请求成功但终态不是 approved: %q", app.Status)
		}
		if statuses[1] == http.StatusOK && app.Status != models.StatusRejected {
			t.Fatalf("驳回请求成功但终态不是 rejected: %q", app.Status)
		}
		if app.Version != 1 {
			t.Fatalf("状态应恰好跃迁一次（version=1），得到 %d", app.Version)
		}
	}
}

// ---- 场景 5：撤回与审批并发同样不能产生矛盾 ----

func TestConcurrentApproveAndRevoke(t *testing.T) {
	e := setupTestEnv(t)
	const rounds = 15
	for r := 0; r < rounds; r++ {
		// 先批准，再并发地“重复审批(另一人驳回)”与“撤回”。
		appID := e.createAndApprove(t, models.TierPreview, time.Now().Add(24*time.Hour))
		decidePath := "/api/applications/" + itoa(appID) + "/decide"
		revokePath := "/api/applications/" + itoa(appID) + "/revoke"

		results := make(chan int, 2)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			body, _ := json.Marshal(map[string]interface{}{"approve": false, "note": "改驳回"})
			req := httptest.NewRequest(http.MethodPost, decidePath, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+e.curator2)
			w := httptest.NewRecorder()
			e.router.ServeHTTP(w, req)
			results <- w.Code
		}()
		go func() {
			defer wg.Done()
			body, _ := json.Marshal(map[string]interface{}{"reason": "紧急撤回"})
			req := httptest.NewRequest(http.MethodPost, revokePath, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+e.curator1)
			w := httptest.NewRecorder()
			e.router.ServeHTTP(w, req)
			results <- w.Code
		}()
		wg.Wait()
		close(results)
		for code := range results {
			if code != http.StatusOK && code != http.StatusConflict {
				t.Fatalf("第 %d 轮只允许 200/409，得到 %d", r, code)
			}
		}
		var app models.Application
		_ = e.db.First(&app, appID).Error
		if app.Status != models.StatusApproved && app.Status != models.StatusRevoked {
			t.Fatalf("终态只能是 approved 或 revoked，得到 %q", app.Status)
		}
	}
}

// ---- 场景 6：同一张图的不同申请独立管理期限 ----

func TestMultipleApplicationsIndependentExpiry(t *testing.T) {
	e := setupTestEnv(t)
	id1 := e.createAndApprove(t, models.TierPreview, time.Now().Add(48*time.Hour))
	id2 := e.createAndApprove(t, models.TierOriginal, time.Now().Add(48*time.Hour))
	// 把第 2 条授权回溯为已到期，第 1 条保持有效：同一藏品互不影响。
	e.db.Model(&models.Application{}).Where("id = ?", id2).
		Update("expires_at", time.Now().Add(-time.Minute))

	// id1 仍有效，id2 已失效：同一藏品互不影响。
	if w := e.do(t, http.MethodGet,
		"/api/applications/"+itoa(id1)+"/download?resolution=preview", e.applicant, nil); w.Code != http.StatusOK {
		t.Fatalf("授权1应仍有效，得到 %d", w.Code)
	}
	if w := e.do(t, http.MethodGet,
		"/api/applications/"+itoa(id2)+"/download?resolution=original", e.applicant, nil); w.Code != http.StatusGone {
		t.Fatalf("授权2已过期应拒绝原图，得到 %d", w.Code)
	}
}

// ---- 场景 7：申请人不能互相查看 / 下载他人申请 ----

func TestCrossUserForbidden(t *testing.T) {
	e := setupTestEnv(t)
	appID := e.createAndApprove(t, models.TierOriginal, time.Now().Add(24*time.Hour))

	// 注册第二个申请人。
	b, _ := json.Marshal(map[string]string{"username": "other", "password": "123456"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("register: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if got := e.do(t, http.MethodGet,
		"/api/applications/"+itoa(appID)+"/download?resolution=original", resp.Token, nil); got.Code != http.StatusForbidden {
		t.Fatalf("他人申请下载应 403，得到 %d", got.Code)
	}
}
