package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	reader := bufio.NewReader(os.Stdin)

	for {
		// 显示提示符
		fmt.Print("shell-demo> ")
		// 刷新输出缓冲区
		os.Stdout.Sync()

		// 读取用户输入
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
			continue
		}

		// 去除首位换行符和空格
		input = strings.TrimSpace(input)

		// 如果输入为空，继续下一次循环
		if input == "" {
			continue
		}

		// 如果输入是 exit，退出程序
		if input == "exit" {
			fmt.Println("Goodbye!")
			break
		}

		// 分割命令和参数 (不考虑 "" '' 等语法)
		args := strings.Fields(input)
		if len(args) == 0 {
			continue
		}

		// 第一个参数是命令，其余是参数
		cmd := exec.Command(args[0], args[1:]...)

		// 设置标准输入、输出、错误输出
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		// 执行命令
		err = cmd.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error executing command: %v\n", err)
		}
	}
}
