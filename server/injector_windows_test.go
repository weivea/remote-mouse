//go:build windows

package main

import "testing"

func TestNormAbs(t *testing.T) {
	cases := []struct {
		name           string
		x, y           int32
		ox, oy, cw, ch int32
		wantX, wantY   int32
	}{
		{"origin", 0, 0, 0, 0, 1920, 1080, 0, 0},
		{"bottom-right", 1919, 1079, 0, 0, 1920, 1080, 65535, 65535},
		{"clamp-negative", -50, -50, 0, 0, 1920, 1080, 0, 0},
		{"clamp-over", 5000, 5000, 0, 0, 1920, 1080, 65535, 65535},
		{"offset-origin-min", -1920, -1080, -1920, -1080, 1920, 1080, 0, 0},
		{"offset-origin-max", -1, -1, -1920, -1080, 1920, 1080, 65535, 65535},
		{"single-pixel", 0, 0, 0, 0, 1, 1, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotX, gotY := normAbs(c.x, c.y, c.ox, c.oy, c.cw, c.ch)
			if gotX != c.wantX || gotY != c.wantY {
				t.Fatalf("normAbs(%d,%d, ox=%d oy=%d cw=%d ch=%d) = (%d,%d), want (%d,%d)",
					c.x, c.y, c.ox, c.oy, c.cw, c.ch, gotX, gotY, c.wantX, c.wantY)
			}
			if gotX < 0 || gotX > 65535 || gotY < 0 || gotY > 65535 {
				t.Fatalf("normAbs out of 0..65535 range: (%d,%d)", gotX, gotY)
			}
		})
	}
}

func TestNormAbsMonotonic(t *testing.T) {
	var prev int32 = -1
	for x := int32(0); x < 1920; x += 64 {
		nx, _ := normAbs(x, 0, 0, 0, 1920, 1080)
		if nx < prev {
			t.Fatalf("normAbs not monotonic at x=%d: %d < %d", x, nx, prev)
		}
		prev = nx
	}
}

func TestClampI32(t *testing.T) {
	cases := []struct{ v, lo, hi, want int32 }{
		{5, 0, 10, 5},
		{-5, 0, 10, 0},
		{15, 0, 10, 10},
		{0, 0, 10, 0},
		{10, 0, 10, 10},
	}
	for _, c := range cases {
		if got := clampI32(c.v, c.lo, c.hi); got != c.want {
			t.Fatalf("clampI32(%d,%d,%d) = %d, want %d", c.v, c.lo, c.hi, got, c.want)
		}
	}
}
