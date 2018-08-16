# reverse-proxy

- ![](./res/describe.png)
- achieve a reverse proxy
- used the heartbeat mechanism


1. client must start and provide http server for 8000 port
2. go run client.go -host 127.0.0.1 -localPort 8000 -remotePort 20012
3. go run server.go -localPort 3002 -remotePort 20012
4. browser request: localhost:3002
5. [quote address](https://gitee.com/wapai/chuantou)