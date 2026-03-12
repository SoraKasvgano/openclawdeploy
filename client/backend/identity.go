package backend

import (
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"time"

	"openclawdeploy/internal/shared"
)

type Identity struct {
	DeviceID    string `json:"device_id"`
	Hostname    string `json:"hostname"`
	MAC         string `json:"mac"`
	LocalIP     string `json:"local_ip"`
	GeneratedAt string `json:"generated_at"`
	NetworkOK   bool   `json:"network_ok"`
}

func GenerateIdentityCode() (string, Identity, error) {
	hostname, _ := os.Hostname()
	mac := primaryMAC()
	localIP := primaryIPv4()
	generatedAt := time.Now().UTC().Add(8 * time.Hour).Format("2006-01-02 15:04:05")

	deviceID := shared.NormalizeDeviceID(strings.Join([]string{
		blankTo(hostname, runtime.GOOS),
		blankTo(mac, "no-mac"),
		generatedAt,
		blankTo(localIP, "no-ip"),
	}, "|"))
	if deviceID == "" {
		deviceID = shared.NormalizeDeviceID(runtime.GOOS + generatedAt)
	}

	return deviceID, Identity{
		DeviceID:    deviceID,
		Hostname:    hostname,
		MAC:         mac,
		LocalIP:     localIP,
		GeneratedAt: generatedAt,
	}, nil
}

func CurrentIdentity(cfg *Config, networkOK bool) Identity {
	hostname, _ := os.Hostname()
	return Identity{
		DeviceID:    cfg.DeviceID,
		Hostname:    hostname,
		MAC:         primaryMAC(),
		LocalIP:     primaryIPv4(),
		GeneratedAt: cfg.DeviceCreatedAt,
		NetworkOK:   networkOK,
	}
}

func primaryMAC() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if iface.HardwareAddr.String() != "" {
			return iface.HardwareAddr.String()
		}
	}

	return ""
}

func primaryIPv4() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP == nil || ipNet.IP.IsLoopback() {
				continue
			}
			if ip := ipNet.IP.To4(); ip != nil {
				return ip.String()
			}
		}
	}

	return ""
}

func RenderIdentityMatrixSVG(value string) string {
	const cells = 21
	const scale = 10

	sum := sha256.Sum256([]byte(value))
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf(`<svg viewBox="0 0 %d %d" xmlns="http://www.w3.org/2000/svg" aria-label="identity matrix">`, cells*scale, cells*scale))
	builder.WriteString(`<rect width="100%" height="100%" fill="#ffffff"/>`)

	for y := range cells {
		for x := range cells {
			if drawFinder(x, y, cells) || matrixBit(sum, x, y) {
				builder.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" rx="1" fill="#132238"/>`, x*scale, y*scale, scale, scale))
			}
		}
	}

	builder.WriteString(`</svg>`)
	return builder.String()
}

func RenderIdentityMatrixASCII(value string) string {
	const cells = 21
	sum := sha256.Sum256([]byte(value))

	var builder strings.Builder
	for y := range cells {
		for x := range cells {
			if drawFinder(x, y, cells) || matrixBit(sum, x, y) {
				builder.WriteString("██")
			} else {
				builder.WriteString("  ")
			}
		}
		builder.WriteByte('\n')
	}
	return builder.String()
}

func matrixBit(sum [32]byte, x, y int) bool {
	index := (x + y*7) % len(sum)
	shift := uint((x*3 + y) % 8)
	return ((sum[index] >> shift) & 1) == 1
}

func drawFinder(x, y, cells int) bool {
	return finderAt(x, y, 0, 0) ||
		finderAt(x, y, cells-7, 0) ||
		finderAt(x, y, 0, cells-7)
}

func finderAt(x, y, left, top int) bool {
	if x < left || x >= left+7 || y < top || y >= top+7 {
		return false
	}
	if x == left || x == left+6 || y == top || y == top+6 {
		return true
	}
	return x >= left+2 && x <= left+4 && y >= top+2 && y <= top+4
}

func blankTo(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
