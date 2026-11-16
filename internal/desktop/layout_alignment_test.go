package desktop

import (
	"image/color"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestLayoutNumberControlsCenterContent(t *testing.T) {
	store := openDesktopStore(t)
	if _, err := store.Create("对齐检查", []byte("# 示例文档\n")); err != nil {
		t.Fatal(err)
	}
	_, _, tt := newLibraryHarness(t, store, 960, 640)
	tt.SetScale(2)
	tt.Frame()
	openLayoutProfile(t, tt)

	for _, tc := range []struct {
		role ui.Role
		name string
	}{
		{ui.RoleTextField, "分节间距 (px)"},
		{ui.RoleButton, "减少分节间距 (px)"},
		{ui.RoleButton, "增加分节间距 (px)"},
	} {
		info, ok := tt.AccessByRole(tc.role, tc.name)
		if !ok {
			t.Fatalf("missing %s", tc.name)
		}
		ink, ok := darkContentBounds(tt, info.Bounds, 2)
		if !ok {
			t.Fatalf("%s has no dark content", tc.name)
		}
		boxX := info.Bounds.X + info.Bounds.W/2
		boxY := info.Bounds.Y + info.Bounds.H/2
		inkX := ink.X + ink.W/2
		inkY := ink.Y + ink.H/2
		if math.Abs(float64(boxX-inkX)) > 0.75 || math.Abs(float64(boxY-inkY)) > 0.75 {
			t.Errorf("%s content center = (%.2f, %.2f), control center = (%.2f, %.2f)",
				tc.name, inkX, inkY, boxX, boxY)
		}
	}
}

func darkContentBounds(tt *ui.Tester, bounds ui.Rect, scale float32) (ui.Rect, bool) {
	img := tt.Image()
	x0 := max(0, int((bounds.X+4)*scale))
	y0 := max(0, int((bounds.Y+4)*scale))
	x1 := min(img.Rect.Dx(), int((bounds.X+bounds.W-4)*scale))
	y1 := min(img.Rect.Dy(), int((bounds.Y+bounds.H-4)*scale))
	minX, minY := img.Rect.Dx(), img.Rect.Dy()
	maxX, maxY := -1, -1
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			got := img.RGBAAt(x, y)
			if rgbDistanceSquared(got, color.RGBA{
				R: carbonInk.R,
				G: carbonInk.G,
				B: carbonInk.B,
				A: carbonInk.A,
			}) > 24 {
				continue
			}
			minX = min(minX, x)
			minY = min(minY, y)
			maxX = max(maxX, x)
			maxY = max(maxY, y)
		}
	}
	if maxX < minX || maxY < minY {
		return ui.Rect{}, false
	}
	return ui.Rect{
		X: float32(minX) / scale,
		Y: float32(minY) / scale,
		W: float32(maxX-minX+1) / scale,
		H: float32(maxY-minY+1) / scale,
	}, true
}

func rgbDistanceSquared(a, b color.RGBA) int {
	dr := int(a.R) - int(b.R)
	dg := int(a.G) - int(b.G)
	db := int(a.B) - int(b.B)
	return dr*dr + dg*dg + db*db
}
