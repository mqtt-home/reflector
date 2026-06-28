package main

import (
	"context"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/mqtt-home/reflector/config"
	"github.com/mqtt-home/reflector/internal/reflector"
	"github.com/mqtt-home/reflector/version"
	"github.com/philipparndt/go-logger"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

// helmReleaseSecretSelector excludes helm release secrets from the secret watch.
// These are large, churn constantly, and are never reflected.
const helmReleaseSecretSelector = "type!=helm.sh/release.v1"

func main() {
	logger.Init("info", logger.Logger())
	logger.Info("reflector", "version", version.Info())

	cfg := config.Load()
	logger.SetLevel(cfg.LogLevel)
	logger.Info("Configuration",
		"logLevel", cfg.LogLevel,
		"watcherTimeout", cfg.WatcherTimeout,
		"excludedNamespaces", cfg.ExcludedNamespaces,
		"skipTlsVerify", cfg.SkipTLSVerify)

	client, err := reflector.NewClient(cfg.SkipTLSVerify)
	if err != nil {
		logger.Error("Failed to build Kubernetes client", "error", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	resync := time.Duration(cfg.WatcherTimeout) * time.Second
	excluded := reflector.ParseGlobPatterns(cfg.ExcludedNamespaces)

	// Namespaces and ConfigMaps share one factory. Secrets use a separate factory
	// with a field selector so helm release secrets never enter the cache.
	baseFactory := informers.NewSharedInformerFactory(client, resync)
	secretFactory := informers.NewSharedInformerFactoryWithOptions(client, resync,
		informers.WithTweakListOptions(func(o *metav1.ListOptions) {
			o.FieldSelector = helmReleaseSecretSelector
		}))

	nsInformer := baseFactory.Core().V1().Namespaces()
	secretBackend := reflector.NewSecretBackend(client, secretFactory)
	configMapBackend := reflector.NewConfigMapBackend(client, baseFactory)

	secretCtrl := reflector.NewController(secretBackend, nsInformer, excluded)
	configMapCtrl := reflector.NewController(configMapBackend, nsInformer, excluded)

	// Health server comes up immediately; readiness reports false until caches sync.
	var ready atomic.Bool
	health := reflector.NewHealthServer(ready.Load)
	go func() {
		if err := health.Start(cfg.HealthAddr); err != nil {
			logger.Error("Health server stopped", "error", err)
		}
	}()
	logger.Info("Health endpoints available", "addr", cfg.HealthAddr,
		"live", "/health/live", "ready", "/health/ready")

	baseFactory.Start(ctx.Done())
	secretFactory.Start(ctx.Done())

	logger.Info("Waiting for caches to sync")
	if !cache.WaitForCacheSync(ctx.Done(),
		nsInformer.Informer().HasSynced,
		secretBackend.Informer().HasSynced,
		configMapBackend.Informer().HasSynced) {
		logger.Error("Failed to sync caches")
		os.Exit(1)
	}
	ready.Store(true)
	logger.Info("Caches synced, reflector is running")

	go secretCtrl.Run(ctx, 2)
	go configMapCtrl.Run(ctx, 2)

	<-ctx.Done()
	logger.Info("Received shutdown signal, exiting")
}
