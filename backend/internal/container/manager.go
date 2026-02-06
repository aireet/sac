package container

import (
	"context"
	"fmt"
	"log"
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

type Manager struct {
	clientset      *kubernetes.Clientset
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

// CreateDeployment creates a per-agent Claude Code deployment (1 replica)
func (m *Manager) CreateDeployment(ctx context.Context, userID string, agentID int64, agentConfig map[string]interface{}) error {
	deploymentName := fmt.Sprintf("claude-code-%s-%d", userID, agentID)
	imageFullPath := fmt.Sprintf("%s/%s", m.dockerRegistry, m.dockerImage)

	// Build environment variables
	envVars := []corev1.EnvVar{
		{
			Name:  "USER_ID",
			Value: userID,
		},
		{
			Name:  "AGENT_ID",
			Value: fmt.Sprintf("%d", agentID),
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

	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: m.namespace,
			Labels: map[string]string{
				"app":      "claude-code",
				"user-id":  userID,
				"agent-id": fmt.Sprintf("%d", agentID),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":      "claude-code",
					"user-id":  userID,
					"agent-id": fmt.Sprintf("%d", agentID),
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":      "claude-code",
						"user-id":  userID,
						"agent-id": fmt.Sprintf("%d", agentID),
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
			},
		},
	}

	_, err := m.clientset.AppsV1().Deployments(m.namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create deployment: %w", err)
	}

	log.Printf("Deployment %s created successfully", deploymentName)
	return nil
}

// GetDeployment gets the Claude Code deployment for a specific user and agent
func (m *Manager) GetDeployment(ctx context.Context, userID string, agentID int64) (*appsv1.Deployment, error) {
	deploymentName := fmt.Sprintf("claude-code-%s-%d", userID, agentID)

	deployment, err := m.clientset.AppsV1().Deployments(m.namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}

	return deployment, nil
}

// DeleteDeployment deletes the Claude Code deployment for a specific user and agent
func (m *Manager) DeleteDeployment(ctx context.Context, userID string, agentID int64) error {
	deploymentName := fmt.Sprintf("claude-code-%s-%d", userID, agentID)

	err := m.clientset.AppsV1().Deployments(m.namespace).Delete(ctx, deploymentName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete deployment: %w", err)
	}

	log.Printf("Deployment %s deleted successfully", deploymentName)
	return nil
}

// CreateService creates a ClusterIP service for Claude Code deployment
func (m *Manager) CreateService(ctx context.Context, userID string, agentID int64) error {
	serviceName := fmt.Sprintf("claude-code-%s-%d", userID, agentID)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: m.namespace,
			Labels: map[string]string{
				"app":      "claude-code",
				"user-id":  userID,
				"agent-id": fmt.Sprintf("%d", agentID),
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"app":      "claude-code",
				"user-id":  userID,
				"agent-id": fmt.Sprintf("%d", agentID),
			},
			Ports: []corev1.ServicePort{
				{
					Name:     "ttyd",
					Port:     7681,
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
	}

	_, err := m.clientset.CoreV1().Services(m.namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}

	log.Printf("Service %s created successfully", serviceName)
	return nil
}

// GetServiceClusterIP gets the ClusterIP of the Claude Code service
func (m *Manager) GetServiceClusterIP(ctx context.Context, userID string, agentID int64) (string, error) {
	serviceName := fmt.Sprintf("claude-code-%s-%d", userID, agentID)

	service, err := m.clientset.CoreV1().Services(m.namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get service: %w", err)
	}

	if service.Spec.ClusterIP == "" {
		return "", fmt.Errorf("service ClusterIP not yet assigned")
	}

	return service.Spec.ClusterIP, nil
}
