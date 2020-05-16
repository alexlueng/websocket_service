package server

import (
	"fmt"
	"time"
)

// 保存连接的map
// key就是fromKey
// map中如果要修改Struct中的值，需要用指针的方式
var ConnectionMap = make(map[string]*Connection)

// 路由表，键是需要读取数据的设备，值是需要接收数据的设备
// 因为s10发送指令的时候，accept, conn指令是同时发送的
// 所以有可能在s10还没收到accept指令的时候，就收到了conn指令
// 当s12收到accept指令的时候，就往这个路由中加入一条记录
// 如果收发数据的设备都在线的话，就开始转发数据
var RouteTable = make(map[string][]string)

var BroadcastTable = make(map[string][]string)

// 连接实例
type Connection struct {
	// 对于每一个连接，是否需要一个id
	ReadClient       Client   // from
	WriteClients     []Client // to
	IsBroadcasting   bool
	BroadcastClients []Client // broadcast

	DisconnectChan chan struct{} //
}

// 两个设备发送数据
// 从fromkey中读出数据，发送到tokey中
func (c *Connection) TransData() {
	fmt.Println("start trans data...")
	for {
		select {
		case <-c.DisconnectChan:
			//断开两个连接
			fmt.Println("断开这个连接")
			// 从map中删除这个元素
			//delete(ConnectionMap, c.ID)
			return
		case data := <-c.ReadClient.Read:
			fmt.Println("data read: ", data)
			if c.IsBroadcasting {
				// 如果正在广播，就只发送给接收广播的设备就好了
				for _, bClient := range c.BroadcastClients {
					bClient.Write <- data
				}
				break
			}

			for _, wClient := range c.WriteClients {
				if wClient.BroadcastRecv {
					// 如果这个设备正在接收广播，就不发给它了
					break
				}
				fmt.Println("writing data to ", wClient.Key)
				wClient.Write <- data
			}

		default:

		}
	}

}

// 新加入一个接入数据的设备，实现一对多传输
func (c *Connection) AddWriteClient(client Client) {
	c.WriteClients = append(c.WriteClients, client)
}

// s10主动要求断开某个连接
func (c *Connection) DeleteWriteClient(toKey string) {
	for index, value := range c.WriteClients {
		if value.Key == toKey {
			c.WriteClients = append(c.WriteClients[:index], c.WriteClients[index+1:]...)
			// TODO：修改路由表
			fmt.Println(ConnectionMap)
			break
		}
	}
}

// 打印当前连接实例的信息
func (c *Connection) ReportConnectStatus() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if c.ReadClient.Key != "" {
				fmt.Println("readClient已经在线：", c.ReadClient.Key)
			}
			fmt.Printf("我要向%d个Client发送数据。\n", len(c.WriteClients))
			for _, client := range c.WriteClients {
				fmt.Println(client.Key + " is recieving message.")
			}
		case <-c.DisconnectChan:
			return
		}
	}

}
