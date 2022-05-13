# Provisioning Pod Network Routes

```bash
kubectl apply -f https://storage.googleapis.com/kubernetes-the-hard-way/coredns-1.8.yaml
```


```bash
kubectl get pods -l k8s-app=kube-dns -n kube-system
```

```
NAME                       READY   STATUS    RESTARTS   AGE
coredns-8494f9c688-hh7r2   1/1     Running   0          10s
coredns-8494f9c688-zqrj2   1/1     Running   0          10s
```


```bash
kubectl run busybox --image=busybox:1.28 --command -- sleep 3600
```

```bash
kubectl get pods -l run=busybox
POD_NAME=$(kubectl get pods -l run=busybox -o jsonpath="{.items[0].metadata.name}")
kubectl exec -ti $POD_NAME -- nslookup kubernetes
```


```
Server:    10.32.0.10
Address 1: 10.32.0.10 kube-dns.kube-system.svc.cluster.local

Name:      kubernetes
Address 1: 10.32.0.1 kubernetes.default.svc.cluster.local
```

