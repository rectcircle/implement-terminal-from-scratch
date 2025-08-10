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

// Execute 解析并执行命令，支持管道符
func (k *JobController) Execute(input string) error {
	// 创建Job并执行
	job, err := NewJob(input)
	if err != nil {
		return err
	}
	return job.Execute()
}

// Job 表示一个作业，包含单个命令或管道命令
type Job struct {
	commands []*exec.Cmd     // 命令列表，每个元素是一个 *exec.Cmd
	hasPipe  bool            // 是否有管道符
	pgid     int             // 进程组ID
	pipes    []io.ReadCloser // 管道连接
}

// NewJob 创建一个新的Job，解析命令字符串
func NewJob(input string) (*Job, error) {
	// 按管道符分割命令
	pipeCommands := strings.Split(input, "|")
	if len(pipeCommands) == 0 {
		return nil, nil
	}

	job := &Job{
		hasPipe: len(pipeCommands) > 1,
	}

	// 将每个命令构造为 *exec.Cmd
	for _, cmdStr := range pipeCommands {
		cmdStr = strings.TrimSpace(cmdStr)
		args := strings.Fields(cmdStr)
		if len(args) == 0 {
			continue
		}

		cmd := exec.Command(args[0], args[1:]...)
		job.commands = append(job.commands, cmd)
	}

	return job, nil
}

// Execute 执行Job中的命令
func (j *Job) Execute() error {
	err := j.Start()
	if err != nil {
		return err
	}
	return j.Wait()
}

// Start 启动Job中的所有命令
func (j *Job) Start() error {
	if len(j.commands) == 0 {
		return nil
	}

	// 如果只有一个命令且没有管道符，直接启动
	if len(j.commands) == 1 && !j.hasPipe {
		cmd := j.commands[0]
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Start()
	}

	// 有管道符的情况，需要设置进程组和管道连接
	return j.startPipeCommands()
}

// Wait 等待Job中的所有进程退出
func (j *Job) Wait() error {
	if len(j.commands) == 0 {
		return nil
	}

	// 如果只有一个命令且没有管道符，直接等待
	if len(j.commands) == 1 && !j.hasPipe {
		return j.commands[0].Wait()
	}

	// 有管道符的情况，等待进程组
	return j.waitPipeCommands()
}

// startPipeCommands 启动管道命令
func (j *Job) startPipeCommands() error {
	cmds := j.commands

	// 设置管道连接
	for i, cmd := range cmds {
		// 设置管道
		if i == 0 {
			// 第一个命令从标准输入读取
			cmd.Stdin = os.Stdin
		} else {
			// 后续命令从前一个命令的输出读取
			cmd.Stdin = j.pipes[i-1]
		}

		if i == len(cmds)-1 {
			// 最后一个命令输出到标准输出
			cmd.Stdout = os.Stdout
		} else {
			// 中间命令的输出作为管道
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				return err
			}
			j.pipes = append(j.pipes, stdout)
		}

		// 所有命令的错误输出都到标准错误
		cmd.Stderr = os.Stderr
	}

	// 启动所有命令，并设置进程组
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
				Pgid:    j.pgid,
			}
		}

		if err := cmd.Start(); err != nil {
			return err
		}

		// 记录第一个进程的进程组ID
		if i == 0 {
			j.pgid = cmd.Process.Pid
		}
	}

	return nil
}

// waitPipeCommands 等待管道命令完成
func (j *Job) waitPipeCommands() error {
	// 等待进程组中的所有进程完成
	// 使用 Wait4 等待进程组
	for {
		var status syscall.WaitStatus
		// 等待进程组中的任意子进程
		pid, err := syscall.Wait4(-j.pgid, &status, 0, nil)
		if err != nil {
			// 如果没有更多子进程，退出循环
			if err == syscall.ECHILD {
				break
			}
			return err
		}

		// 检查是否所有进程都已完成
		allDone := true
		for _, cmd := range j.commands {
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
