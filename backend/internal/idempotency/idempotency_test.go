package idempotency

import (
	"errors"
	"testing"
)

func TestHash_SameBodySameHash(t *testing.T) {
	if Hash([]byte(`{"good":1}`)) != Hash([]byte(`{"good":1}`)) {
		t.Fatal("aynı gövde aynı özeti vermeli")
	}
}

func TestHash_DifferentBodyDifferentHash(t *testing.T) {
	if Hash([]byte(`{"good":1}`)) == Hash([]byte(`{"good":2}`)) {
		t.Fatal("farklı gövde farklı özet vermeli")
	}
}

func TestErrors_AreDistinct(t *testing.T) {
	if errors.Is(ErrKeyReused, ErrInProgress) {
		t.Fatal("iki hata birbirinden ayrı olmalı")
	}
}
