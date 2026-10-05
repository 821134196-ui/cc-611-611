// Package imaging 负责读取原图尺寸并生成预览缩略图（纯 Go，无 CGO 依赖）。
package imaging

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

// Dimensions 读取图片宽高。
func Dimensions(path string) (w, h int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}

// MakePreview 把原图缩放到最长边不超过 maxPx，以 JPEG 写入 outDir，
// 返回预览图路径。原图本身保持不变，预览图是独立的低分辨率派生文件。
func MakePreview(src, outDir string, maxPx int) (string, error) {
	if maxPx <= 0 {
		maxPx = 1200
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	srcF, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer srcF.Close()

	img, _, err := image.Decode(srcF)
	if err != nil {
		return "", fmt.Errorf("解码图片失败: %w", err)
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	scale := 1.0
	if maxSide := max(w, h); maxSide > maxPx {
		scale = float64(maxPx) / float64(maxSide)
	}

	var preview image.Image = img
	if scale < 1.0 {
		nw, nh := max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))
		preview = resize(img, nw, nh)
	}

	base := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	outPath := filepath.Join(outDir, base+"_preview.jpg")
	out, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if err := jpeg.Encode(out, preview, &jpeg.Options{Quality: 85}); err != nil {
		return "", err
	}
	return outPath, nil
}

// init 注册解码器，避免调用方分别导入。
func init() {
	image.RegisterFormat("jpeg", "jpeg", jpeg.Decode, jpeg.DecodeConfig)
	image.RegisterFormat("png", "png", png.Decode, png.DecodeConfig)
}

// resize 使用最近邻缩放（无第三方依赖）；预览图用途足够。
func resize(src image.Image, width, height int) image.Image {
	bounds := src.Bounds()
	sw, sh := bounds.Dx(), bounds.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		sy := bounds.Min.Y + y*sh/height
		for x := 0; x < width; x++ {
			sx := bounds.Min.X + x*sw/width
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

// init 注册解码器，避免调用方分别导入。
func init() {
	image.RegisterFormat("jpeg", "jpeg", jpeg.Decode, jpeg.DecodeConfig)
	image.RegisterFormat("png", "png", png.Decode, png.DecodeConfig)
}
