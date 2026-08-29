package crawler

import "testing"

func TestHelpersMd5Base64(t *testing.T) {
	e := New()
	if err := e.Load(`function go(){ return md5("abc") + "|" + base64Encode("hi") + "|" + base64Decode("aGk=") }`); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("go")
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); got != "900150983cd24fb0d6963f7d28e17f72|aGk=|hi" {
		t.Fatalf("got %q", got)
	}
}

func TestHelpersCookieAndHeader(t *testing.T) {
	e := New()
	if err := e.Load(`function go(){ cookie("sid", "abc"); header("X-Test", "yes"); return cookie("sid") + "|" + header("X-Test") }`); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("go")
	if err != nil || v.String() != "abc|yes" {
		t.Fatalf("got %v, %v", v, err)
	}
}
