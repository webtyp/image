//go:build !wasm

package min

import (
	"sync"

	"webtyp.com/image"
	"webtyp.com/modfind"
)

type ParsedAsset struct {
	AbsPath  string        // absolute path: moduleDir + "/" + asset.Path
	Variants image.Variant // resolved bitmask value
	Alt      string
	BaseName string // base name without extension: "logo", "hero"
}

type Handler struct {
	mu     sync.Mutex
	config *Config
	log    func(messages ...any)
	finder modfind.Discoverer
	assets []ParsedAsset
}

type Config struct {
	RootDir   string
	OutputDir string
	Quality   int
}

func New(c *Config) *Handler {
	return &Handler{
		config: c,
		log:    func(messages ...any) {},
	}
}

func (h *Handler) SetLog(fn func(messages ...any)) {
	h.log = fn
}

func (h *Handler) SetFinder(f modfind.Discoverer) {
	h.finder = f
}

func (h *Handler) Name() string           { return "IMAGE" }
func (h *Handler) Logger(messages ...any) { h.log(messages...) }
func (h *Handler) UnobservedFiles() []string {
	return []string{h.config.OutputDir}
}
