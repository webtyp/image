//go:build !wasm

package min

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"webtyp.com/image"
	"webtyp.com/modfind"
)

func (h *Handler) moduleDirs() ([]string, error) {
	h.mu.Lock()
	if h.finder == nil {
		h.finder = modfind.New()
	}
	h.mu.Unlock()
	mods, err := h.finder.Discover(h.config.RootDir)
	if err != nil {
		return nil, err
	}
	dirs := make([]string, 0, len(mods))
	for _, m := range mods {
		dirs = append(dirs, m.Dir)
	}
	return dirs, nil
}

// LoadImages discovers modules via go list and processes their images.
func (h *Handler) LoadImages() error {
	if h.config.RootDir == "" {
		return fmt.Errorf("config.RootDir is empty")
	}

	moduleDirs, err := h.moduleDirs()
	if err != nil {
		h.log("warning: failed to list modules:", err)
		return nil
	}
	var allAssets []ParsedAsset
	for _, dir := range moduleDirs {
		assets, err := ExtractImages(dir)
		if err != nil {
			h.log("warning: failed to extract images from module", dir, ":", err)
			continue
		}
		for _, asset := range assets {
			if err := h.processAsset(asset); err != nil {
				h.log("warning: failed to process asset", asset.AbsPath, ":", err)
			}
		}
		allAssets = append(allAssets, assets...)
	}
	h.cleanOrphans(allAssets)

	h.mu.Lock()
	h.assets = append([]ParsedAsset(nil), allAssets...)
	h.mu.Unlock()

	return nil
}

// ReloadModule re-extracts and re-processes images for a single module.
func (h *Handler) ReloadModule(moduleDir string) error {
	assets, err := ExtractImages(moduleDir)
	if err != nil {
		return err
	}
	for _, asset := range assets {
		if err := h.processAsset(asset); err != nil {
			h.log("warning: failed to process asset", asset.AbsPath, ":", err)
		}
	}

	h.mu.Lock()
	cleanModuleDir := filepath.Clean(moduleDir)
	var filtered []ParsedAsset
	for _, a := range h.assets {
		if !strings.HasPrefix(filepath.Clean(a.AbsPath), cleanModuleDir+string(filepath.Separator)) && filepath.Clean(a.AbsPath) != cleanModuleDir {
			filtered = append(filtered, a)
		}
	}
	h.assets = append(filtered, assets...)
	h.mu.Unlock()

	return nil
}

func (h *Handler) processAsset(asset ParsedAsset) error {
	if IsUpToDate(asset.AbsPath, asset.Variants, h.config.OutputDir) {
		return nil
	}
	if err := os.MkdirAll(h.config.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}
	_, err := ProcessImage(asset, h.config.OutputDir, h.config.Quality, h.log)
	return err
}

// outputNames son los archivos que un asset produce en la salida. Es la única
// derivación de nombres del paquete: si "¿qué archivos vigentes tiene este
// asset?" y "¿qué archivos hay que servir?" respondieran por separado, un
// archivo podría estar vigente para la limpieza e invisible para el servidor.
func outputNames(asset ParsedAsset) []string {
	if IsVector(asset.AbsPath) {
		return []string{VectorOutputName(asset.AbsPath)}
	}

	ext := ExpectedExt(asset.AbsPath)
	if ext == "" {
		return nil
	}

	variantsList := []image.Variant{
		image.VariantS,
		image.VariantM,
		image.VariantL,
	}
	var out []string
	for _, v := range variantsList {
		if asset.Variants&v != 0 {
			out = append(out, fmt.Sprintf("%s.%s%s", asset.BaseName, v.Suffix(), ext))
		}
	}
	return out
}

func (h *Handler) cleanOrphans(allAssets []ParsedAsset) {
	if h.config.OutputDir == "" {
		return
	}
	activeFiles := make(map[string]bool)
	for _, asset := range allAssets {
		if !IsVector(asset.AbsPath) && ExpectedExt(asset.AbsPath) == "" {
			variantsList := []image.Variant{
				image.VariantS,
				image.VariantM,
				image.VariantL,
			}
			extensions := []string{".jpg", ".webp"}
			for _, v := range variantsList {
				if asset.Variants&v != 0 {
					for _, ext := range extensions {
						activeFiles[fmt.Sprintf("%s.%s%s", asset.BaseName, v.Suffix(), ext)] = true
					}
				}
			}
			continue
		}
		for _, name := range outputNames(asset) {
			activeFiles[name] = true
		}
	}
	files, err := os.ReadDir(h.config.OutputDir)
	if err != nil {
		return
	}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		name := f.Name()
		// El barrido cubre también .svg: un vector solo llega a la salida si
		// alguien lo declaró, así que uno sin declarar es un resto de una
		// declaración borrada, no un archivo que alguien dejó a mano. Depender
		// de archivos colocados a mano en la salida es justo lo que hacía que
		// desarrollo y release sirvieran sitios distintos.
		if strings.HasSuffix(name, ".webp") || strings.HasSuffix(name, ".jpg") || strings.HasSuffix(name, ".jpeg") || strings.HasSuffix(name, vectorExt) {
			if !activeFiles[name] {
				os.Remove(filepath.Join(h.config.OutputDir, name))
			}
		}
	}
}
