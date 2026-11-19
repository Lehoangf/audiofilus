# FN-Switch Daemon cho TrimUI Brick Pro (Knulli OS)

Ứng dụng độc lập chạy nền (Standalone Daemon) chuyên trách điều khiển **công tắc gạt FN switch** trên máy **TrimUI Brick Pro** chạy hệ điều hành **Knulli**.

## Tính năng chính
* **Chế độ Snapshot & Revert thông minh**:
  * Khi gạt **ON**: Hệ thống tự động chụp và lưu lại cấu hình hiện tại (Governor CPU, độ sáng màn hình, âm lượng, đèn LED) $\rightarrow$ áp dụng cấu hình tùy chỉnh (ví dụ: hạ xung CPU `powersave`, giảm sáng 25%, tắt sạch đèn LED để tiết kiệm pin).
  * Khi gạt **OFF**: Tự động **khôi phục chính xác 100% cấu hình trước đó** (trả lại độ sáng cũ, mức âm lượng cũ, bật lại đèn LED, trả lại CPU governor cũ).
* **Tích hợp tính năng tự dò tìm phần cứng (`-probe`)**:
  * Không cần mò mẫm tìm mã `/dev/input/eventX` thủ công, chỉ cần chạy `./fn-daemon -probe` và gạt cần FN, daemon sẽ tự động nhận diện và cập nhật vào `config.json`.
* **Độc lập 100%**:
  * Biên dịch tĩnh (Static arm64 binary), không phụ thuộc thư viện ngoài, không chỉnh sửa phân vùng gốc của Knulli. Cập nhật hệ điều hành thoải mái không lo mất cấu hình.

---

## Cài đặt nhanh: port kéo-thả (không cần SSH)

File `BrickProFNConfig-Knulli.zip` trong thư mục này là một **port Knulli** hoàn chỉnh. Cách dùng:

1. Giải nén file zip vào thẻ nhớ, vào thẳng:
   ```text
   /userdata/roms/ports/
   ```
   Kết quả sau khi giải nén (3 mục nằm trực tiếp trong `roms/ports/`):
   ```text
   /userdata/roms/ports/
   ├── BrickProFNConfig.sh     # launcher
   ├── gameinfo.xml            # tên port trong menu Ports
   ├── README.txt
   └── brickprofnconfig/       # fn-daemon, brick-config, config.json, ...
   ```
2. Khởi động lại EmulationStation (hoặc reboot máy), vào menu **Ports → Brick Pro FN Config**.

Lần đầu chạy, launcher tự động: cài daemon + service khởi động, tạo `config.json` mặc định
(**giữ nguyên nếu đã có**), đăng ký `fn_switch` vào `system.services`, khởi động daemon ngay
và mở giao diện cấu hình. Các lần sau chỉ mở giao diện; daemon vẫn tự chạy lúc boot.

> Nếu `/userdata/roms/ports/` đã có sẵn `gameinfo.xml` của port khác, đừng ghi đè: hãy chèn
> thêm khối `<game>...</game>` từ file `gameinfo.xml` của zip vào file có sẵn.

Log: `/userdata/system/logs/fn-service.log` và `/userdata/system/logs/fn-daemon.log`.

---

## Cấu trúc thư mục

```text
fn-switch/
├── fn-daemon        # File binary thực thi (ARM64 Linux)
├── config.json      # File cấu hình tùy chỉnh
├── port/            # Port Knulli kéo-thả -> BrickProFNConfig-Knulli.zip
│   ├── BrickProFNConfig.sh    # Launcher tự cài đặt + mở giao diện
│   ├── gameinfo.xml           # Tên/mô tả trong menu Ports
│   ├── README.txt
│   └── brickprofnconfig/      # Các file được copy vào /userdata/system/fn_switch
├── install.sh       # Script cài đặt tự động 1 bước trên Knulli
├── service-fn_switch.sh      # Dịch vụ khởi động (/userdata/system/services/fn_switch)
├── trimui-volume-attenuate   # Bản vá /usr/bin/trimui-volume-attenuate của Knulli
├── main.go          # Mã nguồn chính
├── hardware.go      # Xử lý CPU, Brightness, Volume, LED, Snapshot & Revert
├── evdev.go         # Xử lý Linux input event và chế độ dò tìm (-probe)
└── config.go        # Định nghĩa cấu hình
```

---

## Cài đặt thủ công qua SSH (cách cũ)

1. Copy thư mục `fn-switch` vào thẻ nhớ hoặc qua mạng (SFTP/Samba) vào `/userdata/system/fn_switch`.
2. Copy dịch vụ khởi động vào `/userdata/system/services/fn_switch`, rồi thêm `fn_switch` vào `system.services` trong `/userdata/system/knulli.conf` (ví dụ `system.services=ssh fn_switch`).
3. Mở terminal SSH vào máy Knulli, cấp quyền và chạy file cài đặt:
   ```bash
   cd /userdata/system/fn_switch
   chmod +x install.sh fn-daemon
   ./install.sh
   ```
4. Chạy chế độ dò tìm công tắc FN lần đầu tiên:
   ```bash
   /userdata/system/fn_switch/fn-daemon -probe
   ```
   *Màn hình sẽ yêu cầu bạn gạt cần FN qua lại một lần, sau đó tự động lưu vào `config.json`!*
5. Khởi động daemon:
   ```bash
   /userdata/system/fn_switch/fn-daemon &
   ```

---

## Tùy biến `config.json`

File nằm tại `/userdata/system/fn_switch/config.json`:

```json
{
  "device": {
    "event_path": "auto",
    "device_name": "",
    "event_type": 0,
    "event_code": -1,
    "invert": false
  },
  "audio": {
    "dac_volume": 140,
    "headphone_gain": 0
  },
  "actions": {
    "cpu": {
      "enabled": true,
      "governor_on": "powersave"
    },
    "brightness": {
      "enabled": true,
      "percent_on": 25
    },
    "volume": {
      "enabled": false,
      "mute_on": false,
      "percent_on": 0
    },
    "leds": {
      "enabled": true,
      "scale_on": 0
    },
    "custom_commands": {
      "enabled": false,
      "on_command": "",
      "off_command": ""
    }
  }
}
```

* **`cpu.enabled`**: Bật/tắt chỉnh CPU khi gạt FN. `governor_on` có thể là `powersave` hoặc `performance`.
* **`audio.dac_volume`**: Mức `DAC volume` của codec (0–255, `159` = 0 dB). Mặc định `140`,
  đúng mức device tree để lại.
* **`audio.headphone_gain`**: Mức `Headphone Volume` (0–7). Mặc định `0`: trên Brick Pro
  đây là mức làm loa trong **to** (TrimUI đảo ngược cổng tai nghe). `7` làm loa trong nhỏ đi
  rất nhiều.
* **`brightness.enabled`**: Bật/tắt chỉnh độ sáng. `percent_on` từ 0 đến 100%.
* **`volume.enabled`**: Bật/tắt can thiệp âm thanh. `mute_on: true` để tắt tiếng ngay khi gạt ON.
* **`leds.enabled`**: Bật/tắt đèn LED RGB. `scale_on: 0` là tắt hẳn đèn khi gạt ON.

---

## Nút phụ F1 / F2 / Home

Ứng dụng **Brick Pro FN Config** (tab "NÚT PHỤ") ghi vào cùng một `config.json` phần
`controls`. Daemon đọc phần này và nhận cả sự kiện `EV_KEY` từ **mọi** thiết bị input
có các phím đó (thường là `event3`, riêng Home có thể nằm ở `event0` tùy bản Knulli)
nên không cần cấu hình thêm:

```json
"controls": {
  "f1_action": "mute",
  "f2_action": "led_toggle",
  "home_action": "retroarch_menu",
  "f1_command": "",
  "f2_command": "",
  "home_command": "",
  "retroarch_keys": {
    "menu": 59,
    "quick_menu": 172,
    "save_state": 60,
    "fast_forward": 33
  }
}
```

Hành động được hỗ trợ:

| Nút | Hành động | Ý nghĩa |
|---|---|---|
| F1 | `mute`, `screenshot`, `brightness_down`, `retroarch_ff`, `custom`, `none` | Bật/tắt mute sink, chụp màn hình bằng `knulli-screenshot`, giảm sáng 10%, tua nhanh RetroArch, chạy lệnh riêng |
| F2 | `led_toggle`, `brightness_up`, `retroarch_save`, `custom`, `none` | Bật/tắt toàn bộ LED, tăng sáng 10%, lưu nhanh RetroArch, chạy lệnh riêng |
| Home | `retroarch_menu`, `exit_es`, `quick_menu`, `custom`, `none` | Mở menu RetroArch, thoát EmulationStation, mở Quick Menu, chạy lệnh riêng |

Lưu ý về các hành động RetroArch: daemon **đọc** input chứ không chiếm thiết bị, nên
nó không thể "nuốt" phím vật lý thay RetroArch. Vì vậy nó tạo một bàn phím ảo qua
`uinput` ngay lúc khởi động và tiêm phím tương ứng. Mã mặc định theo bind chuẩn của
RetroArch (`F1` = menu, `F2` = lưu, `F` = toggle fast-forward). Nếu RetroArch đã được
remap, đổi mã trong `retroarch_keys` (xem `KEY_*` trong `linux/input-event-codes.h`).
Các giá trị `0` hoặc thiếu trường được hiểu là "dùng mặc định".

Một cú bấm chỉ được xử lý một lần dù phím xuất hiện trên hai thiết bị (chống dội
180 ms). Gạt FN và bấm nút phụ chạy trên cùng một vòng lặp sự kiện, không tạo thêm
luồng đọc thiết bị nào.

---

## Âm thanh: đường ra loa trong

Trên Knulli, phần cứng âm thanh đã có sẵn người lo (`S06audio` chạy `alsactl init` rồi
bật PipeWire, `S27audioconfig` áp `audio.device`/`audio.volume`, `knulli-audio` bật
`HpSpeaker`/`Headphone`, tắt `LINEOUT` và `DAC Swap`). Vì vậy daemon **không** tự đặt lại
cả mixer: nó chỉ áp đúng công thức của Knulli cho bo a133, cộng thêm mức DAC, rồi sửa
những thứ chắc chắn gây câm.

TrimUI đã **đảo ngược cổng tai nghe** (WirePlumber ghi đúng câu đó trong
`51-builtinspeaker.conf`), nên hai control dưới đây hành xử ngược với nhãn của `amixer`.
Đo bằng tai trên máy (2026‑11‑17), mỗi phép thử chỉ đổi một control:

* **`Headphone Volume = 0`**: loa trong **to**, tai nghe vẫn to bình thường.
  `= 7` (amixer ghi 0 dB): loa trong nhỏ đi rất nhiều. Đây chính là nguyên nhân của lỗi
  "loa trong rất nhỏ": script gốc của Knulli đặt `0` khi rút tai nghe, còn profile cũ của
  daemon lại đặt `7`.
* **`digital volume = 0`** (amixer ghi −74.24 dB): mức **to nhất**. `= 63` (amixer ghi
  −1.16 dB): **câm hẳn** — đặt `63` từng làm mất tiếng hoàn toàn.

Cách gọi cũng quan trọng: `amixer sset 'Headphone Volume' 0` báo *"Unable to find simple
control"* vì tên simple element là `Headphone` (gộp cả volume + switch) và `HpSpeaker`,
không phải tên control. Vì vậy daemon (và bản vá `trimui-volume-attenuate`) dùng
`amixer cset name='Headphone Volume' 0` cho mọi control, kể cả enum (`DAC Swap`).

* **Tắt tiếng khi gạt FN ON** chỉ dùng `pactl set-sink-mute` (không đặt âm lượng về 0%),
  vì lúc tắt máy Knulli lưu `audio.volume` bằng chính mức của sink — để 0% sẽ làm lần
  khởi động sau mất tiếng hoàn toàn.
* **Lúc khởi động** daemon áp lại tầng cứng (`repairAudio`): bật chip PA
  (`/sys/class/speaker/mute` = 0), `HpSpeaker Switch` on, `Headphone Switch` on với
  `Headphone Volume` = `audio.headphone_gain`, `ADC volume 0`, `LINEOUT volume 0` (mute,
  đúng mức Knulli dùng), `DAC Swap Off`, `digital volume 0`, và `DAC volume` theo
  `audio.dac_volume`. Sau đó chọn đúng sink của codec, bỏ mute và chỉ nâng âm lượng khi
  sink đang ở 0%. Việc này lặp lại sau 10 và 30 giây.
* **`audio.dac_volume`** (mặc định `140`, tức −14.25 dB): Knulli không đặt mức này, nó giữ
  nguyên mức device tree. Ở mức đó loa trong đã đủ to khi `Headphone Volume = 0`; tăng thêm
  chỉ có nguy cơ vỡ tiếng vì PipeWire còn nhân thêm âm lượng mềm của sink (`soft-mixer`).
* **`/usr/bin/trimui-volume-attenuate`** được thay bằng `trimui-volume-attenuate` trong
  thư mục này, bind-mount đè lên bản gốc lúc khởi động. Bản vá giữ đúng mức của Knulli
  (`0` khi rút tai nghe, `3` khi cắm tai nghe), bật thêm switch để một lần mute tay không
  nằm lại, và gọi bằng `cset name=...` cho khớp tên control thật.

Kiểm tra và sửa tay (in ra sink, âm lượng, mute, toàn bộ mixer, codec và dmesg của
codec/loa/PA):

```sh
/userdata/system/fn_switch/fn-daemon -audio            # xem và sửa
/userdata/system/fn_switch/fn-daemon -audio -tone      # thêm 2 giây âm 440 Hz
/userdata/system/fn_switch/fn-daemon -audio -dac 159 -tone   # thử mức DAC khác
```

Nếu loa vẫn nhỏ, tăng dần `audio.dac_volume` trong `config.json` (ví dụ `155`, `159`) và
chạy lại lệnh trên; nếu mất tiếng, hạ về `140` và xem phần dmesg trong log.

Log nằm ở `/userdata/system/logs/fn-service.log` (dịch vụ khởi động, gồm cả bản dump
trên) và `/userdata/system/logs/fn-daemon.log` (daemon).
