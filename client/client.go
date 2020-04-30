package client

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gorilla/websocket"
	"sanji_s12/models"
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

const s10URL = "ws://127.0.0.1:9910/?clienttype=s12"

//websocket 发送的指令数据格式
type Cmd struct {
	CmdId       int64  `json:"cmdid"`
	Cmd         string `json:"cmd"`
	Role        string `json:"role"`    //client 表示作为客户端，server 表示作为服务端
	Forward     string `json:"forward"` // 值为空表示是给自己的指令，否则是目标的唯一id，S1需要对其转发
	Url         string `json:"url"`
	From        string `json:"from"` // 从 from 过来的数据
	To          string `json:"to"`   // 发给 To
	Rtmp        string `json:"rtmp"`
	File        string `json:"file"` //如果有File值就要保存文件，此指令不中断之前的操作。
	Data        string `json:"data"` //
	FromConnKey string               // 发送方的连接，这个字段自用
}

// 设备上线，下线的数据格式
type ConnectStatus struct {
	ID        int64  `json:"id"` // 设备ID
	Key       string `json:"key"`
	IP        string `json:"ip"`
	Duration  int64  `json:"duration"`   // 连接时长
	LastAlive int64  `json:"last_alive"` //最后活跃时间
	Status    string `json:"status"`     // up down
}

type AllConnections struct {
	ConnectionKeys []string `json:"connection_keys"`
	Total int64 `json:"total"` // 连接总数
	Active int64 `json:"active"` // 正在接发数据的连接
}

type ServerStatus struct {
	CpuInfo int64 `json:"cpu_info"`
	DiskInfo int64 `json:"disk_info"`
}

//var reconnectChan chan bool

type ClientConn struct {
	ServerType string
	OnlineTime int64
	Tm         int64 //最后一次通话的时间 毫秒
	Conn       *websocket.Conn
	WsURL      string
	InCmdChan  chan Cmd // 放置指令的管道
	OutCmdChan chan Cmd // 读取指令的管道
	ErrorChan  chan struct{}
}

func NewClientConn(conn *websocket.Conn, wsURL string) *ClientConn {
	return &ClientConn{
		ServerType: "s12",
		Conn:       conn,
		WsURL:      wsURL,
		InCmdChan:  make(chan Cmd, 100),
		OutCmdChan: make(chan Cmd, 100),
		ErrorChan:  make(chan struct{}, 1),
	}
}

// 读取指令
func (c *ClientConn) ReadCommand() {
	for {
		_, data, err := c.Conn.ReadMessage()
		if err != nil {
			fmt.Println("error while reading command: " + err.Error())
			// 断线重连
			c.ErrorChan <- struct{}{}
			c.Conn.Close()
			continue
		}
		// 反序列化这条指令，把它放到指令管道中
		fmt.Println("reading bytes: ", data)
		cmd := Cmd{}
		if len(data) > 0 {
			err = json.Unmarshal(data, &cmd)
			if err != nil {
				fmt.Println("error while decoding cmd: ", err.Error())
				continue
			}
			c.InCmdChan <- cmd
		}
	}
}

// 发送指令
func (c *ClientConn) SendCommand() {
	for {
		select {
		case cmd := <-c.OutCmdChan:
			jsonValue, err := json.Marshal(cmd)
			if err != nil {
				fmt.Println(err.Error())
				break
			}
			err = c.Conn.WriteMessage(websocket.TextMessage, jsonValue)
			if err != nil {
				fmt.Println("error while sending command: ", err.Error())
				// 断线重连
			}
		}
	}
}

// 处理指令
func (c *ClientConn) HandleCommand() {
	for {
		select {
		case cmd := <-c.InCmdChan:
			switch cmd.Cmd {
			case "set":
				// set指令是接收s10发送过来的key，这个key会变的吗？
				// 将这个key存到系统的内存中
				key := cmd.Data
				models.S12Key = key
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
				models.PermissionKey = append(models.PermissionKey, key)
			default:
				fmt.Println("Don't know what command it is.")
			}
		}
	}
}

// 断线重连
// (长时间没有数据发送的长连接容易被浏览器、移动中间商、nginx、服务端程序断开)
func (c *ClientConn) Reconnect() {

	for {
		select {
		case <-c.ErrorChan:
			fmt.Println("断线重连中。。。")
			var dialer *websocket.Dialer
			conn, _, err := dialer.Dial(c.WsURL, nil)
			if err != nil {
				c.ErrorChan <- struct{}{}
				continue
			}
			c.Conn = conn
		}
	}
}

var addr = flag.String("addr", "127.0.0.1:9910", "websocket service address")

func WSClient() {
	//u := url.URL{Scheme: "ws", Host: *addr, Path: "/clienttype=s10"}
	//fmt.Println(u.String())
	var dialer *websocket.Dialer

	conn, _, err := dialer.Dial(s10URL, nil)
	if err != nil {
		fmt.Println("error while connect s10 in the first time: ", err)
		// 断线重连
		return
	}

	fmt.Println("Establish connection with s10")
	connection := NewClientConn(conn, *addr)

	// 首次连接的时候上报当前的连接信息
	// 连接上S10之后，就开始收发数据，暂不考虑断线重连的问题
	// TODO: 断线重连

	// 这里是发消息的协程，向S10发送report指令
	go connection.SendCommand()

	// 读取S10下发的指令协程
	go connection.ReadCommand()

	// 处理命令的协程
	go connection.HandleCommand()

	// 断线重连
	go connection.Reconnect()
}
