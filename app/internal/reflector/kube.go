package reflector

import (
	"github.com/mqtt-home/reflector/version"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// NewClient builds a Kubernetes clientset. It uses the in-cluster configuration
// when running inside a pod and otherwise falls back to the local kubeconfig, which
// is convenient for development.
func NewClient(skipTLSVerify bool) (kubernetes.Interface, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			clientcmd.NewDefaultClientConfigLoadingRules(),
			&clientcmd.ConfigOverrides{},
		)
		cfg, err = loader.ClientConfig()
		if err != nil {
			return nil, err
		}
	}

	if skipTLSVerify {
		cfg.Insecure = true
		cfg.CAData = nil
		cfg.CAFile = ""
	}
	cfg.UserAgent = "reflector/" + version.Version

	return kubernetes.NewForConfig(cfg)
}
