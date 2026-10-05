// Package media 负责本地图片的入库与多分辨率生成。
// 原图、网页版、预览版分别保存为独立文件，授权按分辨率等级控制取件。
package media

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"

	xdraw "golang.org/x/image/draw"
)

const (
	PreviewMaxWidth = 600
	WebMaxWidth     = 1600
)

// Encode 一张源图生成三级 JPEG：preview(<=600px)、web(<=1600px)、print(原图)
func Encode(src image.Image, dir, baseName string) (previewPath, webPath, printPath string, w, h int, err error) {
	b := src.Bounds()
	w, h = b.Dx(), b.Dy()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", "", 0, 0, err
	}
	previewPath = filepath.Join(dir, baseName+"_preview.jpg")
	webPath = filepath.Join(dir, baseName+"_web.jpg")
	printPath = filepath.Join(dir, baseName+"_print.jpg")

	if e := writeJPEG(printPath, src, 92); e != nil {
		err = e
		return
	}
	webImg := resize(src, WebMaxWidth)
	if e := writeJPEG(webPath, webImg, 85); e != nil {
		err = e
		return
	}
	prevImg := resize(src, PreviewMaxWidth)
	if e := writeJPEG(previewPath, prevImg, 80); e != nil {
		err = e
		return
	}
	return
}

// Decode 从文件解码图片（jpg/png）
func Decode(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return DecodeReader(f)
}

// DecodeReader 从流解码图片（jpg/png）
func DecodeReader(r io.Reader) (image.Image, error) {
	// 嗅探格式：尝试 PNG 魔数
	head := make([]byte, 8)
	n, _ := io.ReadFull(r, head)
	r = io.MultiReader(bytes.NewReader(head[:n]), r)
	if n >= 8 && bytes.Equal(head[:8], []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}) {
		return png.Decode(r)
	}
	return jpeg.Decode(r)
}

func resize(src image.Image, maxWidth int) image.Image {
	b := src.Bounds()
	w := b.Dx()
	if w <= maxWidth {
		return src
	}
	newW := maxWidth
	newH := b.Dy() * newW / w
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	xdraw.CatmullRom.Scale(dst, dst.Rect, src, b, xdraw.Over, nil)
	return dst
}

func writeJPEG(path string, img image.Image, q int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return jpeg.Encode(f, img, &jpeg.Options{Quality: q})
}

// Placeholder 生成一张用于种子数据的演示“藏品图”：纯色底 + 色块 + 编号条
func Placeholder(seedNo int, title string, width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	palettes := []color.RGBA{
		{244, 236, 220, 255}, {224, 232, 240, 255}, {238, 228, 228, 255},
		{228, 238, 230, 255}, {240, 234, 214, 255}, {232, 226, 240, 255},
	}
	bg := palettes[seedNo%len(palettes)]
	draw.Draw(img, img.Bounds(), &image.Uniform{C: bg}, image.Point{}, draw.Src)

	// 中央“器物”色块
	accents := []color.RGBA{
		{120, 60, 40, 255}, {50, 80, 110, 255}, {90, 100, 70, 255},
		{130, 100, 40, 255}, {90, 60, 100, 255}, {40, 100, 100, 255},
	}
	ac := accents[seedNo%len(accents)]
	cx0, cy0 := width/4, height/5
	cx1, cy1 := width*3/4, height*4/5
	draw.Draw(img, image.Rect(cx0, cy0, cx1, cy1), &image.Uniform{C: ac}, image.Point{}, draw.Src)
	// 高光
	hl := color.RGBA{ac.R + 50, ac.G + 50, ac.B + 50, 255}
	draw.Draw(img, image.Rect(cx0, cy0, cx1, cy0+height/14), &image.Uniform{C: hl}, image.Point{}, draw.Over)

	// 底部深色编号条
	bar := color.RGBA{40, 40, 45, 255}
	draw.Draw(img, image.Rect(0, height-height/12, width, height), &image.Uniform{C: bar}, image.Point{}, draw.Src)
	label := color.RGBA{235, 235, 230, 255}
	drawString(img, fmt.Sprintf("NO.%04d  %s", seedNo, truncate(title, 18)), width/30, height-height/28, label)
	return img
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// 极简点阵字体（5x7），仅覆盖演示种子所需字符子集
var font5x7 = map[rune][]string{
	' ': {"00000", "00000", "00000", "00000", "00000", "00000", "00000"},
	'N': {"10001", "11001", "11011", "10101", "10011", "10001", "10001"},
	'O': {"01110", "10001", "10001", "10001", "10001", "10001", "01110"},
	'.': {"00000", "00000", "00000", "00000", "00000", "00110", "00110"},
	'0': {"01110", "10001", "10011", "10101", "11001", "10001", "01110"},
	'1': {"00100", "01100", "00100", "00100", "00100", "00100", "01110"},
	'2': {"01110", "10001", "00001", "00010", "00100", "01000", "11111"},
	'3': {"11110", "00001", "00001", "01110", "00001", "00001", "11110"},
	'4': {"00010", "00110", "01010", "10010", "11111", "00010", "00010"},
	'5': {"11111", "10000", "11110", "00001", "00001", "10001", "01110"},
	'6': {"00110", "01000", "10000", "11110", "10001", "10001", "01110"},
	'7': {"11111", "00001", "00010", "00100", "01000", "01000", "01000"},
	'8': {"01110", "10001", "10001", "01110", "10001", "10001", "01110"},
	'9': {"01110", "10001", "10001", "01111", "00001", "00010", "01100"},
	'A': {"01110", "10001", "10001", "11111", "10001", "10001", "10001"},
	'B': {"11110", "10001", "10001", "11110", "10001", "10001", "11110"},
	'C': {"01110", "10001", "10000", "10000", "10000", "10001", "01110"},
	'D': {"11100", "10010", "10001", "10001", "10001", "10010", "11100"},
	'E': {"11111", "10000", "10000", "11110", "10000", "10000", "11111"},
	'F': {"11111", "10000", "10000", "11110", "10000", "10000", "10000"},
	'G': {"01110", "10001", "10000", "10111", "10001", "10001", "01111"},
	'H': {"10001", "10001", "10001", "11111", "10001", "10001", "10001"},
	'I': {"01110", "00100", "00100", "00100", "00100", "00100", "01110"},
	'J': {"00111", "00010", "00010", "00010", "10001", "10001", "01110"},
	'K': {"10001", "10010", "10100", "11000", "10100", "10010", "10001"},
	'L': {"10000", "10000", "10000", "10000", "10000", "10000", "11111"},
	'M': {"10001", "11011", "11111", "10101", "10001", "10001", "10001"},
	'P': {"11110", "10001", "10001", "11110", "10000", "10000", "10000"},
	'Q': {"01110", "10001", "10001", "10001", "10101", "10010", "01101"},
	'R': {"11110", "10001", "10001", "11110", "10100", "10010", "10001"},
	'S': {"01111", "10000", "10000", "01110", "00001", "00001", "11110"},
	'T': {"11111", "00100", "00100", "00100", "00100", "00100", "00100"},
	'U': {"10001", "10001", "10001", "10001", "10001", "10001", "01110"},
	'V': {"10001", "10001", "10001", "10001", "10001", "01010", "00100"},
	'W': {"10001", "10001", "10001", "10101", "11111", "11011", "10001"},
	'X': {"10001", "10001", "01010", "00100", "01010", "10001", "10001"},
	'Y': {"10001", "10001", "01010", "00100", "00100", "00100", "00100"},
	'Z': {"11111", "00001", "00010", "00100", "01000", "10000", "11111"},
	'、': {"00000", "00000", "00100", "00100", "00100", "01000", "01000"},
	'…': {"00000", "00000", "00000", "00000", "01010", "01010", "01010"},
}

func drawString(img *image.RGBA, s string, x, y int, c color.Color) {
	scale := 4
	cx := x
	for _, ch := range s {
		glyph, ok := font5x7[ch]
		if !ok {
			if ch > 127 { // 中文等未知字符画方块占位
				glyph = []string{"11111", "10001", "10001", "10001", "10001", "10001", "11111"}
			} else {
				glyph = font5x7[' ']
			}
		}
		for row, line := range glyph {
			for col, bit := range line {
				if bit == '1' {
					for dy := 0; dy < scale; dy++ {
						for dx := 0; dx < scale; dx++ {
							img.Set(cx+col*scale+dx, y-7*scale+row*scale+dy, c)
						}
					}
				}
			}
		}
		cx += 6 * scale
	}
}
