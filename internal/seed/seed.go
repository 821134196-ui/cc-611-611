// Package seed 在首次启动时创建演示账号与藏品（含本地示例图片）。
package seed

import (
	"image"
	"image/color"
	"image/jpeg"
	"log"
	"os"
	"path/filepath"

	"gorm.io/gorm"

	"museumlicense/internal/auth"
	"museumlicense/internal/email"
	"museumlicense/internal/imaging"
	"museumlicense/internal/models"
)

type artifactSpec struct {
	code, name, dynasty, category, desc string
	bg                                  color.RGBA
}

var specs = []artifactSpec{
	{"A001", "青花缠枝莲纹梅瓶", "明·永乐", "瓷器", "景德镇官窑青花代表作，胎质细腻，发色浓艳。", color.RGBA{0x1f, 0x4e, 0x79, 0xff}},
	{"A002", "鎏金铜佛像", "唐", "造像", "铜鎏金释迦牟尼坐像，衣纹流畅，保存完好。", color.RGBA{0xbf, 0x8f, 0x00, 0xff}},
	{"A003", "山水人物图轴", "清·乾隆", "书画", "纸本设色，远山近水，文人雅集题材。", color.RGBA{0x37, 0x6e, 0x37, 0xff}},
}

// Run 幂等播种：已有用户 / 藏品时跳过。
func Run(db *gorm.DB, storageDir string) error {
	if err := os.MkdirAll(filepath.Join(storageDir, "originals"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(storageDir, "previews"), 0o755); err != nil {
		return err
	}

	var userCount int64
	db.Model(&models.User{}).Count(&userCount)
	if userCount == 0 {
		users := []models.User{
			{Username: "curator", Role: models.RoleCurator, Email: "curator@museum.local"},
			{Username: "applicant", Role: models.RoleApplicant, Email: "applicant@press.local"},
			{Username: "editor", Role: models.RoleApplicant, Email: "editor@web.local"},
		}
		for i := range users {
			h, err := auth.HashPassword("123456")
			if err != nil {
				return err
			}
			users[i].Password = h
			if err := db.Create(&users[i]).Error; err != nil {
				return err
			}
		}
		log.Println("已创建演示账号: curator/123456 (馆方), applicant/123456, editor/123456 (申请人)")
	}

	var artCount int64
	db.Model(&models.Artifact{}).Count(&artCount)
	if artCount == 0 {
		mailer := email.New(db) // 保持与业务相同的依赖（种子不发信）
		_ = mailer
		for _, sp := range specs {
			origPath := filepath.Join(storageDir, "originals", sp.code+".jpg")
			if err := drawPlaceholder(origPath, 1600, 1200, sp.bg, sp.name); err != nil {
				return err
			}
			w, h, _ := imaging.Dimensions(origPath)
			previewPath, err := imaging.MakePreview(origPath, filepath.Join(storageDir, "previews"), 1200)
			if err != nil {
				previewPath = ""
			}
			art := models.Artifact{
				Code: sp.code, Name: sp.name, Dynasty: sp.dynasty,
				Category: sp.category, Description: sp.desc,
				OriginalPath: origPath, PreviewPath: previewPath,
				OriginalW: w, OriginalH: h,
			}
			if err := db.Create(&art).Error; err != nil {
				return err
			}
		}
		log.Println("已创建 3 件示例藏品及本地图片（原图 + 预览图）")
	}
	return nil
}

// drawPlaceholder 生成一张带色块的 JPEG 作为示例原图，避免仓库内携带二进制素材。
func drawPlaceholder(path string, w, h int, bg color.RGBA, label string) error {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// 底色
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, bg)
		}
	}
	// 画一个浅色方框与对角线条带，让图有可见内容。
	light := color.RGBA{0xff, 0xff, 0xff, 0x55}
	for y := h / 4; y < h*3/4; y++ {
		for x := w / 4; x < w*3/4; x++ {
			if x == w/4 || x == w*3/4-1 || y == h/4 || y == h*3/4-1 {
				img.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
			}
		}
	}
	for i := 0; i < 200; i++ {
		img.SetRGBA(w/4+i*4, h/4+i*3, light)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return jpeg.Encode(f, img, &jpeg.Options{Quality: 90})
}
