package options

import (
	"flag"

	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apiserver/pkg/authorization/authorizerfactory"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericoptions "k8s.io/apiserver/pkg/server/options"
	"k8s.io/apiserver/pkg/storage/storagebackend"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/klog/v2"
)

// RecommendedConfig is a structure used to configure a GenericAPIServer
type RecommendedConfig struct {
	genericapiserver.RecommendedConfig

	Clientset            kubernetes.Interface
	LeaderElectionClient LeaderElectionClient
	LeaderCallbacks      leaderelection.LeaderCallbacks
}

// NewRecommendedConfig returns a RecommendedConfig struct with the default values
func NewRecommendedConfig(codecs serializer.CodecFactory) *RecommendedConfig {
	config := genericapiserver.NewRecommendedConfig(codecs)
	config.Authorization.Authorizer = authorizerfactory.NewAlwaysAllowAuthorizer()
	return &RecommendedConfig{
		RecommendedConfig: *config,
		LeaderCallbacks:   leaderelection.LeaderCallbacks{},
	}
}

// Options contains the options for running an API server
type Options GenericOptions[*RecommendedConfig]

type GenericOptions[T any] interface {
	AddFlags(flagSet *pflag.FlagSet)
	Validate() []error
	ApplyTo(config T) error
}

func NewRecommendedOptions(prefix string, codec runtime.Codec) Options {
	return NewMultipleOptions[*RecommendedConfig](
		NewKLogOptions[*RecommendedConfig](),
		NewCoreAPIOptions(),
		NewAuditOptions(),
		NewSecureServingOptions(),
		NewFeatureOptions(),
		NewAuthenticationOptions(),
		NewEtcdOptions(storagebackend.NewDefaultConfig(prefix, codec)),
		NewElectionOptions(),
	)
}

func NewMultipleOptions[T any](opts ...GenericOptions[T]) GenericOptions[T] {
	return &multipleOptions[T]{options: opts}
}

type multipleOptions[T any] struct {
	options []GenericOptions[T]
}

func (o *multipleOptions[T]) AddFlags(flagSet *pflag.FlagSet) {
	for _, o := range o.options {
		o.AddFlags(flagSet)
	}
}

func (o *multipleOptions[T]) Validate() []error {
	var errs []error
	for _, o := range o.options {
		errs = append(errs, o.Validate()...)
	}
	return errs
}

func (o *multipleOptions[T]) ApplyTo(config T) error {
	for _, o := range o.options {
		if err := o.ApplyTo(config); err != nil {
			return err
		}
	}
	return nil
}

func NewKLogOptions[T any]() GenericOptions[T] {
	return &klogOptions[T]{}
}

type klogOptions[T any] struct{}

func (o *klogOptions[T]) Validate() []error { return nil }
func (o *klogOptions[T]) ApplyTo(T) error   { return nil }

func (o *klogOptions[T]) AddFlags(flagSet *pflag.FlagSet) {
	var allFlagSet flag.FlagSet
	klog.InitFlags(&allFlagSet)
	allFlagSet.VisitAll(func(f *flag.Flag) {
		if f.Name == "v" {
			flagSet.AddGoFlag(f)
		}
	})
}

func NewCoreAPIOptions() Options {
	return &coreAPIOptions{}
}

type coreAPIOptions struct {
	runtimeQPS   float32
	runtimeBurst int
	genericoptions.CoreAPIOptions
}

func (o *coreAPIOptions) AddFlags(fs *pflag.FlagSet) {
	if o == nil {
		return
	}
	fs.StringVar(&o.CoreAPIKubeconfigPath, "core-kubeconfig", "", "kubeconfig pointing at the core kubernetes server")
	fs.Float32Var(&o.runtimeQPS, "core-runtime-qps", 1000, "client qps connect to core apiserver")
	fs.IntVar(&o.runtimeBurst, "core-runtime-burst", 2000, "client burst connect to core apiserver")
}

func (o *coreAPIOptions) ApplyTo(config *RecommendedConfig) error {
	err := o.CoreAPIOptions.ApplyTo(&config.RecommendedConfig)
	if err != nil {
		return err
	}
	config.ClientConfig.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(o.runtimeQPS, o.runtimeBurst)
	config.Clientset, err = kubernetes.NewForConfig(config.ClientConfig)
	return err
}

func NewAuditOptions() Options {
	return &auditOptions{AuditOptions: *genericoptions.NewAuditOptions()}
}

type auditOptions struct {
	genericoptions.AuditOptions
}

func (o *auditOptions) ApplyTo(config *RecommendedConfig) error {
	return o.AuditOptions.ApplyTo(&config.Config)
}

func NewFeatureOptions() Options {
	return &featureOptions{
		FeatureOptions: *genericoptions.NewFeatureOptions(),
	}
}

type featureOptions struct {
	genericoptions.FeatureOptions
}

func (o *featureOptions) ApplyTo(config *RecommendedConfig) error {
	return o.FeatureOptions.ApplyTo(&config.Config)
}

func NewEtcdOptions(config *storagebackend.Config) Options {
	return &etcdOptions{
		EtcdOptions: *genericoptions.NewEtcdOptions(config),
	}
}

type etcdOptions struct {
	genericoptions.EtcdOptions
}

func (o *etcdOptions) ApplyTo(config *RecommendedConfig) error {
	t := config.Config.StorageObjectCountTracker
	stopCh := config.Config.DrainedNotify()
	hook := config.Config.AddPostStartHook
	if err := o.Complete(t, stopCh, hook); err != nil {
		return err
	}
	return o.EtcdOptions.ApplyTo(&config.Config)
}
