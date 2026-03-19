package model

func ExecutionProviderForK8sProvider(provider K8sProvider) ProviderKind {
	switch provider {
	case K8sProviderOrbStackLocal:
		return ProviderOrbStack
	case K8sProviderMinikube:
		return ProviderMinikube
	case K8sProviderK3s:
		return ProviderK3s
	case K8sProviderMicroK8s:
		return ProviderMicroK8s
	default:
		return ProviderNative
	}
}

func LegacyK8sProviderForExecutionProvider(provider ProviderKind) K8sProvider {
	switch provider {
	case ProviderOrbStack:
		return K8sProviderOrbStackLocal
	case ProviderMinikube:
		return K8sProviderMinikube
	case ProviderK3s:
		return K8sProviderK3s
	case ProviderMicroK8s:
		return K8sProviderMicroK8s
	default:
		return K8sProviderRemote
	}
}

func DefaultK8sContext(provider K8sProvider) string {
	switch provider {
	case K8sProviderOrbStackLocal:
		return "orbstack"
	case K8sProviderMinikube:
		return "minikube"
	default:
		return ""
	}
}

func K8sLocalPlatform(provider K8sProvider) string {
	if provider == K8sProviderOrbStackLocal {
		return "orbstack"
	}
	return ""
}
