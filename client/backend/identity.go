package backend

import (
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/skip2/go-qrcode"

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

func RenderIdentityQRSVG(value string) string {
	bitmap, err := identityQRBitmap(value)
	if err != nil {
		return ""
	}

	size := len(bitmap)
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf(`<svg viewBox="0 0 %d %d" xmlns="http://www.w3.org/2000/svg" shape-rendering="crispEdges" aria-label="device qr code">`, size, size))
	builder.WriteString(`<rect width="100%" height="100%" fill="#ffffff"/>`)
	builder.WriteString(`<g fill="#111111">`)

	for y := range bitmap {
		for x := range bitmap[y] {
			if bitmap[y][x] {
				builder.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="1" height="1"/>`, x, y))
			}
		}
	}

	builder.WriteString(`</g></svg>`)
	return builder.String()
}

func RenderIdentityQRCodeCLI(value string) string {
	code, err := newIdentityQRCode(value)
	if err != nil {
		return value + "\n"
	}
	return code.ToSmallString(true)
}

func RenderIdentityMatrixSVG(value string) string {
	return RenderIdentityQRSVG(value)
}

func RenderIdentityMatrixASCII(value string) string {
	return RenderIdentityQRCodeCLI(value)
}

func identityQRBitmap(value string) ([][]bool, error) {
	code, err := newIdentityQRCode(value)
	if err != nil {
		return nil, err
	}
	return code.Bitmap(), nil
}

func newIdentityQRCode(value string) (*qrcode.QRCode, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("device id is empty")
	}
	return qrcode.New(value, qrcode.Medium)
}

func blankTo(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
