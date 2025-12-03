package main

import (
	"context"
	"k8s-gsidecar/kubernetes"
	"k8s-gsidecar/notifier"
	"k8s-gsidecar/writer"
	"log"
	"log/slog"
	"os"
	"path"
	"strings"
	"sync"
)

const (
	METHOD                   = "METHOD"
	NAMESPACE                = "NAMESPACE"
	UNIQUE_FILENAMES         = "UNIQUE_FILENAMES"
	FOLDER                   = "FOLDER"
	FOLDER_ANNOTATION        = "FOLDER_ANNOTATION"
	LABEL                    = "LABEL"
	LABEL_VALUE              = "LABEL_VALUE"
	RESOURCE                 = "RESOURCE"
	RESOURCE_NAME            = "RESOURCE_NAME"
	REQ_PAYLOAD              = "REQ_PAYLOAD"
	REQ_URL                  = "REQ_URL"
	REQ_METHOD               = "REQ_METHOD"
	REQ_SKIP_INIT            = "REQ_SKIP_INIT"
	SCRIPT                   = "SCRIPT"
	ENABLE_5XX               = "ENABLE_5XX"
	IGNORE_ALREADY_PROCESSED = "IGNORE_ALREADY_PROCESSED"
	REQ_USERNAME             = "REQ_USERNAME"
	REQ_PASSWORD             = "REQ_PASSWORD"
)

const (
	METHOD_WATCH = "watch"
	METHOD_LIST  = "list"
	METHOD_SLEEP = "sleep"
)

const (
	RESOURCE_ALL       string = "both"
	RESOURCE_CONFIGMAP string = "configmap"
	RESOURCE_SECRET    string = "secret"
)

const (
	DEFAULT_FOLDER_ANNOTATION = "k8s-sidecar-target-directory"
)

type SideCar struct {
	ctx      context.Context
	client   *kubernetes.Client
	writer   writer.IWriter
	notifier notifier.INotifier

	Method                 string
	Namespaces             []string
	Label                  string
	LabelValue             string
	UniqueFilenames        string
	Folder                 string
	FolderAnnotation       string
	Resource               []string
	ResourceName           string
	ReqPayload             string
	ReqURL                 string
	ReqMethod              string
	ReqBasicAuthUsername   string
	ReqBasicAuthPassword   string
	ReqSkipInit            string
	Script                 string
	Enable5XX              string
	IgnoreAlreadyProcessed string
}

func New(ctx context.Context) *SideCar {
	client, err := kubernetes.NewClient(ctx)
	if err != nil {
		l.Error("Failed to create Kubernetes client", "error", err)
	}

	resouce := os.Getenv(RESOURCE)
	reqURL := os.Getenv(REQ_URL)
	reqMethod := os.Getenv(REQ_METHOD)
	reqPayload := os.Getenv(REQ_PAYLOAD)
	reqUsername := os.Getenv(REQ_USERNAME)
	reqPassword := os.Getenv(REQ_PASSWORD)
	folderAnnotation := os.Getenv(FOLDER_ANNOTATION)
	resources := []string{}
	switch resouce {
	case RESOURCE_ALL:
		resources = []string{RESOURCE_CONFIGMAP, RESOURCE_SECRET}
	case RESOURCE_CONFIGMAP:
		resources = []string{RESOURCE_CONFIGMAP}
	case RESOURCE_SECRET:
		resources = []string{RESOURCE_SECRET}
	}
	basicAuth := &notifier.BasicAuth{
		Username: reqUsername,
		Password: reqPassword,
	}
	fw := writer.NewFileWriter()

	notifier := notifier.NewHTTPNotifier(
		reqURL,
		reqMethod,
		basicAuth,
		reqPayload,
	)

	namesapces_env := os.Getenv(NAMESPACE)
	var namespaces []string
	if namesapces_env == "" || namesapces_env == "ALL" {
		namespaces = []string{}
	} else {
		namespaces = strings.Split(namesapces_env, ",")
	}

	if folderAnnotation == "" {
		folderAnnotation = DEFAULT_FOLDER_ANNOTATION
	}

	return &SideCar{
		ctx:                    ctx,
		client:                 client,
		writer:                 fw,
		notifier:               notifier,
		Namespaces:             namespaces,
		Method:                 strings.ToLower(os.Getenv(METHOD)),
		UniqueFilenames:        os.Getenv(UNIQUE_FILENAMES),
		Folder:                 os.Getenv(FOLDER),
		FolderAnnotation:       folderAnnotation,
		Label:                  os.Getenv(LABEL),
		LabelValue:             os.Getenv(LABEL_VALUE),
		Resource:               resources,
		ResourceName:           os.Getenv(RESOURCE_NAME),
		ReqPayload:             reqPayload,
		ReqURL:                 reqURL,
		ReqMethod:              reqMethod,
		ReqBasicAuthUsername:   reqUsername,
		ReqBasicAuthPassword:   reqPassword,
		ReqSkipInit:            os.Getenv(REQ_SKIP_INIT),
		Script:                 os.Getenv(SCRIPT),
		Enable5XX:              os.Getenv(ENABLE_5XX),
		IgnoreAlreadyProcessed: os.Getenv(IGNORE_ALREADY_PROCESSED),
	}
}

func (s *SideCar) Run() {
	l.Info("Running SideCar with method:", "method", s.Method)
	switch s.Method {
	case METHOD_WATCH, METHOD_SLEEP:
		l.Info("Waiting for changes")
		s.syncResources()

		s.WaitForChanges()
	case METHOD_LIST:
		l.Info("Running once")
		s.RunOnce()
	default:
		l.Error("Invalid method:", "error", s.Method)
	}
}

func (s *SideCar) syncResources() {
	l.Info("Syncing resources")
	for _, resource := range s.Resource {
		l.Info("Syncing resource:", "resource", resource)
		switch resource {
		case RESOURCE_CONFIGMAP:
			configMaps, err := s.client.GetConfigMaps(s.Namespaces, s.Label, s.LabelValue)
			l.Info("Got ConfigMaps:", "count", len(configMaps))
			if err != nil {
				l.Error("Failed to get ConfigMaps:", "error", err)
				return
			}

			for _, configMap := range configMaps {
				for fileName, data := range configMap.Data {
					if !s.writer.IsJSON(fileName) {
						continue
					}

					folder := s.Folder

					if s.FolderAnnotation != "" {
						folder = path.Join(s.Folder, configMap.Annotations[s.FolderAnnotation])
					}

					err = s.writer.Write(folder, fileName, data)
					if err != nil {
						log.Fatalf("Failed to write file: %v", err)
					}
				}
			}

		case RESOURCE_SECRET:
			secrets, err := s.client.GetSecrets(s.Namespaces, s.Label, s.LabelValue)
			l.Info("Got Secrets:", "count", len(secrets))
			if err != nil {
				l.Error("Failed to get Secrets:", "error", err)
				return
			}

			for _, secret := range secrets {
				for fileName, data := range secret.Data {
					if !s.writer.IsJSON(fileName) {
						continue
					}

					folder := s.Folder

					if s.FolderAnnotation != "" {
						folder = path.Join(s.Folder, secret.Annotations[s.FolderAnnotation])
					}

					// Secret.Data is []byte, convert to string
					err = s.writer.Write(folder, fileName, string(data))
					if err != nil {
						slog.Error("Failed to write file:", "error", err)
					}
				}
			}
		}
	}
}

func (s *SideCar) RunOnce() {
	s.syncResources()
	s.notifier.Notify()

}

func (s *SideCar) WaitForChanges() {

	s.client.Wg = &sync.WaitGroup{}

	l.Info("Start waiting for changes")

	for _, resource := range s.Resource {
		switch resource {
		case RESOURCE_CONFIGMAP:
			s.client.Wg.Add(1)
			go s.client.ConfigMapInformerWorker(
				s.Namespaces,
				s.Label,
				s.LabelValue,
				s.Folder,
				s.FolderAnnotation,
				s.writer,
				s.notifier,
			)
		case RESOURCE_SECRET:
			s.client.Wg.Add(1)
			go s.client.SecretInformerWorker(
				s.Namespaces,
				s.Label,
				s.LabelValue,
				s.Folder,
				s.FolderAnnotation,
				s.writer,
				s.notifier,
			)
		}
	}
	s.client.Wg.Wait()
}
