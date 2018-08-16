package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"time"
)

//server端，请求服务，运行在有外网ip的服务器上

var localPort *string = flag.String("localPort", "3002", "user访问地址端口")
var remotePort *string = flag.String("remotePort", "20012", "与client通讯端口")

//与client相关的conn
type client struct {
	conn net.Conn //远程提供服务的client连接
	er   chan bool
	//未收到心跳包通道
	heart chan bool
	//暂未使用！！！原功能tcp连接已经接通，不在需要心跳包
	disheart bool
	writ     chan bool
	recv     chan []byte
	send     chan []byte
}

//读取client过来的数据
func (cli *client) read() {
	for {
		//40秒没有数据传输则断开
		cli.conn.SetReadDeadline(time.Now().Add(time.Second * 40))
		var recv []byte = make([]byte, 10240)
		n, err := cli.conn.Read(recv)

		if err != nil {
			//			if strings.Contains(err.Error(), "timeout") && self.disheart {
			//				fmt.Println("两个tcp已经连接,server不在主动断开")
			//				self.conn.SetReadDeadline(time.Time{})
			//				continue
			//			}
			cli.heart <- true
			cli.er <- true
			cli.writ <- true
			//fmt.Println("长时间未传输信息，或者client已关闭。断开并继续accept新的tcp，", err)
		}
		//收到心跳包hh，原样返回回复
		if recv[0] == 'h' && recv[1] == 'h' {
			cli.conn.Write([]byte("hh"))
			continue
		}
		cli.recv <- recv[:n]

	}
}

//处理心跳包
//func (self client) cHeart() {

//	for {
//		var recv []byte = make([]byte, 2)
//		var chanrecv []byte = make(chan []byte)
//		self.conn.SetReadDeadline(time.Now().Add(time.Second * 30))
//		n, err := self.conn.Read(recv)
//		chanrecv <- recv
//		if err != nil {
//			self.heart <- true
//			fmt.Println("心跳包超时", err)
//			break
//		}
//		if recv[0] == 'h' && recv[1] == 'h' {
//			self.conn.Write([]byte("hh"))
//		}

//	}
//}

//把数据发送给client
func (cli client) write() {

	for {
		var send []byte = make([]byte, 10240)
		select {
		case send = <-cli.send:
			cli.conn.Write(send)
		case <-cli.writ:
			//fmt.Println("写入client进程关闭")
			break
		}
	}
}

//与user相关的conn，请求最最开始发起方 localhost:3002
type user struct {
	conn net.Conn
	er   chan bool
	writ chan bool
	recv chan []byte
	send chan []byte
}

//读取user过来的数据
func (u user) read() {
	u.conn.SetReadDeadline(time.Now().Add(time.Millisecond * 800))
	for {
		var recv []byte = make([]byte, 10240)
		n, err := u.conn.Read(recv)
		u.conn.SetReadDeadline(time.Time{})
		if err != nil {

			u.er <- true
			u.writ <- true
			//fmt.Println("读取user失败", err)

			break
		}
		u.recv <- recv[:n]
	}
}

//把数据发送给user
func (u user) write() {

	for {
		var send []byte = make([]byte, 10240)
		select {
		case send = <-u.send:
			u.conn.Write(send)
		case <-u.writ:
			//fmt.Println("写入user进程关闭")
			break

		}
	}

}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU()) //设置任务可以使用的最大cpu核数

	flag.Parse() //开始解析flag
	if flag.NFlag() != 2 {
		flag.PrintDefaults()
		os.Exit(1)
	}
	local, _ := strconv.Atoi(*localPort)
	remote, _ := strconv.Atoi(*remotePort)
	if !(local >= 0 && local < 65536) {
		fmt.Println("端口设置错误")
		os.Exit(1)
	}
	if !(remote >= 0 && remote < 65536) {
		fmt.Println("端口设置错误")
		os.Exit(1)
	}

	//监听端口
	r, err := net.Listen("tcp", ":" + *remotePort) //连接远程服务
	log(err)
	l, err := net.Listen("tcp", ":" + *localPort) //连接本地服务
	log(err)
	//第一条tcp关闭或者与浏览器建立tcp都要返回重新监听
TOP:
//监听user链接
	Uconn := make(chan net.Conn)
	go goaccept(l, Uconn) //连接本地服务，一定要先接受client
	fmt.Println("准备好连接了")
	rconn := accept(r) //远程服务
	fmt.Println("client已连接", rconn.LocalAddr().String())
	recv := make(chan []byte)
	send := make(chan []byte)
	heart := make(chan bool, 1)
	//1个位置是为了防止两个读取线程一个退出后另一个永远卡住
	er := make(chan bool, 1)
	writ := make(chan bool)
	client := &client{rconn, er, heart, false, writ, recv, send}
	go client.read()
	go client.write()

	//这里可能需要处理心跳
	for {
		select {
		case <-client.heart:
			goto TOP
		case userconnn := <-Uconn: //响应浏览器的请求
			//暂未使用
			client.disheart = true
			recv = make(chan []byte)
			send = make(chan []byte)
			//1个位置是为了防止两个读取线程一个退出后另一个永远卡住
			er = make(chan bool, 1)
			writ = make(chan bool)
			user := &user{userconnn, er, writ, recv, send}
			go user.read()  //从用户浏览器一直读取
			go user.write() //给用户浏览器返回数据，后半部分，client 返回-> server 返回-> user browser
			//当两个socket都创立后进入handle处理
			go handle(client, user)
			goto TOP
		}

	}

}

//监听端口函数
func accept(con net.Listener) net.Conn {
	CorU, err := con.Accept()
	logExit(err)
	return CorU
}

//在另一个进程监听端口函数
func goaccept(con net.Listener, Uconn chan net.Conn) {
	CorU, err := con.Accept()
	logExit(err)
	Uconn <- CorU
}

//显示错误
func log(err error) {
	if err != nil {
		fmt.Printf("出现错误： %v\n", err)
	}
}

//显示错误并退出
func logExit(err error) {
	if err != nil {
		//fmt.Printf("出现错误，退出线程： %v\n", err)
		runtime.Goexit()
	}
}

//显示错误并关闭链接，退出线程
func logClose(err error, conn net.Conn) {
	if err != nil {
		//fmt.Println("对方已关闭", err)
		runtime.Goexit()
	}
}

//两个socket衔接相关处理
func handle(cli *client, user *user) {
	for {
		var clientrecv = make([]byte, 10240)
		var userrecv = make([]byte, 10240)
		select {

		case clientrecv = <-cli.recv:
			user.send <- clientrecv
		case userrecv = <-user.recv:
			//fmt.Println("浏览器发来的消息", string(userrecv))
			cli.send <- userrecv
			//user出现错误，关闭两端socket
		case <-user.er:
			//fmt.Println("user关闭了，关闭client与user")
			cli.conn.Close()
			user.conn.Close()
			runtime.Goexit()
			//client出现错误，关闭两端socket
		case <-cli.er:
			//fmt.Println("client关闭了，关闭client与user")
			user.conn.Close()
			cli.conn.Close()
			runtime.Goexit()
		}
	}
}
