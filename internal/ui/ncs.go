package ui

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// drawNCS renders NCS-style audio visualizers based on the active variant:
//   - 0: 2D radial circle with 64 symmetric frequency bars (NoCopyrightSounds style).
//   - 1: 3D wireframe rotating sphere pulsing and deforming with audio.
//   - 2: 3D particle galaxy sphere with dynamic bass expansion.
func (a *App) drawNCS(img *ebiten.Image) {
	variant := a.vis.variant[visNCS] % 3
	switch variant {
	case 1:
		a.drawNCSSphere3D(img)
	case 2:
		a.drawNCSGalaxy3D(img)
	default:
		a.drawNCSRadial2D(img)
	}
}

// drawNCSRadial2D renders a 2D radial ring with a pulsating core and 64 symmetric frequency bars.
func (a *App) drawNCSRadial2D(img *ebiten.Image) {
	lw, lh := img.Bounds().Dx(), img.Bounds().Dy()
	if lw < 8 || lh < 8 {
		return
	}
	level, bass, bands := a.visBands()
	pulse := a.beatPulse()

	cx, cy := float64(lw)/2, float64(lh)/2
	minDim := float64(min(lw, lh))
	r0 := minDim * (0.16 + 0.06*pulse + 0.05*bass)
	const n = 64

	type pt struct {
		x, y float64
	}
	innerPts := make([]pt, n)
	outerPts := make([]pt, n)

	// Slow rotation of the radial ring
	baseAngle := a.t * 0.25

	for i := 0; i < n; i++ {
		ang := 2*math.Pi*float64(i)/float64(n) + baseAngle
		// Symmetric spectrum distribution
		f := math.Abs(float64(i%n)/float64(n)*2 - 1)
		val := bandAt(bands, f)

		// Peak bar height
		spikeH := val * minDim * 0.32 * (0.75 + 0.35*bass + 0.2*level)
		rOut := r0 + spikeH + 1

		cosA, sinA := math.Cos(ang), math.Sin(ang)
		innerPts[i] = pt{cx + r0*cosA, cy + r0*sinA}
		outerPts[i] = pt{cx + rOut*cosA, cy + rOut*sinA}

		// Radial bar
		intensity := 0.9 + 0.6*val + 0.3*pulse
		vector.StrokeLine(img, float32(innerPts[i].x), float32(innerPts[i].y),
			float32(outerPts[i].x), float32(outerPts[i].y), 1.5, gray(intensity), false)
	}

	// Connect inner ring and outer rim
	for i := 0; i < n; i++ {
		next := (i + 1) % n
		// Solid inner ring
		vector.StrokeLine(img, float32(innerPts[i].x), float32(innerPts[i].y),
			float32(innerPts[next].x), float32(innerPts[next].y), 1.8, gray(1.2+0.4*pulse), false)
		// Outer rim outline
		vector.StrokeLine(img, float32(outerPts[i].x), float32(outerPts[i].y),
			float32(outerPts[next].x), float32(outerPts[next].y), 1.0, gray(0.7+0.5*level), false)
	}

	// Inner concentric circle pulsating on beat
	rInner := r0 * 0.65
	const innerN = 32
	for i := 0; i < innerN; i++ {
		ang1 := 2 * math.Pi * float64(i) / float64(innerN)
		ang2 := 2 * math.Pi * float64(i+1) / float64(innerN)
		x1, y1 := cx+rInner*math.Cos(ang1), cy+rInner*math.Sin(ang1)
		x2, y2 := cx+rInner*math.Cos(ang2), cy+rInner*math.Sin(ang2)
		vector.StrokeLine(img, float32(x1), float32(y1), float32(x2), float32(y2), 1.0, gray(0.8+0.5*pulse), false)
	}
}

// drawNCSSphere3D renders a 3D wireframe sphere rotating on three axes and deformed by audio bands.
func (a *App) drawNCSSphere3D(img *ebiten.Image) {
	lw, lh := img.Bounds().Dx(), img.Bounds().Dy()
	if lw < 8 || lh < 8 {
		return
	}
	level, bass, bands := a.visBands()
	pulse := a.beatPulse()

	cx, cy := float64(lw)/2, float64(lh)/2
	minDim := float64(min(lw, lh))
	rBase := minDim * (0.28 + 0.08*pulse + 0.05*bass)
	camDist := minDim * 1.5

	rotX := a.t * 0.42
	rotY := a.t * 0.68
	rotZ := a.t * 0.21

	sinX, cosX := math.Sin(rotX), math.Cos(rotX)
	sinY, cosY := math.Sin(rotY), math.Cos(rotY)
	sinZ, cosZ := math.Sin(rotZ), math.Cos(rotZ)

	project := func(lat, lon float64) (float32, float32, float64, bool) {
		f := math.Abs(lat) / (math.Pi / 2)
		r := rBase * (1.0 + 0.45*bandAt(bands, f) + 0.25*bass + 0.15*level)

		x0 := r * math.Cos(lat) * math.Cos(lon)
		y0 := r * math.Sin(lat)
		z0 := r * math.Cos(lat) * math.Sin(lon)

		// Y rotation
		x1 := x0*cosY + z0*sinY
		z1 := -x0*sinY + z0*cosY

		// X rotation
		y2 := y0*cosX - z1*sinX
		z2 := y0*sinX + z1*cosX

		// Z rotation
		x3 := x1*cosZ - y2*sinZ
		y3 := x1*sinZ + y2*cosZ
		z3 := z2

		if z3+camDist <= 1 {
			return 0, 0, 0, false
		}
		scale := camDist / (camDist + z3)
		sx := float32(cx + x3*scale)
		sy := float32(cy + y3*scale)

		// Brightness based on depth (brighter in foreground)
		depth := math.Max(0.25, math.Min(1.8, 1.15-z3/(rBase*1.4)))
		return sx, sy, depth, true
	}

	const nLat = 8
	const nLon = 16

	// Latitude lines
	for i := 1; i < nLat; i++ {
		lat := -math.Pi/2 + math.Pi*float64(i)/float64(nLat)
		for j := 0; j < nLon; j++ {
			lon1 := 2 * math.Pi * float64(j) / float64(nLon)
			lon2 := 2 * math.Pi * float64(j+1) / float64(nLon)
			x1, y1, b1, ok1 := project(lat, lon1)
			x2, y2, b2, ok2 := project(lat, lon2)
			if ok1 && ok2 {
				avgB := (b1 + b2) * 0.5
				vector.StrokeLine(img, x1, y1, x2, y2, 1.0, gray(avgB), false)
			}
		}
	}

	// Longitude lines
	for j := 0; j < nLon; j++ {
		lon := 2 * math.Pi * float64(j) / float64(nLon)
		for i := 0; i < nLat; i++ {
			lat1 := -math.Pi/2 + math.Pi*float64(i)/float64(nLat)
			lat2 := -math.Pi/2 + math.Pi*float64(i+1)/float64(nLat)
			x1, y1, b1, ok1 := project(lat1, lon)
			x2, y2, b2, ok2 := project(lat2, lon)
			if ok1 && ok2 {
				avgB := (b1 + b2) * 0.5
				vector.StrokeLine(img, x1, y1, x2, y2, 1.0, gray(avgB), false)
			}
		}
	}
}

// drawNCSGalaxy3D renders a 3D spherical Fibonacci particle cloud with perspective projection.
func (a *App) drawNCSGalaxy3D(img *ebiten.Image) {
	lw, lh := img.Bounds().Dx(), img.Bounds().Dy()
	if lw < 8 || lh < 8 {
		return
	}
	level, bass, bands := a.visBands()
	pulse := a.beatPulse()

	cx, cy := float64(lw)/2, float64(lh)/2
	minDim := float64(min(lw, lh))
	rBase := minDim * (0.34 + 0.12*pulse + 0.10*bass)
	camDist := minDim * 1.4

	rotX := a.t * 0.35
	rotY := a.t * 0.55
	rotZ := a.t * 0.18

	sinX, cosX := math.Sin(rotX), math.Cos(rotX)
	sinY, cosY := math.Sin(rotY), math.Cos(rotY)
	sinZ, cosZ := math.Sin(rotZ), math.Cos(rotZ)

	const nParticles = 210
	goldenRatio := (1.0 + math.Sqrt(5.0)) / 2.0

	for i := 0; i < nParticles; i++ {
		// Uniform spherical distribution (Fibonacci lattice)
		yNorm := 1.0 - (float64(i)/float64(nParticles-1))*2.0 // 1 to -1
		radAtY := math.Sqrt(math.Max(0, 1.0-yNorm*yNorm))
		theta := 2.0 * math.Pi * float64(i) / goldenRatio

		f := math.Abs(yNorm)
		val := bandAt(bands, f)
		r := rBase * (1.0 + 0.55*val + 0.2*level)

		x0 := r * math.Cos(theta) * radAtY
		y0 := r * yNorm
		z0 := r * math.Sin(theta) * radAtY

		// 3D rotation
		x1 := x0*cosY + z0*sinY
		z1 := -x0*sinY + z0*cosY

		y2 := y0*cosX - z1*sinX
		z2 := y0*sinX + z1*cosX

		x3 := x1*cosZ - y2*sinZ
		y3 := x1*sinZ + y2*cosZ
		z3 := z2

		if z3+camDist <= 1 {
			continue
		}
		scale := camDist / (camDist + z3)
		sx := int(cx + x3*scale)
		sy := int(cy + y3*scale)

		if sx < 0 || sx >= lw || sy < 0 || sy >= lh {
			continue
		}

		// Brightness and size scaled by z-depth
		bright := math.Max(0.2, math.Min(1.9, 1.2-z3/(rBase*1.3)))
		sz := 1
		if z3 < 0 && (bass > 0.3 || pulse > 0.4) {
			sz = 2
		}
		a.batch.add(sx, sy, sz, sz, gray(bright))
	}

	a.batch.draw(img)
}
