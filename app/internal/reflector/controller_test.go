package reflector

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

func makeNamespace(name string, labels map[string]string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels}}
}

func makeSecret(ns, name string, ann map[string]string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, Annotations: ann},
		Type:       corev1.SecretTypeOpaque,
		Data:       data,
	}
}

// startController spins up the secret controller against a fake cluster and returns
// the client plus a cancel function.
func startController(t *testing.T, objects ...runtime.Object) (*fake.Clientset, context.Context, context.CancelFunc) {
	t.Helper()
	client := fake.NewSimpleClientset(objects...)
	factory := informers.NewSharedInformerFactory(client, 0)
	nsInformer := factory.Core().V1().Namespaces()
	backend := NewSecretBackend(client, factory)
	ctrl := NewController(backend, nsInformer, nil)

	ctx, cancel := context.WithCancel(context.Background())
	factory.Start(ctx.Done())
	factory.WaitForCacheSync(ctx.Done())
	go ctrl.Run(ctx, 1)
	return client, ctx, cancel
}

func eventually(t *testing.T, msg string, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("condition not met within timeout: %s", msg)
}

func getSecret(t *testing.T, c *fake.Clientset, ctx context.Context, ns, name string) (*corev1.Secret, bool) {
	t.Helper()
	s, err := c.CoreV1().Secrets(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, false
	}
	return s, true
}

func TestAutoReflectionCreatesMirrors(t *testing.T) {
	source := makeSecret("source-ns", "shared", map[string]string{
		AnnReflectionAllowed:        "true",
		AnnReflectionAutoEnabled:    "true",
		AnnReflectionAutoNamespaces: "team-.*",
	}, map[string][]byte{"key": []byte("value")})

	client, ctx, cancel := startController(t,
		makeNamespace("source-ns", nil),
		makeNamespace("team-a", nil),
		makeNamespace("team-b", nil),
		makeNamespace("other", nil),
		source,
	)
	defer cancel()

	for _, ns := range []string{"team-a", "team-b"} {
		ns := ns
		eventually(t, "mirror created in "+ns, func() bool {
			s, ok := getSecret(t, client, ctx, ns, "shared")
			if !ok {
				return false
			}
			return string(s.Data["key"]) == "value" &&
				s.Annotations[AnnReflects] == "source-ns/shared" &&
				s.Annotations[AnnMetaAutoReflects] == "True"
		})
	}

	// The non-matching namespace must never receive a mirror.
	if _, ok := getSecret(t, client, ctx, "other", "shared"); ok {
		t.Error("mirror unexpectedly created in non-matching namespace")
	}
}

func TestDirectReflectionUpdatesMirror(t *testing.T) {
	source := makeSecret("source-ns", "cfg", map[string]string{
		AnnReflectionAllowed:           "true",
		AnnReflectionAllowedNamespaces: "app-.*",
	}, map[string][]byte{"token": []byte("s3cret")})
	source.ResourceVersion = "100" // a real source always carries a version

	// User-created empty mirror pointing at the source.
	mirror := makeSecret("app-1", "cfg", map[string]string{
		AnnReflects: "source-ns/cfg",
	}, nil)

	client, ctx, cancel := startController(t,
		makeNamespace("source-ns", nil),
		makeNamespace("app-1", nil),
		source, mirror,
	)
	defer cancel()

	eventually(t, "direct mirror populated", func() bool {
		s, ok := getSecret(t, client, ctx, "app-1", "cfg")
		if !ok {
			return false
		}
		return string(s.Data["token"]) == "s3cret" &&
			s.Annotations[AnnMetaReflectedVersion] != ""
	})
}

func TestAutoReflectionDeletedWhenSourceRemoved(t *testing.T) {
	source := makeSecret("source-ns", "shared", map[string]string{
		AnnReflectionAllowed:        "true",
		AnnReflectionAutoEnabled:    "true",
		AnnReflectionAutoNamespaces: "team-.*",
	}, map[string][]byte{"key": []byte("value")})

	client, ctx, cancel := startController(t,
		makeNamespace("source-ns", nil),
		makeNamespace("team-a", nil),
		source,
	)
	defer cancel()

	eventually(t, "mirror created", func() bool {
		_, ok := getSecret(t, client, ctx, "team-a", "shared")
		return ok
	})

	if err := client.CoreV1().Secrets("source-ns").Delete(ctx, "shared", metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete source: %v", err)
	}

	eventually(t, "auto mirror removed after source deletion", func() bool {
		_, ok := getSecret(t, client, ctx, "team-a", "shared")
		return !ok
	})
}

func TestAutoReflectionRemovedWhenNamespaceNoLongerMatches(t *testing.T) {
	source := makeSecret("source-ns", "shared", map[string]string{
		AnnReflectionAllowed:        "true",
		AnnReflectionAutoEnabled:    "true",
		AnnReflectionAutoNamespaces: "team-a",
	}, map[string][]byte{"key": []byte("value")})

	client, ctx, cancel := startController(t,
		makeNamespace("source-ns", nil),
		makeNamespace("team-a", nil),
		source,
	)
	defer cancel()

	eventually(t, "mirror created", func() bool {
		_, ok := getSecret(t, client, ctx, "team-a", "shared")
		return ok
	})

	// Narrow the auto-namespaces so team-a no longer matches.
	s, _ := client.CoreV1().Secrets("source-ns").Get(ctx, "shared", metav1.GetOptions{})
	s.Annotations[AnnReflectionAutoNamespaces] = "team-z"
	s.ResourceVersion = "" // let the fake assign a new version
	if _, err := client.CoreV1().Secrets("source-ns").Update(ctx, s, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("update source: %v", err)
	}

	eventually(t, "auto mirror removed after namespace stops matching", func() bool {
		_, ok := getSecret(t, client, ctx, "team-a", "shared")
		return !ok
	})
}
