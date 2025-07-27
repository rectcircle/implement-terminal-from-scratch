package main

import (
	"encoding/json"
	"io"
	"os"
	"os/signal"
)

func main() {
	// 处理 ctrl+c 信号
	ctrlCCh := make(chan os.Signal, 1)
	signal.Notify(ctrlCCh, os.Interrupt)
	go func() {
		<-ctrlCCh
		os.Stdout.Write([]byte("[echo-stdin-json-str][signal]: SIGINT (2)\n"))
		os.Exit(0)
	}()

	// 读并打印 stdin 内容
	buf := make([]byte, 4*1024*1024)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			panic(err)
		}

		input := string(buf[:n])
		inputJsonStr, err := json.Marshal(input)
		if err != nil {
			panic(err)
		}
		os.Stdout.Write([]byte("[echo-stdin-json-str][stdin]: "))
		os.Stdout.Write(inputJsonStr)
		os.Stdout.Write([]byte("\n"))
	}

}
