package ui

// Los textos de Spotify en los catorce idiomas.
func init() {
	for lang, m := range map[string]map[string]string{
		"es": {
			"spotify_smart_shuffle":     "Smart Shuffle",
			"spotify_download_playlist": "Descargar lista",
			"spotify_sort_loading":      "Cargando lista completa para ordenar...",
		},
		"en": {
			"spotify_smart_shuffle":     "Smart Shuffle",
			"spotify_download_playlist": "Download playlist",
			"spotify_sort_loading":      "Loading full list to sort...",
		},
		"tr": {
			"spotify_smart_shuffle":     "Akıllı Karıştırma",
			"spotify_download_playlist": "Çalma listesini indir",
			"spotify_sort_loading":      "Sıralamak için tam liste yükleniyor...",
		},
		"de": {
			"spotify_smart_shuffle":     "Smart Shuffle",
			"spotify_download_playlist": "Playlist herunterladen",
			"spotify_sort_loading":      "Vollständige Liste zum Sortieren laden...",
		},
		"fr": {
			"spotify_smart_shuffle":     "Lecture aléatoire intelligente",
			"spotify_download_playlist": "Télécharger la playlist",
			"spotify_sort_loading":      "Chargement de la liste complète...",
		},
		"it": {
			"spotify_smart_shuffle":     "Smart Shuffle",
			"spotify_download_playlist": "Scarica playlist",
			"spotify_sort_loading":      "Caricamento elenco completo...",
		},
		"pt": {
			"spotify_smart_shuffle":     "Ordem aleatória inteligente",
			"spotify_download_playlist": "Baixar playlist",
			"spotify_sort_loading":      "Carregando lista completa para ordenar...",
		},
		"pl": {
			"spotify_smart_shuffle":     "Inteligentne losowo",
			"spotify_download_playlist": "Pobierz playlistę",
			"spotify_sort_loading":      "Ładowanie pełnej listy do posortowania...",
		},
		"ru": {
			"spotify_smart_shuffle":     "Умное перемешивание",
			"spotify_download_playlist": "Скачать плейлист",
			"spotify_sort_loading":      "Загрузка полного списка для сортировки...",
		},
		"id": {
			"spotify_smart_shuffle":     "Acak Pintar",
			"spotify_download_playlist": "Unduh playlist",
			"spotify_sort_loading":      "Memuat toàn bộ daftar untuk mengurutkan...",
		},
		"ja": {
			"spotify_smart_shuffle":     "スマートシャッフル",
			"spotify_download_playlist": "プレイリストをダウンロード",
			"spotify_sort_loading":      "並べ替えるために全リストを読み込み中...",
		},
		"zh": {
			"spotify_smart_shuffle":     "智能随机播放",
			"spotify_download_playlist": "下载播放列表",
			"spotify_sort_loading":      "正在加载完整列表以排序...",
		},
		"ko": {
			"spotify_smart_shuffle":     "스마트 셔플",
			"spotify_download_playlist": "재생목록 다운로드",
			"spotify_sort_loading":      "정렬을 위해 전체 목록 불러오는 중...",
		},
		"vi": {
			"spotify_smart_shuffle":     "Trộn thông minh",
			"spotify_download_playlist": "Tải playlist",
			"spotify_sort_loading":      "Đang tải toàn bộ danh sách để sắp xếp...",
		},
		"nl": {
			"spotify_smart_shuffle":     "Smart Shuffle",
			"spotify_download_playlist": "Afspeellijst downloaden",
			"spotify_sort_loading":      "Volledige lijst laden om te sorteren...",
		},
		"sv": {
			"spotify_smart_shuffle":     "Smart blandning",
			"spotify_download_playlist": "Ladda ner spellista",
			"spotify_sort_loading":      "Laddar hela listan för att sortera...",
		},
		"th": {
			"spotify_smart_shuffle":     "สับเปลี่ยนอัจฉริยะ",
			"spotify_download_playlist": "ดาวน์โหลดเพลย์ลิสต์",
			"spotify_sort_loading":      "กำลังโหลดรายการทั้งหมดเพื่อจัดเรียง...",
		},
		"uk": {
			"spotify_smart_shuffle":     "Розумне перемішування",
			"spotify_download_playlist": "Завантажити список відтворення",
			"spotify_sort_loading":      "Завантаження повного списку для сортування...",
		},
		"ar": {
			"spotify_smart_shuffle":     "الخلط الذكي",
			"spotify_download_playlist": "تنزيل قائمة التشغيل",
			"spotify_sort_loading":      "جارٍ تحميل القائمة الكاملة للفرز...",
		},
		"hi": {
			"spotify_smart_shuffle":     "स्मार्ट शफ़ल",
			"spotify_download_playlist": "प्लेलिस्ट डाउनलोड करें",
			"spotify_sort_loading":      "क्रमबद्ध करने के लिए पूरी सूची लोड हो रही है...",
		},
		"zh-TW": {
			"spotify_smart_shuffle":     "智慧隨機播放",
			"spotify_download_playlist": "下載播放清單",
			"spotify_sort_loading":      "正在載入完整清單以進行排序...",
		},
	} {
		for k, v := range m {
			strs[lang][k] = v
		}
	}
}
