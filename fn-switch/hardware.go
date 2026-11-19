package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// HardwareSnapshot lưu lại trạng thái trước khi bật FN switch.
type HardwareSnapshot struct {
	Governor   string
	Brightness int
	Volume     int
	LEDState   map[string]string // Lưu toàn bộ giá trị led_anim và sunxi_leds
}

// HardwareManager quản lý đọc và ghi các thông số phần cứng.
type HardwareManager struct {
	mu        sync.Mutex
	snapshot  *HardwareSnapshot
	isON      bool
	ledsOn    bool         // trạng thái đèn hiện tại, dùng cho hành động led_toggle
	lastPress [3]time.Time // thời điểm bấm gần nhất của từng nút, chống dội phím
	cfg       *Config      // để các lần kiểm tra lại dùng đúng mức âm thanh đã cấu hình
}

// --- ÂM THANH ---
//
// Trên Knulli phần cứng âm thanh đã có sẵn người lo: S06audio chạy `alsactl init`
// rồi bật PipeWire, S27audioconfig áp audio.device/audio.volume, và knulli-audio
// (nhánh a133) bật HpSpeaker/Headphone, đặt gain tai nghe, tắt LINEOUT và tắt DAC
// Swap. Vì vậy daemon chỉ chạm vào đúng những gì có thể gây câm, làm theo đúng
// công thức của Knulli, và ghi log lại kết quả để lần sau còn biết nó hỏng ở đâu.
//
// Đường ra loa trong của Brick Pro (a133) là: DAC -> Headphone (gain 0-7) ->
// HpSpeaker -> chip PA (chân PH6). LINEOUT là đường line-out riêng và Knulli luôn
// để volume 0 (mute).
//
// TrimUI đã đảo ngược cổng tai nghe (WirePlumber ghi đúng câu đó trong
// 51-builtinspeaker.conf), nên hai control dưới đây hành xử ngược với nhãn của
// amixer. Đo bằng tai trên máy (2026-11-17), mỗi phép thử đổi đúng một control:
//
//   - "Headphone Volume" = 0 thì loa trong TO, tai nghe vẫn to bình thường;
//     = 7 (amixer ghi 0 dB) thì loa trong nhỏ đi rất nhiều. Đây chính là thứ làm
//     loa trong "rất nhỏ" suốt nhiều lần khởi động: script gốc của Knulli
//     (trimui-volume-attenuate) đặt 0 khi rút tai nghe, còn bản vá của daemon và
//     profile dưới đây lại đặt 7.
//   - "digital volume" = 0 (amixer ghi -74.24 dB) mới là mức to nhất; = 63
//     (amixer ghi -1.16 dB) là câm hẳn. Đặt 63 từng làm mất tiếng hoàn toàn.
//
// DAC volume thì Knulli không đặt, nó giữ mức 140 (-14.25 dB) của device tree. Loa
// đã đủ to ở mức đó khi Headphone Volume = 0, nên daemon chỉ đặt lại đúng 140 trừ
// khi người dùng đổi audio.dac_volume: tăng thêm chỉ có nguy cơ vỡ tiếng vì
// PipeWire còn nhân thêm âm lượng mềm của sink.

// speakerMutePath là thuộc tính của driver sunxi-leds: ghi 0 thì driver gọi
// sunxi_spk_pa_ctrl(1) (bật chip PA), ghi 1 thì gọi sunxi_spk_pa_ctrl(0) (force mute).
// Đã đối chiếu với phần dịch ngược hàm store của sunxi-leds trong boot.img.
const speakerMutePath = "/sys/class/speaker/mute"

// codecSinkPrefix là tên sink PipeWire của codec nội bộ (sound card 0).
const codecSinkPrefix = "alsa_output._sys_devices_platform_soc_sndcodec"

// attenuatePath là bản vá cho /usr/bin/trimui-volume-attenuate của Knulli.
const attenuatePath = "/userdata/system/fn_switch/trimui-volume-attenuate"

// Các mức phần cứng mặc định, dùng khi config.json không có mục "audio".
const (
	// defaultDACVolume là mức "DAC volume" của device tree (140, -14.25 dB):
	// mức nhà sản xuất đặt, đủ to khi Headphone Volume = 0.
	defaultDACVolume = 140
	// defaultHeadphoneGain là mức "Headphone Volume" (0-7) làm loa trong to trên
	// máy này; tai nghe vẫn to bình thường ở mức 0 (xem chú thích đầu mục).
	defaultHeadphoneGain = 0
	// speakerDigitalVolume là mức "digital volume" (0-63) to nhất. Amixer ghi 0
	// là -74.24 dB nhưng thực tế ngược lại: 63 mới là câm.
	speakerDigitalVolume = 0
)

// runCmd chạy một lệnh và ghi log khi thất bại: stderr của amixer/pactl nói rõ lý do
// (control không tồn tại, sai cú pháp, không có sink...), im lặng nuốt lỗi là cách
// nhanh nhất để có một cái loa câm mà không ai biết vì sao.
func runCmd(name string, args ...string) bool {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		log.Printf("[AUDIO] %s %s lỗi: %v (%s)", name, strings.Join(args, " "),
			err, strings.TrimSpace(string(out)))
		return false
	}
	return true
}

// setSpeakerPower bật chip khuếch đại công suất (PA) của loa trong.
func setSpeakerPower(on bool) {
	val := "0"
	if !on {
		val = "1"
	}
	if err := os.WriteFile(speakerMutePath, []byte(val+"\n"), 0200); err != nil {
		log.Printf("[AUDIO] Không ghi được %s: %v", speakerMutePath, err)
	}
}

// ensureAttenuateOverride gắn bản vá lên /usr/bin/trimui-volume-attenuate. Script
// gốc của Knulli đặt "Headphone 0" khi rút tai nghe, đúng mức làm loa trong to trên
// máy này; bản vá chỉ thêm "unmute" để một lần mute tay không nằm lại. Nếu không
// tìm thấy script thì thôi: profile mixer của daemon đã đặt lại mức đúng.
func ensureAttenuateOverride() {
	if _, err := os.Stat(attenuatePath); err != nil {
		return
	}
	if mounts, err := os.ReadFile("/proc/mounts"); err == nil &&
		strings.Contains(string(mounts), "/usr/bin/trimui-volume-attenuate") {
		return // đã gắn rồi (service gắn lúc khởi động)
	}
	if runCmd("mount", "--bind", attenuatePath, "/usr/bin/trimui-volume-attenuate") {
		log.Printf("[AUDIO] Đã gắn bản vá trimui-volume-attenuate (bind mount)")
	}
}

// listSinks trả về tên các sink PipeWire đang có.
func listSinks() []string {
	out, err := exec.Command("pactl", "list", "short", "sinks").Output()
	if err != nil {
		log.Printf("[AUDIO] Không liệt kê được sink: %v", err)
		return nil
	}
	var sinks []string
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			sinks = append(sinks, fields[1])
		}
	}
	return sinks
}

// defaultSink trả về tên sink mặc định hiện tại (rỗng nếu chưa có).
func defaultSink() string {
	out, err := exec.Command("pactl", "info").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "Default Sink:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Default Sink:"))
		}
	}
	return ""
}

// sinkVolume đọc âm lượng phần trăm của một sink (-1 nếu không đọc được).
func sinkVolume(sink string) int {
	out, err := exec.Command("pactl", "get-sink-volume", sink).Output()
	if err != nil {
		return -1
	}
	for _, f := range strings.Fields(string(out)) {
		if strings.HasSuffix(f, "%") {
			if v, err := strconv.Atoi(strings.TrimSuffix(f, "%")); err == nil {
				return v
			}
		}
	}
	return -1
}

// sinkMuted cho biết sink có đang bị mute phần mềm hay không.
func sinkMuted(sink string) bool {
	out, err := exec.Command("pactl", "get-sink-mute", sink).Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "Mute: yes")
}

// configVolume đọc audio.volume trong knulli.conf (0 nếu không có).
func configVolume() int {
	for _, conf := range []string{"/userdata/system/knulli.conf", "/userdata/system/batocera.conf"} {
		data, err := os.ReadFile(conf)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "audio.volume=") {
				if v, err := strconv.Atoi(strings.TrimPrefix(line, "audio.volume=")); err == nil {
					return v
				}
			}
		}
	}
	return 0
}

// sinkState trả về trạng thái của sink theo pactl (RUNNING, IDLE, SUSPENDED...).
func sinkState(sink string) string {
	out, err := exec.Command("pactl", "list", "short", "sinks").Output()
	if err != nil {
		return "?"
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 5 && fields[1] == sink {
			return fields[len(fields)-1]
		}
	}
	return "?"
}

// dumpAudioState ghi ra toàn bộ trạng thái âm thanh: sink của PipeWire (kể cả
// route và trạng thái), các mức ALSA và DAPM của codec, cùng những dòng dmesg nói
// về codec/loa/PA. Khi loa câm mà mixer trông vẫn đúng thì đây là bằng chứng duy
// nhất còn lại để biết nó câm ở tầng nào.
func dumpAudioState(cfg *Config) {
	dac, gain := audioSettings(cfg)
	fmt.Printf("Mức sẽ áp:        DAC volume=%d, Headphone=%d/7 (từ config.json)\n", dac, gain)
	fmt.Printf("Sink mặc định:    %q\n", defaultSink())
	for _, s := range listSinks() {
		fmt.Printf("  sink %-70s %3d%% mute=%v state=%s\n",
			s, sinkVolume(s), sinkMuted(s), sinkState(s))
	}
	if out, err := exec.Command("pactl", "list", "sinks").Output(); err == nil {
		fmt.Printf("--- pactl list sinks ---\n%s\n", out)
	}
	for _, control := range []string{
		"Headphone Switch", "Headphone Volume", "HpSpeaker Switch",
		"DAC volume", "digital volume",
		"LINEOUT Switch", "LINEOUT volume", "LINEOUT Output Select", "DAC Swap",
		"ADC volume",
	} {
		fmt.Printf("%-22s %s\n", control+":", mixerValue(control))
	}
	fmt.Printf("Speaker PA mute:  %s (0 = bật)\n", speakerMuteState())
	fmt.Printf("audio.volume:     %d%%\n", configVolume())
	if out, err := exec.Command("amixer", "-c", "0", "contents").Output(); err == nil {
		fmt.Printf("--- amixer contents ---\n%s\n", out)
	}
	if data, err := os.ReadFile("/proc/asound/card0/codec#0"); err == nil {
		fmt.Printf("--- codec#0 ---\n%s\n", data)
	}
	fmt.Printf("--- dmesg (codec/loa/PA) ---\n%s\n", dmesgAudio())
}

// dmesgAudio lọc những dòng dmesg nói về codec, loa và chip PA: đó là nơi driver
// sunxi báo "speaker force mute", "hp_detect_case" hay lỗi đọc GPIO của amp.
func dmesgAudio() string {
	out, err := exec.Command("dmesg").Output()
	if err != nil {
		return "(không đọc được dmesg: " + err.Error() + ")"
	}
	var keep []string
	for _, line := range strings.Split(string(out), "\n") {
		l := strings.ToLower(line)
		if strings.Contains(l, "codec") || strings.Contains(l, "speaker") ||
			strings.Contains(l, "spk") || strings.Contains(l, "audio") ||
			strings.Contains(l, "hp_detect") || strings.Contains(l, "jack") {
			keep = append(keep, line)
		}
	}
	if len(keep) == 0 {
		return "(không có dòng nào)"
	}
	return strings.Join(keep, "\n")
}

// audioSettings trả về mức DAC và gain tai nghe từ cấu hình, kẹp trong giới hạn
// thật của codec để một con số sai trong config.json không làm câm loa.
func audioSettings(cfg *Config) (dac, gain int) {
	dac, gain = defaultDACVolume, defaultHeadphoneGain
	if cfg != nil {
		if cfg.Audio.DACVolume > 0 {
			dac = cfg.Audio.DACVolume
		}
		// 0 là mức hợp lệ (và là mức đúng trên máy này), nên chỉ loại số ngoài
		// khoảng của control; thiếu mục trong config.json cũng ra 0 và trùng mặc định.
		if cfg.Audio.HeadphoneGain >= 0 && cfg.Audio.HeadphoneGain <= 7 {
			gain = cfg.Audio.HeadphoneGain
		}
	}
	if dac < 1 {
		dac = 1
	} else if dac > 255 {
		dac = 255
	}
	if gain < 0 {
		gain = 0
	} else if gain > 7 {
		gain = 7
	}
	return dac, gain
}

// cset đặt một control theo tên của Ctl (amixer cset name=...). Phải dùng cset chứ
// không phải sset: "sset" chỉ thấy các "simple element", mà tên element lại khác tên
// control ("Headphone Volume" nằm trong element "Headphone", "HpSpeaker Switch" trong
// "HpSpeaker"), nên "sset 'Headphone Volume' 0" báo "Unable to find simple control".
// Không ghi log ở đây vì hàm này còn dùng để thử nghiệm.
func cset(control, value string) bool {
	return exec.Command("amixer", "-q", "-c", "0", "cset", "name="+control, value).Run() == nil
}

// mixerInt đọc giá trị số đầu tiên của một control ALSA (-1 nếu không đọc được).
// "cget name=" đọc được cả control không phải simple element lẫn enum; "sget" chỉ
// còn là đường dự phòng cho những bản amixer cũ.
func mixerInt(control string) int {
	if out, err := exec.Command("amixer", "-c", "0", "cget", "name="+control).Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, ": values=") {
				continue
			}
			first := strings.Split(strings.TrimPrefix(line, ": values="), ",")[0]
			if v, err := strconv.Atoi(strings.TrimSpace(first)); err == nil {
				return v
			}
		}
	}
	out, err := exec.Command("amixer", "-c", "0", "sget", control).Output()
	if err != nil {
		return -1
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Mono:") && !strings.HasPrefix(line, "Front Left:") {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(strings.TrimPrefix(line, "Mono:"), "Front Left:"))
		if len(fields) == 0 {
			continue
		}
		if v, err := strconv.Atoi(fields[0]); err == nil {
			return v
		}
	}
	return -1
}

// setMixerInt đặt một control âm lượng số và đọc lại để chắc chắn nó đã vào. Control
// hai kênh (DAC volume) cần giá trị cho cả hai kênh, nên nếu dạng một giá trị bị từ
// chối thì thử lại với "v,v". Trả về false nếu vẫn không khớp, để log nói rõ.
func setMixerInt(control string, value int) bool {
	if mixerInt(control) == value {
		return true
	}
	v := strconv.Itoa(value)
	if !cset(control, v) {
		cset(control, v+","+v)
	}
	if got := mixerInt(control); got != value {
		log.Printf("[AUDIO] %q không nhận %d (đang là %d)", control, value, got)
		return false
	}
	return true
}

// setMixerSwitch bật/tắt một control kiểu công tắc và ghi log nếu không đặt được.
func setMixerSwitch(control string, on bool) {
	value := "off"
	if on {
		value = "on"
	}
	if !cset(control, value) {
		log.Printf("[AUDIO] Không đặt được %q = %s", control, value)
	}
}

// applyMixerProfile áp lại đúng công thức của knulli-audio cho bo a133, cộng thêm
// mức DAC của người dùng. Đây là toàn bộ những gì tầng cứng cần để loa trong kêu.
// Control được gọi bằng tên đầy đủ ("HpSpeaker Switch") thay vì tiền tố như bản gốc
// của Knulli: tiền tố khớp nhiều control thì amixer tự chọn, và chọn nhầm trong im
// lặng là đúng kiểu sự cố đang phải truy.
func applyMixerProfile(cfg *Config) {
	dac, gain := audioSettings(cfg)

	// 1. Chip PA (chân PH6) và công tắc loa trong.
	setSpeakerPower(true)
	setMixerSwitch("HpSpeaker Switch", true)

	// 2. Công tắc tai nghe và gain analog: trên máy này 0 mới là mức làm loa trong
	// to (ngược với nhãn của amixer, xem chú thích đầu mục).
	setMixerSwitch("Headphone Switch", true)
	setMixerInt("Headphone Volume", gain)

	// 3. Những gì Knulli đặt mà một lần chỉnh tay có thể để lại sai.
	setMixerInt("ADC volume", 0)
	// LINEOUT là đường line-out riêng; Knulli luôn để nó mute, và mọi lần chỉnh
	// tay bật nó lên đều là thay đổi thừa đối với loa trong.
	setMixerInt("LINEOUT volume", 0)
	// "DAC Swap" là enum (Off/On), không phải công tắc, nên phải gọi bằng tên item.
	if !cset("DAC Swap", "Off") {
		log.Printf("[AUDIO] Không đặt được %q = Off", "DAC Swap")
	}

	// 4. Mức số: digital volume ở mức to nhất (0, không phải 63), DAC theo cấu hình.
	setMixerInt("digital volume", speakerDigitalVolume)
	setMixerInt("DAC volume", dac)
}

// repairAudio bảo đảm loa trong có tiếng mà không đổi ý người dùng: áp lại tầng
// cứng theo công thức của Knulli, rồi chỉ sửa những gì chắc chắn gây câm ở tầng
// phần mềm (sai sink, sink mute, âm lượng 0%).
func repairAudio(cfg *Config) {
	ensureAttenuateOverride()
	applyMixerProfile(cfg)

	sinks := listSinks()
	if len(sinks) == 0 {
		log.Printf("[AUDIO] Chưa có sink PipeWire nào (PipeWire chưa chạy?)")
		return
	}
	sink := ""
	for _, s := range sinks {
		if strings.HasPrefix(s, codecSinkPrefix) {
			sink = s
			break
		}
	}
	if sink == "" {
		sink = sinks[0]
		log.Printf("[AUDIO] Không thấy sink của codec, dùng tạm %s", sink)
	}
	if cur := defaultSink(); cur != sink {
		runCmd("pactl", "set-default-sink", sink)
		log.Printf("[AUDIO] Sink mặc định: %q -> %s", cur, sink)
	}
	runCmd("pactl", "set-sink-mute", sink, "0")
	if vol := sinkVolume(sink); vol <= 0 {
		vol = configVolume()
		if vol <= 0 {
			vol = 70
		}
		runCmd("pactl", "set-sink-volume", sink, fmt.Sprintf("%d%%", vol))
		log.Printf("[AUDIO] Sink đang ở 0%%, đặt lại %d%%", vol)
	}
	// Đọc lại từ mixer chứ không lấy từ cấu hình: nếu amixer không đặt được thì
	// log phải nói đúng con số đang có, đó là manh mối duy nhất còn lại.
	log.Printf("[AUDIO] Sink=%s, âm lượng=%d%%, mute=%v, Headphone=%d/7, "+
		"digital=%d, DAC=%d, LINEOUT=0, PA=%s",
		sink, sinkVolume(sink), sinkMuted(sink), mixerInt("Headphone Volume"),
		mixerInt("digital volume"), mixerInt("DAC volume"), speakerMuteState())
}

// playTestTone phát hai giây âm 440 Hz qua paplay, để kiểm tra loa mà không cần
// mở nhạc. WAV được tạo ngay trong /tmp: 48 kHz, 16-bit, hai kênh, biên độ 35%
// để không bị vỡ tiếng khi DAC đang ở mức cao.
func playTestTone() bool {
	path := filepath.Join(os.TempDir(), "fn-tone.wav")
	if err := writeToneWAV(path, 440, 2*time.Second, 0.35); err != nil {
		log.Printf("[AUDIO] Không tạo được âm thử: %v", err)
		return false
	}
	defer os.Remove(path)
	if runCmd("paplay", path) {
		log.Printf("[AUDIO] Đã phát âm thử 440 Hz")
		return true
	}
	// paplay thiếu hoặc không nối được PipeWire: thử thẳng ALSA.
	if runCmd("aplay", "-q", path) {
		log.Printf("[AUDIO] Đã phát âm thử 440 Hz (aplay)")
		return true
	}
	return false
}

// writeToneWAV ghi một file WAV PCM 16-bit stereo 48 kHz với âm sin đơn giản.
func writeToneWAV(path string, freq int, dur time.Duration, amp float64) error {
	const rate = 48000
	frames := int(dur.Seconds() * rate)
	data := make([]byte, frames*4)
	for i := 0; i < frames; i++ {
		s := int16(amp * 32767 * math.Sin(2*math.Pi*float64(freq)*float64(i)/rate))
		binary.LittleEndian.PutUint16(data[i*4:], uint16(s))
		binary.LittleEndian.PutUint16(data[i*4+2:], uint16(s))
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	header := make([]byte, 44)
	copy(header[0:], "RIFF")
	binary.LittleEndian.PutUint32(header[4:], uint32(36+len(data)))
	copy(header[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(header[16:], 16)     // kích thước khối fmt
	binary.LittleEndian.PutUint16(header[20:], 1)      // PCM
	binary.LittleEndian.PutUint16(header[22:], 2)      // hai kênh
	binary.LittleEndian.PutUint32(header[24:], rate)   // tần số lấy mẫu
	binary.LittleEndian.PutUint32(header[28:], rate*4) // byte mỗi giây
	binary.LittleEndian.PutUint16(header[32:], 4)      // byte mỗi khung
	binary.LittleEndian.PutUint16(header[34:], 16)     // bit mỗi mẫu
	copy(header[36:], "data")
	binary.LittleEndian.PutUint32(header[40:], uint32(len(data)))
	if _, err := f.Write(header); err != nil {
		return err
	}
	_, err = f.Write(data)
	return err
}

// mixerValue trả về dòng giá trị của một control ALSA, để log còn biết tầng cứng
// đang ở đâu khi có sự cố (trả về "?" nếu control không tồn tại). Control kiểu enum
// ("DAC Swap") không phải simple control nên "sget" không thấy; "cget name=" thì
// đọc được cả hai, nên thử nó trước và lấy phần sau "values=".
func mixerValue(control string) string {
	if out, err := exec.Command("amixer", "-c", "0", "cget", "name="+control).Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, ": values=") {
				return strings.TrimSpace(strings.TrimPrefix(line, ": values="))
			}
		}
	}
	out, err := exec.Command("amixer", "-c", "0", "sget", control).Output()
	if err != nil {
		return "?"
	}
	var vals []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Mono:") || strings.HasPrefix(line, "Front Left:") ||
			strings.HasPrefix(line, "Playback:") {
			vals = append(vals, line)
		}
	}
	return strings.Join(vals, " | ")
}

// speakerMuteState đọc trạng thái chip PA (0 = bật, 1 = force mute).
func speakerMuteState() string {
	data, err := os.ReadFile(speakerMutePath)
	if err != nil {
		return "?"
	}
	return strings.TrimSpace(string(data))
}

// bootRecheck kiểm tra lại đường ra âm thanh hai lần sau khi khởi động (10 và 30
// giây). Lúc boot có hai thứ đến muộn: sink của PipeWire và sự kiện jack tai nghe
// của triggerhappy (trimui-volume-attenuate chạy lại và đổi Headphone Volume).
// Không sửa khi FN switch đang ON, vì lúc đó câm là chủ ý của người dùng.
func (h *HardwareManager) bootRecheck() {
	for _, wait := range []time.Duration{10 * time.Second, 20 * time.Second} {
		time.Sleep(wait)
		h.mu.Lock()
		on := h.isON
		h.mu.Unlock()
		if on {
			return
		}
		log.Printf("[AUDIO] Kiểm tra lại đường ra âm thanh sau khởi động")
		repairAudio(h.cfg)
	}
}

// NewHardwareManager khởi tạo quản lý phần cứng. cfg chỉ dùng cho mức âm thanh
// (audio.dac_volume, audio.headphone_gain), nên nó được giữ lại cho các lần kiểm
// tra sau khởi động.
func NewHardwareManager(cfg *Config) *HardwareManager {
	repairAudio(cfg)
	// Knulli bật đèn mặc định lúc boot, nên trạng thái khởi tạo là "đang sáng"
	// để lần đầu bấm F2 (led_toggle) là tắt, đúng kỳ vọng.
	h := &HardwareManager{cfg: cfg, ledsOn: true}
	go h.bootRecheck()
	return h
}

// ApplySwitchState chuyển đổi trạng thái khi gạt FN switch.
func (h *HardwareManager) ApplySwitchState(on bool, cfg *Config) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if on == h.isON && h.snapshot != nil {
		return // Trạng thái không đổi
	}

	if on {
		// 1. Chụp lại cấu hình hiện tại trước khi thay đổi (chỉ chụp khi snapshot chưa có)
		if h.snapshot == nil {
			h.snapshot = h.takeSnapshotLocked()
			log.Printf("[STATE] FN gạt ON. Đã lưu cấu hình gốc: CPU=%s, Sáng=%d%%, Âm lượng=%d%%, LED_items=%d",
				h.snapshot.Governor, h.snapshot.Brightness, h.snapshot.Volume, len(h.snapshot.LEDState))
		}

		// 2. Áp dụng cấu hình khi ON
		h.applyOnLocked(cfg)
		h.isON = true
	} else {
		// 3. Khôi phục lại chính xác cấu hình trước đó
		if h.snapshot != nil {
			log.Printf("[STATE] FN gạt OFF. Khôi phục lại: CPU=%s, Sáng=%d%%, Âm lượng=%d%%",
				h.snapshot.Governor, h.snapshot.Brightness, h.snapshot.Volume)
			h.restoreLocked(cfg, h.snapshot)
			h.snapshot = nil
		} else {
			log.Printf("[STATE] FN gạt OFF (Không có snapshot, giữ nguyên âm lượng người dùng, mở tầng cứng ALSA).")
			// Không có snapshot: Khôi phục cấu hình mặc định an toàn:
			// Sửa những gì có thể gây câm, khôi phục LED và CPU governor
			// (GIỮ NGUYÊN volume người dùng, không ép lên 100%)
			repairAudio(cfg)
			setCPUGovernor("schedutil")
			restoreAllLEDs(nil)
			h.ledsOn = true
		}
		h.isON = false
	}
}

// takeSnapshotLocked chụp lại trạng thái hệ thống.
func (h *HardwareManager) takeSnapshotLocked() *HardwareSnapshot {
	vol := getVolume()
	if vol <= 0 {
		// Mặc định an toàn 70%
		vol = 70
	}
	s := &HardwareSnapshot{
		Governor:   getCPUGovernor(),
		Brightness: getBrightness(),
		Volume:     vol,
		LEDState:   saveLEDState(),
	}
	return s
}

// applyOnLocked áp dụng các thiết lập ON.
func (h *HardwareManager) applyOnLocked(cfg *Config) {
	// 1. TẠM DỪNG NHẠC VÀ MUTE TRƯỚC TIÊN (khi CPU vẫn đang ở tốc độ bình thường)
	// Tránh việc hạ xung CPU trước khiến bộ giải mã âm thanh bị thiếu chu kỳ dẫn đến khựng/lặp đoạn buffer
	if cfg.Actions.Volume.Enabled {
		if cfg.Actions.Volume.PauseMedia {
			pauseMedia()
		}
		if cfg.Actions.Volume.MuteOn {
			setVolume(0)
		} else if cfg.Actions.Volume.PercentOn > 0 {
			setVolume(cfg.Actions.Volume.PercentOn)
		}
	}

	// Chờ ngắn để luồng âm thanh xả sạch bộ đệm
	time.Sleep(60 * time.Millisecond)

	// 2. CPU Governor: sau khi nhạc đã dừng mới chuyển sang powersave
	if cfg.Actions.CPU.Enabled && cfg.Actions.CPU.GovernorOn != "" {
		setCPUGovernor(cfg.Actions.CPU.GovernorOn)
	}

	// 3. Độ sáng màn hình
	if cfg.Actions.Brightness.Enabled {
		setBrightness(cfg.Actions.Brightness.PercentOn)
	}

	// 4. Đèn LED RGB: Tắt sạch 100% mọi vùng LED (kể cả F1/F2, cần analog, vai máy)
	if cfg.Actions.LEDs.Enabled {
		turnOffAllLEDs()
		h.ledsOn = false
	}

	// 5. Custom script/command khi ON
	if cfg.Actions.CustomCommands.Enabled && cfg.Actions.CustomCommands.OnCommand != "" {
		runShellCommand(cfg.Actions.CustomCommands.OnCommand)
	}
}

// restoreLocked khôi phục lại cấu hình đã lưu.
func (h *HardwareManager) restoreLocked(cfg *Config, s *HardwareSnapshot) {
	// 1. Khôi phục CPU Governor TRƯỚC TIÊN để hệ thống phản hồi tức thì
	if cfg.Actions.CPU.Enabled && s.Governor != "" {
		setCPUGovernor(s.Governor)
	}

	// 2. Khôi phục độ sáng màn hình
	if cfg.Actions.Brightness.Enabled && s.Brightness >= 0 {
		setBrightness(s.Brightness)
	}

	// 3. Khôi phục âm thanh: khôi phục ALSA hardware gain 100% trước, đặt volume hệ thống, sau đó phát tiếp
	if cfg.Actions.Volume.Enabled {
		vol := s.Volume
		if vol <= 0 {
			vol = 100
		}
		setVolume(vol)
		if cfg.Actions.Volume.PauseMedia {
			resumeMedia()
		}
	}

	// 4. Khôi phục toàn bộ đèn LED
	if cfg.Actions.LEDs.Enabled {
		restoreAllLEDs(s.LEDState)
		h.ledsOn = true
	}

	// 5. Custom script/command khi OFF
	if cfg.Actions.CustomCommands.Enabled && cfg.Actions.CustomCommands.OffCommand != "" {
		runShellCommand(cfg.Actions.CustomCommands.OffCommand)
	}
}

// --- QUẢN LÝ ĐÈN LED CHI TIẾT CHO TRIMUI BRICK PRO ---

var ledAnimScaleFiles = []string{
	"/sys/class/led_anim/max_scale",
	"/sys/class/led_anim/max_scale_lr",
	"/sys/class/led_anim/max_scale_rear",
	"/sys/class/led_anim/max_scale_f1f2",
}

var ledAnimEffectFiles = []string{
	"/sys/class/led_anim/effect_lr",
	"/sys/class/led_anim/effect_m",
	"/sys/class/led_anim/effect_rear",
	"/sys/class/led_anim/effect_f1",
	"/sys/class/led_anim/effect_f2",
}

var ledAnimCycleFiles = []string{
	"/sys/class/led_anim/effect_cycles_lr",
	"/sys/class/led_anim/effect_cycles_m",
	"/sys/class/led_anim/effect_cycles_rear",
	"/sys/class/led_anim/effect_cycles_f1",
	"/sys/class/led_anim/effect_cycles_f2",
}

// saveLEDState chụp lại toàn bộ cấu hình đèn hiện tại
func saveLEDState() map[string]string {
	state := make(map[string]string)
	for _, f := range ledAnimScaleFiles {
		if data, err := os.ReadFile(f); err == nil {
			state[f] = strings.TrimSpace(string(data))
		}
	}
	for _, f := range ledAnimEffectFiles {
		if data, err := os.ReadFile(f); err == nil {
			state[f] = strings.TrimSpace(string(data))
		}
	}
	for _, f := range ledAnimCycleFiles {
		if data, err := os.ReadFile(f); err == nil {
			state[f] = strings.TrimSpace(string(data))
		}
	}
	matches, _ := filepath.Glob("/sys/class/leds/sunxi_led*/brightness")
	for _, f := range matches {
		if data, err := os.ReadFile(f); err == nil {
			state[f] = strings.TrimSpace(string(data))
		}
	}
	return state
}

// turnOffAllLEDs tắt sạch 100% tất cả các bóng LED (kể cả F1/F2 và analog)
func turnOffAllLEDs() {
	// 1. Tắt tất cả các scale độ sáng (kể cả F1/F2 và analog, rear)
	for _, f := range ledAnimScaleFiles {
		_ = os.WriteFile(f, []byte("0\n"), 0644)
	}
	_ = os.WriteFile("/sys/class/led_anim/effect_enable", []byte("0\n"), 0644)

	// 2. Tắt các effect cycles
	for _, f := range ledAnimCycleFiles {
		_ = os.WriteFile(f, []byte("0\n"), 0644)
	}

	// 3. Tắt tất cả các bóng 22 LED RGB trong sunxi_leds
	matches, _ := filepath.Glob("/sys/class/leds/sunxi_led*/brightness")
	for _, f := range matches {
		_ = os.WriteFile(f, []byte("0\n"), 0644)
	}
	log.Printf("[LED] Đã tắt sạch 100%% toàn bộ các vùng LED (F1/F2, Analog, Top, Rear).")
}

// restoreAllLEDs bật lại đúng trạng thái trước đó (có fallback nếu snapshot trống/bị 0)
func restoreAllLEDs(saved map[string]string) {
	// 1. Khôi phục max_scale (có fallback an toàn nếu snapshot bị 0)
	for _, f := range ledAnimScaleFiles {
		val := ""
		if saved != nil {
			val = saved[f]
		}
		if val == "" || val == "0" {
			if strings.HasSuffix(f, "max_scale") {
				val = "255"
			} else {
				val = "40" // max_scale_lr, max_scale_rear, max_scale_f1f2
			}
		}
		_ = os.WriteFile(f, []byte(val+"\n"), 0644)
	}

	// 2. Bật lại effect_enable
	_ = os.WriteFile("/sys/class/led_anim/effect_enable", []byte("1\n"), 0644)

	// 3. Khôi phục các effect cycles (-1 là lặp vô tận)
	for _, f := range ledAnimCycleFiles {
		val := ""
		if saved != nil {
			val = saved[f]
		}
		if val == "" || val == "0" {
			val = "-1"
		}
		_ = os.WriteFile(f, []byte(val+"\n"), 0644)
	}

	// 4. Kích hoạt lại effect (2 = breath, hiệu ứng mặc định rất đẹp của Brick Pro)
	for _, f := range ledAnimEffectFiles {
		val := ""
		if saved != nil {
			val = saved[f]
		}
		if val == "" || val == "0" {
			val = "2"
		}
		_ = os.WriteFile(f, []byte(val+"\n"), 0644)
	}

	// 5. Khôi phục độ sáng các bóng sunxi_leds
	if saved != nil {
		for f, val := range saved {
			if strings.HasPrefix(f, "/sys/class/leds/") {
				_ = os.WriteFile(f, []byte(val+"\n"), 0644)
			}
		}
	}
	log.Printf("[LED] Đã khôi phục hoàn chỉnh toàn bộ LED (F1/F2, Analog, Top, Rear).")
}

// --- CÁC HÀM XỬ LÝ PHẦN CỨNG CHI TIẾT CHO KNULLI ---

// CPU Governor
func getCPUGovernor() string {
	b, err := os.ReadFile("/sys/devices/system/cpu/cpu0/cpufreq/scaling_governor")
	if err != nil {
		return "schedutil"
	}
	return strings.TrimSpace(string(b))
}

func setCPUGovernor(gov string) {
	gov = strings.TrimSpace(gov)
	files, _ := filepath.Glob("/sys/devices/system/cpu/cpu*/cpufreq/scaling_governor")
	if len(files) == 0 {
		return
	}
	for _, f := range files {
		_ = os.WriteFile(f, []byte(gov+"\n"), 0644)
	}
	log.Printf("[CPU] Governor -> %s", gov)
}

// Độ sáng màn hình (Brightness qua knulli-brightness)
func getBrightness() int {
	out, err := exec.Command("knulli-brightness").Output()
	if err == nil {
		val, err := strconv.Atoi(strings.TrimSpace(string(out)))
		if err == nil && val >= 0 {
			return val
		}
	}
	return 70
}

func setBrightness(percent int) {
	if percent < 0 {
		percent = 0
	} else if percent > 100 {
		percent = 100
	}
	out, err := exec.Command("knulli-brightness", strconv.Itoa(percent)).CombinedOutput()
	if err != nil {
		log.Printf("[BRIGHTNESS] Lỗi đặt knulli-brightness: %v (%s)", err, strings.TrimSpace(string(out)))
	} else {
		log.Printf("[BRIGHTNESS] knulli-brightness -> %d%%", percent)
	}
}

// Âm thanh (knulli-audio, pactl & ALSA)
func getVolume() int {
	// 1. Đọc thẳng sink đang phát (đây là mức người dùng thấy trên OSD)
	if v := pactlVolume(); v > 0 {
		return v
	}

	// 2. Dự phòng: knulli-audio getSystemVolume
	for _, bin := range []string{"knulli-audio", "batocera-audio"} {
		if out, err := exec.Command(bin, "getSystemVolume").Output(); err == nil {
			if v, err := strconv.Atoi(strings.TrimSpace(string(out))); err == nil && v > 0 {
				return v
			}
		}
	}

	// 3. Đọc từ file cấu hình hệ thống knulli.conf hoặc batocera.conf
	if v := configVolume(); v > 0 {
		return v
	}

	// 4. Mặc định 70% (giữ mức âm lượng vừa phải, không ép lên 100%)
	return 70
}

func pactlVolume() int {
	return sinkVolume("@DEFAULT_SINK@")
}

// setVolume đổi âm lượng phần mềm của sink mặc định.
// Mức 0 chỉ mute, KHÔNG đặt âm lượng về 0%: lúc tắt máy Knulli lưu audio.volume
// bằng chính mức của sink, nên để 0% sẽ làm lần khởi động sau mất tiếng hoàn toàn.
func setVolume(percent int) {
	if percent <= 0 {
		runCmd("pactl", "set-sink-mute", "@DEFAULT_SINK@", "1")
		log.Printf("[VOLUME] Đã tắt tiếng (mute phần mềm, giữ nguyên mức âm lượng)")
		return
	}

	if percent > 100 {
		percent = 100
	}
	runCmd("pactl", "set-sink-mute", "@DEFAULT_SINK@", "0")
	runCmd("pactl", "set-sink-volume", "@DEFAULT_SINK@", fmt.Sprintf("%d%%", percent))
	log.Printf("[VOLUME] Đã khôi phục âm lượng -> %d%%", percent)
}

// sendAudiofilusCommand gửi lệnh JSON trực tiếp qua Unix Socket của Audiofilus
func sendAudiofilusCommand(cmd string) bool {
	socks, _ := filepath.Glob("/tmp/audiofilus*.sock")
	if len(socks) == 0 {
		return false
	}
	sent := false
	msg := fmt.Sprintf(`{"cmd":"%s"}`+"\n", cmd)
	for _, sock := range socks {
		conn, err := net.DialTimeout("unix", sock, 200*time.Millisecond)
		if err == nil {
			_ = conn.SetDeadline(time.Now().Add(300 * time.Millisecond))
			_, _ = conn.Write([]byte(msg))
			var buf [256]byte
			_, _ = conn.Read(buf[:])
			_ = conn.Close()
			sent = true
			log.Printf("[MEDIA] Gửi lệnh Audiofilus (%s) qua socket %s thành công", cmd, sock)
		}
	}
	return sent
}

// pauseMedia tạm dừng các trình phát nhạc (Audiofilus, playerctl)
func pauseMedia() {
	log.Printf("[MEDIA] Tạm dừng phát nhạc...")
	// 1. Gửi lệnh pause trực tiếp tới Audiofilus qua socket Unix (cực nhanh và tức thì)
	if !sendAudiofilusCommand("pause") {
		// Dự phòng nếu chưa có socket thì gọi CLI
		for _, bin := range []string{
			"/userdata/roms/ports/audiofilus/audiofilus.aarch64",
			"/userdata/roms/ports/audiofilus/audiofilus",
			"/roms/ports/audiofilus/audiofilus.aarch64",
			"/roms/ports/audiofilus/audiofilus",
			"audiofilus",
		} {
			if _, err := os.Stat(bin); err == nil {
				_ = exec.Command(bin, "pause").Run()
				break
			}
		}
	}
	// 2. Tạm dừng playerctl
	_ = exec.Command("playerctl", "pause").Run()
}

// resumeMedia tiếp tục phát nhạc khi gạt OFF
func resumeMedia() {
	log.Printf("[MEDIA] Tiếp tục phát nhạc...")
	// 1. Gửi lệnh resume trực tiếp tới Audiofilus qua socket Unix
	if !sendAudiofilusCommand("resume") {
		for _, bin := range []string{
			"/userdata/roms/ports/audiofilus/audiofilus.aarch64",
			"/userdata/roms/ports/audiofilus/audiofilus",
			"/roms/ports/audiofilus/audiofilus.aarch64",
			"/roms/ports/audiofilus/audiofilus",
			"audiofilus",
		} {
			if _, err := os.Stat(bin); err == nil {
				_ = exec.Command(bin, "resume").Run()
				break
			}
		}
	}
	// 2. Tiếp tục playerctl
	_ = exec.Command("playerctl", "play").Run()
}

// getLEDScale đọc độ sáng LED chính
func getLEDScale() string {
	b, err := os.ReadFile("/sys/class/led_anim/max_scale")
	if err != nil {
		return "255"
	}
	return strings.TrimSpace(string(b))
}

// Chạy lệnh shell tùy ý
func runShellCommand(cmd string) {
	log.Printf("[HOOK] Chạy lệnh: %s", cmd)
	go func() {
		_ = exec.Command("sh", "-c", cmd).Run()
	}()
}
