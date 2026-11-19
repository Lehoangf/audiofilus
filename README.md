<p align="center">
  <img src="site/img/logo.png" alt="Audiofilus" width="460">
</p>

<p align="center">
  <b>A pixel-art music player for Linux handhelds, desktops, and retro gaming consoles.</b><br>
  <i>The music keeps playing in the background while you game, with pocket mode, synchronized lyrics, beat visualizers, and online streaming.</i>
</p>

<p align="center">
  <a href="https://github.com/juanvillacortac/audiofilus"><img src="https://img.shields.io/badge/Upstream-juanvillacortac%2Faudiofilus-blue?logo=github" alt="Upstream Repository"></a>
  <a href="https://portmaster.games/detail.html?name=audiofilus"><img src="https://img.shields.io/badge/PortMaster-Get%20it-ff5fa2" alt="Get it on PortMaster"></a>
  <a href="https://ko-fi.com/juanvillacortac"><img src="https://img.shields.io/badge/Ko--fi-Support%20Original%20Author-6fe3c4?logo=kofi&logoColor=white" alt="Support Juan Villacorta"></a>
  <img src="https://img.shields.io/badge/License-MIT-green" alt="MIT License">
  <img src="https://img.shields.io/badge/Go-1.24%2B-00ADD8?logo=go" alt="Go 1.24+">
  <img src="https://img.shields.io/badge/Spotify-OAuth2%20PKCE-1ED760?logo=spotify&logoColor=white" alt="Spotify Support">
</p>

<p align="center">
  <img src="site/img/player.webp" alt="Now Playing" width="360">
  <img src="site/img/vis-lake.webp" alt="Lake visualizer" width="320">
</p>

---

## 📌 Acknowledgement of Original Author / Lời cảm ơn tác giả gốc

> **Original Creator & Architecture**: This project is a community-enhanced fork of **[Audiofilus](https://github.com/juanvillacortac/audiofilus)**, designed and built by **[Juan Villacorta](https://github.com/juanvillacortac)** ([Official Website](https://juanvillacortac.github.io/audiofilus/) · [Ko-fi](https://ko-fi.com/juanvillacortac)).
> 
> All core architectural paradigms—the autonomous background audio daemon, low-power pocket mode, gapless playback, pure-Go 8-bit sound chip emulations, Chiptune Tracker, and retro pixel aesthetic—are the brilliant work of Juan Villacorta. If you enjoy this music player, please consider supporting Juan on **[Ko-fi](https://ko-fi.com/juanvillacortac)**!

---

## 🌟 What's New in this Fork / Điểm nổi bật của bản Fork

Bản fork này bổ sung các tính năng nâng cao dành riêng cho cộng đồng máy chơi game cầm tay (đặc biệt là **TrimUI Brick / Brick Pro**, **Anbernic RG34XX-SP / RG35XX**, **muOS**, **Knulli**) và hỗ trợ nghe nhạc online Spotify:

### 1. 🎵 Tích hợp Spotify Online & Giao diện Đăng nhập Pixel Art
- **Truy cập kho nhạc Spotify**: Duyệt Playlists cá nhân, Bài hát đã thích (Liked Songs), Album nghệ sĩ và Tìm kiếm bài hát.
- **Smart Shuffle**: Tự động gợi ý và chèn thông minh các bài hát tương tự theo phong cách Spotify.
- **Đăng nhập bằng mã QR & Cổng Web Pixel Art (`http://<ip>:8888/`)**:
  - Không cần nhập tài khoản/mật khẩu trực tiếp trên máy cầm tay: chỉ cần quét mã QR bằng điện thoại.
  - **Tự nhập Spotify API Key**: Cho phép người dùng nhập trực tiếp **Client ID** và **Client Secret** từ tài khoản [Spotify Developer](https://developer.spotify.com/dashboard) để sở hữu kết nối riêng biệt, ổn định, không lo giới hạn lượt gọi API.
  - Thiết kế theo phong cách **Pixel Art Arcade** đồng bộ hoàn toàn với giao diện của Audiofilus.
  - Hỗ trợ lưu cấu hình key tự động vào `data/spotify_client.json`, có nút bấm `[⚡ DÙNG KEY MẪU]` và `[✕ XÓA TRỐNG]`.
- **Bộ nhớ đệm thông minh (LRU Cache)**: Lưu bài hát ngoại tuyến vào thẻ nhớ và tải trước (prefetch) bài kế tiếp để phát nhạc mượt mà không bị gián đoạn.

### 2. 🕹️ Tối ưu hóa cho TrimUI Brick / Brick Pro & muOS
- **Hỗ trợ đa hệ điều hành**: Tương thích hoàn toàn với **muOS** (gói `.muxapp` cài đặt trực tiếp qua Archive Manager), **Knulli** (Allwinner A133P / Scarab), **PortMaster**, **ArkOS**, **ROCKNIX**, **EmuELEC**.
- **Tối ưu phím điều khiển cầm tay**: Hỗ trợ đầy đủ phím FN, phím gạt, phím media và các tổ hợp phím tắt nhanh chuyển bài, tạm dừng, Pocket Mode.
- **Tiết kiệm pin vượt trội (Pocket Mode)**: Cho phép tắt màn hình khi đang phát nhạc, tiêu thụ cực ít pin khi bỏ túi.

### 3. 🎤 Mini Lyrics, Karaoke Đồng bộ & Kho Visualizer Mở Rộng
- **Mini Lyrics Overlay**: Hiển thị lời bài hát thu nhỏ nổi trên nền khi đang xem Visualizers hoặc chơi Mini-games.
- **Hát Karaoke theo thời gian thực**: Lời bài hát đổi màu theo từng câu hát, đếm ngược nhịp dạo đầu (tích hợp tìm kiếm tự động từ [LRCLIB](https://lrclib.net)).
- **Bộ Visualizer phong phú**: Bổ sung thêm các hiệu ứng Matrix Rain, Warp Speed, NCS Circle Pulse, Nyan Cat bên cạnh MilkDrop (Winamp presets), Lake, Drift, Trench, Plasma, v.v.

### 4. 🇻🇳 Việt hóa 100% & Bộ Font chữ Pixel Tiếng Việt
- Toàn bộ giao diện, menu cài đặt và thông báo đều được dịch sang tiếng Việt chuẩn xác.
- Bộ font chữ Pixel chuyên biệt có đầy đủ dấu tiếng Việt không bị lỗi ô vuông: `Jersey 10 VN`, `Tiny5 VN`, `SVN-Retron-2000`, `dearpix`, `kvn-97`.

### 5. 🔄 Đồng bộ mã nguồn mới nhất từ Upstream (juanvillacortac/audiofilus)
- Tự động phát hiện và chuyển đổi **USB DAC** khi cắm/rút tai nghe Type-C hoặc DAC rời.
- Công nghệ dựng chữ **HarfBuzz / Silver** hỗ trợ hiển thị 21 ngôn ngữ.
- Tính năng **MusicBrainz Lookup**: Tự động tìm kiếm và sửa thông tin bài hát (tên, album, năm, ảnh bìa).
- Thuật toán nhận diện Folder-as-Album và tìm kiếm nghệ sĩ trên YouTube Music.

---

## 📥 Hướng dẫn cài đặt / Installation Guide

### Cách 1: Cài đặt trên máy cầm tay TrimUI Brick / Brick Pro

#### A. Hệ điều hành Knulli OS
1. Tải bản phát hành mới nhất hoặc file `audiofilus.aarch64`.
2. Cắm thẻ nhớ vào máy tính (hoặc kết nối qua Samba/SFTP).
3. Sao chép toàn bộ thư mục `audiofilus` vào thư mục Ports trên thẻ nhớ:
   ```
   /userdata/roms/ports/audiofilus/
   ```
4. Đảm bảo file launcher `Audiofilus.sh` nằm tại:
   ```
   /userdata/roms/ports/Audiofilus.sh
   ```
5. Lắp thẻ vào máy, cập nhật danh sách game (Update Gamelist) và mở **Audiofilus** trong mục **Ports**.

#### B. Hệ điều hành muOS
1. Tải gói `audiofilus.muxapp` từ mục Releases.
2. Sao chép file `audiofilus.muxapp` vào thư mục `ARCHIVE/` trên thẻ nhớ SD1 hoặc SD2.
3. Trên máy TrimUI Brick Pro, vào **Applications** -> **Archive Manager**, chọn `audiofilus.muxapp` và nhấn cài đặt.
4. Mở Audiofilus trong danh mục **Applications**.

---

### Cách 2: Cài đặt qua PortMaster (Anbernic, ArkOS, ROCKNIX, EmuELEC)

1. Tải file `audiofilus-portmaster.zip` từ mục Releases.
2. Giải nén vào thư mục `ports/` trên thẻ nhớ:
   - File `Audiofilus.sh` nằm trong thư mục `ports/`.
   - Thư mục `audiofilus/` chứa toàn bộ dữ liệu nằm trong `ports/audiofilus/`.
3. Khởi động lại máy hoặc cập nhật danh sách Ports, sau đó khởi chạy Audiofilus.

---

### Cách 3: Chạy trên máy tính (Windows & Linux Desktop)

#### Windows (10 / 11)
1. Tải file `audiofilus-windows-amd64.zip`.
2. Giải nén vào một thư mục bất kỳ.
3. Chạy `audiofilus.exe`. Toàn bộ thư viện giải mã âm thanh (MP3, FLAC, AAC, Opus, WMA, Chiptune) đã được đóng gói sẵn trong file zip.
4. *(Tùy chọn cho YouTube Music)* Tải `yt-dlp.exe` đặt vào thư mục `yt-dlp/` cạnh file `audiofilus.exe`.

#### Linux Desktop
```sh
# Biên dịch và chạy trực tiếp
./build.sh
./dist/audiofilus
```
Trên Linux Desktop, ứng dụng tự động tích hợp phím Media, biểu tượng khay hệ thống (System Tray) và giao thức điều khiển MPRIS.

---

## 🔑 Hướng dẫn cấu hình Spotify (Từng bước chi tiết)

Để trải nghiệm tính năng Spotify trên Audiofilus:

1. **Tạo Spotify Developer App (Miễn phí)**:
   - Truy cập **[developer.spotify.com/dashboard](https://developer.spotify.com/dashboard)** và đăng nhập tài khoản Spotify của bạn.
   - Bấm **"Create App"**:
     - *App name*: Nhập `Audiofilus` (hoặc tên tùy thích).
     - *App description*: `Retro Music Player`.
   - Tại mục **Redirect URIs**, nhấn **Add** và điền chính xác 2 đường link sau:
     ```text
     http://127.0.0.1:8888/callback
     http://127.0.0.1/callback
     ```
   - Chọn mục **Web API** và **Web Playback SDK**, đánh dấu đồng ý điều khoản rồi bấm **Save**.
   - Vào mục **Settings** của App vừa tạo, bạn sẽ thấy **Client ID** (và nút *View client secret*).

2. **Đăng nhập trên máy chơi game**:
   - Mở Audiofilus trên máy chơi game, chuyển sang tab **Spotify** (biểu tượng ♫ hoặc chọn từ thanh dock `SELECT`).
   - Nhấn nút xác nhận (`A` hoặc `B` tùy hệ máy) để hiển thị **Mã QR đăng nhập**.
   - Dùng điện thoại hoặc máy tính cùng mạng Wi-Fi quét mã QR để mở trang web cổng đăng nhập (ví dụ: `http://192.168.1.15:8888/`).

3. **Cổng Web Pixel Art**:
   - **Bước 1**: Dán **Client ID** (và Client Secret) từ trang Spotify Developer vào ô nhập trên web -> Bấm `[ 💾 LƯU CẤU HÌNH ]`.
   - **Bước 2**: Nhấn nút `[ ▶ 1. MỞ TRANG SPOTIFY (TAB MỚI) ◀ ]`. Đăng nhập tài khoản Spotify của bạn và nhấn **"Đồng ý" (Agree)**.
   - **Bước 3**: Sau khi nhấn Agree, tab mới sẽ hiển thị trang báo lỗi kết nối (vì điện thoại không mở cổng 8888). Hãy **sao chép toàn bộ đường link trên thanh địa chỉ** của tab đó (có dạng `http://127.0.0.1:8888/callback?code=...`) rồi dán vào ô ở Bước 3 -> Nhấn nút `[ ★ 2. XÁC NHẬN MÃ & KẾT NỐI ★ ]`.
   - Trang web sẽ hiển thị thông báo thành công `★ SPOTIFY CONNECTED ★`. Bạn có thể đóng tab và thưởng thức âm nhạc ngay trên máy chơi game!

---

## 🛠️ Biên dịch từ mã nguồn / Building from Source

Yêu cầu: Đã cài đặt **Go 1.24+**. Quá trình biên dịch cho máy cầm tay **không sử dụng cgo** (`CGO_ENABLED=0`), giúp binary cực kỳ nhẹ và tương thích tốt trên mọi bản phân phối Linux ARM64.

### Biên dịch nhị phân Linux ARM64 (Knulli, muOS, PortMaster):
```bash
# Biên dịch binary aarch64 tối ưu hóa kích thước
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w -X github.com/juanvillacortac/audiofilus/internal/version.Version=dev" -o dist/audiofilus.aarch64 ./cmd/audiofilus
```

### Đóng gói file cài đặt PortMaster:
```bash
go run ./tools/pack_port
# File nén zip hoàn chỉnh sẽ được tạo tại dist/audiofilus-portmaster.zip
```

### Chạy kiểm thử toàn bộ dự án:
```bash
# Kiểm tra định dạng mã nguồn
gofmt -l .

# Chạy kiểm thử tĩnh
GOOS=windows go vet ./...
go vet ./...

# Chạy bộ unit test
go test ./...
```

---

## 🎮 Phím điều khiển cơ bản / Controls

| Thao tác | Nút bấm |
|---|---|
| **Xem Dock chuyển màn hình** | `SELECT` |
| **Menu tác vụ bổ sung (More)** | `START` |
| **Phát / Tạm dừng** | `Y` (hoặc `X` tùy layout) / `FN` shortcut |
| **Chuyển bài kế tiếp** | `R1` |
| **Quay lại bài trước** | `L1` |
| **Tua nhanh / Tua lùi** | Giữ `←` / `→` (hoặc phím Triggers `L2` / `R2`) |
| **Chế độ màn hình tối (Pocket Mode)** | Nhấn đúp `START` hoặc bấm nút Nguồn |
| **Chà đĩa vinyl (Scratch)** | `↑` / `↓` trên màn hình Now Playing |
| **Xem bảng trợ giúp phím** | Giữ nút `FN` |

---

## 📜 Giấy phép & Bản quyền / License

Dự án phát hành dưới giấy phép mã nguồn mở **[MIT License](LICENSE)**.

- **Bản quyền gốc (Original Work)**: © Juan Villacorta ([juanvillacortac/audiofilus](https://github.com/juanvillacortac/audiofilus)).
- **Bản fork cộng đồng (Community Fork)**: © Lehoangf ([Lehoangf/audiofilus](https://github.com/Lehoangf/audiofilus)).
- Các bộ font chữ đi kèm:
  - `Jersey 10`, `Tiny5`, `Fusion Pixel` theo giấy phép SIL Open Font License (`OFL-*.txt`).
  - Các font chữ Việt hóa thuộc bản quyền của các tác giả tương ứng và được sử dụng cho mục đích phi thương mại.
- Các thư viện giải mã độc lập: `libgme` (LGPL), `libvgm` (GPL-2.0) được tải động tại runtime qua `purego`, đảm bảo mã nguồn Audiofilus luôn tuân thủ giấy phép MIT thuần túy.
