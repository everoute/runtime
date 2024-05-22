package options

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/spf13/pflag"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/server/dynamiccertificates"
)

func NewSecureServingOptions() Options {
	return &secureServingOptions{}
}

type secureServingOptions struct {
	AdvertiseIP   net.IP
	BindIP        net.IP
	BindPort      int
	MinTLSVersion uint16
	CACrtPath     string
	TLSCrtPath    string
	TLSKeyPath    string
}

func (o *secureServingOptions) AddFlags(flagSet *pflag.FlagSet) {
	flagSet.IPVar(&o.AdvertiseIP, "serve-advertise-ip", o.AdvertiseIP, "the IP address on which to advertise the apiserver to members")
	flagSet.IPVar(&o.BindIP, "serve-ip", net.ParseIP("0.0.0.0"), "the IP address on which to listen")
	flagSet.IntVar(&o.BindPort, "serve-port", 443, "the port on which to serve https")
	flagSet.Uint16Var(&o.MinTLSVersion, "serve-min-tls-version", tls.VersionTLS12, "the min tls version to serve")
	flagSet.StringVar(&o.CACrtPath, "serve-ca-crt-path", "", "serve use ca crt file path")
	flagSet.StringVar(&o.TLSCrtPath, "serve-tls-crt-path", "", "serve use tls crt file path")
	flagSet.StringVar(&o.TLSKeyPath, "serve-tls-key-path", "", "serve use tls key file path")
}

func (o *secureServingOptions) Validate() []error {
	if _, err := os.Stat(o.CACrtPath); err != nil {
		return []error{fmt.Errorf("invalid ca path: %s", err)}
	}
	if _, err := os.Stat(o.TLSCrtPath); err != nil {
		return []error{fmt.Errorf("invalid tls crt path: %s", err)}
	}
	if _, err := os.Stat(o.TLSKeyPath); err != nil {
		return []error{fmt.Errorf("invalid tls key path: %s", err)}
	}
	return nil
}

func (o *secureServingOptions) ApplyTo(config *RecommendedConfig) error {
	tcpListener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: o.BindIP, Port: o.BindPort})
	if err != nil {
		return err
	}

	config.SecureServing = &genericapiserver.SecureServingInfo{
		Listener:      tcpListener,
		MinTLSVersion: o.MinTLSVersion,
		//// We are composing recommended options for an aggregated api-server,
		//// whose client is typically a proxy multiplexing many operations ---
		//// notably including long-running ones --- into one HTTP/2 connection
		//// into this server.  So allow many concurrent operations.
		HTTP2MaxStreamsPerConnection: 1000,
	}
	config.PublicAddress = o.AdvertiseIP

	config.SecureServing.Cert, err = dynamiccertificates.NewDynamicServingContentFromFiles("serving-cert", o.TLSCrtPath, o.TLSKeyPath)
	if err != nil {
		return err
	}
	config.SecureServing.ClientCA, err = dynamiccertificates.NewDynamicCAContentFromFile("client-ca", o.CACrtPath)
	if err != nil {
		return err
	}
	config.LoopbackClientConfig, err = config.SecureServing.NewLoopbackClientConfig(uuid.New().String(), lo.Must(os.ReadFile(o.CACrtPath)))
	return err
}
