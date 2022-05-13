# Smoke Test

```bash
kubectl create secret generic kubernetes-the-hard-way \
  --from-literal="mykey=mydata"
```

```bash
sshpass -p "$password" ssh root@${master0} "ETCDCTL_API=3 etcdctl get \
                                             --endpoints=https://127.0.0.1:2379 \
                                             --cacert=/etc/etcd/ca.pem \
                                             --cert=/etc/etcd/kubernetes.pem \
                                             --key=/etc/etcd/kubernetes-key.pem\
                                             /registry/secrets/default/kubernetes-the-hard-way | hexdump -C"

```


输出

```
00000000  2f 72 65 67 69 73 74 72  79 2f 73 65 63 72 65 74  |/registry/secret|
00000010  73 2f 64 65 66 61 75 6c  74 2f 6b 75 62 65 72 6e  |s/default/kubern|
00000020  65 74 65 73 2d 74 68 65  2d 68 61 72 64 2d 77 61  |etes-the-hard-wa|
00000030  79 0a 6b 38 73 3a 65 6e  63 3a 61 65 73 63 62 63  |y.k8s:enc:aescbc|
00000040  3a 76 31 3a 6b 65 79 31  3a d2 38 50 95 67 9b e6  |:v1:key1:.8P.g..|
00000050  b6 67 fc 76 da 02 3e 12  22 96 aa fb a8 73 07 e8  |.g.v..>."....s..|
00000060  80 ae 96 05 ef 64 4b 09  77 a0 be c1 44 f1 ac 79  |.....dK.w...D..y|
00000070  89 cd 90 d2 be b5 34 9b  85 18 43 b3 83 cf 46 a2  |......4...C...F.|
00000080  59 d3 f0 4b a8 c6 76 de  6c 97 71 ee d4 76 40 1e  |Y..K..v.l.q..v@.|
00000090  7e eb 66 80 be d8 d1 ec  3d b1 a1 64 ff a2 9d e7  |~.f.....=..d....|
000000a0  36 79 94 a2 68 72 41 e9  ad 4d 19 6a 3c 43 6d da  |6y..hrA..M.j<Cm.|
000000b0  2c 4c 01 e7 c0 56 1f 2e  c4 de 1f d6 30 58 05 f8  |,L...V......0X..|
000000c0  3a b0 ed 4c 19 af 7b f6  73 3e 2a 6c 03 48 a1 fa  |:..L..{.s>*l.H..|
000000d0  dc 05 0b ed c8 44 3b 9f  bb 25 f2 57 93 e2 bf be  |.....D;..%.W....|
000000e0  d0 39 66 df 8e 0a 8d 34  68 f2 fd 9e a3 35 23 9d  |.9f....4h....5#.|
000000f0  75 26 40 28 3c 8a f8 e3  f2 3c 4a d4 a6 e7 5d 11  |u&@(<....<J...].|
00000100  db 42 b3 bb b1 c1 3c 6d  2d d8 d7 80 97 d7 45 64  |.B....<m-.....Ed|
00000110  d0 28 db 0a 65 b2 b0 06  4d da 83 97 8f c0 1e 85  |.(..e...M.......|
00000120  f6 6d e4 22 20 91 c4 ce  84 4a 5c d3 c5 59 e4 7f  |.m." ....J\..Y..|
00000130  9d d0 c7 a0 e4 9f 40 0f  2b 07 63 7d 4e ec e6 31  |......@.+.c}N..1|
00000140  3c 1a db 65 62 85 35 68  33 86 f5 ca 79 7f f9 c8  |<..eb.5h3...y...|
00000150  3f c4 bf 58 57 e5 d5 b2  2a 0a                    |?..XW...*.|
0000015a
```

### Deployment

```bash
kubectl create deployment nginx --image=nginx
kubectl get pods -l app=nginx
```

等一会

```
NAME                    READY   STATUS    RESTARTS   AGE
nginx-f89759699-kpn5m   1/1     Running   0          10s
```

### Port Forwarding

```
POD_NAME=$(kubectl get pods -l app=nginx -o jsonpath="{.items[0].metadata.name}")
kubectl port-forward $POD_NAME 8080:80
```


```
curl --head http://127.0.0.1:8080

HTTP/1.1 200 OK
Server: nginx/1.19.10
Date: Sun, 02 May 2021 05:29:25 GMT
Content-Type: text/html
Content-Length: 612
Last-Modified: Tue, 13 Apr 2021 15:13:59 GMT
Connection: keep-alive
ETag: "6075b537-264"
Accept-Ranges: bytes
```

### Exec

```
kubectl exec -ti $POD_NAME -- nginx -v
```

```
nginx version: nginx/1.21.6
```

### Services

```bash
kubectl expose deployment nginx --port 80 --type NodePort
```

```
NODE_PORT=$(kubectl get svc nginx \
  --output=jsonpath='{range .spec.ports[0]}{.nodePort}')
```


```bash
curl -I http://$worker0:$NODE_PORT
curl -I http://$worker1:$NODE_PORT
```