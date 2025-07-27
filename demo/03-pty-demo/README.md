# 03-pty-demo

Go 版本

```bash
cd demo/03-pty-demo
go build -o echo-stdin-json-str ./02-echo-stdin-json-str
go run ./01-pty-host
```

C 版本

```bash
cd demo/03-pty-demo
go build -o echo-stdin-json-str ./02-echo-stdin-json-str
gcc -o pty-host-linux-c ./03-pty-host-linux-c/main.c
./pty-host-linux-c
```
