package ui

// Los textos de la vista previa "Siguiente" (Up Next) en la pantalla de
// reproducción (upnext.go, menu.go), en los catorce idiomas.
func init() {
	for lang, m := range map[string]map[string]string{
		"es": {
			"set_up_next":   "Siguiente",
			"upnext_end":    "15s finales",
			"upnext_always": "Siempre",
			"upnext_label":  "SIG.",
		},
		"en": {
			"set_up_next":   "Up Next",
			"upnext_end":    "Last 15s",
			"upnext_always": "Always",
			"upnext_label":  "NEXT",
		},
		"tr": {
			"set_up_next":   "Sıradaki",
			"upnext_end":    "Son 15s",
			"upnext_always": "Her zaman",
			"upnext_label":  "SONR.",
		},
		"de": {
			"set_up_next":   "Als Nächstes",
			"upnext_end":    "Letzte 15s",
			"upnext_always": "Immer",
			"upnext_label":  "DANACH",
		},
		"fr": {
			"set_up_next":   "À suivre",
			"upnext_end":    "15s fin",
			"upnext_always": "Toujours",
			"upnext_label":  "SUIV.",
		},
		"it": {
			"set_up_next":   "Prossimo",
			"upnext_end":    "Ultimi 15s",
			"upnext_always": "Sempre",
			"upnext_label":  "PROSS.",
		},
		"pt": {
			"set_up_next":   "A seguir",
			"upnext_end":    "Últimos 15s",
			"upnext_always": "Sempre",
			"upnext_label":  "PRÓX.",
		},
		"pl": {
			"set_up_next":   "Następny",
			"upnext_end":    "Ost. 15s",
			"upnext_always": "Zawsze",
			"upnext_label":  "NAST.",
		},
		"ru": {
			"set_up_next":   "Далее",
			"upnext_end":    "Посл. 15с",
			"upnext_always": "Всегда",
			"upnext_label":  "ДАЛЕЕ",
		},
		"id": {
			"set_up_next":   "Berikutnya",
			"upnext_end":    "15d akhir",
			"upnext_always": "Selalu",
			"upnext_label":  "NEXT",
		},
		"ja": {
			"set_up_next":   "次の曲",
			"upnext_end":    "残り15秒",
			"upnext_always": "常に",
			"upnext_label":  "次へ",
		},
		"zh": {
			"set_up_next":   "下一首",
			"upnext_end":    "最后15秒",
			"upnext_always": "始终",
			"upnext_label":  "下一首",
		},
		"ko": {
			"set_up_next":   "다음 곡",
			"upnext_end":    "마지막 15초",
			"upnext_always": "항상",
			"upnext_label":  "다음",
		},
		"vi": {
			"set_up_next":   "Bài tiếp theo",
			"upnext_end":    "15s cuối",
			"upnext_always": "Luôn bật",
			"upnext_label":  "TIẾP",
		},
	} {
		for k, v := range m {
			strs[lang][k] = v
		}
	}
}
