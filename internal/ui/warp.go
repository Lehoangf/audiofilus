package ui

import (
	"math"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// warpStar represents a star in the 3D starfield.
type warpStar struct {
	x, y, z float64
	pz      float64 // Previous frame Z for streak tracing
}

// warpState stores starfield state for the hyperspace visualizer.
type warpState struct {
	stars [256]warpStar
	init  bool
	rng   *rand.Rand
}

// drawWarp renders the Starfield Warp Speed visualizer:
//   - Variant 0: Direct light-speed tunnel towards the camera.
//   - Variant 1: Hyperspace jump vortex with angular rotation.
func (a *App) drawWarp(img *ebiten.Image) {
	lw, lh := img.Bounds().Dx(), img.Bounds().Dy()
	if lw < 8 || lh < 8 {
		return
	}
	level, bass, _ := a.visBands()
	pulse := a.beatPulse()

	ws := a.vis.warp
	if ws == nil || !ws.init {
		ws = &warpState{
			rng: rand.New(rand.NewSource(42)),
		}
		for i := range ws.stars {
			ws.stars[i].x = (ws.rng.Float64()*2.0 - 1.0) * float64(lw) * 1.5
			ws.stars[i].y = (ws.rng.Float64()*2.0 - 1.0) * float64(lh) * 1.5
			ws.stars[i].z = ws.rng.Float64()*900.0 + 10.0
			ws.stars[i].pz = ws.stars[i].z
		}
		ws.init = true
		a.vis.warp = ws
	}

	cx, cy := float64(lw)/2, float64(lh)/2
	fov := float64(min(lw, lh)) * 0.85
	const maxZ = 950.0

	// Speed modulated by volume and bass hits
	speed := 4.0 + 5.0*level + 26.0*bass + 14.0*pulse
	variant := a.vis.variant[visWarp] % 2

	// Vortex rotation angle for variant 1
	rotDelta := 0.0
	if variant == 1 {
		rotDelta = 0.015 + 0.03*bass + 0.01*pulse
	}

	for i := range ws.stars {
		s := &ws.stars[i]
		s.pz = s.z
		s.z -= speed

		if variant == 1 && rotDelta > 0 {
			// Rotate around Z axis
			cosR, sinR := math.Cos(rotDelta), math.Sin(rotDelta)
			nx := s.x*cosR - s.y*sinR
			ny := s.x*sinR + s.y*cosR
			s.x, s.y = nx, ny
		}

		// Reset stars passing behind the camera
		if s.z <= 2.0 {
			s.x = (ws.rng.Float64()*2.0 - 1.0) * float64(lw) * 1.5
			s.y = (ws.rng.Float64()*2.0 - 1.0) * float64(lh) * 1.5
			s.z = maxZ - ws.rng.Float64()*100.0
			s.pz = s.z
		}

		sx := cx + (s.x/s.z)*fov
		sy := cy + (s.y/s.z)*fov

		// Reset stars leaving screen boundaries
		if sx < 0 || sx >= float64(lw) || sy < 0 || sy >= float64(lh) {
			s.x = (ws.rng.Float64()*2.0 - 1.0) * float64(lw) * 1.5
			s.y = (ws.rng.Float64()*2.0 - 1.0) * float64(lh) * 1.5
			s.z = maxZ - ws.rng.Float64()*100.0
			s.pz = s.z
			continue
		}

		spx := cx + (s.x/s.pz)*fov
		spy := cy + (s.y/s.pz)*fov

		depth := 1.0 - (s.z / maxZ)
		intensity := math.Max(0.15, math.Min(1.9, depth*(1.1+0.6*bass+0.4*pulse)))

		dx := sx - spx
		dy := sy - spy
		distSq := dx*dx + dy*dy

		// Draw streak line if moving fast or near camera
		if distSq > 1.5 && (speed > 8.0 || depth > 0.4) {
			strokeW := float32(1.0)
			if depth > 0.7 && (bass > 0.3 || pulse > 0.3) {
				strokeW = 1.6
			}
			vector.StrokeLine(img, float32(spx), float32(spy), float32(sx), float32(sy),
				strokeW, gray(intensity), false)
		} else {
			// Distant point star
			sz := 1
			if depth > 0.75 {
				sz = 2
			}
			a.batch.add(int(sx), int(sy), sz, sz, gray(intensity))
		}
	}

	a.batch.draw(img)
}
