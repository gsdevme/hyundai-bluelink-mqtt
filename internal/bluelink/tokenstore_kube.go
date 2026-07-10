package bluelink

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// tokenSecretKey is the key within the Secret's data map holding the token JSON.
const tokenSecretKey = "tokens.json"

// namespaceFile is the in-cluster ServiceAccount namespace path.
const namespaceFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

// kubeSecretStore persists Tokens in a Kubernetes Secret the pod mutates, so the
// deployment stays stateless (etcd is the source of truth). It reads the Secret
// on Load and patches it on Save with optimistic concurrency.
type kubeSecretStore struct {
	client    kubernetes.Interface
	namespace string
	name      string
}

// NewKubeSecretStore builds a TokenStore backed by the named Secret, using the
// in-cluster config and the mounted ServiceAccount namespace.
func NewKubeSecretStore(name string) (TokenStore, error) {
	if name == "" {
		return nil, errors.New("kube token store: secret name required")
	}
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("kube in-cluster config: %w", err)
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("kube client: %w", err)
	}
	ns, err := readNamespace()
	if err != nil {
		return nil, err
	}
	return &kubeSecretStore{client: client, namespace: ns, name: name}, nil
}

func readNamespace() (string, error) {
	b, err := os.ReadFile(namespaceFile)
	if err != nil {
		return "", fmt.Errorf("read serviceaccount namespace: %w", err)
	}
	ns := strings.TrimSpace(string(b))
	if ns == "" {
		return "", errors.New("serviceaccount namespace is empty")
	}
	return ns, nil
}

func (k *kubeSecretStore) Load(ctx context.Context) (Tokens, bool, error) {
	sec, err := k.client.CoreV1().Secrets(k.namespace).Get(ctx, k.name, metav1.GetOptions{})
	if err != nil {
		return Tokens{}, false, fmt.Errorf("get token secret: %w", err)
	}
	raw, ok := sec.Data[tokenSecretKey]
	if !ok || len(raw) == 0 {
		return Tokens{}, false, nil // created empty by the deploy repo
	}
	var t Tokens
	if err := json.Unmarshal(raw, &t); err != nil {
		return Tokens{}, false, fmt.Errorf("decode stored tokens: %w", err)
	}
	return t, true, nil
}

func (k *kubeSecretStore) Save(ctx context.Context, t Tokens) error {
	raw, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("encode tokens: %w", err)
	}
	// Optimistic concurrency: read the current resourceVersion, update, and
	// retry once on conflict (single replica ⇒ conflicts are rare).
	for attempt := 0; attempt < 2; attempt++ {
		sec, err := k.client.CoreV1().Secrets(k.namespace).Get(ctx, k.name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("get token secret for update: %w", err)
		}
		if sec.Data == nil {
			sec.Data = map[string][]byte{}
		}
		sec.Data[tokenSecretKey] = raw
		_, err = k.client.CoreV1().Secrets(k.namespace).Update(ctx, sec, metav1.UpdateOptions{})
		if err == nil {
			return nil
		}
		if apierrors.IsConflict(err) && attempt == 0 {
			continue // reread and retry once
		}
		return fmt.Errorf("update token secret: %w", err)
	}
	return errors.New("update token secret: exhausted retries")
}
