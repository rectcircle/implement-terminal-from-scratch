package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	reader := bufio.NewReader(os.Stdin)
	jobController := &JobController{}

	if !jobController.CanEnableJobControl() {
		fmt.Println("Job control not available. Exiting.")
		os.Exit(1)
	}

	for {
		// 显示提示符
		fmt.Print("shell-demo> ")
		// 刷新输出缓冲区
		os.Stdout.Sync()

		// 读取用户输入
		input, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Println("\nGoodbye!")
				break
			}
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

		// 解析并执行命令
		err = jobController.Execute(input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error executing command: %v\n", err)
		}
	}
}
