package auth

import (
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestHashPassword(t *testing.T) {
	if _, err := HashPassword("short"); err != ErrWeakPassword {
		t.Errorf("короткий пароль: %v", err)
	}
	h, err := HashPassword("достаточно-длинный")
	if err != nil || !strings.HasPrefix(h, "$2") || bcrypt.CompareHashAndPassword([]byte(h), []byte("достаточно-длинный")) != nil {
		t.Errorf("hash=%q err=%v", h, err)
	}
}

func TestGeneratePassword(t *testing.T) {
	a, b := GeneratePassword(), GeneratePassword()
	if len(a) != 16 || a == b || strings.ContainsAny(a, "0O1lI") {
		t.Errorf("a=%q b=%q", a, b)
	}
}

func TestLimiter(t *testing.T) {
	l := NewLimiter(3, 50*time.Millisecond)
	for i := 0; i < 3; i++ {
		if l.Blocked("x") {
			t.Fatalf("заблокирован после %d попыток", i)
		}
		l.Fail("x")
	}
	if !l.Blocked("x") || l.Blocked("y") {
		t.Fatal("ждали блокировку только для x")
	}
	time.Sleep(60 * time.Millisecond)
	if l.Blocked("x") {
		t.Fatal("окно прошло — блокировки быть не должно")
	}
	l.Fail("x")
	l.Reset("x")
	if len(l.fails) != 0 {
		t.Fatal("Reset не очистил попытки")
	}
}

func TestValidRole(t *testing.T) {
	if !ValidRole("admin") || !ValidRole("hr") || ValidRole("root") {
		t.Error("ValidRole")
	}
}

func TestNormalizeModules(t *testing.T) {
	all, err := NormalizeModules(nil)
	if err != nil || len(all) != len(DefaultModules) || len(all) != len(Modules)-1 {
		t.Fatalf("nil → модули по умолчанию: %v %v", all, err)
	}
	for _, m := range all {
		if m == ModTV {
			t.Fatal("ТВ не выдаётся по умолчанию")
		}
	}
	got, err := NormalizeModules([]string{ModEmployees, ModSprints, ModSprints})
	if err != nil || len(got) != 2 || got[0] != ModSprints || got[1] != ModEmployees {
		t.Fatalf("порядок и дубли: %v %v", got, err)
	}
	if none, err := NormalizeModules([]string{}); err != nil || len(none) != 0 {
		t.Fatalf("пустой список допустим: %v %v", none, err)
	}
	if _, err := NormalizeModules([]string{"nope"}); err == nil {
		t.Fatal("неизвестный модуль должен давать ошибку")
	}
}

func TestHasModule(t *testing.T) {
	if !(User{Role: RoleAdmin}).HasModule(ModAttendance) {
		t.Fatal("админ видит все модули")
	}
	u := User{Role: RoleLead, Modules: []string{ModSprints}}
	if !u.HasModule(ModSprints) || u.HasModule(ModLeadControl) {
		t.Fatal("доступ только к выданным модулям")
	}
}
