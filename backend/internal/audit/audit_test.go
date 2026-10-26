package audit

import (
	"context"
	"testing"
)

func TestUserFrom_ReturnsStoredUser(t *testing.T) {
	ctx := WithUser(context.Background(), 42)

	id, ok := UserFrom(ctx)

	if !ok || id != 42 {
		t.Fatalf("42 beklenirdi, gelen %d (bulundu: %v)", id, ok)
	}
}

func TestUserFrom_MissingUser(t *testing.T) {
	if _, ok := UserFrom(context.Background()); ok {
		t.Fatal("kullanıcı konmamış context'te kullanıcı bulunmamalı")
	}
}

func TestWithUser_DoesNotChangeParentContext(t *testing.T) {
	parent := context.Background()

	_ = WithUser(parent, 7)

	if _, ok := UserFrom(parent); ok {
		t.Fatal("üst context değişmemeli")
	}
}

func TestWithUser_InnermostValueWins(t *testing.T) {
	ctx := WithUser(WithUser(context.Background(), 1), 2)

	id, ok := UserFrom(ctx)

	if !ok || id != 2 {
		t.Fatalf("2 beklenirdi, gelen %d", id)
	}
}
