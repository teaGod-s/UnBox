package crawler

// Vod 是爬虫返回的影片条目（drpy vod_* 字段对齐）。
type Vod struct {
	VodID      string `json:"vod_id"`
	VodName    string `json:"vod_name"`
	VodPic     string `json:"vod_pic"`
	TypeName   string `json:"type_name"`
	VodRemarks string `json:"vod_remarks"`
}

// Class 是爬虫首页返回的分类。
type Class struct {
	TypeID   string `json:"type_id"`
	TypeName string `json:"type_name"`
}

// Detail 是爬虫返回的详情。
type Detail struct {
	Vod
	VodContent  string `json:"vod_content"`
	VodYear     string `json:"vod_year"`
	VodArea     string `json:"vod_area"`
	VodPlayFrom string `json:"vod_play_from"`
	VodPlayURL  string `json:"vod_play_url"`
}

// Rule 是声明式 drpy rule 的常用字段。
type Rule struct {
	Title                 string            `json:"title"`
	Host                  string            `json:"host"`
	HomeURL               string            `json:"homeUrl"`
	SearchURL             string            `json:"searchUrl"`
	DetailURL             string            `json:"detailUrl"`
	PlayURL               string            `json:"playUrl"`
	URL                   string            `json:"url"`
	ClassParse            string            `json:"class_parse"`
	Lazy                  string            `json:"lazy"`
	Headers               map[string]string `json:"headers"`
	Timeout               int64             `json:"timeout"`
	ClassName             string            `json:"class_name"`
	ClassURL              string            `json:"class_url"`
	VodSelector           string            `json:"vod_selector"`
	NameSelector          string            `json:"name_selector"`
	PicSelector           string            `json:"pic_selector"`
	TypeSelector          string            `json:"type_selector"`
	RemarksSelector       string            `json:"remarks_selector"`
	IDSelector            string            `json:"id_selector"`
	DetailNameSelector    string            `json:"detail_name_selector"`
	DetailContentSelector string            `json:"detail_content_selector"`
	DetailYearSelector    string            `json:"detail_year_selector"`
	DetailAreaSelector    string            `json:"detail_area_selector"`
	PlaySelector          string            `json:"play_selector"`
	PlayNameSelector      string            `json:"play_name_selector"`
	PlayURLSelector       string            `json:"play_url_selector"`
}
