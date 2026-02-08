package container

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/util/homedir"
)

type Manager struct {
	clientset      *kubernetes.Clientset
	restConfig     *rest.Config
	namespace      string
	dockerImage    string
	dockerRegistry string
}

// NewManager creates a new container manager
func NewManager(kubeconfigPath, namespace, dockerRegistry, dockerImage string) (*Manager, error) {
	// Use default kubeconfig path if not provided
	if kubeconfigPath == "" {
		if home := homedir.HomeDir(); home != "" {
			kubeconfigPath = filepath.Join(home, ".kube", "config")
		}
	}

	// Build config from kubeconfig file
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build config: %w", err)
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	if namespace == "" {
		namespace = "default"
	}

	return &Manager{
		clientset:      clientset,
		restConfig:     config,
		namespace:      namespace,
		dockerImage:    dockerImage,
		dockerRegistry: dockerRegistry,
	}, nil
}

// CreatePod creates a new pod for the user
// agentConfig is optional - if provided, it will be used to configure ANTHROPIC environment variables
func (m *Manager) CreatePod(ctx context.Context, userID, sessionID string, agentConfig map[string]interface{}) (*corev1.Pod, error) {
	podName := fmt.Sprintf("claude-code-%s-%s", userID, sessionID)

	imageFullPath := fmt.Sprintf("%s/%s", m.dockerRegistry, m.dockerImage)

	// Build environment variables
	envVars := []corev1.EnvVar{
		{
			Name:  "USER_ID",
			Value: userID,
		},
		{
			Name:  "SESSION_ID",
			Value: sessionID,
		},
	}

	// Add ANTHROPIC configuration from agent config
	if agentConfig != nil {
		if authToken, ok := agentConfig["anthropic_auth_token"].(string); ok && authToken != "" {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "ANTHROPIC_AUTH_TOKEN",
				Value: authToken,
			})
		}
		if baseURL, ok := agentConfig["anthropic_base_url"].(string); ok && baseURL != "" {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "ANTHROPIC_BASE_URL",
				Value: baseURL,
			})
		}
		if haikuModel, ok := agentConfig["anthropic_haiku_model"].(string); ok && haikuModel != "" {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "ANTHROPIC_DEFAULT_HAIKU_MODEL",
				Value: haikuModel,
			})
		}
		if opusModel, ok := agentConfig["anthropic_opus_model"].(string); ok && opusModel != "" {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "ANTHROPIC_DEFAULT_OPUS_MODEL",
				Value: opusModel,
			})
		}
		if sonnetModel, ok := agentConfig["anthropic_sonnet_model"].(string); ok && sonnetModel != "" {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "ANTHROPIC_DEFAULT_SONNET_MODEL",
				Value: sonnetModel,
			})
		}
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: m.namespace,
			Labels: map[string]string{
				"app":        "claude-code",
				"user-id":    userID,
				"session-id": sessionID,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "claude-code",
					Image: imageFullPath,
					Ports: []corev1.ContainerPort{
						{
							Name:          "ttyd",
							ContainerPort: 7681,
							Protocol:      corev1.ProtocolTCP,
						},
					},
					Env: envVars,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("2"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("2"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "workspace",
							MountPath: "/workspace",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "workspace",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},
		},
	}

	createdPod, err := m.clientset.CoreV1().Pods(m.namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create pod: %w", err)
	}

	log.Printf("Pod %s created successfully", podName)
	return createdPod, nil
}

// GetPodIP retrieves the IP address of a pod
func (m *Manager) GetPodIP(ctx context.Context, userID, sessionID string) (string, error) {
	podName := fmt.Sprintf("claude-code-%s-%s", userID, sessionID)

	pod, err := m.clientset.CoreV1().Pods(m.namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get pod: %w", err)
	}

	if pod.Status.PodIP == "" {
		return "", fmt.Errorf("pod IP not yet assigned")
	}

	return pod.Status.PodIP, nil
}

// DeletePod deletes a pod
func (m *Manager) DeletePod(ctx context.Context, userID, sessionID string) error {
	podName := fmt.Sprintf("claude-code-%s-%s", userID, sessionID)

	err := m.clientset.CoreV1().Pods(m.namespace).Delete(ctx, podName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete pod: %w", err)
	}

	log.Printf("Pod %s deleted successfully", podName)
	return nil
}

// GetPodStatus retrieves the status of a pod
func (m *Manager) GetPodStatus(ctx context.Context, userID, sessionID string) (corev1.PodPhase, error) {
	podName := fmt.Sprintf("claude-code-%s-%s", userID, sessionID)

	pod, err := m.clientset.CoreV1().Pods(m.namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get pod: %w", err)
	}

	return pod.Status.Phase, nil
}

// ListPods lists all claude-code pods
func (m *Manager) ListPods(ctx context.Context) (*corev1.PodList, error) {
	pods, err := m.clientset.CoreV1().Pods(m.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=claude-code",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	return pods, nil
}

// CreatePVC creates a PersistentVolumeClaim for a user
func (m *Manager) CreatePVC(ctx context.Context, userID, sessionID string) error {
	pvcName := fmt.Sprintf("pvc-%s-%s", userID, sessionID)

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: m.namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("10Gi"),
				},
			},
		},
	}

	_, err := m.clientset.CoreV1().PersistentVolumeClaims(m.namespace).Create(ctx, pvc, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create pvc: %w", err)
	}

	log.Printf("PVC %s created successfully", pvcName)
	return nil
}

// DeletePodByName deletes a specific pod by name, used to restart StatefulSet pods
func (m *Manager) DeletePodByName(ctx context.Context, podName string) error {
	err := m.clientset.CoreV1().Pods(m.namespace).Delete(ctx, podName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete pod %s: %w", podName, err)
	}
	log.Printf("Pod %s deleted (will be recreated by StatefulSet)", podName)
	return nil
}

// statefulSetName returns the consistent name for a user-agent StatefulSet
func (m *Manager) statefulSetName(userID string, agentID int64) string {
	return fmt.Sprintf("claude-code-%s-%d", userID, agentID)
}

// buildAgentEnvVars builds environment variables from agent configuration
func (m *Manager) buildAgentEnvVars(userID string, agentID int64, agentConfig map[string]interface{}) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{Name: "USER_ID", Value: userID},
		{Name: "AGENT_ID", Value: fmt.Sprintf("%d", agentID)},
	}

	if agentConfig == nil {
		return envVars
	}

	configMap := map[string]string{
		"anthropic_auth_token":   "ANTHROPIC_AUTH_TOKEN",
		"anthropic_base_url":     "ANTHROPIC_BASE_URL",
		"anthropic_haiku_model":  "ANTHROPIC_DEFAULT_HAIKU_MODEL",
		"anthropic_opus_model":   "ANTHROPIC_DEFAULT_OPUS_MODEL",
		"anthropic_sonnet_model": "ANTHROPIC_DEFAULT_SONNET_MODEL",
	}

	for jsonKey, envKey := range configMap {
		if val, ok := agentConfig[jsonKey].(string); ok && val != "" {
			envVars = append(envVars, corev1.EnvVar{Name: envKey, Value: val})
		}
	}

	return envVars
}

// CreateStatefulSet creates a per-agent StatefulSet with a headless service.
// The headless service is required for StatefulSet DNS to work.
// Pod DNS will be: claude-code-{userID}-{agentID}-0.claude-code-{userID}-{agentID}.{namespace}.svc.cluster.local
func (m *Manager) CreateStatefulSet(ctx context.Context, userID string, agentID int64, agentConfig map[string]interface{}) error {
	name := m.statefulSetName(userID, agentID)
	imageFullPath := fmt.Sprintf("%s/%s", m.dockerRegistry, m.dockerImage)
	envVars := m.buildAgentEnvVars(userID, agentID, agentConfig)

	labels := map[string]string{
		"app":      "claude-code",
		"user-id":  userID,
		"agent-id": fmt.Sprintf("%d", agentID),
	}

	// Step 1: Create headless service (ClusterIP: None) — required for StatefulSet
	headlessSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: m.namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "None",
			Selector:  labels,
			Ports: []corev1.ServicePort{
				{
					Name:     "ttyd",
					Port:     7681,
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
	}

	_, err := m.clientset.CoreV1().Services(m.namespace).Create(ctx, headlessSvc, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create headless service: %w", err)
	}
	log.Printf("Headless Service %s created successfully", name)

	// Step 2: Create StatefulSet
	replicas := int32(1)
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: m.namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: name, // must match headless service name
			Replicas:    &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "claude-code",
							Image: imageFullPath,
							Ports: []corev1.ContainerPort{
								{
									Name:          "ttyd",
									ContainerPort: 7681,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env: envVars,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("4Gi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("4Gi"),
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "workspace",
									MountPath: "/workspace",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "workspace",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}

	_, err = m.clientset.AppsV1().StatefulSets(m.namespace).Create(ctx, sts, metav1.CreateOptions{})
	if err != nil {
		// Cleanup headless service if StatefulSet creation fails
		_ = m.clientset.CoreV1().Services(m.namespace).Delete(ctx, name, metav1.DeleteOptions{})
		return fmt.Errorf("failed to create statefulset: %w", err)
	}

	log.Printf("StatefulSet %s created successfully", name)
	return nil
}

// GetStatefulSet gets the StatefulSet for a specific user and agent
func (m *Manager) GetStatefulSet(ctx context.Context, userID string, agentID int64) (*appsv1.StatefulSet, error) {
	name := m.statefulSetName(userID, agentID)

	sts, err := m.clientset.AppsV1().StatefulSets(m.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get statefulset: %w", err)
	}

	return sts, nil
}

// DeleteStatefulSet deletes the StatefulSet and its headless service
func (m *Manager) DeleteStatefulSet(ctx context.Context, userID string, agentID int64) error {
	name := m.statefulSetName(userID, agentID)

	// Delete StatefulSet first
	err := m.clientset.AppsV1().StatefulSets(m.namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete statefulset: %w", err)
	}
	log.Printf("StatefulSet %s deleted successfully", name)

	// Delete headless service
	err = m.clientset.CoreV1().Services(m.namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		log.Printf("Warning: failed to delete headless service %s: %v", name, err)
		// Not fatal — StatefulSet is already deleted
	} else {
		log.Printf("Headless Service %s deleted successfully", name)
	}

	return nil
}

// GetStatefulSetPodIP returns the Pod IP of the StatefulSet's replica-0 pod.
// StatefulSet pods have stable IPs that persist across restarts.
func (m *Manager) GetStatefulSetPodIP(ctx context.Context, userID string, agentID int64) (string, error) {
	name := m.statefulSetName(userID, agentID)
	podName := fmt.Sprintf("%s-0", name)

	pod, err := m.clientset.CoreV1().Pods(m.namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get pod %s: %w", podName, err)
	}

	if pod.Status.PodIP == "" {
		return "", fmt.Errorf("pod %s has no IP assigned yet", podName)
	}

	return pod.Status.PodIP, nil
}

// PodInfo contains detailed information about a StatefulSet's pod.
type PodInfo struct {
	Status        string `json:"status"`
	RestartCount  int32  `json:"restart_count"`
	CPURequest    string `json:"cpu_request"`
	CPULimit      string `json:"cpu_limit"`
	MemoryRequest string `json:"memory_request"`
	MemoryLimit   string `json:"memory_limit"`
}

// GetStatefulSetPodInfo returns detailed info for a StatefulSet's pod.
// Returns PodInfo with Status "NotDeployed" if the pod doesn't exist,
// "Error" for CrashLoopBackOff/ImagePullBackOff, or the pod phase string.
func (m *Manager) GetStatefulSetPodInfo(ctx context.Context, userID string, agentID int64) PodInfo {
	name := m.statefulSetName(userID, agentID)
	podName := fmt.Sprintf("%s-0", name)

	pod, err := m.clientset.CoreV1().Pods(m.namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return PodInfo{Status: "NotDeployed"}
		}
		return PodInfo{Status: "Unknown"}
	}

	info := PodInfo{
		Status: string(pod.Status.Phase),
	}

	// Read resource requests/limits from the first container
	if len(pod.Spec.Containers) > 0 {
		res := pod.Spec.Containers[0].Resources
		if q, ok := res.Requests[corev1.ResourceCPU]; ok {
			info.CPURequest = q.String()
		}
		if q, ok := res.Requests[corev1.ResourceMemory]; ok {
			info.MemoryRequest = q.String()
		}
		if q, ok := res.Limits[corev1.ResourceCPU]; ok {
			info.CPULimit = q.String()
		}
		if q, ok := res.Limits[corev1.ResourceMemory]; ok {
			info.MemoryLimit = q.String()
		}
	}

	// Check container statuses for error conditions and restart count
	for _, cs := range pod.Status.ContainerStatuses {
		info.RestartCount = cs.RestartCount
		if cs.State.Waiting != nil {
			reason := cs.State.Waiting.Reason
			if reason == "CrashLoopBackOff" || reason == "ImagePullBackOff" || reason == "ErrImagePull" {
				info.Status = "Error"
			}
		}
	}

	return info
}

// ExecInPod executes a command inside a pod using SPDY remotecommand.
func (m *Manager) ExecInPod(ctx context.Context, podName string, command []string, stdin io.Reader) (string, string, error) {
	req := m.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(m.namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "claude-code",
			Command:   command,
			Stdin:     stdin != nil,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(m.restConfig, "POST", req.URL())
	if err != nil {
		return "", "", fmt.Errorf("failed to create executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	streamOpts := remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	if stdin != nil {
		streamOpts.Stdin = stdin
	}

	err = exec.StreamWithContext(ctx, streamOpts)
	return stdout.String(), stderr.String(), err
}

// WriteFileInPod writes content to a file inside a pod.
func (m *Manager) WriteFileInPod(ctx context.Context, podName, filePath, content string) error {
	cmd := []string{"bash", "-c", fmt.Sprintf("mkdir -p $(dirname %s) && cat > %s", filePath, filePath)}
	_, stderr, err := m.ExecInPod(ctx, podName, cmd, strings.NewReader(content))
	if err != nil {
		return fmt.Errorf("failed to write file %s in pod %s: %w (stderr: %s)", filePath, podName, err, stderr)
	}
	return nil
}

// DeleteFileInPod deletes a file from a pod.
func (m *Manager) DeleteFileInPod(ctx context.Context, podName, filePath string) error {
	cmd := []string{"rm", "-f", filePath}
	_, stderr, err := m.ExecInPod(ctx, podName, cmd, nil)
	if err != nil {
		return fmt.Errorf("failed to delete file %s in pod %s: %w (stderr: %s)", filePath, podName, err, stderr)
	}
	return nil
}

// ListFilesInPod lists files in a directory inside a pod.
func (m *Manager) ListFilesInPod(ctx context.Context, podName, dirPath string) ([]string, error) {
	cmd := []string{"bash", "-c", fmt.Sprintf("ls -1 %s 2>/dev/null || true", dirPath)}
	stdout, _, err := m.ExecInPod(ctx, podName, cmd, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list files in %s: %w", dirPath, err)
	}

	var files []string
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// WaitForStatefulSetReady polls until the StatefulSet pod is Running.
func (m *Manager) WaitForStatefulSetReady(ctx context.Context, userID string, agentID int64, maxRetries int, retryInterval time.Duration) error {
	name := m.statefulSetName(userID, agentID)
	podName := fmt.Sprintf("%s-0", name)

	for i := 0; i < maxRetries; i++ {
		pod, err := m.clientset.CoreV1().Pods(m.namespace).Get(ctx, podName, metav1.GetOptions{})
		if err == nil && pod.Status.Phase == corev1.PodRunning {
			log.Printf("StatefulSet pod %s is ready", podName)
			return nil
		}

		if i < maxRetries-1 {
			time.Sleep(retryInterval)
		}
	}

	return fmt.Errorf("timeout waiting for statefulset pod %s to be ready", podName)
}
