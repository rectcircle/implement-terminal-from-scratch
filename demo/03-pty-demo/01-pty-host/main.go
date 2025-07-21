package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/creack/pty"
)

const ASNIEInputSeqDemo = "hello world\r" + // 第一行: 常规的 ascii 字符，应用程序原样接受
	"中文\r" + // 第二行：中文字符，行为和第一行一样，应用程序原样接受
	// TODO: 测试更多序列情况。比如方向键，退格删除等等。
	"\u0003" // 最后一行：ctrl+c 信号

func main() {
	binPath, err := filepath.Abs("./echo-stdin-json-str")
	if err != nil {
		panic(err)
	}
	cmd := exec.Command(binPath)

	ptyMaster, err := pty.Start(cmd)
	if err != nil {
		panic(err)
	}
	defer ptyMaster.Close()
	go func() {
		_, _ = io.Copy(os.Stdout, ptyMaster)
	}()
	for _, b := range []byte(ASNIEInputSeqDemo) {
		_, err = ptyMaster.Write([]byte{b})
		if err != nil {
			panic(err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
