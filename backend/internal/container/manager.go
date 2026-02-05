package container

import (
	"context"
	"fmt"
	"log"
	"path/filepath"

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
func (m *Manager) CreatePod(ctx context.Context, userID, sessionID string) (*corev1.Pod, error) {
	podName := fmt.Sprintf("claude-code-%s-%s", userID, sessionID)

	imageFullPath := fmt.Sprintf("%s/%s", m.dockerRegistry, m.dockerImage)

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
					Env: []corev1.EnvVar{
						{
							Name:  "USER_ID",
							Value: userID,
						},
						{
							Name:  "SESSION_ID",
							Value: sessionID,
						},
					},
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
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: fmt.Sprintf("pvc-%s-%s", userID, sessionID),
						},
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
			Resources: corev1.ResourceRequirements{
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
