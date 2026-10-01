//go:build !wasm

package favicon

import (
	"bytes"
	"image"
	"image/png"

	// Decoders para los formatos que acepta Source.Raster.
	_ "golang.org/x/image/webp"
	_ "image/jpeg"

	"github.com/disintegration/imaging"
)

// MinEdge es el lado mínimo de la fuente. Cubre todos los tamaños que se
// emiten sin ampliar ninguno: el mayor es el icono de Android, 192.
const MinEdge = 256

// Source es la marca que declara un proyecto.
type Source struct {
	// Raster es el logo cuadrado. Obligatorio. PNG, WebP o JPEG.
	Raster []byte
	// SVG es el mismo logo en vectorial. Opcional: mejora la nitidez en
	// pantallas grandes y no reemplaza al ráster.
	SVG []byte
}

// File es un artefacto listo para escribirse en la salida del sitio.
type File struct {
	Name      string // "icon-32.png"
	Content   []byte
	Mediatype string // "image/png"
	Rel       string // "icon", "apple-touch-icon" — vacío: no se enlaza
	Sizes     string // "32x32" — vacío: sin atributo sizes
	Type      string // valor del atributo type del <link> — vacío: sin atributo
}

// spec entries in the exact order required (32, 192, 180, 512).
type iconSpec struct {
	Name      string
	Size      int
	Mediatype string
	Rel       string
	Sizes     string
	Type      string
}

var pngSpecs = []iconSpec{
	{Name: "icon-32.png", Size: 32, Mediatype: "image/png", Rel: "icon", Sizes: "32x32", Type: "image/png"},
	{Name: "icon-192.png", Size: 192, Mediatype: "image/png", Rel: "icon", Sizes: "192x192", Type: "image/png"},
	{Name: "apple-touch-icon.png", Size: 180, Mediatype: "image/png", Rel: "apple-touch-icon", Sizes: "180x180", Type: ""},
	// Install icon of a PWA manifest (webtyp.com/pwa needs 192 and 512). Not linked from <head>,
	// and only emitted when the source is at least 512: a PWA with a smaller logo fails in
	// pwa.Generate with a message naming the missing size, instead of shipping an upscaled icon.
	{Name: "icon-512.png", Size: 512, Mediatype: "image/png", Rel: "", Sizes: "512x512", Type: ""},
}

// Derive produce el juego completo a partir de la fuente.
func Derive(s Source) ([]File, error) {
	if len(s.Raster) == 0 {
		return nil, ErrNoRaster
	}

	// Decodifica para validar y obtener dimensiones.
	img, _, err := image.Decode(bytes.NewReader(s.Raster))
	if err != nil {
		return nil, ErrUndecodable
	}
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	if w != h {
		return nil, errNotSquare(w, h)
	}
	if w < MinEdge {
		return nil, errTooSmall(w, h)
	}
	srcEdge := w

	var out []File
	var png32 []byte

	for _, spec := range pngSpecs {
		if spec.Size > srcEdge {
			continue
		}
		resized := imaging.Resize(img, spec.Size, spec.Size, imaging.Lanczos)
		buf := &bytes.Buffer{}
		if err := png.Encode(buf, resized); err != nil {
			return nil, err
		}
		data := buf.Bytes()
		// Guardar una copia independiente.
		cp := make([]byte, len(data))
		copy(cp, data)

		if spec.Size == 32 {
			png32 = cp
		}

		out = append(out, File{
			Name:      spec.Name,
			Content:   cp,
			Mediatype: spec.Mediatype,
			Rel:       spec.Rel,
			Sizes:     spec.Sizes,
			Type:      spec.Type,
		})
	}

	// favicon.ico — 22 bytes de cabecera + PNG de 32x32.
	// No lleva Rel/Sizes/Type a propósito.
	if len(png32) > 0 {
		icoData := encodeICO(png32)
		out = append(out, File{
			Name:      "favicon.ico",
			Content:   icoData,
			Mediatype: "image/x-icon",
			Rel:       "",
			Sizes:     "",
			Type:      "",
		})
	}

	// favicon.svg — copia verbatim si existe.
	if len(s.SVG) > 0 {
		cp := make([]byte, len(s.SVG))
		copy(cp, s.SVG)
		out = append(out, File{
			Name:      "favicon.svg",
			Content:   cp,
			Mediatype: "image/svg+xml",
			Rel:       "icon",
			Sizes:     "",
			Type:      "image/svg+xml",
		})
	}

	return out, nil
}
