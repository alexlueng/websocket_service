package server

import (
	"encoding/json"
	"fmt"
	"sanji_s12/commands"
	"sanji_s12/util"
	"strings"
	"sync"
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
	Lock       sync.Mutex
}

// 开始处理连接
// 设备上线下线的情况
func (manager *ClientManager) Start() {
	for {
		select {
		// 设备上线
		case conn := <-manager.Register:
			fmt.Println("a new connect has arrived: ", conn.Key)
			manager.Lock.Lock()
			manager.Clients[conn] = true
			manager.Lock.Unlock()

			fmt.Println("a new client has joined: ", conn.Key)

			jsonMessage, _ := json.Marshal(&Message{Content: "/A new socket has connected. The key is: " + conn.Key})
			manager.send(jsonMessage, conn)

			// TODO：生成一条report指令，放到writeCmdChan中
			manager.Report(conn, "online")

		// 设备下线
		case conn := <-manager.Unregister:
			if _, ok := manager.Clients[conn]; ok {
				close(conn.Read)
				close(conn.Write)
				delete(manager.Clients, conn)
				jsonMessage, _ := json.Marshal(&Message{Content: "/A socket has disconnected."})
				manager.send(jsonMessage, conn)

				// 处理路由表 key=fromKey key=toKey
				manager.HandleOffline(conn.Key)

				// TODO：生成一条report指令，放到writeCmdChan中
				manager.Report(conn, "offline")
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

func (manager *ClientManager) CheckClientExist(key string) (*Client, bool) {
	manager.Lock.Lock()
	for client, ok := range manager.Clients {
		if ok {
			if client.Key == key {
				return client, ok
			}
		}
	}
	manager.Lock.Unlock()
	return nil, false
}

// 设备离线的情况，作为收发设备具有不同的处理方式
func (manager *ClientManager) HandleOffline(key string) {

	// 一个设备可以同是接收数据和发送数据
	// 如果它在发送数据
	// 就把这个Connection在ConnectionMap中删除
	for connKey := range ConnectionMap {
		if connKey == key {
			ConnectionMap[connKey].DisconnectChan <- struct{}{}
			close(ConnectionMap[connKey].DisconnectChan)
			delete(ConnectionMap, connKey)
		}
	}

	// 遍历一下routetable, 查找一下它在哪个连接中接收数据
	for fromKey, conns := range RouteTable {
		for _, conn := range conns {
			if conn == key {
				ConnectionMap[fromKey].DeleteWriteClient(key)
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
			case "conn":
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
			case "disconn":
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
			case "report":
				fmt.Println("report!!!!!!!!!!")
				manager.ReportAll()
			case "broadcast":
				manager.HandleBroadcast(cmd.From, cmd.To)
			default:
				fmt.Println("Don't know what command it is.")
			}
		}
	}
}

// 等待toKey设备上线
func (manager *ClientManager) WaitForToKey(conn *Connection, key string) {

	found := false

	for {
		manager.Lock.Lock()
		for client, _ := range manager.Clients {
			if client.Key == key {
				fmt.Println("找到设备：", key)
				conn.AddWriteClient(*client)
				found = true
			}
		}
		manager.Lock.Unlock()
		if found {
			break
		}
	}
}

// 等待fromKey设备的上线
func (manager *ClientManager) WaitForFromKey(conn *Connection, key string) {

	found := false

	for {
		manager.Lock.Lock()
		for client, _ := range manager.Clients {
			if client.Key == key {
				fmt.Println("找到设备：", key)
				conn.ReadClient = *client
				go conn.TransData()
				found = true
			}
		}
		manager.Lock.Unlock()
		if found {
			break
		}
	}
}

// 为两个设备建立连接
// 1. 基本情况：fromkey, tokey都已经连接到s12了。在连接列表中找出两个key, 从fromKey中读取数据，然后写到toKey中
// 2. 如果fromkey存在，tokey不存在。
// 3. 如果fromkey不存在
// RouterTable表fromkey里的值应该和该connection里的writeClients同步
func (manager *ClientManager) Connect(fromKey string, toKey string) {

	// 如果fromkey存在且已经有在发送数据
	// 找到tokey并发送数据
	if conn, ok := ConnectionMap[fromKey]; ok {
		// 在已经连接的设备中找到toKey
		exist := false
		for client, _ := range manager.Clients {
			if client.Key == toKey {
				exist = true
				conn.AddWriteClient(*client)
				RouteTable[fromKey] = append(RouteTable[fromKey], toKey)
				return
			}
		}

		// 如果toKey还没连接上来
		// 在路由表中先记录这个路由
		// 等待这个设备上线，如何做这个等待(遍历clientmanager.Client)
		if !exist {
			RouteTable[fromKey] = append(RouteTable[fromKey], toKey)
			go manager.WaitForToKey(conn, toKey)
			return
		}
		// 上线后就向toKey发送数据

	}

	// 以下代码是处理connectionMap中没有fromKey的情况
	// 如果connect指令发过来的时候，找到了fromKey, 但没有找到toKey
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

	if readClient.Key != "" { //说明找到了fromKey
		if writeClient.Key != "" { // 也找到了tokey
			RouteTable[fromKey] = append(RouteTable[fromKey], toKey)
			connection := Connection{
				ReadClient:     readClient,
				WriteClients:   []Client{writeClient},
				DisconnectChan: make(chan struct{}, 1),
			}

			ConnectionMap[fromKey] = &connection

			go connection.TransData()
			go connection.ReportConnectStatus()
		} else { // 找到fromKey, 但没有找到toKey
			RouteTable[fromKey] = append(RouteTable[fromKey], toKey)
			// 如果没有找到这个fromkey
			// 实例化连接
			connection := Connection{
				ReadClient:     readClient,
				WriteClients:   []Client{},
				DisconnectChan: make(chan struct{}, 1),
			}

			ConnectionMap[fromKey] = &connection

			go connection.TransData()
			go connection.ReportConnectStatus()
			go manager.WaitForToKey(&connection, toKey)
		}
		return
	}

	// 没有找到fromKey的情况
	if readClient.Key == "" { // fromkey设备还没连接上来
		if writeClient.Key != "" { // 但是tokey已经在线
			RouteTable[fromKey] = append(RouteTable[fromKey], toKey)
			connection := Connection{
				ReadClient:     Client{},
				WriteClients:   []Client{writeClient},
				DisconnectChan: make(chan struct{}, 1),
			}

			ConnectionMap[fromKey] = &connection

			//go connection.TransData()
			// 先不用发数据，等fromkey上线之后再发数据
			go manager.WaitForFromKey(&connection, fromKey)
			go connection.ReportConnectStatus()
		} else { // 既没有找到fromkey, 也没有找到tokey
			RouteTable[fromKey] = append(RouteTable[fromKey], toKey)
			connection := Connection{
				ReadClient:     Client{},
				WriteClients:   []Client{},
				DisconnectChan: make(chan struct{}, 1),
			}

			ConnectionMap[fromKey] = &connection

			//go connection.TransData()
			// 先不用发数据，等fromkey上线之后再发数据
			go manager.WaitForFromKey(&connection, fromKey)
			go manager.WaitForToKey(&connection, toKey)
			go connection.ReportConnectStatus()
		}
	}

}

// 断开连接，这个断开是把连接的管道断开，设备是没有下线的
func (manager *ClientManager) Disconnect(fromKey, toKey string) {
	// 找到这个连接实例
	// 往这个连接实例中的disConnectChan中发送信号
	if conn, ok := ConnectionMap[fromKey]; ok {
		conn.DeleteWriteClient(toKey)
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

// 设备上下线发送的report指令
// 生成一条report指令放到写通道中
func (manager *ClientManager) Report(conn *Client, state string) {
	report := commands.Cmd{
		CmdId:   util.GetCmdId(),
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

	connStatus := commands.ConnectStatus{}
	connStatus.State = state
	connStatus.Routers = RouteTable
	connStatus.WhiteList = commands.PermissionKey

	clients := make(map[string]interface{})
	//for client, _ := range manager.Clients {
	//	clients[client.Key] = *client
	//	//util.SmartPrint(*client)
	//}
	clients[conn.Key] = conn

	connStatus.Clients = clients
	// TODO: connStatus.Active

	connStatusJSON, err := json.Marshal(connStatus)
	if err != nil {
		fmt.Println("Can't encode data: ", err)
		return
	}

	report.Data = string(connStatusJSON)

	fmt.Println("report指令内容：", report)

	commands.OutCmdChan <- report
	return
}

// 设备上下线发送的report指令
// 生成一条report指令放到写通道中
func (manager *ClientManager) ReportAll() {
	report := commands.Cmd{
		CmdId:   util.GetCmdId(),
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

	connStatus := commands.ConnectStatus{}
	connStatus.Routers = RouteTable
	connStatus.WhiteList = commands.PermissionKey

	clients := make(map[string]interface{})
	for client, _ := range manager.Clients {
		clients[client.Key] = *client
		//util.SmartPrint(*client)
	}

	connStatus.Clients = clients
	// TODO: connStatus.Active

	connStatusJSON, err := json.Marshal(connStatus)
	if err != nil {
		fmt.Println("Can't encode data: ", err)
		return
	}

	report.Data = string(connStatusJSON)

	fmt.Println("report指令内容：", report)

	commands.OutCmdChan <- report
	return
}

func (manager *ClientManager) send(message []byte, ignore *Client) {
	for conn := range manager.Clients {
		if conn != ignore {
			conn.Write <- message
		}
	}
}

// S12收到broadcast指令后，首先更新BroadcastTable
// 广播结束的标志是什么
func (manager *ClientManager) HandleBroadcast(fromKey, toKey string) {

	toKeyArray := strings.Split(toKey, ",")

	if conn, ok := ConnectionMap[fromKey]; ok {
		// 说明已经在发送数据了，现转为广播
		// 广播结束的时候要把标志改为false
		conn.IsBroadcasting = true

		for _, key := range toKeyArray {
			if client, ok := manager.CheckClientExist(key); ok {
				client.BroadcastRecv = true
				conn.BroadcastClients = append(conn.BroadcastClients, *client)
			} else {
				// 应该扩充tokey，是加入writeClient还是broadcastClient
				go manager.WaitForToKey(conn, key)
			}
		}

	} else {
		// 新建一个Connection
		connection := Connection{
			IsBroadcasting: true,
			WriteClients: []Client{},
		}

		if conn, ok := manager.CheckClientExist(fromKey); ok {
			connection.ReadClient = *conn
		} else {
			go manager.WaitForFromKey(&connection, fromKey)
		}

		for _, toKey := range toKeyArray {
			if conn, ok := manager.CheckClientExist(toKey); ok {
				connection.BroadcastClients = append(connection.BroadcastClients, *conn)
			} else {
				go manager.WaitForFromKey(&connection, toKey)
			}
		}

		ConnectionMap[fromKey] = &connection
	}

	BroadcastTable[fromKey] = toKeyArray
}

func (manager *ClientManager) ReportCurrentState() {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			fmt.Println("my key: ", commands.S12Key)
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
