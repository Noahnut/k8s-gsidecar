package kubernetes

import (
	"context"
	"flag"
	"fmt"
	"k8s-gsidecar/notifier"
	"k8s-gsidecar/writer"
	"log"
	"os"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

type Client struct {
	Ctx    context.Context
	Client kubernetes.Interface
	Wg     *sync.WaitGroup
}

func NewClient(ctx context.Context) (*Client, error) {
	var client kubernetes.Interface
	if cfg, err := rest.InClusterConfig(); err == nil {
		client, err = kubernetes.NewForConfig(cfg)

		if err != nil {
			log.Fatalf("Failed to create Kubernetes client: %v", err)
		}

		return &Client{
			Ctx:    ctx,
			Client: client,
		}, nil
	}

	home, _ := os.UserHomeDir()
	kubeconfig := fmt.Sprintf("%s/.kube/config", home)
	if env := os.Getenv("KUBECONFIG"); env != "" {
		kubeconfig = env
	}

	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}
	configOverrides := &clientcmd.ConfigOverrides{}
	flag.Parse()

	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides).ClientConfig()
	if err != nil {
		return nil, err
	}

	client, err = kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &Client{
		Ctx:    ctx,
		Client: client,
	}, nil
}

func (c *Client) GetConfigMaps(
	namespaces []string,
	label string,
	labelValue string,
) ([]corev1.ConfigMap, error) {

	labelSelector := label

	if labelValue != "" {
		labelSelector = fmt.Sprintf("%s=%s", label, labelValue)
	}

	configMapOpt := metav1.ListOptions{
		LabelSelector: labelSelector,
	}

	var allConfigMaps []corev1.ConfigMap

	if len(namespaces) == 0 {
		configMaps, err := c.Client.CoreV1().ConfigMaps(metav1.NamespaceAll).List(c.Ctx, configMapOpt)
		if err != nil {
			return nil, err
		}
		allConfigMaps = append(allConfigMaps, configMaps.Items...)
	} else {
		for _, namespace := range namespaces {
			configMaps, err := c.Client.CoreV1().ConfigMaps(namespace).List(c.Ctx, configMapOpt)
			if err != nil {
				return nil, err
			}
			allConfigMaps = append(allConfigMaps, configMaps.Items...)
		}
	}

	return allConfigMaps, nil
}

func (c *Client) GetSecrets(
	namespaces []string,
	label string,
	labelValue string,
) ([]corev1.Secret, error) {

	labelSelector := label

	if labelValue != "" {
		labelSelector = fmt.Sprintf("%s=%s", label, labelValue)
	}

	secretOpt := metav1.ListOptions{
		LabelSelector: labelSelector,
	}

	var allSecrets []corev1.Secret

	if len(namespaces) == 0 {
		secrets, err := c.Client.CoreV1().Secrets(metav1.NamespaceAll).List(c.Ctx, secretOpt)
		if err != nil {
			return nil, err
		}
		allSecrets = append(allSecrets, secrets.Items...)
	} else {
		for _, namespace := range namespaces {
			secrets, err := c.Client.CoreV1().Secrets(namespace).List(c.Ctx, secretOpt)
			if err != nil {
				return nil, err
			}
			allSecrets = append(allSecrets, secrets.Items...)
		}
	}

	return allSecrets, nil
}

func (c *Client) ConfigMapInformerWorker(
	namespaces []string,
	label string,
	labelValue string,
	writer writer.IWriter,
	notifier notifier.INotifier,
) {

	// event driven worker
	if len(namespaces) == 0 {
		c.configMapInformerWorker(nil, label, labelValue, writer, notifier)
	} else {
		for _, namespace := range namespaces {
			c.configMapInformerWorker(&namespace, label, labelValue, writer, notifier)
		}
	}

	<-c.Ctx.Done()
	c.Wg.Done()
}

func (c *Client) SecretInformerWorker(
	namespaces []string,
	label string,
	labelValue string,
	writer writer.IWriter,
	notifier notifier.INotifier,
) {
	if len(namespaces) == 0 {
		c.secretInformerWorker(nil, label, labelValue, writer, notifier)
	} else {
		for _, namespace := range namespaces {
			c.secretInformerWorker(&namespace, label, labelValue, writer, notifier)
		}
	}

	<-c.Ctx.Done()
	c.Wg.Done()
}

func (c *Client) matchesLabel(resourceLabels map[string]string,
	expectedLabel string,
	expectedLabelValue string) bool {

	if expectedLabel == "" {
		return true
	}

	for resourceLabel, resourceLabelValue := range resourceLabels {
		if expectedLabelValue == "" && resourceLabel == expectedLabel {
			return true
		}

		if resourceLabel == expectedLabel && resourceLabelValue == expectedLabelValue {
			return true
		}
	}

	return false
}

func (c *Client) configMapInformerWorker(
	namespace *string,
	label string,
	labelValue string,
	writer writer.IWriter,
	notifier notifier.INotifier,
) {
	rsync := 0 * time.Second
	labelSelector := label
	if labelValue != "" {
		labelSelector = fmt.Sprintf("%s=%s", label, labelValue)
	}

	var factory informers.SharedInformerFactory

	if namespace == nil {
		factory = informers.NewSharedInformerFactoryWithOptions(
			c.Client,
			rsync,
			informers.WithTweakListOptions(func(options *metav1.ListOptions) {
				options.LabelSelector = labelSelector
			}),
		)

	} else {
		factory = informers.NewSharedInformerFactoryWithOptions(
			c.Client,
			rsync,
			informers.WithNamespace(*namespace),
			informers.WithTweakListOptions(func(options *metav1.ListOptions) {
				options.LabelSelector = labelSelector
			}),
		)
	}

	cmInformer := factory.Core().V1().ConfigMaps().Informer()

	cmInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			cm := obj.(*corev1.ConfigMap)

			if !c.matchesLabel(cm.Labels, label, labelValue) {
				log.Printf("ConfigMap %s does not match label %s=%s", cm.Name, label, labelValue)
				return
			}

			for fileName, data := range cm.Data {
				if !writer.IsJSON(fileName) {
					continue
				}
				writer.Write(fileName, data)
			}
			notifier.Notify()
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			cm := newObj.(*corev1.ConfigMap)

			if !c.matchesLabel(cm.Labels, label, labelValue) {
				log.Printf("ConfigMap %s does not match label %s=%s", cm.Name, label, labelValue)
				return
			}

			for fileName, data := range cm.Data {
				if !writer.IsJSON(fileName) {
					continue
				}
				writer.Write(fileName, data)
			}
		},
		DeleteFunc: func(obj interface{}) {
			cm := obj.(*corev1.ConfigMap)

			if !c.matchesLabel(cm.Labels, label, labelValue) {
				log.Printf("ConfigMap %s does not match label %s=%s", cm.Name, label, labelValue)
				return
			}

			for fileName := range cm.Data {
				if !writer.IsJSON(fileName) {
					continue
				}
				writer.Remove(fileName)
			}
		},
	})

	factory.Start(c.Ctx.Done())
	factory.WaitForCacheSync(c.Ctx.Done())
}

func (c *Client) secretInformerWorker(
	namespace *string,
	label string,
	labelValue string,
	writer writer.IWriter,
	notifier notifier.INotifier,
) {
	rsync := 0 * time.Second
	labelSelector := label
	if labelValue != "" {
		labelSelector = fmt.Sprintf("%s=%s", label, labelValue)
	}

	var factory informers.SharedInformerFactory

	if namespace == nil {
		factory = informers.NewSharedInformerFactoryWithOptions(
			c.Client,
			rsync,
			informers.WithTweakListOptions(func(options *metav1.ListOptions) {
				options.LabelSelector = labelSelector
			}),
		)
	} else {
		factory = informers.NewSharedInformerFactoryWithOptions(
			c.Client,
			rsync,
			informers.WithNamespace(*namespace),
			informers.WithTweakListOptions(func(options *metav1.ListOptions) {
				options.LabelSelector = labelSelector
			}),
		)
	}

	secretInformer := factory.Core().V1().Secrets().Informer()

	secretInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			secret := obj.(*corev1.Secret)
			if !c.matchesLabel(secret.Labels, label, labelValue) {
				log.Printf("Secret %s does not match label %s=%s", secret.Name, label, labelValue)
				return
			}

			for fileName, data := range secret.Data {
				if !writer.IsJSON(fileName) {
					continue
				}
				writer.Write(fileName, string(data))
			}
			notifier.Notify()
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			secret := newObj.(*corev1.Secret)
			if !c.matchesLabel(secret.Labels, label, labelValue) {
				log.Printf("Secret %s does not match label %s=%s", secret.Name, label, labelValue)
				return
			}

			for fileName, data := range secret.Data {
				if !writer.IsJSON(fileName) {
					continue
				}
				writer.Write(fileName, string(data))
			}
		},
		DeleteFunc: func(obj interface{}) {
			secret := obj.(*corev1.Secret)
			if !c.matchesLabel(secret.Labels, label, labelValue) {
				log.Printf("Secret %s does not match label %s=%s", secret.Name, label, labelValue)
				return
			}
			for fileName := range secret.Data {
				if !writer.IsJSON(fileName) {
					continue
				}
				writer.Remove(fileName)
			}
		},
	})

	factory.Start(c.Ctx.Done())
	factory.WaitForCacheSync(c.Ctx.Done())
}
