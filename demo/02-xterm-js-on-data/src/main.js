import './style.css'
import '../node_modules/@xterm/xterm/css/xterm.css'
import { Terminal } from '@xterm/xterm'

const terminalASNIEscapeSeqDemo = `随意按键盘观察 xterm.js onData 的输出: `;

async function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

async function main() {
  const terminal = new Terminal();
  terminal.open(document.querySelector('#terminal'));
  const xtermJsOnDataCode = document.querySelector('#xterm_js_on_data_code');

  terminal.onData((data) => {
    xtermJsOnDataCode.textContent += JSON.stringify(data) + " " + data.charCodeAt(0) + "\n";
  });

  for (const char of terminalASNIEscapeSeqDemo) {
    terminal.write(char);
    await sleep(100);
  }

}

main();