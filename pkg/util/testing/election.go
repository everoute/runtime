package testing

import (
	"sync"

	"go.uber.org/atomic"
)

type FakeLeaderElectionClient struct {
	leaderName             atomic.String
	name                   string
	leadingStateUpdateCond *sync.Cond
}

func NewFakeLeaderElectionClient(name string) *FakeLeaderElectionClient {
	return &FakeLeaderElectionClient{
		leaderName:             atomic.String{},
		name:                   name,
		leadingStateUpdateCond: sync.NewCond(&sync.Mutex{}),
	}
}

func (c *FakeLeaderElectionClient) GetLeader() string { return c.leaderName.Load() }
func (c *FakeLeaderElectionClient) IsLeader() bool    { return c.leaderName.Load() == c.name }
func (c *FakeLeaderElectionClient) Identity() string  { return c.name }

func (c *FakeLeaderElectionClient) SetLeader(name string) {
	c.leaderName.Store(name)
	c.leadingStateUpdateCond.Broadcast()
}

func (c *FakeLeaderElectionClient) UntilLeadingStateUpdate(stopCh <-chan struct{}) bool {
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
