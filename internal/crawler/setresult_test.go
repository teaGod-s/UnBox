package crawler

import (
	"context"
	"testing"
	"time"
)

// drpy2 脚本的详情/搜索动作在手动组装 VOD 字段后，常调 setResult(d) 收尾
// （百忙无果二级 `...d.map(...).join("#"); setResult(d)`）。setResult 接收
// 播放项数组 [{title,url,...}]，在 VOD.vod_play_url 为空时用 title$url 兜底生成。
func TestSetResultHelper(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	e := New()
	e.SetContext(ctx)
	if _, err := e.vm.RunScript("setresult.js", `
var VOD = {vod_name:"测试影片", vod_play_url:""};
var d = [
  {title:"第1集", url:"http://a.m3u8"},
  {title:"第2集", url:"http://b.m3u8"}
];
setResult(d);
out = VOD.vod_play_url;
outFrom = VOD.vod_play_from;
`); err != nil {
		t.Fatalf("RunScript: %v", err)
	}
	got := e.vm.Get("out")
	if got == nil || got.String() != "第1集$http://a.m3u8#第2集$http://b.m3u8" {
		t.Fatalf("vod_play_url = %v", got)
	}
	// VOD.vod_play_url 已有值时不覆盖。
	if _, err := e.vm.RunScript("setresult2.js", `
VOD = {vod_name:"测试", vod_play_url:"已存在$http://x.m3u8"};
setResult([{title:"不该覆盖", url:"http://y.m3u8"}]);
out2 = VOD.vod_play_url;
`); err != nil {
		t.Fatalf("RunScript2: %v", err)
	}
	if got := e.vm.Get("out2"); got.String() != "已存在$http://x.m3u8" {
		t.Fatalf("已有值时被覆盖: %v", got)
	}
}
