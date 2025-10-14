package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// Job 状态常量
const (
	JobStatusRunning = "Running"
	JobStatusStopped = "Stopped"
	JobStatusDone    = "Done"
)

type JobController struct {
	// 当前 shell 所在的前台进程组 ID
	shellForegroundPgid int
	// 运行中的 job id (从 1 开始)
	runningJobIds map[int]*Job
}

func NewJobController() (*JobController, error) {
	currentPgid, err := unix.Getpgid(0)
	if err != nil {
		return nil, fmt.Errorf("Execute job, get pgid failed: %s", err)
	}

	return &JobController{
		shellForegroundPgid: currentPgid,
		runningJobIds:       make(map[int]*Job),
	}, nil
}

// CanEnableJobControl 判断当前进程是否可以启用作业控制
func (k *JobController) CanEnableJobControl() bool {
	// 检查是否有控制终端
	if !isatty(os.Stdin.Fd()) {
		return false
	}
	// 获取前台进程组ID
	pgrpid := syscall.Getpgrp()

	// 如果当前进程组就是前台进程组，则可以启用作业控制
	return k.shellForegroundPgid == pgrpid
}

func (k *JobController) ForceSetShellForeground() {
	// 需要忽略 SIGTTOU 信号，否则会导致前台进程组切换失败，原因如下：
	// 1. Unix 系统为了安全，当调用 TIOCSPGRP 的进程不在前台进程组时，会发送 SIGTTOU 信号，而 SIGTTOU 的默认行为是退出进程。
	//    因为 TIOCSPGRP 是给 Shell 程序调用的，如果普通程序调用这个函数，会破坏 Shell 的作业管理，因此 Unix 系统才设计了这个机制。
	// 2. 我们实现的这个程序就是一个 Shell，因此就是要调用 TIOCSPGRP 的，因此需要避免 SIGTTOU 信号的影响，有两种办法。
	//    a. 忽略这个信号，这里采用这个方案。
	//    b. 通过 sigprocmask 屏蔽这个信号（这里需要说一下，对于其他信号，屏蔽信号只是延后信号的处理，但是对于 SIGTTOU 信号，屏蔽了之后，就不会再产生了） Go 的 syscall.SysProcAttr.Foreground 通过该方案实现。
	signal.Ignore(syscall.SIGTTOU)
	defer signal.Reset(syscall.SIGTTOU)
	err := unix.IoctlSetPointerInt(int(os.Stdin.Fd()), unix.TIOCSPGRP, k.shellForegroundPgid)
	if err != nil {
		panic(err)
	}
}

// NewJob 新建一个 Job，返回 JobID
func (k *JobController) AddJob(input string) (int, error) {
	job, err := NewJob(input)
	if err != nil {
		return 0, err
	}
	var jobId = 1
	for ; ; jobId++ {
		if _, ok := k.runningJobIds[jobId]; !ok {
			k.runningJobIds[jobId] = job
			break
		}
	}

	return jobId, nil
}

// Execute 解析并执行命令，支持管道符
func (k *JobController) Execute(input string) error {
	// 前置流程：检查后台进程是否执行完成
	for _, jobId := range k.sortedJobIds() {
		job := k.runningJobIds[jobId]
		if job.background {
			err := job.Wait(true)
			if err != nil {
				return err
			}
			var statusStr = ""
			if job.exitCode == -1 {
				// 进程还在运行
				continue
			} else if job.exitCode == 0 {
				statusStr = JobStatusDone
			} else {
				statusStr = fmt.Sprintf("Exit %d", job.exitCode)
			}
			fmt.Printf("[%d] %s                  %s\n", jobId, statusStr, job.commandStr)
			delete(k.runningJobIds, jobId)
		}
	}

	// 空字符串啥都不做
	if input == "" {
		return nil
	}

	// 尝试执行内建命令
	isBuiltin, err := k.tryExecuteBuiltinCommand(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		return nil
	}
	if isBuiltin {
		return nil
	}

	// 创建 Job
	jobId, err := k.AddJob(input)
	if err != nil {
		return err
	}
	job := k.runningJobIds[jobId]
	// 启动 Job
	err = job.Start()
	if err != nil {
		delete(k.runningJobIds, jobId)
		return err
	}
	// 前台执行
	if !job.background {
		return k.waitForegroundJob(jobId, job)
	}
	// 后台执行
	fmt.Printf("[%d] %d\n", jobId, job.pgid)
	return nil
}

func (k *JobController) sortedJobIds() []int {
	keys := make([]int, 0, len(k.runningJobIds))
	for id := range k.runningJobIds {
		keys = append(keys, id)
	}
	sort.Ints(keys) // 升序
	return keys
}

// tryExecuteBuiltinCommand 处理内置命令，返回是否是内置命令
func (k *JobController) tryExecuteBuiltinCommand(input string) (bool, error) {
	args := strings.Fields(input)
	if len(args) == 0 {
		return false, nil
	}

	switch args[0] {
	case "jobs":
		return true, k.handleJobsCommand()
	case "bg":
		if len(args) != 2 {
			return true, fmt.Errorf("bg: usage: bg <jobid>")
		}
		return true, k.handleBgCommand(args[1])
	case "fg":
		if len(args) != 2 {
			return true, fmt.Errorf("fg: usage: fg <jobid>")
		}
		return true, k.handleFgCommand(args[1])
	default:
		return false, nil
	}
}

// handleJobsCommand 处理 jobs 命令
func (k *JobController) handleJobsCommand() error {
	for _, jobId := range k.sortedJobIds() {
		job := k.runningJobIds[jobId]
		var statusStr string
		if job.exitCode == -1 {
			statusStr = JobStatusRunning
		} else if job.exitCode == -2 {
			statusStr = JobStatusStopped
		} else if job.exitCode == 0 {
			statusStr = JobStatusDone
		} else {
			statusStr = fmt.Sprintf("Exit %d", job.exitCode)
		}

		commandStr := job.commandStr
		if job.exitCode == -1 && job.background {
			commandStr += " &"
		}

		fmt.Printf("[%d] %s                  %s\n", jobId, statusStr, commandStr)
	}
	return nil
}

// handleBgCommand 处理 bg 命令
func (k *JobController) handleBgCommand(jobIdStr string) error {
	jobId, err := strconv.Atoi(jobIdStr)
	if err != nil {
		return fmt.Errorf("bg: invalid job id: %s", jobIdStr)
	}

	job, exists := k.runningJobIds[jobId]
	if !exists {
		return fmt.Errorf("bg: job %d not found", jobId)
	}

	if job.exitCode != -2 {
		return fmt.Errorf("bg: job %d is not stopped", jobId)
	}

	// 向进程组发送 SIGCONT 信号
	err = syscall.Kill(-job.pgid, syscall.SIGCONT)
	if err != nil {
		return fmt.Errorf("bg: failed to send SIGCONT to job %d: %v", jobId, err)
	}

	// 更新 Job 状态
	job.exitCode = -1
	job.background = true

	// 打印 Job 信息
	fmt.Printf("[%d] %s &\n", jobId, job.commandStr)

	return nil
}

// handleFgCommand 处理 fg 命令
func (k *JobController) handleFgCommand(jobIdStr string) error {
	jobId, err := strconv.Atoi(jobIdStr)
	if err != nil {
		return fmt.Errorf("fg: invalid job id: %s", jobIdStr)
	}

	job, exists := k.runningJobIds[jobId]
	if !exists {
		return fmt.Errorf("fg: job %d not found", jobId)
	}

	// 打印命令字符串
	fmt.Printf("%s\n", job.commandStr)

	// 如果 Job 处于 Stop 状态，先发送 SIGCONT 信号
	if job.exitCode == -2 {
		err = syscall.Kill(-job.pgid, syscall.SIGCONT)
		if err != nil {
			return fmt.Errorf("fg: failed to send SIGCONT to job %d: %v", jobId, err)
		}
	}

	// 将 Job 的进程组设置为前台进程组
	signal.Ignore(syscall.SIGTTOU)
	defer signal.Reset(syscall.SIGTTOU)
	err = unix.IoctlSetPointerInt(int(os.Stdin.Fd()), unix.TIOCSPGRP, job.pgid)
	if err != nil {
		return fmt.Errorf("fg: failed to set job %d as foreground: %v", jobId, err)
	}

	// 更新 Job 状态
	job.background = false
	job.exitCode = -1

	// TODO: 走统一的 wait 前台进程逻辑，也需要支撑 ctrl + z
	// 阻塞等待 Job 退出或暂停
	return k.waitForegroundJob(jobId, job)
}

// waitForegroundJob 等待前台 Job 执行完成，并处理清理逻辑
func (k *JobController) waitForegroundJob(jobId int, job *Job) error {
	defer func() { // 执行结束后，从 job 列表中删除
		if job.exitCode != -1 && job.exitCode != -2 {
			delete(k.runningJobIds, jobId)
		}
	}()
	defer k.ForceSetShellForeground() // 执行结束后强制把 shell 进程设置为前台

	err := job.Wait(false)
	if err != nil {
		return err
	}

	// 判断是否是 stop 状态
	if job.exitCode == -2 {
		fmt.Printf("[%d] %s                  %s\n", jobId, JobStatusStopped, job.commandStr)
	}

	return nil
}

// Job 表示一个作业，包含单个命令或管道命令
type Job struct {
	commandStr string          // 命令字符串
	commands   []*exec.Cmd     // 命令列表，每个元素是一个 *exec.Cmd
	pgid       int             // 进程组ID
	pipes      []io.ReadCloser // 管道连接
	exitCode   int             // Job 整体退出码：-1 运行中，-2 已暂停，其他值为退出码
	background bool            // 是否是后台 job
}

// NewJob 创建一个新的Job，解析命令字符串
func NewJob(commandStr string) (*Job, error) {
	commandStr = strings.TrimSpace(commandStr)
	background := false
	if strings.HasSuffix(commandStr, "&") {
		background = true
		commandStr = strings.TrimSuffix(commandStr, "&")
	}

	// 按管道符分割命令
	pipeCommands := strings.Split(commandStr, "|")
	if len(pipeCommands) == 0 {
		return nil, nil
	}

	job := &Job{}
	job.commandStr = commandStr
	job.background = background
	job.exitCode = -1

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
				Foreground: !j.background, // 将当前进程组设置为 session 的前台进程组
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

func (j *Job) Wait(wnohang bool) error {
	if j.exitCode != -1 {
		return nil
	}
	if len(j.commands) == 0 {
		j.exitCode = 0
		return nil
	}
	// 调用 wait 命令检查进程状态
	waitOptions := syscall.WUNTRACED // 添加 WUNTRACED
	if wnohang {
		waitOptions |= syscall.WNOHANG // 使用位或操作
	}

	for i, cmd := range j.commands {
		if cmd.Process == nil {
			// 进程还没有启动
			continue
		}
		var wstatus syscall.WaitStatus
		wpid, err := syscall.Wait4(cmd.Process.Pid, &wstatus, waitOptions, nil)
		// Wait4 出错，可能是进程不存在或权限问题
		if err != nil {
			if err == syscall.ECHILD {
				// 进程已经不存在了
				continue
			}
			// 未知错误，直接抛异常
			return err
		}
		if wpid == 0 {
			// WNOHANG 且没有子进程状态变化，说明进程还在运行
			continue
		}
		// 检查进程是否被暂停
		if wstatus.Stopped() {
			j.exitCode = -2
			// 对于暂停的进程，不设置 exitCode
			return nil
		}

		if i == len(j.commands)-1 {
			// 最后一个命令的退出码作为 job 的退出码
			j.exitCode = wstatus.ExitStatus()
		}
	}
	return nil
}

// isatty 检查文件描述符是否是终端
func isatty(fd uintptr) bool {
	_, err := unix.IoctlGetTermios(int(fd), ioctlReadTermios)
	return err == nil
}
