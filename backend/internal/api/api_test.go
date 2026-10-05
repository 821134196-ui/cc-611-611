package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"museum-license/internal/auth"
	"museum-license/internal/config"
	"museum-license/internal/mail"
	"museum-license/internal/models"
	"museum-license/internal/store"
)

type testEnv struct{}

func setup(t *testing.T) (*Server, *httptest.Server, func()) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{
		Port:      "0",
		DataDir:   dir,
		StaticDir: filepath.Join(dir, "no-dist"),
	}
	gdb, err := store.Open(cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	am := auth.New(gdb)
	ms := mail.New(gdb)
	srv := NewServer(gdb, cfg, am, ms)
	ts := httptest.NewServer(srv.Router())
	return srv, ts, func() { ts.Close() }
}

type client struct {
	t    *testing.T
	h    *http.Client
	base string
}

func newClient(t *testing.T, ts *httptest.Server) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, h: &http.Client{Jar: jar}, base: ts.URL}
}

func (c *client) login(username string) {
	resp, err := c.h.Post(c.base+"/api/auth/login", "application/json",
		bytes.NewBufferString(fmt.Sprintf(`{"username":%q}`, username)))
	if err != nil {
		c.t.Fatalf("login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		c.t.Fatalf("login %s status %d: %s", username, resp.StatusCode, body)
	}
}

func (c *client) do(method, path, body string) (int, string) {
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, c.base+path, rdr)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.h.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func (c *client) getJSON(path string, out any) int {
	resp, err := c.h.Get(c.base + path)
	if err != nil {
		c.t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(b, out)
	return resp.StatusCode
}

// createPendingApp 以 alice 身份创建一条待审批申请，返回 ID
func createPendingApp(t *testing.T, alice *client, imageID uint) uint {
	body := fmt.Sprintf(`{"imageId":%d,"purpose":"测试用途","channel":"web","startAt":"2026-10-01","expireAt":"2027-10-01"}`, imageID)
	st, raw := alice.do("POST", "/api/applications", body)
	if st != 201 {
		t.Fatalf("create application: %d %s", st, raw)
	}
	var resp struct {
		Item struct {
			ID uint `json:"id"`
		} `json:"item"`
	}
	_ = json.Unmarshal([]byte(raw), &resp)
	return resp.Item.ID
}

// approveApp 馆方批准申请，指定分辨率等级与到期日
func approveApp(t *testing.T, staff *client, appID uint, level int, expireAt string) {
	body := fmt.Sprintf(`{"approve":true,"maxResolution":%d,"canEdit":true,"expireAt":%q,"note":"ok"}`, level, expireAt)
	st, raw := staff.do("POST", fmt.Sprintf("/api/applications/%d/review", appID), body)
	if st != 200 {
		t.Fatalf("approve: %d %s", st, raw)
	}
}

func downloadToken(t *testing.T, cl *client, appID uint, level int) (int, string) {
	st, raw := cl.do("GET", fmt.Sprintf("/api/applications/%d/download-token?level=%d", appID, level), "")
	if st != 200 {
		return st, raw
	}
	var resp struct {
		URL string `json:"url"`
	}
	_ = json.Unmarshal([]byte(raw), &resp)
	return st, resp.URL
}

// fetchDownload 用令牌取件，返回状态码与响应体
func fetchDownload(t *testing.T, cl *client, url string) (int, []byte) {
	resp, err := cl.h.Get(cl.base + url)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

// TestRevokedDownloadBlocked 验证要求一（撤回）：撤回后旧下载链接立即停止提供原图
func TestRevokedDownloadBlocked(t *testing.T) {
	srv, ts, closeFn := setup(t)
	defer closeFn()

	alice := newClient(t, ts)
	alice.login("alice")
	staff := newClient(t, ts)
	staff.login("curator")

	appID := createPendingApp(t, alice, 1)
	approveApp(t, staff, appID, models.ResPrint, "2027-10-01")

	// 批准后：可取得原图
	st, url := downloadToken(t, alice, appID, models.ResPrint)
	if st != 200 {
		t.Fatalf("批准后应能换取下载链接: %d %s", st, url)
	}
	code, body := fetchDownload(t, alice, url)
	if code != 200 || len(body) < 1000 {
		t.Fatalf("批准后原图下载应成功, got %d bytes=%d", code, len(body))
	}
	if ct := http.DetectContentType(body[:512]); !strings.HasPrefix(ct, "image/jpeg") {
		t.Fatalf("原图应为 JPEG, got %s", ct)
	}

	// 馆方撤回
	if st, raw := staff.do("POST", fmt.Sprintf("/api/applications/%d/revoke", appID), `{"reason":"授权争议"}`); st != 200 {
		t.Fatalf("撤回失败: %d %s", st, raw)
	}

	// 旧链接立即失效：不返回任何图片字节
	code, body = fetchDownload(t, alice, url)
	if code != 403 {
		t.Fatalf("撤回后旧链接必须 403, got %d", code)
	}
	if strings.Contains(string(body), "JFIF") {
		t.Fatal("撤回后响应不得包含图片数据")
	}

	// 撤回后也不允许换发新链接
	if st, _ := downloadToken(t, alice, appID, models.ResPreview); st != 403 {
		t.Fatalf("撤回后不得换发新链接, got %d", st)
	}

	// 历史申请记录仍保留
	var appResp struct {
		Item struct {
			Status       string `json:"status"`
			Effective    string `json:"effectiveStatus"`
			RevokeReason string `json:"revokeReason"`
		} `json:"item"`
	}
	if staff.getJSON(fmt.Sprintf("/api/applications/%d", appID), &appResp) != 200 {
		t.Fatal("撤回后历史申请应仍可查询")
	}
	if appResp.Item.Effective != models.StatusRevoked {
		t.Fatalf("状态应为 revoked, got %s", appResp.Item.Effective)
	}
	_ = srv
}

// TestExpiredDownloadBlocked 验证要求一（过期）：到期瞬间旧链接停止提供原图，不依赖后台扫描
func TestExpiredDownloadBlocked(t *testing.T) {
	srv, ts, closeFn := setup(t)
	defer closeFn()

	alice := newClient(t, ts)
	alice.login("alice")
	staff := newClient(t, ts)
	staff.login("curator")

	appID := createPendingApp(t, alice, 2)
	approveApp(t, staff, appID, models.ResPrint, "2027-10-01")
	_, url := downloadToken(t, alice, appID, models.ResPrint)

	// 直接把数据库中的到期时间改为过去（模拟时间流逝，链接早已发出）
	if err := srv.db.Model(&models.Application{}).Where("id = ?", appID).
		Update("expire_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}

	code, body := fetchDownload(t, alice, url)
	if code != 403 || !strings.Contains(string(body), "到期") {
		t.Fatalf("过期后旧链接应 403 并提示到期, got %d: %s", code, body)
	}
	if st, _ := downloadToken(t, alice, appID, models.ResPrint); st != 403 {
		t.Fatalf("过期后不得换发新链接, got %d", st)
	}
}

// TestResolutionTierEnforced 验证预览/原图分级权限
func TestResolutionTierEnforced(t *testing.T) {
	_, ts, closeFn := setup(t)
	defer closeFn()

	alice := newClient(t, ts)
	alice.login("alice")
	staff := newClient(t, ts)
	staff.login("curator")

	appID := createPendingApp(t, alice, 3)
	approveApp(t, staff, appID, models.ResWeb, "2027-10-01") // 仅批到网页版

	st, webURL := downloadToken(t, alice, appID, models.ResWeb)
	if st != 200 {
		t.Fatalf("网页级应可下载: %d %s", st, webURL)
	}
	if code, _ := fetchDownload(t, alice, webURL); code != 200 {
		t.Fatalf("网页级下载应成功, got %d", code)
	}
	if st, raw := downloadToken(t, alice, appID, models.ResPrint); st != 403 {
		t.Fatalf("未获原图授权时取原图必须 403, got %d: %s", st, raw)
	}
}

// TestConcurrentReviewConsistency 验证要求二：并发审批同一申请，最终只有一个一致结果
func TestConcurrentReviewConsistency(t *testing.T) {
	_, ts, closeFn := setup(t)
	defer closeFn()

	const n = 24
	// 每组用一个独立申请，重复 5 轮，降低偶然性
	for round := 0; round < 5; round++ {
		staffClients := make([]*client, n)
		for i := range staffClients {
			staffClients[i] = newClient(t, ts)
			staffClients[i].login("curator")
		}
		alice := newClient(t, ts)
		alice.login("alice")
		appID := createPendingApp(t, alice, 4)

		var wg sync.WaitGroup
		statuses := make([]int, n)
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				var body string
				if i%2 == 0 {
					body = `{"approve":true,"maxResolution":3,"canEdit":false,"expireAt":"2027-10-01"}`
				} else {
					body = `{"approve":false,"note":"材料不全"}`
				}
				statuses[i], _ = staffClients[i].do("POST",
					fmt.Sprintf("/api/applications/%d/review", appID), body)
			}(i)
		}
		wg.Wait()

		ok, conflict := 0, 0
		for _, st := range statuses {
			switch st {
			case 200:
				ok++
			case 409:
				conflict++
			default:
				t.Fatalf("审批只应返回 200 或 409, got %d", st)
			}
		}
		if ok != 1 || conflict != n-1 {
			t.Fatalf("第 %d 轮: 必须恰好 1 个成功、其余冲突, got 成功=%d 冲突=%d", round, ok, conflict)
		}

		// 多次读取，状态必须稳定且唯一（无互相矛盾的批准结果）
		final := ""
		for k := 0; k < 5; k++ {
			var resp struct {
				Item struct {
					Status string `json:"status"`
				} `json:"item"`
			}
			staffClients[0].getJSON(fmt.Sprintf("/api/applications/%d", appID), &resp)
			if final == "" {
				final = resp.Item.Status
			} else if resp.Item.Status != final {
				t.Fatalf("最终状态读取不一致: %s vs %s", final, resp.Item.Status)
			}
			if final != models.StatusApproved && final != models.StatusRejected {
				t.Fatalf("最终状态必须确定, got %s", final)
			}
		}
	}
}

// TestReviewOnlyActsOnTarget 审批只针对相应申请生效
func TestReviewOnlyActsOnTarget(t *testing.T) {
	_, ts, closeFn := setup(t)
	defer closeFn()

	alice := newClient(t, ts)
	alice.login("alice")
	staff := newClient(t, ts)
	staff.login("curator")

	id1 := createPendingApp(t, alice, 5)
	id2 := createPendingApp(t, alice, 6)
	approveApp(t, staff, id1, models.ResPreview, "2027-10-01")

	var resp struct {
		Item struct {
			Status string `json:"status"`
		} `json:"item"`
	}
	staff.getJSON(fmt.Sprintf("/api/applications/%d", id2), &resp)
	if resp.Item.Status != models.StatusPending {
		t.Fatalf("审批 id1 不应影响 id2, id2=%s", resp.Item.Status)
	}
}

// TestPublicationsSurviveRevocation 验证要求三：撤回/过期后已发表位置仍可查找与导出
func TestPublicationsSurviveRevocation(t *testing.T) {
	_, ts, closeFn := setup(t)
	defer closeFn()

	alice := newClient(t, ts)
	alice.login("alice")
	staff := newClient(t, ts)
	staff.login("curator")

	appID := createPendingApp(t, alice, 1)
	approveApp(t, staff, appID, models.ResPrint, "2027-10-01")

	pubBody := `{"title":"年度大展导览网页","publisher":"艺文在线","url":"https://example.org/article/xyz","channel":"web","publishedAt":"2026-09-01"}`
	if st, raw := alice.do("POST", fmt.Sprintf("/api/applications/%d/publications", appID), pubBody); st != 201 {
		t.Fatalf("登记发表位置失败: %d %s", st, raw)
	}

	// 撤回授权
	if _, raw := staff.do("POST", fmt.Sprintf("/api/applications/%d/revoke", appID), `{"reason":"超范围使用"}`); !strings.Contains(raw, "ok") {
		t.Fatalf("撤回失败: %s", raw)
	}

	// 馆方仍可检索到该引用位置
	var listResp struct {
		Items []struct {
			URL         string `json:"url"`
			Application struct {
				EffectiveStatus string `json:"effectiveStatus"`
				Image           struct {
					Title string `json:"title"`
				} `json:"image"`
				Applicant struct {
					Email string `json:"email"`
				} `json:"applicant"`
			} `json:"application"`
		} `json:"items"`
	}
	if staff.getJSON("/api/publications?q=example.org", &listResp) != 200 {
		t.Fatal("撤回后发表位置查询失败")
	}
	found := false
	for _, it := range listResp.Items {
		if strings.Contains(it.URL, "example.org/article/xyz") {
			found = true
			if it.Application.EffectiveStatus != models.StatusRevoked {
				t.Fatalf("关联授权应显示 revoked, got %s", it.Application.EffectiveStatus)
			}
		}
	}
	if !found {
		t.Fatal("撤回后必须仍能查到已发表位置")
	}

	// CSV 导出仍包含该行，且标注授权已撤回，可用于联系使用方
	resp, err := staff.h.Get(ts.URL + "/api/publications/export.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	csvBytes, _ := io.ReadAll(resp.Body)
	csvText := string(csvBytes)
	if !strings.Contains(csvText, "example.org/article/xyz") || !strings.Contains(csvText, "revoked") {
		t.Fatalf("导出 CSV 必须保留该引用位置并标注撤回状态:\n%s", csvText)
	}
	if !strings.Contains(csvText, "alice@press.example") {
		t.Fatal("CSV 应包含申请人联系邮箱，便于馆方联系使用方")
	}

	// 申请人本人同样还能看到自己的历史登记
	var mine struct {
		Items []struct {
			URL string `json:"url"`
		} `json:"items"`
	}
	alice.getJSON("/api/publications", &mine)
	if len(mine.Items) == 0 {
		t.Fatal("申请人视角也应保留发表登记历史")
	}
}
