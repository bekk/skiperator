package texas

import (
	skiperatorv1alpha1 "github.com/kartverket/skiperator/api/v1alpha1"
	"github.com/kartverket/skiperator/api/v1alpha1/podtypes"
	"github.com/kartverket/skiperator/pkg/resourcegenerator/tokenx/jwker"
	"github.com/kartverket/skiperator/pkg/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func getTexasEnv(application *skiperatorv1alpha1.Application) ([]corev1.EnvVar, error) {
	env := []corev1.EnvVar{
		{Name: "BIND_ADDRESS", Value: "127.0.0.1:3000"},
		{Name: "DOWNSTREAM_APP_NAME", Value: application.Name},
		{Name: "DOWNSTREAM_APP_NAMESPACE", Value: application.Namespace},
		{Name: "DOWNSTREAM_APP_CLUSTER", Value: "kind-skiperator"},
		{Name: "TOKEN_X_ENABLED", Value: "true"},
	}

	secretName, err := jwker.GetJwkerSecretName(application.Name)
	if err != nil {
		return nil, err
	}

	tokenXEnvVars := jwker.GetJwkerEnvVariables(secretName)

	allEnvVars := append(env, tokenXEnvVars...)
	return allEnvVars, nil
}

func CreateTexasSidecarContainer(application *skiperatorv1alpha1.Application) corev1.Container {
	texasEnv, err := getTexasEnv(application)
	if err != nil {
		panic(err)
	}

	return corev1.Container{
		Name:            "texas",
		Image:           "ghcr.io/nais/texas:latest",
		ImagePullPolicy: corev1.PullAlways,
		Ports: []corev1.ContainerPort{
			{
				Name:          "main",
				ContainerPort: 3000,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		SecurityContext: &corev1.SecurityContext{
			Privileged:               util.PointTo(false),
			AllowPrivilegeEscalation: util.PointTo(false),
			ReadOnlyRootFilesystem:   util.PointTo(true),
			RunAsUser:                util.PointTo(util.SkiperatorUser),
			RunAsGroup:               util.PointTo(util.SkiperatorUser),
			RunAsNonRoot:             util.PointTo(true),
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{
					"NET_BIND_SERVICE",
				},
				Drop: []corev1.Capability{"ALL"},
			},
		},
		Env: texasEnv,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("64Mi"),
				corev1.ResourceCPU:    resource.MustParse("100m"),
			},
		},
		TerminationMessagePath:   corev1.TerminationMessagePathDefault,
		TerminationMessagePolicy: corev1.TerminationMessageReadFile,
	}
}

func TokenXSpecifiedInSpec(accessPolicy *podtypes.AccessPolicy) bool {
	return accessPolicy != nil && accessPolicy.TokenX
}

func TexasSpecifiedInSpec(accessPolicy *podtypes.AccessPolicy) bool {
	return accessPolicy != nil && accessPolicy.TokenX && accessPolicy.Texas
}
