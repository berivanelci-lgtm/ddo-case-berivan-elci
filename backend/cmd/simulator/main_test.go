package main

import "testing"

func TestSimulation_SameSeedSameValues(t *testing.T) {
	first, second := newSimulation(7, 3), newSimulation(7, 3)
	for i := 0; i < 250; i++ {
		first.tick++
		second.tick++
		if a, b := first.measurement(), second.measurement(); a != b {
			t.Fatalf("%d. adımda aynı tohum farklı değer üretti: %v != %v", i, a, b)
		}
	}
}

func TestSimulation_DriftsOutOfControl(t *testing.T) {
	sim := newSimulation(1, 1)
	var stable, drifted float64
	for i := 1; i <= 199; i++ {
		sim.tick = i
		value := sim.measurement()
		if i <= 60 {
			stable += value / 60
		}
		if i > 180 {
			drifted += value / 19
		}
	}
	if drifted-stable < 5 {
		t.Fatalf("döngü sonunda değerler belirgin biçimde kaymalı: kararlı %.2f, son %.2f", stable, drifted)
	}
}
