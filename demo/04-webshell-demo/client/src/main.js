import './style.css'
import '../node_modules/@xterm/xterm/css/xterm.css'
import { Terminal } from '@xterm/xterm'

async function main() {
  const terminal = new Terminal();
  terminal.open(document.querySelector('#app'));


  const wsConn = new WebSocket(`ws://localhost:8080/`);

  terminal.onData((data) => {
    console.log("terminal->ws: "+JSON.stringify(data) + " " + data.charCodeAt(0));
    wsConn.send(data);
  });
  
  wsConn.onmessage = (event) => {
    console.log("ws->terminal: "+JSON.stringify(event.data));
    terminal.write(event.data);
  };
}

main();