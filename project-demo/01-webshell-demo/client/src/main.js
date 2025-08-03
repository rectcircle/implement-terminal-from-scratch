// 引入项目 css
import './style.css'
// 引入 xterm 的 css
import '../node_modules/@xterm/xterm/css/xterm.css'
// 引入 xterm.js
import { Terminal } from '@xterm/xterm'


// 逻辑
async function main() {
  // 创建一个终端实例
  const terminal = new Terminal();
  terminal.open(document.querySelector('#app'));

  // 创建 websocket client ， 连接到 server。
  const wsConn = new WebSocket(`ws://localhost:8080/`);

  // 从 terminal 获取到的用户输入的 ANSI escape 字符流，发送给服务端。
  terminal.onData((data) => {
    // 打印日志
    console.log("terminal->ws: "+JSON.stringify(data) + " [" + (new TextEncoder()).encode(data) + "]");
    wsConn.send(data);
  });
  
  // 从 websocket 读取服务端返回的 ANSI escape 字符流，写入终端中。
  wsConn.onmessage = (event) => {
    // 打印日志
    console.log("ws->terminal: "+JSON.stringify(event.data) + " [" + (new TextEncoder()).encode(event.data) + "]");
    terminal.write(event.data);
  };

  // 其他： 略
  wsConn.onerror = (event) => {
    console.error('WebSocket error: ', event);
    // TODO: 错误处理
  }
  wsConn.onclose = (event) => {
    // TODO: 关闭处理
  }

}

main();