# Design Specification: Audiofilus Visualizers & Effects Hub for TrimUI Brick Pro (muOS)

**Date:** 2026-11-10  
**Status:** Draft / Pending Review  
**Target Platform:** TrimUI Brick Pro (Allwinner A133P, 1024×768 4:3 display, muOS / MustardOS) and standard Audiofilus platforms (Linux arm64, PC desktop, Windows)

---

## 1. Executive Summary

This specification defines the architectural design and implementation plan for expanding **Audiofilus** with:
1. **New Audio-Reactive Background Visualizers** in the Visualizer engine, notably:
   - **NCS / Spherical Mesh (`vis_ncs`)**: With multiple switchable variants (2D radial pulsing NCS circle, 3D wireframe audio-deforming sphere, and 3D particle galaxy).
   - **Digital Rain / Matrix (`vis_matrix`)**: Cascading code rain reacting in column height and brightness to live audio bands.
   - **Starfield Hyperdrive (`vis_warp`)**: Deep space warp speed effect accelerating and expanding outward on kick/bass drops.
2. **Now Playing Visualizer Customization**:
   - Expanding the single hardcoded pixel block spectrum to 5 selectable modes: LED Blocks (default), Smooth Bars, Waveform Line, Mirror Bars, and Off.
3. **Customizable Progress Bar with Retro Nyan Cat**:
   - 5 distinct visual styles for the track progress bar: Classic (default), Retro Segmented, Thin Minimal, Bold Pill, and **Nyan Cat** (animated flying pixel cat knob with wavy rainbow trail).
4. **Unified Effects Hub & Streamlined Customization Workflow**:
   - Unifying background visualizers, Now Playing spectrum styles, and progress bar styles into the **Screen Bar (Dock) Visualizer Hub**.
   - Offering fast cycling options in the Now Playing **More (START)** menu without altering user gameplay/controller habits (e.g. retaining button assignments like Y for play/pause).
   - Persisting all visual choices cleanly in `audiofilus.json`.

---

## 2. Architecture & Component Breakdown

### 2.1 Configuration Schema (`internal/config/config.go`)

Extend the `UI` configuration struct:

```go
type UI struct {
    // ... existing fields ...
    Visualizer     string `json:"visualizer,omitempty"`
    VisualBG       bool   `json:"visual_bg"`
    NowPlayingViz  string `json:"now_playing_viz,omitempty"`  // "led", "smooth", "wave", "mirror", "off" (default: "led")
    ProgressStyle  string `json:"progress_style,omitempty"`   // "classic", "retro", "thin", "pill", "nyancat" (default: "classic")
}
```

- When empty, defaults preserve 100% backward compatibility:
  - `NowPlayingViz == ""` -> `"led"`
  - `ProgressStyle == ""` -> `"classic"`

---

### 2.2 Background Visualizers (`internal/ui/visual.go`, `internal/ui/ncs.go`, `internal/ui/matrix.go`, `internal/ui/warp.go`)

Extend the visualizer registry:

```go
const (
    visDrift = iota
    visBars
    visTunnel
    visPlasma
    visWaves
    visTrench
    visLake
    visRidges
    visMilk
    visNCS      // NCS Radial Circle & 3D Wireframe Sphere
    visMatrix   // Digital Matrix Rain
    visWarp     // Deep Space Warp Speed
    visCount
)
```

#### A. NCS Visualizer (`visNCS`, `internal/ui/ncs.go`)
- **Variant 0: Classic 2D Radial NCS**:
  - Center pulsing circle with radius scaled by instantaneous bass energy ($r = r_0 + \text{bass} \times \Delta r$).
  - 64 radial frequency spikes radiating outward $360^\circ$ calculated from interpolated FFT bands.
  - Beat-reactive circular particles dispersing from the perimeter.
- **Variant 1: 3D Wireframe Audio-Reactive Sphere**:
  - Pure Go 3D vertex transform pipeline (orthographic projection onto 2D canvas, no external C/GPU shader requirements).
  - Rotates smoothly around pitch/yaw axes.
  - Surface vertices deform outward based on frequency band amplitude mapped to latitude/longitude.
  - Rendered via Ebitengine batch lines/segments for 60 FPS performance on Allwinner A133P.
- **Variant 2: 3D Particle Galaxy Sphere**:
  - Orbiting swarm of 3D point stars forming a spherical shell.
  - Particles burst outward in radius upon sudden bass spikes (transients) and contract back into orbit.

#### B. Digital Matrix Rain (`visMatrix`, `internal/ui/matrix.go`)
- Vertical streams of glowing characters/pixels spanning the width of the display.
- Column drop speeds and illumination intensities are modulated by the 16 audio frequency bands.
- Bass kicks trigger phosphor-glow lightning flashes across active columns.

#### C. Starfield Hyperdrive (`visWarp`, `internal/ui/warp.go`)
- 200–300 3D star coordinates with $Z$ coordinates approaching the viewer.
- Normal flight speed when quiet; kicks and sub-bass transients dynamically warp the velocity vector, drawing streak trails radiating from the center.

---

### 2.3 Now Playing Visualizer (`internal/ui/screens.go`)

Replace the monolithic `drawSpectrum` logic with modular rendering strategies based on `a.cfg.UI.NowPlayingViz`:

```go
const (
    NowVizLED    = "led"     // Classic stacked pixel blocks with falling peaks
    NowVizSmooth = "smooth"  // Solid gradient columns with smooth interpolation
    NowVizWave   = "wave"    // Smooth continuous oscilloscope-like frequency wave
    NowVizMirror = "mirror"  // Center-outward symmetric bars
    NowVizOff    = "off"     // Hidden, maximizing clean layout
)
```

1. **LED Blocks (`NowVizLED`)**: The existing retro Hi-Fi LED bar analyzer with gravity-delayed peak markers.
2. **Smooth Bars (`NowVizSmooth`)**: Smooth anti-aliased or solid gradient bars without segmented gaps, reacting fluidly to audio dynamics.
3. **Waveform Curve (`NowVizWave`)**: Single connected spline/wave plotted across `[margin, w - margin]`, modulated by frequency harmonics and phase shifts.
4. **Mirror Bars (`NowVizMirror`)**: Spectrum mirrored horizontally from screen center outward (lows in center, highs on edges).
5. **Off (`NowVizOff`)**: Skips spectrum rendering completely.

---

### 2.4 Progress Bar Styles & Nyan Cat (`internal/ui/screens.go`, `internal/ui/nyancat.go`)

Replace fixed line rendering in `drawProgress` with selectable renderers:

```go
const (
    ProgClassic = "classic"  // Standard line + rectangular thumb
    ProgRetro   = "retro"    // Discrete segmented LED blocks (LCD style)
    ProgThin    = "thin"     // Ultra-thin 1px minimalist line
    ProgPill    = "pill"     // Rounded pill / capsule bar
    ProgNyan    = "nyancat"  // Animated pixel Nyan Cat + rainbow trail
)
```

#### Nyan Cat Implementation (`ProgNyan`)
- **Rainbow Trail**: 6 horizontal stripes (Red `#FF0000`, Orange `#FF9900`, Yellow `#FFFF00`, Green `#33FF00`, Blue `#0099FF`, Purple `#6633FF`).
- **Wave Oscillation**: The rainbow path oscillates with a sine wave $\sin(x \cdot \omega + t \cdot \phi)$ that synchronizes subtly with audio beats.
- **Animated Cat Knob**:
  - Embedded 16×11 pixel art of Nyan Cat (gray cat head, pink Pop-Tart body with red sprinkles, wagging tail and paws).
  - 2-frame running animation toggled by `frame := int(a.t * 8) % 2`.
  - Positioned exactly at the track progress coordinate: $x_{\text{cat}} = x_{\text{start}} + \text{progress} \cdot \text{width}$.

---

### 2.5 Workflow & User Interface Integration

1. **Screen Bar (Dock) Effects Hub**:
   - The dock item `{screen: Visual}` remains the central hub (`scr_visual` / `vis_title`).
   - In Visual screen:
     - `Left` / `Right` switches background visualizers.
     - `A` cycles visualizer variants (cameras/modes).
     - `Menu` (`START` or `openVisualMenu`) opens the full Visualizer Menu containing:
       - Visualizer selection list.
       - Quick switcher for `Now Playing Viz`.
       - Quick switcher for `Progress Bar`.
2. **Now Playing Quick More Menu (`openNowMenu`)**:
   - Activated via `START` button on Now Playing.
   - Retains all existing controls (Y button is untouched, preserving user play/pause preferences).
   - In `openNowMenu`, add interactive `menuItem`s:
     - **Now Playing Viz**: Cycles `LED` $\to$ `Smooth` $\to$ `Wave` $\to$ `Mirror` $\to$ `Off` with live value badge.
     - **Progress Bar**: Cycles `Classic` $\to$ `Retro` $\to$ `Thin` $\to$ `Pill` $\to$ `Nyan Cat` with live value badge.
     - **Background Viz**: Retains toggle for `VisualBG` on/off.
     - **Go to Effects Hub**: Directly navigates to the Visualizer screen.
3. **Settings Screen (`internal/ui/settings.go`)**:
   - Add tiles in Settings for `Now Playing Viz` and `Progress Bar Style` for permanent customization.
4. **Localization (`internal/ui/lang*.go` & `internal/ui/lang_vi.go`)**:
   - New localized strings for all 13 languages + Vietnamese:
     - `vis_ncs`, `vis_matrix`, `vis_warp`
     - `np_viz_led`, `np_viz_smooth`, `np_viz_wave`, `np_viz_mirror`, `np_viz_off`
     - `prog_classic`, `prog_retro`, `prog_thin`, `prog_pill`, `prog_nyan`
     - Menu labels and tooltips.

---

## 3. Hardware & Performance Guarantees

- **Target Device:** TrimUI Brick Pro (Allwinner A133P: 4x Cortex-A53 @ 1.8 GHz, PowerVR GE8300 GPU).
- **Target Framerate:** Solid 60 FPS on 1024×768 resolution.
- **Resource Constraints:**
  - Zero CGO (`CGO_ENABLED=0`).
  - Pre-allocated vertex/point slices to prevent GC pressure during audio visualization loops.
  - Batch drawing (`rectBatch` / `vector.StrokeLine`) to minimize Ebitengine draw calls.
  - Background rendering in `drawVisualBG` scales gracefully with low alpha blending overhead.

---

## 4. Verification & Testing Strategy

1. **Unit & Logic Tests**:
   - Config serialization/deserialization tests in `internal/config`.
   - Cycle helper tests for Now Playing viz and progress styles.
   - Geometry and bounds checking for 3D sphere vertex calculations.
2. **PC Benchmarking & Validation**:
   - `go vet ./...` and `go test ./...` on PC.
   - Run `dist/audiofilus -bench lake` and new visualizer benchmarks to ensure zero frame drops (< 2 ms render frame budget).
   - Verify all 14 languages compile without missing key errors.
3. **Cross-Compilation**:
   - Verify clean build without CGO:
     `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /dev/null ./cmd/audiofilus`
     `GOOS=windows go vet ./...`
