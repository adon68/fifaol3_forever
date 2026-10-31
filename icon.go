package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

func trayIcon() []byte { return makeIconPNG(64) }

func makeIconPNG(size int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	c := float64(size) / 2
	r := c - 1

	ball := color.NRGBA{R: 0xF2, G: 0xC2, B: 0x00, A: 0xFF}
	ink := color.NRGBA{R: 0x0f, G: 0x16, B: 0x20, A: 0xFF}
	edge := color.NRGBA{R: 0x0f, G: 0x16, B: 0x20, A: 0xFF}

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)+0.5-c, float64(y)+0.5-c
			d := math.Hypot(dx, dy)
			switch {
			case d > r:

			case d > r-float64(size)*0.05:
				img.Set(x, y, edge)
			case patch(dx/r, dy/r):
				img.Set(x, y, ink)
			default:
				img.Set(x, y, ball)
			}
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func patch(nx, ny float64) bool {
	if math.Hypot(nx, ny) < 0.26 {
		return true
	}

	for i := 0; i < 5; i++ {
		a := float64(i)*2*math.Pi/5 - math.Pi/2
		px, py := math.Cos(a)*0.62, math.Sin(a)*0.62
		if math.Hypot(nx-px, ny-py) < 0.19 {
			return true
		}
	}
	return false
}
