package container

import (
	"bytes"
	"context"
	"fmt"
	"github.com/rs/zerolog/log"
	"io"
	"path/filepath"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
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
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

type Manager struct {
	clientset      *kubernetes.Clientset
	metricsClient  metricsv.Interface
	restConfig     *rest.Config
	namespace      string
	dockerImage    string
	dockerRegistry string
	sidecarImage   string
}

// NewManager creates a new container manager
func NewManager(kubeconfigPath, namespace, dockerRegistry, dockerImage string, extraOpts ...string) (*Manager, error) {
	var config *rest.Config
	var err error

	// Try in-cluster config first (when running inside K8s)
	config, err = rest.InClusterConfig()
	if err != nil {
		// Fall back to kubeconfig file (local development)
		if kubeconfigPath == "" {
			if home := homedir.HomeDir(); home != "" {
				kubeconfigPath = filepath.Join(home, ".kube", "config")
			}
		}
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to build config: %w", err)
		}
		log.Info().Str("path", kubeconfigPath).Msg("using kubeconfig")
	} else {
		log.Info().Msg("using in-cluster Kubernetes config")
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	// Create metrics client (best-effort, nil if unavailable)
	mc, mcErr := metricsv.NewForConfig(config)
	if mcErr != nil {
		log.Warn().Err(mcErr).Msg("metrics client unavailable, pod usage will not be reported")
		mc = nil
	}

	if namespace == "" {
		namespace = "default"
	}

	sidecarImage := ""
	if len(extraOpts) > 0 {
		sidecarImage = extraOpts[0]
	}

	return &Manager{
		clientset:      clientset,
		metricsClient:  mc,
		restConfig:     config,
		namespace:      namespace,
		dockerImage:    dockerImage,
		dockerRegistry: dockerRegistry,
		sidecarImage:   sidecarImage,
	}, nil
}

// GetClientset returns the Kubernetes clientset for direct API access.
func (m *Manager) GetClientset() *kubernetes.Clientset {
	return m.clientset
}

// GetNamespace returns the configured namespace.
func (m *Manager) GetNamespace() string {
	return m.namespace
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

	log.Debug().Str("pod", podName).Msg("pod created")
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

	log.Debug().Str("pod", podName).Msg("pod deleted")
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

	log.Debug().Str("pvc", pvcName).Msg("PVC created")
	return nil
}

// DeletePodByName deletes a specific pod by name, used to restart StatefulSet pods
func (m *Manager) DeletePodByName(ctx context.Context, podName string) error {
	grace := int64(0)
	err := m.clientset.CoreV1().Pods(m.namespace).Delete(ctx, podName, metav1.DeleteOptions{
		GracePeriodSeconds: &grace,
	})
	if err != nil {
		return fmt.Errorf("failed to delete pod %s: %w", podName, err)
	}
	log.Debug().Str("pod", podName).Msg("pod force-deleted (will be recreated by StatefulSet)")
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
		"http_proxy":             "HTTP_PROXY",
		"https_proxy":            "HTTPS_PROXY",
	}

	for jsonKey, envKey := range configMap {
		if val, ok := agentConfig[jsonKey].(string); ok && val != "" {
			envVars = append(envVars, corev1.EnvVar{Name: envKey, Value: val})
		}
	}

	return envVars
}

// ResourceConfig holds CPU/memory resource settings for pod creation.
type ResourceConfig struct {
	CPURequest    string
	CPULimit      string
	MemoryRequest string
	MemoryLimit   string
}

// hooksConfigMapName returns the name of the shared ConfigMap for Claude Code hooks.
const hooksConfigMapName = "sac-claude-hooks"

// CreateStatefulSet creates a per-agent StatefulSet with a headless service.
// The headless service is required for StatefulSet DNS to work.
// Pod DNS will be: claude-code-{userID}-{agentID}-0.claude-code-{userID}-{agentID}.{namespace}.svc.cluster.local
func (m *Manager) CreateStatefulSet(ctx context.Context, userID string, agentID int64, agentConfig map[string]interface{}, rc *ResourceConfig, imageOverride string) error {
	name := m.statefulSetName(userID, agentID)
	imageFullPath := imageOverride
	if imageFullPath == "" {
		imageFullPath = fmt.Sprintf("%s/%s", m.dockerRegistry, m.dockerImage)
	}
	envVars := m.buildAgentEnvVars(userID, agentID, agentConfig)

	// Add SAC_API_URL env var for hook scripts
	envVars = append(envVars, corev1.EnvVar{
		Name:  "SAC_API_URL",
		Value: "http://api-gateway.sac.svc.cluster.local:8080",
	})

	// Use provided resource config or defaults (trim whitespace to avoid parse errors)
	cpuReq, cpuLim, memReq, memLim := "2", "2", "4Gi", "4Gi"
	if rc != nil {
		if v := strings.TrimSpace(rc.CPURequest); v != "" {
			cpuReq = v
		}
		if v := strings.TrimSpace(rc.CPULimit); v != "" {
			cpuLim = v
		}
		if v := strings.TrimSpace(rc.MemoryRequest); v != "" {
			memReq = v
		}
		if v := strings.TrimSpace(rc.MemoryLimit); v != "" {
			memLim = v
		}
	}

	// Parse resource quantities (non-panicking)
	parsedCPUReq, err := resource.ParseQuantity(cpuReq)
	if err != nil {
		return fmt.Errorf("invalid cpu_request %q: %w", cpuReq, err)
	}
	parsedCPULim, err2 := resource.ParseQuantity(cpuLim)
	if err2 != nil {
		return fmt.Errorf("invalid cpu_limit %q: %w", cpuLim, err2)
	}
	parsedMemReq, err3 := resource.ParseQuantity(memReq)
	if err3 != nil {
		return fmt.Errorf("invalid memory_request %q: %w", memReq, err3)
	}
	parsedMemLim, err4 := resource.ParseQuantity(memLim)
	if err4 != nil {
		return fmt.Errorf("invalid memory_limit %q: %w", memLim, err4)
	}

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

	_, svcErr := m.clientset.CoreV1().Services(m.namespace).Create(ctx, headlessSvc, metav1.CreateOptions{})
	if svcErr != nil {
		if apierrors.IsAlreadyExists(svcErr) {
			log.Debug().Str("service", name).Msg("headless service already exists, reusing")
		} else {
			return fmt.Errorf("failed to create headless service: %w", svcErr)
		}
	} else {
		log.Debug().Str("service", name).Msg("headless service created")
	}

	// Build sidecar container (output-watcher)
	var sidecarContainers []corev1.Container
	if m.sidecarImage != "" {
		sidecarImageFull := fmt.Sprintf("%s/%s", m.dockerRegistry, m.sidecarImage)
		sidecarContainers = append(sidecarContainers, corev1.Container{
			Name:  "output-watcher",
			Image: sidecarImageFull,
			Env: []corev1.EnvVar{
				{Name: "USER_ID", Value: userID},
				{Name: "AGENT_ID", Value: fmt.Sprintf("%d", agentID)},
				{Name: "SAC_API_URL", Value: "http://api-gateway.sac.svc.cluster.local:8080"},
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("50m"),
					corev1.ResourceMemory: resource.MustParse("32Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("64Mi"),
				},
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "workspace",
					MountPath: "/workspace",
				},
			},
		})
	}

	// Step 2: Create StatefulSet with hooks volume mounts
	replicas := int32(1)
	defaultMode := int32(0755)
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
					Containers: append([]corev1.Container{
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
									corev1.ResourceCPU:    parsedCPUReq,
									corev1.ResourceMemory: parsedMemReq,
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    parsedCPULim,
									corev1.ResourceMemory: parsedMemLim,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "workspace",
									MountPath: "/workspace",
								},
								{
									Name:      "claude-settings",
									MountPath: "/root/.claude/settings.json",
									SubPath:   "settings.json",
								},
								{
									Name:      "hook-scripts",
									MountPath: "/hooks",
								},
							},
						},
					}, sidecarContainers...),
					Volumes: []corev1.Volume{
						{
							Name: "workspace",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "claude-settings",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: hooksConfigMapName,
									},
									Items: []corev1.KeyToPath{
										{Key: "settings.json", Path: "settings.json"},
									},
								},
							},
						},
						{
							Name: "hook-scripts",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: hooksConfigMapName,
									},
									DefaultMode: &defaultMode,
									Items: []corev1.KeyToPath{
										{Key: "conversation-sync.mjs", Path: "conversation-sync.mjs"},
									},
								},
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

	log.Info().Str("name", name).Msg("StatefulSet created")
	return nil
}

// UpdateStatefulSetImage patches the container image of a StatefulSet.
// K8s will automatically perform a rolling update of the pod.
func (m *Manager) UpdateStatefulSetImage(ctx context.Context, userID string, agentID int64, image string) error {
	name := m.statefulSetName(userID, agentID)

	sts, err := m.clientset.AppsV1().StatefulSets(m.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get statefulset %s: %w", name, err)
	}

	for i := range sts.Spec.Template.Spec.Containers {
		if sts.Spec.Template.Spec.Containers[i].Name == "claude-code" {
			sts.Spec.Template.Spec.Containers[i].Image = image
			break
		}
	}

	_, err = m.clientset.AppsV1().StatefulSets(m.namespace).Update(ctx, sts, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update statefulset %s image: %w", name, err)
	}

	log.Info().Str("name", name).Str("image", image).Msg("StatefulSet image updated")
	return nil
}

// ListStatefulSets lists all claude-code StatefulSets in the namespace.
func (m *Manager) ListStatefulSets(ctx context.Context) (*appsv1.StatefulSetList, error) {
	return m.clientset.AppsV1().StatefulSets(m.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=claude-code",
	})
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

// DeleteStatefulSet deletes the StatefulSet, its headless service, and any orphan pods.
// All steps tolerate NotFound so partial cleanups from previous attempts don't block deletion.
func (m *Manager) DeleteStatefulSet(ctx context.Context, userID string, agentID int64) error {
	name := m.statefulSetName(userID, agentID)

	// Delete StatefulSet
	err := m.clientset.AppsV1().StatefulSets(m.namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete statefulset: %w", err)
	}
	if err == nil {
		log.Info().Str("name", name).Msg("StatefulSet deleted")
	}

	// Delete headless service
	err = m.clientset.CoreV1().Services(m.namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		log.Warn().Err(err).Str("service", name).Msg("failed to delete headless service")
	} else if err == nil {
		log.Debug().Str("service", name).Msg("headless service deleted")
	}

	// Delete orphan pod (StatefulSet pod naming: {name}-0)
	podName := fmt.Sprintf("%s-0", name)
	err = m.clientset.CoreV1().Pods(m.namespace).Delete(ctx, podName, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		log.Warn().Err(err).Str("pod", podName).Msg("failed to delete pod")
	} else if err == nil {
		log.Debug().Str("pod", podName).Msg("pod deleted")
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
	PodName            string  `json:"pod_name"`
	Status             string  `json:"status"`
	RestartCount       int32   `json:"restart_count"`
	CPURequest         string  `json:"cpu_request"`
	CPULimit           string  `json:"cpu_limit"`
	MemoryRequest      string  `json:"memory_request"`
	MemoryLimit        string  `json:"memory_limit"`
	Image              string  `json:"image"`
	CPUUsage           string  `json:"cpu_usage"`
	MemoryUsage        string  `json:"memory_usage"`
	CPUUsagePercent    float64 `json:"cpu_usage_percent"`
	MemoryUsagePercent float64 `json:"memory_usage_percent"`
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
		PodName: podName,
		Status:  string(pod.Status.Phase),
	}

	// Pod with a deletion timestamp is being terminated — report accurately
	if pod.DeletionTimestamp != nil {
		info.Status = "Terminating"
	}

	// Read image and resource requests/limits from the first container
	if len(pod.Spec.Containers) > 0 {
		info.Image = pod.Spec.Containers[0].Image
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

	// Fetch real-time resource usage from Metrics Server (best-effort)
	m.enrichPodMetrics(ctx, podName, &info)

	return info
}

// enrichPodMetrics queries the Metrics Server for real-time CPU/memory usage
// and populates the usage fields in PodInfo. Fails silently if unavailable.
func (m *Manager) enrichPodMetrics(ctx context.Context, podName string, info *PodInfo) {
	if m.metricsClient == nil {
		return
	}

	podMetrics, err := m.metricsClient.MetricsV1beta1().PodMetricses(m.namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return
	}

	// Sum usage across all containers (claude-code + sidecar)
	var cpuTotal, memTotal int64
	for _, c := range podMetrics.Containers {
		cpuTotal += c.Usage.Cpu().MilliValue()
		memTotal += c.Usage.Memory().Value()
	}

	info.CPUUsage = fmt.Sprintf("%dm", cpuTotal)
	info.MemoryUsage = formatMemory(memTotal)

	// Calculate percentages against limits
	if info.CPULimit != "" {
		if lim, err := resource.ParseQuantity(info.CPULimit); err == nil && lim.MilliValue() > 0 {
			info.CPUUsagePercent = float64(cpuTotal) / float64(lim.MilliValue()) * 100
		}
	}
	if info.MemoryLimit != "" {
		if lim, err := resource.ParseQuantity(info.MemoryLimit); err == nil && lim.Value() > 0 {
			info.MemoryUsagePercent = float64(memTotal) / float64(lim.Value()) * 100
		}
	}
}

// formatMemory converts bytes to a human-readable string (Mi/Gi).
func formatMemory(bytes int64) string {
	const gi = 1024 * 1024 * 1024
	const mi = 1024 * 1024
	if bytes >= gi {
		return fmt.Sprintf("%.1fGi", float64(bytes)/float64(gi))
	}
	return fmt.Sprintf("%dMi", bytes/mi)
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

// RestartClaudeCodeProcess kills the Claude Code process in a pod, triggering auto-restart.
// The pod's entrypoint script (/tmp/claude-loop.sh) will automatically restart Claude Code.
func (m *Manager) RestartClaudeCodeProcess(ctx context.Context, podName string) error {
	// Use pkill with exact match to avoid killing other processes
	cmd := []string{"pkill", "-9", "-f", "^claude$"}
	_, stderr, err := m.ExecInPod(ctx, podName, cmd, nil)
	if err != nil {
		// pkill returns exit code 1 if no process matched, which is fine
		if !strings.Contains(stderr, "no process found") && stderr != "" {
			return fmt.Errorf("failed to restart Claude Code in pod %s: %w (stderr: %s)", podName, err, stderr)
		}
	}
	log.Debug().Str("pod", podName).Msg("restarted Claude Code process")
	return nil
}

// WaitForStatefulSetReady polls until the StatefulSet pod is Running.
func (m *Manager) WaitForStatefulSetReady(ctx context.Context, userID string, agentID int64, maxRetries int, retryInterval time.Duration) error {
	name := m.statefulSetName(userID, agentID)
	podName := fmt.Sprintf("%s-0", name)

	for i := 0; i < maxRetries; i++ {
		pod, err := m.clientset.CoreV1().Pods(m.namespace).Get(ctx, podName, metav1.GetOptions{})
		if err == nil && pod.Status.Phase == corev1.PodRunning {
			log.Info().Str("pod", podName).Msg("StatefulSet pod is ready")
			return nil
		}

		if i < maxRetries-1 {
			time.Sleep(retryInterval)
		}
	}

	return fmt.Errorf("timeout waiting for statefulset pod %s to be ready", podName)
}

// maintenancePodSpec builds a PodSpec for the maintenance Job/CronJob.
func (m *Manager) maintenancePodSpec(image string, envVars []corev1.EnvVar) corev1.PodSpec {
	return corev1.PodSpec{
		RestartPolicy: corev1.RestartPolicyNever,
		Containers: []corev1.Container{
			{
				Name:    "maintenance",
				Image:   image,
				Command: []string{"/app/maintenance"},
				Env:     envVars,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
			},
		},
	}
}

// EnsureCronJob creates or updates a CronJob with the given schedule.
func (m *Manager) EnsureCronJob(ctx context.Context, name, schedule, image string, envVars []corev1.EnvVar) error {
	podSpec := m.maintenancePodSpec(image, envVars)
	backoffLimit := int32(3)
	ttl := int32(300)

	cronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: m.namespace,
			Labels: map[string]string{
				"app": "maintenance",
			},
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          schedule,
			ConcurrencyPolicy: batchv1.ForbidConcurrent,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					BackoffLimit:            &backoffLimit,
					TTLSecondsAfterFinished: &ttl,
					Template: corev1.PodTemplateSpec{
						Spec: podSpec,
					},
				},
			},
		},
	}

	existing, err := m.clientset.BatchV1().CronJobs(m.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get cronjob %s: %w", name, err)
		}
		// Create
		_, err = m.clientset.BatchV1().CronJobs(m.namespace).Create(ctx, cronJob, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create cronjob %s: %w", name, err)
		}
		log.Info().Str("name", name).Str("schedule", schedule).Msg("CronJob created")
		return nil
	}

	// Update
	existing.Spec = cronJob.Spec
	_, err = m.clientset.BatchV1().CronJobs(m.namespace).Update(ctx, existing, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update cronjob %s: %w", name, err)
	}
	log.Info().Str("name", name).Str("schedule", schedule).Msg("CronJob updated")
	return nil
}

// CreateOneOffJob creates a single Job for manual skill sync trigger.
func (m *Manager) CreateOneOffJob(ctx context.Context, name, image string, envVars []corev1.EnvVar) error {
	podSpec := m.maintenancePodSpec(image, envVars)
	backoffLimit := int32(3)
	ttl := int32(300)

	jobName := fmt.Sprintf("%s-%d", name, time.Now().Unix())
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: m.namespace,
			Labels: map[string]string{
				"app": "maintenance",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				Spec: podSpec,
			},
		},
	}

	_, err := m.clientset.BatchV1().Jobs(m.namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create job %s: %w", jobName, err)
	}
	log.Info().Str("name", jobName).Msg("one-off Job created")
	return nil
}

// DeleteCronJob removes a CronJob, tolerating NotFound.
func (m *Manager) DeleteCronJob(ctx context.Context, name string) error {
	err := m.clientset.BatchV1().CronJobs(m.namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete cronjob %s: %w", name, err)
	}
	if err == nil {
		log.Info().Str("name", name).Msg("CronJob deleted")
	}
	return nil
}
