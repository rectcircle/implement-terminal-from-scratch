package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
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

	job := &Job{}

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
	// 不管怎样，都需要获取当前进程的进程组ID，并将其设置为前台进程组
	// 先获取当前的进程组 ID
	currentPgid, err := unix.Getpgid(0)
	if err != nil {
		return fmt.Errorf("Execute job, get pgid failed: %s", err)
	}
	defer func() {
		// 需要忽略 SIGTTOU 信号，否则会导致前台进程组切换失败，原因如下：
		// 1. Unix 系统为了安全，当调用 TIOCSPGRP 的进程不在前台进程组时，会发送 SIGTTOU 信号，而 SIGTTOU 的默认行为是退出进程。
		//    因为 TIOCSPGRP 是给 Shell 程序调用的，如果普通程序调用这个函数，会破坏 Shell 的作业管理，因此 Unix 系统才设计了这个机制。
		// 2. 我们实现的这个程序就是一个 Shell，因此就是要调用 TIOCSPGRP 的，因此需要避免 SIGTTOU 信号的影响，有两种办法。
		//    a. 忽略这个信号，这里采用这个方案。
		//    b. 通过 sigprocmask 屏蔽这个信号（这里需要说一下，对于其他信号，屏蔽信号只是延后信号的处理，但是对于 SIGTTOU 信号，屏蔽了之后，就不会再产生了） Go 的 syscall.SysProcAttr.Foreground 通过该方案实现。
		signal.Ignore(syscall.SIGTTOU)
		defer signal.Reset(syscall.SIGTTOU)
		err = unix.IoctlSetPointerInt(int(os.Stdin.Fd()), unix.TIOCSPGRP, currentPgid)
		if err != nil {
			panic(err)
		}
	}()
	// 获取当前进程组
	err = j.Start()
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

	// 统一处理所有命令，都创建进程组
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
			// 第一个进程作为进程组组长，并将该进程组设置为前台
			cmd.SysProcAttr = &syscall.SysProcAttr{
				Setpgid: true,
				Pgid:    0, // 0 表示使用进程自己的PID作为进程组ID
				// 实现原理是：
				// 1. 在 fork 之前，调用 sigprocmask 屏蔽了所有信号 (runtime/proc.go syscall_runtime_BeforeFork)。
				// 2. 在 fork 之后 exec 之前：
				//    a. 调用 TIOCSPGRP 将子进程进程组设置为 session 的前台进程组 (syscall/exec_libc2.go forkAndExecInChild)。
				//    b. 调用 msigrestore 恢复到信号屏蔽集 (runtime/proc.go syscall_runtime_AfterForkInChild)。
				Foreground: true, // 将当前进程组设置为 session 的前台进程组
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

// Wait 等待Job中的所有进程退出
func (j *Job) Wait() error {
	if len(j.commands) == 0 {
		return nil
	}

	// 统一使用进程组等待
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

// CanEnableJobControl 判断当前进程是否可以启用作业控制
func (k *JobController) CanEnableJobControl() bool {
	// 检查是否有控制终端
	if !isatty(os.Stdin.Fd()) {
		return false
	}

	// 获取当前进程的进程组ID
	currentPgid, err := unix.Getpgid(0)
	if err != nil {
		return false
	}

	// 获取前台进程组ID
	pgrpid := syscall.Getpgrp()

	// 如果当前进程组就是前台进程组，则可以启用作业控制
	return currentPgid == pgrpid
}

// isatty 检查文件描述符是否是终端
func isatty(fd uintptr) bool {
	_, err := unix.IoctlGetTermios(int(fd), ioctlReadTermios)
	return err == nil
}
