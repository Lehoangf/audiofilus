package ui

// Los textos de Spotify en los catorce idiomas.
func init() {
	for lang, m := range map[string]map[string]string{
		"es": {
			"spotify_smart_shuffle":     "Smart Shuffle",
			"spotify_download_playlist": "Descargar lista",
		},
		"en": {
			"spotify_smart_shuffle":     "Smart Shuffle",
			"spotify_download_playlist": "Download playlist",
		},
		"tr": {
			"spotify_smart_shuffle":     "Akıllı Karıştırma",
			"spotify_download_playlist": "Çalma listesini indir",
		},
		"de": {
			"spotify_smart_shuffle":     "Smart Shuffle",
			"spotify_download_playlist": "Playlist herunterladen",
		},
		"fr": {
			"spotify_smart_shuffle":     "Lecture aléatoire intelligente",
			"spotify_download_playlist": "Télécharger la playlist",
		},
		"it": {
			"spotify_smart_shuffle":     "Smart Shuffle",
			"spotify_download_playlist": "Scarica playlist",
		},
		"pt": {
			"spotify_smart_shuffle":     "Ordem aleatória inteligente",
			"spotify_download_playlist": "Baixar playlist",
		},
		"pl": {
			"spotify_smart_shuffle":     "Inteligentne losowo",
			"spotify_download_playlist": "Pobierz playlistę",
		},
		"ru": {
			"spotify_smart_shuffle":     "Умное перемешивание",
			"spotify_download_playlist": "Скачать плейлист",
		},
		"id": {
			"spotify_smart_shuffle":     "Acak Pintar",
			"spotify_download_playlist": "Unduh playlist",
		},
		"ja": {
			"spotify_smart_shuffle":     "スマートシャッフル",
			"spotify_download_playlist": "プレイリストをダウンロード",
		},
		"zh": {
			"spotify_smart_shuffle":     "智能随机播放",
			"spotify_download_playlist": "下载播放列表",
		},
		"ko": {
			"spotify_smart_shuffle":     "스마트 셔플",
			"spotify_download_playlist": "재생목록 다운로드",
		},
		"vi": {
			"spotify_smart_shuffle":     "Trộn thông minh",
			"spotify_download_playlist": "Tải playlist",
		},
	} {
		for k, v := range m {
			strs[lang][k] = v
		}
	}
}
