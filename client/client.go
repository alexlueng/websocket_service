package client

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gorilla/websocket"
	"sanji_s12/commands"
	"sanji_s12/util"
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

const s10URL = "ws://127.0.0.1:7777/ws"

var ErrorChan = make(chan struct{}, 1)

// 封装与s10的连接
type ClientConn struct {
	ServerType string
	LoginTime  int64 // 上线时间
	OnlineTime int64 // 在线时间
	Tm         int64 //最后一次通话的时间 毫秒
	Conn       *websocket.Conn
	WsURL      string
	ErrorChan  chan struct{}
}

func NewClientConn(conn *websocket.Conn, wsURL string) *ClientConn {
	return &ClientConn{
		ServerType: "s12",
		Conn:       conn,
		WsURL:      wsURL,
		LoginTime:  time.Now().Unix(),
		//InCmdChan:  make(chan Cmd, 100),
		//OutCmdChan: make(chan Cmd, 100),
		ErrorChan: make(chan struct{}, 1),
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
		fmt.Println("reading bytes: ", data)
		cmd := commands.Cmd{}
		if len(data) > 0 {
			err = json.Unmarshal(data, &cmd)
			if err != nil {
				fmt.Println("error while decoding cmd: ", err.Error())
				continue
			}

			util.SmartPrint(cmd)

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
		default:
		}
	}
}

// 处理指令
// 处理指令是这个客户端该做的事情吗？好像不是，客户端只负责读取指令和发送指令
// 这个函数的功能应该是服务端来处理的
func (c *ClientConn) HandleCommand() {
	for {
		select {
		case cmd := <-commands.InCmdChan:
			switch cmd.Cmd {
			case "set":
				// set指令是接收s10发送过来的key，这个key会变的吗？
				// 将这个key存到系统的内存中
				key := cmd.Data
				commands.S12Key = key
			case "connect":
				// connect微指令，s10告知s12要为哪两台设备搭建一条收发数据的管道
				// 建立管道的流程就像是建立一个聊天室，只不过这个聊天室比较特殊：
				// 只有两个人
				// 一个在不停地说话， 一个在不停地接收
				// 需求：如何搭建这个管道，可以快速地找到并关闭
				//fromKey := cmd.From
				//toKey := cmd.To
				// 为这两个key建立一条管道，如何建立才能方便断开
				// 如何管理这些管道
				// 1 从fromKey中读数据，然后写到toKey中
				//go EstabilshPipe(fromKey, toKey)
			case "disconnect":
				// 把connect指令建立的map断开
				//fromKey := cmd.From
				//toKey := cmd.To
			case "reset":
				// 重置所有连接，保留data字段内的连接，其余的都关闭
			case "close":
				// 关闭data字段内的连接
			case "accept":
				// s10告知s12哪个设备可以连接
				key := cmd.Data
				commands.PermissionKey = append(commands.PermissionKey, key)
			default:
				fmt.Println("Don't know what command it is.")
			}
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

var addr = flag.String("addr", "127.0.0.1:9910", "websocket service address")

// s12启动后作为客户端的主入口
func WSClient() {

	// 建立与s10的websocket通信
	var dialer *websocket.Dialer
	conn, _, err := dialer.Dial(s10URL, nil)
	if err != nil {
		fmt.Println("error while connect s10 in the first time: ", err)
		// 断线重连
		return
	}

	fmt.Println("Establish connection with s10")
	connection := NewClientConn(conn, *addr)
	connection.LoginTime = time.Now().Unix()

	// 连接上S10之后，就开始收发数据，暂不考虑断线重连的问题
	// TODO: 断线重连

	// 这里是发消息的协程，向S10发送report指令
	go connection.SendCommand()

	// 读取S10下发的指令协程
	go connection.ReadCommand()

	// 处理命令的协程
	//go connection.HandleCommand()

	// 断线重连
	go Reconnect()

	// 首次连接的时候上报当前的连接信息
	// 组装一条report指令，放到指令channel中
	firstReport := commands.Cmd{
		CmdId:   0,
		Cmd:     "report",
		Role:    "client",
		Forward: "",
		Url:     "",
		From:    "",
		To:      "",
		Rtmp:    "",
		File:    "",
		Data:    "", // data里还包含了当前的连接情况，连接总数，
	}

	reportData := commands.ReportData{
		Type: "online",
		Data: "",
	}

	//type Client struct {
	//	DeviceId   string
	//	DeviceType string // 设备类型 1:winapp,2:deviceapp，3:phoneapp
	//	Key        string // 设备key 作为设备的唯一标识
	//	Mac        string
	//	IP         string
	//	Tm         int64 //最后一次通话的时间 毫秒
	//}

	connStatus := commands.ConnectStatus{
		Client: map[key]Client, // 设备类型
		ServerInfo: {}, // 服务器信息
		Routers: map[string][]string, // 路由信息
		WhiteList: []string{}, // 白名单
		Active: 0,
	}


	connStatusJson, _ := json.Marshal(connStatus)
	reportData.Data = result

	reportDataJson, _ := json.Marshal(reportData)
	firstReport.Data = string(reportDataJson)

	commands.OutCmdChan <- firstReport

}
