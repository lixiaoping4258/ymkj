// Package captcha 生成图形验证码。
//
// 对应原项目 app/api/logic/CaptchaLogic.php + vendor/mickeywaugh/captcha。
//
// ⚠️ 与原文的**有意差异**（属"基础设施必需"，不改变接口契约）：
//
//	PHP 用 5 个 TTF 字体文件（data/fonts/*.ttf）随机会制，返回
//	`data:image/png;base64,...`（CaptchaBuilder::inline）。
//	Go 侧不引入字体文件，改用 golang.org/x/image/font/basicfont 内置点阵字体，
//	**接口契约（{id, image} 与 data URI 格式）完全一致**，只是字形不同。
//
// 字符集与原文逐字一致：去掉容易混淆的 I/J/L/O/0/1。
package captcha

import (
	"bytes"
	"crypto/rand"
	"image"
	"image/color"
	"image/png"
	"math/big"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// Charset 逐字对应 CaptchaLogic::getImageBuilder 里的 $str。
var Charset = []byte("ABCDEFGHKMNPQRSTUVWXYZ23456789")

// Length 对应 `array_rand($str, 4)` —— 固定 4 位。
const Length = 4

// Generate 生成一个验证码明文。
func Generate() string {
	b := make([]byte, Length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(Charset))))
		if err != nil {
			// crypto/rand 失败极罕见；退化为固定值也不该 panic
			b[i] = Charset[0]
			continue
		}
		b[i] = Charset[n.Int64()]
	}
	return string(b)
}

// Draw 把验证码画成 PNG 字节。
//
// 原实现是 $captchaBuilder->build($w, $h, $fontPath)，产出 PNG。
// 这里用内置点阵字体 + 干扰线/噪点，保证机器能读、简单 OCR 不易读。
func Draw(phrase string, w, h int) ([]byte, error) {
	if w < 1 {
		w = 150
	}
	if h < 1 {
		h = 40
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))

	// 背景：浅色（原实现默认也是浅底）
	bg := color.RGBA{R: 245, G: 245, B: 245, A: 255}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, bg)
		}
	}

	// 干扰线
	for i := 0; i < 4; i++ {
		x0, y0 := randInt(w), randInt(h)
		x1, y1 := randInt(w), randInt(h)
		drawLine(img, x0, y0, x1, y1, randColor())
	}

	// 逐字符绘制，带随机纵向抖动
	face := basicfont.Face7x13
	glyphW := 8 // Face7x13 的字宽约 8px
	totalW := glyphW * len(phrase)
	startX := (w - totalW) / 2
	if startX < 2 {
		startX = 2
	}
	for i, ch := range phrase {
		x := startX + i*glyphW
		y := h/2 + 5 + (randInt(7) - 3)
		d := &font.Drawer{
			Dst:  img,
			Src:  image.NewUniform(randDarkColor()),
			Face: face,
			Dot:  fixed.P(x, y),
		}
		d.DrawString(string(ch))
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

/* ------------------------------------------------------------------ 小工具 */

func randInt(n int) int {
	if n <= 0 {
		return 0
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(v.Int64())
}

func randColor() color.RGBA {
	return color.RGBA{R: uint8(120 + randInt(120)), G: uint8(120 + randInt(120)), B: uint8(120 + randInt(120)), A: 255}
}

func randDarkColor() color.RGBA {
	return color.RGBA{R: uint8(randInt(100)), G: uint8(randInt(100)), B: uint8(randInt(140)), A: 255}
}

// drawLine 用 Bresenham 画线（stdlib 没有画线原语）。
func drawLine(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	dx := abs(x1 - x0)
	dy := -abs(y1 - y0)
	sx := 1
	if x0 > x1 {
		sx = -1
	}
	sy := 1
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy
	for {
		img.Set(x0, y0, c)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
