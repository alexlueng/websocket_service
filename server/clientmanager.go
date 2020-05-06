package server

import (
	"encoding/json"
	"fmt"
	"sanji_s12/commands"
	"strings"
	"time"
)
// 连接管理器
// 这个管理器，需要处理各种指令
// 需要管理设备上线下线的情况
type ClientManager struct {
	Clients    map[*Client]bool
	Broadcast  chan []byte
	Register   chan *Client // 设备连接
	Unregister chan *Client // 设备下线
}

// 开始处理连接
// 设备上线下线的情况
func (manager *ClientManager) Start() {
	for {
		select {
		// 设备上线
		case conn := <-manager.Register:
			manager.Clients[conn] = true
			jsonMessage, _ := json.Marshal(&Message{Content: "/A new socket has connected. The key is: " + conn.Key})
			manager.send(jsonMessage, conn)

			// TODO：生成一条report指令，放到writeCmdChan中

		// 设备下线
		case conn := <-manager.Unregister:
			if _, ok := manager.Clients[conn]; ok {
				close(conn.Read)
				close(conn.Write)
				delete(manager.Clients, conn)
				jsonMessage, _ := json.Marshal(&Message{Content: "/A socket has disconnected."})
				manager.send(jsonMessage, conn)

				// TODO：生成一条report指令，放到writeCmdChan中
			}
		case message := <-manager.Broadcast:
			for conn := range manager.Clients {
				select {
				case conn.Read <- message:
				default:
					close(conn.Read)
					close(conn.Write)
					delete(manager.Clients, conn)
				}
			}
		}
	}
}

// 处理s10发给s12的指令
func (manager *ClientManager) HandleCommand() {
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
				manager.Connect(cmd.From, cmd.To)
			case "disconnect":
				// 把connect指令建立的map断开
				//fromKey := cmd.From
				//toKey := cmd.To
				manager.Disconnect(cmd.From, cmd.To)
			case "reset":
				// 重置所有连接，保留data字段内的连接，其余的都关闭
				manager.Reset(cmd.Data)
			case "close":
				// 关闭data字段内的连接
				manager.Close(cmd.Data)
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

// 为两个设备建立连接
// 在连接列表中找出两个key, 从fromKey中读取数据，然后写到toKey中
// 这个连接可能需要封装一下
func (manager *ClientManager) Connect(fromKey string, toKey string) {

	fmt.Println("开始传输数据")

	var readClient, writeClient Client

	for client, _ := range manager.Clients {
		if client.Key == fromKey {
			fmt.Println("Find from device")
			readClient = *client
		}
		if client.Key == toKey {
			fmt.Println("find to device")
			writeClient = *client
		}
	}

	// 实例化连接
	connection := Connection{
		ID: readClient.Key + writeClient.Key,
		ReadClient: readClient,
		WriteClient: writeClient,
		DisconnectChan: make(chan struct{}, 1),
	}

	fmt.Println("connection id: ", connection.ID)

	ConnectionMap[connection.ID] = connection

	go connection.TransData()
}

// 断开连接
func (manager *ClientManager) Disconnect(fromKey, toKey string) {
	// 找到这个连接实例
	// 往这个连接实例中的disConnectChan中发送信号
	id := fromKey + toKey
	for key, conn := range ConnectionMap {
		if key == id {
			conn.DisconnectChan <- struct{}{}
		}
	}
}

// 重置连接，批量管理Connection
// 解释data里的数据，存到一个数组中
// 然后遍历这个数组
// 如果ConnectionMap中的key在这个数组，则断开这个连接
func (manager *ClientManager) Reset(data string) {
	// version 1
	connArray := strings.Split(data, ",")
	for _, conn := range connArray {
		keys := strings.Split(conn, "=")
		fromKey := keys[0]
		toKey := keys[1]
		id := fromKey + toKey
		for key, value := range ConnectionMap {
			if key == id {
				value.DisconnectChan <- struct{}{}
			}
		}
	}
}

func (manager *ClientManager) Reset2(data string) {
	// version 2
	// 在这个数组中的连接将保留
	connArray := strings.Split(data, ",")
	var KeepConn []string
	for _, conn := range connArray {
		keys := strings.Split(conn, "=")
		fromKey := keys[0]
		toKey := keys[1]
		id := fromKey + toKey
		KeepConn = append(KeepConn, id)
	}
	for key, value := range ConnectionMap {
		exist := false
		for _, conn := range KeepConn {
			if key == conn {
				exist = true
			}
		}
		if !exist {
			value.DisconnectChan <- struct{}{}
		}
	}
}

// 关闭指定设备的连接
func (manager *ClientManager) Close(data string) {
	for client, _ := range manager.Clients {
		if client.Key == data {
			manager.Unregister <- client
		}
	}
}

func (manager *ClientManager) send(message []byte, ignore *Client) {
	for conn := range manager.Clients {
		if conn != ignore {
			conn.Write <- message
		}
	}
}

func (manager *ClientManager) Report() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			fmt.Println("my key: ",commands.S12Key)
			fmt.Println("当前连接数：", len(ConnectionMap))
			fmt.Printf("当前有%d个设备在连接\n", len(manager.Clients))
			fmt.Println("允许连接的key:")
			for _, key := range commands.PermissionKey {
				fmt.Println("key: ", key)
			}
		}
	}
}

type Message struct {
	Sender    string `json:"sender,omitempty"`
	Recipient string `json:"recipient,omitempty"`
	Content   string `json:"content,omitempty"`
}
