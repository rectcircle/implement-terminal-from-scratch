import './style.css'
import '../node_modules/@xterm/xterm/css/xterm.css'
import { Terminal } from '@xterm/xterm'

async function main() {
  const terminal = new Terminal();
  terminal.open(document.querySelector('#app'));


  const wsConn = new WebSocket(`ws://localhost:8080/`);

  terminal.onData((data) => {
    wsConn.send(data);
  });

  wsConn.onmessage = (event) => {
    terminal.write(event.data);
  };
}

main();