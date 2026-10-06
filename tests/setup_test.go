//go:build !wasm

package image_test

import (
	"fmt"
	stdimage "image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"webtyp.com/image"
	"webtyp.com/image/min"
	"webtyp.com/modfind"
)

type TestEnv struct {
	t         *testing.T
	ModuleDir string
	OutputDir string
	Handler   *min.Handler
}

func newTestEnv(t *testing.T) *TestEnv {
	moduleDir := t.TempDir()
	outputDir := t.TempDir()

	config := &min.Config{
		RootDir:   moduleDir,
		OutputDir: outputDir,
		Quality:   82,
	}

	handler := min.New(config)

	handler.SetFinder(fakeModules([]modfind.Module{
		{Dir: moduleDir, Path: "webtyp.com/image/testmodule"},
	}))

	return &TestEnv{
		t:         t,
		ModuleDir: moduleDir,
		OutputDir: outputDir,
		Handler:   handler,
	}
}

func (e *TestEnv) writeImageGo(content string) {
	err := os.WriteFile(filepath.Join(e.ModuleDir, "image.go"), []byte(content), 0644)
	if err != nil {
		e.t.Fatalf("failed to write image.go: %v", err)
	}
}

func (e *TestEnv) writeImageGoWithImages(assets []image.Asset) {
	content := "//go:build !wasm\n\npackage module\n\nimport \"webtyp.com/image\"\n\nfunc RenderImages() []image.Asset {\n\treturn []image.Asset{\n"
	for _, asset := range assets {
		content += fmt.Sprintf("\t\t{Path: %q, Variants: image.Variant(%d), Alt: %q},\n", asset.Path, asset.Variants, asset.Alt)
	}
	content += "\t}\n}\n"
	e.writeImageGo(content)
}

func (e *TestEnv) copyTestImage(destRelPath, testdataFile string) {
	srcPath := filepath.Join("testdata", testdataFile)
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		// Try to find it if we are in project root
		srcPath = filepath.Join("tests", "testdata", testdataFile)
	}

	data, err := os.ReadFile(srcPath)
	if err != nil {
		e.t.Fatalf("failed to read test image %s: %v", testdataFile, err)
	}

	destPath := filepath.Join(e.ModuleDir, destRelPath)
	err = os.MkdirAll(filepath.Dir(destPath), 0755)
	if err != nil {
		e.t.Fatalf("failed to create dest dir: %v", err)
	}

	err = os.WriteFile(destPath, data, 0644)
	if err != nil {
		e.t.Fatalf("failed to write dest image: %v", err)
	}
}

// createTinyImage creates a 4×4 synthetic opaque PNG for loader behaviour tests.
func (e *TestEnv) createTinyImage(destRelPath string) {
	destPath := filepath.Join(e.ModuleDir, destRelPath)
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		e.t.Fatalf("failed to create dir for %s: %v", destRelPath, err)
	}
	if err := createTestPNG(destPath, 4, 4, false); err != nil {
		e.t.Fatalf("failed to create tiny image %s: %v", destRelPath, err)
	}
}

func (e *TestEnv) assertJPGExists(name string, v image.Variant) {
	path := filepath.Join(e.OutputDir, fmt.Sprintf("%s.%s.jpg", name, v.Suffix()))
	if _, err := os.Stat(path); os.IsNotExist(err) {
		e.t.Errorf("expected JPG variant %s for %s to exist", v.Suffix(), name)
	}
}

func (e *TestEnv) assertJPGNotExists(name string, v image.Variant) {
	path := filepath.Join(e.OutputDir, fmt.Sprintf("%s.%s.jpg", name, v.Suffix()))
	if _, err := os.Stat(path); err == nil {
		e.t.Errorf("expected JPG variant %s for %s NOT to exist", v.Suffix(), name)
	}
}

func (e *TestEnv) assertWebPExists(name string, v image.Variant) {
	path := filepath.Join(e.OutputDir, fmt.Sprintf("%s.%s.webp", name, v.Suffix()))
	if _, err := os.Stat(path); os.IsNotExist(err) {
		e.t.Errorf("expected WebP variant %s for %s to exist", v.Suffix(), name)
	}
}

func (e *TestEnv) assertWebPNotExists(name string, v image.Variant) {
	path := filepath.Join(e.OutputDir, fmt.Sprintf("%s.%s.webp", name, v.Suffix()))
	if _, err := os.Stat(path); err == nil {
		e.t.Errorf("expected WebP variant %s for %s NOT to exist", v.Suffix(), name)
	}
}

func createTestImage(path string, width, height int) error {
	img := stdimage.NewRGBA(stdimage.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 0, 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return jpeg.Encode(f, img, nil)
}

func createTestPNG(path string, width, height int, alpha bool) error {
	img := stdimage.NewRGBA(stdimage.Rect(0, 0, width, height))
	a := uint8(255)
	if alpha {
		a = 128
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 0, a})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// fakeModules is the test double for modfind.Discoverer: it returns fixed modules.
type fakeModules []modfind.Module

func (f fakeModules) Discover(string) ([]modfind.Module, error) { return f, nil }
