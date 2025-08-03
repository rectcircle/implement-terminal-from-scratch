package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"

	"github.com/coder/websocket"
	"github.com/creack/pty"
)

func startShellByPty() (*os.File, error) {
	cmd := exec.Command("/bin/bash", "-il")
	// 使用伪终端启动这个命令
	return pty.Start(cmd)
}

func ptyWsHandler(w http.ResponseWriter, r *http.Request) {

	// 允许跨域（仅测试）。
	options := &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	}

	// 接收 websocket upgrade 请求。
	wsConn, err := websocket.Accept(w, r, options)
	if err != nil {
		slog.Error("websocket accept failed", "err", err)
		return
	}
	defer wsConn.CloseNow()

	// 创建 pty，创建一个 bash 进程，并将 pty slave 和 bash 进程绑定。
	// 然后，返回 pty master。
	ptyFile, err := startShellByPty()
	if err != nil {
		slog.Error("start shell by pty failed", "err", err)
		wsConn.Close(websocket.StatusInternalError, "start shell by pty failed")
		return
	}
	defer ptyFile.Close()

	// 创建一个新的 ctx。
	ctx := context.Background()

	// 创建两个 channel 接收 websocket 关闭信号。
	clientToPtyCloseCh := make(chan struct{})
	ptyToClientCloseCh := make(chan struct{})

	// 从 pty master 读取 -> 写入到 websocket
	go func() {
		buf := make([]byte, 1024)
		for {
			// 从 pty 读取数据
			n, err := ptyFile.Read(buf)
			if err != nil {
				// TODO: 细化错误处理
				slog.Error("read from pty failed", "err", err)
				wsConn.Close(websocket.StatusNormalClosure, err.Error())
				close(clientToPtyCloseCh)
				return
			}
			if n > 0 {
				// 打印日志
				jsonStr, _ := json.Marshal(string(buf[:n]))
				fmt.Printf("pty->ws: %s, %v\n", string(jsonStr), buf[:n])
				// 读取到数据后，将其写入 WebSocket
				err := wsConn.Write(ctx, websocket.MessageText, buf[:n])
				if err != nil {
					// TODO: 细化错误处理
					slog.Error("write to websocket failed", "err", err)
					_ = ptyFile.Close()
					close(clientToPtyCloseCh)
					return
				}
			}
		}
	}()

	// read from websocket -> write to pty file
	// 从 websocket 读取 -> 写入到 pty master
	go func() {
		for {
			// 从 WebSocket 读取数据
			_, buf, err := wsConn.Read(ctx)
			if err != nil {
				// TODO: 细化错误处理
				slog.Error("read from websocket failed", "err", err)
				_ = ptyFile.Close()
				close(ptyToClientCloseCh)
				return
			}

			if len(buf) > 0 {
				// 打印日志
				jsonStr, _ := json.Marshal(string(buf))
				fmt.Printf("ws->pty: %s, %v\n", string(jsonStr), buf)
				// 读取到数据后，将其写入 pty
				_, err := ptyFile.Write(buf)
				if err != nil {
					// TODO: 细化错误处理
					slog.Error("write to pty failed", "err", err)
					_ = wsConn.Close(websocket.StatusNormalClosure, err.Error())
					close(ptyToClientCloseCh)
					return
				}
			}
		}
	}()

	select {
	case <-clientToPtyCloseCh:
	case <-ptyToClientCloseCh:
	}
}

func main() {
	// 注册处理函数
	http.HandleFunc("/", ptyWsHandler)

	// 在 8080 端口启动 http 服务器
	slog.Info("Starting webshell demo server on :8080...")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		slog.Error("http listen and serve failed ", "err", err)
		os.Exit(1)
	}
}
