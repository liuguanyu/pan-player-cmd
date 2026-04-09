package main

import (
	"fmt"
	"os"
	"time"

	"github.com/faiface/beep"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run test_audio.go <mp3_file>")
		os.Exit(1)
	}

	filePath := os.Args[1]
	fmt.Printf("Opening file: %s\n", filePath)

	// 打开 MP3 文件
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	// 解码 MP3
	streamer, format, err := mp3.Decode(file)
	if err != nil {
		fmt.Printf("Error decoding MP3: %v\n", err)
		os.Exit(1)
	}
	defer streamer.Close()

	fmt.Printf("Sample Rate: %d\n", format.SampleRate)
	fmt.Printf("Channels: %d\n", format.NumChannels)
	fmt.Printf("Precision: %d\n", format.Precision)

	// 初始化扬声器
	err = speaker.Init(format.SampleRate, format.SampleRate.N(time.Second/10))
	if err != nil {
		fmt.Printf("Error initializing speaker: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Playing audio... Press Ctrl+C to stop.")

	// 创建通道等待播放完成
	done := make(chan bool)

	// 注册回调
	speaker.Play(beep.Seq(streamer, beep.Callback(func() {
		done <- true
	})))

	// 等待播放完成
	<-done
	fmt.Println("\nPlayback completed!")
}
