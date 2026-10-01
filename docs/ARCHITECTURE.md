# Imagemin Architecture

`image` is a specialized image processing library for Go SSR (Server-Side Rendering) modules within the `webtyp` ecosystem. Its primary goal is to automate the generation of responsive WebP images from declarations in module source code and to compress client-side uploaded images before sending them over the network.

## Package Structure

The package is split into four parts:

| Layer | Package | Build | When Executed |
|---|---|---|---|
| **Builders** | `webtyp.com/image` | Tagless | Render, both server & client |
| **Pipeline** | `webtyp.com/image/min` | `//go:build !wasm` | Build time, on server |
| **Browser** | `webtyp.com/image/browser` | `//go:build wasm` | Client-side pre-upload |
| **Favicon** | `webtyp.com/image/favicon` | `//go:build !wasm` | Build time, icon derivation |

`favicon` valida y deriva, pero no sanea: un SVG de un tercero se limpia en `webtyp/svg`, no en el redimensionador.

- **`types.go`**: Contains core types like `Asset` and `Variant`, as well as suffix conventions (`Variant.Suffix()`) and pixel widths (`Variant.Width()`).
- **`builders.go`**: HTML builders like `Img()`, `Responsive()`, `Picture()`, and `Source()`.
- **`browser/compress.go`**: Client-side `Compress` and `CompressToFit` utilizing browser `OffscreenCanvas`.
- **`browser/js.go`**: Constant definitions for JS global/property names.
- **`browser/errors.go`**: Sentinel errors like `ErrUnsupported`.
- **`favicon/favicon.go`**: `Derive` — de un logo cuadrado al juego de iconos (`icon-32.png`, `icon-192.png`, `apple-touch-icon.png`, `icon-512.png` (solo con un logo de 512 o más; no se enlaza en `<head>`, es el ícono de instalación de la PWA), `favicon.ico`, `favicon.svg`).
- **`favicon/ico.go`**: Codificador ICO mínimo (22 bytes de cabecera + PNG de 32×32 embebido).
- **`favicon/errors.go`**: `ErrNoRaster`, `ErrUndecodable`, `ErrNotSquare`, `ErrTooSmall` (valida y deriva, no sanea SVG).
- **`min/extract.go`**: Uses the `go/ast` and `go/parser` packages to analyze Go source code. It extracts the `RenderImages` function from `image.go` files.
- **`min/convert.go`**: Handles image transformation using `imaging` and `nativewebp`.
- **`min/loader.go`**: Orchestrates discovery via `go list`.
- **`min/cache.go`**: Implements mtime-based change detection.
- **`min/artifacts.go`**: Exposes processed images as in-memory `Artifact` values (`Handler.Artifacts()`) and resolves the default cache location (`DefaultCacheDir`).

## Core Concepts

### Browser Image Compression (`OffscreenCanvas`)
The client layer (`webtyp.com/image/browser`) decompresses, resizes, and re-encodes user-selected files using the browser's native capabilities before uploading.

Using `OffscreenCanvas` and `convertToBlob()` guarantees all asynchronous operations return `Promise` objects, seamlessly integrating with `webtyp.com/await`. This avoids DOM manipulation or callback-based APIs (`canvas.toBlob`), keeping client-side operations fast and decoupled from document elements.

### Responsive Images (`<img srcset>`) vs `<picture>`

Para **resolution switching** (servir la misma imagen en distintas resoluciones), el elemento HTML adecuado es `<img srcset sizes>`. Esto permite al navegador seleccionar la variante óptima considerando el ancho de pantalla, la densidad de píxeles (DPR) y las condiciones de red. `<picture>` se reserva para *art direction* (recortes o composiciones diferentes por tamaño de pantalla) o formatos alternativos.

La convención de sufijos (`.S`, `.M`, `.L`) y anchos (`WidthS`, `WidthM`, `WidthL`) reside en `types.go` en el paquete raíz sin build tags. Esta única definición compartida permite que tanto el pipeline de optimización en backend (`min/`) como los renderizadores de HTML en frontend (WASM) compartan la misma convención sin duplicar cadenas ni depender de código específico de backend. `min/convert.go` **consume** `Variant.Width()`: no repite los anchos, porque un ancho que el pipeline escribe y el `srcset` declara distinto es una imagen que el navegador elige mal.

### La escalera de anchos: 480 / 1024 / 1600

El navegador no elige por el tamaño de la pantalla: calcula `ancho_de_layout_css × DPR` y toma el candidato más chico que lo cubra. Esa fórmula gobierna toda la escalera.

| dispositivo | CSS px | DPR | pide | elige a `100vw` |
|---|---|---|---|---|
| Galaxy S23 | 360 | 3 | 1080 | L |
| iPhone 15 | 393 | 3 | 1179 | L |
| iPhone SE | 375 | 2 | 750 | M |
| laptop retina | 1440 | 2 | 2880 | L |

De ahí salen las dos decisiones:

**La variante S nunca se elige a `100vw`.** Ningún teléfono moderno pide menos de 720 px reales. S sólo entra cuando `sizes` declara un slot chico — una tarjeta en grilla de 3 columnas son ~131 px CSS, que a DPR 3 piden 393. Por eso el piso es **480** y no 640: cubre ese caso con margen y baja la tarjeta de 60 KB a 35 KB. Bajar más (400, 320) ahorra menos de 10 KB y empieza a verse blando.

**Un teléfono DPR 3 cae en L.** El techo no lo consume sólo el escritorio: es lo que descarga un teléfono a pantalla completa. Por eso L cierra en **1600** y no 1920 — a 1920 y calidad 75 una foto densa pesa ~385 KB, muy por encima del presupuesto sano de 100–200 KB para una imagen hero. 1600 cubre 1366, 1440 y 1536 de forma nativa, y en un monitor Full HD se escala *hacia arriba* (0.83x), lo que suaviza en vez de pixelar.

### Calidad JPEG por defecto: 62

Con esta escalera ninguna variante se muestra 1:1 en una pantalla moderna: siempre hay downscale (DPR ≥ 2) o un leve upscale (monitor de 1920 recibiendo 1600). El downscale destruye los artefactos JPEG antes de que el ojo los alcance, así que la calidad puede bajar sin costo perceptual — y es la palanca de peso más grande que existe, mayor que cualquier ajuste de ancho:

| ancho | q62 | q75 |
|---|---|---|
| 480 | 28 KB | 36 KB |
| 1024 | 106 KB | 138 KB |
| 1600 | ~202 KB | ~267 KB |

(Foto densa, el peor caso de entropía; una fachada o un interior plano pesan ~30% menos.) `Config.Quality` sigue siendo configurable: 62 es sólo el valor por defecto cuando nadie lo fija.

### El descriptor `w` puede sobredeclarar

`ProcessImage` no amplía: una fuente de 1280 px produce un `foto.L.jpg` de 1280 px, pero `Responsive()` lo declara `1600w` porque sólo recibe una ruta y no puede medir el archivo. El navegador entonces elige esa variante creyendo que recibe más píxeles de los que hay.

Resolverlo en el render exigiría que el builder conociera el ancho real, y ese builder compila a WASM sin acceso al registro de assets. La contención vive en el pipeline: `ProcessImage` **avisa** cuando la fuente es más angosta que una variante declarada, porque el defecto está en el contenido (una fuente demasiado chica), no en el render.

### Asset Declarations
Instead of manually managing images, modules declare their requirements in a standard `image.go` file. By implementing `RenderImages() []image.Asset`, a module tells the system which images it needs and what responsive variants (Small, Medium, Large) should be generated.

### Stat-Based Caching (mtime)
To ensure fast development cycles, `image` avoids reprocessing images that haven't changed. Instead of maintaining a separate database or JSON file with image hashes, it uses the **Modification Time (mtime)** provided by the operating system's file system.

**The logic is simple and efficient:**
1. For every declared asset, the system checks if the output WebP files already exist.
2. If they exist, it compares the `mtime` of the source file (e.g., `.png`) with the `mtime` of the output files (e.g., `.S.webp`).
3. If the source file is newer than any of its outputs, the image is reprocessed.
4. If the outputs are newer or equal, the system skips them.

### Serving From Memory: `Artifacts()`

`Config.OutputDir` is where `LoadImages` writes converted files, but a
development server that serves a site from memory has no other way to obtain
that content — without `Artifacts()`, the consumer would have to keep a real
output directory inside the user's project just to have something to serve.

`Handler.Artifacts()` returns one `Artifact{Path, Mediatype, Content}` per file
that `LoadImages`/`ReloadModule` last produced, read back from `OutputDir`.
`Path` is `img/<name>` and doubles as the URL; `Mediatype` is derived from the
file extension (`image/jpeg`, `image/webp`, `image/svg+xml`). A source whose
output can't be read is skipped silently — the conversion failure was already
logged.

### Cache vs. Output: `DefaultCacheDir`

`OutputDir` plays two roles that this design keeps separate: it is the
**cache** that avoids re-encoding unchanged sources between runs, and it can
also be the **output** a site publishes. A consumer that only needs the former
(because it serves via `Artifacts()`) should not point `OutputDir` inside the
user's project. `DefaultCacheDir(rootDir)` returns a per-project directory
under the OS user cache (`os.UserCacheDir()`), keyed by a truncated SHA-256 of
`rootDir`, so the cache never lives inside — or gets confused with — a
project's publishable output.

## SEO Considerations

- **WebP Format**: Modern compression for faster page loads.
- **Responsive Variants**: Serve the smallest appropriate image for the user's device.
- **Alt Text**: Declared in `image.go` for use in HTML generation.

## Workflow

1. **Discovery**: `image/min` finds all modules via `go list`.
2. **Extraction**: For each module, it parses `image.go` to find image declarations.
3. **Validation**: It checks `mtime` to see which images actually need processing.
4. **Processing**: It resizes and encodes the necessary variants.
5. **Cleanup**: It removes any WebP files that are no longer declared by any module.
