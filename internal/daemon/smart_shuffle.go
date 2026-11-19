package daemon

import (
	"context"
	"fmt"

	"github.com/juanvillacortac/audiofilus/internal/control"
	"github.com/juanvillacortac/audiofilus/internal/spotify"
)

// isSpotifySmartShuffle reports whether Smart Shuffle is enabled.
func (d *Daemon) isSpotifySmartShuffle() bool {
	d.spotSmartMu.Lock()
	defer d.spotSmartMu.Unlock()
	return d.spotSmartOn
}

// setSpotifySmartShuffle enables or disables Smart Shuffle.
func (d *Daemon) setSpotifySmartShuffle(on bool) {
	d.spotSmartMu.Lock()
	d.spotSmartOn = on
	if on {
		d.spotSmartPlayed = 0
		if d.spotSmartInjected == nil {
			d.spotSmartInjected = make(map[string]bool)
		}
	}
	d.spotSmartMu.Unlock()

	if on {
		// When turned on, ensure engine shuffle is also enabled
		if d.eng != nil && !d.eng.Status().Shuffle {
			d.eng.SetShuffle(true)
		}
		go d.refillSmartShuffleBuffer()
	}
	go d.saveState()
	control.Notify()
}

// onTrackChangedSmartShuffle inspects playback progress and injects recommendations.
func (d *Daemon) onTrackChangedSmartShuffle(currentTrack string) {
	d.spotSmartMu.Lock()
	if !d.spotSmartOn || d.eng == nil {
		d.spotSmartMu.Unlock()
		return
	}
	d.spotSmartPlayed++
	played := d.spotSmartPlayed
	d.spotSmartMu.Unlock()

	// Every 3 tracks played, inject 1 recommendation if we are in a playlist containing Spotify tracks
	if played >= 3 {
		d.injectSmartShuffleTrack()
	}

	// Refill buffer if running low
	d.spotSmartMu.Lock()
	bufLen := len(d.spotSmartBuffer)
	d.spotSmartMu.Unlock()
	if bufLen < 5 {
		go d.refillSmartShuffleBuffer()
	}
}

// injectSmartShuffleTrack picks the next recommendation and appends it to the queue.
func (d *Daemon) injectSmartShuffleTrack() bool {
	if d.eng == nil {
		return false
	}
	queue := d.eng.Queue()
	if len(queue) == 0 {
		return false
	}
	// Check if queue has at least one Spotify track
	hasSpotify := false
	inQueue := make(map[string]bool)
	for _, tr := range queue {
		if spotify.IsSpotify(tr) {
			hasSpotify = true
			inQueue[spotify.ExtractTrackID(tr)] = true
		}
	}
	if !hasSpotify {
		return false
	}

	d.spotSmartMu.Lock()
	defer d.spotSmartMu.Unlock()

	if d.spotSmartInjected == nil {
		d.spotSmartInjected = make(map[string]bool)
	}

	candidateIdx := -1
	for i, tr := range d.spotSmartBuffer {
		if !inQueue[tr.ID] && !d.spotSmartInjected[tr.ID] {
			candidateIdx = i
			break
		}
	}

	if candidateIdx == -1 {
		go d.refillSmartShuffleBuffer()
		return false
	}

	tr := d.spotSmartBuffer[candidateIdx]
	d.spotSmartBuffer = append(d.spotSmartBuffer[:candidateIdx], d.spotSmartBuffer[candidateIdx+1:]...)
	d.spotSmartInjected[tr.ID] = true
	d.spotSmartPlayed = 0

	// Inject into queue with smart flag
	smartURI := fmt.Sprintf("spotify://track/%s?smart=1", tr.ID)
	d.eng.Append([]string{smartURI})
	return true
}

// refillSmartShuffleBuffer fetches recommendations using current queue as seeds.
func (d *Daemon) refillSmartShuffleBuffer() {
	if d.spotifyMgr == nil || d.eng == nil {
		return
	}

	d.spotSmartMu.Lock()
	if d.spotSmartFetching || !d.spotSmartOn {
		d.spotSmartMu.Unlock()
		return
	}
	d.spotSmartFetching = true
	d.spotSmartMu.Unlock()

	defer func() {
		d.spotSmartMu.Lock()
		d.spotSmartFetching = false
		d.spotSmartMu.Unlock()
	}()

	queue := d.eng.Queue()
	var seeds []string
	seenSeeds := make(map[string]bool)
	for _, raw := range queue {
		if spotify.IsSpotify(raw) {
			id := spotify.ExtractTrackID(raw)
			if id != "" && !seenSeeds[id] {
				seenSeeds[id] = true
				seeds = append(seeds, id)
				if len(seeds) >= 5 {
					break
				}
			}
		}
	}

	recs, err := d.spotifyMgr.Catalog.Recommendations(context.Background(), seeds, 20)
	if err != nil || len(recs) == 0 {
		return
	}
	d.spotifyMgr.CacheTracksInMemory(recs)

	d.spotSmartMu.Lock()
	defer d.spotSmartMu.Unlock()

	seenBuffer := make(map[string]bool)
	for _, tr := range d.spotSmartBuffer {
		seenBuffer[tr.ID] = true
	}
	for _, tr := range recs {
		if !seenBuffer[tr.ID] && (d.spotSmartInjected == nil || !d.spotSmartInjected[tr.ID]) {
			seenBuffer[tr.ID] = true
			d.spotSmartBuffer = append(d.spotSmartBuffer, tr)
		}
	}
}
