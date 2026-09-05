package store

import (
	"path/filepath"
	"testing"
)

func TestVodSearchHistoryCRUD(t *testing.T) {
	s := openTemp(t)
	defer s.Close()
	if err := s.RecordVodSearch("  斗罗  "); err != nil {
		t.Fatalf("RecordVodSearch: %v", err)
	}
	if err := s.RecordVodSearch("庆余年"); err != nil {
		t.Fatalf("RecordVodSearch: %v", err)
	}
	if err := s.RecordVodSearch("斗罗"); err != nil {
		t.Fatalf("RecordVodSearch duplicate: %v", err)
	}
	history, err := s.ListVodSearchHistory(10)
	if err != nil || len(history) != 2 || history[0] != "斗罗" {
		t.Fatalf("ListVodSearchHistory = %#v, %v", history, err)
	}
	if err := s.DeleteVodSearch("斗罗"); err != nil {
		t.Fatalf("DeleteVodSearch: %v", err)
	}
	history, err = s.ListVodSearchHistory(10)
	if err != nil || len(history) != 1 || history[0] != "庆余年" {
		t.Fatalf("history after delete = %#v, %v", history, err)
	}
}

func TestVodFavoriteCRUD(t *testing.T) {
	s := openTemp(t)
	defer s.Close()
	if err := s.AddVodFavorite("site-a", "vod-1", "影片一", "logo", "电影"); err != nil {
		t.Fatalf("AddVodFavorite: %v", err)
	}
	if err := s.AddVodFavorite("site-a", "vod-1", "影片一（更新）", "logo2", "电视剧"); err != nil {
		t.Fatalf("AddVodFavorite duplicate: %v", err)
	}
	ok, err := s.IsVodFavorite("site-a", "vod-1")
	if err != nil || !ok {
		t.Fatalf("IsVodFavorite = %v, %v", ok, err)
	}
	favs, err := s.ListVodFavorites()
	if err != nil || len(favs) != 1 || favs[0].Title != "影片一（更新）" {
		t.Fatalf("ListVodFavorites = %#v, %v", favs, err)
	}
	if err := s.RemoveVodFavorite("site-a", "vod-1"); err != nil {
		t.Fatalf("RemoveVodFavorite: %v", err)
	}
	if ok, _ := s.IsVodFavorite("site-a", "vod-1"); ok {
		t.Fatal("favorite still exists after removal")
	}
}

func TestDeleteVodHistory(t *testing.T) {
	s := openTemp(t)
	defer s.Close()
	if err := s.UpsertVodHistory(VodHistory{Site: "site-a", VodID: "vod-1", VodTitle: "影片一"}); err != nil {
		t.Fatalf("UpsertVodHistory: %v", err)
	}
	if err := s.DeleteVodHistory("site-a", "vod-1"); err != nil {
		t.Fatalf("DeleteVodHistory: %v", err)
	}
	history, err := s.ListVodHistory(10)
	if err != nil || len(history) != 0 {
		t.Fatalf("history after delete = %#v, %v", history, err)
	}
}

func TestFavoriteCRUD(t *testing.T) {
	s := openTemp(t)
	defer s.Close()
	if err := s.AddFavorite("央视/CCTV-1", "CCTV-1", "央视", "http://x/1"); err != nil {
		t.Fatalf("AddFavorite: %v", err)
	}
	ok, err := s.IsFavorite("央视/CCTV-1")
	if err != nil || !ok {
		t.Fatalf("IsFavorite = %v, %v", ok, err)
	}
	favs, err := s.ListFavorites()
	if err != nil || len(favs) != 1 || favs[0].Name != "CCTV-1" {
		t.Fatalf("ListFavorites = %+v, %v", favs, err)
	}
	if err := s.RemoveFavorite("央视/CCTV-1"); err != nil {
		t.Fatalf("RemoveFavorite: %v", err)
	}
	if favs, _ = s.ListFavorites(); len(favs) != 0 {
		t.Fatalf("删除后仍有 %d 条", len(favs))
	}
}

func TestRecentKeepsLatestFirst(t *testing.T) {
	s := openTemp(t)
	defer s.Close()
	s.AddRecent("a", "a", "组", "http://a")
	s.AddRecent("b", "b", "组", "http://b")
	rs, err := s.ListRecent(1)
	if err != nil || len(rs) != 1 || rs[0].Name != "b" {
		t.Fatalf("ListRecent(1) = %+v, %v", rs, err)
	}
}

func TestGroupCRUD(t *testing.T) {
	s := openTemp(t)
	defer s.Close()
	s.AddGroup("我的分组")
	s.AddGroup("我的分组") // 幂等
	gs, err := s.ListGroups()
	if err != nil || len(gs) != 1 {
		t.Fatalf("ListGroups = %+v, %v", gs, err)
	}
}

func TestLibraryDirs(t *testing.T) {
	s := openTemp(t)
	defer s.Close()

	if err := s.AddLibraryDir("/mnt/movies"); err != nil {
		t.Fatal(err)
	}
	dirs, err := s.ListLibraryDirs()
	if err != nil || len(dirs) != 1 || dirs[0].Path != "/mnt/movies" {
		t.Fatalf("dirs=%+v err=%v", dirs, err)
	}
	if err := s.RemoveLibraryDir("/mnt/movies"); err != nil {
		t.Fatal(err)
	}
	dirs, _ = s.ListLibraryDirs()
	if len(dirs) != 0 {
		t.Fatalf("删除后应空，got %+v", dirs)
	}
}

func TestLibraryItemsReplacePerDir(t *testing.T) {
	s := openTemp(t)
	defer s.Close()

	a := []LibraryItem{{Path: "/a/1.mp4", Name: "1", Dir: "/a", Ext: "mp4", Size: 10}}
	b := []LibraryItem{{Path: "/b/2.mkv", Name: "2", Dir: "/b", Ext: "mkv", Size: 20}}
	if err := s.ReplaceLibraryItems("/a", a); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceLibraryItems("/b", b); err != nil {
		t.Fatal(err)
	}
	a2 := []LibraryItem{{Path: "/a/1b.mkv", Name: "1b", Dir: "/a", Ext: "mkv", Size: 11}}
	if err := s.ReplaceLibraryItems("/a", a2); err != nil {
		t.Fatal(err)
	}
	items, err := s.ListLibraryItems()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items=%+v", items)
	}
	for _, it := range items {
		if it.Path == "/a/1.mp4" {
			t.Fatalf("已删文件仍在: %+v", items)
		}
	}
}

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s
}
