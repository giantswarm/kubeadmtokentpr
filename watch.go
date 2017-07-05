package kubeadmtokentpr

import (
	"fmt"
	"time"

	microerror "github.com/giantswarm/microkit/error"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	WatchTimeout = 90 * time.Second
)

func FindToken(k8sClient kubernetes.Interface, clusterID string) (string, error) {
	listWatch := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return k8sClient.Core().Secrets(api.NamespaceDefault).List(metav1.ListOptions{
				LabelSelector: fmt.Sprintf("%s=%s", ClusterIDLabel, clusterID),
			})
		},

		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return k8sClient.Core().Secrets(api.NamespaceDefault).Watch(metav1.ListOptions{
				LabelSelector: fmt.Sprintf("%s=%s", ClusterIDLabel, clusterID),
			})
		},
	}

	kubeadmTokenChan := make(chan string)
	stopChan := make(chan struct{})

	_, clusterInformer := cache.NewInformer(
		listWatch,
		&CustomObject{},
		WatchTimeout,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				secret := obj.(*v1.Secret)
				bToken, ok := secret.Data[KubeadmTokenKey]
				if !ok {
					return
				}

				token := string(bToken[:])

				// If token was successfully found, send the token and stop the watcher.
				kubeadmTokenChan <- token
				stopChan <- struct{}{}
			},
		},
	)

	clusterInformer.Run(stopChan)

	select {
	case token := <-kubeadmTokenChan:
		return token, nil
	case <-time.After(WatchTimeout):
		// If timeout is exceeded, stop the watcher and return an error.
		stopChan <- struct{}{}
		return "", microerror.MaskAny(tokenRetrievalFailedError)
	}
}
