// #define _GNU_SOURCE
#include <stdio.h>       // printf, perror, snprintf
#include <stdlib.h>      // exit
#include <string.h>      // strlen
#include <unistd.h>      // fork, close, read, write, dup2, execl, setsid
#include <fcntl.h>       // open, O_RDWR, O_NOCTTY
#include <sys/wait.h>    // waitpid
// #include <pty.h>         // openpty, unlockpt, ptsname_r (POSIX PTY API)
#include <signal.h>      // kill, SIGTERM
#include <time.h>        // nanosleep, timespec
#include <sys/ioctl.h>   // ioctl, TIOCSPTLCK, TIOCGPTN, TIOCSCTTY

// 定义输入序列，与Go版本保持一致
const char* ANSI_INPUT_SEQ_DEMO = 
    "hello world\r"  // 第一行: 常规的 ascii 字符，应用程序原样接受
    "中文\r"        // 第二行：中文字符，行为和第一行一样，应用程序原样接受
    "  对于可打印字符(中英文)\r"
    "    1.在应用程序接受之前已经打印了，这是行规程的回显功能\r"
    "    2.行规程原样透传到应用程序\r"
    "    3.行规程将 \\r 转换为 \\n 传递给应用程序\r"
    "    4.行规程有一个行 buffer 遇到 \\r 才会将 buffer 的内容传递给应用程序\r"
    "测试行编辑(按退格的效果\x7f): hello world,\x7f!\r"
    "  可以看出，\\x7f 删除了前面的逗号, 应用程序接受到的是 hello world!\r"
    "测试行编辑(按方向键效果): world\x1b[D\x1b[D\x1b[D\x1b[D\x1b[Dhello \r"
    "  可以看出，方向键不会影响行规程的行编辑\r"
    "* 即将发送 ctrl+c 信号，应用程序将收到 SIGINT(2) 信号\r"
    "\x03";  // 最后一行：ctrl+c 信号

void sleep_ms(int ms) {
    struct timespec ts;
    ts.tv_sec = ms / 1000;
    ts.tv_nsec = (ms % 1000) * 1000000;
    nanosleep(&ts, NULL);
}

int main() {
    int master_fd, slave_fd;
    pid_t pid;
    char slave_name[256];
    
    // 如下是 POSIX PTY API 实现
    // if (openpty(&master_fd, &slave_fd, slave_name, NULL, NULL) == -1) {
    //     perror("openpty failed");
    //     exit(1);
    // }
    
    // 如下是 Linux 原生方式创建PTY
    // 打开 master 端
    master_fd = open("/dev/ptmx", O_RDWR | O_NOCTTY);
    if (master_fd == -1) {
        perror("open /dev/ptmx failed");
        exit(1);
    }
    
    // 解锁 slave 端
    // if (unlockpt(master_fd) == -1) {
    //     perror("unlockpt failed");
    //     close(master_fd);
    //     exit(1);
    // }
    
    // 使用 ioctl 解锁 slave 端
    int unlock = 0;
    if (ioctl(master_fd, TIOCSPTLCK, &unlock) == -1) {
        perror("ioctl TIOCSPTLCK failed");
        close(master_fd);
        exit(1);
    }

    // 获取 slave 端名称
    // if (ptsname_r(master_fd, slave_name, sizeof(slave_name)) != 0) {
    //     perror("ptsname_r failed");
    //     close(master_fd);
    //     exit(1);
    // }

    // 使用ioctl获取PTY编号并构造slave名称
    unsigned int pty_num;
    if (ioctl(master_fd, TIOCGPTN, &pty_num) == -1) {
        perror("ioctl TIOCGPTN failed");
        close(master_fd);
        exit(1);
    }
    snprintf(slave_name, sizeof(slave_name), "/dev/pts/%u", pty_num);

    printf("pty slave path is: %s\n", slave_name);

    // 4. 打开slave端
    slave_fd = open(slave_name, O_RDWR | O_NOCTTY);
    if (slave_fd == -1) {
        perror("open slave failed");
        close(master_fd);
        exit(1);
    }
    
    printf("PTY slave file path: %s\n", slave_name);
    
    // Fork子进程
    pid = fork();
    if (pid == -1) {
        perror("fork failed");
        exit(1);
    }
    
    if (pid == 0) {
        // 子进程：设置为slave端并执行echo-stdin-json-str
        close(master_fd);
        
        // 创建新会话
        if (setsid() == -1) {
            perror("setsid failed");
            exit(1);
        }
        
        // 使用ioctl设置slave_fd为控制终端
        if (ioctl(slave_fd, TIOCSCTTY, 0) == -1) {
            perror("ioctl TIOCSCTTY failed");
            exit(1);
        }

        // 重定向标准输入输出到slave
        if (dup2(slave_fd, STDIN_FILENO) == -1 ||
            dup2(slave_fd, STDOUT_FILENO) == -1 ||
            dup2(slave_fd, STDERR_FILENO) == -1) {
            perror("dup2 failed");
            exit(1);
        }
        
        close(slave_fd);
        
        // 执行echo-stdin-json-str程序
        execl("./echo-stdin-json-str", NULL);
        perror("execl failed");
        exit(1);
    } else {
        // 父进程：作为 master 端
        close(slave_fd);
        
        // 创建子进程来读取 PTY 输出并打印到 stdout
        pid_t reader_pid = fork();
        if (reader_pid == 0) {
            // 读取子进程
            char buffer[1024];
            ssize_t bytes_read;
            
            while ((bytes_read = read(master_fd, buffer, sizeof(buffer))) > 0) {
                write(STDOUT_FILENO, buffer, bytes_read);
            }
            exit(0);
        }
        
        // 发送输入序列
        const char* input = ANSI_INPUT_SEQ_DEMO;
        size_t len = strlen(input);
        
        for (size_t i = 0; i < len; i++) {
            if (write(master_fd, &input[i], 1) == -1) {
                perror("write to master failed");
                break;
            }
            sleep_ms(10);  // 10毫秒延迟，与Go版本一致
        }
        
        // 等待子进程结束
        int status;
        waitpid(pid, &status, 0);
        
        // 终止读取进程
        kill(reader_pid, SIGTERM);
        waitpid(reader_pid, NULL, 0);
        
        close(master_fd);
    }
    
    return 0;
}