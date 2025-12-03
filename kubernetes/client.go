package kubernetes

import (
	"context"
	"flag"
	"fmt"
	"k8s-gsidecar/logger"
	"k8s-gsidecar/notifier"
	"k8s-gsidecar/writer"
	"log/slog"
	"os"
	"path"
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

var l *slog.Logger = logger.GetLogger()

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
			l.Error("Failed to create Kubernetes client", "error", err)
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
		l.Debug("Getting all configmaps")
		configMaps, err := c.Client.CoreV1().ConfigMaps(metav1.NamespaceAll).List(c.Ctx, configMapOpt)
		if err != nil {
			return nil, err
		}
		allConfigMaps = append(allConfigMaps, configMaps.Items...)
	} else {
		l.Debug("Getting configmaps for namespaces", "namespaces", namespaces)
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
		l.Debug("Getting all secrets")
		secrets, err := c.Client.CoreV1().Secrets(metav1.NamespaceAll).List(c.Ctx, secretOpt)
		if err != nil {
			return nil, err
		}
		allSecrets = append(allSecrets, secrets.Items...)
	} else {
		l.Debug("Getting secrets for namespaces", "namespaces", namespaces)
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
	folder string,
	folderAnnotation string,
	writer writer.IWriter,
	notifier notifier.INotifier,
) {

	// event driven worker
	if len(namespaces) == 0 {
		l.Debug("Start waiting for changes for all namespaces")
		c.configMapInformerWorker(nil, label, labelValue, folder, folderAnnotation, writer, notifier)
	} else {
		for _, namespace := range namespaces {
			l.Debug("Start waiting for changes for namespace:", "namespace", namespace)
			c.configMapInformerWorker(&namespace, label, labelValue, folder, folderAnnotation, writer, notifier)
		}
	}

	<-c.Ctx.Done()
	c.Wg.Done()
}

func (c *Client) SecretInformerWorker(
	namespaces []string,
	label string,
	labelValue string,
	folder string,
	folderAnnotation string,
	writer writer.IWriter,
	notifier notifier.INotifier,
) {
	if len(namespaces) == 0 {
		l.Debug("Start waiting for changes for all namespaces")
		c.secretInformerWorker(nil, label, labelValue, folder, folderAnnotation, writer, notifier)
	} else {
		for _, namespace := range namespaces {
			l.Debug("Start waiting for changes for namespace:", "namespace", namespace)
			c.secretInformerWorker(&namespace, label, labelValue, folder, folderAnnotation, writer, notifier)
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
	folder string,
	folderAnnotation string,
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
			l.Debug("ConfigMap added:", "name", obj.(*corev1.ConfigMap).Name)
			cm := obj.(*corev1.ConfigMap)

			if !c.matchesLabel(cm.Labels, label, labelValue) {
				l.Debug("ConfigMap does not match label:", "name", cm.Name, "label", label, "labelValue", labelValue)
				return
			}

			for fileName, data := range cm.Data {
				if !writer.IsJSON(fileName) {
					l.Debug("ConfigMap file is not JSON:", "name", cm.Name, "fileName", fileName)
					continue
				}

				folder := folder

				if folderAnnotation != "" {
					l.Debug("ConfigMap folder annotation:", "name", cm.Name, "folderAnnotation", folderAnnotation)
					folder = path.Join(folder, cm.Annotations[folderAnnotation])
				}

				l.Debug("ConfigMap writing file:", "name", cm.Name, "fileName", fileName)
				writer.Write(folder, fileName, data)
			}
			notifier.Notify()
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			cm := newObj.(*corev1.ConfigMap)

			if !c.matchesLabel(cm.Labels, label, labelValue) {
				l.Debug("ConfigMap does not match label:", "name", cm.Name, "label", label, "labelValue", labelValue)
				return
			}

			for fileName, data := range cm.Data {
				if !writer.IsJSON(fileName) {
					l.Debug("ConfigMap file is not JSON:", "name", cm.Name, "fileName", fileName)
					continue
				}

				folder := folder

				if folderAnnotation != "" {
					folder = path.Join(folder, cm.Annotations[folderAnnotation])
				}

				l.Debug("ConfigMap updating file:", "name", cm.Name, "fileName", fileName)
				writer.Write(folder, fileName, data)
			}
		},
		DeleteFunc: func(obj interface{}) {
			cm := obj.(*corev1.ConfigMap)

			if !c.matchesLabel(cm.Labels, label, labelValue) {
				l.Debug("ConfigMap does not match label:", "name", cm.Name, "label", label, "labelValue", labelValue)
				return
			}

			for fileName := range cm.Data {
				if !writer.IsJSON(fileName) {
					l.Debug("ConfigMap file is not JSON:", "name", cm.Name, "fileName", fileName)
					continue
				}

				folder := folder

				if folderAnnotation != "" {
					folder = path.Join(folder, cm.Annotations[folderAnnotation])
				}

				l.Debug("ConfigMap removing file:", "name", cm.Name, "fileName", fileName)
				writer.Remove(folder, fileName)
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
	folder string,
	folderAnnotation string,
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
				l.Debug("Secret does not match label:", "name", secret.Name, "label", label, "labelValue", labelValue)
				return
			}

			for fileName, data := range secret.Data {
				if !writer.IsJSON(fileName) {
					l.Debug("Secret file is not JSON:", "name", secret.Name, "fileName", fileName)
					continue
				}

				folder := folder

				if folderAnnotation != "" {
					l.Debug("Secret folder annotation:", "name", secret.Name, "folderAnnotation", folderAnnotation)
					folder = path.Join(folder, secret.Annotations[folderAnnotation])
				}

				l.Debug("Secret writing file:", "name", secret.Name, "fileName", fileName)
				writer.Write(folder, fileName, string(data))
			}
			notifier.Notify()
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			secret := newObj.(*corev1.Secret)
			if !c.matchesLabel(secret.Labels, label, labelValue) {
				l.Debug("Secret does not match label:", "name", secret.Name, "label", label, "labelValue", labelValue)
				return
			}

			for fileName, data := range secret.Data {
				if !writer.IsJSON(fileName) {
					l.Debug("Secret file is not JSON:", "name", secret.Name, "fileName", fileName)
					continue
				}

				folder := folder

				if folderAnnotation != "" {
					l.Debug("Secret folder annotation:", "name", secret.Name, "folderAnnotation", folderAnnotation)
					folder = path.Join(folder, secret.Annotations[folderAnnotation])
				}

				l.Debug("Secret updating file:", "name", secret.Name, "fileName", fileName)
				writer.Write(folder, fileName, string(data))
			}
		},
		DeleteFunc: func(obj interface{}) {
			secret := obj.(*corev1.Secret)
			if !c.matchesLabel(secret.Labels, label, labelValue) {
				l.Debug("Secret does not match label:", "name", secret.Name, "label", label, "labelValue", labelValue)
				return
			}
			for fileName := range secret.Data {
				if !writer.IsJSON(fileName) {
					l.Debug("Secret file is not JSON:", "name", secret.Name, "fileName", fileName)
					continue
				}

				folder := folder

				if folderAnnotation != "" {
					l.Debug("Secret folder annotation:", "name", secret.Name, "folderAnnotation", folderAnnotation)
					folder = path.Join(folder, secret.Annotations[folderAnnotation])
				}

				l.Debug("Secret removing file:", "name", secret.Name, "fileName", fileName)
				writer.Remove(folder, fileName)
			}
		},
	})

	factory.Start(c.Ctx.Done())
	factory.WaitForCacheSync(c.Ctx.Done())
}
