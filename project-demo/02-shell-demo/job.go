package main

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

type JobController struct {
}

// parseAndExecuteCommand 解析并执行命令，支持管道符
func (k *JobController) Execute(input string) error {
	// 按管道符分割命令
	pipeCommands := strings.Split(input, "|")
	if len(pipeCommands) == 0 {
		return nil
	}

	// 如果只有一个命令，使用原来的逻辑
	if len(pipeCommands) == 1 {
		return k.executeSingleCommand(strings.TrimSpace(pipeCommands[0]))
	}

	// 处理管道命令
	return k.executePipeCommands(pipeCommands)
}

// executeSingleCommand 执行单个命令
func (k *JobController) executeSingleCommand(input string) error {
	// 分割命令和参数 (不考虑 "" '' 等语法)
	args := strings.Fields(input)
	if len(args) == 0 {
		return nil
	}

	// 第一个参数是命令，其余是参数
	cmd := exec.Command(args[0], args[1:]...)

	// 设置标准输入、输出、错误输出
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 执行命令
	return cmd.Run()
}

// executePipeCommands 执行管道命令
func (k *JobController) executePipeCommands(pipeCommands []string) error {
	var cmds []*exec.Cmd
	var pipes []io.ReadCloser
	var pgid int // 进程组ID

	// 创建所有命令，并设置好 stdio 连接关系
	for i, cmdStr := range pipeCommands {
		cmdStr = strings.TrimSpace(cmdStr)
		args := strings.Fields(cmdStr)
		if len(args) == 0 {
			continue
		}

		cmd := exec.Command(args[0], args[1:]...)
		cmds = append(cmds, cmd)

		// 设置管道
		if i == 0 {
			// 第一个命令从标准输入读取
			cmd.Stdin = os.Stdin
		} else {
			// 后续命令从前一个命令的输出读取
			cmd.Stdin = pipes[i-1]
		}

		if i == len(pipeCommands)-1 {
			// 最后一个命令输出到标准输出
			cmd.Stdout = os.Stdout
		} else {
			// 中间命令的输出作为管道
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				return err
			}
			pipes = append(pipes, stdout)
		}

		// 所有命令的错误输出都到标准错误
		cmd.Stderr = os.Stderr
	}

	// 启动所有命令，并设置好进程组
	for i, cmd := range cmds {
		if i == 0 {
			// 第一个进程作为进程组组长
			cmd.SysProcAttr = &syscall.SysProcAttr{
				Setpgid: true,
				Pgid:    0, // 0 表示使用进程自己的PID作为进程组ID
			}
		} else {
			// 后续进程加入第一个进程的进程组
			cmd.SysProcAttr = &syscall.SysProcAttr{
				Setpgid: true,
				Pgid:    pgid,
			}
		}

		if err := cmd.Start(); err != nil {
			return err
		}

		// 记录第一个进程的进程组ID
		if i == 0 {
			pgid = cmd.Process.Pid
		}
	}

	// 等待进程组中的所有进程完成
	// 使用 Wait4 等待进程组
	for {
		var status syscall.WaitStatus
		// 等待进程组中的任意子进程
		pid, err := syscall.Wait4(-pgid, &status, 0, nil)
		if err != nil {
			// 如果没有更多子进程，退出循环
			if err == syscall.ECHILD {
				break
			}
			return err
		}

		// 检查是否所有进程都已完成
		allDone := true
		for _, cmd := range cmds {
			if cmd.Process != nil && cmd.ProcessState == nil {
				allDone = false
				break
			}
		}

		if allDone {
			break
		}

		// 如果进程异常退出，返回错误
		if !status.Exited() || status.ExitStatus() != 0 {
			// 继续等待其他进程，但记录错误状态
			_ = pid // 可以在这里记录哪个进程出错
		}
	}

	return nil
}
