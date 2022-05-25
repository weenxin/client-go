# Leader-Election

Kubernetes中大部分组件都是高可用的，比如Scheduler，Control-Manager等，这些组件实现高可用的方式是，一个节点作为Leader，其他组件作为备份节点。工作时，只有Leader在工作，备份组件以为在等待获取Leader身份，保证系统的高可用。

client-go中有关于Leader-Election的相关代码。

实现逻辑为：
- 定义一种锁定资源，支持Lease，ConfigMap，Endpoint
- Leader节点，定时刷新租期，通过`Get` -> `Update` 最后时间的方式，持续获得租期
- Slave节点，定时获取`Get`资源，如果Leader节点租期过了，就会`Update`抢占资源（多个Slave抢占节点，由于ResourceVersion的问题，只能有一个成功）


## 如何使用

[leader-election.go](/tools/leader-election/leader-election.go)

## 代码实现


### 资源锁


#### 定义

```go
type LeaderElectionRecord struct {
	// HolderIdentity is the ID that owns the lease. If empty, no one owns this lease and
	// all callers may acquire. Versions of this library prior to Kubernetes 1.14 will not
	// attempt to acquire leases with empty identities and will wait for the full lease
	// interval to expire before attempting to reacquire. This value is set to empty when
	// a client voluntarily steps down.
	// Leader节点的标识
	HolderIdentity       string      `json:"holderIdentity"`
	//租期长度
	LeaseDurationSeconds int         `json:"leaseDurationSeconds"`
	//获取时间
	AcquireTime          metav1.Time `json:"acquireTime"`
	// 刷新时间
	RenewTime            metav1.Time `json:"renewTime"`
	// Leader转换了几次
	LeaderTransitions    int         `json:"leaderTransitions"`
}
type Interface interface {
	// Get returns the LeaderElectionRecord
	// 获取资源信息, 并且设置资源的最新ResouceVersion
	Get(ctx context.Context) (*LeaderElectionRecord, []byte, error)

	// Create attempts to create a LeaderElectionRecord
	// 创建对象
	Create(ctx context.Context, ler LeaderElectionRecord) error

	// Update will update and existing LeaderElectionRecord
	// 更新对象，基于ler做Spec的生成，但是ResourceVersion使用的是之前获取和缓存的对象
	Update(ctx context.Context, ler LeaderElectionRecord) error

	// RecordEvent is used to record events
	// 记录事件
	RecordEvent(string)

	// Identity will return the locks Identity
	// 节点的唯一身份标识
	Identity() string

	// Describe is used to convert details on current resource lock
	// into a string
	// 描述信息
	Describe() string
}
```

#### 实现

```go
type ResourceLockConfig struct {
	// Identity is the unique string identifying a lease holder across
	// all participants in an election.
	Identity string
	// EventRecorder is optional.
	EventRecorder EventRecorder

type LeaseLock struct {
	// LeaseMeta should contain a Name and a Namespace of a
	// LeaseMeta object that the LeaderElector will attempt to lead.
	// Lease的Meta信息
	LeaseMeta  metav1.ObjectMeta
	//添加更新使用的client
	Client     coordinationv1client.LeasesGetter
	//当前节点的身份标识
	LockConfig ResourceLockConfig
	//缓存的Lease对象，当服务器的ResouceVersion与当前对象的ResouceVersion不一致时，会拒绝更新操作
	lease      *coordinationv1.Lease
}
```


#### 新建操作

```go

// Manufacture will create a lock of a given type according to the input parameters
func New(lockType string, ns string, name string, coreClient corev1.CoreV1Interface, coordinationClient coordinationv1.CoordinationV1Interface, rlc ResourceLockConfig) (Interface, error) {
	endpointsLock := &EndpointsLock{
		EndpointsMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Client:     coreClient,
		LockConfig: rlc,
	}
	configmapLock := &ConfigMapLock{
		ConfigMapMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Client:     coreClient,
		LockConfig: rlc,
	}
	leaseLock := &LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Client:     coordinationClient,
		LockConfig: rlc,
	}
	switch lockType {
	case EndpointsResourceLock:
		return endpointsLock, nil
	case ConfigMapsResourceLock:
		return configmapLock, nil
	case LeasesResourceLock:
		return leaseLock, nil
	case EndpointsLeasesResourceLock:
		return &MultiLock{
			Primary:   endpointsLock,
			Secondary: leaseLock,
		}, nil
	case ConfigMapsLeasesResourceLock:
		return &MultiLock{
			Primary:   configmapLock,
			Secondary: leaseLock,
		}, nil
	default:
		return nil, fmt.Errorf("Invalid lock-type %s", lockType)
	}
}

// NewFromKubeconfig will create a lock of a given type according to the input parameters.
// Timeout set for a client used to contact to Kubernetes should be lower than
// RenewDeadline to keep a single hung request from forcing a leader loss.
// Setting it to max(time.Second, RenewDeadline/2) as a reasonable heuristic.
func NewFromKubeconfig(lockType string, ns string, name string, rlc ResourceLockConfig, kubeconfig *restclient.Config, renewDeadline time.Duration) (Interface, error) {
	// shallow copy, do not modify the kubeconfig
	config := *kubeconfig
	timeout := renewDeadline / 2
	if timeout < time.Second {
		timeout = time.Second
	}
	config.Timeout = timeout
	leaderElectionClient := clientset.NewForConfigOrDie(restclient.AddUserAgent(&config, "leader-election"))
	return New(lockType, ns, name, leaderElectionClient.CoreV1(), leaderElectionClient.CoordinationV1(), rlc)
}

```

## LeaderElector

#### 定义

```go
type LeaderElectionConfig struct {
	// Lock is the resource that will be used for locking
	// 锁的对象，可以更新，增加，自身节点的id
	Lock rl.Interface

	// LeaseDuration is the duration that non-leader candidates will
	// wait to force acquire leadership. This is measured against time of
	// last observed ack.
	//
	// A client needs to wait a full LeaseDuration without observing a change to
	// the record before it can attempt to take over. When all clients are
	// shutdown and a new set of clients are started with different names against
	// the same leader record, they must wait the full LeaseDuration before
	// attempting to acquire the lease. Thus LeaseDuration should be as short as
	// possible (within your tolerance for clock skew rate) to avoid a possible
	// long waits in the scenario.
	//
	// Core clients default this value to 15 seconds.
	// 租期时间
	LeaseDuration time.Duration
	// RenewDeadline is the duration that the acting master will retry
	// refreshing leadership before giving up.
	//
	// Core clients default this value to 10 seconds.
	// 每次刷新租期，获取租期的Deadline
	RenewDeadline time.Duration
	// RetryPeriod is the duration the LeaderElector clients should wait
	// between tries of actions.
	//
	// Core clients default this value to 2 seconds.
	// 没隔多久执行一次租期续约
	RetryPeriod time.Duration

	// Callbacks are callbacks that are triggered during certain lifecycle
	// events of the LeaderElector
	// 租期变化的回调函数，比如成为leader，获取租期失败等等
	Callbacks LeaderCallbacks

	// WatchDog is the associated health checker
	// WatchDog may be null if its not needed/configured.
	WatchDog *HealthzAdaptor

	// ReleaseOnCancel should be set true if the lock should be released
	// when the run context is cancelled. If you set this to true, you must
	// ensure all code guarded by this lease has successfully completed
	// prior to cancelling the context, or you may have two processes
	// simultaneously acting on the critical path.
	// 当Leader节点主动退出时，释放租期
	ReleaseOnCancel bool

	// Name is the name of the resource lock for debugging
	Name string
}

// LeaderCallbacks are callbacks that are triggered during certain
// lifecycle events of the LeaderElector. These are invoked asynchronously.
//
// possible future callbacks:
//  * OnChallenge()
type LeaderCallbacks struct {
	// OnStartedLeading is called when a LeaderElector client starts leading
	// 当某个节点成为Leader时调用
	OnStartedLeading func(context.Context)
	// OnStoppedLeading is called when a LeaderElector client stops leading
	// 当前节点不再是Leader时调用
	OnStoppedLeading func()
	// OnNewLeader is called when the client observes a leader that is
	// not the previously observed leader. This includes the first observed
	// leader when the client starts.
	// 当新节点产生时调用
	OnNewLeader func(identity string)
}

// LeaderElector is a leader election client.
type LeaderElector struct {
    // 配置，锁对象，获取租期的相关参数
	config LeaderElectionConfig
	// internal bookkeeping
	// 缓存监控到的选举记录
	observedRecord    rl.LeaderElectionRecord
	// marshal后的字符数组
	observedRawRecord []byte
	// 最后观察时间
	observedTime      time.Time
	// used to implement OnNewLeader(), may lag slightly from the
	// value observedRecord.HolderIdentity if the transition has
	// not yet been reported.
	// 上一次的Leader名称，如果本次获取的和这次不一样就会调用OnNewLeader 回调函数
	reportedLeader string

	// clock is wrapper around time to allow for less flaky testing
	clock clock.Clock

	metrics leaderMetricsAdapter
}

```


#### 新建

```go
// NewLeaderElector creates a LeaderElector from a LeaderElectionConfig
func NewLeaderElector(lec LeaderElectionConfig) (*LeaderElector, error) {
    // 合法检查

    // 如果租期小于Deadline，配置有问题
	if lec.LeaseDuration <= lec.RenewDeadline {
		return nil, fmt.Errorf("leaseDuration must be greater than renewDeadline")
	}
	// 如果刷新时间小于retry时间
	if lec.RenewDeadline <= time.Duration(JitterFactor*float64(lec.RetryPeriod)) {
		return nil, fmt.Errorf("renewDeadline must be greater than retryPeriod*JitterFactor")
	}
	if lec.LeaseDuration < 1 {
		return nil, fmt.Errorf("leaseDuration must be greater than zero")
	}
	if lec.RenewDeadline < 1 {
		return nil, fmt.Errorf("renewDeadline must be greater than zero")
	}
	if lec.RetryPeriod < 1 {
		return nil, fmt.Errorf("retryPeriod must be greater than zero")
	}
	if lec.Callbacks.OnStartedLeading == nil {
		return nil, fmt.Errorf("OnStartedLeading callback must not be nil")
	}
	if lec.Callbacks.OnStoppedLeading == nil {
		return nil, fmt.Errorf("OnStoppedLeading callback must not be nil")
	}

	if lec.Lock == nil {
		return nil, fmt.Errorf("Lock must not be nil.")
	}
	le := LeaderElector{
		config:  lec,
		clock:   clock.RealClock{},
		metrics: globalMetricsFactory.newLeaderMetrics(),
	}
	le.metrics.leaderOff(le.config.Name)
	return &le, nil
}

```


#### 核心时间

```go
// RunOrDie starts a client with the provided config or panics if the config
// fails to validate. RunOrDie blocks until leader election loop is
// stopped by ctx or it has stopped holding the leader lease
func RunOrDie(ctx context.Context, lec LeaderElectionConfig) {
	le, err := NewLeaderElector(lec)
	//如果配置有问题，直接返回错误
	if err != nil {
		panic(err)
	}
	// 向watchdog中注入Le
	if lec.WatchDog != nil {
		lec.WatchDog.SetLeaderElection(le)
	}
	// 开始跑
	le.Run(ctx)
}

// Run starts the leader election loop. Run will not return
// before leader election loop is stopped by ctx or it has
// stopped holding the leader lease
func (le *LeaderElector) Run(ctx context.Context) {
	defer runtime.HandleCrash()
	defer func() {
		le.config.Callbacks.OnStoppedLeading()
	}()

    // 如果没有成功获取到租期则返回，函数退出只有一种可能，成功获取租期或者context被cancel
	if !le.acquire(ctx) {
		return // ctx signalled done
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// 成功获取到了租期
	go le.config.Callbacks.OnStartedLeading(ctx)
    // 不断刷新租期
	le.renew(ctx)
}

// acquire loops calling tryAcquireOrRenew and returns true immediately when tryAcquireOrRenew succeeds.
// Returns false if ctx signals done.
func (le *LeaderElector) acquire(ctx context.Context) bool {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	succeeded := false
	desc := le.config.Lock.Describe()
	klog.Infof("attempting to acquire leader lease %v...", desc)
	// 每隔Retry时间段执行一次执行一次，知道context被取消
	wait.JitterUntil(func() {
        // 获取租期
		succeeded = le.tryAcquireOrRenew(ctx)
		// 更新最后获取的leader信息
		le.maybeReportTransition()
		// 如果没有获取租期，继续等待知道获取到租期
		if !succeeded {
			klog.V(4).Infof("failed to acquire lease %v", desc)
			return
		}
		// 成功获取租期，
		le.config.Lock.RecordEvent("became leader")
		le.metrics.leaderOn(le.config.Name)
		klog.Infof("successfully acquired lease %v", desc)
		// 停止Until函数执行
		cancel()
	}, le.config.RetryPeriod, JitterFactor, true, ctx.Done())
	// 返回结果
	return succeeded
}

// tryAcquireOrRenew tries to acquire a leader lease if it is not already acquired,
// else it tries to renew the lease if it has already been acquired. Returns true
// on success else returns false.
func (le *LeaderElector) tryAcquireOrRenew(ctx context.Context) bool {
	now := metav1.Now()
	leaderElectionRecord := rl.LeaderElectionRecord{
		HolderIdentity:       le.config.Lock.Identity(),
		LeaseDurationSeconds: int(le.config.LeaseDuration / time.Second),
		RenewTime:            now,
		AcquireTime:          now,
	}

	// 1. obtain or create the ElectionRecord
	// 从APIServer获取记录
	oldLeaderElectionRecord, oldLeaderElectionRawRecord, err := le.config.Lock.Get(ctx)
	if err != nil {
		if !errors.IsNotFound(err) {
			klog.Errorf("error retrieving resource lock %v: %v", le.config.Lock.Describe(), err)
			return false
		}
		// 之前没有则直接创建
		if err = le.config.Lock.Create(ctx, leaderElectionRecord); err != nil {
			klog.Errorf("error initially creating leader election record: %v", err)
			return false
		}
		le.observedRecord = leaderElectionRecord
		le.observedTime = le.clock.Now()
		// 成功获取租期
		return true
	}

	// 2. Record obtained, check the Identity & Time
	// 如果成功从APIServer中获取到了记录，则看下是否和之前不一样 , 增更新下
	if !bytes.Equal(le.observedRawRecord, oldLeaderElectionRawRecord) {
		le.observedRecord = *oldLeaderElectionRecord
		le.observedRawRecord = oldLeaderElectionRawRecord
		le.observedTime = le.clock.Now()
	}
	// 如果我不是Leader，并且在Leader的任期内，则什么都不做
	if len(oldLeaderElectionRecord.HolderIdentity) > 0 &&
		le.observedTime.Add(le.config.LeaseDuration).After(now.Time) &&
		!le.IsLeader() {
		klog.V(4).Infof("lock is held by %v and has not yet expired", oldLeaderElectionRecord.HolderIdentity)
		return false
	}

	// 3. We're going to try to update. The leaderElectionRecord is set to it's default
	// here. Let's correct it before updating.
	// 此时，要不就是没有leader了(所有Slave需要抢任期)，要不就是我就是leader(刷新任期)
	if le.IsLeader() {
        //刷新任期
		leaderElectionRecord.AcquireTime = oldLeaderElectionRecord.AcquireTime
		leaderElectionRecord.LeaderTransitions = oldLeaderElectionRecord.LeaderTransitions
	} else {
        // 我不是任期，增加一次Leader转换
		leaderElectionRecord.LeaderTransitions = oldLeaderElectionRecord.LeaderTransitions + 1
	}

	// update the lock itself
	// 更新锁，多个Slave同时更新只会有一个成功，因为ResouceVersion有问题
	if err = le.config.Lock.Update(ctx, leaderElectionRecord); err != nil {
		klog.Errorf("Failed to update lock: %v", err)
		return false
	}

    //刷新，或者抢任期完成
	le.observedRecord = leaderElectionRecord
	le.observedTime = le.clock.Now()
	return true
}

// renew loops calling tryAcquireOrRenew and returns immediately when tryAcquireOrRenew fails or ctx signals done.
func (le *LeaderElector) renew(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// 每隔 retry刷新下租期
	wait.Until(func() {
		timeoutCtx, timeoutCancel := context.WithTimeout(ctx, le.config.RenewDeadline)
		defer timeoutCancel()
		// 每隔Retry执行刷新下任期，直到timeout，或者成功刷新租期
		err := wait.PollImmediateUntil(le.config.RetryPeriod, func() (bool, error) {
			return le.tryAcquireOrRenew(timeoutCtx), nil
		}, timeoutCtx.Done())

		le.maybeReportTransition()
		desc := le.config.Lock.Describe()
		if err == nil {
			klog.V(5).Infof("successfully renewed lease %v", desc)
			return
		}
		// 此时timeout了，无法刷新任期
		le.config.Lock.RecordEvent("stopped leading")
		le.metrics.leaderOff(le.config.Name)
		klog.Infof("failed to renew lease %v: %v", desc, err)
		// 任期结束，结束Util
		cancel()
	}, le.config.RetryPeriod, ctx.Done())

	// if we hold the lease, give it up
	if le.config.ReleaseOnCancel {
		le.release()
	}
}



/ GetLeader returns the identity of the last observed leader or returns the empty string if
// no leader has yet been observed.
func (le *LeaderElector) GetLeader() string {
	return le.observedRecord.HolderIdentity
}

// IsLeader returns true if the last observed leader was this client else returns false.
func (le *LeaderElector) IsLeader() bool {
    // 当前节点和最后观察到的记录是否一致
	return le.observedRecord.HolderIdentity == le.config.Lock.Identity()
}



```