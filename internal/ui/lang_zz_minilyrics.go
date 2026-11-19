package ui

// Los textos de la mini letra en Reproduciendo y su atajo (lyrics.go,
// screens.go), en los catorce idiomas.
func init() {
	for lang, m := range map[string]map[string]string{
		"es": {
			"lyr_now_playing": "Ver en Reproduciendo",
			"h_lyrics":        "Letra",
		},
		"en": {
			"lyr_now_playing": "Show in Now Playing",
			"h_lyrics":        "Lyrics",
		},
		"tr": {
			"lyr_now_playing": "Şimdi Çalınıyor'da göster",
			"h_lyrics":        "Sözler",
		},
		"de": {
			"lyr_now_playing": "In Aktuell wiedergegeben zeigen",
			"h_lyrics":        "Liedtext",
		},
		"fr": {
			"lyr_now_playing": "Afficher dans Lecture",
			"h_lyrics":        "Paroles",
		},
		"it": {
			"lyr_now_playing": "Mostra in In riproduzione",
			"h_lyrics":        "Testo",
		},
		"pt": {
			"lyr_now_playing": "Mostrar em Reproduzindo",
			"h_lyrics":        "Letra",
		},
		"pl": {
			"lyr_now_playing": "Pokaż w Odtwarzaniu",
			"h_lyrics":        "Tekst",
		},
		"ru": {
			"lyr_now_playing": "Показать на экране плеера",
			"h_lyrics":        "Текст",
		},
		"id": {
			"lyr_now_playing": "Tampilkan di Sedang Diputar",
			"h_lyrics":        "Lirik",
		},
		"ja": {
			"lyr_now_playing": "再生中画面に表示",
			"h_lyrics":        "歌詞",
		},
		"zh": {
			"lyr_now_playing": "在正在播放中显示",
			"h_lyrics":        "歌词",
		},
		"ko": {
			"lyr_now_playing": "재생 중 화면에 표시",
			"h_lyrics":        "가사",
		},
		"vi": {
			"lyr_now_playing": "Hiện ở Đang phát",
			"h_lyrics":        "Lời",
		},
	} {
		for k, v := range m {
			strs[lang][k] = v
		}
	}
}
