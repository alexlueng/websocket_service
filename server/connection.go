package server

import "fmt"

// 保存连接的map
var ConnectionMap = make(map[string]Connection)

// 连接实例
type Connection struct {
	ID             string // id = readclient.key + writeclient.key
	ReadClient     Client // from
	WriteClient    Client // to

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
			delete(ConnectionMap, c.ID)
			break
		case data := <-c.ReadClient.Read:
			fmt.Println("data read: ", string(data))
			fmt.Println("writing data to ", c.WriteClient.Key)
			c.WriteClient.Write <-data
		default:

		}
	}

}

