package options

import (
	"fmt"
	"net/http"

	"github.com/spf13/pflag"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/request/anonymous"
	"k8s.io/apiserver/pkg/authentication/request/x509"
)

func NewAuthenticationOptions() Options {
	return &authenticationOptions{}
}

type authenticationOptions struct {
	Enabled bool
}

func (o *authenticationOptions) AddFlags(flagSet *pflag.FlagSet) {
	flagSet.BoolVar(&o.Enabled, "authentication-enabled", false, "enable client authentication with x509")
}

func (o *authenticationOptions) Validate() []error { return nil }

func (o *authenticationOptions) ApplyTo(config *RecommendedConfig) error {
	if !o.Enabled {
		return nil
	}
	if config.SecureServing == nil || config.SecureServing.ClientCA == nil {
		return fmt.Errorf("enable client authentication need CA")
	}
	x509Auth := x509.NewDynamic(config.SecureServing.ClientCA.VerifyOptions, x509.CommonNameUserConversion)
	anonymousAuth := anonymous.NewAuthenticator()
	config.Authentication.Authenticator = authenticator.RequestFunc(func(req *http.Request) (*authenticator.Response, bool, error) {
		switch req.URL.Path {
		case "/healthz", "/livez", "/readyz", "/version":
			return anonymousAuth.AuthenticateRequest(req)
		default:
			return x509Auth.AuthenticateRequest(req)
		}
	})
	return nil
}
