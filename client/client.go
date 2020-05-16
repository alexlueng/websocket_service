package client

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"sanji_s12/commands"
	"time"
)

// s12要做的事（功能）：
// 作为客户端去连接s10
// 处理命令
// 向S10发送report指令
// 收发信息，带宽占用，连接状态信息
// 规定指令的格式（struct内的字段）
//conn.ReadMessage()
// 接收S10发送过来的指令 transfers11(针对s11) conn(xx-[yy,zz]) accept key s10告诉s12哪个设备将要连接s12
//conn.WriteMessage()

const s10URL = "ws://192.168.1.85:9910/?clienttype=s12"
//const s10URL = "ws://127.0.0.1:7777/ws"

var ErrorChan = make(chan struct{}, 1)

// 封装与s10的连接
type ClientConn struct {
	ServerType string
	LoginTime  int64 // 上线时间
	OnlineTime int64 // 在线时间
	Tm         int64 //最后一次通话的时间 毫秒
	Conn       *websocket.Conn
	WsURL      string
	CloseChan  chan struct{}
}

func NewClientConn(conn *websocket.Conn, wsURL string) *ClientConn {
	return &ClientConn{
		ServerType: "s12",
		Conn:       conn,
		WsURL:      wsURL,
		LoginTime:  time.Now().Unix(),
		//InCmdChan:  make(chan Cmd, 100),
		//OutCmdChan: make(chan Cmd, 100),
		CloseChan: make(chan struct{}, 1),
	}
}

// 读取指令
func (c *ClientConn) ReadCommand() {
	for {
		_, data, err := c.Conn.ReadMessage()
		if err != nil {
			fmt.Println("error while reading command: " + err.Error())
			// 断线重连
			c.Conn.Close()
			ErrorChan <- struct{}{}
			break
		}
		// 反序列化这条指令，把它放到指令管道中
		fmt.Println("reading bytes: ", string(data))
		cmd := commands.Cmd{}
		if len(data) > 0 {
			err = json.Unmarshal(data, &cmd)
			if err != nil {
				fmt.Println("error while decoding cmd: ", err.Error())
				continue
			}

			commands.InCmdChan <- cmd
		}
	}
}

// 发送指令
func (c *ClientConn) SendCommand() {
	for {
		select {
		case cmd := <-commands.OutCmdChan:
			jsonValue, err := json.Marshal(cmd)
			if err != nil {
				fmt.Println(err.Error())
				continue
			}
			fmt.Println("get data from outCmdChan: " + string(jsonValue))
			err = c.Conn.WriteMessage(websocket.TextMessage, jsonValue)
			if err != nil {
				fmt.Println("error while sending command: ", err.Error())
				// 断线重连
				ErrorChan <- struct{}{}
			}
		case <-c.CloseChan:
			return
		}
	}
}


// 断线重连
// (长时间没有数据发送的长连接容易被浏览器、移动中间商、nginx、服务端程序断开)
func Reconnect() {
	for {
		select {
		case <-ErrorChan:
			fmt.Println("断线重连中。。。")
			time.Sleep(5 * time.Second)
			var dialer *websocket.Dialer
			conn, _, err := dialer.Dial(s10URL, nil)
			if err != nil {
				ErrorChan <- struct{}{}
				continue
			}
			connection := NewClientConn(conn, s10URL)

			go connection.SendCommand()
			go connection.ReadCommand()


		}
	}
}

// s12启动后作为客户端的主入口
func WSClient() {

	// 建立与s10的websocket通信
	var dialer *websocket.Dialer
	conn, _, err := dialer.Dial(s10URL, nil)
	if err != nil {
		fmt.Println("error while connect s10 in the first time: ", err)
		ErrorChan <- struct{}{}
		// 断线重连
		go Reconnect()
		return
	}

	fmt.Println("Establish connection with s10")
	connection := NewClientConn(conn, s10URL)
	connection.LoginTime = time.Now().Unix()

	// 这里是发消息的协程，向S10发送report指令
	go connection.SendCommand()

	// 读取S10下发的指令协程
	go connection.ReadCommand()

	// 断线重连
	go Reconnect()


}
