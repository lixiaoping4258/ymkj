package biz

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"github.com/lixiaoping4258/ymkj/internal/pkg/captcha"
)

// CaptchaUsecase 对应原项目 app/api/logic/CaptchaLogic.php。
//
// 原文（CaptchaLogic::getCaptcha）：
//
//	public static function getCaptcha(int $w = 150, int $h = 40): array {
//	    $id = Id::uuid();
//	    $cacheKey = sprintf('captcha:%s', $id);
//	    $captchaBuilder = self::getImageBuilder($w, $h);
//	    $phrase = $captchaBuilder->getPhrase();
//	    cache($cacheKey, $phrase, 300);
//	    $image = $captchaBuilder->inline();
//	    return ['id' => $id, 'image' => $image];
//	}
type CaptchaUsecase struct {
	repo CaptchaRepo
}

func NewCaptchaUsecase(repo CaptchaRepo) *CaptchaUsecase {
	return &CaptchaUsecase{repo: repo}
}

// CaptchaResult 对应 getCaptcha 的返回值 ['id'=>..., 'image'=>...]。
type CaptchaResult struct {
	ID string
	// Image 是 `data:image/png;base64,...` 形式（对应 CaptchaBuilder::inline）
	Image string
}

// CaptchaWidthDefault / CaptchaHeightDefault 对应 `$this->request->get('w/d', 150)` 等的默认值。
const (
	CaptchaWidthDefault  = 150
	CaptchaHeightDefault = 40
	// CaptchaRangeMax 对应 `if($w < 1 || $w > 300) { $w = 150; }`
	CaptchaRangeMax = 300
)

// GetCaptcha 对应 CaptchaController::index + CaptchaLogic::getCaptcha。
//
// 原文的宽高处理（注意是"越界就回到默认值"，不是"钳制到边界"）：
//
//	$w = $this->request->get('w/d', 150);
//	if($w < 1 || $w > 300) { $w = 150; }        // ← 越界回默认，不是 clamp
//	$h = $this->request->get('h/d', 40);
//	if($h < 1 || $h > 300) { $h = 40; }
func (uc *CaptchaUsecase) GetCaptcha(ctx context.Context, w, h int) (*CaptchaResult, error) {
	// PHP 的 'w/d' 把非数字转成 0，而 0 < 1 会触发回到默认值 —— 行为一致。
	if w < 1 || w > CaptchaRangeMax {
		w = CaptchaWidthDefault
	}
	if h < 1 || h > CaptchaRangeMax {
		h = CaptchaHeightDefault
	}

	id := newUUIDLikeID()
	phrase := captcha.Generate()

	if err := uc.repo.Set(ctx, id, phrase); err != nil {
		return nil, err
	}
	png, err := captcha.Draw(phrase, w, h)
	if err != nil {
		return nil, err
	}
	return &CaptchaResult{
		ID:    id,
		Image: "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
	}, nil
}

// newUUIDLikeID 对应 Id::uuid()：
//
//	$chars = md5(uniqid(mt_rand(), true));
//	$uuid = substr($chars,0,8).'-'.substr($chars,8,4).'-'.substr($chars,12,4)
//	      .'-'.substr($chars,16,4).'-'.substr($chars,20,12);
//
// 值本身是随机的（不可复现），Go 侧只要**形态一致**（8-4-4-4-12 的十六进制）
// 即可 —— 前端只把它当不透明标识回传。
func newUUIDLikeID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// 极罕见；退化成一个固定形态的串，避免 panic
		return "00000000-0000-0000-0000-000000000000"
	}
	s := hex.EncodeToString(b[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", s[0:8], s[8:12], s[12:16], s[16:20], s[20:32])
}
