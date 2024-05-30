package server_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/klog/v2"

	"github.com/everoute/runtime/pkg/options"
	"github.com/everoute/runtime/pkg/server"
	. "github.com/everoute/runtime/pkg/util/testing"
)

func TestGracefulShutdown(t *testing.T) {
	RegisterTestingT(t)

	ctx := context.Background()
	messageCh := make(chan string, 10)
	patches := gomonkey.ApplyFunc(klog.Fatalf, func(format string, args ...interface{}) {
		klog.Infof(format, args...)
		messageCh <- fmt.Sprintf(format, args...)
	})
	defer patches.Reset()

	t.Run("should shutdown when another node has been becomes leader", func(t *testing.T) {
		electionClient := NewFakeLeaderElectionClient(rand.String(20))
		electionClient.SetLeader(rand.String(20))
		go server.GracefulShutdown(ctx, &options.RecommendedConfig{LeaderElectionClient: electionClient})

		select {
		case msg := <-messageCh:
			Expect(msg).Should(ContainSubstring("becomes leader"))
		case <-time.After(time.Second):
			t.Fatalf("unexpect timeout wait graceful shutdown")
		}
	})

	t.Run("should shutdown when another node becomes leader", func(t *testing.T) {
		electionClient := NewFakeLeaderElectionClient(rand.String(20))
		go func() { time.Sleep(200 * time.Millisecond); electionClient.SetLeader(rand.String(20)) }()
		go server.GracefulShutdown(ctx, &options.RecommendedConfig{LeaderElectionClient: electionClient})

		select {
		case msg := <-messageCh:
			Expect(msg).Should(ContainSubstring("becomes leader"))
		case <-time.After(time.Second):
			t.Fatalf("unexpect timeout wait graceful shutdown")
		}
	})

	t.Run("should shutdown when context done", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		defer cancel()
		electionClient := NewFakeLeaderElectionClient(rand.String(20))
		go server.GracefulShutdown(ctx, &options.RecommendedConfig{LeaderElectionClient: electionClient})

		select {
		case msg := <-messageCh:
			Expect(msg).Should(ContainSubstring("stopped when context done"))
		case <-time.After(time.Second):
			t.Fatalf("unexpect timeout wait graceful shutdown")
		}
	})
}
