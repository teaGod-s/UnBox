// 脱敏 dr_py 声明式爬虫 fixture，域名固定为 example.com。
var rule = {
  title: '示例站',
  host: 'https://example.com',
  url: '/list/fyclass-fypage.html',
  searchUrl: '/s?wd=**',
  class_parse: '.nav&&li;a&&Text;a&&href;/(\\w+).html',
  lazy: 'js:input=input',
  headers: { 'User-Agent': 'UA' },
  timeout: 30,
};
