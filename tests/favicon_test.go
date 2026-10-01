//go:build !wasm

package image_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"webtyp.com/image/favicon"
)

func rasterPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 0, 255})
		}
	}
	buf := &bytes.Buffer{}
	if err := png.Encode(buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

func findFile(files []favicon.File, name string) *favicon.File {
	for i := range files {
		if files[i].Name == name {
			return &files[i]
		}
	}
	return nil
}

func TestDeriveEmitsTheFullSet(t *testing.T) {
	raster := rasterPNG(t, 512, 512)
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect width="10" height="10"/></svg>`)

	withSVG, err := favicon.Derive(favicon.Source{Raster: raster, SVG: svg})
	if err != nil {
		t.Fatalf("Derive with SVG: %v", err)
	}
	if len(withSVG) != 6 {
		t.Fatalf("with SVG: expected 6 files, got %d: %v", len(withSVG), names(withSVG))
	}
	expectOrder := []string{"icon-32.png", "icon-192.png", "apple-touch-icon.png", "icon-512.png", "favicon.ico", "favicon.svg"}
	for i, want := range expectOrder {
		if withSVG[i].Name != want {
			t.Errorf("with SVG: file %d name = %q, want %q", i, withSVG[i].Name, want)
		}
	}

	withoutSVG, err := favicon.Derive(favicon.Source{Raster: raster})
	if err != nil {
		t.Fatalf("Derive without SVG: %v", err)
	}
	if len(withoutSVG) != 5 {
		t.Fatalf("without SVG: expected 5 files, got %d: %v", len(withoutSVG), names(withoutSVG))
	}
	expectNoSVG := []string{"icon-32.png", "icon-192.png", "apple-touch-icon.png", "icon-512.png", "favicon.ico"}
	for i, want := range expectNoSVG {
		if withoutSVG[i].Name != want {
			t.Errorf("without SVG: file %d name = %q, want %q", i, withoutSVG[i].Name, want)
		}
	}
}

func names(files []favicon.File) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Name
	}
	return out
}

func TestDeriveSizes(t *testing.T) {
	raster := rasterPNG(t, 512, 512)
	files, err := favicon.Derive(favicon.Source{Raster: raster})
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	cases := map[string]int{
		"icon-32.png":          32,
		"icon-192.png":         192,
		"apple-touch-icon.png": 180,
	}
	for name, wantSize := range cases {
		f := findFile(files, name)
		if f == nil {
			t.Fatalf("missing file %s", name)
		}
		img, _, err := image.Decode(bytes.NewReader(f.Content))
		if err != nil {
			t.Fatalf("decode %s: %v", name, err)
		}
		b := img.Bounds()
		if b.Dx() != wantSize || b.Dy() != wantSize {
			t.Errorf("%s size = %dx%d, want %dx%d", name, b.Dx(), b.Dy(), wantSize, wantSize)
		}
		// Also check File metadata.
		wantSizes := map[string]string{
			"icon-32.png": "32x32", "icon-192.png": "192x192", "apple-touch-icon.png": "180x180",
		}
		if f.Sizes != wantSizes[name] {
			t.Errorf("%s Sizes = %q, want %q", name, f.Sizes, wantSizes[name])
		}
	}
	// Check Rel/Type fields per spec.
	if f := findFile(files, "icon-32.png"); f != nil {
		if f.Rel != "icon" || f.Type != "image/png" || f.Mediatype != "image/png" {
			t.Errorf("icon-32.png meta Rel=%q Type=%q Mediatype=%q", f.Rel, f.Type, f.Mediatype)
		}
	}
	if f := findFile(files, "icon-192.png"); f != nil {
		if f.Rel != "icon" || f.Type != "image/png" {
			t.Errorf("icon-192.png Rel=%q Type=%q", f.Rel, f.Type)
		}
	}
	if f := findFile(files, "apple-touch-icon.png"); f != nil {
		if f.Rel != "apple-touch-icon" || f.Type != "" {
			t.Errorf("apple-touch-icon.png Rel=%q Type=%q", f.Rel, f.Type)
		}
	}
	if f := findFile(files, "favicon.ico"); f != nil {
		if f.Rel != "" || f.Sizes != "" || f.Type != "" {
			t.Errorf("favicon.ico should have empty Rel/Sizes/Type, got Rel=%q Sizes=%q Type=%q", f.Rel, f.Sizes, f.Type)
		}
	}
	if f := findFile(files, "favicon.svg"); f != nil {
		t.Errorf("should not have favicon.svg without SVG source, but got one")
	}
}

func TestDeriveRejectsNonSquare(t *testing.T) {
	raster := rasterPNG(t, 800, 600)
	_, err := favicon.Derive(favicon.Source{Raster: raster})
	if err == nil {
		t.Fatal("expected ErrNotSquare, got nil")
	}
	if !errors.Is(err, favicon.ErrNotSquare) {
		t.Fatalf("expected ErrNotSquare, got %v", err)
	}
	if !strings.Contains(err.Error(), "800x600") {
		t.Errorf("error message should contain 800x600, got %q", err.Error())
	}
}

func TestDeriveRejectsTooSmall(t *testing.T) {
	raster := rasterPNG(t, 128, 128)
	_, err := favicon.Derive(favicon.Source{Raster: raster})
	if err == nil {
		t.Fatal("expected ErrTooSmall, got nil")
	}
	if !errors.Is(err, favicon.ErrTooSmall) {
		t.Fatalf("expected ErrTooSmall, got %v", err)
	}
	if !strings.Contains(err.Error(), "128x128") {
		t.Errorf("error message should contain 128x128, got %q", err.Error())
	}
	// Also must mention MinEdge.
	if !strings.Contains(err.Error(), "256x256") {
		t.Errorf("error message should contain 256x256, got %q", err.Error())
	}
}

func TestDeriveRejectsEmptyAndGarbage(t *testing.T) {
	_, err := favicon.Derive(favicon.Source{Raster: nil})
	if !errors.Is(err, favicon.ErrNoRaster) {
		t.Fatalf("empty raster: expected ErrNoRaster, got %v", err)
	}
	_, err = favicon.Derive(favicon.Source{Raster: []byte{}})
	if !errors.Is(err, favicon.ErrNoRaster) {
		t.Fatalf("empty slice: expected ErrNoRaster, got %v", err)
	}
	_, err = favicon.Derive(favicon.Source{Raster: []byte("not an image")})
	if !errors.Is(err, favicon.ErrUndecodable) {
		t.Fatalf("garbage: expected ErrUndecodable, got %v", err)
	}
	_, err = favicon.Derive(favicon.Source{Raster: []byte{0, 1, 2, 3, 4}})
	if !errors.Is(err, favicon.ErrUndecodable) {
		t.Fatalf("random bytes: expected ErrUndecodable, got %v", err)
	}
}

func TestDeriveNeverUpscales(t *testing.T) {
	raster := rasterPNG(t, 256, 256)
	files, err := favicon.Derive(favicon.Source{Raster: raster})
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	for _, f := range files {
		if strings.HasSuffix(f.Name, ".png") {
			img, _, err := image.Decode(bytes.NewReader(f.Content))
			if err != nil {
				t.Fatalf("decode %s: %v", f.Name, err)
			}
			b := img.Bounds()
			if b.Dx() > 256 || b.Dy() > 256 {
				t.Errorf("%s size %dx%d exceeds source 256", f.Name, b.Dx(), b.Dy())
			}
		}
		if f.Name == "favicon.ico" {
			// ICO wraps PNG 32x32, should be within limit (already checked via PNG).
			if len(f.Content) < 22 {
				t.Errorf("ico too short")
			}
		}
	}
}

func TestDeriveCopiesSVGVerbatim(t *testing.T) {
	raster := rasterPNG(t, 512, 512)
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><g><rect width="10"/></g></svg>`)
	files, err := favicon.Derive(favicon.Source{Raster: raster, SVG: svg})
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	f := findFile(files, "favicon.svg")
	if f == nil {
		t.Fatal("missing favicon.svg")
	}
	if !bytes.Equal(f.Content, svg) {
		t.Errorf("SVG not verbatim: got %q want %q", string(f.Content), string(svg))
	}
	if f.Mediatype != "image/svg+xml" || f.Rel != "icon" || f.Type != "image/svg+xml" {
		t.Errorf("favicon.svg meta Mediatype=%q Rel=%q Type=%q", f.Mediatype, f.Rel, f.Type)
	}
	if f.Sizes != "" {
		t.Errorf("favicon.svg Sizes should be empty, got %q", f.Sizes)
	}
}

func TestIcoWrapsPNG(t *testing.T) {
	raster := rasterPNG(t, 512, 512)
	files, err := favicon.Derive(favicon.Source{Raster: raster})
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	f := findFile(files, "favicon.ico")
	if f == nil {
		t.Fatal("missing favicon.ico")
	}
	data := f.Content
	if len(data) < 22 {
		t.Fatalf("ico too short: %d bytes", len(data))
	}
	// Check header per §3.3
	if got := binary.LittleEndian.Uint16(data[0:2]); got != 0 {
		t.Errorf("ICONDIR reserved = %d, want 0", got)
	}
	if got := binary.LittleEndian.Uint16(data[2:4]); got != 1 {
		t.Errorf("ICONDIR type = %d, want 1", got)
	}
	if got := binary.LittleEndian.Uint16(data[4:6]); got != 1 {
		t.Errorf("ICONDIR count = %d, want 1", got)
	}
	if data[6] != 32 {
		t.Errorf("width byte = %d, want 32", data[6])
	}
	if data[7] != 32 {
		t.Errorf("height byte = %d, want 32", data[7])
	}
	if data[8] != 0 {
		t.Errorf("palette = %d, want 0", data[8])
	}
	if data[9] != 0 {
		t.Errorf("reserved = %d, want 0", data[9])
	}
	if got := binary.LittleEndian.Uint16(data[10:12]); got != 1 {
		t.Errorf("planes = %d, want 1", got)
	}
	if got := binary.LittleEndian.Uint16(data[12:14]); got != 32 {
		t.Errorf("bpp = %d, want 32", got)
	}
	pngLen := binary.LittleEndian.Uint32(data[14:18])
	if int(pngLen) != len(data)-22 {
		t.Errorf("size field = %d, want %d", pngLen, len(data)-22)
	}
	if got := binary.LittleEndian.Uint32(data[18:22]); got != 22 {
		t.Errorf("offset = %d, want 22", got)
	}
	pngData := data[22:]
	if len(pngData) == 0 {
		t.Fatal("no PNG data after header")
	}
	// Must be decodable PNG 32x32
	img, _, err := image.Decode(bytes.NewReader(pngData))
	if err != nil {
		t.Fatalf("PNG inside ICO not decodable: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != 32 || b.Dy() != 32 {
		t.Errorf("ICO PNG size = %dx%d, want 32x32", b.Dx(), b.Dy())
	}
	// Check PNG signature
	if !bytes.HasPrefix(pngData, []byte{0x89, 'P', 'N', 'G'}) {
		t.Error("ICO payload does not start with PNG signature")
	}
}

// The PWA install icon is emitted only from a source of at least 512, never upscaled, and is not
// linked from <head> (Rel empty): webtyp.com/pwa lists it in the manifest.
func TestDeriveIcon512OnlyFromLargeSource(t *testing.T) {
	large, err := favicon.Derive(favicon.Source{Raster: rasterPNG(t, 512, 512)})
	if err != nil {
		t.Fatal(err)
	}
	f := findFile(large, "icon-512.png")
	if f == nil {
		t.Fatalf("512 source: icon-512.png missing: %v", names(large))
	}
	if f.Rel != "" || f.Sizes != "512x512" || f.Mediatype != "image/png" {
		t.Errorf("icon-512.png Rel=%q Sizes=%q Mediatype=%q", f.Rel, f.Sizes, f.Mediatype)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(f.Content))
	if err != nil || cfg.Width != 512 || cfg.Height != 512 {
		t.Errorf("icon-512.png decodes as %dx%d, %v", cfg.Width, cfg.Height, err)
	}

	small, err := favicon.Derive(favicon.Source{Raster: rasterPNG(t, 256, 256)})
	if err != nil {
		t.Fatal(err)
	}
	if findFile(small, "icon-512.png") != nil {
		t.Error("256 source: icon-512.png emitted; it must never be upscaled")
	}
}
