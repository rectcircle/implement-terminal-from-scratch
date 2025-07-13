import './style.css'
import '../node_modules/@xterm/xterm/css/xterm.css'
import { Terminal } from '@xterm/xterm'

const terminalASNIEscapeSeqDemo = 
`这是正常的 ASNI 编码的字符串 (UTF8)，在终端中会被原样渲染\r\n` +
`在终端里面必须使用\\r(回车)\\n(换行)进行换行操作\r\n` + 
"终端可以对文字进行修饰，此时就需要使用 escape code，如：\x1B[1;3;31m粗体斜体红色前景色\x1B[0m\r\n" + 
"    首先 escape code 是 \\x1B (ESC) 字符告诉终端接下来是一个逃逸指令\r\n" +
"    然后 [ 表示这是一个控制序列 (CSI) 后面需要跟随着参数\r\n" +
"    1;3;31 表示 1 是粗体，3 斜体，31 是 31 号颜色红色前景色\r\n" +
"    m 表示参数结束，告诉终端可以进行渲染了\r\n" +
"    后面可以跟随着任意的 UTF8 编码的字符串，会被渲染为红色前景色\r\n" +
"    最后 \\x1B[0m 也是一个 CSI 指令，0 表示重置所有参数\r\n" + 
"    总结来说: \\x1B[数字;数字;...m 用来设置如何渲染接下来的文本\r\n" +
"除了 CSI 指令，还有很多其他指令，如：\r\n" +
"    \\x1bc 清屏指令，实现类似于 clear 的效果\r\n" +
"    发送特殊字符如 \\x1bD (回车) \\x1bE (换行) 等\r\n" +
"    光标操作：\r\n" +
"        \\x1B[1A 上移一行\r\n" +
"        \\x1B[1B 下移一行\r\n" +
"        \\x1B[1C 右移一列\r\n" +
"        \\x1B[1D 左移一列 *\x1B[1B\x1B[1C" +
"这段文字应该打印在 * 号的右下角\r\n"+
"";

async function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

async function main() {
    const terminal = new Terminal();
    terminal.open(document.querySelector('#app'));
    // 遍历 terminalASNIEscapeSeqDemo
    for (const char of terminalASNIEscapeSeqDemo) {
        terminal.write(char);
        await sleep(2);
    }
}

main();