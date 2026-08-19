package store

import (
	"path/filepath"
	"testing"
)

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

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s
}
