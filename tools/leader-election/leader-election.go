package main

import (
	"context"
	"flag"
	"fmt"
	"k8s.io/client-go/tools/clientcmd"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/klog"
)

var (
	client             *clientset.Clientset
	leaseLockName      string
	leaseLockNamespace string
	instanceName       string
	mode               runMode
	changeModeCh       = make(chan runMode)
)

type runMode string

const modeMaster = "master"
const modeSlave = "slave"

// 创建lock对象，使用LeaseLock
func getNewLock(lockname, instanceName, namespace string) *resourcelock.LeaseLock {
	return &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      lockname,
			Namespace: namespace,
		},
		Client: client.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: instanceName,
		},
	}
}

// 干活
func doStuff(changeModeCh chan runMode) {
	for {
		select {
		// 如果有模式变更立刻停止工作
		case theMode, ok := <-changeModeCh:
			if !ok {
				return
			}
			mode = theMode
		default:
			// 如果当前是leader角色，则干活
			if mode == modeMaster {
				klog.Info("working ...")
				time.Sleep(4 * time.Second)
			}
		}
	}
}

func runLeaderElection(lock *resourcelock.LeaseLock, ctx context.Context, instanceName string) {
	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:            lock,             // 制定锁的资源
		ReleaseOnCancel: true,             //当cancel时，主动释放锁
		LeaseDuration:   15 * time.Second, // 每次获取15s的任期
		RenewDeadline:   10 * time.Second, //
		RetryPeriod:     2 * time.Second,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(c context.Context) {
				klog.Info("a leader start leading .")
			},
			OnStoppedLeading: func() {
				changeModeCh <- modeSlave
				klog.Info("I am no longer the leader, staying inactive.")
			},
			OnNewLeader: func(leaderName string) {
				if leaderName == instanceName {
					klog.Info("I am leader!")
					changeModeCh <- modeMaster
					return
				}
				klog.Infof("%s is new leader ", leaderName)
			},
		},
	})
}

func main() {
	flag.StringVar(&leaseLockName, "lease-name", "", "Name of lease lock")
	flag.StringVar(&leaseLockNamespace, "lease-namespace", "default", "Name of lease lock namespace")
	flag.StringVar(&instanceName, "instance-name", "", "Name of instance")
	flag.Parse()

	if leaseLockName == "" {
		klog.Fatal("missing lease-name flag")
	}
	if leaseLockNamespace == "" {
		klog.Fatal("missing lease-namespace flag")
	}
	if instanceName == "" {
		klog.Fatal("missing instance-name flag")
	}

	config, err := clientcmd.BuildConfigFromFlags("", "/Users/yulongxin/.kube/config")
	if err != nil {
		klog.Fatalf(fmt.Sprintf("create client failed : %s", err))
	}
	client = clientset.NewForConfigOrDie(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go doStuff(changeModeCh)

	lock := getNewLock(leaseLockName, instanceName, leaseLockNamespace)
	runLeaderElection(lock, ctx, instanceName)
}
