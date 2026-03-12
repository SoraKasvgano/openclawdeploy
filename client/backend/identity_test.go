package backend

import (
	"strings"
	"testing"

	"github.com/makiuchi-d/gozxing"
	gozxingqrcode "github.com/makiuchi-d/gozxing/qrcode"
)

func TestIdentityQRCodeEncodesDeviceID(t *testing.T) {
	deviceID := "likeqi345a60f45388202603120811451006403"

	code, err := newIdentityQRCode(deviceID)
	if err != nil {
		t.Fatalf("new qr code: %v", err)
	}

	bitmap, err := gozxing.NewBinaryBitmapFromImage(code.Image(512))
	if err != nil {
		t.Fatalf("binary bitmap: %v", err)
	}

	result, err := gozxingqrcode.NewQRCodeReader().Decode(bitmap, nil)
	if err != nil {
		t.Fatalf("decode qr code: %v", err)
	}
	if result.GetText() != deviceID {
		t.Fatalf("decoded text mismatch: got %q want %q", result.GetText(), deviceID)
	}
}

func TestRenderIdentityQRSVGProducesSVG(t *testing.T) {
	svg := RenderIdentityQRSVG("likeqi345a60f45388202603120811451006403")
	if !strings.Contains(svg, "<svg") {
		t.Fatalf("expected svg output, got %q", svg)
	}
	if !strings.Contains(svg, `aria-label="device qr code"`) {
		t.Fatalf("expected qr svg label, got %q", svg)
	}
}
