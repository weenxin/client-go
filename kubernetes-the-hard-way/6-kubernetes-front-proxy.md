# Enable HTTP Health Checks


```bash
sshpass -p "$password" ssh root@${proxy} yum install -y nginx
sshpass -p "$password" ssh root@${proxy}  yum install nginx-mod-stream -y
sshpass -p "$password" ssh root@${proxy} systemctl restart nginx

sshpass -p "$password" ssh root@${proxy} sudo mkdir -p /etc/nginx/tcpconf.d

sshpass -p "$password" ssh root@${proxy} 'echo "include /etc/nginx/tcpconf.d/*;" >>/etc/nginx/nginx.conf'

cat << EOF | tee kubernetes.conf
stream {
    upstream kubernetes {
        server $master0:6443;
        server $master1:6443;
        server $master2:6443;
    }

    server {
        listen 6443;
        listen 443;
        proxy_pass kubernetes;
    }
}
EOF
sshpass -p "$password" scp -o StrictHostKeyChecking=no kubernetes.conf root@${proxy}:/etc/nginx/tcpconf.d/
sshpass -p "$password" ssh root@${proxy} nginx -s reload
```

 测试

```bash
sshpass -p "$password" ssh root@${proxy} curl -k https://localhost:6443/version
```
