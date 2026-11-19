import re
import os

new_translations = {
    "en": {
        "set_splash": "Splash Screen",
        "splash_preview": "Preview",
        "splash_color": "Logo Color",
        "splash_col_gold": "Classic Gold",
        "splash_col_now": "Now Playing Tone",
        "splash_col_theme": "Theme Accent",
        "splash_col_cyber": "Cyberpunk Neon",
        "splash_col_matrix": "Matrix Green",
        "splash_col_amber": "Retro Amber",
        "splash_col_silver": "Silver Chrome",
        "splash_word": "Logo text",
        "splash_by": "Signature",
        "splash_img": "Custom image",
        "splash_clear_img": "Remove image",
        "splash_cleared": "Custom splash image removed",
        "splash_always": "Always show on launch",
        "splash_reset": "Reset to default",
        "splash_reset_done": "Splash reset to default",
        "set_sleeptimer": "Sleep Timer",
        "spotify_recent": "Recent",
        "ytm_tab_search": "Search",
        "ytm_tab_trending": "Trending",
        "ytm_tab_charts": "Top Charts",
        "ytm_tab_genres": "Genres & Moods",
        "ytm_tab_recent": "Recent",
        "ytm_loading_trending": "Loading trending songs...",
        "ytm_loading_charts": "Loading charts...",
        "ytm_no_bin": "Place yt-dlp next to audiofilus to enable YouTube Music",
        "ytm_search_q": "Search YouTube Music",
        "ytm_searching": "Searching YouTube Music...",
        "ytm_no_results": "No results found",
        "ytm_search_prompt": "Press A to search songs on YouTube",
        "ytm_no_trending": "Could not load trending songs",
        "ytm_no_recent": "No recent YouTube tracks",
        "sf_spotify": "Spotify",
    },
    "es": {
        "set_splash": "Pantalla de inicio",
        "splash_preview": "Vista previa",
        "splash_color": "Color del logo",
        "splash_col_gold": "Oro clásico",
        "splash_col_now": "Tono de reproducción",
        "splash_col_theme": "Color del tema",
        "splash_col_cyber": "Neón Cyberpunk",
        "splash_col_matrix": "Verde Matrix",
        "splash_col_amber": "Ámbar CRT",
        "splash_col_silver": "Plata cromada",
        "splash_word": "Texto del logo",
        "splash_by": "Firma",
        "splash_img": "Imagen personalizada",
        "splash_clear_img": "Quitar imagen",
        "splash_cleared": "Imagen personalizada quitada",
        "splash_always": "Mostrar siempre al abrir",
        "splash_reset": "Restablecer por defecto",
        "splash_reset_done": "Splash restablecido",
        "set_sleeptimer": "Temporizador de apagado",
        "spotify_recent": "Recientes",
        "ytm_tab_search": "Buscar",
        "ytm_tab_trending": "Tendencias",
        "ytm_tab_charts": "Top / Listas",
        "ytm_tab_genres": "Géneros y estados",
        "ytm_tab_recent": "Recientes",
        "ytm_loading_trending": "Cargando canciones en tendencia...",
        "ytm_loading_charts": "Cargando listas top...",
        "ytm_no_bin": "Coloca yt-dlp junto a audiofilus para activar YouTube Music",
        "ytm_search_q": "Buscar en YouTube Music",
        "ytm_searching": "Buscando en YouTube Music...",
        "ytm_no_results": "Sin resultados",
        "ytm_search_prompt": "Pulsa A para buscar en YouTube",
        "ytm_no_trending": "No se pudieron cargar tendencias",
        "ytm_no_recent": "Sin canciones recientes de YouTube",
        "sf_spotify": "Spotify",
    },
    "vi": {
        "set_splash": "Màn hình Splash",
        "splash_preview": "Xem thử ngay",
        "splash_color": "Màu sắc logo",
        "splash_col_gold": "Vàng cổ điển",
        "splash_col_now": "Theo bài hát đang phát",
        "splash_col_theme": "Màu chủ đề",
        "splash_col_cyber": "Neon Cyberpunk",
        "splash_col_matrix": "Xanh Matrix",
        "splash_col_amber": "Hổ phách CRT",
        "splash_col_silver": "Bạc Chrome",
        "splash_word": "Chữ Logo",
        "splash_by": "Chữ ký",
        "splash_img": "Ảnh tùy biến",
        "splash_clear_img": "Xóa hình nền",
        "splash_cleared": "Đã xóa hình nền tự chọn",
        "splash_always": "Luôn hiện khi mở app",
        "splash_reset": "Khôi phục mặc định",
        "splash_reset_done": "Đã khôi phục mặc định",
        "set_sleeptimer": "Hẹn giờ tắt",
        "spotify_recent": "Gần đây",
        "ytm_tab_search": "Tìm kiếm",
        "ytm_tab_trending": "Thịnh hành",
        "ytm_tab_charts": "Bảng xếp hạng",
        "ytm_tab_genres": "Thể loại & Tâm trạng",
        "ytm_tab_recent": "Gần đây",
        "ytm_loading_trending": "Đang tải bài hát thịnh hành...",
        "ytm_loading_charts": "Đang tải bảng xếp hạng...",
        "ytm_no_bin": "Đặt yt-dlp cạnh audiofilus để bật YouTube Music",
        "ytm_search_q": "Tìm kiếm YouTube Music",
        "ytm_searching": "Đang tìm trên YouTube Music...",
        "ytm_no_results": "Không có kết quả",
        "ytm_search_prompt": "Bấm A để tìm kiếm trên YouTube",
        "ytm_no_trending": "Không thể tải bài hát thịnh hành",
        "ytm_no_recent": "Chưa có bài hát YouTube gần đây",
        "sf_spotify": "Spotify",
    },
    "tr": {
        "set_splash": "Açılış Ekranı",
        "splash_preview": "Önizleme",
        "splash_color": "Logo Rengi",
        "splash_col_gold": "Klasik Altın",
        "splash_col_now": "Çalınan Parça Tonu",
        "splash_col_theme": "Tema Rengi",
        "splash_col_cyber": "Siberpunk Neon",
        "splash_col_matrix": "Matrix Yeşili",
        "splash_col_amber": "Retro Kehribar",
        "splash_col_silver": "Gümüş Krom",
        "splash_word": "Logo yazısı",
        "splash_by": "İmza",
        "splash_img": "Özel görsel",
        "splash_clear_img": "Görseli kaldır",
        "splash_cleared": "Özel görsel kaldırıldı",
        "splash_always": "Her açılışta göster",
        "splash_reset": "Varsayılana sıfırla",
        "splash_reset_done": "Açılış sıfırlandı",
        "set_sleeptimer": "Uyku Zamanlayıcı",
        "spotify_recent": "Son Çalınanlar",
        "ytm_tab_search": "Ara",
        "ytm_tab_trending": "Trendler",
        "ytm_tab_charts": "En İyiler",
        "ytm_tab_genres": "Türler ve Ruh Halleri",
        "ytm_tab_recent": "Son Çalınanlar",
        "ytm_loading_trending": "Trend şarkılar yükleniyor...",
        "ytm_loading_charts": "Listeler yükleniyor...",
        "ytm_no_bin": "YouTube Music için yt-dlp dosyasını audiofilus yanına koyun",
        "ytm_search_q": "YouTube Music'te ara",
        "ytm_searching": "YouTube Music aranıyor...",
        "ytm_no_results": "Sonuç bulunamadı",
        "ytm_search_prompt": "Aramak için A'ya basın",
        "ytm_no_trending": "Trend şarkılar yüklenemedi",
        "ytm_no_recent": "Son parça yok",
        "sf_spotify": "Spotify",
    }
}

# Standalone files
for code in ["de", "fr", "id", "it", "ja", "ko", "pl", "pt", "ru", "zh", "vi"]:
    path = f"internal/ui/lang_{code}.go"
    if not os.path.exists(path):
        continue
    with open(path, "r", encoding="utf-8") as f:
        content = f.read()

    existing_keys = set(re.findall(r'"([^"]+)":', content))
    to_add = []
    trans_map = new_translations.get(code, new_translations["en"])
    for k in new_translations["en"]:
        if k not in existing_keys:
            val = trans_map.get(k, new_translations["en"][k])
            escaped_v = val.replace('"', '\\"')
            to_add.append(f'\t\t\t"{k}": "{escaped_v}",')

    if to_add:
        idx = content.rfind("\t}\n}")
        if idx != -1:
            addition = "\n" + "\n".join(to_add) + "\n"
            content = content[:idx] + addition + content[idx:]
            with open(path, "w", encoding="utf-8", newline="") as f:
                f.write(content)
            print(f"Updated {path} with {len(to_add)} keys")

# lang.go with es, en, tr
with open("internal/ui/lang.go", "r", encoding="utf-8") as f:
    content = f.read()

for code in ["es", "en", "tr"]:
    trans_map = new_translations[code]
    sec_match = re.search(r'(\t"' + code + r'": \{)(.*?)(\n\t\},)', content, re.DOTALL)
    if not sec_match:
        print(f"Section {code} not found in lang.go")
        continue
    sec_content = sec_match.group(2)
    existing_keys = set(re.findall(r'"([^"]+)":', sec_content))
    to_add = []
    for k in new_translations["en"]:
        if k not in existing_keys:
            val = trans_map.get(k, new_translations["en"][k])
            escaped_v = val.replace('"', '\\"')
            to_add.append(f'\t\t\t"{k}": "{escaped_v}",')
    if to_add:
        insert_pos = sec_match.start(3)
        addition = "\n" + "\n".join(to_add)
        content = content[:insert_pos] + addition + content[insert_pos:]
        print(f"Added {len(to_add)} keys to {code} in lang.go")

with open("internal/ui/lang.go", "w", encoding="utf-8", newline="") as f:
    f.write(content)

print("Done syncing language keys with tools/sync_langs2.py")
