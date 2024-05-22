package options

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/spf13/pflag"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

// LeaderElectionClient show election state
type LeaderElectionClient interface {
	GetLeader() string
	IsLeader() bool
	Identity() string
	UntilLeadingStateUpdate(stopCh <-chan struct{}) bool
}

func NewElectionOptions() Options {
	return &electionOptions{}
}

type electionOptions struct {
	Enabled       bool
	NodeIdentity  string
	Name          string
	Namespace     string
	LeaseDuration time.Duration
	RenewDeadline time.Duration
	RetryPeriod   time.Duration
	LeaseTimeout  time.Duration
}

func (o *electionOptions) AddFlags(flagSet *pflag.FlagSet) {
	flagSet.BoolVar(&o.Enabled, "election-enabled", false, "whether enable leader election")
	flagSet.StringVar(&o.NodeIdentity, "election-identity", "", "leader election node identity")
	flagSet.StringVar(&o.Name, "election-name", "", "leader election lease name to use")
	flagSet.StringVar(&o.Namespace, "election-namespace", "kube-system", "leader election lease namespace to use")
	flagSet.DurationVar(&o.LeaseDuration, "election-lease-duration", 15*time.Second, "duration that non-leader candidates will wait to acquire leadership")
	flagSet.DurationVar(&o.RenewDeadline, "election-renew-deadline", 10*time.Second, "duration that the master refreshing leadership before giving up")
	flagSet.DurationVar(&o.RetryPeriod, "election-retry-period", 2*time.Second, "duration that the clients should wait between tries of actions")
	flagSet.DurationVar(&o.LeaseTimeout, "election-lease-timeout", 20*time.Second, "timeout of the lease expiry to be allowed")
}

func (o *electionOptions) Validate() []error { return nil }

func (o *electionOptions) ApplyTo(config *RecommendedConfig) error {
	if !o.Enabled {
		config.LeaderElectionClient = noopElectionClient{}
		return nil
	}
	if config.LeaderCallbacks.OnStartedLeading == nil {
		config.LeaderCallbacks.OnStartedLeading = func(context.Context) {}
	}
	if config.LeaderCallbacks.OnStoppedLeading == nil {
		config.LeaderCallbacks.OnStoppedLeading = func() {}
	}
	if o.NodeIdentity == "" {
		o.NodeIdentity = config.PublicAddress.String() + "_" + uuid.New().String()
	}

	lec := &leaderelection.LeaderElectionConfig{
		Lock: &resourcelock.LeaseLock{
			LeaseMeta:  metav1.ObjectMeta{Name: o.Name, Namespace: o.Namespace},
			Client:     config.Clientset.CoordinationV1(),
			LockConfig: resourcelock.ResourceLockConfig{Identity: o.NodeIdentity},
		},
		LeaseDuration:   o.LeaseDuration,
		RenewDeadline:   o.RenewDeadline,
		RetryPeriod:     o.RetryPeriod,
		Callbacks:       config.LeaderCallbacks,
		WatchDog:        leaderelection.NewLeaderHealthzAdaptor(o.LeaseTimeout),
		ReleaseOnCancel: true,
		Name:            o.Name,
	}
	le, leaderElectionClient := newLeaderElection(*lec)
	config.AddHealthChecks(lec.WatchDog)
	config.LeaderElectionClient = leaderElectionClient

	return config.AddPostStartHook("leader-election-hook", func(context genericapiserver.PostStartHookContext) error {
		go wait.UntilWithContext(wait.ContextForChannel(context.StopCh), le.Run, time.Second)
		return nil
	})
}

type noopElectionClient struct{}

func (noopElectionClient) GetLeader() string                               { return "" }
func (noopElectionClient) IsLeader() bool                                  { return false }
func (noopElectionClient) Identity() string                                { return "" }
func (noopElectionClient) UntilLeadingStateUpdate(sc <-chan struct{}) bool { <-sc; return false }

func newLeaderElection(lec leaderelection.LeaderElectionConfig) (*leaderelection.LeaderElector, LeaderElectionClient) {
	leadingStateUpdateCond := sync.NewCond(&sync.Mutex{})
	originOnNewLeader := lec.Callbacks.OnNewLeader
	lec.Callbacks.OnNewLeader = func(identity string) {
		if originOnNewLeader != nil {
			originOnNewLeader(identity)
		}
		leadingStateUpdateCond.Broadcast()
	}
	le := lo.Must(leaderelection.NewLeaderElector(lec))
	lec.WatchDog.SetLeaderElection(le)
	return le, &electionClient{
		Interface:              lec.Lock,
		LeaderElector:          le,
		leadingStateUpdateCond: leadingStateUpdateCond,
	}
}

type electionClient struct {
	resourcelock.Interface
	*leaderelection.LeaderElector
	leadingStateUpdateCond *sync.Cond
}

func (c *electionClient) UntilLeadingStateUpdate(stopCh <-chan struct{}) bool {
	go func() { // make sure never hang when stop
		<-stopCh
		c.leadingStateUpdateCond.Broadcast()
	}()

	c.leadingStateUpdateCond.L.Lock()
	c.leadingStateUpdateCond.Wait()
	c.leadingStateUpdateCond.L.Unlock()

	select {
	case <-stopCh:
		return false
	default:
		return true
	}
}
