package server

import (
	"context"

	"k8s.io/klog/v2"

	"github.com/everoute/runtime/pkg/options"
)

// GracefulShutdown graceful shutdown apiserver when leader lost
// make sure api should available before the apiserver shutdown
// stopped until context done, or another node becomes leader ready
func GracefulShutdown(ctx context.Context, config *options.RecommendedConfig) {
	electionCh := make(chan string)

	go func() {
		defer close(electionCh)
		electionClient := config.LeaderElectionClient
		if electionClient == nil {
			return
		}
		// check if leader has been changed
		if ld := electionClient.GetLeader(); ld != "" && ld != electionClient.Identity() {
			electionCh <- ld
			return
		}
		// until leading state update
		for electionClient.UntilLeadingStateUpdate(ctx.Done()) {
			if ld := electionClient.GetLeader(); ld != "" && ld != electionClient.Identity() {
				electionCh <- ld
			}
		}
	}()

	select {
	case <-ctx.Done():
		klog.Fatalf("stopped when context done: %s", ctx.Err())
	case ld := <-electionCh:
		klog.Fatalf("stopped when node %s becomes leader", ld)
	}
}
