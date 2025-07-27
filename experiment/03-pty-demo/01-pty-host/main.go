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
	"  对于可打印字符(中英文)\r" +
	"    1.在应用程序接受之前已经打印了，这是行规程的回显功能\r" +
	"    2.行规程原样透传到应用程序\r" +
	"    3.行规程将 \\r 转换为 \\n 传递给应用程序\r" +
	"    4.行规程有一个行 buffer 遇到 \\r 才会将 buffer 的内容传递给应用程序\r" +
	"测试行编辑(按退格的效果\\u007f): hello world,\u007f!\r" +
	"  可以看出，\\u007f 删除了前面的逗号, 应用程序接受到的是 hello world!\r" +
	"测试行编辑(按方向键效果): world\u001b[D\u001b[D\u001b[D\u001b[D\u001b[Dhello \r" +
	"  可以看出，方向键不会影响行规程的行编辑\r" +
	"* 即将发送 ctrl+c 信号，应用程序将收到 SIGINT(2) 信号\r" +
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
		time.Sleep(10 * time.Millisecond)
	}
}
