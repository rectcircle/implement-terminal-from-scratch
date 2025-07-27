package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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

	options := &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	}

	wsConn, err := websocket.Accept(w, r, options)
	if err != nil {
		slog.Error("websocket accept failed", "err", err)
		return
	}
	defer wsConn.CloseNow()

	ptyFile, err := startShellByPty()
	if err != nil {
		slog.Error("start shell by pty failed", "err", err)
		wsConn.Close(websocket.StatusInternalError, "start shell by pty failed")
		return
	}

	// Set the context as needed. Use of r.Context() is not recommended
	// to avoid surprising behavior (see http.Hijacker).
	ctx := context.Background()

	clientToPtyCloseCh := make(chan struct{})
	ptyToClientCloseCh := make(chan struct{})

	// read from pty file -> write to websocket
	go func() {
		buf := make([]byte, 1024)
		for {
			// 从 pty 读取数据
			n, err := ptyFile.Read(buf)
			if err != nil {
				slog.Error("read from pty failed", "err", err)
				wsConn.Close(websocket.StatusNormalClosure, err.Error())
				close(clientToPtyCloseCh)
				return
			}
			// 读取到数据后，将其写入 WebSocket
			if n > 0 {
				err := wsConn.Write(ctx, websocket.MessageText, buf[:n])
				if err != nil {
					slog.Error("write to websocket failed", "err", err)
					_ = ptyFile.Close()
					close(clientToPtyCloseCh)
					return
				}
			}
		}
	}()

	// read from websocket -> write to pty file
	go func() {
		for {
			// 从 WebSocket 读取数据
			_, buf, err := wsConn.Read(ctx)
			if err != nil {
				slog.Error("read from websocket failed", "err", err)
				_ = ptyFile.Close()
				close(ptyToClientCloseCh)
				return
			}
			jsonStr, _ := json.Marshal(string(buf))
			fmt.Printf("ws->pty: %s, %v\n", string(jsonStr), buf)

			// 读取到数据后，将其写入 pty
			if len(buf) > 0 {
				_, err := ptyFile.Write(buf)
				if err != nil {
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

	// 启动服务器在 8080 端口
	log.Println("Starting webshell demo server on :8080...")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
