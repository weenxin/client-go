
## Bootstrapping the etcd Cluster


```
cd ../etcd/
tar -xvf ../binary/etcd-v3.4.15-linux-amd64.tar.gz -C ./
```

```bash
for instance in master0 master1 master2; do
  echo "processing ${instance}"
  sshpass -p "$password" scp -o StrictHostKeyChecking=no  etcd-v3.4.15-linux-amd64/etcd etcd-v3.4.15-linux-amd64/etcdctl root@${(P)instance}:/usr/local/bin/

   sshpass -p "$password" ssh   root@${(P)instance} sudo mkdir -p /etc/etcd /var/lib/etcd
   sshpass -p "$password" ssh   root@${(P)instance} sudo chmod 700 /var/lib/etcd
   sshpass -p "$password" ssh  root@${(P)instance}  cp ca.pem kubernetes-key.pem kubernetes.pem /etc/etcd/
done


for instance in master0 master1 master2; do
INTERNAL_IP=${(P)instance}

cat <<EOF | tee etcd.service
[Unit]
Description=etcd
Documentation=https://github.com/coreos

[Service]
Type=notify
ExecStart=/usr/local/bin/etcd \\
  --name ${instance} \\
  --cert-file=/etc/etcd/kubernetes.pem \\
  --key-file=/etc/etcd/kubernetes-key.pem \\
  --peer-cert-file=/etc/etcd/kubernetes.pem \\
  --peer-key-file=/etc/etcd/kubernetes-key.pem \\
  --trusted-ca-file=/etc/etcd/ca.pem \\
  --peer-trusted-ca-file=/etc/etcd/ca.pem \\
  --peer-client-cert-auth \\
  --client-cert-auth \\
  --initial-advertise-peer-urls https://$INTERNAL_IP:2380 \\
  --listen-peer-urls https://$INTERNAL_IP:2380 \\
  --listen-client-urls https://$INTERNAL_IP:2379,https://127.0.0.1:2379 \\
  --advertise-client-urls https://$INTERNAL_IP:2379 \\
  --initial-cluster-token etcd-cluster-0 \\
  --initial-cluster master0=https://$master0:2380,master1=https://$master1:2380,master2=https://$master2:2380 \\
  --initial-cluster-state new \\
  --data-dir=/var/lib/etcd
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF


sshpass -p "$password" scp -o StrictHostKeyChecking=no  etcd.service root@${(P)instance}:/etc/systemd/system/etcd.service
sshpass -p "$password" ssh root@${(P)instance} systemctl daemon-reload
sshpass -p "$password" ssh root@${(P)instance} systemctl enable etcd
sshpass -p "$password" ssh root@${(P)instance} systemctl start etcd
done
```


过一段时间后，检查：

```bash
for instance in master0 master1 master2; do
sshpass -p "$password" ssh root@${(P)instance} ETCDCTL_API=3 etcdctl member list \
                                                 --endpoints=https://127.0.0.1:2379 \
                                                 --cacert=/etc/etcd/ca.pem \
                                                 --cert=/etc/etcd/kubernetes.pem \
                                                 --key=/etc/etcd/kubernetes-key.pem
done

```