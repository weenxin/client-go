# SSL

## 证书使用过程

### CA产生证书

`openssl req -out  -new -newkey rsa:2048 -nodes -keyout private.key -out server.crt`

![create-csr](/ch3/images/create-crt.png)

### 启动Server

`go run ./ch3/server/server.go --private-key=./private.key --public-cert=./server.crt`

### 访问server

`curl https://cyfh.com:8080/ping`

由于证书不受信任，因此输出：

```
curl: (60) SSL certificate problem: self signed certificate
More details here: https://curl.se/docs/sslcerts.html

curl failed to verify the legitimacy of the server and therefore could not
establish a secure connection to it. To learn more about this situation and
how to fix it, please visit the web page mentioned above.

```

指定cert， 运行` curl --cacert ./server.crt  https://cyfh.com:8080/ping`，可以正常访问

```
pong
```

将cert加入到信任cert中。

```bash
sudo security add-trusted-cert \
-d -r trustRoot \
-k /Library/Keychains/System.keychain ./server.crt
```

再次访问，不基于cert。` curl https://cyfh.com:8080/ping` 成功访问。

删除cert

```
sudo security remove-trusted-cert \
  -d  ./server.crt
```




