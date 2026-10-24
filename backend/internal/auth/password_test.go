package auth

import "testing"

func TestHashPassword_DoesNotKeepPlainText(t *testing.T) {
	hash, err := HashPassword("gizli-parola")
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if hash == "gizli-parola" {
		t.Fatal("özet, parolanın kendisi olmamalı")
	}
}

func TestCheckPassword_CorrectPassword(t *testing.T) {
	hash, err := HashPassword("gizli-parola")
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if !CheckPassword(hash, "gizli-parola") {
		t.Fatal("doğru parola kabul edilmeli")
	}
}

func TestCheckPassword_WrongPassword(t *testing.T) {
	hash, err := HashPassword("gizli-parola")
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if CheckPassword(hash, "yanlis-parola") {
		t.Fatal("yanlış parola reddedilmeli")
	}
}

func TestHashPassword_SamePasswordGivesDifferentHashes(t *testing.T) {
	first, err := HashPassword("gizli-parola")
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	second, err := HashPassword("gizli-parola")
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if first == second {
		t.Fatal("rastgele tuz nedeniyle iki özet farklı olmalı")
	}
}
