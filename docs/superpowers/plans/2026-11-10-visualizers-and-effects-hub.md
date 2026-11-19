# Visualizers & Effects Hub Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add audio-reactive background visualizers (NCS circle/3D sphere, Matrix rain, Warp speed), 5 customizable Now Playing spectrum styles, 5 progress bar styles including retro animated Nyan Cat, and a unified Effects Hub menu workflow.

**Architecture:** Visualizer engines are implemented in modular pure Go Ebitengine packages (`internal/ui/ncs.go`, `matrix.go`, `warp.go`, `nyancat.go`). Now Playing spectrum and progress bar renderers are abstracted in `internal/ui/screens.go` driven by persisted config keys (`now_playing_viz`, `progress_style`). The Screen Bar (Dock) and Now Playing More (START) menu provide frictionless live cycling without touching existing button habits (like Y for play/pause).

**Tech Stack:** Go 1.22+, Ebitengine v2 (`github.com/hajimehoshi/ebiten/v2`), pure Go 3D orthographic projection, zero CGO.

**Spec:** `docs/superpowers/specs/2026-11-10-visualizers-and-effects-hub-design.md`

## Global Constraints

- Zero CGO (`CGO_ENABLED=0`): Must compile cleanly for `linux/arm64` and `windows/amd64`.
- Documentation in English; code comments in Spanish in full sentences (`// Load lee...`).
- Error and test messages in English. Text seen by users in `internal/ui/lang.go` and all `lang_*.go`.
- Code formatted with `gofmt -l .` clean; `go vet ./...` and `go test ./...` must pass.
- Target device TrimUI Brick Pro (Allwinner A133P): pre-allocate geometry slices, zero per-frame heap allocations during visualization loops to ensure steady 60 FPS.

## Review Focus

1. **Zero-length audio bands / silent state:** Rendering visualizers when `meterNow()` returns all zeros or audio is paused must not divide by zero, panic, or render NaN floats.
2. **Extreme aspect ratios & screen sizes:** Visualizer math must scale safely from 320×240 (retro handheld) up to 1024×768 (TrimUI Brick Pro) and ultrawide 21:9 desktop windows without clipping or out-of-bounds panics.
3. **Invalid or uninitialized config values:** If `now_playing_viz` or `progress_style` in `audiofilus.json` contains an unknown string or is empty, fallback gracefully to default `"led"` and `"classic"`.
4. **Nyan Cat progress seek clamping:** Seeking past 100% or scrubbing when duration is unknown/live must not render the cat outside the bounding box or corrupt progress coordinates.
5. **No regressions on controller button assignments:** Y button in Now Playing must remain unchanged (play/pause as user configured), menu activations remain on START (`ActMenu`).

---

### Task 1: Configuration & Localization Foundation

**Files:**
- Modify: `internal/config/config.go:133-210`
- Modify: `internal/ui/lang.go`
- Modify: `internal/ui/lang_*.go` (all 13 language files + `lang_vi.go`)
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces:
  - `config.UI.NowPlayingViz`: string (`"led"`, `"smooth"`, `"wave"`, `"mirror"`, `"off"`)
  - `config.UI.ProgressStyle`: string (`"classic"`, `"retro"`, `"thin"`, `"pill"`, `"nyancat"`)
  - Translation keys: `vis_ncs`, `vis_matrix`, `vis_warp`, `np_viz_led`, `np_viz_smooth`, `np_viz_wave`, `np_viz_mirror`, `np_viz_off`, `prog_classic`, `prog_retro`, `prog_thin`, `prog_pill`, `prog_nyan`, `set_now_viz`, `set_prog_style`

- [ ] **Step 1: Write unit test for UI config defaults and validation**

In `internal/config/config_test.go`:
```go
func TestUIConfigVisualizerDefaults(t *testing.T) {
    var ui UI
    if ui.NowPlayingViz != "" || ui.ProgressStyle != "" {
        t.Errorf("expected empty zero values for new config fields")
    }
}
```

- [ ] **Step 2: Run test to verify it passes**

Run: `go test -v ./internal/config -run TestUIConfigVisualizerDefaults`
Expected: PASS

- [ ] **Step 3: Add `NowPlayingViz` and `ProgressStyle` to `internal/config/config.go`**

Add fields to struct `UI` with json tags:
```go
	// NowPlayingViz es el visualizador de Reproduciendo ("led", "smooth", "wave", "mirror", "off").
	NowPlayingViz string `json:"now_playing_viz,omitempty"`
	// ProgressStyle es el estilo de la barra de progreso ("classic", "retro", "thin", "pill", "nyancat").
	ProgressStyle string `json:"progress_style,omitempty"`
```

- [ ] **Step 4: Add localization strings across all language files**

In `internal/ui/lang.go` (Spanish, English, Turkish base tables), add:
- `vis_ncs`: "NCS / Esfera", "NCS / Sphere", "NCS / Küre"
- `vis_matrix`: "Matrix", "Matrix", "Matrix"
- `vis_warp`: "Hiperespacio", "Hyperdrive", "Hiperuzay"
- `np_viz_led`: "LED", "LED", "LED"
- `np_viz_smooth`: "Suave", "Smooth", "Yumuşak"
- `np_viz_wave`: "Onda", "Waveform", "Dalga"
- `np_viz_mirror`: "Espejo", "Mirror", "Ayna"
- `np_viz_off`: "Apagado", "Off", "Kapalı"
- `prog_classic`: "Clásico", "Classic", "Klasik"
- `prog_retro`: "Retro", "Retro", "Retro"
- `prog_thin`: "Fino", "Thin", "İnce"
- `prog_pill`: "Píldora", "Pill", "Hap"
- `prog_nyan`: "Nyan Cat", "Nyan Cat", "Nyan Cat"
- `set_now_viz`: "Visualizador de reproducción", "Now Playing Visualizer", "Çalma Görselleştiricisi"
- `set_prog_style`: "Estilo de barra de progreso", "Progress Bar Style", "İlerleme Çubuğu Stili"

Update all other language files (`lang_de.go`, `lang_fr.go`, `lang_id.go`, `lang_it.go`, `lang_ja.go`, `lang_ko.go`, `lang_pl.go`, `lang_pt.go`, `lang_ru.go`, `lang_zh.go`, `lang_vi.go`).

- [ ] **Step 5: Run tests to verify compilation and translation completeness**

Run: `go test ./internal/ui -run TestLanguages` (or verify with `go test ./internal/ui`)
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go internal/ui/lang*.go
git commit -m "feat(config): add now playing visualizer and progress bar style settings and localization"
```

---

### Task 2: NCS Radial & 3D Wireframe Sphere Visualizer (`visNCS`)

**Files:**
- Create: `internal/ui/ncs.go`
- Create: `internal/ui/ncs_test.go`
- Modify: `internal/ui/visual.go:40-60, 100-155, 340-360`

**Interfaces:**
- Produces:
  - `visNCS` constant in `internal/ui/visual.go`
  - `func (a *App) drawNCS(img *ebiten.Image)`
  - Variant 0: 2D radial pulsing circle with 64 radiating spikes.
  - Variant 1: 3D wireframe audio-reactive sphere with rotating latitude/longitude rings.
  - Variant 2: 3D particle galaxy sphere with transient bass expansion.

- [ ] **Step 1: Write unit test for NCS math transformations and variant safety**

In `internal/ui/ncs_test.go`:
```go
package ui

import (
    "testing"
    "github.com/hajimehoshi/ebiten/v2"
)

func TestNCSVisualizerDrawNoPanic(t *testing.T) {
    app := &App{}
    img := ebiten.NewImage(320, 240)
    for variant := 0; variant < 3; variant++ {
        app.vis.variant[visNCS] = variant
        app.drawNCS(img)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/ui -run TestNCSVisualizerDrawNoPanic`
Expected: FAIL (`visNCS` or `drawNCS` undefined)

- [ ] **Step 3: Implement `visNCS` in `internal/ui/visual.go` and `internal/ui/ncs.go`**

1. In `internal/ui/visual.go`:
   - Add `visNCS` to visualizer constants.
   - Add `"vis_ncs"` to `visNames`.
   - Add `"vis_ncs"` (or `"disc"`) to `visIcons`.
   - In `drawVisual`, add case for `visNCS: a.drawNCS(v.img)`.
   - In `updateVisual`, support cycling 3 variants for `visNCS`.
2. In `internal/ui/ncs.go`:
   - Implement `ncsState`:
     - Pre-allocate sphere vertices: 12 latitude rings $\times$ 16 longitude steps.
     - Orthographic 3D rotation around Y and X axes:
       $x' = x \cos \theta - z \sin \theta$, $z' = x \sin \theta + z \cos \theta$, $y' = y \cos \phi - z' \sin \phi$.
     - Vertex radius displacement based on audio band energy: $R = R_0 \cdot (1 + 0.8 \cdot \text{band}(lat))$.
     - Variant 0 (2D NCS circle): Draw center circle pulsing with bass, plus 64 radiating spoke bars around $2\pi$.
     - Variant 1 (3D Sphere Wireframe): Draw connected latitude and longitude ring line segments via vector/batch lines.
     - Variant 2 (Particle Galaxy): 120 orbital points with radial kick bursts.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/ui -run TestNCSVisualizerDrawNoPanic`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ui/ncs.go internal/ui/ncs_test.go internal/ui/visual.go
git commit -m "feat(ui): implement NCS radial circle and 3D wireframe sphere visualizer"
```

---

### Task 3: Matrix Rain & Starfield Warp Speed Visualizers (`visMatrix`, `visWarp`)

**Files:**
- Create: `internal/ui/matrix.go`
- Create: `internal/ui/warp.go`
- Create: `internal/ui/visual_extra_test.go`
- Modify: `internal/ui/visual.go`

**Interfaces:**
- Produces:
  - `visMatrix` and `visWarp` constants in `internal/ui/visual.go`
  - `func (a *App) drawMatrix(img *ebiten.Image)`
  - `func (a *App) drawWarp(img *ebiten.Image)`

- [ ] **Step 1: Write unit test for Matrix and Warp renderers**

In `internal/ui/visual_extra_test.go`:
```go
package ui

import (
    "testing"
    "github.com/hajimehoshi/ebiten/v2"
)

func TestMatrixAndWarpDrawNoPanic(t *testing.T) {
    app := &App{}
    img := ebiten.NewImage(320, 240)
    app.drawMatrix(img)
    app.drawWarp(img)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/ui -run TestMatrixAndWarpDrawNoPanic`
Expected: FAIL (`drawMatrix`, `drawWarp` undefined)

- [ ] **Step 3: Implement Matrix Rain in `internal/ui/matrix.go` and Warp in `internal/ui/warp.go`**

1. `matrix.go`:
   - Pre-allocate column streams (one column every 6 pixels).
   - Each column tracks head Y, drop velocity, and length.
   - Frequency band amplitude controls drop speed and tail brightness.
   - Draw glyph/pixel segments in column streams.
2. `warp.go`:
   - 256 star particles with $(X, Y, Z)$ coordinates where $Z \in [1, 1000]$.
   - Move stars towards camera ($Z \leftarrow Z - V$).
   - Base speed modulated by volume; kicks ($bass > 0.6$) dramatically increase speed and draw light streaks from prior $(X_{prev}, Y_{prev})$ to current $(X_{curr}, Y_{curr})$.
3. Update `internal/ui/visual.go`:
   - Register `visMatrix`, `visWarp` in `visNames`, `visIcons`.
   - Hook into `drawVisual`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/ui -run TestMatrixAndWarpDrawNoPanic`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ui/matrix.go internal/ui/warp.go internal/ui/visual_extra_test.go internal/ui/visual.go
git commit -m "feat(ui): add matrix rain and starfield warp speed visualizers"
```

---

### Task 4: Modular Now Playing Visualizer Styles (`NowPlayingViz`)

**Files:**
- Modify: `internal/ui/screens.go:850-910`
- Create: `internal/ui/screens_viz_test.go`

**Interfaces:**
- Consumes: `a.cfg.UI.NowPlayingViz`
- Produces:
  - `func (a *App) nowPlayingViz() string`
  - `func (a *App) cycleNowPlayingViz()`
  - `func (a *App) nowPlayingVizLabel() string`
  - Renderers: `drawSpectrumLED`, `drawSpectrumSmooth`, `drawSpectrumWave`, `drawSpectrumMirror`

- [ ] **Step 1: Write test for cycling and rendering Now Playing visualizers**

In `internal/ui/screens_viz_test.go`:
```go
package ui

import (
    "testing"
    "github.com/hajimehoshi/ebiten/v2"
)

func TestNowPlayingVizCycleAndDraw(t *testing.T) {
    app := &App{}
    modes := []string{"led", "smooth", "wave", "mirror", "off"}
    for _, mode := range modes {
        app.cfg.UI.NowPlayingViz = mode
        img := ebiten.NewImage(320, 240)
        app.drawSpectrum(img, 10, 150, 300, 40)
    }
    app.cfg.UI.NowPlayingViz = "led"
    app.cycleNowPlayingViz()
    if app.cfg.UI.NowPlayingViz != "smooth" {
        t.Errorf("expected smooth, got %s", app.cfg.UI.NowPlayingViz)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/ui -run TestNowPlayingVizCycleAndDraw`
Expected: FAIL (`cycleNowPlayingViz` undefined)

- [ ] **Step 3: Implement modular spectrum styles in `internal/ui/screens.go`**

1. Add helper methods:
   - `nowPlayingViz()`: returns `a.cfg.UI.NowPlayingViz` or `"led"` if empty.
   - `cycleNowPlayingViz()`: advances through `["led", "smooth", "wave", "mirror", "off"]`, calls `a.saveConfig()`.
   - `nowPlayingVizLabel()`: returns localized string (`T("np_viz_" + mode)`).
2. Refactor `drawSpectrum`:
   - If `a.nowPlayingViz() == "off"`: return immediately.
   - `"led"`: delegate to existing stacked LED block logic (`drawSpectrumLED`).
   - `"smooth"`: draw continuous solid vertical bars with theme gradient (`Accent` to `Accent2`) without inter-block spacing.
   - `"wave"`: draw a continuous connecting line plot across bands with smooth cubic/sine interpolation.
   - `"mirror"`: draw vertical bars expanding symmetrically from center ($x = w/2$) outward to edges.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/ui -run TestNowPlayingVizCycleAndDraw`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ui/screens.go internal/ui/screens_viz_test.go
git commit -m "feat(ui): add selectable Now Playing visualizer modes (LED, Smooth, Wave, Mirror, Off)"
```

---

### Task 5: Progress Bar Styles & Animated Nyan Cat (`ProgressStyle`)

**Files:**
- Create: `internal/ui/nyancat.go`
- Create: `internal/ui/nyancat_test.go`
- Modify: `internal/ui/screens.go:909-950`

**Interfaces:**
- Consumes: `a.cfg.UI.ProgressStyle`
- Produces:
  - `func (a *App) progressStyle() string`
  - `func (a *App) cycleProgressStyle()`
  - `func (a *App) progressStyleLabel() string`
  - `func drawNyanCatProgress(dst *ebiten.Image, bx, y, bw int, progress float64, t float64)`

- [ ] **Step 1: Write test for Nyan Cat and progress bar style cycling**

In `internal/ui/nyancat_test.go`:
```go
package ui

import (
    "testing"
    "github.com/hajimehoshi/ebiten/v2"
)

func TestProgressStyleCycleAndNyanDraw(t *testing.T) {
    app := &App{}
    img := ebiten.NewImage(320, 240)
    styles := []string{"classic", "retro", "thin", "pill", "nyancat"}
    for _, s := range styles {
        app.cfg.UI.ProgressStyle = s
        app.drawProgress(img, 10, 200, 300)
    }
    app.cfg.UI.ProgressStyle = "classic"
    app.cycleProgressStyle()
    if app.cfg.UI.ProgressStyle != "retro" {
        t.Errorf("expected retro, got %s", app.cfg.UI.ProgressStyle)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/ui -run TestProgressStyleCycleAndNyanDraw`
Expected: FAIL (`cycleProgressStyle` undefined)

- [ ] **Step 3: Implement Nyan Cat in `internal/ui/nyancat.go` and styles in `internal/ui/screens.go`**

1. In `internal/ui/nyancat.go`:
   - Define 16×11 pixel art matrix for Nyan Cat (gray head, pink body `#FFAACC` with darker pink sprinkles `#FF3399`, tail, and paws).
   - Frame 0 and Frame 1 paw/tail positions for 2-frame walking cycle.
   - Draw 6-color rainbow bands (Red `#FF0000`, Orange `#FF9900`, Yellow `#FFFF00`, Green `#33FF00`, Blue `#0099FF`, Purple `#6633FF`) each 1px high with vertical sine oscillation.
   - Position Nyan Cat at $x = bx + fill - 8$, $y = y - 4$.
2. In `internal/ui/screens.go`:
   - `progressStyle()`: returns `a.cfg.UI.ProgressStyle` or `"classic"`.
   - `cycleProgressStyle()`: advances through `["classic", "retro", "thin", "pill", "nyancat"]`, calls `a.saveConfig()`.
   - Update `drawProgress`:
     - `"classic"`: existing 3px bar + 5x7 panel thumb.
     - `"retro"`: segmented discrete blocks with gaps.
     - `"thin"`: 1px high line across $bw$, no thumb.
     - `"pill"`: 5px high bar with rounded ends.
     - `"nyancat"`: calls `drawNyanCatProgress`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/ui -run TestProgressStyleCycleAndNyanDraw`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ui/nyancat.go internal/ui/nyancat_test.go internal/ui/screens.go
git commit -m "feat(ui): add customizable progress bar styles with animated Nyan Cat"
```

---

### Task 6: Unified Effects Hub & Menu Workflow

**Files:**
- Modify: `internal/ui/dock.go:224-260`
- Modify: `internal/ui/menu.go:260-310`
- Modify: `internal/ui/settings.go:24-65, 300-380`
- Create: `internal/ui/menu_effects_test.go`

**Interfaces:**
- In `openNowMenu`: quick items for `Now Playing Viz` and `Progress Bar` with live current values and click-to-cycle action.
- In `openVisualMenu`: quick toggles for background viz, Now Playing viz, and progress bar style.
- In `Settings`: configuration tiles for Now Playing Viz and Progress Bar Style.

- [ ] **Step 1: Write test for menu items generation in Now Playing and Visual menus**

In `internal/ui/menu_effects_test.go`:
```go
package ui

import (
    "testing"
)

func TestNowMenuHasEffectsItems(t *testing.T) {
    app := &App{}
    app.openNowMenu()
    if app.menu == nil {
        t.Fatalf("expected menu to be open")
    }
    foundViz, foundProg := false, false
    for _, item := range app.menu.items {
        if item.label == T("set_now_viz") {
            foundViz = true
        }
        if item.label == T("set_prog_style") {
            foundProg = true
        }
    }
    if !foundViz || !foundProg {
        t.Errorf("missing effects items in now menu: foundViz=%v, foundProg=%v", foundViz, foundProg)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/ui -run TestNowMenuHasEffectsItems`
Expected: FAIL

- [ ] **Step 3: Integrate quick cycling in `internal/ui/menu.go`, `internal/ui/dock.go`, and `internal/ui/settings.go`**

1. In `internal/ui/menu.go` (`openNowMenu`):
   - Add `menuItem` for `set_now_viz`:
     ```go
     m.items = append(m.items, menuItem{
         label: T("set_now_viz"),
         icon:  "vis_bars",
         value: func() string { return a.nowPlayingVizLabel() },
         do:    func() { a.cycleNowPlayingViz() },
     })
     ```
   - Add `menuItem` for `set_prog_style`:
     ```go
     m.items = append(m.items, menuItem{
         label: T("set_prog_style"),
         icon:  "note",
         value: func() string { return a.progressStyleLabel() },
         do:    func() { a.cycleProgressStyle() },
     })
     ```
   - Add `menuItem` to jump to Visualizer screen:
     ```go
     m.items = append(m.items, menuItem{
         label: T("scr_visual"),
         icon:  "visual",
         do:    func() { a.screen = Visual; a.menu = nil },
     })
     ```
2. In `internal/ui/dock.go` (`openVisualMenu`):
   - Add items for cycling `Now Playing Viz` and `Progress Bar Style` alongside the background visualizer choices.
3. In `internal/ui/settings.go`:
   - Add `setNowViz` and `setProgStyle` to settings tiles.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/ui -run TestNowMenuHasEffectsItems`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ui/menu.go internal/ui/dock.go internal/ui/settings.go internal/ui/menu_effects_test.go
git commit -m "feat(ui): integrate visualizer and progress bar style cycling into menus and settings"
```

---

### Task 7: Comprehensive Verification & Cross-Platform Benchmarks

**Files:**
- All modified/created files across the repo

- [ ] **Step 1: Check code formatting**

Run: `gofmt -l .`
Expected: Output is empty (no unformatted files). If not empty, run `gofmt -w .`.

- [ ] **Step 2: Run all tests and vet on PC**

Run: `go vet ./... && go test ./...`
Expected: PASS across all packages.

- [ ] **Step 3: Verify cross-compilation for console without CGO**

Run: `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /dev/null ./cmd/audiofilus`
Expected: Exit code 0, compiles cleanly without errors.

- [ ] **Step 4: Verify Windows build without CGO**

Run: `GOOS=windows go vet ./...`
Expected: Exit code 0, no errors.

- [ ] **Step 5: Commit final polish and verification tests**

```bash
git commit --allow-empty -m "chore: verify visualizers and effects hub across all target platforms"
```
