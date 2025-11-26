# k8s-gsidecar

A lightweight Kubernetes Sidecar container that monitors ConfigMap and Secret changes, synchronizes content to the local filesystem, and supports HTTP notification mechanisms.

## Features

- 🔄 **Real-time Monitoring**: Supports Watch and List modes for monitoring Kubernetes ConfigMaps and Secrets
- 📁 **Automatic Synchronization**: Automatically syncs JSON files from ConfigMaps/Secrets to local directories
- 🔔 **Notification Mechanism**: Supports HTTP notifications to trigger external services on resource changes
- 🔐 **Authentication Support**: Supports HTTP Basic Authentication
- 🎯 **Flexible Filtering**: Supports filtering resources by Labels and Namespaces
- 🚀 **Multiple Run Modes**: Supports Watch, List, and one-time execution modes

## Run Modes

### Watch Mode (Default)
Uses the Kubernetes Informer mechanism to monitor resource changes in real-time. Automatically syncs when ConfigMaps/Secrets are added, modified, or deleted.

### List Mode
Periodically lists and syncs resources that match the specified criteria.

### Sleep/Once Mode
Performs a one-time sync and exits.

## Environment Variables Configuration

### Basic Configuration

| Environment Variable | Description | Default | Required |
|---------------------|-------------|---------|----------|
| `METHOD` | Run mode: `watch`/`list`/`sleep` | - | ✓ |
| `NAMESPACE` | Namespaces to monitor, comma-separated, `ALL` for all namespaces | `ALL` | ✓ |
| `FOLDER` | Target folder for synced files | - | ✓ |
| `LABEL` | Label key for filtering | - | ✓ |
| `LABEL_VALUE` | Label value (optional) | - | ✗ |
| `RESOURCE` | Resource type: `configmap`/`secret`/`both` | - | ✓ |

### Notification Configuration

| Environment Variable | Description | Default | Required |
|---------------------|-------------|---------|----------|
| `REQ_URL` | HTTP URL for notifications | - | ✗ |
| `REQ_METHOD` | HTTP method: `GET`/`POST` | `GET` | ✗ |
| `REQ_PAYLOAD` | Payload for POST requests | - | ✗ |
| `REQ_USERNAME` | HTTP Basic Auth username | - | ✗ |
| `REQ_PASSWORD` | HTTP Basic Auth password | - | ✗ |
| `REQ_SKIP_INIT` | Skip initial notification | `false` | ✗ |

### Advanced Configuration

| Environment Variable | Description | Default | Required |
|---------------------|-------------|---------|----------|
| `UNIQUE_FILENAMES` | Ensure unique filenames | `false` | ✗ |
| `FOLDER_ANNOTATION` | Read target folder from annotation | - | ✗ |
| `RESOURCE_NAME` | Specific resource name (not implemented) | - | ✗ |
| `SCRIPT` | Custom script (not implemented) | - | ✗ |
| `ENABLE_5XX` | Enable 5XX retry (not implemented) | `false` | ✗ |
| `IGNORE_ALREADY_PROCESSED` | Ignore already processed resources (not implemented) | `false` | ✗ |

## Usage Examples

### 1. Watch Mode for ConfigMap Monitoring

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: app-with-sidecar
spec:
  containers:
  - name: app
    image: your-app:latest
    volumeMounts:
    - name: config
      mountPath: /config
  
  - name: k8s-gsidecar
    image: k8s-gsidecar:latest
    env:
    - name: METHOD
      value: "watch"
    - name: NAMESPACE
      value: "default"
    - name: LABEL
      value: "app"
    - name: LABEL_VALUE
      value: "myapp"
    - name: RESOURCE
      value: "configmap"
    - name: FOLDER
      value: "/config"
    - name: REQ_URL
      value: "http://localhost:8080/reload"
    - name: REQ_METHOD
      value: "POST"
    volumeMounts:
    - name: config
      mountPath: /config
  
  volumes:
  - name: config
    emptyDir: {}
```

### 2. Multi-Namespace Monitoring

```bash
export METHOD=watch
export NAMESPACE=default,production,staging
export LABEL=config
export RESOURCE=configmap
export FOLDER=/app/config
```

### 3. With HTTP Notification and Authentication

```bash
export METHOD=watch
export NAMESPACE=default
export LABEL=app-config
export RESOURCE=configmap
export FOLDER=/config
export REQ_URL=https://api.example.com/webhook
export REQ_METHOD=POST
export REQ_PAYLOAD='{"event":"config-updated"}'
export REQ_USERNAME=admin
export REQ_PASSWORD=secret123
```

### 4. One-time Sync

```bash
export METHOD=sleep
export NAMESPACE=default
export LABEL=init-config
export RESOURCE=configmap
export FOLDER=/init-config
```

## How It Works

### Watch Mode Flow

```
1. Start Kubernetes Informer
   ↓
2. Listen for ConfigMap/Secret changes
   ↓
3. Event triggered (Add/Update/Delete)
   ↓
4. Filter resources matching Label criteria
   ↓
5. Process only .json file extensions
   ↓
6. Write/Delete local files
   ↓
7. Trigger HTTP notification (if configured)
```

### File Filtering Rules

- Only syncs files with `.json` extension
- Each key in ConfigMap's Data field becomes a filename
- Files are written to the directory specified by `FOLDER`

## RBAC Permissions Required

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: k8s-gsidecar
  namespace: default
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: k8s-gsidecar
rules:
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: k8s-gsidecar
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: k8s-gsidecar
subjects:
- kind: ServiceAccount
  name: k8s-gsidecar
  namespace: default
```

## Build and Deployment

### Local Build

```bash
# Build binary
go build -o k8s-gsidecar .

# Run
./k8s-gsidecar
```

### Docker Build

```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o k8s-gsidecar .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/k8s-gsidecar .
CMD ["./k8s-gsidecar"]
```

```bash
# Build image
docker build -t k8s-gsidecar:latest .

# Push to registry
docker push your-registry/k8s-gsidecar:latest
```

## Testing

```bash
# Run tests
go test ./...

# Run specific tests
go test ./sidecar_test.go
```

## Architecture Design

```
┌─────────────────────────────────────────┐
│           SideCar Main                  │
│  ┌─────────────────────────────────┐   │
│  │   Kubernetes Client             │   │
│  │  - InCluster / Kubeconfig       │   │
│  │  - Informer/Watcher             │   │
│  └─────────────────────────────────┘   │
│              ↓                          │
│  ┌─────────────────────────────────┐   │
│  │   Resource Filtering            │   │
│  │  - Namespace                    │   │
│  │  - Label Selector               │   │
│  │  - Resource Type                │   │
│  └─────────────────────────────────┘   │
│              ↓                          │
│  ┌─────────────────────────────────┐   │
│  │   Writer Interface              │   │
│  │  - FileWriter                   │   │
│  │  - JSON Filter                  │   │
│  └─────────────────────────────────┘   │
│              ↓                          │
│  ┌─────────────────────────────────┐   │
│  │   Notifier Interface            │   │
│  │  - HTTPNotifier                 │   │
│  │  - Basic Auth                   │   │
│  └─────────────────────────────────┘   │
└─────────────────────────────────────────┘
```

## FAQ

### Q1: Why only sync JSON files?
A: The current version only processes files with `.json` extension, as defined in the `writer.IsJSON()` method. Support for other file types can be extended in the future.

### Q2: How to monitor all Namespaces?
A: Set `NAMESPACE=ALL` or leave it empty.

### Q3: Is Secret support complete?
A: Secret support is not fully implemented yet. ConfigMap is recommended for now.

### Q4: What happens if notification fails?
A: Currently, notification failures are logged but do not interrupt the file sync process.

### Q5: Does it support both In-Cluster and Out-of-Cluster modes?
A: Yes, the program automatically detects the environment. It uses ServiceAccount when running inside a cluster and kubeconfig when running outside.

## License

This project is licensed under the MIT License.

## Contributing

Issues and Pull Requests are welcome!

## Roadmap

- [ ] Full Secret resource support
- [ ] Support more file formats (YAML, TXT, etc.)
- [ ] Implement Script execution feature
- [ ] Implement 5XX retry mechanism
- [ ] Support Prometheus Metrics
- [ ] Add more notification methods (Slack, Email, etc.)
- [ ] Implement UNIQUE_FILENAMES feature
- [ ] Support reading target folder from Annotations
