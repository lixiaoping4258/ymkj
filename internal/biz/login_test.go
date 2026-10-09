package biz

import "testing"

// joinFileURL 是 FileService::format 的移植。
//
// 期望值来源分两类，下面逐条标注：
//
//	[PHP实测] = PHP CLI 真机跑 FileService::format() 得到
//	[源码推导] = 对着 substr_replace 的语义推导（真机没覆盖到该输入，故标注区分）
func TestJoinFileURL(t *testing.T) {
	cases := []struct{ name, domain, uri, want string }{
		// [PHP实测]
		{"无尾斜杠+无首斜杠", "https://cdn.example.com", "uploads/a.png", "https://cdn.example.com/uploads/a.png"},
		{"有尾斜杠+有首斜杠", "https://cdn.example.com/", "/uploads/a.png", "https://cdn.example.com/uploads/a.png"},
		{"有尾斜杠+无首斜杠", "https://cdn.example.com/", "uploads/a.png", "https://cdn.example.com/uploads/a.png"},
		{"无尾斜杠+有首斜杠", "https://cdn.example.com", "/uploads/a.png", "https://cdn.example.com/uploads/a.png"},
		{"uri 为空", "http://x.cn", "", "http://x.cn/"},
		{"domain 为空", "", "uploads/a.png", "/uploads/a.png"},
		{"两端带空白", "  https://cdn.example.com  ", "  uploads/a.png  ", "https://cdn.example.com/uploads/a.png"},

		// [源码推导] —— 验证"只去一个斜杠"这条。
		// PHP: substr_replace($domain,'',$len-1,1) 只删最后 1 个字符，
		// 所以 "https://x.cn//" 会剩一个 '/'；用 TrimRight 会全删掉，结果不同。
		{"双尾斜杠只去一个", "https://x.cn//", "u.png", "https://x.cn//u.png"},
		{"双首斜杠只去一个", "https://x.cn", "//u.png", "https://x.cn//u.png"},
	}
	for _, c := range cases {
		got := joinFileURL(c.domain, c.uri)
		if got != c.want {
			t.Errorf("%s: joinFileURL(%q, %q) = %q, 期望 %q", c.name, c.domain, c.uri, got, c.want)
		}
	}
}

// getFileUrl 的前两条短路：**已是完整 URL 的原样返回**。
//
// 缺了这两条会给外部头像 URL 拼上本项目的域名前缀 —— 那是静默错误，
// 前端只会看到一张裂图。
// [PHP实测]
func TestJoinFileURL_AlreadyAbsoluteURLShortCircuit(t *testing.T) {
	for _, u := range []string{"http://a.cn/x.png", "https://b.cn/y.png"} {
		if got := joinFileURL("https://our.domain", u); got != u {
			t.Errorf("完整 URL 应原样返回：输入 %q，得到 %q", u, got)
		}
	}
	// 区分力：如果实现里没有短路，上面两条会失败。
	// 这里再确认相对路径**会**被拼前缀，证明短路不是"什么都不拼"。
	if got := joinFileURL("https://our.domain", "uploads/a.png"); got != "https://our.domain/uploads/a.png" {
		t.Errorf("相对路径应拼上前缀，实际 %q", got)
	}
}
