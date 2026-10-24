package user

import "testing"

func TestAllRoles_HasSixRoles(t *testing.T) {
	if got := len(AllRoles()); got != 6 {
		t.Fatalf("6 rol beklenirdi, gelen %d", got)
	}
}

func TestAllRoles_AreAllValid(t *testing.T) {
	for _, role := range AllRoles() {
		if !role.Valid() {
			t.Errorf("%q tanımlı bir rol olarak geçerli olmalı", role)
		}
	}
}

func TestRole_Valid_RejectsUnknownRoles(t *testing.T) {
	for _, role := range []Role{"", "admin", "Operator", "OPERATOR"} {
		if role.Valid() {
			t.Errorf("%q geçersiz sayılmalı", role)
		}
	}
}
