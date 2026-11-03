package config

import "strings"

// SpiderDescriptor 是配置里 spider 字段的解析结果。
// TVBox 的写法是 "<地址>;md5;<小写 md5>"，地址可以是相对路径。
type SpiderDescriptor struct {
	Raw  string
	Path string // 去掉 ;md5; 后缀的地址
	MD5  string // 声明的小写 MD5；未声明时为空
}

// ParseSpider 解析 "<path>;md5;<hash>"，未带 MD5 时 Path 为去空白后的原值。
func ParseSpider(raw string) SpiderDescriptor {
	d := SpiderDescriptor{Raw: raw}
	path, md5, ok := strings.Cut(raw, ";md5;")
	if !ok {
		d.Path = strings.TrimSpace(raw)
		return d
	}
	d.Path = strings.TrimSpace(path)
	d.MD5 = strings.ToLower(strings.TrimSpace(md5))
	return d
}

// String 还原成 TVBox 的 spider 写法。
func (d SpiderDescriptor) String() string {
	if d.MD5 == "" {
		return d.Path
	}
	return d.Path + ";md5;" + d.MD5
}
