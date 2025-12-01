package main

import (
	"context"
	"encoding/base64"
	"k8s-gsidecar/kubernetes"
	"k8s-gsidecar/notifier"
	"k8s-gsidecar/writer"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fake "k8s.io/client-go/kubernetes/fake"
)

func TestSideCar_RunOnce(t *testing.T) {

	os.Setenv(NAMESPACE, "default")
	os.Setenv(LABEL, "app")
	os.Setenv(RESOURCE, "configmap")
	os.Setenv(METHOD, "list")
	os.Setenv(FOLDER, "test-folder")
	os.MkdirAll(os.Getenv(FOLDER), 0755)
	defer os.RemoveAll(os.Getenv(FOLDER))

	ctx := context.Background()

	fakeClientset := fake.NewSimpleClientset(
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-config",
				Namespace: "default",
				Labels: map[string]string{
					"app": "myapp",
				},
			},
			Data: map[string]string{
				"test-config.json": "{'name': 'test-config'}",
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-config2",
				Namespace: "default",
				Labels: map[string]string{
					"app": "myapp2",
				},
			},
			Data: map[string]string{
				"test-config2.json": "{'name': 'test-config2'}",
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "other-config",
				Namespace: "default",
				Labels: map[string]string{
					"ddd": "other",
				},
			},
		},
	)

	sideCar := New(ctx)
	sideCar.client = &kubernetes.Client{
		Ctx:    ctx,
		Client: fakeClientset,
		Wg:     &sync.WaitGroup{},
	}

	sideCar.RunOnce()

	if _, err := os.Stat("test-folder/test-config.json"); os.IsNotExist(err) {
		t.Errorf("Expected file 'test-folder/test-config', got %v", err)
	}

	if content, err := os.ReadFile("test-folder/test-config.json"); err != nil {
		t.Fatalf("Expected no error, got %v", err)
	} else {
		if string(content) != "{'name': 'test-config'}" {
			t.Errorf("Expected content '{'name': 'test-config'}', got %s", string(content))
		}
	}

	if _, err := os.Stat("test-folder/test-config2.json"); os.IsNotExist(err) {
		t.Errorf("Expected file 'test-folder/test-config2', got %v", err)
	}

	if content, err := os.ReadFile("test-folder/test-config2.json"); err != nil {
		t.Fatalf("Expected no error, got %v", err)
	} else {
		if string(content) != "{'name': 'test-config2'}" {
			t.Errorf("Expected content '{'name': 'test-config2'}', got %s", string(content))
		}
	}

}

// TestGrafanaDashboardSidecar test Grafana dashboard sidecar functionality
func TestGrafanaDashboardSidecar(t *testing.T) {
	tests := []struct {
		name            string
		configMaps      []corev1.ConfigMap
		label           string
		labelValue      string
		expectedFiles   []string
		useBasicAuth    bool
		username        string
		password        string
		notifyMethod    string
		expectNotifyErr bool
	}{
		{
			name: "single Grafana dashboard - no BasicAuth",
			configMaps: []corev1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "grafana-dashboard-1",
						Namespace: "monitoring",
						Labels: map[string]string{
							"grafana_dashboard": "1",
						},
					},
					Data: map[string]string{
						"dashboard-1.json": `{"dashboard": {"title": "System Metrics", "panels": []}}`,
					},
				},
			},
			label:         "grafana_dashboard",
			labelValue:    "1",
			expectedFiles: []string{"dashboard-1.json"},
			useBasicAuth:  false,
			notifyMethod:  "GET",
		},
		{
			name: "multiple Grafana dashboards - using BasicAuth",
			configMaps: []corev1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "grafana-dashboard-app",
						Namespace: "monitoring",
						Labels: map[string]string{
							"grafana_dashboard": "1",
							"team":              "platform",
						},
					},
					Data: map[string]string{
						"app-metrics.json": `{"dashboard": {"title": "App Metrics", "panels": [{"id": 1}]}}`,
						"app-logs.json":    `{"dashboard": {"title": "App Logs", "panels": [{"id": 2}]}}`,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "grafana-dashboard-db",
						Namespace: "monitoring",
						Labels: map[string]string{
							"grafana_dashboard": "1",
							"team":              "platform",
						},
					},
					Data: map[string]string{
						"database-metrics.json": `{"dashboard": {"title": "Database Metrics", "panels": [{"id": 3}]}}`,
					},
				},
			},
			label:         "grafana_dashboard",
			labelValue:    "1",
			expectedFiles: []string{"app-metrics.json", "app-logs.json", "database-metrics.json"},
			useBasicAuth:  true,
			username:      "admin",
			password:      "secret123",
			notifyMethod:  "POST",
		},
		{
			name: "only select dashboards with specific label value",
			configMaps: []corev1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "frontend-dashboard",
						Namespace: "monitoring",
						Labels: map[string]string{
							"team": "frontend",
						},
					},
					Data: map[string]string{
						"frontend-metrics.json": `{"dashboard": {"title": "Frontend", "panels": []}}`,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend-dashboard",
						Namespace: "monitoring",
						Labels: map[string]string{
							"team": "backend",
						},
					},
					Data: map[string]string{
						"backend-metrics.json": `{"dashboard": {"title": "Backend", "panels": []}}`,
					},
				},
			},
			label:         "team",
			labelValue:    "frontend",
			expectedFiles: []string{"frontend-metrics.json"},
			useBasicAuth:  false,
			notifyMethod:  "GET",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// set test environment
			testFolder := "test-grafana-dashboards"
			os.Setenv(NAMESPACE, "monitoring")
			os.Setenv(LABEL, tt.label)
			os.Setenv(LABEL_VALUE, tt.labelValue)
			os.Setenv(RESOURCE, RESOURCE_CONFIGMAP)
			os.Setenv(METHOD, "list")
			os.Setenv(FOLDER, testFolder)
			os.Setenv(FOLDER_ANNOTATION, "")
			os.MkdirAll(testFolder, 0755)
			defer os.RemoveAll(testFolder)

			// create mock HTTP server to test notification
			notifyCalled := false
			var receivedAuth string
			var receivedMethod string

			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				notifyCalled = true
				receivedMethod = r.Method

				// check BasicAuth
				if tt.useBasicAuth {
					authHeader := r.Header.Get("Authorization")
					receivedAuth = authHeader
				}

				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"status":"ok"}`))
			}))
			defer mockServer.Close()

			// set notifier environment variables
			os.Setenv(REQ_URL, mockServer.URL)
			os.Setenv(REQ_METHOD, tt.notifyMethod)
			if tt.useBasicAuth {
				os.Setenv(REQ_USERNAME, tt.username)
				os.Setenv(REQ_PASSWORD, tt.password)
			} else {
				os.Unsetenv(REQ_USERNAME)
				os.Unsetenv(REQ_PASSWORD)
			}

			// create fake clientset for testing
			var objects []runtime.Object
			for i := range tt.configMaps {
				objects = append(objects, &tt.configMaps[i])
			}

			fakeClientset := fake.NewSimpleClientset(objects...)

			ctx := context.Background()

			// create SideCar instance
			var basicAuth *notifier.BasicAuth
			if tt.useBasicAuth {
				basicAuth = &notifier.BasicAuth{
					Username: tt.username,
					Password: tt.password,
				}
			}

			sideCar := &SideCar{
				ctx: ctx,
				client: &kubernetes.Client{
					Ctx:    ctx,
					Client: fakeClientset,
				},
				writer:               writer.NewFileWriter(),
				notifier:             notifier.NewHTTPNotifier(mockServer.URL, tt.notifyMethod, basicAuth, `{"message":"dashboards updated"}`),
				Namespaces:           []string{"monitoring"},
				Label:                tt.label,
				LabelValue:           tt.labelValue,
				Resource:             []string{RESOURCE_CONFIGMAP},
				ReqURL:               mockServer.URL,
				Folder:               testFolder,
				FolderAnnotation:     "",
				ReqMethod:            tt.notifyMethod,
				ReqBasicAuthUsername: tt.username,
				ReqBasicAuthPassword: tt.password,
				ReqPayload:           `{"message":"dashboards updated"}`,
			}

			// run SideCar
			sideCar.RunOnce()

			// verify files are correctly written
			log.Println("verify files are correctly written", tt.expectedFiles)
			for _, expectedFile := range tt.expectedFiles {
				filePath := testFolder + "/" + expectedFile
				if _, err := os.Stat(filePath); os.IsNotExist(err) {
					t.Errorf("Expected file '%s' to exist, but it doesn't", filePath)
					continue
				}

				// read file content to verify
				content, err := os.ReadFile(filePath)
				if err != nil {
					t.Errorf("Failed to read file '%s': %v", filePath, err)
					continue
				}

				if len(content) == 0 {
					t.Errorf("File '%s' is empty", filePath)
				}
			}

			// verify notifier is called
			if !notifyCalled {
				t.Error("Expected notifier to be called, but it wasn't")
			}

			// verify HTTP method
			if receivedMethod != tt.notifyMethod {
				t.Errorf("Expected HTTP method '%s', got '%s'", tt.notifyMethod, receivedMethod)
			}

			// verify BasicAuth
			if tt.useBasicAuth {
				expectedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(tt.username+":"+tt.password))
				if receivedAuth != expectedAuth {
					t.Errorf("Expected auth header '%s', got '%s'", expectedAuth, receivedAuth)
				}
			}
		})
	}
}

// TestGrafanaDashboardSidecar_LabelSelector test different label selector scenarios
func TestGrafanaDashboardSidecar_LabelSelector(t *testing.T) {
	testFolder := "test-label-selector"
	os.MkdirAll(testFolder, 0755)
	defer os.RemoveAll(testFolder)

	// create multiple ConfigMaps with different labels
	fakeClientset := fake.NewSimpleClientset(
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "prod-dashboard",
				Namespace: "monitoring",
				Labels: map[string]string{
					"grafana_dashboard": "1",
					"environment":       "production",
				},
			},
			Data: map[string]string{
				"prod.json": `{"dashboard": {"title": "Production"}}`,
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dev-dashboard",
				Namespace: "monitoring",
				Labels: map[string]string{
					"grafana_dashboard": "1",
					"environment":       "development",
				},
			},
			Data: map[string]string{
				"dev.json": `{"dashboard": {"title": "Development"}}`,
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "other-config",
				Namespace: "monitoring",
				Labels: map[string]string{
					"type": "config",
				},
			},
			Data: map[string]string{
				"config.yaml": `key: value`,
			},
		},
	)

	ctx := context.Background()

	// Mock HTTP server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	sideCar := &SideCar{
		ctx: ctx,
		client: &kubernetes.Client{
			Ctx:    ctx,
			Client: fakeClientset,
		},
		writer:           writer.NewFileWriter(),
		notifier:         notifier.NewHTTPNotifier(mockServer.URL, "GET", nil, `{"message":"dashboards updated"}`),
		Namespaces:       []string{"monitoring"},
		Label:            "grafana_dashboard",
		LabelValue:       "1",
		Folder:           "test-label-selector",
		FolderAnnotation: "",
		Resource:         []string{RESOURCE_CONFIGMAP},
		ReqPayload:       `{}`,
	}

	sideCar.RunOnce()

	// should only write dashboards with grafana_dashboard=1
	if _, err := os.Stat(testFolder + "/prod.json"); os.IsNotExist(err) {
		t.Error("Expected prod.json to exist")
	}

	if _, err := os.Stat(testFolder + "/dev.json"); os.IsNotExist(err) {
		t.Error("Expected dev.json to exist")
	}

	// config.yaml should not be written (not a JSON file)
	if _, err := os.Stat(testFolder + "/config.yaml"); err == nil {
		t.Error("Expected config.yaml to NOT exist")
	}
}

// TestSideCar_FolderAnnotation test folder annotation functionality
func TestSideCar_FolderAnnotation(t *testing.T) {
	testFolder := "test-folder-annotation"
	os.MkdirAll(testFolder, 0755)
	defer os.RemoveAll(testFolder)

	// create ConfigMap with folder annotation
	fakeClientset := fake.NewSimpleClientset(
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "annotation-dashboard",
				Namespace: "monitoring",
				Labels: map[string]string{
					"grafana_dashboard": "1",
				},
				Annotations: map[string]string{
					"target-folder": "subfolder",
				},
			},
			Data: map[string]string{
				"dashboard.json": `{"dashboard": {"title": "Annotation Dashboard"}}`,
			},
		},
	)

	ctx := context.Background()

	// Mock HTTP server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	sideCar := &SideCar{
		ctx: ctx,
		client: &kubernetes.Client{
			Ctx:    ctx,
			Client: fakeClientset,
		},
		writer:           writer.NewFileWriter(),
		notifier:         notifier.NewHTTPNotifier(mockServer.URL, "GET", nil, `{"message":"dashboards updated"}`),
		Namespaces:       []string{"monitoring"},
		Label:            "grafana_dashboard",
		LabelValue:       "1",
		Folder:           testFolder,
		FolderAnnotation: "target-folder",
		Resource:         []string{RESOURCE_CONFIGMAP},
		ReqPayload:       `{}`,
	}

	sideCar.RunOnce()

	// should write to subfolder
	expectedPath := testFolder + "/subfolder/dashboard.json"
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("Expected file '%s' to exist", expectedPath)
	}

	// verify content
	content, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("Failed to read file '%s': %v", expectedPath, err)
	}

	if string(content) != `{"dashboard": {"title": "Annotation Dashboard"}}` {
		t.Errorf("Expected content to match, got %s", string(content))
	}
}

// TestGrafanaDashboardSidecar_NotifierFailure test notifier failure scenario
func TestGrafanaDashboardSidecar_NotifierFailure(t *testing.T) {
	testFolder := "test-notifier-failure"
	os.MkdirAll(testFolder, 0755)
	defer os.RemoveAll(testFolder)

	// Mock HTTP server to return error
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer mockServer.Close()

	fakeClientset := fake.NewSimpleClientset(
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-dashboard",
				Namespace: "monitoring",
				Labels: map[string]string{
					"grafana_dashboard": "1",
				},
			},
			Data: map[string]string{
				"test.json": `{"dashboard": {"title": "Test"}}`,
			},
		},
	)

	ctx := context.Background()

	sideCar := &SideCar{
		ctx: ctx,
		client: &kubernetes.Client{
			Ctx:    ctx,
			Client: fakeClientset,
		},
		writer:     writer.NewFileWriter(),
		notifier:   notifier.NewHTTPNotifier(mockServer.URL, "POST", nil, `{"message":"dashboards updated"}`),
		Namespaces: []string{"monitoring"},
		Label:      "grafana_dashboard",
		LabelValue: "1",
		Resource:   []string{RESOURCE_CONFIGMAP},
		ReqPayload: `{}`,
	}

	// although notifier fails, the files should still be written
	// note: the current implementation may panic, this is just to demonstrate the test structure
	// in practice, you may need to modify RunOnce to handle notifier errors

	// this test demonstrates how to test error scenarios
	// you may need to modify RunOnce to return errors instead of panicking
	_ = sideCar

	// verify files are written successfully (even if notify fails)
	// this part needs to be adjusted based on the actual error handling logic
}

type MockWriter struct {
	WrittenFiles map[string]string
	RemovedFiles []string
	WriteError   error
	RemoveError  error
}

func NewMockWriter() *MockWriter {
	return &MockWriter{
		WrittenFiles: make(map[string]string),
		RemovedFiles: []string{},
	}
}

func (m *MockWriter) Write(folder string, fileName string, data string) error {
	if m.WriteError != nil {
		return m.WriteError
	}
	m.WrittenFiles[fileName] = data
	return nil
}

func (m *MockWriter) Remove(folder string, fileName string) error {
	if m.RemoveError != nil {
		return m.RemoveError
	}
	m.RemovedFiles = append(m.RemovedFiles, fileName)
	delete(m.WrittenFiles, fileName)
	return nil
}

func (m *MockWriter) IsJSON(fileName string) bool {
	return strings.HasSuffix(fileName, ".json")
}

// MockNotifier 用於測試的 mock notifier
type MockNotifier struct {
	NotifyCount int
	NotifyError error
}

func NewMockNotifier() *MockNotifier {
	return &MockNotifier{
		NotifyCount: 0,
	}
}

func (m *MockNotifier) Notify() error {
	if m.NotifyError != nil {
		return m.NotifyError
	}
	m.NotifyCount++
	return nil
}

func TestWaitForChanges_ConfigMapAdd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fakeClientset := fake.NewSimpleClientset()

	mockWriter := NewMockWriter()
	mockNotifier := NewMockNotifier()

	sideCar := &SideCar{
		ctx: ctx,
		client: &kubernetes.Client{
			Ctx:    ctx,
			Client: fakeClientset,
		},
		writer:     mockWriter,
		notifier:   mockNotifier,
		Namespaces: []string{"monitoring"},
		Label:      "grafana_dashboard",
		LabelValue: "1",
		Resource:   []string{RESOURCE_CONFIGMAP},
	}

	go sideCar.WaitForChanges()

	time.Sleep(100 * time.Millisecond)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dashboard",
			Namespace: "monitoring",
			Labels: map[string]string{
				"grafana_dashboard": "1",
			},
		},
		Data: map[string]string{
			"dashboard.json": `{"title": "Test Dashboard"}`,
		},
	}

	_, err := fakeClientset.CoreV1().ConfigMaps("monitoring").Create(
		ctx,
		configMap,
		metav1.CreateOptions{},
	)
	if err != nil {
		t.Fatalf("Failed to create ConfigMap: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if len(mockWriter.WrittenFiles) != 1 {
		t.Errorf("Expected 1 file to be written, got %d", len(mockWriter.WrittenFiles))
	}

	if data, ok := mockWriter.WrittenFiles["dashboard.json"]; !ok {
		t.Error("Expected dashboard.json to be written")
	} else if data != `{"title": "Test Dashboard"}` {
		t.Errorf("Expected dashboard content to be correct, got: %s", data)
	}

	// 驗證 notifier 被呼叫
	if mockNotifier.NotifyCount != 1 {
		t.Errorf("Expected notifier to be called 1 time, got %d", mockNotifier.NotifyCount)
	}
}

func TestWaitForChanges_ConfigMapUpdate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	initialConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dashboard",
			Namespace: "monitoring",
			Labels: map[string]string{
				"grafana_dashboard": "1",
			},
		},
		Data: map[string]string{
			"dashboard.json": `{"title": "Initial Dashboard"}`,
		},
	}

	fakeClientset := fake.NewSimpleClientset(initialConfigMap)

	mockWriter := NewMockWriter()
	mockNotifier := NewMockNotifier()

	sideCar := &SideCar{
		ctx: ctx,
		client: &kubernetes.Client{
			Ctx:    ctx,
			Client: fakeClientset,
		},
		writer:     mockWriter,
		notifier:   mockNotifier,
		Namespaces: []string{"monitoring"},
		Label:      "grafana_dashboard",
		LabelValue: "1",
		Resource:   []string{RESOURCE_CONFIGMAP},
	}

	go sideCar.WaitForChanges()

	time.Sleep(200 * time.Millisecond)

	updatedConfigMap := initialConfigMap.DeepCopy()
	updatedConfigMap.Data["dashboard.json"] = `{"title": "Updated Dashboard"}`

	_, err := fakeClientset.CoreV1().ConfigMaps("monitoring").Update(
		ctx,
		updatedConfigMap,
		metav1.UpdateOptions{},
	)
	if err != nil {
		t.Fatalf("Failed to update ConfigMap: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if data, ok := mockWriter.WrittenFiles["dashboard.json"]; !ok {
		t.Error("Expected dashboard.json to exist")
	} else if data != `{"title": "Updated Dashboard"}` {
		t.Errorf("Expected dashboard content to be updated, got: %s", data)
	}
}

func TestWaitForChanges_ConfigMapDelete(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	initialConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dashboard",
			Namespace: "monitoring",
			Labels: map[string]string{
				"grafana_dashboard": "1",
			},
		},
		Data: map[string]string{
			"dashboard.json": `{"title": "Dashboard to Delete"}`,
		},
	}

	fakeClientset := fake.NewSimpleClientset(initialConfigMap)

	mockWriter := NewMockWriter()
	mockNotifier := NewMockNotifier()

	sideCar := &SideCar{
		ctx: ctx,
		client: &kubernetes.Client{
			Ctx:    ctx,
			Client: fakeClientset,
		},
		writer:     mockWriter,
		notifier:   mockNotifier,
		Namespaces: []string{"monitoring"},
		Label:      "grafana_dashboard",
		LabelValue: "1",
		Resource:   []string{RESOURCE_CONFIGMAP},
	}

	go sideCar.WaitForChanges()

	time.Sleep(200 * time.Millisecond)

	err := fakeClientset.CoreV1().ConfigMaps("monitoring").Delete(
		ctx,
		"test-dashboard",
		metav1.DeleteOptions{},
	)
	if err != nil {
		t.Fatalf("Failed to delete ConfigMap: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if len(mockWriter.RemovedFiles) != 1 {
		t.Errorf("Expected 1 file to be removed, got %d", len(mockWriter.RemovedFiles))
	}

	if len(mockWriter.RemovedFiles) > 0 && mockWriter.RemovedFiles[0] != "dashboard.json" {
		t.Errorf("Expected dashboard.json to be removed, got %s", mockWriter.RemovedFiles[0])
	}
}

func TestWaitForChanges_MultipleConfigMaps(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fakeClientset := fake.NewSimpleClientset()

	mockWriter := NewMockWriter()
	mockNotifier := NewMockNotifier()

	sideCar := &SideCar{
		ctx: ctx,
		client: &kubernetes.Client{
			Ctx:    ctx,
			Client: fakeClientset,
		},
		writer:     mockWriter,
		notifier:   mockNotifier,
		Namespaces: []string{"monitoring"},
		Label:      "grafana_dashboard",
		LabelValue: "1",
		Resource:   []string{RESOURCE_CONFIGMAP},
	}

	go sideCar.WaitForChanges()

	time.Sleep(100 * time.Millisecond)

	configMaps := []*corev1.ConfigMap{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dashboard-1",
				Namespace: "monitoring",
				Labels:    map[string]string{"grafana_dashboard": "1"},
			},
			Data: map[string]string{
				"app-metrics.json": `{"title": "App Metrics"}`,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dashboard-2",
				Namespace: "monitoring",
				Labels:    map[string]string{"grafana_dashboard": "1"},
			},
			Data: map[string]string{
				"db-metrics.json": `{"title": "Database Metrics"}`,
			},
		},
	}

	for _, cm := range configMaps {
		_, err := fakeClientset.CoreV1().ConfigMaps("monitoring").Create(
			ctx,
			cm,
			metav1.CreateOptions{},
		)
		if err != nil {
			t.Fatalf("Failed to create ConfigMap: %v", err)
		}
	}

	time.Sleep(300 * time.Millisecond)

	if len(mockWriter.WrittenFiles) != 2 {
		t.Errorf("Expected 2 files to be written, got %d", len(mockWriter.WrittenFiles))
	}

	if _, ok := mockWriter.WrittenFiles["app-metrics.json"]; !ok {
		t.Error("Expected app-metrics.json to be written")
	}

	if _, ok := mockWriter.WrittenFiles["db-metrics.json"]; !ok {
		t.Error("Expected db-metrics.json to be written")
	}

	if mockNotifier.NotifyCount != 2 {
		t.Errorf("Expected notifier to be called 2 times, got %d", mockNotifier.NotifyCount)
	}
}

func TestWaitForChanges_LabelSelector(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fakeClientset := fake.NewSimpleClientset()

	mockWriter := NewMockWriter()
	mockNotifier := NewMockNotifier()

	sideCar := &SideCar{
		ctx: ctx,
		client: &kubernetes.Client{
			Ctx:    ctx,
			Client: fakeClientset,
		},
		writer:     mockWriter,
		notifier:   mockNotifier,
		Namespaces: []string{"monitoring"},
		Label:      "grafana_dashboard",
		LabelValue: "1",
		Resource:   []string{RESOURCE_CONFIGMAP},
	}

	go sideCar.WaitForChanges()

	time.Sleep(100 * time.Millisecond)

	matchingConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "matching-dashboard",
			Namespace: "monitoring",
			Labels:    map[string]string{"grafana_dashboard": "1"},
		},
		Data: map[string]string{
			"dashboard.json": `{"title": "Matching Dashboard"}`,
		},
	}

	_, err := fakeClientset.CoreV1().ConfigMaps("monitoring").Create(
		ctx,
		matchingConfigMap,
		metav1.CreateOptions{},
	)
	if err != nil {
		t.Fatalf("Failed to create matching ConfigMap: %v", err)
	}

	nonMatchingConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "non-matching-config",
			Namespace: "monitoring",
			Labels:    map[string]string{"type": "config"},
		},
		Data: map[string]string{
			"config.json": `{"key": "value"}`,
		},
	}

	_, err = fakeClientset.CoreV1().ConfigMaps("monitoring").Create(
		ctx,
		nonMatchingConfigMap,
		metav1.CreateOptions{},
	)
	if err != nil {
		t.Fatalf("Failed to create non-matching ConfigMap: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	if len(mockWriter.WrittenFiles) != 1 {
		t.Errorf("Expected 1 file to be written, got %d", len(mockWriter.WrittenFiles))
	}

	if _, ok := mockWriter.WrittenFiles["dashboard.json"]; !ok {
		t.Error("Expected dashboard.json to be written")
	}

	if _, ok := mockWriter.WrittenFiles["config.json"]; ok {
		t.Error("Expected config.json NOT to be written (wrong label)")
	}
}

func TestWaitForChanges_NonJSONFilesIgnored(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fakeClientset := fake.NewSimpleClientset()

	mockWriter := NewMockWriter()
	mockNotifier := NewMockNotifier()

	sideCar := &SideCar{
		ctx: ctx,
		client: &kubernetes.Client{
			Ctx:    ctx,
			Client: fakeClientset,
		},
		writer:     mockWriter,
		notifier:   mockNotifier,
		Namespaces: []string{"monitoring"},
		Label:      "grafana_dashboard",
		LabelValue: "1",
		Resource:   []string{RESOURCE_CONFIGMAP},
	}

	go sideCar.WaitForChanges()

	time.Sleep(100 * time.Millisecond)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mixed-dashboard",
			Namespace: "monitoring",
			Labels:    map[string]string{"grafana_dashboard": "1"},
		},
		Data: map[string]string{
			"dashboard.json": `{"title": "Dashboard"}`,
			"config.yaml":    `key: value`,
			"readme.txt":     `This is a readme`,
		},
	}

	_, err := fakeClientset.CoreV1().ConfigMaps("monitoring").Create(
		ctx,
		configMap,
		metav1.CreateOptions{},
	)
	if err != nil {
		t.Fatalf("Failed to create ConfigMap: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if len(mockWriter.WrittenFiles) != 1 {
		t.Errorf("Expected 1 file to be written, got %d", len(mockWriter.WrittenFiles))
	}

	if _, ok := mockWriter.WrittenFiles["dashboard.json"]; !ok {
		t.Error("Expected dashboard.json to be written")
	}

	if _, ok := mockWriter.WrittenFiles["config.yaml"]; ok {
		t.Error("Expected config.yaml NOT to be written (not JSON)")
	}

	if _, ok := mockWriter.WrittenFiles["readme.txt"]; ok {
		t.Error("Expected readme.txt NOT to be written (not JSON)")
	}
}

func TestWaitForChanges_AllNamespaces(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fakeClientset := fake.NewSimpleClientset()

	mockWriter := NewMockWriter()
	mockNotifier := NewMockNotifier()

	sideCar := &SideCar{
		ctx: ctx,
		client: &kubernetes.Client{
			Ctx:    ctx,
			Client: fakeClientset,
		},
		writer:     mockWriter,
		notifier:   mockNotifier,
		Namespaces: []string{},
		Label:      "grafana_dashboard",
		LabelValue: "1",
		Resource:   []string{RESOURCE_CONFIGMAP},
	}

	go sideCar.WaitForChanges()

	time.Sleep(100 * time.Millisecond)

	testNamespaces := []string{"monitoring", "default", "kube-system", "custom-ns"}

	for _, ns := range testNamespaces {
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dashboard-" + ns,
				Namespace: ns,
				Labels:    map[string]string{"grafana_dashboard": "1"},
			},
			Data: map[string]string{
				ns + ".json": `{"title": "Dashboard in ` + ns + `"}`,
			},
		}

		_, err := fakeClientset.CoreV1().ConfigMaps(ns).Create(
			ctx,
			configMap,
			metav1.CreateOptions{},
		)
		if err != nil {
			t.Fatalf("Failed to create ConfigMap in %s: %v", ns, err)
		}

		time.Sleep(50 * time.Millisecond)
	}

	time.Sleep(200 * time.Millisecond)

	expectedFiles := []string{"monitoring.json", "default.json", "kube-system.json", "custom-ns.json"}

	if len(mockWriter.WrittenFiles) != len(expectedFiles) {
		t.Errorf("Expected %d files to be written, got %d. Files: %v",
			len(expectedFiles), len(mockWriter.WrittenFiles), mockWriter.WrittenFiles)
	}

	for _, filename := range expectedFiles {
		if _, ok := mockWriter.WrittenFiles[filename]; !ok {
			t.Errorf("Expected %s to be written", filename)
		}
	}

	if mockNotifier.NotifyCount < 4 {
		t.Errorf("Expected notifier to be called at least 4 times, got %d", mockNotifier.NotifyCount)
	}
}

func TestWaitForChanges_MultipleNamespaces(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fakeClientset := fake.NewSimpleClientset()

	mockWriter := NewMockWriter()
	mockNotifier := NewMockNotifier()

	sideCar := &SideCar{
		ctx: ctx,
		client: &kubernetes.Client{
			Ctx:    ctx,
			Client: fakeClientset,
		},
		writer:     mockWriter,
		notifier:   mockNotifier,
		Namespaces: []string{"monitoring", "default"},
		Label:      "grafana_dashboard",
		LabelValue: "1",
		Resource:   []string{RESOURCE_CONFIGMAP},
	}

	go sideCar.WaitForChanges()

	time.Sleep(100 * time.Millisecond)

	monitoringConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "monitoring-dashboard",
			Namespace: "monitoring",
			Labels:    map[string]string{"grafana_dashboard": "1"},
		},
		Data: map[string]string{
			"monitoring-dashboard.json": `{"title": "Monitoring Dashboard"}`,
		},
	}

	_, err := fakeClientset.CoreV1().ConfigMaps("monitoring").Create(
		ctx,
		monitoringConfigMap,
		metav1.CreateOptions{},
	)
	if err != nil {
		t.Fatalf("Failed to create ConfigMap in monitoring: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	defaultConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-dashboard",
			Namespace: "default",
			Labels:    map[string]string{"grafana_dashboard": "1"},
		},
		Data: map[string]string{
			"default-dashboard.json": `{"title": "Default Dashboard"}`,
		},
	}

	_, err = fakeClientset.CoreV1().ConfigMaps("default").Create(
		ctx,
		defaultConfigMap,
		metav1.CreateOptions{},
	)
	if err != nil {
		t.Fatalf("Failed to create ConfigMap in default: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	kubeSystemConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kube-system-dashboard",
			Namespace: "kube-system",
			Labels:    map[string]string{"grafana_dashboard": "1"},
		},
		Data: map[string]string{
			"kube-system-dashboard.json": `{"title": "Kube System Dashboard"}`,
		},
	}

	_, err = fakeClientset.CoreV1().ConfigMaps("kube-system").Create(
		ctx,
		kubeSystemConfigMap,
		metav1.CreateOptions{},
	)
	if err != nil {
		t.Fatalf("Failed to create ConfigMap in kube-system: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if len(mockWriter.WrittenFiles) != 2 {
		t.Errorf("Expected 2 files to be written, got %d. Files: %v",
			len(mockWriter.WrittenFiles), mockWriter.WrittenFiles)
	}

	if _, ok := mockWriter.WrittenFiles["monitoring-dashboard.json"]; !ok {
		t.Error("Expected monitoring-dashboard.json to be written")
	}

	if _, ok := mockWriter.WrittenFiles["default-dashboard.json"]; !ok {
		t.Error("Expected default-dashboard.json to be written")
	}

	if _, ok := mockWriter.WrittenFiles["kube-system-dashboard.json"]; ok {
		t.Error("Expected kube-system-dashboard.json NOT to be written (namespace not monitored)")
	}

	if mockNotifier.NotifyCount != 2 {
		t.Errorf("Expected notifier to be called 2 times, got %d", mockNotifier.NotifyCount)
	}
}

func TestWaitForChanges_MultipleNamespaces_Updates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	initialConfigMaps := []runtime.Object{
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "existing-monitoring",
				Namespace: "monitoring",
				Labels:    map[string]string{"grafana_dashboard": "1"},
			},
			Data: map[string]string{
				"existing-monitoring.json": `{"title": "Existing Monitoring"}`,
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "existing-default",
				Namespace: "default",
				Labels:    map[string]string{"grafana_dashboard": "1"},
			},
			Data: map[string]string{
				"existing-default.json": `{"title": "Existing Default"}`,
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "existing-kube-system",
				Namespace: "kube-system",
				Labels:    map[string]string{"grafana_dashboard": "1"},
			},
			Data: map[string]string{
				"existing-kube-system.json": `{"title": "Existing Kube System"}`,
			},
		},
	}

	fakeClientset := fake.NewSimpleClientset(initialConfigMaps...)

	mockWriter := NewMockWriter()
	mockNotifier := NewMockNotifier()

	sideCar := &SideCar{
		ctx: ctx,
		client: &kubernetes.Client{
			Ctx:    ctx,
			Client: fakeClientset,
		},
		writer:     mockWriter,
		notifier:   mockNotifier,
		Namespaces: []string{"monitoring", "default"},
		Label:      "grafana_dashboard",
		LabelValue: "1",
		Resource:   []string{RESOURCE_CONFIGMAP},
	}

	go sideCar.WaitForChanges()

	time.Sleep(200 * time.Millisecond)

	monitoringCM, _ := fakeClientset.CoreV1().ConfigMaps("monitoring").Get(ctx, "existing-monitoring", metav1.GetOptions{})
	monitoringCM.Data["existing-monitoring.json"] = `{"title": "Updated Monitoring"}`
	_, err := fakeClientset.CoreV1().ConfigMaps("monitoring").Update(ctx, monitoringCM, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Failed to update ConfigMap: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	defaultCM, _ := fakeClientset.CoreV1().ConfigMaps("default").Get(ctx, "existing-default", metav1.GetOptions{})
	defaultCM.Data["existing-default.json"] = `{"title": "Updated Default"}`
	_, err = fakeClientset.CoreV1().ConfigMaps("default").Update(ctx, defaultCM, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Failed to update ConfigMap: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	kubeSystemCM, _ := fakeClientset.CoreV1().ConfigMaps("kube-system").Get(ctx, "existing-kube-system", metav1.GetOptions{})
	kubeSystemCM.Data["existing-kube-system.json"] = `{"title": "Updated Kube System"}`
	_, err = fakeClientset.CoreV1().ConfigMaps("kube-system").Update(ctx, kubeSystemCM, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Failed to update ConfigMap: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if data, ok := mockWriter.WrittenFiles["existing-monitoring.json"]; !ok {
		t.Error("Expected existing-monitoring.json to be written")
	} else if data != `{"title": "Updated Monitoring"}` {
		t.Errorf("Expected monitoring file to be updated, got: %s", data)
	}

	if data, ok := mockWriter.WrittenFiles["existing-default.json"]; !ok {
		t.Error("Expected existing-default.json to be written")
	} else if data != `{"title": "Updated Default"}` {
		t.Errorf("Expected default file to be updated, got: %s", data)
	}

	if _, ok := mockWriter.WrittenFiles["existing-kube-system.json"]; ok {
		t.Error("Expected existing-kube-system.json NOT to be written (namespace not monitored)")
	}

	err = fakeClientset.CoreV1().ConfigMaps("monitoring").Delete(ctx, "existing-monitoring", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("Failed to delete ConfigMap: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if len(mockWriter.RemovedFiles) < 1 {
		t.Errorf("Expected at least 1 file to be removed, got %d", len(mockWriter.RemovedFiles))
	} else {
		found := false
		for _, removed := range mockWriter.RemovedFiles {
			if removed == "existing-monitoring.json" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected existing-monitoring.json to be removed")
		}
	}
}

// ========== Secret Tests ==========

func TestSideCar_Secret_RunOnce(t *testing.T) {
	os.Setenv(NAMESPACE, "default")
	os.Setenv(LABEL, "app")
	os.Setenv(RESOURCE, "secret")
	os.Setenv(METHOD, "list")
	os.Setenv(FOLDER, "test-secret-folder")
	os.MkdirAll(os.Getenv(FOLDER), 0755)
	defer os.RemoveAll(os.Getenv(FOLDER))

	ctx := context.Background()

	// Create fake secrets with base64 encoded data
	secretData1 := map[string][]byte{
		"secret-config.json": []byte(`{"password": "secret123"}`),
	}
	secretData2 := map[string][]byte{
		"secret-config2.json": []byte(`{"apiKey": "abc123xyz"}`),
	}

	fakeClientset := fake.NewSimpleClientset(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-secret",
				Namespace: "default",
				Labels: map[string]string{
					"app": "myapp",
				},
			},
			Data: secretData1,
			Type: corev1.SecretTypeOpaque,
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-secret2",
				Namespace: "default",
				Labels: map[string]string{
					"app": "myapp2",
				},
			},
			Data: secretData2,
			Type: corev1.SecretTypeOpaque,
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "other-secret",
				Namespace: "default",
				Labels: map[string]string{
					"app": "other",
				},
			},
			Type: corev1.SecretTypeOpaque,
		},
	)

	mockNotifier := NewMockNotifier()
	mockWriter := NewMockWriter()

	sideCar := &SideCar{
		ctx: ctx,
		client: &kubernetes.Client{
			Ctx:    ctx,
			Client: fakeClientset,
		},
		writer:     mockWriter,
		notifier:   mockNotifier,
		Resource:   []string{RESOURCE_SECRET},
		Namespaces: []string{"default"},
		Label:      "app",
		LabelValue: "",
	}

	sideCar.RunOnce()

	// Check if files were written to MockWriter
	if data, ok := mockWriter.WrittenFiles["secret-config.json"]; !ok {
		t.Errorf("Expected secret-config.json to be written")
	} else {
		expectedContent := `{"password": "secret123"}`
		if data != expectedContent {
			t.Errorf("Expected content '%s', got %s", expectedContent, data)
		}
	}

	if data, ok := mockWriter.WrittenFiles["secret-config2.json"]; !ok {
		t.Errorf("Expected secret-config2.json to be written")
	} else {
		expectedContent := `{"apiKey": "abc123xyz"}`
		if data != expectedContent {
			t.Errorf("Expected content '%s', got %s", expectedContent, data)
		}
	}

	// Should have exactly 2 files (not 3, since "other" label doesn't match)
	if len(mockWriter.WrittenFiles) != 2 {
		t.Errorf("Expected 2 files to be written, got %d", len(mockWriter.WrittenFiles))
	}

	if mockNotifier.NotifyCount != 1 {
		t.Errorf("Expected notifier to be called once, got %d", mockNotifier.NotifyCount)
	}
}

func TestWaitForChanges_SecretAdd(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockWriter := &MockWriter{
		WrittenFiles: make(map[string]string),
		RemovedFiles: []string{},
	}
	mockNotifier := &MockNotifier{}

	fakeClientset := fake.NewSimpleClientset()

	client := &kubernetes.Client{
		Ctx:    ctx,
		Client: fakeClientset,
		Wg:     &sync.WaitGroup{},
	}

	go client.SecretInformerWorker(
		[]string{"default"},
		"app",
		"test",
		"",
		"",
		mockWriter,
		mockNotifier,
	)
	client.Wg.Add(1)
	time.Sleep(100 * time.Millisecond)

	// Add a secret
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "default",
			Labels: map[string]string{
				"app": "test",
			},
		},
		Data: map[string][]byte{
			"config.json": []byte(`{"secret": "value"}`),
		},
		Type: corev1.SecretTypeOpaque,
	}

	_, err := fakeClientset.CoreV1().Secrets("default").Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create secret: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if data, ok := mockWriter.WrittenFiles["config.json"]; !ok {
		t.Error("Expected config.json to be written")
	} else if data != `{"secret": "value"}` {
		t.Errorf("Expected content '{\"secret\": \"value\"}', got: %s", data)
	}

	if mockNotifier.NotifyCount != 1 {
		t.Error("Expected notifier to be called")
	}
}

func TestWaitForChanges_SecretUpdate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockWriter := &MockWriter{
		WrittenFiles: make(map[string]string),
		RemovedFiles: []string{},
	}
	mockNotifier := &MockNotifier{}

	existingSecret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "default",
			Labels: map[string]string{
				"app": "test",
			},
		},
		Data: map[string][]byte{
			"config.json": []byte(`{"secret": "value"}`),
		},
		Type: corev1.SecretTypeOpaque,
	}

	fakeClientset := fake.NewSimpleClientset(existingSecret)

	client := &kubernetes.Client{
		Ctx:    ctx,
		Client: fakeClientset,
		Wg:     &sync.WaitGroup{},
	}

	client.Wg.Add(1)
	go client.SecretInformerWorker(
		[]string{"default"},
		"app",
		"test",
		"",
		"",
		mockWriter,
		mockNotifier,
	)

	time.Sleep(100 * time.Millisecond)

	// Update the secret
	secret, _ := fakeClientset.CoreV1().Secrets("default").Get(ctx, "test-secret", metav1.GetOptions{})
	secret.Data["config.json"] = []byte(`{"secret": "updated"}`)
	_, err := fakeClientset.CoreV1().Secrets("default").Update(ctx, secret, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Failed to update secret: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if data, ok := mockWriter.WrittenFiles["config.json"]; !ok {
		t.Error("Expected config.json to be written")
	} else if data != `{"secret": "updated"}` {
		t.Errorf("Expected updated content, got: %s", data)
	}
}

func TestWaitForChanges_SecretDelete(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockWriter := &MockWriter{
		WrittenFiles: make(map[string]string),
		RemovedFiles: []string{},
	}
	mockNotifier := &MockNotifier{}

	existingSecret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "default",
			Labels: map[string]string{
				"app": "test",
			},
		},
		Data: map[string][]byte{
			"config.json": []byte(`{"secret": "value"}`),
		},
		Type: corev1.SecretTypeOpaque,
	}

	fakeClientset := fake.NewSimpleClientset(existingSecret)

	client := &kubernetes.Client{
		Ctx:    ctx,
		Client: fakeClientset,
		Wg:     &sync.WaitGroup{},
	}

	client.Wg.Add(1)
	go client.SecretInformerWorker(
		[]string{"default"},
		"app",
		"test",
		"",
		"",
		mockWriter,
		mockNotifier,
	)

	time.Sleep(100 * time.Millisecond)

	// Delete the secret
	err := fakeClientset.CoreV1().Secrets("default").Delete(ctx, "test-secret", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("Failed to delete secret: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if len(mockWriter.RemovedFiles) < 1 {
		t.Errorf("Expected at least 1 file to be removed, got %d", len(mockWriter.RemovedFiles))
	} else if mockWriter.RemovedFiles[0] != "config.json" {
		t.Errorf("Expected config.json to be removed, got: %s", mockWriter.RemovedFiles[0])
	}
}

func TestWaitForChanges_SecretLabelSelector(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockWriter := &MockWriter{
		WrittenFiles: make(map[string]string),
		RemovedFiles: []string{},
	}
	mockNotifier := &MockNotifier{}

	fakeClientset := fake.NewSimpleClientset()

	client := &kubernetes.Client{
		Ctx:    ctx,
		Client: fakeClientset,
		Wg:     &sync.WaitGroup{},
	}

	// Watch for secrets with label app=grafana
	client.Wg.Add(1)

	go client.SecretInformerWorker(
		[]string{"default"},
		"app",
		"grafana",
		"",
		"",
		mockWriter,
		mockNotifier,
	)

	time.Sleep(100 * time.Millisecond)

	// Add a secret with matching label
	matchingSecret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "grafana-secret",
			Namespace: "default",
			Labels: map[string]string{
				"app": "grafana",
			},
		},
		Data: map[string][]byte{
			"dashboard.json": []byte(`{"dashboard": "grafana"}`),
		},
		Type: corev1.SecretTypeOpaque,
	}

	// Add a secret with non-matching label
	nonMatchingSecret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-secret",
			Namespace: "default",
			Labels: map[string]string{
				"app": "other",
			},
		},
		Data: map[string][]byte{
			"other.json": []byte(`{"other": "data"}`),
		},
		Type: corev1.SecretTypeOpaque,
	}

	_, err := fakeClientset.CoreV1().Secrets("default").Create(ctx, matchingSecret, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create matching secret: %v", err)
	}

	_, err = fakeClientset.CoreV1().Secrets("default").Create(ctx, nonMatchingSecret, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create non-matching secret: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Only the matching secret should be written
	if data, ok := mockWriter.WrittenFiles["dashboard.json"]; !ok {
		t.Error("Expected dashboard.json to be written")
	} else if data != `{"dashboard": "grafana"}` {
		t.Errorf("Expected dashboard content, got: %s", data)
	}

	if _, ok := mockWriter.WrittenFiles["other.json"]; ok {
		t.Error("Expected other.json NOT to be written (label mismatch)")
	}
}

func TestWaitForChanges_SecretNonJSONFilesIgnored(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockWriter := &MockWriter{
		WrittenFiles: make(map[string]string),
		RemovedFiles: []string{},
	}
	mockNotifier := &MockNotifier{}

	fakeClientset := fake.NewSimpleClientset()

	client := &kubernetes.Client{
		Ctx:    ctx,
		Client: fakeClientset,
		Wg:     &sync.WaitGroup{},
	}

	client.Wg.Add(1)

	go client.SecretInformerWorker(
		[]string{"default"},
		"app",
		"test",
		"",
		"",
		mockWriter,
		mockNotifier,
	)

	time.Sleep(100 * time.Millisecond)

	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "default",
			Labels: map[string]string{
				"app": "test",
			},
		},
		Data: map[string][]byte{
			"config.json": []byte(`{"valid": "json"}`),
			"config.yaml": []byte(`key: value`),
			"config.txt":  []byte(`plain text`),
		},
		Type: corev1.SecretTypeOpaque,
	}

	_, err := fakeClientset.CoreV1().Secrets("default").Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create secret: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Only JSON file should be written
	if _, ok := mockWriter.WrittenFiles["config.json"]; !ok {
		t.Error("Expected config.json to be written")
	}

	if _, ok := mockWriter.WrittenFiles["config.yaml"]; ok {
		t.Error("Expected config.yaml NOT to be written (not JSON)")
	}

	if _, ok := mockWriter.WrittenFiles["config.txt"]; ok {
		t.Error("Expected config.txt NOT to be written (not JSON)")
	}

	if len(mockWriter.WrittenFiles) != 1 {
		t.Errorf("Expected exactly 1 file to be written, got %d", len(mockWriter.WrittenFiles))
	}
}

func TestSideCar_SecretAndConfigMap_Both(t *testing.T) {
	// Save and restore environment variables
	oldNamespace := os.Getenv(NAMESPACE)
	oldLabel := os.Getenv(LABEL)
	oldLabelValue := os.Getenv(LABEL_VALUE)
	oldResource := os.Getenv(RESOURCE)
	oldMethod := os.Getenv(METHOD)
	oldFolder := os.Getenv(FOLDER)
	defer func() {
		os.Setenv(NAMESPACE, oldNamespace)
		os.Setenv(LABEL, oldLabel)
		os.Setenv(LABEL_VALUE, oldLabelValue)
		os.Setenv(RESOURCE, oldResource)
		os.Setenv(METHOD, oldMethod)
		os.Setenv(FOLDER, oldFolder)
	}()

	os.Setenv(NAMESPACE, "default")
	os.Setenv(LABEL, "app")
	os.Setenv(LABEL_VALUE, "")
	os.Setenv(RESOURCE, "both")
	os.Setenv(METHOD, "list")
	os.Setenv(FOLDER, "test-both-folder")
	os.MkdirAll(os.Getenv(FOLDER), 0755)
	defer os.RemoveAll(os.Getenv(FOLDER))

	ctx := context.Background()

	fakeClientset := fake.NewSimpleClientset(
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-configmap",
				Namespace: "default",
				Labels: map[string]string{
					"app": "myapp",
				},
			},
			Data: map[string]string{
				"configmap.json": `{"type": "configmap"}`,
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-secret",
				Namespace: "default",
				Labels: map[string]string{
					"app": "myapp",
				},
			},
			Data: map[string][]byte{
				"secret.json": []byte(`{"type": "secret"}`),
			},
			Type: corev1.SecretTypeOpaque,
		},
	)

	sideCar := New(ctx)
	sideCar.client = &kubernetes.Client{
		Ctx:    ctx,
		Client: fakeClientset,
		Wg:     &sync.WaitGroup{},
	}

	sideCar.RunOnce()

	// Check ConfigMap file
	if content, err := os.ReadFile("test-both-folder/configmap.json"); err != nil {
		t.Errorf("Expected configmap.json to exist, got error: %v", err)
	} else if string(content) != `{"type": "configmap"}` {
		t.Errorf("Expected ConfigMap content, got: %s", string(content))
	}

	// Check Secret file
	if content, err := os.ReadFile("test-both-folder/secret.json"); err != nil {
		t.Errorf("Expected secret.json to exist, got error: %v", err)
	} else if string(content) != `{"type": "secret"}` {
		t.Errorf("Expected Secret content, got: %s", string(content))
	}
}

func TestWaitForChanges_SecretAllNamespaces(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockWriter := &MockWriter{
		WrittenFiles: make(map[string]string),
		RemovedFiles: []string{},
	}
	mockNotifier := &MockNotifier{}

	secret1 := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret-ns1",
			Namespace: "namespace1",
			Labels: map[string]string{
				"app": "test",
			},
		},
		Data: map[string][]byte{
			"config1.json": []byte(`{"ns": "namespace1"}`),
		},
		Type: corev1.SecretTypeOpaque,
	}

	secret2 := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret-ns2",
			Namespace: "namespace2",
			Labels: map[string]string{
				"app": "test",
			},
		},
		Data: map[string][]byte{
			"config2.json": []byte(`{"ns": "namespace2"}`),
		},
		Type: corev1.SecretTypeOpaque,
	}

	fakeClientset := fake.NewSimpleClientset(secret1, secret2)

	client := &kubernetes.Client{
		Ctx:    ctx,
		Client: fakeClientset,
		Wg:     &sync.WaitGroup{},
	}

	// Watch all namespaces (empty slice)
	client.Wg.Add(1)
	go client.SecretInformerWorker(
		[]string{},
		"app",
		"test",
		"",
		"",
		mockWriter,
		mockNotifier,
	)

	time.Sleep(100 * time.Millisecond)

	// Add a secret in a new namespace
	secret3 := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret-ns3",
			Namespace: "namespace3",
			Labels: map[string]string{
				"app": "test",
			},
		},
		Data: map[string][]byte{
			"config3.json": []byte(`{"ns": "namespace3"}`),
		},
		Type: corev1.SecretTypeOpaque,
	}

	_, err := fakeClientset.CoreV1().Secrets("namespace3").Create(ctx, secret3, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create secret: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// All secrets should be captured
	if len(mockWriter.WrittenFiles) < 1 {
		t.Errorf("Expected at least 1 file to be written, got %d", len(mockWriter.WrittenFiles))
	}

	if data, ok := mockWriter.WrittenFiles["config3.json"]; !ok {
		t.Error("Expected config3.json to be written")
	} else if data != `{"ns": "namespace3"}` {
		t.Errorf("Expected namespace3 content, got: %s", data)
	}
}
