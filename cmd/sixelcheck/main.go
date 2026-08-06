package main

import (
	"fmt"
	"image/color"
	"os"

	"github.com/liuguanyu/pan-player-cmd/internal/visualizer"
)

func main() {
	// Check terminal
	supported, reason := visualizer.SixelCapable()
	fmt.Fprintf(os.Stderr, "Terminal check: supported=%v reason=%q\n", supported, reason)
	fmt.Fprintf(os.Stderr, "TERM=%s TERM_PROGRAM=%s TERM_PROGRAM_VERSION=%s\n",
		os.Getenv("TERM"), os.Getenv("TERM_PROGRAM"), os.Getenv("TERM_PROGRAM_VERSION"))

	if !supported {
		fmt.Println("Sixel not supported:", reason)
		os.Exit(1)
	}

	// Draw a simple test image: 320x240 red-green-blue-yellow bands + white circle
	c := visualizer.NewCanvas(320, 240)
	c.FillRect(0, 0, 320, 60, color.RGBA{255, 50, 50, 255})
	c.FillRect(0, 60, 320, 60, color.RGBA{50, 255, 50, 255})
	c.FillRect(0, 120, 320, 60, color.RGBA{50, 50, 255, 255})
	c.FillRect(0, 180, 320, 60, color.RGBA{255, 255, 50, 255})
	c.FillEllipse(160, 120, 100, 80, color.RGBA{255, 255, 255, 255})

	result := visualizer.SixelEncode(c.Image())

	fmt.Print("=== Before Sixel ===\n")
	os.Stdout.Write([]byte(result))
	fmt.Print("\n=== After Sixel ===\n")
}
